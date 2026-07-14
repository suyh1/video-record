package records

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"video-record/internal/storage"
)

type Repository interface {
	FindState(context.Context, string, string) (State, bool, error)
	InsertState(context.Context, State) error
	UpdateState(context.Context, State, int) (bool, error)
	ApplyStateAndEvent(context.Context, State, int, bool, WatchEvent) (bool, error)
	CreateWatchEvent(context.Context, WatchEvent) error
	WatchEvents(context.Context, string, string) ([]WatchEvent, error)
	DeleteWatchEvent(context.Context, string, string) error
	Library(context.Context, string, Status) ([]CatalogItem, error)
	SearchMedia(context.Context, string, string) ([]CatalogItem, error)
	CalendarEvents(context.Context, string, time.Time, time.Time, CalendarFilter) ([]CalendarEvent, error)
	Episodes(context.Context, string, string) (SeriesProgress, error)
	ApplyEpisodeProgress(context.Context, EpisodeProgressInput, []string, bool) (bool, error)
	ApplyExternalEpisodeProgress(context.Context, EpisodeProgressInput, []EpisodeReference, bool) (bool, error)
	SetTags(context.Context, string, string, []string) error
	SetTagsVersioned(context.Context, string, string, []string, int) (bool, error)
	Tags(context.Context, string, string) ([]string, error)
	CreateCollection(context.Context, string, string) (Collection, error)
	AddCollectionItem(context.Context, string, string, string) error
	ReplaceCollectionItems(context.Context, string, string, []string) error
	Collections(context.Context, string) ([]Collection, error)
	ExportDocument(context.Context, string) (exportDocument, error)
	ImportDocument(context.Context, string, exportDocument) (ImportReport, error)
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
	var startedAt, completedAt sql.NullString
	err := repository.db.Reader().QueryRowContext(ctx, `
		SELECT status, rating, note, version, status_source, rating_source, note_source,
		       started_at, completed_at
		FROM user_media_states WHERE user_id = ? AND media_id = ?
	`, userID, mediaID).Scan(
		&state.Status, &rating, &note, &state.Version,
		&state.StatusSource, &state.RatingSource, &state.NoteSource,
		&startedAt, &completedAt,
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
	state.StartedAt, err = parseNullableEventTime(startedAt)
	if err != nil {
		return State{}, false, err
	}
	state.CompletedAt, err = parseNullableEventTime(completedAt)
	if err != nil {
		return State{}, false, err
	}
	return state, true, nil
}

func (repository *SQLiteRepository) InsertState(ctx context.Context, state State) error {
	return insertState(ctx, repository.db.Writer(), state)
}

func insertState(ctx context.Context, executor sqlExecutor, state State) error {
	_, err := executor.ExecContext(ctx, `
		INSERT INTO user_media_states (
			user_id, media_id, status, rating, note, version,
			status_source, rating_source, note_source, started_at, completed_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, strftime('%s', 'now') * 1000)
	`, state.UserID, state.MediaID, state.Status, nullableInt(state.Rating), nullableText(state.Note),
		state.Version, state.StatusSource, state.RatingSource, state.NoteSource,
		nullableEventTime(state.StartedAt), nullableEventTime(state.CompletedAt))
	return err
}

func (repository *SQLiteRepository) UpdateState(ctx context.Context, state State, expectedVersion int) (bool, error) {
	return updateState(ctx, repository.db.Writer(), state, expectedVersion)
}

func updateState(ctx context.Context, executor sqlExecutor, state State, expectedVersion int) (bool, error) {
	result, err := executor.ExecContext(ctx, `
		UPDATE user_media_states SET
			status = ?, rating = ?, note = ?, version = ?,
			status_source = ?, rating_source = ?, note_source = ?,
			started_at = ?, completed_at = ?,
			updated_at = strftime('%s', 'now') * 1000
		WHERE user_id = ? AND media_id = ? AND version = ?
	`, state.Status, nullableInt(state.Rating), nullableText(state.Note), state.Version,
		state.StatusSource, state.RatingSource, state.NoteSource,
		nullableEventTime(state.StartedAt), nullableEventTime(state.CompletedAt),
		state.UserID, state.MediaID, expectedVersion)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

type sqlExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func (repository *SQLiteRepository) ApplyStateAndEvent(
	ctx context.Context,
	state State,
	expectedVersion int,
	exists bool,
	event WatchEvent,
) (bool, error) {
	tx, err := repository.db.Writer().BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if exists {
		updated, err := updateState(ctx, tx, state, expectedVersion)
		if err != nil || !updated {
			return updated, err
		}
	} else if err := insertState(ctx, tx, state); err != nil {
		return false, err
	}
	if err := insertWatchEvent(ctx, tx, event); err != nil {
		return false, err
	}
	if err := recomputeWatchDates(ctx, tx, event.UserID, event.MediaID); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (repository *SQLiteRepository) CreateWatchEvent(ctx context.Context, event WatchEvent) error {
	tx, err := repository.db.Writer().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := insertWatchEvent(ctx, tx, event); err != nil {
		return err
	}
	if err := recomputeWatchDates(ctx, tx, event.UserID, event.MediaID); err != nil {
		return err
	}
	return tx.Commit()
}

func insertWatchEvent(ctx context.Context, tx *sql.Tx, event WatchEvent) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO watch_events (
			id, created_by_user_id, media_id, episode_id, watched_at,
			viewing_method, source, external_event_id, completion, note, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, strftime('%s', 'now') * 1000)
	`, event.ID, event.UserID, event.MediaID, nullableString(event.EpisodeID), formatEventTime(event.WatchedAt),
		nullableString(event.ViewingMethod), event.Source, nullableString(event.ExternalEventID),
		event.Completion, nullableString(event.Note)); err != nil {
		return err
	}
	for _, participantID := range event.ParticipantIDs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO watch_event_participants (event_id, user_id) VALUES (?, ?)
		`, event.ID, participantID); err != nil {
			return err
		}
	}
	return nil
}

func (repository *SQLiteRepository) WatchEvents(ctx context.Context, userID, mediaID string) ([]WatchEvent, error) {
	rows, err := repository.db.Reader().QueryContext(ctx, `
		SELECT we.id, we.created_by_user_id, we.media_id, we.episode_id, we.watched_at,
		       we.viewing_method, we.source, we.external_event_id, we.completion, we.note
		FROM watch_events we
		JOIN watch_event_participants participant ON participant.event_id = we.id
		WHERE participant.user_id = ? AND we.media_id = ?
		ORDER BY we.watched_at, we.id
	`, userID, mediaID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	events := make([]WatchEvent, 0)
	for rows.Next() {
		var event WatchEvent
		var episodeID, viewingMethod, externalEventID, note sql.NullString
		var watchedAt string
		if err := rows.Scan(
			&event.ID, &event.UserID, &event.MediaID, &episodeID, &watchedAt,
			&viewingMethod, &event.Source, &externalEventID, &event.Completion, &note,
		); err != nil {
			return nil, err
		}
		event.WatchedAt, err = time.Parse(eventTimeLayout, watchedAt)
		if err != nil {
			return nil, err
		}
		event.EpisodeID = episodeID.String
		event.ViewingMethod = viewingMethod.String
		event.ExternalEventID = externalEventID.String
		event.Note = note.String
		events = append(events, event)
	}
	return events, rows.Err()
}

func (repository *SQLiteRepository) DeleteWatchEvent(ctx context.Context, userID, eventID string) error {
	tx, err := repository.db.Writer().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var mediaID string
	var episodeID sql.NullString
	err = tx.QueryRowContext(ctx, `
		SELECT media_id, episode_id FROM watch_events WHERE id = ? AND created_by_user_id = ?
	`, eventID, userID).Scan(&mediaID, &episodeID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrWatchEventNotFound
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM watch_events WHERE id = ?", eventID); err != nil {
		return err
	}
	if err := recomputeWatchDates(ctx, tx, userID, mediaID); err != nil {
		return err
	}
	if episodeID.Valid {
		if err := reprojectSeriesStateAfterEventDeletion(ctx, tx, userID, mediaID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func recomputeWatchDates(ctx context.Context, tx *sql.Tx, userID, mediaID string) error {
	var startedAt, completedAt sql.NullString
	if err := tx.QueryRowContext(ctx, `
		SELECT MIN(we.watched_at), MAX(we.watched_at)
		FROM watch_events we
		JOIN watch_event_participants participant ON participant.event_id = we.id
		WHERE participant.user_id = ? AND we.media_id = ?
	`, userID, mediaID).Scan(&startedAt, &completedAt); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `
		UPDATE user_media_states SET started_at = ?, completed_at = ?, updated_at = strftime('%s', 'now') * 1000
		WHERE user_id = ? AND media_id = ?
	`, nullableNullString(startedAt), nullableNullString(completedAt), userID, mediaID)
	return err
}

const eventTimeLayout = "2006-01-02T15:04:05.000000000Z07:00"

func formatEventTime(value time.Time) string {
	return value.UTC().Format(eventTimeLayout)
}

func parseNullableEventTime(value sql.NullString) (*time.Time, error) {
	if !value.Valid {
		return nil, nil
	}
	parsed, err := time.Parse(eventTimeLayout, value.String)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func nullableEventTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return formatEventTime(*value)
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableNullString(value sql.NullString) any {
	if !value.Valid {
		return nil
	}
	return value.String
}

func (repository *SQLiteRepository) Library(ctx context.Context, userID string, status Status) ([]CatalogItem, error) {
	query := `
		SELECT media.id, media.media_type,
		       COALESCE(media.custom_title, media.external_title), media.original_title,
		       SUBSTR(COALESCE(NULLIF(media.release_date, ''), media.custom_year, ''), 1, 4),
		       media.poster_path, state.status, tmdb.source_id
		FROM user_media_states state
		JOIN media_items media ON media.id = state.media_id
		LEFT JOIN media_external_ids tmdb ON tmdb.media_id = media.id AND tmdb.source = 'tmdb'
		WHERE state.user_id = ?`
	arguments := []any{userID}
	if status != "" && status != StatusNone {
		query += " AND state.status = ?"
		arguments = append(arguments, status)
	}
	query += " ORDER BY state.updated_at DESC, media.id LIMIT 100"
	return repository.catalogItems(ctx, query, arguments...)
}

func (repository *SQLiteRepository) SearchMedia(ctx context.Context, userID, query string) ([]CatalogItem, error) {
	pattern := "%" + escapeLike(query) + "%"
	return repository.catalogItems(ctx, `
		SELECT media.id, media.media_type,
		       COALESCE(media.custom_title, media.external_title), media.original_title,
		       SUBSTR(COALESCE(NULLIF(media.release_date, ''), media.custom_year, ''), 1, 4),
		       media.poster_path, COALESCE(state.status, 'none'), tmdb.source_id
		FROM media_items media
		LEFT JOIN user_media_states state ON state.media_id = media.id AND state.user_id = ?
		LEFT JOIN media_external_ids tmdb ON tmdb.media_id = media.id AND tmdb.source = 'tmdb'
		WHERE COALESCE(media.custom_title, media.external_title) LIKE ? ESCAPE '\'
		   OR media.original_title LIKE ? ESCAPE '\'
		ORDER BY CASE WHEN COALESCE(media.custom_title, media.external_title) = ? THEN 0 ELSE 1 END,
		         media.updated_at DESC, media.id
		LIMIT 20
	`, userID, pattern, pattern, query)
}

func (repository *SQLiteRepository) catalogItems(ctx context.Context, query string, arguments ...any) ([]CatalogItem, error) {
	rows, err := repository.db.Reader().QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]CatalogItem, 0)
	for rows.Next() {
		var item CatalogItem
		var tmdbID sql.NullString
		if err := rows.Scan(
			&item.ID, &item.MediaType, &item.Title, &item.OriginalTitle,
			&item.Year, &item.PosterPath, &item.Status, &tmdbID,
		); err != nil {
			return nil, err
		}
		if tmdbID.Valid {
			value, err := strconv.Atoi(tmdbID.String)
			if err != nil || value < 1 {
				return nil, ErrInvalidRecord
			}
			item.TMDBID = &value
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "%", "\\%")
	return strings.ReplaceAll(value, "_", "\\_")
}

func (repository *SQLiteRepository) SetTags(ctx context.Context, userID, mediaID string, names []string) error {
	tx, err := repository.db.Writer().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := setTags(ctx, tx, userID, mediaID, names); err != nil {
		return err
	}
	return tx.Commit()
}

func (repository *SQLiteRepository) SetTagsVersioned(
	ctx context.Context,
	userID, mediaID string,
	names []string,
	expectedVersion int,
) (bool, error) {
	tx, err := repository.db.Writer().BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `
		UPDATE user_media_states
		SET version = version + 1, updated_at = strftime('%s', 'now') * 1000
		WHERE user_id = ? AND media_id = ? AND version = ?
	`, userID, mediaID, expectedVersion)
	if err != nil {
		return false, err
	}
	updated, err := result.RowsAffected()
	if err != nil || updated != 1 {
		return false, err
	}
	if err := setTags(ctx, tx, userID, mediaID, names); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func setTags(ctx context.Context, tx *sql.Tx, userID, mediaID string, names []string) error {
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
	return nil
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
	names := make([]string, 0)
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

func (repository *SQLiteRepository) ReplaceCollectionItems(
	ctx context.Context,
	userID, collectionID string,
	mediaIDs []string,
) error {
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
	for _, mediaID := range mediaIDs {
		var exists int
		if err := tx.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM media_items WHERE id = ?",
			mediaID,
		).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return ErrInvalidRecord
		}
	}
	if _, err := tx.ExecContext(ctx,
		"DELETE FROM collection_items WHERE collection_id = ?",
		collectionID,
	); err != nil {
		return err
	}
	for position, mediaID := range mediaIDs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO collection_items (collection_id, media_id, position) VALUES (?, ?, ?)
		`, collectionID, mediaID, position); err != nil {
			return err
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
