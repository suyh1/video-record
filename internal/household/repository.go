package household

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"video-record/internal/storage"
)

const watchTimeLayout = "2006-01-02T15:04:05.000000000Z07:00"

type SQLiteRepository struct {
	db *storage.DB
}

func NewRepository(db *storage.DB) *SQLiteRepository {
	return &SQLiteRepository{db: db}
}

func authID() string {
	return uuid.NewString()
}

func (repository *SQLiteRepository) FindMember(ctx context.Context, id string) (Member, error) {
	var member Member
	var active int
	var createdAt int64
	err := repository.db.Reader().QueryRowContext(ctx, `
		SELECT id, username, role, active, created_at FROM users WHERE id = ?
	`, id).Scan(&member.ID, &member.Username, &member.Role, &active, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Member{}, ErrMemberNotFound
	}
	if err != nil {
		return Member{}, err
	}
	member.Active = active == 1
	member.CreatedAt = time.UnixMilli(createdAt).UTC()
	return member, nil
}

func (repository *SQLiteRepository) Members(ctx context.Context) ([]Member, error) {
	rows, err := repository.db.Reader().QueryContext(ctx, `
		SELECT id, username, role, active, created_at
		FROM users ORDER BY CASE role WHEN 'admin' THEN 0 ELSE 1 END, username COLLATE NOCASE
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	members := make([]Member, 0)
	for rows.Next() {
		var member Member
		var active int
		var createdAt int64
		if err := rows.Scan(&member.ID, &member.Username, &member.Role, &active, &createdAt); err != nil {
			return nil, err
		}
		member.Active = active == 1
		member.CreatedAt = time.UnixMilli(createdAt).UTC()
		members = append(members, member)
	}
	return members, rows.Err()
}

func (repository *SQLiteRepository) CreateMember(
	ctx context.Context,
	member Member,
	passwordHash, actorID string,
) error {
	tx, err := repository.db.Writer().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO users (id, username, password_hash, role, active, created_at)
		VALUES (?, ?, ?, ?, 1, ?)
	`, member.ID, member.Username, passwordHash, member.Role, member.CreatedAt.UnixMilli()); err != nil {
		return err
	}
	if err := insertAudit(ctx, tx, actorID, "member.create", member.ID, member.CreatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (repository *SQLiteRepository) ResetPassword(
	ctx context.Context,
	actorID, targetID, passwordHash string,
	now time.Time,
) error {
	tx, err := repository.db.Writer().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, "UPDATE users SET password_hash = ? WHERE id = ? AND active = 1", passwordHash, targetID)
	if err != nil {
		return err
	}
	if rows, err := result.RowsAffected(); err != nil || rows != 1 {
		return ErrMemberNotFound
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE sessions SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL
	`, now.UnixMilli(), targetID); err != nil {
		return err
	}
	if err := insertAudit(ctx, tx, actorID, "member.reset_password", targetID, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (repository *SQLiteRepository) DeactivateMember(
	ctx context.Context,
	actorID, targetID string,
	now time.Time,
) error {
	tx, err := repository.db.Writer().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, "UPDATE users SET active = 0 WHERE id = ? AND active = 1", targetID)
	if err != nil {
		return err
	}
	if rows, err := result.RowsAffected(); err != nil || rows != 1 {
		return ErrMemberNotFound
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE sessions SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL
	`, now.UnixMilli(), targetID); err != nil {
		return err
	}
	if err := insertAudit(ctx, tx, actorID, "member.deactivate", targetID, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (repository *SQLiteRepository) RecordPrivacy(ctx context.Context, ownerID, mediaID string) (recordPrivacy, error) {
	var record recordPrivacy
	var rating sql.NullInt64
	var note, sharedReview sql.NullString
	var shareRating, shareReview int
	err := repository.db.Reader().QueryRowContext(ctx, `
		SELECT current.rating, current.note,
		       profile.share_rating, profile.share_review, profile.shared_review, profile.version
		FROM user_media_profiles profile
		LEFT JOIN watch_rounds current ON current.id = (
			SELECT candidate.id FROM watch_rounds candidate
			WHERE candidate.user_id = profile.user_id AND candidate.media_id = profile.media_id
			  AND candidate.archived_at IS NULL
			ORDER BY candidate.updated_at DESC, candidate.season_number DESC, candidate.id DESC
			LIMIT 1
		)
		WHERE profile.user_id = ? AND profile.media_id = ?
	`, ownerID, mediaID).Scan(&rating, &note, &shareRating, &shareReview, &sharedReview, &record.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return recordPrivacy{}, ErrRecordNotFound
	}
	if err != nil {
		return recordPrivacy{}, err
	}
	record.OwnerID, record.MediaID = ownerID, mediaID
	record.ShareRating, record.ShareReview = shareRating == 1, shareReview == 1
	if rating.Valid {
		value := int(rating.Int64)
		record.Rating = &value
	}
	if note.Valid {
		value := note.String
		record.PrivateNote = &value
	}
	if sharedReview.Valid {
		value := sharedReview.String
		record.SharedReview = &value
	}
	return record, nil
}

func (repository *SQLiteRepository) UpdateSharing(
	ctx context.Context,
	ownerID, mediaID string,
	input SharingInput,
) (recordPrivacy, error) {
	var review any
	if input.ShareReview {
		review = input.SharedReview
	}
	result, err := repository.db.Writer().ExecContext(ctx, `
		UPDATE user_media_profiles SET
			share_rating = ?, share_review = ?, shared_review = ?,
			version = version + 1, updated_at = strftime('%s', 'now') * 1000
		WHERE user_id = ? AND media_id = ? AND version = ?
	`, boolInt(input.ShareRating), boolInt(input.ShareReview), review, ownerID, mediaID, input.ExpectedVersion)
	if err != nil {
		return recordPrivacy{}, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return recordPrivacy{}, err
	}
	if rows != 1 {
		var exists int
		if err := repository.db.Reader().QueryRowContext(ctx, `
			SELECT COUNT(*) FROM user_media_profiles WHERE user_id = ? AND media_id = ?
		`, ownerID, mediaID).Scan(&exists); err != nil {
			return recordPrivacy{}, err
		}
		if exists == 0 {
			return recordPrivacy{}, ErrRecordNotFound
		}
		return recordPrivacy{}, ErrVersionConflict
	}
	return repository.RecordPrivacy(ctx, ownerID, mediaID)
}

func (repository *SQLiteRepository) SharedEvents(ctx context.Context, viewerID string) ([]SharedEvent, error) {
	rows, err := repository.db.Reader().QueryContext(ctx, `
		SELECT event.id, event.media_id, COALESCE(media.custom_title, media.external_title),
		       event.watched_at, participant_user.username
		FROM watch_events event
		JOIN watch_event_participants viewer
		  ON viewer.event_id = event.id AND viewer.user_id = ?
		JOIN media_items media ON media.id = event.media_id
		JOIN watch_event_participants participant ON participant.event_id = event.id
		JOIN users participant_user ON participant_user.id = participant.user_id
		WHERE (SELECT COUNT(*) FROM watch_event_participants counted WHERE counted.event_id = event.id) > 1
		ORDER BY event.watched_at DESC, event.id, participant_user.username
	`, viewerID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	events := make([]SharedEvent, 0)
	for rows.Next() {
		var event SharedEvent
		var watchedAt, participant string
		if err := rows.Scan(&event.ID, &event.MediaID, &event.Title, &watchedAt, &participant); err != nil {
			return nil, err
		}
		event.WatchedAt, err = time.Parse(watchTimeLayout, watchedAt)
		if err != nil {
			return nil, err
		}
		if len(events) > 0 && events[len(events)-1].ID == event.ID {
			events[len(events)-1].Participants = append(events[len(events)-1].Participants, participant)
			continue
		}
		event.Participants = []string{participant}
		events = append(events, event)
	}
	return events, rows.Err()
}

func insertAudit(ctx context.Context, tx *sql.Tx, actorID, action, targetID string, now time.Time) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO audit_events (id, actor_user_id, action, target_type, target_id, metadata_json, created_at)
		VALUES (?, ?, ?, 'user', ?, '{}', ?)
	`, uuid.NewString(), actorID, action, targetID, now.UnixMilli())
	return err
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
