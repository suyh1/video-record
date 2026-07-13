package records

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/google/uuid"

	"video-record/internal/storage"
)

type Repository interface {
	FindState(context.Context, string, string) (State, bool, error)
	InsertState(context.Context, State) error
	UpdateState(context.Context, State, int) (bool, error)
	SetTags(context.Context, string, string, []string) error
	Tags(context.Context, string, string) ([]string, error)
	CreateCollection(context.Context, string, string) (Collection, error)
	AddCollectionItem(context.Context, string, string, string) error
	Collections(context.Context, string) ([]Collection, error)
}

type SQLiteRepository struct {
	db *storage.DB
}

func NewRepository(db *storage.DB) *SQLiteRepository {
	return &SQLiteRepository{db: db}
}

func (repository *SQLiteRepository) FindState(ctx context.Context, userID, mediaID string) (State, bool, error) {
	var state State
	var rating sql.NullInt64
	var note sql.NullString
	err := repository.db.Reader().QueryRowContext(ctx, `
		SELECT status, rating, note, version, status_source, rating_source, note_source
		FROM user_media_states WHERE user_id = ? AND media_id = ?
	`, userID, mediaID).Scan(
		&state.Status, &rating, &note, &state.Version,
		&state.StatusSource, &state.RatingSource, &state.NoteSource,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return State{UserID: userID, MediaID: mediaID, Status: StatusNone}, false, nil
	}
	if err != nil {
		return State{}, false, err
	}
	state.UserID, state.MediaID = userID, mediaID
	if rating.Valid {
		value := int(rating.Int64)
		state.Rating = &value
	}
	if note.Valid {
		value := note.String
		state.Note = &value
	}
	return state, true, nil
}

func (repository *SQLiteRepository) InsertState(ctx context.Context, state State) error {
	_, err := repository.db.Writer().ExecContext(ctx, `
		INSERT INTO user_media_states (
			user_id, media_id, status, rating, note, version,
			status_source, rating_source, note_source, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, strftime('%s', 'now') * 1000)
	`, state.UserID, state.MediaID, state.Status, nullableInt(state.Rating), nullableText(state.Note),
		state.Version, state.StatusSource, state.RatingSource, state.NoteSource)
	return err
}

func (repository *SQLiteRepository) UpdateState(ctx context.Context, state State, expectedVersion int) (bool, error) {
	result, err := repository.db.Writer().ExecContext(ctx, `
		UPDATE user_media_states SET
			status = ?, rating = ?, note = ?, version = ?,
			status_source = ?, rating_source = ?, note_source = ?,
			updated_at = strftime('%s', 'now') * 1000
		WHERE user_id = ? AND media_id = ? AND version = ?
	`, state.Status, nullableInt(state.Rating), nullableText(state.Note), state.Version,
		state.StatusSource, state.RatingSource, state.NoteSource,
		state.UserID, state.MediaID, expectedVersion)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

func (repository *SQLiteRepository) SetTags(ctx context.Context, userID, mediaID string, names []string) error {
	tx, err := repository.db.Writer().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, "DELETE FROM user_media_tags WHERE user_id = ? AND media_id = ?", userID, mediaID); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(names))
	for _, rawName := range names {
		name := strings.TrimSpace(rawName)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO tags (id, user_id, name) VALUES (?, ?, ?)
			ON CONFLICT(user_id, name) DO NOTHING
		`, uuid.NewString(), userID, name); err != nil {
			return err
		}
		var tagID string
		if err := tx.QueryRowContext(ctx, "SELECT id FROM tags WHERE user_id = ? AND name = ?", userID, name).Scan(&tagID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO user_media_tags (user_id, media_id, tag_id) VALUES (?, ?, ?)
		`, userID, mediaID, tagID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (repository *SQLiteRepository) Tags(ctx context.Context, userID, mediaID string) ([]string, error) {
	rows, err := repository.db.Reader().QueryContext(ctx, `
		SELECT t.name FROM tags t
		JOIN user_media_tags umt ON umt.tag_id = t.id AND umt.user_id = t.user_id
		WHERE umt.user_id = ? AND umt.media_id = ?
		ORDER BY t.name
	`, userID, mediaID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

func (repository *SQLiteRepository) CreateCollection(ctx context.Context, userID, name string) (Collection, error) {
	collection := Collection{ID: uuid.NewString(), UserID: userID, Name: name, Items: []string{}}
	_, err := repository.db.Writer().ExecContext(ctx, `
		INSERT INTO collections (id, user_id, name) VALUES (?, ?, ?)
	`, collection.ID, collection.UserID, collection.Name)
	return collection, err
}

func (repository *SQLiteRepository) AddCollectionItem(ctx context.Context, userID, collectionID, mediaID string) error {
	tx, err := repository.db.Writer().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var owned int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM collections WHERE id = ? AND user_id = ?
	`, collectionID, userID).Scan(&owned); err != nil {
		return err
	}
	if owned == 0 {
		return ErrCollectionNotFound
	}
	var position int
	err = tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(position) + 1, 0) FROM collection_items WHERE collection_id = ?
	`, collectionID).Scan(&position)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO collection_items (collection_id, media_id, position)
		VALUES (?, ?, ?)
		ON CONFLICT(collection_id, media_id) DO NOTHING
	`, collectionID, mediaID, position)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		var exists int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM collection_items WHERE collection_id = ? AND media_id = ?
		`, collectionID, mediaID).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return ErrCollectionNotFound
		}
	}
	return tx.Commit()
}

func (repository *SQLiteRepository) Collections(ctx context.Context, userID string) ([]Collection, error) {
	rows, err := repository.db.Reader().QueryContext(ctx, `
		SELECT c.id, c.name, ci.media_id
		FROM collections c
		LEFT JOIN collection_items ci ON ci.collection_id = c.id
		WHERE c.user_id = ?
		ORDER BY c.name, ci.position
	`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	byID := make(map[string]*Collection)
	var ordered []string
	for rows.Next() {
		var id, name string
		var mediaID sql.NullString
		if err := rows.Scan(&id, &name, &mediaID); err != nil {
			return nil, err
		}
		collection := byID[id]
		if collection == nil {
			collection = &Collection{ID: id, UserID: userID, Name: name, Items: []string{}}
			byID[id] = collection
			ordered = append(ordered, id)
		}
		if mediaID.Valid {
			collection.Items = append(collection.Items, mediaID.String)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	collections := make([]Collection, 0, len(ordered))
	for _, id := range ordered {
		collections = append(collections, *byID[id])
	}
	return collections, nil
}

func nullableInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableText(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}
