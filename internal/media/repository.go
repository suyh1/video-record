package media

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"video-record/internal/storage"
)

type Repository interface {
	UpsertExternal(context.Context, ExternalSnapshot) (Item, error)
	CreateCustom(context.Context, CreateCustomInput) (Item, error)
	LinkExternal(context.Context, string, ExternalSnapshot) (Item, error)
	FindByID(context.Context, string) (Item, error)
}

type SQLiteRepository struct {
	db *storage.DB
}

func NewRepository(db *storage.DB) *SQLiteRepository {
	return &SQLiteRepository{db: db}
}

func (repository *SQLiteRepository) UpsertExternal(ctx context.Context, snapshot ExternalSnapshot) (Item, error) {
	tx, err := repository.db.Writer().BeginTx(ctx, nil)
	if err != nil {
		return Item{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var itemID string
	err = tx.QueryRowContext(ctx, `
		SELECT media_id FROM media_external_ids
		WHERE source = ? AND source_id = ? AND media_type = ?
	`, snapshot.Source, snapshot.SourceID, snapshot.MediaType).Scan(&itemID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		itemID = uuid.NewString()
		now := time.Now().UTC().UnixMilli()
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO media_items (
				id, media_type, external_title, original_title, release_date,
				external_overview, poster_path, backdrop_path, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, itemID, snapshot.MediaType, snapshot.Title, snapshot.OriginalTitle,
			snapshot.ReleaseDate, snapshot.Overview, snapshot.PosterPath, snapshot.BackdropPath,
			now, now); err != nil {
			return Item{}, err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO media_external_ids (media_id, source, source_id, media_type)
			VALUES (?, ?, ?, ?)
		`, itemID, snapshot.Source, snapshot.SourceID, snapshot.MediaType); err != nil {
			return Item{}, err
		}
	case err != nil:
		return Item{}, err
	default:
		if err := updateExternalSnapshot(ctx, tx, itemID, snapshot); err != nil {
			return Item{}, err
		}
	}

	item, err := findItem(ctx, tx, itemID)
	if err != nil {
		return Item{}, err
	}
	if err := tx.Commit(); err != nil {
		return Item{}, err
	}
	return item, nil
}

func (repository *SQLiteRepository) CreateCustom(ctx context.Context, input CreateCustomInput) (Item, error) {
	id := uuid.NewString()
	now := time.Now().UTC().UnixMilli()
	var overview any
	if input.Overview != "" {
		overview = input.Overview
	}
	_, err := repository.db.Writer().ExecContext(ctx, `
		INSERT INTO media_items (
			id, media_type, external_title, original_title, release_date,
			external_overview, poster_path, backdrop_path,
			custom_title, custom_overview, custom_year, created_at, updated_at
		) VALUES (?, ?, '', '', '', '', '', '', ?, ?, ?, ?, ?)
	`, id, input.MediaType, input.Title, overview, nullableString(input.Year), now, now)
	if err != nil {
		return Item{}, err
	}
	return repository.FindByID(ctx, id)
}

func (repository *SQLiteRepository) LinkExternal(ctx context.Context, itemID string, snapshot ExternalSnapshot) (Item, error) {
	tx, err := repository.db.Writer().BeginTx(ctx, nil)
	if err != nil {
		return Item{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var mediaType MediaType
	if err := tx.QueryRowContext(ctx, "SELECT media_type FROM media_items WHERE id = ?", itemID).Scan(&mediaType); err != nil {
		return Item{}, err
	}
	if mediaType != snapshot.MediaType {
		return Item{}, ErrMediaTypeMismatch
	}
	var ownerID string
	err = tx.QueryRowContext(ctx, `
		SELECT media_id FROM media_external_ids
		WHERE source = ? AND source_id = ? AND media_type = ?
	`, snapshot.Source, snapshot.SourceID, snapshot.MediaType).Scan(&ownerID)
	if err == nil && ownerID != itemID {
		return Item{}, ErrExternalIdentityConflict
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Item{}, err
	}
	if errors.Is(err, sql.ErrNoRows) {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO media_external_ids (media_id, source, source_id, media_type)
			VALUES (?, ?, ?, ?)
		`, itemID, snapshot.Source, snapshot.SourceID, snapshot.MediaType); err != nil {
			return Item{}, err
		}
	}
	if err := updateExternalSnapshot(ctx, tx, itemID, snapshot); err != nil {
		return Item{}, err
	}
	item, err := findItem(ctx, tx, itemID)
	if err != nil {
		return Item{}, err
	}
	if err := tx.Commit(); err != nil {
		return Item{}, err
	}
	return item, nil
}

func (repository *SQLiteRepository) FindByID(ctx context.Context, id string) (Item, error) {
	return findItem(ctx, repository.db.Reader(), id)
}

func updateExternalSnapshot(ctx context.Context, tx *sql.Tx, itemID string, snapshot ExternalSnapshot) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE media_items SET
			external_title = ?, original_title = ?, release_date = ?, external_overview = ?,
			poster_path = ?, backdrop_path = ?, updated_at = ?
		WHERE id = ?
	`, snapshot.Title, snapshot.OriginalTitle, snapshot.ReleaseDate, snapshot.Overview,
		snapshot.PosterPath, snapshot.BackdropPath, time.Now().UTC().UnixMilli(), itemID)
	return err
}

func findItem(ctx context.Context, query interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id string) (Item, error) {
	var item Item
	err := query.QueryRowContext(ctx, `
		SELECT
			id, media_type,
			COALESCE(custom_title, external_title),
			COALESCE(custom_overview, external_overview),
			external_title, external_overview, original_title, release_date,
			poster_path, backdrop_path
		FROM media_items WHERE id = ?
	`, id).Scan(
		&item.ID, &item.MediaType, &item.Title, &item.Overview,
		&item.ExternalTitle, &item.ExternalOverview, &item.OriginalTitle,
		&item.ReleaseDate, &item.PosterPath, &item.BackdropPath,
	)
	return item, err
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
