package records

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"video-record/internal/media"
	"video-record/internal/storage"
)

func TestEpisodeProgressSupportsSingleRangeSeasonAndAbsoluteCount(t *testing.T) {
	service, _, userID, mediaID, seasons := newTestSeriesService(t)
	ctx := context.Background()
	watchedAt := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)

	progress, err := service.UpdateEpisodeProgress(ctx, EpisodeProgressInput{
		UserID: userID, MediaID: mediaID, Action: EpisodeProgressSingle,
		EpisodeID: seasons[0][0], WatchedAt: watchedAt, Source: SourceManual,
	})
	require.NoError(t, err)
	require.Equal(t, 1, progress.WatchedEpisodes)
	require.Equal(t, StatusWatching, progress.Status)
	require.Equal(t, 1, progress.Version)
	require.Equal(t, 1, progress.Episodes[0].AbsoluteNumber)
	require.True(t, progress.Episodes[0].Watched)

	progress, err = service.UpdateEpisodeProgress(ctx, EpisodeProgressInput{
		UserID: userID, MediaID: mediaID, Action: EpisodeProgressRange,
		EpisodeID: seasons[0][1], ThroughEpisodeID: seasons[0][2],
		WatchedAt: watchedAt.Add(time.Hour), Source: SourceManual, ExpectedVersion: progress.Version,
	})
	require.NoError(t, err)
	require.Equal(t, 3, progress.WatchedEpisodes)
	require.Equal(t, 3, progress.Episodes[2].AbsoluteNumber)

	progress, err = service.UpdateEpisodeProgress(ctx, EpisodeProgressInput{
		UserID: userID, MediaID: mediaID, Action: EpisodeProgressSeason,
		SeasonID:  seasonIDForEpisode(t, service, userID, mediaID, seasons[1][0]),
		WatchedAt: watchedAt.Add(2 * time.Hour), Source: SourceManual, ExpectedVersion: progress.Version,
	})
	require.NoError(t, err)
	require.Equal(t, 6, progress.WatchedEpisodes)
	require.Equal(t, 6, progress.TotalEpisodes)
	require.Equal(t, StatusCompleted, progress.Status)
	require.Equal(t, 5, progress.Episodes[4].AbsoluteNumber)
	require.Equal(t, 2, progress.Episodes[4].SeasonNumber)
	require.Equal(t, 2, progress.Episodes[4].EpisodeNumber)
	require.Len(t, mustWatchEvents(t, service, userID, mediaID), 6)
}

func TestEpisodeProgressAdvancesNextAndUndoesItsEvent(t *testing.T) {
	service, _, userID, mediaID, seasons := newTestSeriesService(t)
	ctx := context.Background()

	progress, err := service.UpdateEpisodeProgress(ctx, EpisodeProgressInput{
		UserID: userID, MediaID: mediaID, Action: EpisodeProgressNext,
		WatchedAt: time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC), Source: SourceManual,
	})
	require.NoError(t, err)
	require.NotNil(t, progress.LastWatched)
	require.Equal(t, seasons[0][0], progress.LastWatched.ID)
	require.NotNil(t, progress.NextEpisode)
	require.Equal(t, seasons[0][1], progress.NextEpisode.ID)
	require.Len(t, mustWatchEvents(t, service, userID, mediaID), 1)

	progress, err = service.UpdateEpisodeProgress(ctx, EpisodeProgressInput{
		UserID: userID, MediaID: mediaID, Action: EpisodeProgressUndo,
		EpisodeID: seasons[0][0], Source: SourceManual, ExpectedVersion: progress.Version,
	})
	require.NoError(t, err)
	require.Zero(t, progress.WatchedEpisodes)
	require.False(t, progress.Episodes[0].Watched)
	require.Empty(t, mustWatchEvents(t, service, userID, mediaID))
}

func TestEpisodeProgressCompletesSeriesWithoutOverwritingDropped(t *testing.T) {
	service, _, userID, mediaID, seasons := newTestSeriesService(t)
	dropped, err := service.UpdateState(context.Background(), UpdateStateInput{
		UserID: userID, MediaID: mediaID, Status: StatusDropped,
		Source: SourceManual, ExpectedVersion: 0,
	})
	require.NoError(t, err)

	progress, err := service.UpdateEpisodeProgress(context.Background(), EpisodeProgressInput{
		UserID: userID, MediaID: mediaID, Action: EpisodeProgressRange,
		EpisodeID: seasons[0][0], ThroughEpisodeID: seasons[1][2],
		WatchedAt: time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC),
		Source:    SourceManual, ExpectedVersion: dropped.Version,
	})
	require.NoError(t, err)
	require.Equal(t, StatusDropped, progress.Status)
	require.Equal(t, 6, progress.WatchedEpisodes)
	require.Greater(t, progress.Version, dropped.Version)
	events := mustWatchEvents(t, service, userID, mediaID)
	require.NoError(t, service.DeleteWatchEvent(context.Background(), userID, events[len(events)-1].ID))
	progress, err = service.EpisodeProgress(context.Background(), userID, mediaID)
	require.NoError(t, err)
	require.Equal(t, StatusDropped, progress.Status)
	require.Equal(t, 5, progress.WatchedEpisodes)
}

func TestEpisodeProgressValidatesSelectorsVersionsAndNoopActions(t *testing.T) {
	service, _, userID, mediaID, seasons := newTestSeriesService(t)
	ctx := context.Background()

	_, err := service.EpisodeProgress(ctx, "", mediaID)
	require.ErrorIs(t, err, ErrInvalidEpisodeProgress)
	initial, err := service.EpisodeProgress(ctx, userID, mediaID)
	require.NoError(t, err)
	require.Zero(t, initial.Version)
	require.Len(t, initial.Episodes, 6)
	_, err = service.repository.ApplyEpisodeProgress(ctx, EpisodeProgressInput{
		UserID: userID, MediaID: mediaID, Source: SourceManual,
	}, []string{"missing"}, true)
	require.ErrorIs(t, err, ErrEpisodeNotFound)

	invalidInputs := []EpisodeProgressInput{
		{MediaID: mediaID, Action: EpisodeProgressNext, Source: SourceManual},
		{UserID: userID, Action: EpisodeProgressNext, Source: SourceManual},
		{UserID: userID, MediaID: mediaID, Action: EpisodeProgressNext, Source: "unknown"},
		{UserID: userID, MediaID: mediaID, Action: "unknown", Source: SourceManual},
		{UserID: userID, MediaID: mediaID, Action: EpisodeProgressSingle, EpisodeID: "missing", Source: SourceManual},
		{UserID: userID, MediaID: mediaID, Action: EpisodeProgressRange, EpisodeID: seasons[1][0], ThroughEpisodeID: seasons[0][0], Source: SourceManual},
		{UserID: userID, MediaID: mediaID, Action: EpisodeProgressSeason, SeasonID: "missing", Source: SourceManual},
	}
	for _, input := range invalidInputs {
		_, err := service.UpdateEpisodeProgress(ctx, input)
		require.Error(t, err)
	}

	progress, err := service.UpdateEpisodeProgress(ctx, EpisodeProgressInput{
		UserID: userID, MediaID: mediaID, Action: EpisodeProgressSingle,
		EpisodeID: seasons[0][0], Source: SourceManual,
	})
	require.NoError(t, err)
	_, err = service.repository.ApplyEpisodeProgress(ctx, EpisodeProgressInput{
		UserID: userID, MediaID: mediaID, Source: SourceManual, ExpectedVersion: 0,
	}, []string{seasons[0][1]}, true)
	require.ErrorIs(t, err, ErrVersionConflict)
	_, err = service.UpdateEpisodeProgress(ctx, EpisodeProgressInput{
		UserID: userID, MediaID: mediaID, Action: EpisodeProgressNext,
		Source: SourceManual, ExpectedVersion: 0,
	})
	require.ErrorIs(t, err, ErrVersionConflict)

	unchanged, err := service.UpdateEpisodeProgress(ctx, EpisodeProgressInput{
		UserID: userID, MediaID: mediaID, Action: EpisodeProgressSingle,
		EpisodeID: seasons[0][0], Source: SourceManual, ExpectedVersion: progress.Version,
	})
	require.NoError(t, err)
	require.Equal(t, progress.Version, unchanged.Version)
	require.Len(t, mustWatchEvents(t, service, userID, mediaID), 1)

	unchanged, err = service.UpdateEpisodeProgress(ctx, EpisodeProgressInput{
		UserID: userID, MediaID: mediaID, Action: EpisodeProgressUndo,
		EpisodeID: seasons[0][1], Source: SourceManual, ExpectedVersion: unchanged.Version,
	})
	require.NoError(t, err)
	require.Equal(t, progress.Version, unchanged.Version)
}

func TestEpisodeProgressRespectsHigherPriorityStatusAndExhaustedNext(t *testing.T) {
	service, _, userID, mediaID, seasons := newTestSeriesService(t)
	ctx := context.Background()
	wishlist, err := service.UpdateState(ctx, UpdateStateInput{
		UserID: userID, MediaID: mediaID, Status: StatusWishlist,
		Source: SourceManual, ExpectedVersion: 0,
	})
	require.NoError(t, err)

	progress, err := service.UpdateEpisodeProgress(ctx, EpisodeProgressInput{
		UserID: userID, MediaID: mediaID, Action: EpisodeProgressRange,
		EpisodeID: seasons[0][0], ThroughEpisodeID: seasons[1][2],
		Source: SourceConfirmedSync, ExpectedVersion: wishlist.Version,
	})
	require.NoError(t, err)
	require.Equal(t, StatusWishlist, progress.Status)
	require.Equal(t, 6, progress.WatchedEpisodes)

	_, err = service.UpdateEpisodeProgress(ctx, EpisodeProgressInput{
		UserID: userID, MediaID: mediaID, Action: EpisodeProgressNext,
		Source: SourceManual, ExpectedVersion: progress.Version,
	})
	require.ErrorIs(t, err, ErrEpisodeNotFound)
}

func TestDeletingEpisodeWatchEventReprojectsSeriesProgress(t *testing.T) {
	service, _, userID, mediaID, seasons := newTestSeriesService(t)
	progress, err := service.UpdateEpisodeProgress(context.Background(), EpisodeProgressInput{
		UserID: userID, MediaID: mediaID, Action: EpisodeProgressRange,
		EpisodeID: seasons[0][0], ThroughEpisodeID: seasons[1][2],
		Source: SourceManual,
	})
	require.NoError(t, err)
	require.Equal(t, StatusCompleted, progress.Status)
	events := mustWatchEvents(t, service, userID, mediaID)
	require.Len(t, events, 6)

	require.NoError(t, service.DeleteWatchEvent(context.Background(), userID, events[len(events)-1].ID))
	progress, err = service.EpisodeProgress(context.Background(), userID, mediaID)
	require.NoError(t, err)
	require.Equal(t, 5, progress.WatchedEpisodes)
	require.Equal(t, StatusWatching, progress.Status)
	require.Equal(t, 2, progress.Version)
}

func TestSparseExternalEpisodeProgressStoresOnlySelectedIdentities(t *testing.T) {
	service, db, userID, movieID := newTestRecordsService(t)
	ctx := context.Background()
	require.NoError(t, func() error {
		_, err := db.Writer().ExecContext(ctx, "DELETE FROM media_items WHERE id = ?", movieID)
		return err
	}())
	mediaService := media.NewService(media.NewRepository(db))
	series, err := mediaService.UpsertExternal(ctx, media.ExternalSnapshot{
		Source: "tmdb", SourceID: "1399", MediaType: media.MediaTypeTV, Title: "权力的游戏",
	})
	require.NoError(t, err)
	watchedAt := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	references := []EpisodeReference{
		{SourceID: "63056", SeasonNumber: 1, EpisodeNumber: 1, AbsoluteNumber: 1},
		{SourceID: "63057", SeasonNumber: 1, EpisodeNumber: 2, AbsoluteNumber: 2},
	}

	progress, err := service.UpdateEpisodeProgress(ctx, EpisodeProgressInput{
		UserID: userID, MediaID: series.ID, Action: EpisodeProgressRange,
		EpisodeRefs: references, TotalEpisodes: 12,
		WatchedAt: watchedAt, Source: SourceManual,
	})
	require.NoError(t, err)
	require.Equal(t, StatusWatching, progress.Status)
	require.Equal(t, 2, progress.WatchedEpisodes)
	require.Equal(t, 12, progress.TotalEpisodes)
	require.Len(t, progress.Episodes, 2)
	require.Equal(t, "63056", progress.Episodes[0].SourceID)
	require.Equal(t, 1, progress.Episodes[0].AbsoluteNumber)

	var seasonCount, episodeCount int
	require.NoError(t, db.Reader().QueryRowContext(ctx, "SELECT COUNT(*) FROM seasons WHERE media_id = ?", series.ID).Scan(&seasonCount))
	require.NoError(t, db.Reader().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM episodes episode
		JOIN seasons season ON season.id = episode.season_id
		WHERE season.media_id = ?
	`, series.ID).Scan(&episodeCount))
	require.Equal(t, 1, seasonCount)
	require.Equal(t, 2, episodeCount)
	var name, overview, stillPath, airDate string
	require.NoError(t, db.Reader().QueryRowContext(ctx, `
		SELECT name, overview, still_path, air_date FROM episodes WHERE source_id = '63056'
	`).Scan(&name, &overview, &stillPath, &airDate))
	require.Empty(t, name)
	require.Empty(t, overview)
	require.Empty(t, stillPath)
	require.Empty(t, airDate)
	require.Len(t, mustWatchEvents(t, service, userID, series.ID), 2)

	replayed, err := service.UpdateEpisodeProgress(ctx, EpisodeProgressInput{
		UserID: userID, MediaID: series.ID, Action: EpisodeProgressRange,
		EpisodeRefs: references, TotalEpisodes: 12,
		WatchedAt: watchedAt, Source: SourceManual, ExpectedVersion: progress.Version,
	})
	require.NoError(t, err)
	require.Equal(t, progress.Version, replayed.Version)
	require.Len(t, mustWatchEvents(t, service, userID, series.ID), 2)

	undone, err := service.UpdateEpisodeProgress(ctx, EpisodeProgressInput{
		UserID: userID, MediaID: series.ID, Action: EpisodeProgressUndo,
		EpisodeRefs: []EpisodeReference{references[1]}, TotalEpisodes: 12,
		Source: SourceManual, ExpectedVersion: replayed.Version,
	})
	require.NoError(t, err)
	require.Equal(t, 1, undone.WatchedEpisodes)
	require.Equal(t, StatusWatching, undone.Status)
	require.Len(t, mustWatchEvents(t, service, userID, series.ID), 1)
}

func TestSparseExternalEpisodeProgressRejectsInvalidReferences(t *testing.T) {
	service, _, userID, mediaID, _ := newTestSeriesService(t)
	for _, input := range []EpisodeProgressInput{
		{UserID: userID, MediaID: mediaID, Action: EpisodeProgressSingle, Source: SourceManual, EpisodeRefs: []EpisodeReference{{SeasonNumber: 1, EpisodeNumber: 1, AbsoluteNumber: 1}}, TotalEpisodes: 1},
		{UserID: userID, MediaID: mediaID, Action: EpisodeProgressSingle, Source: SourceManual, EpisodeRefs: []EpisodeReference{{SourceID: "1", EpisodeNumber: 1, AbsoluteNumber: 1}}, TotalEpisodes: 1},
		{UserID: userID, MediaID: mediaID, Action: EpisodeProgressSingle, Source: SourceManual, EpisodeRefs: []EpisodeReference{{SourceID: "1", SeasonNumber: 1, EpisodeNumber: 1, AbsoluteNumber: 2}}, TotalEpisodes: 1},
		{UserID: userID, MediaID: mediaID, Action: EpisodeProgressSingle, Source: SourceManual, EpisodeRefs: []EpisodeReference{{SourceID: "1", SeasonNumber: 1, EpisodeNumber: 1, AbsoluteNumber: 1}, {SourceID: "1", SeasonNumber: 1, EpisodeNumber: 1, AbsoluteNumber: 1}}, TotalEpisodes: 1},
	} {
		_, err := service.UpdateEpisodeProgress(context.Background(), input)
		require.ErrorIs(t, err, ErrInvalidEpisodeProgress)
	}
}

func newTestSeriesService(t *testing.T) (*Service, *storage.DB, string, string, [][]string) {
	t.Helper()
	service, db, userID, movieID := newTestRecordsService(t)
	_, err := db.Writer().ExecContext(context.Background(), "DELETE FROM media_items WHERE id = ?", movieID)
	require.NoError(t, err)
	mediaService := media.NewService(media.NewRepository(db))
	series, err := mediaService.CreateCustom(context.Background(), media.CreateCustomInput{
		MediaType: media.MediaTypeTV, Title: "测试剧集",
	})
	require.NoError(t, err)

	seasonEpisodes := make([][]string, 2)
	for seasonNumber := 1; seasonNumber <= 2; seasonNumber++ {
		seasonID := uuid.NewString()
		_, err = db.Writer().ExecContext(context.Background(), `
			INSERT INTO seasons (id, media_id, season_number, name, overview, poster_path, air_date)
			VALUES (?, ?, ?, ?, '', '', '')
		`, seasonID, series.ID, seasonNumber, "第 "+string(rune('0'+seasonNumber))+" 季")
		require.NoError(t, err)
		for episodeNumber := 1; episodeNumber <= 3; episodeNumber++ {
			episodeID := uuid.NewString()
			_, err = db.Writer().ExecContext(context.Background(), `
				INSERT INTO episodes (id, season_id, episode_number, name, overview, still_path, air_date)
				VALUES (?, ?, ?, ?, '', '', '')
			`, episodeID, seasonID, episodeNumber, "测试单集")
			require.NoError(t, err)
			seasonEpisodes[seasonNumber-1] = append(seasonEpisodes[seasonNumber-1], episodeID)
		}
	}
	return service, db, userID, series.ID, seasonEpisodes
}

func seasonIDForEpisode(t *testing.T, service *Service, userID, mediaID, episodeID string) string {
	t.Helper()
	progress, err := service.repository.Episodes(context.Background(), userID, mediaID)
	require.NoError(t, err)
	for _, episode := range progress.Episodes {
		if episode.ID == episodeID {
			return episode.SeasonID
		}
	}
	t.Fatalf("episode %s not found", episodeID)
	return ""
}
