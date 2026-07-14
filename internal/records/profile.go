package records

import (
	"context"
	"database/sql"
	"errors"
)

type MediaProfile struct {
	UserID       string
	MediaID      string
	Status       Status
	Version      int
	ShareRating  bool
	ShareReview  bool
	SharedReview string
}

func (repository *SQLiteRepository) FindProfile(
	ctx context.Context,
	userID, mediaID string,
) (MediaProfile, bool, error) {
	var profile MediaProfile
	var shareRating, shareReview int
	var sharedReview sql.NullString
	err := repository.db.Reader().QueryRowContext(ctx, `
		SELECT status, version, share_rating, share_review, shared_review
		FROM user_media_profiles WHERE user_id = ? AND media_id = ?
	`, userID, mediaID).Scan(
		&profile.Status, &profile.Version, &shareRating, &shareReview, &sharedReview,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return MediaProfile{UserID: userID, MediaID: mediaID, Status: StatusNone}, false, nil
	}
	if err != nil {
		return MediaProfile{}, false, err
	}
	profile.UserID, profile.MediaID = userID, mediaID
	profile.ShareRating, profile.ShareReview = shareRating == 1, shareReview == 1
	if sharedReview.Valid {
		profile.SharedReview = sharedReview.String
	}
	return profile, true, nil
}

func ensureMediaProfile(ctx context.Context, tx *sql.Tx, userID, mediaID string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO user_media_profiles (
			user_id, media_id, status, version, share_rating, share_review, updated_at
		) VALUES (?, ?, 'none', 1, 0, 0, strftime('%s', 'now') * 1000)
		ON CONFLICT(user_id, media_id) DO NOTHING
	`, userID, mediaID)
	return err
}

func projectMediaProfile(ctx context.Context, tx *sql.Tx, userID, mediaID string) error {
	var status Status
	err := tx.QueryRowContext(ctx, `
		SELECT status FROM watch_rounds
		WHERE user_id = ? AND media_id = ? AND archived_at IS NULL
		ORDER BY CASE WHEN status = 'watching' THEN 0 ELSE 1 END,
		         updated_at DESC, season_number DESC, id DESC
		LIMIT 1
	`, userID, mediaID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		status = StatusNone
	} else if err != nil {
		return err
	}
	if err := ensureMediaProfile(ctx, tx, userID, mediaID); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE user_media_profiles
		SET status = ?, updated_at = strftime('%s', 'now') * 1000
		WHERE user_id = ? AND media_id = ?
	`, status, userID, mediaID)
	return err
}
