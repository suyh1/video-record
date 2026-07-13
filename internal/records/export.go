package records

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

const exportVersion = 1

type ExportFormat string

const (
	ExportFormatJSON ExportFormat = "json"
	ExportFormatCSV  ExportFormat = "csv"
)

var ErrInvalidExport = errors.New("invalid export request")

type ExportFile struct {
	Filename    string
	ContentType string
	Data        []byte
}

type exportDocument struct {
	Version     int                `json:"version"`
	Records     []exportRecord     `json:"records"`
	Collections []exportCollection `json:"collections"`
}

type exportRecord struct {
	Media    exportMedia      `json:"media"`
	State    *exportState     `json:"state,omitempty"`
	Tags     []string         `json:"tags"`
	Events   []exportEvent    `json:"events"`
	Progress []exportProgress `json:"progress"`
}

type exportMedia struct {
	ID               string             `json:"id"`
	MediaType        string             `json:"mediaType"`
	ExternalTitle    string             `json:"externalTitle"`
	OriginalTitle    string             `json:"originalTitle"`
	ReleaseDate      string             `json:"releaseDate"`
	ExternalOverview string             `json:"externalOverview"`
	PosterPath       string             `json:"posterPath"`
	BackdropPath     string             `json:"backdropPath"`
	CustomTitle      *string            `json:"customTitle,omitempty"`
	CustomOverview   *string            `json:"customOverview,omitempty"`
	CustomYear       *string            `json:"customYear,omitempty"`
	RuntimeMinutes   *int               `json:"runtimeMinutes,omitempty"`
	ExternalIDs      []exportExternalID `json:"externalIds"`
	Genres           []exportGenre      `json:"genres"`
	Seasons          []exportSeason     `json:"seasons"`
}

type exportExternalID struct {
	Source    string `json:"source"`
	SourceID  string `json:"sourceId"`
	MediaType string `json:"mediaType"`
}

type exportGenre struct {
	Source   string `json:"source"`
	SourceID string `json:"sourceId"`
	Name     string `json:"name"`
}

type exportSeason struct {
	ID           string          `json:"id"`
	SourceID     *string         `json:"sourceId,omitempty"`
	SeasonNumber int             `json:"seasonNumber"`
	Name         string          `json:"name"`
	Overview     string          `json:"overview"`
	PosterPath   string          `json:"posterPath"`
	AirDate      string          `json:"airDate"`
	Episodes     []exportEpisode `json:"episodes"`
}

type exportEpisode struct {
	ID            string  `json:"id"`
	SourceID      *string `json:"sourceId,omitempty"`
	EpisodeNumber int     `json:"episodeNumber"`
	Name          string  `json:"name"`
	Overview      string  `json:"overview"`
	StillPath     string  `json:"stillPath"`
	AirDate       string  `json:"airDate"`
	Runtime       *int    `json:"runtime,omitempty"`
}

type exportState struct {
	Status       Status  `json:"status"`
	Rating       *int    `json:"rating,omitempty"`
	Note         *string `json:"note,omitempty"`
	Version      int     `json:"version"`
	StatusSource Source  `json:"statusSource"`
	RatingSource Source  `json:"ratingSource"`
	NoteSource   Source  `json:"noteSource"`
	StartedAt    *string `json:"startedAt,omitempty"`
	CompletedAt  *string `json:"completedAt,omitempty"`
	ShareRating  bool    `json:"shareRating"`
	ShareReview  bool    `json:"shareReview"`
	SharedReview *string `json:"sharedReview,omitempty"`
}

type exportEvent struct {
	ID              string  `json:"id"`
	EpisodeID       *string `json:"episodeId,omitempty"`
	WatchedAt       string  `json:"watchedAt"`
	ViewingMethod   *string `json:"viewingMethod,omitempty"`
	Source          Source  `json:"source"`
	ExternalEventID *string `json:"externalEventId,omitempty"`
	Completion      int     `json:"completion"`
	Note            *string `json:"note,omitempty"`
}

type exportProgress struct {
	EpisodeID    string `json:"episodeId"`
	WatchedAt    string `json:"watchedAt"`
	Source       Source `json:"source"`
	WatchEventID string `json:"watchEventId"`
}

type exportCollection struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	MediaIDs []string `json:"mediaIds"`
}

func (service *Service) ExportData(ctx context.Context, userID string, format ExportFormat) (ExportFile, error) {
	if userID == "" {
		return ExportFile{}, ErrInvalidExport
	}
	document, err := service.repository.ExportDocument(ctx, userID)
	if err != nil {
		return ExportFile{}, err
	}
	switch format {
	case ExportFormatJSON:
		data, err := json.Marshal(document)
		if err != nil {
			return ExportFile{}, err
		}
		return ExportFile{
			Filename: "video-record-export.json", ContentType: "application/json", Data: data,
		}, nil
	case ExportFormatCSV:
		data, err := encodeExportCSV(document)
		if err != nil {
			return ExportFile{}, err
		}
		return ExportFile{
			Filename: "video-record-export.csv", ContentType: "text/csv; charset=utf-8", Data: data,
		}, nil
	default:
		return ExportFile{}, ErrInvalidExport
	}
}

func encodeExportCSV(document exportDocument) ([]byte, error) {
	var output strings.Builder
	writer := csv.NewWriter(&output)
	if err := writer.Write([]string{
		"media_id", "media_type", "title", "original_title", "release_date", "overview",
		"external_source", "external_id", "status", "rating", "note", "started_at",
		"completed_at", "tags",
	}); err != nil {
		return nil, err
	}
	for _, record := range document.Records {
		title := record.Media.ExternalTitle
		if record.Media.CustomTitle != nil {
			title = *record.Media.CustomTitle
		}
		overview := record.Media.ExternalOverview
		if record.Media.CustomOverview != nil {
			overview = *record.Media.CustomOverview
		}
		var externalSource, externalID string
		if len(record.Media.ExternalIDs) > 0 {
			externalSource = record.Media.ExternalIDs[0].Source
			externalID = record.Media.ExternalIDs[0].SourceID
		}
		var status, rating, note, startedAt, completedAt string
		if record.State != nil {
			status = string(record.State.Status)
			if record.State.Rating != nil {
				rating = strconv.Itoa(*record.State.Rating)
			}
			if record.State.Note != nil {
				note = *record.State.Note
			}
			if record.State.StartedAt != nil {
				startedAt = *record.State.StartedAt
			}
			if record.State.CompletedAt != nil {
				completedAt = *record.State.CompletedAt
			}
		}
		row := []string{
			record.Media.ID, record.Media.MediaType, title, record.Media.OriginalTitle,
			record.Media.ReleaseDate, overview, externalSource, externalID,
			status, rating, note, startedAt, completedAt, strings.Join(record.Tags, "|"),
		}
		for index := range row {
			row[index] = neutralizeSpreadsheetFormula(row[index])
		}
		if err := writer.Write(row); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return []byte(output.String()), nil
}

func neutralizeSpreadsheetFormula(value string) string {
	if value == "" {
		return value
	}
	switch value[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + value
	default:
		return value
	}
}

func (repository *SQLiteRepository) ExportDocument(ctx context.Context, userID string) (exportDocument, error) {
	document := exportDocument{
		Version: exportVersion, Records: make([]exportRecord, 0), Collections: make([]exportCollection, 0),
	}
	rows, err := repository.db.Reader().QueryContext(ctx, `
		SELECT DISTINCT media.id, media.media_type, media.external_title, media.original_title,
		       media.release_date, media.external_overview, media.poster_path, media.backdrop_path,
		       media.custom_title, media.custom_overview, media.custom_year, media.runtime_minutes
		FROM media_items media
		WHERE EXISTS (
			SELECT 1 FROM user_media_states state WHERE state.user_id = ? AND state.media_id = media.id
		) OR EXISTS (
			SELECT 1 FROM watch_events event
			JOIN watch_event_participants participant ON participant.event_id = event.id
			WHERE participant.user_id = ? AND event.media_id = media.id
		) OR EXISTS (
			SELECT 1 FROM collections collection
			JOIN collection_items item ON item.collection_id = collection.id
			WHERE collection.user_id = ? AND item.media_id = media.id
		)
		ORDER BY media.id
	`, userID, userID, userID)
	if err != nil {
		return exportDocument{}, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var record exportRecord
		var customTitle, customOverview, customYear sql.NullString
		var runtime sql.NullInt64
		if err := rows.Scan(
			&record.Media.ID, &record.Media.MediaType, &record.Media.ExternalTitle,
			&record.Media.OriginalTitle, &record.Media.ReleaseDate, &record.Media.ExternalOverview,
			&record.Media.PosterPath, &record.Media.BackdropPath, &customTitle, &customOverview,
			&customYear, &runtime,
		); err != nil {
			return exportDocument{}, err
		}
		record.Media.CustomTitle = nullableStringPointer(customTitle)
		record.Media.CustomOverview = nullableStringPointer(customOverview)
		record.Media.CustomYear = nullableStringPointer(customYear)
		record.Media.RuntimeMinutes = nullableIntPointer(runtime)
		record.Media.ExternalIDs, err = repository.exportExternalIDs(ctx, record.Media.ID)
		if err != nil {
			return exportDocument{}, err
		}
		record.Media.Genres, err = repository.exportGenres(ctx, record.Media.ID)
		if err != nil {
			return exportDocument{}, err
		}
		record.Media.Seasons, err = repository.exportSeasons(ctx, record.Media.ID)
		if err != nil {
			return exportDocument{}, err
		}
		record.State, err = repository.exportState(ctx, userID, record.Media.ID)
		if err != nil {
			return exportDocument{}, err
		}
		record.Tags, err = repository.Tags(ctx, userID, record.Media.ID)
		if err != nil {
			return exportDocument{}, err
		}
		record.Events, err = repository.exportEvents(ctx, userID, record.Media.ID)
		if err != nil {
			return exportDocument{}, err
		}
		record.Progress, err = repository.exportProgress(ctx, userID, record.Media.ID)
		if err != nil {
			return exportDocument{}, err
		}
		document.Records = append(document.Records, record)
	}
	if err := rows.Err(); err != nil {
		return exportDocument{}, err
	}
	document.Collections, err = repository.exportCollections(ctx, userID)
	if err != nil {
		return exportDocument{}, err
	}
	return document, nil
}

func (repository *SQLiteRepository) exportExternalIDs(ctx context.Context, mediaID string) ([]exportExternalID, error) {
	rows, err := repository.db.Reader().QueryContext(ctx, `
		SELECT source, source_id, media_type FROM media_external_ids WHERE media_id = ? ORDER BY source
	`, mediaID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]exportExternalID, 0)
	for rows.Next() {
		var item exportExternalID
		if err := rows.Scan(&item.Source, &item.SourceID, &item.MediaType); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (repository *SQLiteRepository) exportGenres(ctx context.Context, mediaID string) ([]exportGenre, error) {
	rows, err := repository.db.Reader().QueryContext(ctx, `
		SELECT genre.source, genre.source_id, genre.name
		FROM genres genre
		JOIN media_genres media_genre
		  ON media_genre.source = genre.source AND media_genre.source_id = genre.source_id
		WHERE media_genre.media_id = ? ORDER BY genre.source, genre.source_id
	`, mediaID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	genres := make([]exportGenre, 0)
	for rows.Next() {
		var genre exportGenre
		if err := rows.Scan(&genre.Source, &genre.SourceID, &genre.Name); err != nil {
			return nil, err
		}
		genres = append(genres, genre)
	}
	return genres, rows.Err()
}

func (repository *SQLiteRepository) exportSeasons(ctx context.Context, mediaID string) ([]exportSeason, error) {
	rows, err := repository.db.Reader().QueryContext(ctx, `
		SELECT id, source_id, season_number, name, overview, poster_path, air_date
		FROM seasons WHERE media_id = ? ORDER BY season_number, id
	`, mediaID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	seasons := make([]exportSeason, 0)
	for rows.Next() {
		var season exportSeason
		var sourceID sql.NullString
		if err := rows.Scan(
			&season.ID, &sourceID, &season.SeasonNumber, &season.Name,
			&season.Overview, &season.PosterPath, &season.AirDate,
		); err != nil {
			return nil, err
		}
		season.SourceID = nullableStringPointer(sourceID)
		season.Episodes, err = repository.exportEpisodes(ctx, season.ID)
		if err != nil {
			return nil, err
		}
		seasons = append(seasons, season)
	}
	return seasons, rows.Err()
}

func (repository *SQLiteRepository) exportEpisodes(ctx context.Context, seasonID string) ([]exportEpisode, error) {
	rows, err := repository.db.Reader().QueryContext(ctx, `
		SELECT id, source_id, episode_number, name, overview, still_path, air_date, runtime
		FROM episodes WHERE season_id = ? ORDER BY episode_number, id
	`, seasonID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	episodes := make([]exportEpisode, 0)
	for rows.Next() {
		var episode exportEpisode
		var sourceID sql.NullString
		var runtime sql.NullInt64
		if err := rows.Scan(
			&episode.ID, &sourceID, &episode.EpisodeNumber, &episode.Name,
			&episode.Overview, &episode.StillPath, &episode.AirDate, &runtime,
		); err != nil {
			return nil, err
		}
		episode.SourceID = nullableStringPointer(sourceID)
		episode.Runtime = nullableIntPointer(runtime)
		episodes = append(episodes, episode)
	}
	return episodes, rows.Err()
}

func (repository *SQLiteRepository) exportState(ctx context.Context, userID, mediaID string) (*exportState, error) {
	var state exportState
	var rating sql.NullInt64
	var note, startedAt, completedAt, sharedReview sql.NullString
	var shareRating, shareReview int
	err := repository.db.Reader().QueryRowContext(ctx, `
		SELECT status, rating, note, version, status_source, rating_source, note_source,
		       started_at, completed_at, share_rating, share_review, shared_review
		FROM user_media_states WHERE user_id = ? AND media_id = ?
	`, userID, mediaID).Scan(
		&state.Status, &rating, &note, &state.Version, &state.StatusSource,
		&state.RatingSource, &state.NoteSource, &startedAt, &completedAt,
		&shareRating, &shareReview, &sharedReview,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	state.Rating = nullableIntPointer(rating)
	state.Note = nullableStringPointer(note)
	state.StartedAt = nullableStringPointer(startedAt)
	state.CompletedAt = nullableStringPointer(completedAt)
	state.ShareRating = shareRating == 1
	state.ShareReview = shareReview == 1
	state.SharedReview = nullableStringPointer(sharedReview)
	return &state, nil
}

func (repository *SQLiteRepository) exportEvents(ctx context.Context, userID, mediaID string) ([]exportEvent, error) {
	rows, err := repository.db.Reader().QueryContext(ctx, `
		SELECT event.id, event.episode_id, event.watched_at, event.viewing_method,
		       CASE WHEN event.created_by_user_id = ? THEN event.source ELSE 'confirmed_import' END,
		       CASE WHEN event.created_by_user_id = ? THEN event.external_event_id ELSE NULL END,
		       event.completion,
		       CASE WHEN event.created_by_user_id = ? THEN event.note ELSE NULL END
		FROM watch_events event
		JOIN watch_event_participants participant ON participant.event_id = event.id
		WHERE participant.user_id = ? AND event.media_id = ?
		ORDER BY event.watched_at, event.id
	`, userID, userID, userID, userID, mediaID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	events := make([]exportEvent, 0)
	for rows.Next() {
		var event exportEvent
		var episodeID, viewingMethod, externalEventID, note sql.NullString
		if err := rows.Scan(
			&event.ID, &episodeID, &event.WatchedAt, &viewingMethod, &event.Source,
			&externalEventID, &event.Completion, &note,
		); err != nil {
			return nil, err
		}
		event.EpisodeID = nullableStringPointer(episodeID)
		event.ViewingMethod = nullableStringPointer(viewingMethod)
		event.ExternalEventID = nullableStringPointer(externalEventID)
		event.Note = nullableStringPointer(note)
		events = append(events, event)
	}
	return events, rows.Err()
}

func (repository *SQLiteRepository) exportProgress(ctx context.Context, userID, mediaID string) ([]exportProgress, error) {
	rows, err := repository.db.Reader().QueryContext(ctx, `
		SELECT episode_id, watched_at, source, watch_event_id
		FROM episode_progress WHERE user_id = ? AND media_id = ? ORDER BY episode_id
	`, userID, mediaID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	progress := make([]exportProgress, 0)
	for rows.Next() {
		var item exportProgress
		if err := rows.Scan(&item.EpisodeID, &item.WatchedAt, &item.Source, &item.WatchEventID); err != nil {
			return nil, err
		}
		progress = append(progress, item)
	}
	return progress, rows.Err()
}

func (repository *SQLiteRepository) exportCollections(ctx context.Context, userID string) ([]exportCollection, error) {
	rows, err := repository.db.Reader().QueryContext(ctx, `
		SELECT id, name FROM collections WHERE user_id = ? ORDER BY name COLLATE NOCASE, id
	`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	collections := make([]exportCollection, 0)
	for rows.Next() {
		var collection exportCollection
		if err := rows.Scan(&collection.ID, &collection.Name); err != nil {
			return nil, err
		}
		collection.MediaIDs = make([]string, 0)
		itemRows, err := repository.db.Reader().QueryContext(ctx, `
			SELECT media_id FROM collection_items WHERE collection_id = ? ORDER BY position
		`, collection.ID)
		if err != nil {
			return nil, err
		}
		for itemRows.Next() {
			var mediaID string
			if err := itemRows.Scan(&mediaID); err != nil {
				_ = itemRows.Close()
				return nil, err
			}
			collection.MediaIDs = append(collection.MediaIDs, mediaID)
		}
		if err := itemRows.Err(); err != nil {
			_ = itemRows.Close()
			return nil, err
		}
		_ = itemRows.Close()
		collections = append(collections, collection)
	}
	return collections, rows.Err()
}

func nullableStringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

func nullableIntPointer(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	result := int(value.Int64)
	return &result
}
