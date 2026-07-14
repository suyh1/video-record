package records

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"video-record/internal/media"
	"video-record/internal/storage"
)

func TestApplySyncedWatchValidatesInputAndMediaScope(t *testing.T) {
	_, db, userID, movieID := newTestRecordsService(t)
	ctx := context.Background()
	base := SyncedWatchInput{
		UserID: userID, MediaID: movieID, WatchedAt: time.Date(2026, 7, 12, 20, 1, 2, 0, time.UTC),
		ExternalEventID: "sync-validation", Completion: 100,
	}
	for _, mutate := range []func(*SyncedWatchInput){
		func(input *SyncedWatchInput) { input.UserID = "" },
		func(input *SyncedWatchInput) { input.MediaID = "" },
		func(input *SyncedWatchInput) { input.ExternalEventID = "" },
		func(input *SyncedWatchInput) { input.WatchedAt = time.Time{} },
		func(input *SyncedWatchInput) { input.Completion = -1 },
		func(input *SyncedWatchInput) { input.Completion = 101 },
	} {
		input := base
		mutate(&input)
		_, err := applySyncedWatchForTest(t, db, input)
		require.ErrorIs(t, err, ErrInvalidWatchEvent)
	}

	invalidMovieEpisode := base
	invalidMovieEpisode.EpisodeID = "not-a-movie-episode"
	_, err := applySyncedWatchForTest(t, db, invalidMovieEpisode)
	require.ErrorIs(t, err, ErrInvalidRoundScope)

	missingMedia := base
	missingMedia.MediaID = "missing-media"
	_, err = applySyncedWatchForTest(t, db, missingMedia)
	require.ErrorIs(t, err, ErrInvalidRoundScope)

	mediaService := media.NewService(media.NewRepository(db))
	series, err := mediaService.CreateCustom(ctx, media.CreateCustomInput{
		MediaType: media.MediaTypeTV, Title: "同步作用域剧集",
	})
	require.NoError(t, err)
	seriesWithoutEpisode := base
	seriesWithoutEpisode.MediaID = series.ID
	_, err = applySyncedWatchForTest(t, db, seriesWithoutEpisode)
	require.ErrorIs(t, err, ErrInvalidRoundScope)

	missingEpisode := seriesWithoutEpisode
	missingEpisode.EpisodeID = "missing-episode"
	_, err = applySyncedWatchForTest(t, db, missingEpisode)
	require.ErrorIs(t, err, ErrEpisodeNotFound)
}

func TestApplySyncedWatchProjectsMovieRoundsAndSourcePriority(t *testing.T) {
	service, db, userID, movieID := newTestRecordsService(t)
	ctx := context.Background()
	firstTime := time.Date(2026, 7, 12, 20, 1, 2, 0, time.UTC)

	first, err := applySyncedWatchForTest(t, db, SyncedWatchInput{
		UserID: userID, MediaID: movieID, WatchedAt: firstTime,
		ViewingMethod: "媒体服务器", ExternalEventID: "movie-sync-1", Completion: 50,
	})
	require.NoError(t, err)
	require.NotEmpty(t, first.RoundID)
	require.NotEmpty(t, first.EventID)
	current, err := service.CurrentRound(ctx, RoundScope{UserID: userID, MediaID: movieID})
	require.NoError(t, err)
	require.Equal(t, first.RoundID, current.ID)
	require.Equal(t, 1, current.RoundNumber)
	require.Equal(t, StatusWatching, current.Status)
	require.Equal(t, firstTime, *current.StartedAt)
	require.Nil(t, current.CompletedAt)

	completedTime := firstTime.Add(time.Hour)
	completed, err := applySyncedWatchForTest(t, db, SyncedWatchInput{
		UserID: userID, MediaID: movieID, WatchedAt: completedTime,
		ExternalEventID: "movie-sync-2", Completion: 100,
	})
	require.NoError(t, err)
	require.Equal(t, first.RoundID, completed.RoundID)
	current, err = service.CurrentRound(ctx, RoundScope{UserID: userID, MediaID: movieID})
	require.NoError(t, err)
	require.Equal(t, StatusCompleted, current.Status)
	require.Equal(t, completedTime, *current.CompletedAt)
	require.Equal(t, 2, current.Version)

	rewatchTime := completedTime.Add(24 * time.Hour)
	rewatch, err := applySyncedWatchForTest(t, db, SyncedWatchInput{
		UserID: userID, MediaID: movieID, WatchedAt: rewatchTime,
		ExternalEventID: "movie-sync-3", Completion: 100,
	})
	require.NoError(t, err)
	require.NotEqual(t, completed.RoundID, rewatch.RoundID)
	current, err = service.CurrentRound(ctx, RoundScope{UserID: userID, MediaID: movieID})
	require.NoError(t, err)
	require.Equal(t, rewatch.RoundID, current.ID)
	require.Equal(t, 2, current.RoundNumber)
	require.Equal(t, StatusCompleted, current.Status)
	history, err := service.RoundHistory(ctx, RoundScope{UserID: userID, MediaID: movieID})
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.Equal(t, completed.RoundID, history[0].ID)

	_, err = db.Writer().ExecContext(ctx, `
		UPDATE watch_rounds SET status_source = 'manual' WHERE id = ?
	`, current.ID)
	require.NoError(t, err)
	_, err = applySyncedWatchForTest(t, db, SyncedWatchInput{
		UserID: userID, MediaID: movieID, WatchedAt: rewatchTime.Add(time.Hour),
		ExternalEventID: "movie-sync-rejected", Completion: 100,
	})
	require.ErrorIs(t, err, ErrVersionConflict)
	var eventCount int
	require.NoError(t, db.Reader().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM watch_events WHERE media_id = ?", movieID,
	).Scan(&eventCount))
	require.Equal(t, 3, eventCount)
}

func TestApplySyncedWatchProjectsSeasonProgressAndStartsNextRound(t *testing.T) {
	service, db, userID, seriesID, seasons := newTestSeriesService(t)
	ctx := context.Background()
	firstTime := time.Date(2026, 7, 12, 20, 1, 2, 0, time.UTC)

	partial, err := applySyncedWatchForTest(t, db, SyncedWatchInput{
		UserID: userID, MediaID: seriesID, EpisodeID: seasons[0][0], WatchedAt: firstTime,
		ExternalEventID: "episode-sync-partial", Completion: 50,
	})
	require.NoError(t, err)
	progress, err := service.EpisodeProgress(ctx, userID, seriesID, 1)
	require.NoError(t, err)
	require.Equal(t, partial.RoundID, progress.RoundID)
	require.Equal(t, StatusWatching, progress.Status)
	require.Zero(t, progress.WatchedEpisodes)

	for index, episodeID := range seasons[0] {
		_, err = applySyncedWatchForTest(t, db, SyncedWatchInput{
			UserID: userID, MediaID: seriesID, EpisodeID: episodeID,
			WatchedAt:       firstTime.Add(time.Duration(index+1) * time.Hour),
			ExternalEventID: "episode-sync-complete-" + string(rune('1'+index)), Completion: 100,
		})
		require.NoError(t, err)
	}
	progress, err = service.EpisodeProgress(ctx, userID, seriesID, 1)
	require.NoError(t, err)
	require.Equal(t, StatusCompleted, progress.Status)
	require.Equal(t, 3, progress.WatchedEpisodes)
	completedRoundID := progress.RoundID

	seasonTwo, err := applySyncedWatchForTest(t, db, SyncedWatchInput{
		UserID: userID, MediaID: seriesID, EpisodeID: seasons[1][0], WatchedAt: firstTime.Add(5 * time.Hour),
		ExternalEventID: "episode-sync-season-two", Completion: 100,
	})
	require.NoError(t, err)
	seasonTwoProgress, err := service.EpisodeProgress(ctx, userID, seriesID, 2)
	require.NoError(t, err)
	require.Equal(t, seasonTwo.RoundID, seasonTwoProgress.RoundID)
	require.Equal(t, StatusWatching, seasonTwoProgress.Status)
	require.Equal(t, 1, seasonTwoProgress.WatchedEpisodes)

	next, err := applySyncedWatchForTest(t, db, SyncedWatchInput{
		UserID: userID, MediaID: seriesID, EpisodeID: seasons[0][0], WatchedAt: firstTime.Add(24 * time.Hour),
		ExternalEventID: "episode-sync-next-round", Completion: 100,
	})
	require.NoError(t, err)
	require.NotEqual(t, completedRoundID, next.RoundID)
	progress, err = service.EpisodeProgress(ctx, userID, seriesID, 1)
	require.NoError(t, err)
	require.Equal(t, next.RoundID, progress.RoundID)
	require.Equal(t, StatusWatching, progress.Status)
	require.Equal(t, 1, progress.WatchedEpisodes)
	history, err := service.RoundHistory(ctx, RoundScope{
		UserID: userID, MediaID: seriesID, SeasonNumber: integerPointer(1),
	})
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.Equal(t, completedRoundID, history[0].ID)
}

func applySyncedWatchForTest(
	t *testing.T,
	db *storage.DB,
	input SyncedWatchInput,
) (SyncedWatchResult, error) {
	t.Helper()
	tx, err := db.Writer().BeginTx(context.Background(), nil)
	require.NoError(t, err)
	result, err := ApplySyncedWatch(context.Background(), tx, input)
	if err != nil {
		require.NoError(t, tx.Rollback())
		return SyncedWatchResult{}, err
	}
	require.NoError(t, tx.Commit())
	return result, nil
}
