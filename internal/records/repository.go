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
	ValidateRoundScope(context.Context, RoundScope) error
	FindCurrentRound(context.Context, RoundScope) (WatchRound, bool, error)
	FindLatestRound(context.Context, RoundScope) (WatchRound, bool, error)
	ArchiveCurrentRound(context.Context, RoundScope, int, time.Time) (RewatchResult, error)
	ArchivedRounds(context.Context, RoundScope) ([]RoundSummary, error)
	FindArchivedRoundDetail(context.Context, RoundScope, string) (RoundDetail, bool, error)
	FindProfile(context.Context, string, string) (MediaProfile, bool, error)
	InsertRound(context.Context, WatchRound, []string) error
	UpdateRound(context.Context, WatchRound, int, []string) (bool, error)
	FindState(context.Context, string, string) (State, bool, error)
	WatchEvents(context.Context, string, string) ([]WatchEvent, error)
	Library(context.Context, string, Status) ([]CatalogItem, error)
	SearchMedia(context.Context, string, string) ([]CatalogItem, error)
	CalendarEvents(context.Context, string, time.Time, time.Time, CalendarFilter) ([]CalendarEvent, error)
	SeasonEpisodes(context.Context, string, string, int) (SeriesProgress, error)
	ApplySeasonEpisodeProgress(context.Context, EpisodeProgressInput, []string) (bool, error)
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
	var statusSource, ratingSource, noteSource sql.NullString
	var startedAt, completedAt sql.NullString
	err := repository.db.Reader().QueryRowContext(ctx, `
		SELECT profile.status, round.rating, round.note, profile.version,
		       round.status_source, round.rating_source, round.note_source,
		       round.started_at, round.completed_at
		FROM user_media_profiles AS profile
		LEFT JOIN watch_rounds AS round ON round.id = (
			SELECT current.id
			FROM watch_rounds AS current
			WHERE current.user_id = profile.user_id
			  AND current.media_id = profile.media_id
			  AND current.archived_at IS NULL
			ORDER BY CASE WHEN current.status = 'watching' THEN 0 ELSE 1 END,
			         current.updated_at DESC, current.season_number DESC, current.id DESC
			LIMIT 1
		)
		WHERE profile.user_id = ? AND profile.media_id = ?
	`, userID, mediaID).Scan(
		&state.Status, &rating, &note, &state.Version,
		&statusSource, &ratingSource, &noteSource,
		&startedAt, &completedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return State{UserID: userID, MediaID: mediaID, Status: StatusNone}, false, nil
	}
	if err != nil {
		return State{}, false, err
	}
	state.UserID, state.MediaID = userID, mediaID
	state.StatusSource = SourceExternalDefault
	state.RatingSource = SourceExternalDefault
	state.NoteSource = SourceExternalDefault
	if statusSource.Valid {
		state.StatusSource = Source(statusSource.String)
	}
	if ratingSource.Valid {
		state.RatingSource = Source(ratingSource.String)
	}
	if noteSource.Valid {
		state.NoteSource = Source(noteSource.String)
	}
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

type sqlExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func insertWatchEvent(ctx context.Context, tx *sql.Tx, event WatchEvent) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO watch_events (
			id, round_id, created_by_user_id, media_id, episode_id, watched_at,
			viewing_method, source, external_event_id, completion, note, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, strftime('%s', 'now') * 1000)
	`, event.ID, event.RoundID, event.UserID, event.MediaID, nullableString(event.EpisodeID), formatEventTime(event.WatchedAt),
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
		SELECT we.id, we.round_id, we.created_by_user_id, we.media_id, we.episode_id, we.watched_at,
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
			&event.ID, &event.RoundID, &event.UserID, &event.MediaID, &episodeID, &watchedAt,
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

func (repository *SQLiteRepository) Library(ctx context.Context, userID string, status Status) ([]CatalogItem, error) {
	query := `
		SELECT media.id, media.media_type,
		       COALESCE(media.custom_title, media.external_title), media.original_title,
		       SUBSTR(COALESCE(NULLIF(media.release_date, ''), media.custom_year, ''), 1, 4),
		       media.poster_path, profile.status, tmdb.source_id
		FROM user_media_profiles profile
		JOIN media_items media ON media.id = profile.media_id
		LEFT JOIN media_external_ids tmdb ON tmdb.media_id = media.id AND tmdb.source = 'tmdb'
		WHERE profile.user_id = ?`
	arguments := []any{userID}
	if status != "" && status != StatusNone {
		query += " AND profile.status = ?"
		arguments = append(arguments, status)
	}
	query += " ORDER BY profile.updated_at DESC, media.id LIMIT 100"
	return repository.catalogItems(ctx, query, arguments...)
}

func (repository *SQLiteRepository) SearchMedia(ctx context.Context, userID, query string) ([]CatalogItem, error) {
	pattern := "%" + escapeLike(query) + "%"
	return repository.catalogItems(ctx, `
		SELECT media.id, media.media_type,
		       COALESCE(media.custom_title, media.external_title), media.original_title,
		       SUBSTR(COALESCE(NULLIF(media.release_date, ''), media.custom_year, ''), 1, 4),
		       media.poster_path, COALESCE(profile.status, 'none'), tmdb.source_id
		FROM media_items media
		LEFT JOIN user_media_profiles profile ON profile.media_id = media.id AND profile.user_id = ?
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
	if err := ensureMediaProfile(ctx, tx, userID, mediaID); err != nil {
		return err
	}
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
	var currentVersion int
	err = tx.QueryRowContext(ctx, `
		SELECT version FROM user_media_profiles WHERE user_id = ? AND media_id = ?
	`, userID, mediaID).Scan(&currentVersion)
	switch {
	case errors.Is(err, sql.ErrNoRows) && expectedVersion == 0:
		if err := ensureMediaProfile(ctx, tx, userID, mediaID); err != nil {
			return false, err
		}
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, err
	case currentVersion != expectedVersion:
		return false, nil
	default:
		result, err := tx.ExecContext(ctx, `
			UPDATE user_media_profiles
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
