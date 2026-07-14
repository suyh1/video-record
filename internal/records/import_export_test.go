package records

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"video-record/internal/auth"
	"video-record/internal/media"
	"video-record/internal/storage"
)

func TestViewingRoundJSONRoundTripPreservesArchivesEpisodesAndProfile(t *testing.T) {
	ctx := context.Background()
	service, db, userID, movieID := newTestRecordsService(t)
	mediaService := media.NewService(media.NewRepository(db))
	series, err := mediaService.CreateCustom(ctx, media.CreateCustomInput{
		MediaType: media.MediaTypeTV, Title: "轮次导出剧集",
	})
	require.NoError(t, err)
	seasonID := uuid.NewString()
	episodeID := uuid.NewString()
	_, err = db.Writer().ExecContext(ctx, `
		INSERT INTO seasons (id, media_id, season_number, name, overview, poster_path, air_date)
		VALUES (?, ?, 1, '第 1 季', '', '', '')
	`, seasonID, series.ID)
	require.NoError(t, err)
	_, err = db.Writer().ExecContext(ctx, `
		INSERT INTO episodes (id, season_id, episode_number, name, overview, still_path, air_date)
		VALUES (?, ?, 1, '第一集', '', '', '')
	`, episodeID, seasonID)
	require.NoError(t, err)
	rating := 93
	note := "电影第一轮"
	method := "家庭投影"
	movieTime := time.Date(2026, 7, 12, 20, 1, 2, 0, time.UTC)
	movieRound, err := service.UpdateRound(ctx, UpdateRoundInput{
		Scope:  RoundScope{UserID: userID, MediaID: movieID},
		Status: StatusCompleted, Rating: &rating, RatingSet: true,
		Note: &note, NoteSet: true, ViewingMethod: &method, ViewingMethodSet: true,
		CompletedAt: &movieTime, Source: SourceManual,
	})
	require.NoError(t, err)
	_, err = service.StartRewatch(ctx, RewatchInput{
		Scope: RoundScope{UserID: userID, MediaID: movieID}, ExpectedVersion: movieRound.Version,
	})
	require.NoError(t, err)
	episodeTime := time.Date(2026, 7, 13, 21, 2, 3, 0, time.UTC)
	seasonRound, err := service.UpdateEpisodeProgress(ctx, EpisodeProgressInput{
		UserID: userID, MediaID: series.ID, SeasonNumber: 1,
		Action: EpisodeProgressSingle, EpisodeID: episodeID,
		WatchedAt: episodeTime, Source: SourceManual, TotalEpisodes: 1,
	})
	require.NoError(t, err)
	seasonRewatch, err := service.StartRewatch(ctx, RewatchInput{
		Scope:           RoundScope{UserID: userID, MediaID: series.ID, SeasonNumber: integerPointer(1)},
		ExpectedVersion: seasonRound.Version,
	})
	require.NoError(t, err)
	require.Equal(t, 2, seasonRewatch.Current.RoundNumber)
	require.NoError(t, service.SetTags(ctx, userID, movieID, []string{"多刷", "收藏"}))
	collection, err := service.CreateCollection(ctx, userID, "轮次片单")
	require.NoError(t, err)
	require.NoError(t, service.AddCollectionItem(ctx, userID, collection.ID, movieID))
	_, err = db.Writer().ExecContext(ctx, `
		UPDATE user_media_profiles
		SET share_rating = 1, share_review = 1, shared_review = '公开短评'
		WHERE user_id = ? AND media_id = ?
	`, userID, movieID)
	require.NoError(t, err)

	exported, err := service.ExportData(ctx, userID, ExportFormatJSON)
	require.NoError(t, err)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(exported.Data, &raw))
	require.Equal(t, float64(2), raw["version"])
	require.Contains(t, string(exported.Data), `"rounds"`)
	require.NotContains(t, string(exported.Data), `"state"`)

	targetService, targetDB, targetUserID := newEmptyRecordsService(t)
	report, err := targetService.ImportData(ctx, targetUserID, exported.Filename, exported.Data)
	require.NoError(t, err)
	require.Equal(t, 2, report.ImportedRecords, "%+v", report)
	require.Empty(t, report.Failures)
	movieHistory, err := targetService.RoundHistory(ctx, RoundScope{UserID: targetUserID, MediaID: movieID})
	require.NoError(t, err)
	require.Len(t, movieHistory, 1)
	movieDetail, err := targetService.RoundDetail(
		ctx, RoundScope{UserID: targetUserID, MediaID: movieID}, movieHistory[0].ID,
	)
	require.NoError(t, err)
	require.Equal(t, rating, *movieDetail.Round.Rating)
	require.Equal(t, note, *movieDetail.Round.Note)
	seasonHistory, err := targetService.RoundHistory(ctx, RoundScope{
		UserID: targetUserID, MediaID: series.ID, SeasonNumber: integerPointer(1),
	})
	require.NoError(t, err)
	require.Len(t, seasonHistory, 1)
	seasonDetail, err := targetService.RoundDetail(ctx, RoundScope{
		UserID: targetUserID, MediaID: series.ID, SeasonNumber: integerPointer(1),
	}, seasonHistory[0].ID)
	require.NoError(t, err)
	require.Len(t, seasonDetail.Episodes, 1)
	require.Equal(t, episodeTime, *seasonDetail.Episodes[0].WatchedAt)
	var shareRating, shareReview int
	var sharedReview string
	require.NoError(t, targetDB.Reader().QueryRowContext(ctx, `
		SELECT share_rating, share_review, shared_review FROM user_media_profiles
		WHERE user_id = ? AND media_id = ?
	`, targetUserID, movieID).Scan(&shareRating, &shareReview, &sharedReview))
	require.Equal(t, 1, shareRating)
	require.Equal(t, 1, shareReview)
	require.Equal(t, "公开短评", sharedReview)
	tags, err := targetService.Tags(ctx, targetUserID, movieID)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"多刷", "收藏"}, tags)
	collections, err := targetService.Collections(ctx, targetUserID)
	require.NoError(t, err)
	require.Len(t, collections, 1)
	require.Equal(t, []string{movieID}, collections[0].Items)
}

func TestCSVExportRoundTripNeutralizesFormulasAndIncludesRoundScope(t *testing.T) {
	service, db, userID, movieID := newTestRecordsService(t)
	ctx := context.Background()
	_, err := db.Writer().ExecContext(ctx, "UPDATE media_items SET custom_title = '=2+3' WHERE id = ?", movieID)
	require.NoError(t, err)
	note := "@SUM(1,1)"
	completedAt := time.Date(2026, 7, 13, 20, 1, 2, 0, time.UTC)
	_, err = service.UpdateRound(ctx, UpdateRoundInput{
		Scope:  RoundScope{UserID: userID, MediaID: movieID},
		Status: StatusCompleted, Note: &note, NoteSet: true,
		CompletedAt: &completedAt, Source: SourceManual,
	})
	require.NoError(t, err)

	exported, err := service.ExportData(ctx, userID, ExportFormatCSV)
	require.NoError(t, err)
	require.Contains(t, string(exported.Data), "round_id,round_number,season_number")
	require.Contains(t, string(exported.Data), "'=2+3")
	require.Contains(t, string(exported.Data), "'@SUM(1,1)")
	require.False(t, bytes.Contains(exported.Data, []byte("\n=2+3")))
	targetService, _, targetUserID := newEmptyRecordsService(t)
	report, err := targetService.ImportData(ctx, targetUserID, exported.Filename, exported.Data)
	require.NoError(t, err)
	require.Equal(t, 1, report.ImportedRecords)
	current, err := targetService.CurrentRound(ctx, RoundScope{UserID: targetUserID, MediaID: movieID})
	require.NoError(t, err)
	require.Equal(t, StatusCompleted, current.Status)
	require.Equal(t, note, *current.Note)
}

func TestImportVersionAndFileValidation(t *testing.T) {
	service, _, userID := newEmptyRecordsService(t)
	ctx := context.Background()
	_, err := service.ImportData(ctx, userID, "legacy.json", []byte(
		`{"version":1,"records":[],"collections":[]}`,
	))
	require.ErrorIs(t, err, ErrInvalidImport)
	_, err = service.ImportData(ctx, userID, "../unsafe.json", []byte("{}"))
	require.ErrorIs(t, err, ErrUnsafeImportFilename)
	_, err = service.ImportData(ctx, userID, "large.json", make([]byte, maxImportBytes+1))
	require.ErrorIs(t, err, ErrImportTooLarge)
	_, err = service.ImportData(ctx, userID, "unknown.json", []byte(
		`{"version":2,"records":[],"collections":[],"unknown":true}`,
	))
	require.ErrorIs(t, err, ErrInvalidImport)
	_, err = service.ImportData(ctx, userID, "invalid.csv", []byte("bad,header\n"))
	require.ErrorIs(t, err, ErrInvalidImport)
}

func TestImportIsIdempotentAndReportsExternalIdentityConflicts(t *testing.T) {
	service, _, userID := newEmptyRecordsService(t)
	ctx := context.Background()
	record := validVersionTwoImportRecord("import-one", "Imported One", "9001")
	document := exportDocument{
		Version: exportVersion, Records: []exportRecord{record}, Collections: []exportCollection{},
	}
	data, err := json.Marshal(document)
	require.NoError(t, err)
	for range 2 {
		report, err := service.ImportData(ctx, userID, "records.json", data)
		require.NoError(t, err)
		require.Equal(t, 1, report.ImportedRecords, "%+v", report)
		require.Empty(t, report.Failures)
	}

	conflict := validVersionTwoImportRecord("import-two", "Imported Two", "9001")
	conflictData, err := json.Marshal(exportDocument{
		Version: exportVersion, Records: []exportRecord{conflict}, Collections: []exportCollection{},
	})
	require.NoError(t, err)
	report, err := service.ImportData(ctx, userID, "conflict.json", conflictData)
	require.NoError(t, err)
	require.Zero(t, report.ImportedRecords)
	require.Equal(t, []ImportFailure{{RecordID: "import-two", Code: "external_identity_conflict"}}, report.Failures)
}

func TestRichVersionTwoImportExportPreservesMetadataAndCollections(t *testing.T) {
	service, db, userID := newEmptyRecordsService(t)
	ctx := context.Background()
	record := validRichVersionTwoSeriesRecord()
	document := exportDocument{
		Version: exportVersion,
		Records: []exportRecord{record},
		Collections: []exportCollection{{
			ID: "rich-collection", Name: "  精选剧集  ", MediaIDs: []string{record.Media.ID},
		}},
	}
	data, err := json.Marshal(document)
	require.NoError(t, err)
	report, err := service.ImportData(ctx, userID, "rich.json", data)
	require.NoError(t, err)
	require.Equal(t, 1, report.ImportedRecords)
	require.Equal(t, 1, report.ImportedCollections)
	require.Empty(t, report.Failures)
	reimported, err := service.ImportData(ctx, userID, "rich.json", data)
	require.NoError(t, err)
	require.Equal(t, 1, reimported.ImportedRecords)
	require.Equal(t, 1, reimported.ImportedCollections)
	require.Empty(t, reimported.Failures)

	exported, err := service.ExportData(ctx, userID, ExportFormatJSON)
	require.NoError(t, err)
	csvExport, err := service.ExportData(ctx, userID, ExportFormatCSV)
	require.NoError(t, err)
	require.Contains(t, string(csvExport.Data), "rich-series,tv")
	require.Contains(t, string(csvExport.Data), "rich-round,1,1,completed,92")
	csvRecord := record
	csvRecord.Rounds = append([]exportRound(nil), record.Rounds...)
	customOverview := "自定义简介"
	archivedAt := formatEventTime(time.Date(2026, 7, 14, 8, 0, 0, 0, time.UTC))
	csvRecord.Media.CustomOverview = &customOverview
	csvRecord.Rounds[0].ArchivedAt = &archivedAt
	csvOnly, err := encodeExportCSV(exportDocument{
		Version: exportVersion,
		Records: []exportRecord{
			csvRecord,
			validVersionTwoImportRecord("organized-only", "仅整理", "8802"),
		},
		Collections: []exportCollection{},
	})
	require.NoError(t, err)
	require.Contains(t, string(csvOnly), "自定义简介")
	require.Contains(t, string(csvOnly), archivedAt)
	require.Contains(t, string(csvOnly), "organized-only,movie")
	var restored exportDocument
	require.NoError(t, json.Unmarshal(exported.Data, &restored))
	require.Len(t, restored.Records, 1)
	require.Equal(t, record.Media.ExternalIDs, restored.Records[0].Media.ExternalIDs)
	require.Equal(t, record.Media.Genres, restored.Records[0].Media.Genres)
	require.Equal(t, record.Media.Seasons, restored.Records[0].Media.Seasons)
	require.Equal(t, record.Profile, restored.Records[0].Profile)
	require.ElementsMatch(t, []string{"紧张", "夜晚"}, restored.Records[0].Tags)
	require.Equal(t, []exportCollection{{
		ID: "rich-collection", Name: "精选剧集", MediaIDs: []string{record.Media.ID},
	}}, restored.Collections)

	var eventParticipants int
	require.NoError(t, db.Reader().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM watch_event_participants WHERE event_id = 'rich-event'
	`).Scan(&eventParticipants))
	require.Equal(t, 1, eventParticipants)
	_, err = service.ExportData(ctx, "", ExportFormatJSON)
	require.ErrorIs(t, err, ErrInvalidExport)
	_, err = service.ExportData(ctx, userID, ExportFormat("xml"))
	require.ErrorIs(t, err, ErrInvalidExport)
}

func TestVersionTwoCSVDecoderCoversOptionalFieldsAndRejectsMalformedRows(t *testing.T) {
	header := []string{
		"media_id", "media_type", "title", "original_title", "release_date", "overview",
		"external_source", "external_id", "round_id", "round_number", "season_number",
		"status", "rating", "note", "viewing_method", "started_at", "completed_at",
		"archived_at", "tags",
	}
	watchedAt := formatEventTime(time.Date(2026, 7, 13, 20, 1, 2, 0, time.UTC))
	row := []string{
		"", "tv", "", "原名", "2026-07-01", "简介", "tmdb", "8801",
		"csv-round", "2", "1", "completed", "91", "私人笔记", "家庭电视",
		watchedAt, watchedAt, watchedAt, " 标签一 | |标签二 ",
	}
	document, err := decodeImportCSV(encodeCSVRowsForTest(t, header, row))
	require.NoError(t, err)
	require.Len(t, document.Records, 1)
	require.NotEmpty(t, document.Records[0].Media.ID)
	require.Equal(t, "Imported item", document.Records[0].Media.ExternalTitle)
	require.Equal(t, []string{"标签一", "标签二"}, document.Records[0].Tags)
	require.Equal(t, 2, document.Records[0].Rounds[0].RoundNumber)
	require.Equal(t, 1, *document.Records[0].Rounds[0].SeasonNumber)
	require.Equal(t, 91, *document.Records[0].Rounds[0].Rating)

	for _, invalid := range [][]byte{
		{},
		[]byte("\"unterminated"),
		encodeCSVRowsForTest(t, append([]string(nil), header[:len(header)-1]...)),
		encodeCSVRowsForTest(t, header, row[:len(row)-1]),
		encodeCSVRowsForTest(t, header, mutateCSVRow(row, 7, "")),
		encodeCSVRowsForTest(t, header, mutateCSVRow(row, 9, "not-a-number")),
		encodeCSVRowsForTest(t, header, mutateCSVRow(row, 10, "not-a-number")),
		encodeCSVRowsForTest(t, header, mutateCSVRow(row, 12, "not-a-number")),
	} {
		_, err := decodeImportCSV(invalid)
		require.ErrorIs(t, err, ErrInvalidImport)
	}
}

func TestVersionTwoImportValidationRejectsMalformedRoundFacts(t *testing.T) {
	invalidRecords := []exportRecord{
		mutateRichImportRecord(func(record *exportRecord) { record.Media.ID = "" }),
		mutateRichImportRecord(func(record *exportRecord) { record.Media.MediaType = "book" }),
		mutateRichImportRecord(func(record *exportRecord) {
			record.Media.ExternalTitle = ""
			record.Media.CustomTitle = nil
		}),
		mutateRichImportRecord(func(record *exportRecord) { record.Media.ExternalIDs[0].Source = "" }),
		mutateRichImportRecord(func(record *exportRecord) { record.Tags = nil }),
		mutateRichImportRecord(func(record *exportRecord) { record.Rounds = nil }),
		mutateRichImportRecord(func(record *exportRecord) { record.Profile.Version = 0 }),
		mutateRichImportRecord(func(record *exportRecord) { record.Rounds[0].ID = "" }),
		mutateRichImportRecord(func(record *exportRecord) { record.Rounds[0].RoundNumber = 0 }),
		mutateRichImportRecord(func(record *exportRecord) { record.Rounds[0].Version = 0 }),
		mutateRichImportRecord(func(record *exportRecord) { record.Rounds[0].Status = "paused" }),
		mutateRichImportRecord(func(record *exportRecord) { record.Rounds[0].StatusSource = "unknown" }),
		mutateRichImportRecord(func(record *exportRecord) { record.Rounds[0].RatingSource = "unknown" }),
		mutateRichImportRecord(func(record *exportRecord) { record.Rounds[0].NoteSource = "unknown" }),
		mutateRichImportRecord(func(record *exportRecord) { *record.Rounds[0].Rating = 101 }),
		mutateRichImportRecord(func(record *exportRecord) { record.Rounds[0].Events = nil }),
		mutateRichImportRecord(func(record *exportRecord) { record.Rounds[0].Episodes = nil }),
		mutateRichImportRecord(func(record *exportRecord) { record.Rounds[0].SeasonNumber = nil }),
		mutateRichImportRecord(func(record *exportRecord) { *record.Rounds[0].SeasonNumber = 0 }),
		mutateRichImportRecord(func(record *exportRecord) { record.Rounds[0].StartedAt = stringPointer("bad") }),
		mutateRichImportRecord(func(record *exportRecord) { record.Rounds[0].Events[0].ID = "" }),
		mutateRichImportRecord(func(record *exportRecord) { record.Rounds[0].Events[0].WatchedAt = "bad" }),
		mutateRichImportRecord(func(record *exportRecord) { record.Rounds[0].Events[0].Source = "unknown" }),
		mutateRichImportRecord(func(record *exportRecord) { record.Rounds[0].Events[0].Completion = 101 }),
		mutateRichImportRecord(func(record *exportRecord) { record.Rounds[0].Episodes[0].EpisodeID = "" }),
		mutateRichImportRecord(func(record *exportRecord) { record.Rounds[0].Episodes[0].WatchEventID = "" }),
		mutateRichImportRecord(func(record *exportRecord) { record.Rounds[0].Episodes[0].Source = "unknown" }),
		mutateRichImportRecord(func(record *exportRecord) { record.Rounds[0].Episodes[0].WatchedAt = "bad" }),
		mutateRichImportRecord(func(record *exportRecord) {
			second := record.Rounds[0]
			second.ID = "duplicate-number"
			second.ArchivedAt = stringPointer(formatEventTime(time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)))
			record.Rounds = append(record.Rounds, second)
		}),
		mutateRichImportRecord(func(record *exportRecord) {
			second := record.Rounds[0]
			second.ID = "duplicate-current"
			second.RoundNumber = 2
			record.Rounds = append(record.Rounds, second)
		}),
	}
	for _, record := range invalidRecords {
		require.ErrorIs(t, validateImportRecord(record), ErrInvalidImport)
	}

	movieWithSeason := validRichVersionTwoSeriesRecord()
	movieWithSeason.Media.MediaType = "movie"
	movieWithSeason.Media.ExternalIDs[0].MediaType = "movie"
	require.ErrorIs(t, validateImportRecord(movieWithSeason), ErrInvalidImport)
}

func TestImportReportsInvalidMetadataCollectionsAndDocumentDuplicates(t *testing.T) {
	service, db, userID := newEmptyRecordsService(t)
	ctx := context.Background()
	for _, record := range []exportRecord{
		mutateRichImportRecord(func(record *exportRecord) { record.Media.Genres[0].Source = "" }),
		mutateRichImportRecord(func(record *exportRecord) { record.Media.Seasons[0].ID = "" }),
		mutateRichImportRecord(func(record *exportRecord) { record.Media.Seasons[0].Episodes[0].ID = "" }),
	} {
		report, err := service.repository.ImportDocument(ctx, userID, exportDocument{
			Version: exportVersion, Records: []exportRecord{record}, Collections: []exportCollection{},
		})
		require.NoError(t, err)
		require.Equal(t, []ImportFailure{{RecordID: record.Media.ID, Code: "record_import_failed"}}, report.Failures)
	}

	first := validVersionTwoImportRecord("duplicate-one", "Duplicate One", "shared-id")
	second := validVersionTwoImportRecord("duplicate-two", "Duplicate Two", "shared-id")
	report, err := service.repository.ImportDocument(ctx, userID, exportDocument{
		Version: exportVersion, Records: []exportRecord{first, second}, Collections: []exportCollection{
			{ID: "", Name: "invalid", MediaIDs: []string{}},
			{ID: "missing-media", Name: "Missing", MediaIDs: []string{"does-not-exist"}},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, report.ImportedRecords)
	require.Equal(t, []ImportFailure{
		{RecordID: "duplicate-two", Code: "external_identity_conflict"},
		{RecordID: "", Code: "collection_import_failed"},
		{RecordID: "missing-media", Code: "collection_import_failed"},
	}, report.Failures)

	otherUserID := insertTestUser(t, db, "collection-import-owner")
	_, err = db.Writer().ExecContext(ctx, `
		INSERT INTO collections (id, user_id, name) VALUES ('owned-elsewhere', ?, 'Private')
	`, otherUserID)
	require.NoError(t, err)
	report, err = service.repository.ImportDocument(ctx, userID, exportDocument{
		Version: exportVersion, Records: []exportRecord{}, Collections: []exportCollection{{
			ID: "owned-elsewhere", Name: "Override", MediaIDs: []string{},
		}},
	})
	require.NoError(t, err)
	require.Equal(t, []ImportFailure{{RecordID: "owned-elsewhere", Code: "collection_import_failed"}}, report.Failures)
	_, err = service.repository.ImportDocument(ctx, userID, exportDocument{Version: 1})
	require.ErrorIs(t, err, ErrInvalidImport)

	_, err = db.Writer().ExecContext(ctx, `
		INSERT INTO watch_rounds (
			id, user_id, media_id, season_number, round_number, status, version,
			status_source, rating_source, note_source, created_at, updated_at
		) VALUES ('owned-round', ?, 'duplicate-one', NULL, 1, 'watching', 1,
		          'confirmed_import', 'confirmed_import', 'confirmed_import', 0, 0)
	`, otherUserID)
	require.NoError(t, err)
	foreignRound := validVersionTwoImportRecord("foreign-round-media", "Foreign Round", "foreign-round-id")
	foreignRound.Rounds = []exportRound{{
		ID: "owned-round", RoundNumber: 1, Status: StatusWatching, Version: 1,
		StatusSource: SourceConfirmedImport, RatingSource: SourceConfirmedImport,
		NoteSource: SourceConfirmedImport, Events: []exportEvent{}, Episodes: []exportRoundEpisode{},
	}}
	report, err = service.repository.ImportDocument(ctx, userID, exportDocument{
		Version: exportVersion, Records: []exportRecord{foreignRound}, Collections: []exportCollection{},
	})
	require.NoError(t, err)
	require.Equal(t, []ImportFailure{{RecordID: "foreign-round-media", Code: "record_import_failed"}}, report.Failures)
}

func validVersionTwoImportRecord(id, title, externalID string) exportRecord {
	return exportRecord{
		Media: exportMedia{
			ID: id, MediaType: "movie", ExternalTitle: title, OriginalTitle: title,
			ExternalIDs: []exportExternalID{{Source: "tmdb", SourceID: externalID, MediaType: "movie"}},
			Genres:      []exportGenre{}, Seasons: []exportSeason{},
		},
		Tags: []string{}, Rounds: []exportRound{},
	}
}

func validRichVersionTwoSeriesRecord() exportRecord {
	watchedAt := formatEventTime(time.Date(2026, 7, 13, 20, 1, 2, 0, time.UTC))
	rating := 92
	note := "完整轮次"
	method := "家庭影院"
	externalEventID := "provider-rich-event"
	eventNote := "同步事实"
	seasonNumber := 1
	seasonSourceID := "season-source"
	episodeSourceID := "episode-source"
	runtime := 48
	customTitle := "自定义剧名"
	sharedReview := "公开短评"
	return exportRecord{
		Media: exportMedia{
			ID: "rich-series", MediaType: "tv", ExternalTitle: "完整剧集", OriginalTitle: "Rich Series",
			ReleaseDate: "2026-07-01", ExternalOverview: "外部简介", CustomTitle: &customTitle,
			RuntimeMinutes: &runtime,
			ExternalIDs:    []exportExternalID{{Source: "tmdb", SourceID: "8801", MediaType: "tv"}},
			Genres:         []exportGenre{{Source: "tmdb", SourceID: "18", Name: "剧情"}},
			Seasons: []exportSeason{{
				ID: "rich-season", SourceID: &seasonSourceID, SeasonNumber: 1, Name: "第一季",
				Episodes: []exportEpisode{{
					ID: "rich-episode", SourceID: &episodeSourceID, EpisodeNumber: 1,
					Name: "第一集", Runtime: &runtime,
				}},
			}},
		},
		Profile: &exportProfile{Version: 3, ShareRating: true, ShareReview: true, SharedReview: &sharedReview},
		Tags:    []string{" 紧张 ", "夜晚", "紧张", ""},
		Rounds: []exportRound{{
			ID: "rich-round", SeasonNumber: &seasonNumber, RoundNumber: 1, Status: StatusCompleted,
			Rating: &rating, Note: &note, ViewingMethod: &method, StartedAt: &watchedAt, CompletedAt: &watchedAt,
			Version: 2, StatusSource: SourceConfirmedImport,
			RatingSource: SourceConfirmedImport, NoteSource: SourceConfirmedImport,
			Events: []exportEvent{{
				ID: "rich-event", EpisodeID: stringPointer("rich-episode"), WatchedAt: watchedAt,
				ViewingMethod: &method, Source: SourceConfirmedImport, ExternalEventID: &externalEventID,
				Completion: 100, Note: &eventNote,
			}},
			Episodes: []exportRoundEpisode{{
				EpisodeID: "rich-episode", WatchedAt: watchedAt, Source: SourceConfirmedImport,
				WatchEventID: "rich-event",
			}},
		}},
	}
}

func mutateRichImportRecord(mutate func(*exportRecord)) exportRecord {
	record := validRichVersionTwoSeriesRecord()
	mutate(&record)
	return record
}

func encodeCSVRowsForTest(t *testing.T, rows ...[]string) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := csv.NewWriter(&output)
	writer.WriteAll(rows)
	require.NoError(t, writer.Error())
	return output.Bytes()
}

func mutateCSVRow(row []string, index int, value string) []string {
	result := append([]string(nil), row...)
	result[index] = value
	return result
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
