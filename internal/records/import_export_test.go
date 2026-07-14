package records

import (
	"bytes"
	"context"
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
