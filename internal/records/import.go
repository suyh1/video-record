package records

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const maxImportBytes = 10 << 20

var (
	ErrImportTooLarge       = errors.New("import file too large")
	ErrInvalidImport        = errors.New("invalid import file")
	ErrUnsafeImportFilename = errors.New("unsafe import filename")
	errExternalIDConflict   = errors.New("external identity conflict")
)

type ImportFailure struct {
	RecordID string `json:"recordId"`
	Code     string `json:"code"`
}

type ImportReport struct {
	ImportedRecords     int             `json:"importedRecords"`
	ImportedCollections int             `json:"importedCollections"`
	Failures            []ImportFailure `json:"failures"`
}

func (service *Service) ImportData(
	ctx context.Context,
	userID, filename string,
	data []byte,
) (ImportReport, error) {
	if userID == "" || !safeImportFilename(filename) {
		return ImportReport{}, ErrUnsafeImportFilename
	}
	if len(data) > maxImportBytes {
		return ImportReport{}, ErrImportTooLarge
	}
	if !utf8.Valid(data) {
		return ImportReport{}, ErrInvalidImport
	}
	var document exportDocument
	var err error
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".json":
		document, err = decodeImportJSON(data)
	case ".csv":
		document, err = decodeImportCSV(data)
	default:
		return ImportReport{}, ErrInvalidImport
	}
	if err != nil {
		return ImportReport{}, err
	}
	return service.repository.ImportDocument(ctx, userID, document)
}

func safeImportFilename(filename string) bool {
	return filename != "" && filepath.Base(filename) == filename &&
		!strings.ContainsAny(filename, `/\`)
}

func decodeImportJSON(data []byte) (exportDocument, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var document exportDocument
	if err := decoder.Decode(&document); err != nil {
		return exportDocument{}, ErrInvalidImport
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return exportDocument{}, ErrInvalidImport
	}
	if document.Version != exportVersion || document.Records == nil || document.Collections == nil {
		return exportDocument{}, ErrInvalidImport
	}
	return document, nil
}

func decodeImportCSV(data []byte) (exportDocument, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil || len(rows) == 0 {
		return exportDocument{}, ErrInvalidImport
	}
	wantedHeader := []string{
		"media_id", "media_type", "title", "original_title", "release_date", "overview",
		"external_source", "external_id", "round_id", "round_number", "season_number",
		"status", "rating", "note", "viewing_method", "started_at", "completed_at",
		"archived_at", "tags",
	}
	if len(rows[0]) != len(wantedHeader) {
		return exportDocument{}, ErrInvalidImport
	}
	for index := range wantedHeader {
		if rows[0][index] != wantedHeader[index] {
			return exportDocument{}, ErrInvalidImport
		}
	}
	document := exportDocument{
		Version: exportVersion, Records: make([]exportRecord, 0, len(rows)-1), Collections: make([]exportCollection, 0),
	}
	for _, rawRow := range rows[1:] {
		if len(rawRow) != len(wantedHeader) {
			return exportDocument{}, ErrInvalidImport
		}
		row := make([]string, len(rawRow))
		for index, value := range rawRow {
			row[index] = restoreSpreadsheetValue(value)
		}
		mediaID := strings.TrimSpace(row[0])
		if mediaID == "" {
			mediaID = uuid.NewString()
		}
		record := exportRecord{
			Media: exportMedia{
				ID: mediaID, MediaType: strings.TrimSpace(row[1]),
				ExternalTitle: strings.TrimSpace(row[2]), OriginalTitle: strings.TrimSpace(row[3]),
				ReleaseDate: strings.TrimSpace(row[4]), ExternalOverview: row[5],
				ExternalIDs: make([]exportExternalID, 0), Genres: make([]exportGenre, 0),
				Seasons: make([]exportSeason, 0),
			},
			Tags: make([]string, 0), Rounds: make([]exportRound, 0),
		}
		if record.Media.ExternalTitle == "" {
			record.Media.ExternalTitle = "Imported item"
		}
		if row[6] != "" || row[7] != "" {
			if row[6] == "" || row[7] == "" {
				return exportDocument{}, ErrInvalidImport
			}
			record.Media.ExternalIDs = append(record.Media.ExternalIDs, exportExternalID{
				Source: row[6], SourceID: row[7], MediaType: record.Media.MediaType,
			})
		}
		if row[11] != "" {
			round := exportRound{
				ID: row[8], Status: Status(row[11]), Version: 1,
				StatusSource: SourceConfirmedImport, RatingSource: SourceConfirmedImport,
				NoteSource: SourceConfirmedImport,
				Events:     make([]exportEvent, 0), Episodes: make([]exportRoundEpisode, 0),
			}
			if round.ID == "" {
				round.ID = uuid.NewString()
			}
			if row[9] == "" {
				round.RoundNumber = 1
			} else {
				roundNumber, err := strconv.Atoi(row[9])
				if err != nil {
					return exportDocument{}, ErrInvalidImport
				}
				round.RoundNumber = roundNumber
			}
			if row[10] != "" {
				seasonNumber, err := strconv.Atoi(row[10])
				if err != nil {
					return exportDocument{}, ErrInvalidImport
				}
				round.SeasonNumber = &seasonNumber
			}
			if row[12] != "" {
				rating, err := strconv.Atoi(row[12])
				if err != nil {
					return exportDocument{}, ErrInvalidImport
				}
				round.Rating = &rating
			}
			if row[13] != "" {
				round.Note = stringPointer(row[13])
			}
			if row[14] != "" {
				round.ViewingMethod = stringPointer(row[14])
			}
			if row[15] != "" {
				round.StartedAt = stringPointer(row[15])
			}
			if row[16] != "" {
				round.CompletedAt = stringPointer(row[16])
			}
			if row[17] != "" {
				round.ArchivedAt = stringPointer(row[17])
			}
			record.Rounds = append(record.Rounds, round)
		}
		if row[18] != "" {
			for _, tag := range strings.Split(row[18], "|") {
				if tag = strings.TrimSpace(tag); tag != "" {
					record.Tags = append(record.Tags, tag)
				}
			}
		}
		document.Records = append(document.Records, record)
	}
	return document, nil
}

func restoreSpreadsheetValue(value string) string {
	if len(value) > 1 && value[0] == '\'' {
		switch value[1] {
		case '=', '+', '-', '@', '\t', '\r':
			return value[1:]
		}
	}
	return value
}

func (repository *SQLiteRepository) ImportDocument(
	ctx context.Context,
	userID string,
	document exportDocument,
) (ImportReport, error) {
	if document.Version != exportVersion {
		return ImportReport{}, ErrInvalidImport
	}
	report := ImportReport{Failures: make([]ImportFailure, 0)}
	seenExternalIDs := make(map[string]string)
	importedMedia := make(map[string]struct{})
	for _, record := range document.Records {
		if conflict := duplicateExternalID(record, seenExternalIDs); conflict {
			report.Failures = append(report.Failures, ImportFailure{
				RecordID: record.Media.ID, Code: "external_identity_conflict",
			})
			continue
		}
		err := repository.importRecord(ctx, userID, record)
		if err != nil {
			code := "record_import_failed"
			if errors.Is(err, errExternalIDConflict) {
				code = "external_identity_conflict"
			}
			report.Failures = append(report.Failures, ImportFailure{RecordID: record.Media.ID, Code: code})
			continue
		}
		report.ImportedRecords++
		importedMedia[record.Media.ID] = struct{}{}
	}
	for _, collection := range document.Collections {
		if err := repository.importCollection(ctx, userID, collection, importedMedia); err != nil {
			report.Failures = append(report.Failures, ImportFailure{
				RecordID: collection.ID, Code: "collection_import_failed",
			})
			continue
		}
		report.ImportedCollections++
	}
	return report, nil
}

func duplicateExternalID(record exportRecord, seen map[string]string) bool {
	for _, identity := range record.Media.ExternalIDs {
		key := identity.Source + "\x00" + identity.SourceID + "\x00" + identity.MediaType
		if owner, exists := seen[key]; exists && owner != record.Media.ID {
			return true
		}
		seen[key] = record.Media.ID
	}
	return false
}

func (repository *SQLiteRepository) importRecord(ctx context.Context, userID string, record exportRecord) error {
	if err := validateImportRecord(record); err != nil {
		return err
	}
	tx, err := repository.db.Writer().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var mediaExists int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM media_items WHERE id = ?", record.Media.ID).Scan(&mediaExists); err != nil {
		return err
	}
	if mediaExists == 0 {
		now := time.Now().UTC().UnixMilli()
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO media_items (
				id, media_type, external_title, original_title, release_date,
				external_overview, poster_path, backdrop_path, custom_title,
				custom_overview, custom_year, runtime_minutes, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, record.Media.ID, record.Media.MediaType, record.Media.ExternalTitle,
			record.Media.OriginalTitle, record.Media.ReleaseDate, record.Media.ExternalOverview,
			record.Media.PosterPath, record.Media.BackdropPath, nullableText(record.Media.CustomTitle),
			nullableText(record.Media.CustomOverview), nullableText(record.Media.CustomYear),
			nullableInt(record.Media.RuntimeMinutes), now, now); err != nil {
			return err
		}
	}
	if err := importExternalIDs(ctx, tx, record.Media, mediaExists == 0); err != nil {
		return err
	}
	if mediaExists == 0 {
		if err := importGenres(ctx, tx, record.Media); err != nil {
			return err
		}
		if err := importSeasons(ctx, tx, record.Media); err != nil {
			return err
		}
	}
	if record.Profile != nil || len(record.Rounds) > 0 || len(record.Tags) > 0 {
		if err := importProfile(ctx, tx, userID, record.Media.ID, record.Profile); err != nil {
			return err
		}
	}
	if err := importTags(ctx, tx, userID, record.Media.ID, record.Tags); err != nil {
		return err
	}
	if err := importRounds(ctx, tx, userID, record.Media.ID, record.Rounds); err != nil {
		return err
	}
	if len(record.Rounds) > 0 {
		if err := projectMediaProfile(ctx, tx, userID, record.Media.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func validateImportRecord(record exportRecord) error {
	if strings.TrimSpace(record.Media.ID) == "" ||
		(record.Media.MediaType != "movie" && record.Media.MediaType != "tv") ||
		(strings.TrimSpace(record.Media.ExternalTitle) == "" &&
			(record.Media.CustomTitle == nil || strings.TrimSpace(*record.Media.CustomTitle) == "")) {
		return ErrInvalidImport
	}
	for _, identity := range record.Media.ExternalIDs {
		if identity.Source == "" || identity.SourceID == "" || identity.MediaType != record.Media.MediaType {
			return ErrInvalidImport
		}
	}
	if record.Tags == nil || record.Rounds == nil {
		return ErrInvalidImport
	}
	if record.Profile != nil && record.Profile.Version < 1 {
		return ErrInvalidImport
	}
	currentScopes := make(map[int]struct{})
	roundNumbers := make(map[[2]int]struct{})
	for _, round := range record.Rounds {
		if round.ID == "" || round.RoundNumber < 1 || round.Version < 1 ||
			ValidateStatus(round.Status) != nil || sourcePriority(round.StatusSource) == 0 ||
			sourcePriority(round.RatingSource) == 0 || sourcePriority(round.NoteSource) == 0 ||
			(round.Rating != nil && (*round.Rating < 0 || *round.Rating > 100)) ||
			round.Events == nil || round.Episodes == nil {
			return ErrInvalidImport
		}
		scope := 0
		if round.SeasonNumber != nil {
			scope = *round.SeasonNumber
		}
		if record.Media.MediaType == "movie" && round.SeasonNumber != nil ||
			record.Media.MediaType == "tv" && (round.SeasonNumber == nil || scope < 1) {
			return ErrInvalidImport
		}
		key := [2]int{scope, round.RoundNumber}
		if _, exists := roundNumbers[key]; exists {
			return ErrInvalidImport
		}
		roundNumbers[key] = struct{}{}
		if round.ArchivedAt == nil {
			if _, exists := currentScopes[scope]; exists {
				return ErrInvalidImport
			}
			currentScopes[scope] = struct{}{}
		}
		for _, value := range []*string{round.StartedAt, round.CompletedAt, round.ArchivedAt} {
			if value != nil {
				if _, err := time.Parse(eventTimeLayout, *value); err != nil {
					return ErrInvalidImport
				}
			}
		}
		for _, event := range round.Events {
			if event.ID == "" || event.WatchedAt == "" || sourcePriority(event.Source) == 0 ||
				event.Completion < 0 || event.Completion > 100 {
				return ErrInvalidImport
			}
			if _, err := time.Parse(eventTimeLayout, event.WatchedAt); err != nil {
				return ErrInvalidImport
			}
		}
		for _, episode := range round.Episodes {
			if episode.EpisodeID == "" || episode.WatchEventID == "" ||
				sourcePriority(episode.Source) == 0 {
				return ErrInvalidImport
			}
			if _, err := time.Parse(eventTimeLayout, episode.WatchedAt); err != nil {
				return ErrInvalidImport
			}
		}
	}
	if record.State != nil || len(record.Events) > 0 || len(record.Progress) > 0 {
		return ErrInvalidImport
	}
	return nil
}

func importProfile(
	ctx context.Context,
	tx *sql.Tx,
	userID, mediaID string,
	profile *exportProfile,
) error {
	if err := ensureMediaProfile(ctx, tx, userID, mediaID); err != nil {
		return err
	}
	if profile == nil {
		return nil
	}
	_, err := tx.ExecContext(ctx, `
		UPDATE user_media_profiles SET
			version = ?, share_rating = ?, share_review = ?, shared_review = ?,
			updated_at = strftime('%s', 'now') * 1000
		WHERE user_id = ? AND media_id = ?
	`, profile.Version, boolInt(profile.ShareRating), boolInt(profile.ShareReview),
		nullableText(profile.SharedReview), userID, mediaID)
	return err
}

func importRounds(
	ctx context.Context,
	tx *sql.Tx,
	userID, mediaID string,
	rounds []exportRound,
) error {
	for _, round := range rounds {
		var owner, existingMedia string
		err := tx.QueryRowContext(ctx, `
			SELECT user_id, media_id FROM watch_rounds WHERE id = ?
		`, round.ID).Scan(&owner, &existingMedia)
		if err == nil && (owner != userID || existingMedia != mediaID) {
			return ErrInvalidImport
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM watch_rounds WHERE user_id = ? AND media_id = ?
	`, userID, mediaID); err != nil {
		return err
	}
	for _, round := range rounds {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO watch_rounds (
				id, user_id, media_id, season_number, round_number, status,
				rating, note, viewing_method, started_at, completed_at, archived_at,
				version, status_source, rating_source, note_source, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
			          strftime('%s', 'now') * 1000, strftime('%s', 'now') * 1000)
		`, round.ID, userID, mediaID, nullableInt(round.SeasonNumber), round.RoundNumber,
			round.Status, nullableInt(round.Rating), nullableText(round.Note), nullableText(round.ViewingMethod),
			nullableText(round.StartedAt), nullableText(round.CompletedAt), nullableText(round.ArchivedAt),
			round.Version, round.StatusSource, round.RatingSource, round.NoteSource); err != nil {
			return err
		}
		if err := importRoundEvents(ctx, tx, userID, mediaID, round.ID, round.Events); err != nil {
			return err
		}
		if err := importRoundEpisodes(ctx, tx, round.ID, round.Episodes); err != nil {
			return err
		}
	}
	return nil
}

func importRoundEvents(
	ctx context.Context,
	tx *sql.Tx,
	userID, mediaID, roundID string,
	events []exportEvent,
) error {
	for _, event := range events {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO watch_events (
				id, round_id, created_by_user_id, media_id, episode_id, watched_at,
				viewing_method, source, external_event_id, completion, note, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, strftime('%s', 'now') * 1000)
		`, event.ID, roundID, userID, mediaID, nullableText(event.EpisodeID), event.WatchedAt,
			nullableText(event.ViewingMethod), event.Source, nullableText(event.ExternalEventID),
			event.Completion, nullableText(event.Note)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO watch_event_participants (event_id, user_id) VALUES (?, ?)
		`, event.ID, userID); err != nil {
			return err
		}
	}
	return nil
}

func importRoundEpisodes(
	ctx context.Context,
	tx *sql.Tx,
	roundID string,
	episodes []exportRoundEpisode,
) error {
	for _, episode := range episodes {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO round_episode_progress (
				round_id, episode_id, watched_at, source, watch_event_id, updated_at
			) VALUES (?, ?, ?, ?, ?, strftime('%s', 'now') * 1000)
		`, roundID, episode.EpisodeID, episode.WatchedAt, episode.Source, episode.WatchEventID); err != nil {
			return err
		}
	}
	return nil
}

func importExternalIDs(ctx context.Context, tx *sql.Tx, media exportMedia, replace bool) error {
	for _, identity := range media.ExternalIDs {
		var owner string
		err := tx.QueryRowContext(ctx, `
			SELECT media_id FROM media_external_ids
			WHERE source = ? AND source_id = ? AND media_type = ?
		`, identity.Source, identity.SourceID, identity.MediaType).Scan(&owner)
		if err == nil && owner != media.ID {
			return errExternalIDConflict
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	if !replace {
		return nil
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM media_external_ids WHERE media_id = ?", media.ID); err != nil {
		return err
	}
	for _, identity := range media.ExternalIDs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO media_external_ids (media_id, source, source_id, media_type)
			VALUES (?, ?, ?, ?)
		`, media.ID, identity.Source, identity.SourceID, identity.MediaType); err != nil {
			return err
		}
	}
	return nil
}

func importGenres(ctx context.Context, tx *sql.Tx, media exportMedia) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM media_genres WHERE media_id = ?", media.ID); err != nil {
		return err
	}
	for _, genre := range media.Genres {
		if genre.Source == "" || genre.SourceID == "" || genre.Name == "" {
			return ErrInvalidImport
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO genres (source, source_id, name) VALUES (?, ?, ?)
			ON CONFLICT(source, source_id) DO UPDATE SET name = excluded.name
		`, genre.Source, genre.SourceID, genre.Name); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO media_genres (media_id, source, source_id) VALUES (?, ?, ?)
		`, media.ID, genre.Source, genre.SourceID); err != nil {
			return err
		}
	}
	return nil
}

func importSeasons(ctx context.Context, tx *sql.Tx, media exportMedia) error {
	for _, season := range media.Seasons {
		if season.ID == "" || season.SeasonNumber < 0 {
			return ErrInvalidImport
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO seasons (id, media_id, source_id, season_number, name, overview, poster_path, air_date)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				media_id = excluded.media_id, source_id = excluded.source_id,
				season_number = excluded.season_number, name = excluded.name,
				overview = excluded.overview, poster_path = excluded.poster_path,
				air_date = excluded.air_date
		`, season.ID, media.ID, nullableText(season.SourceID), season.SeasonNumber,
			season.Name, season.Overview, season.PosterPath, season.AirDate); err != nil {
			return err
		}
		for _, episode := range season.Episodes {
			if episode.ID == "" || episode.EpisodeNumber < 1 {
				return ErrInvalidImport
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO episodes (
					id, season_id, source_id, episode_number, name, overview, still_path, air_date, runtime
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(id) DO UPDATE SET
					season_id = excluded.season_id, source_id = excluded.source_id,
					episode_number = excluded.episode_number, name = excluded.name,
					overview = excluded.overview, still_path = excluded.still_path,
					air_date = excluded.air_date, runtime = excluded.runtime
			`, episode.ID, season.ID, nullableText(episode.SourceID), episode.EpisodeNumber,
				episode.Name, episode.Overview, episode.StillPath, episode.AirDate,
				nullableInt(episode.Runtime)); err != nil {
				return err
			}
		}
	}
	return nil
}

func importState(ctx context.Context, tx *sql.Tx, userID, mediaID string, state exportState) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO user_media_states (
			user_id, media_id, status, rating, note, version,
			status_source, rating_source, note_source, started_at, completed_at,
			share_rating, share_review, shared_review, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, strftime('%s', 'now') * 1000)
		ON CONFLICT(user_id, media_id) DO UPDATE SET
			status = excluded.status, rating = excluded.rating, note = excluded.note,
			version = excluded.version, status_source = excluded.status_source,
			rating_source = excluded.rating_source, note_source = excluded.note_source,
			started_at = excluded.started_at, completed_at = excluded.completed_at,
			share_rating = excluded.share_rating, share_review = excluded.share_review,
			shared_review = excluded.shared_review,
			updated_at = excluded.updated_at
	`, userID, mediaID, state.Status, nullableInt(state.Rating), nullableText(state.Note),
		state.Version, state.StatusSource, state.RatingSource, state.NoteSource,
		nullableText(state.StartedAt), nullableText(state.CompletedAt), boolInt(state.ShareRating),
		boolInt(state.ShareReview), nullableText(state.SharedReview)); err != nil {
		return err
	}
	return nil
}

func importTags(ctx context.Context, tx *sql.Tx, userID, mediaID string, tags []string) error {
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM user_media_tags WHERE user_id = ? AND media_id = ?
	`, userID, mediaID); err != nil {
		return err
	}
	seen := make(map[string]struct{})
	for _, name := range tags {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		tagID := uuid.NewString()
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO tags (id, user_id, name) VALUES (?, ?, ?)
			ON CONFLICT(user_id, name) DO NOTHING
		`, tagID, userID, name); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `
			SELECT id FROM tags WHERE user_id = ? AND name = ?
		`, userID, name).Scan(&tagID); err != nil {
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

func importEvents(ctx context.Context, tx *sql.Tx, userID, mediaID string, events []exportEvent) error {
	for _, event := range events {
		if event.ID == "" || event.WatchedAt == "" || sourcePriority(event.Source) == 0 ||
			event.Completion < 0 || event.Completion > 100 {
			return ErrInvalidImport
		}
		if _, err := time.Parse(eventTimeLayout, event.WatchedAt); err != nil {
			return ErrInvalidImport
		}
		var owner, existingMedia string
		err := tx.QueryRowContext(ctx, `
			SELECT created_by_user_id, media_id FROM watch_events WHERE id = ?
		`, event.ID).Scan(&owner, &existingMedia)
		if err == nil && (owner != userID || existingMedia != mediaID) {
			return ErrInvalidImport
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO watch_events (
				id, created_by_user_id, media_id, episode_id, watched_at,
				viewing_method, source, external_event_id, completion, note, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, strftime('%s', 'now') * 1000)
			ON CONFLICT(id) DO UPDATE SET
				episode_id = excluded.episode_id, watched_at = excluded.watched_at,
				viewing_method = excluded.viewing_method, source = excluded.source,
				external_event_id = excluded.external_event_id,
				completion = excluded.completion, note = excluded.note
		`, event.ID, userID, mediaID, nullableText(event.EpisodeID), event.WatchedAt,
			nullableText(event.ViewingMethod), event.Source, nullableText(event.ExternalEventID),
			event.Completion, nullableText(event.Note)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO watch_event_participants (event_id, user_id) VALUES (?, ?)
			ON CONFLICT(event_id, user_id) DO NOTHING
		`, event.ID, userID); err != nil {
			return err
		}
	}
	return nil
}

func importProgress(
	ctx context.Context,
	tx *sql.Tx,
	userID, mediaID string,
	progress []exportProgress,
) error {
	for _, item := range progress {
		if item.EpisodeID == "" || item.WatchedAt == "" || item.WatchEventID == "" ||
			sourcePriority(item.Source) == 0 {
			return ErrInvalidImport
		}
		if _, err := time.Parse(eventTimeLayout, item.WatchedAt); err != nil {
			return ErrInvalidImport
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO episode_progress (
				user_id, media_id, episode_id, watched_at, source, watch_event_id, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, strftime('%s', 'now') * 1000)
			ON CONFLICT(user_id, episode_id) DO UPDATE SET
				media_id = excluded.media_id, watched_at = excluded.watched_at,
				source = excluded.source, watch_event_id = excluded.watch_event_id,
				updated_at = excluded.updated_at
		`, userID, mediaID, item.EpisodeID, item.WatchedAt, item.Source, item.WatchEventID); err != nil {
			return err
		}
	}
	return nil
}

func (repository *SQLiteRepository) importCollection(
	ctx context.Context,
	userID string,
	collection exportCollection,
	importedMedia map[string]struct{},
) error {
	if collection.ID == "" || strings.TrimSpace(collection.Name) == "" {
		return ErrInvalidImport
	}
	tx, err := repository.db.Writer().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var existingOwner string
	err = tx.QueryRowContext(ctx, "SELECT user_id FROM collections WHERE id = ?", collection.ID).Scan(&existingOwner)
	if err == nil && existingOwner != userID {
		return ErrInvalidImport
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO collections (id, user_id, name) VALUES (?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name = excluded.name
	`, collection.ID, userID, strings.TrimSpace(collection.Name)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM collection_items WHERE collection_id = ?", collection.ID); err != nil {
		return err
	}
	for position, mediaID := range collection.MediaIDs {
		if _, ok := importedMedia[mediaID]; !ok {
			var exists int
			if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM media_items WHERE id = ?", mediaID).Scan(&exists); err != nil {
				return err
			}
			if exists == 0 {
				return ErrInvalidImport
			}
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO collection_items (collection_id, media_id, position) VALUES (?, ?, ?)
		`, collection.ID, mediaID, position); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func stringPointer(value string) *string {
	return &value
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
