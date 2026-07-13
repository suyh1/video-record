package records

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"video-record/internal/auth"
	"video-record/internal/storage"
)

func TestJSONExportImportRoundTripIsUserScoped(t *testing.T) {
	ctx := context.Background()
	service, db, userID, mediaID := newTestRecordsService(t)
	_, err := db.Writer().ExecContext(ctx, `
		UPDATE media_items SET custom_title = ? WHERE id = ?
	`, "=HYPERLINK(\"https://example.invalid\")", mediaID)
	require.NoError(t, err)
	_, err = db.Writer().ExecContext(ctx, `
		INSERT INTO media_external_ids (media_id, source, source_id, media_type)
		VALUES (?, 'tmdb', '42', 'movie')
	`, mediaID)
	require.NoError(t, err)
	rating := 93
	note := "owner-private-note"
	state, event, err := service.RecordStatus(ctx, RecordStatusInput{UpdateStateInput: UpdateStateInput{
		UserID: userID, MediaID: mediaID, Status: StatusCompleted,
		Rating: &rating, Note: &note, Source: SourceManual, ExpectedVersion: 0,
	}, WatchedAt: time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC), ViewingMethod: "家庭投影"})
	require.NoError(t, err)
	require.NotNil(t, event)
	_, err = db.Writer().ExecContext(ctx, `
		UPDATE user_media_states
		SET share_rating = 1, share_review = 1, shared_review = 'shared review'
		WHERE user_id = ? AND media_id = ?
	`, userID, mediaID)
	require.NoError(t, err)
	require.NoError(t, service.SetTags(ctx, userID, mediaID, []string{"家庭", "科幻"}))
	collection, err := service.CreateCollection(ctx, userID, "周末电影")
	require.NoError(t, err)
	require.NoError(t, service.AddCollectionItem(ctx, userID, collection.ID, mediaID))

	otherUserID := insertTestUser(t, db, "other-member")
	otherNote := "other-member-secret"
	_, err = service.UpdateState(ctx, UpdateStateInput{
		UserID: otherUserID, MediaID: mediaID, Status: StatusWishlist,
		Note: &otherNote, Source: SourceManual, ExpectedVersion: 0,
	})
	require.NoError(t, err)

	exported, err := service.ExportData(ctx, userID, ExportFormatJSON)
	require.NoError(t, err)
	require.Equal(t, "video-record-export.json", exported.Filename)
	require.Equal(t, "application/json", exported.ContentType)
	require.NotContains(t, string(exported.Data), otherNote)
	require.NotContains(t, string(exported.Data), "password_hash")
	require.NotContains(t, string(exported.Data), "TMDB_READ_ACCESS_TOKEN")

	targetService, _, targetUserID := newEmptyRecordsService(t)
	report, err := targetService.ImportData(ctx, targetUserID, exported.Filename, exported.Data)
	require.NoError(t, err)
	require.Equal(t, 1, report.ImportedRecords, "%+v", report)
	require.Empty(t, report.Failures)
	targetExport, err := targetService.ExportData(ctx, targetUserID, ExportFormatJSON)
	require.NoError(t, err)
	require.JSONEq(t, string(exported.Data), string(targetExport.Data))

	targetState, exists, err := targetService.repository.FindState(ctx, targetUserID, mediaID)
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, state.Status, targetState.Status)
	require.Equal(t, note, *targetState.Note)
}

func TestCSVExportNeutralizesSpreadsheetFormulas(t *testing.T) {
	service, db, userID, mediaID := newTestRecordsService(t)
	_, err := db.Writer().ExecContext(context.Background(), `
		UPDATE media_items SET custom_title = ? WHERE id = ?
	`, "=2+3", mediaID)
	require.NoError(t, err)
	note := "@SUM(1,1)"
	rating := 77
	_, _, err = service.RecordStatus(context.Background(), RecordStatusInput{
		UpdateStateInput: UpdateStateInput{
			UserID: userID, MediaID: mediaID, Status: StatusCompleted,
			Rating: &rating, Note: &note, Source: SourceManual, ExpectedVersion: 0,
		},
		WatchedAt: time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.NoError(t, service.SetTags(context.Background(), userID, mediaID, []string{"家庭", "科幻"}))

	exported, err := service.ExportData(context.Background(), userID, ExportFormatCSV)
	require.NoError(t, err)
	require.Equal(t, "video-record-export.csv", exported.Filename)
	require.Equal(t, "text/csv; charset=utf-8", exported.ContentType)
	require.Contains(t, string(exported.Data), "'=2+3")
	require.Contains(t, string(exported.Data), "'@SUM(1,1)")
	require.False(t, bytes.Contains(exported.Data, []byte("\n=2+3")))

	targetService, _, targetUserID := newEmptyRecordsService(t)
	report, err := targetService.ImportData(context.Background(), targetUserID, exported.Filename, exported.Data)
	require.NoError(t, err)
	require.Equal(t, 1, report.ImportedRecords)
	targetExport, err := targetService.ExportData(context.Background(), targetUserID, ExportFormatCSV)
	require.NoError(t, err)
	require.Equal(t, string(exported.Data), string(targetExport.Data))
}

func TestJSONRoundTripPreservesSeriesGenresAndEpisodeProgress(t *testing.T) {
	ctx := context.Background()
	service, db, userID, mediaID, seasons := newTestSeriesService(t)
	_, err := db.Writer().ExecContext(ctx, `
		INSERT INTO genres (source, source_id, name) VALUES ('tmdb', '18', '剧情')
	`)
	require.NoError(t, err)
	_, err = db.Writer().ExecContext(ctx, `
		INSERT INTO media_genres (media_id, source, source_id) VALUES (?, 'tmdb', '18')
	`, mediaID)
	require.NoError(t, err)
	_, err = db.Writer().ExecContext(ctx, `
		UPDATE seasons SET source_id = 'season-1', poster_path = '/season.jpg', air_date = '2026-01-01'
		WHERE media_id = ? AND season_number = 1
	`, mediaID)
	require.NoError(t, err)
	_, err = db.Writer().ExecContext(ctx, `
		UPDATE episodes SET source_id = 'episode-1', runtime = 48, still_path = '/episode.jpg', air_date = '2026-01-02'
		WHERE id = ?
	`, seasons[0][0])
	require.NoError(t, err)

	progress, err := service.UpdateEpisodeProgress(ctx, EpisodeProgressInput{
		UserID: userID, MediaID: mediaID, Action: EpisodeProgressSingle,
		EpisodeID: seasons[0][0], WatchedAt: time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC),
		Source: SourceManual, ExpectedVersion: 0,
	})
	require.NoError(t, err)
	require.Equal(t, 1, progress.WatchedEpisodes)

	exported, err := service.ExportData(ctx, userID, ExportFormatJSON)
	require.NoError(t, err)
	targetService, _, targetUserID := newEmptyRecordsService(t)
	report, err := targetService.ImportData(ctx, targetUserID, exported.Filename, exported.Data)
	require.NoError(t, err)
	require.Equal(t, 1, report.ImportedRecords, "%+v", report)
	targetExport, err := targetService.ExportData(ctx, targetUserID, ExportFormatJSON)
	require.NoError(t, err)
	require.JSONEq(t, string(exported.Data), string(targetExport.Data))
	targetProgress, err := targetService.EpisodeProgress(ctx, targetUserID, mediaID)
	require.NoError(t, err)
	require.Equal(t, 1, targetProgress.WatchedEpisodes)
	require.Equal(t, 6, targetProgress.TotalEpisodes)
}

func TestImportRejectsHostileFilesAndReportsDuplicateExternalIdentities(t *testing.T) {
	service, _, userID := newEmptyRecordsService(t)
	validDocument := exportDocument{
		Version: exportVersion, Collections: []exportCollection{},
		Records: []exportRecord{
			portableRecord("media-1", "Shared identity"),
			portableRecord("media-2", "Shared identity"),
		},
	}
	data, err := json.Marshal(validDocument)
	require.NoError(t, err)

	_, err = service.ImportData(context.Background(), userID, "../escape.json", data)
	require.ErrorIs(t, err, ErrUnsafeImportFilename)
	_, err = service.ImportData(context.Background(), userID, "bad.json", []byte{0xff, 0xfe})
	require.ErrorIs(t, err, ErrInvalidImport)
	_, err = service.ImportData(context.Background(), userID, "large.json", make([]byte, maxImportBytes+1))
	require.ErrorIs(t, err, ErrImportTooLarge)

	report, err := service.ImportData(context.Background(), userID, "records.json", data)
	require.NoError(t, err)
	require.Equal(t, 1, report.ImportedRecords)
	require.Len(t, report.Failures, 1)
	require.Equal(t, "media-2", report.Failures[0].RecordID)
	require.Equal(t, "external_identity_conflict", report.Failures[0].Code)
}

func TestImportValidationRejectsMalformedDocumentsAndNestedRecords(t *testing.T) {
	ctx := context.Background()
	service, _, userID := newEmptyRecordsService(t)
	_, err := service.ExportData(ctx, "", ExportFormatJSON)
	require.ErrorIs(t, err, ErrInvalidExport)
	_, err = service.ExportData(ctx, userID, "xml")
	require.ErrorIs(t, err, ErrInvalidExport)
	_, err = service.ImportData(ctx, userID, "records.txt", []byte("content"))
	require.ErrorIs(t, err, ErrInvalidImport)

	invalidJSON := [][]byte{
		[]byte(`{"version":1`),
		[]byte(`{"version":1,"records":[],"collections":[]} trailing`),
		[]byte(`{"version":2,"records":[],"collections":[]}`),
		[]byte(`{"version":1,"records":null,"collections":[]}`),
		[]byte(`{"version":1,"records":[],"collections":[],"unknown":true}`),
	}
	for _, data := range invalidJSON {
		_, err = service.ImportData(ctx, userID, "records.json", data)
		require.ErrorIs(t, err, ErrInvalidImport)
	}
	invalidCSV := [][]byte{
		{},
		[]byte("wrong,header\n"),
		[]byte("media_id,media_type,title,original_title,release_date,overview,external_source,external_id,status,rating,note,started_at,completed_at,tags\nonly-one-field\n"),
		[]byte("media_id,media_type,title,original_title,release_date,overview,external_source,external_id,status,rating,note,started_at,completed_at,tags\nmedia-1,movie,Title,,,,tmdb,,wishlist,,,,,\n"),
		[]byte("media_id,media_type,title,original_title,release_date,overview,external_source,external_id,status,rating,note,started_at,completed_at,tags\nmedia-1,movie,Title,,,,,,wishlist,not-a-number,,,,\n"),
	}
	for _, data := range invalidCSV {
		_, err = service.ImportData(ctx, userID, "records.csv", data)
		require.ErrorIs(t, err, ErrInvalidImport)
	}
	minimalCSV := strings.Join([]string{
		"media_id,media_type,title,original_title,release_date,overview,external_source,external_id,status,rating,note,started_at,completed_at,tags",
		strings.Join([]string{"", "movie", "", "", "", "", "", "", "", "", "", "", "", ""}, ","),
		"",
	}, "\n")
	minimalReport, err := service.ImportData(ctx, userID, "minimal.csv", []byte(minimalCSV))
	require.NoError(t, err)
	require.Equal(t, 1, minimalReport.ImportedRecords)

	repository := service.repository.(*SQLiteRepository)
	invalidRecords := []exportRecord{
		{},
		{Media: exportMedia{ID: "bad-type", MediaType: "book", ExternalTitle: "Bad"}},
		{Media: exportMedia{ID: "no-title", MediaType: "movie"}},
		{
			Media: exportMedia{
				ID: "bad-external", MediaType: "movie", ExternalTitle: "Bad",
				ExternalIDs: []exportExternalID{{Source: "", SourceID: "1", MediaType: "movie"}},
			},
		},
		{
			Media: exportMedia{ID: "bad-state", MediaType: "movie", ExternalTitle: "Bad"},
			State: &exportState{Status: "unknown", Version: 0},
		},
	}
	for _, record := range invalidRecords {
		require.ErrorIs(t, repository.importRecord(ctx, userID, record), ErrInvalidImport)
	}

	nestedInvalid := []exportRecord{
		withNestedRecord("bad-genre", func(record *exportRecord) {
			record.Media.Genres = []exportGenre{{Source: "", SourceID: "18", Name: "剧情"}}
		}),
		withNestedRecord("bad-season", func(record *exportRecord) {
			record.Media.Seasons = []exportSeason{{ID: "", SeasonNumber: 1}}
		}),
		withNestedRecord("bad-episode", func(record *exportRecord) {
			record.Media.Seasons = []exportSeason{{
				ID: "season-1", SeasonNumber: 1,
				Episodes: []exportEpisode{{ID: "", EpisodeNumber: 1}},
			}}
		}),
		withNestedRecord("bad-event", func(record *exportRecord) {
			record.Events = []exportEvent{{ID: "event-1", WatchedAt: "not-a-time", Source: SourceManual, Completion: 100}}
		}),
		withNestedRecord("bad-progress", func(record *exportRecord) {
			record.Progress = []exportProgress{{
				EpisodeID: "episode-1", WatchedAt: "not-a-time",
				Source: SourceManual, WatchEventID: "event-1",
			}}
		}),
	}
	for _, record := range nestedInvalid {
		require.ErrorIs(t, repository.importRecord(ctx, userID, record), ErrInvalidImport)
	}

	_, err = repository.ImportDocument(ctx, userID, exportDocument{Version: 99})
	require.ErrorIs(t, err, ErrInvalidImport)
	require.ErrorIs(t, repository.importCollection(ctx, userID, exportCollection{}, nil), ErrInvalidImport)
	require.ErrorIs(t, repository.importCollection(ctx, userID, exportCollection{
		ID: "collection-1", Name: "Missing", MediaIDs: []string{"missing"},
	}, map[string]struct{}{}), ErrInvalidImport)
}

func TestImportIsIdempotentAndDetectsStoredExternalIdentityConflicts(t *testing.T) {
	ctx := context.Background()
	service, _, userID := newEmptyRecordsService(t)
	record := portableRecord("media-1", "Imported")
	document := exportDocument{
		Version:     exportVersion,
		Records:     []exportRecord{record},
		Collections: []exportCollection{{ID: "collection-1", Name: "Imported list", MediaIDs: []string{"media-1"}}},
	}
	data, err := json.Marshal(document)
	require.NoError(t, err)
	first, err := service.ImportData(ctx, userID, "records.json", data)
	require.NoError(t, err)
	require.Equal(t, 1, first.ImportedRecords)
	require.Equal(t, 1, first.ImportedCollections)
	second, err := service.ImportData(ctx, userID, "records.json", data)
	require.NoError(t, err)
	require.Equal(t, 1, second.ImportedRecords)
	require.Equal(t, 1, second.ImportedCollections)
	require.Empty(t, second.Failures)

	conflict := portableRecord("media-2", "Conflict")
	conflictData, err := json.Marshal(exportDocument{
		Version: exportVersion, Records: []exportRecord{conflict}, Collections: []exportCollection{},
	})
	require.NoError(t, err)
	report, err := service.ImportData(ctx, userID, "conflict.json", conflictData)
	require.NoError(t, err)
	require.Equal(t, 0, report.ImportedRecords)
	require.Equal(t, "external_identity_conflict", report.Failures[0].Code)
}

func TestImportCannotOverwriteSharedMediaOrTakeAnotherUsersCollection(t *testing.T) {
	ctx := context.Background()
	service, db, ownerID := newEmptyRecordsService(t)
	original := portableRecord("media-1", "Original title")
	original.Media.ExternalIDs = []exportExternalID{}
	originalDocument, err := json.Marshal(exportDocument{
		Version: exportVersion, Records: []exportRecord{original}, Collections: []exportCollection{},
	})
	require.NoError(t, err)
	_, err = service.ImportData(ctx, ownerID, "original.json", originalDocument)
	require.NoError(t, err)
	_, err = db.Writer().ExecContext(ctx, `
		INSERT INTO collections (id, user_id, name) VALUES ('shared-collection', ?, 'Owner list')
	`, ownerID)
	require.NoError(t, err)

	attackerID := insertTestUser(t, db, "import-attacker")
	malicious := portableRecord("media-1", "Overwritten title")
	malicious.Media.ExternalIDs = []exportExternalID{}
	maliciousData, err := json.Marshal(exportDocument{
		Version: exportVersion,
		Records: []exportRecord{malicious},
		Collections: []exportCollection{{
			ID: "shared-collection", Name: "Taken list", MediaIDs: []string{"media-1"},
		}},
	})
	require.NoError(t, err)
	report, err := service.ImportData(ctx, attackerID, "malicious.json", maliciousData)
	require.NoError(t, err)
	require.Equal(t, 1, report.ImportedRecords)
	require.Equal(t, 0, report.ImportedCollections)
	require.Equal(t, "collection_import_failed", report.Failures[0].Code)

	var title, collectionOwner, collectionName string
	require.NoError(t, db.Reader().QueryRowContext(ctx, `
		SELECT external_title FROM media_items WHERE id = 'media-1'
	`).Scan(&title))
	require.Equal(t, "Original title", title)
	require.NoError(t, db.Reader().QueryRowContext(ctx, `
		SELECT user_id, name FROM collections WHERE id = 'shared-collection'
	`).Scan(&collectionOwner, &collectionName))
	require.Equal(t, ownerID, collectionOwner)
	require.Equal(t, "Owner list", collectionName)
}

func withNestedRecord(id string, mutate func(*exportRecord)) exportRecord {
	record := exportRecord{
		Media: exportMedia{
			ID: id, MediaType: "movie", ExternalTitle: "Nested",
			ExternalIDs: []exportExternalID{}, Genres: []exportGenre{}, Seasons: []exportSeason{},
		},
		Tags: []string{}, Events: []exportEvent{}, Progress: []exportProgress{},
	}
	mutate(&record)
	return record
}

func portableRecord(id, title string) exportRecord {
	return exportRecord{
		Media: exportMedia{
			ID: id, MediaType: "movie", ExternalTitle: title, OriginalTitle: title,
			ExternalIDs: []exportExternalID{{Source: "tmdb", SourceID: "9001", MediaType: "movie"}},
		},
		State: &exportState{Status: StatusWishlist, Version: 1, StatusSource: SourceConfirmedImport},
	}
}

func newEmptyRecordsService(t *testing.T) (*Service, *storage.DB, string) {
	t.Helper()
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(ctx, db))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	authService := auth.NewService(auth.NewRepository(db), auth.ServiceOptions{})
	user, err := authService.Initialize(ctx, "owner-"+uuid.NewString(), "correct horse battery staple")
	require.NoError(t, err)
	return NewService(NewRepository(db)), db, user.ID
}
