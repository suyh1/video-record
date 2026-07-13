package testutil

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"video-record/internal/auth"
	"video-record/internal/storage"
)

var ErrInvalidSeedOptions = errors.New("invalid seed options")

const seedEventTimeLayout = "2006-01-02T15:04:05.000000000Z07:00"

type SeedOptions struct {
	Users       int
	MediaItems  int
	WatchEvents int
	Password    string
	Now         time.Time
}

type SeedResult struct {
	Username string
	UserIDs  []string
	MediaIDs []string
}

func Seed(ctx context.Context, db *storage.DB, options SeedOptions) (SeedResult, error) {
	if options.Users <= 0 || options.MediaItems < 0 || options.WatchEvents < 0 ||
		(options.WatchEvents > 0 && options.MediaItems == 0) || len(options.Password) < 12 || db == nil {
		return SeedResult{}, ErrInvalidSeedOptions
	}

	passwordHash, err := auth.HashPassword(options.Password)
	if err != nil {
		return SeedResult{}, fmt.Errorf("hash seed password: %w", err)
	}
	now := options.Now.UTC()
	if options.Now.IsZero() {
		now = time.Unix(0, 0).UTC()
	}

	tx, err := db.Writer().BeginTx(ctx, nil)
	if err != nil {
		return SeedResult{}, fmt.Errorf("begin seed transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	statements, err := prepareSeedStatements(ctx, tx)
	if err != nil {
		return SeedResult{}, err
	}
	defer statements.close()

	result := SeedResult{
		Username: "perf-user-0",
		UserIDs:  make([]string, 0, options.Users),
		MediaIDs: make([]string, 0, options.MediaItems),
	}
	for index := 0; index < options.Users; index++ {
		id := fmt.Sprintf("perf-user-id-%05d", index)
		role := auth.RoleMember
		if index == 0 {
			role = auth.RoleAdmin
		}
		if _, err := statements.user.ExecContext(
			ctx, id, fmt.Sprintf("perf-user-%d", index), passwordHash, role, now.UnixMilli(),
		); err != nil {
			return SeedResult{}, fmt.Errorf("seed user %d: %w", index, err)
		}
		result.UserIDs = append(result.UserIDs, id)
	}

	firstWatch := now
	if options.WatchEvents > 0 {
		firstWatch = now.Add(-time.Duration(options.WatchEvents-1) * time.Hour)
	}
	for index := 0; index < options.MediaItems; index++ {
		id := fmt.Sprintf("perf-media-%05d", index)
		title := fmt.Sprintf("Synthetic movie %05d", index)
		year := 2000 + index%25
		if _, err := statements.media.ExecContext(
			ctx, id, title, title, fmt.Sprintf("%04d-01-01", year), now.UnixMilli(), now.UnixMilli(),
		); err != nil {
			return SeedResult{}, fmt.Errorf("seed media %d: %w", index, err)
		}
		if _, err := statements.externalID.ExecContext(ctx, id, index+1); err != nil {
			return SeedResult{}, fmt.Errorf("seed media identity %d: %w", index, err)
		}
		if _, err := statements.state.ExecContext(
			ctx, result.UserIDs[0], id, firstWatch.Format(seedEventTimeLayout),
			now.Format(seedEventTimeLayout), now.UnixMilli(),
		); err != nil {
			return SeedResult{}, fmt.Errorf("seed media state %d: %w", index, err)
		}
		result.MediaIDs = append(result.MediaIDs, id)
	}

	for index := 0; index < options.WatchEvents; index++ {
		eventID := fmt.Sprintf("perf-event-%05d", index)
		watchedAt := firstWatch.Add(time.Duration(index) * time.Hour)
		if _, err := statements.event.ExecContext(
			ctx, eventID, result.UserIDs[0], result.MediaIDs[index%len(result.MediaIDs)],
			watchedAt.Format(seedEventTimeLayout), now.UnixMilli(),
		); err != nil {
			return SeedResult{}, fmt.Errorf("seed watch event %d: %w", index, err)
		}
		if _, err := statements.participant.ExecContext(ctx, eventID, result.UserIDs[0]); err != nil {
			return SeedResult{}, fmt.Errorf("seed event participant %d: %w", index, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return SeedResult{}, fmt.Errorf("commit seed transaction: %w", err)
	}
	return result, nil
}

type seedStatements struct {
	user        *sql.Stmt
	media       *sql.Stmt
	externalID  *sql.Stmt
	state       *sql.Stmt
	event       *sql.Stmt
	participant *sql.Stmt
}

func prepareSeedStatements(ctx context.Context, tx *sql.Tx) (seedStatements, error) {
	queries := []string{
		`INSERT INTO users (id, username, password_hash, role, active, created_at)
		 VALUES (?, ?, ?, ?, 1, ?)`,
		`INSERT INTO media_items (
			id, media_type, external_title, original_title, release_date,
			external_overview, poster_path, backdrop_path, created_at, updated_at
		 ) VALUES (?, 'movie', ?, ?, ?, '', '', '', ?, ?)`,
		`INSERT INTO media_external_ids (media_id, source, source_id, media_type)
		 VALUES (?, 'tmdb', CAST(? AS TEXT), 'movie')`,
		`INSERT INTO user_media_states (
			user_id, media_id, status, started_at, completed_at, version,
			status_source, rating_source, note_source, updated_at
		 ) VALUES (?, ?, 'completed', ?, ?, 1,
			'external_default', 'external_default', 'external_default', ?)`,
		`INSERT INTO watch_events (
			id, created_by_user_id, media_id, watched_at, viewing_method,
			source, completion, created_at
		 ) VALUES (?, ?, ?, ?, 'Synthetic', 'external_default', 100, ?)`,
		`INSERT INTO watch_event_participants (event_id, user_id) VALUES (?, ?)`,
	}
	prepared := make([]*sql.Stmt, 0, len(queries))
	for _, query := range queries {
		statement, err := tx.PrepareContext(ctx, query)
		if err != nil {
			for _, existing := range prepared {
				_ = existing.Close()
			}
			return seedStatements{}, fmt.Errorf("prepare seed statement: %w", err)
		}
		prepared = append(prepared, statement)
	}
	return seedStatements{
		user: prepared[0], media: prepared[1], externalID: prepared[2],
		state: prepared[3], event: prepared[4], participant: prepared[5],
	}, nil
}

func (statements seedStatements) close() {
	for _, statement := range []*sql.Stmt{
		statements.user, statements.media, statements.externalID,
		statements.state, statements.event, statements.participant,
	} {
		_ = statement.Close()
	}
}
