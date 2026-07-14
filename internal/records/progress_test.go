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

func TestEpisodeProgressSupportsSelectedSeasonActions(t *testing.T) {
	service, _, userID, mediaID, seasons := newTestSeriesService(t)
	ctx := context.Background()
	watchedAt := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)

	initial, err := service.EpisodeProgress(ctx, userID, mediaID, 1)
	require.NoError(t, err)
	require.Zero(t, initial.Version)
	require.Empty(t, initial.RoundID)
	require.Equal(t, 1, initial.SeasonNumber)
	require.Len(t, initial.Episodes, 3)

	progress, err := service.UpdateEpisodeProgress(ctx, EpisodeProgressInput{
		UserID: userID, MediaID: mediaID, SeasonNumber: 1,
		Action: EpisodeProgressSingle, EpisodeID: seasons[0][0],
		WatchedAt: watchedAt, Source: SourceManual,
	})
	require.NoError(t, err)
	require.Equal(t, 1, progress.WatchedEpisodes)
	require.Equal(t, StatusWatching, progress.Status)
	require.Equal(t, 1, progress.Version)
	require.Equal(t, 1, progress.Episodes[0].AbsoluteNumber)
	require.True(t, progress.Episodes[0].Watched)

	progress, err = service.UpdateEpisodeProgress(ctx, EpisodeProgressInput{
		UserID: userID, MediaID: mediaID, SeasonNumber: 1,
		Action: EpisodeProgressRange, EpisodeID: seasons[0][1], ThroughEpisodeID: seasons[0][2],
		WatchedAt: watchedAt.Add(time.Hour), Source: SourceManual, ExpectedVersion: progress.Version,
	})
	require.NoError(t, err)
	require.Equal(t, 3, progress.WatchedEpisodes)
	require.Equal(t, StatusCompleted, progress.Status)
	require.Equal(t, watchedAt.Add(time.Hour), *progress.LastWatched.WatchedAt)

	secondSeason, err := service.EpisodeProgress(ctx, userID, mediaID, 2)
	require.NoError(t, err)
	require.Equal(t, StatusNone, secondSeason.Status)
	require.Zero(t, secondSeason.WatchedEpisodes)
	require.Empty(t, secondSeason.RoundID)

	secondSeason, err = service.UpdateEpisodeProgress(ctx, EpisodeProgressInput{
		UserID: userID, MediaID: mediaID, SeasonNumber: 2,
		Action: EpisodeProgressNext, WatchedAt: watchedAt.Add(2 * time.Hour), Source: SourceManual,
	})
	require.NoError(t, err)
	require.Equal(t, seasons[1][0], secondSeason.LastWatched.ID)
	require.Equal(t, seasons[1][1], secondSeason.NextEpisode.ID)
	require.NotEqual(t, progress.RoundID, secondSeason.RoundID)
	require.Len(t, mustWatchEvents(t, service, userID, mediaID), 4)
}

func TestEpisodeProgressValidatesSeasonSelectorsVersionsAndNoops(t *testing.T) {
	service, _, userID, mediaID, seasons := newTestSeriesService(t)
	ctx := context.Background()

	_, err := service.EpisodeProgress(ctx, userID, mediaID)
	require.ErrorIs(t, err, ErrInvalidEpisodeProgress)
	_, err = service.EpisodeProgress(ctx, userID, mediaID, 0)
	require.ErrorIs(t, err, ErrInvalidEpisodeProgress)
	_, err = service.UpdateEpisodeProgress(ctx, EpisodeProgressInput{
		UserID: userID, MediaID: mediaID, SeasonNumber: 1,
		Action: EpisodeProgressSingle, EpisodeID: seasons[1][0], Source: SourceManual,
	})
	require.ErrorIs(t, err, ErrEpisodeNotFound)
	_, err = service.UpdateEpisodeProgress(ctx, EpisodeProgressInput{
		UserID: userID, MediaID: mediaID, SeasonNumber: 1,
		Action: "unknown", Source: SourceManual,
	})
	require.ErrorIs(t, err, ErrInvalidEpisodeProgress)

	progress, err := service.UpdateEpisodeProgress(ctx, EpisodeProgressInput{
		UserID: userID, MediaID: mediaID, SeasonNumber: 1,
		Action: EpisodeProgressSingle, EpisodeID: seasons[0][0], Source: SourceManual,
	})
	require.NoError(t, err)
	_, err = service.UpdateEpisodeProgress(ctx, EpisodeProgressInput{
		UserID: userID, MediaID: mediaID, SeasonNumber: 1,
		Action: EpisodeProgressNext, Source: SourceManual, ExpectedVersion: 0,
	})
	require.ErrorIs(t, err, ErrVersionConflict)

	unchanged, err := service.UpdateEpisodeProgress(ctx, EpisodeProgressInput{
		UserID: userID, MediaID: mediaID, SeasonNumber: 1,
		Action: EpisodeProgressSingle, EpisodeID: seasons[0][0],
		Source: SourceManual, ExpectedVersion: progress.Version,
	})
	require.NoError(t, err)
	require.Equal(t, progress.Version, unchanged.Version)
	require.Len(t, mustWatchEvents(t, service, userID, mediaID), 1)

	unchanged, err = service.UpdateEpisodeProgress(ctx, EpisodeProgressInput{
		UserID: userID, MediaID: mediaID, SeasonNumber: 1,
		Action: EpisodeProgressUndo, EpisodeID: seasons[0][1],
		Source: SourceManual, ExpectedVersion: unchanged.Version,
	})
	require.NoError(t, err)
	require.Equal(t, progress.Version, unchanged.Version)
}

func TestSparseExternalEpisodeProgressStoresOnlySelectedIdentities(t *testing.T) {
	service, db, userID, movieID := newTestRecordsService(t)
	ctx := context.Background()
	_, err := db.Writer().ExecContext(ctx, "DELETE FROM media_items WHERE id = ?", movieID)
	require.NoError(t, err)
	mediaService := media.NewService(media.NewRepository(db))
	series, err := mediaService.UpsertExternal(ctx, media.ExternalSnapshot{
		Source: "tmdb", SourceID: "1399", MediaType: media.MediaTypeTV, Title: "权力的游戏",
	})
	require.NoError(t, err)
	watchedAt := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	references := []EpisodeReference{
		{SourceID: "63056", SeasonNumber: 1, EpisodeNumber: 1, AbsoluteNumber: 1},
		{SourceID: "63057", SeasonNumber: 1, EpisodeNumber: 2, AbsoluteNumber: 2},
	}

	progress, err := service.UpdateEpisodeProgress(ctx, EpisodeProgressInput{
		UserID: userID, MediaID: series.ID, SeasonNumber: 1,
		Action: EpisodeProgressRange, EpisodeRefs: references, TotalEpisodes: 12,
		WatchedAt: watchedAt, Source: SourceManual,
	})
	require.NoError(t, err)
	require.Equal(t, StatusWatching, progress.Status)
	require.Equal(t, 2, progress.WatchedEpisodes)
	require.Equal(t, 12, progress.TotalEpisodes)
	require.Len(t, progress.Episodes, 2)
	require.Equal(t, "63056", progress.Episodes[0].SourceID)

	var seasonCount, episodeCount int
	require.NoError(t, db.Reader().QueryRowContext(
		ctx, "SELECT COUNT(*) FROM seasons WHERE media_id = ?", series.ID,
	).Scan(&seasonCount))
	require.NoError(t, db.Reader().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM episodes episode
		JOIN seasons season ON season.id = episode.season_id
		WHERE season.media_id = ?
	`, series.ID).Scan(&episodeCount))
	require.Equal(t, 1, seasonCount)
	require.Equal(t, 2, episodeCount)
	events := mustWatchEvents(t, service, userID, series.ID)
	require.Len(t, events, 2)
	require.Equal(t, progress.RoundID, events[0].RoundID)

	replayed, err := service.UpdateEpisodeProgress(ctx, EpisodeProgressInput{
		UserID: userID, MediaID: series.ID, SeasonNumber: 1,
		Action: EpisodeProgressRange, EpisodeRefs: references, TotalEpisodes: 12,
		WatchedAt: watchedAt, Source: SourceManual, ExpectedVersion: progress.Version,
	})
	require.NoError(t, err)
	require.Equal(t, progress.Version, replayed.Version)

	undone, err := service.UpdateEpisodeProgress(ctx, EpisodeProgressInput{
		UserID: userID, MediaID: series.ID, SeasonNumber: 1,
		Action: EpisodeProgressUndo, EpisodeRefs: []EpisodeReference{references[1]},
		TotalEpisodes: 12, Source: SourceManual, ExpectedVersion: replayed.Version,
	})
	require.NoError(t, err)
	require.Equal(t, 1, undone.WatchedEpisodes)
	require.Len(t, mustWatchEvents(t, service, userID, series.ID), 1)
}

func TestSparseExternalEpisodeProgressRejectsInvalidReferences(t *testing.T) {
	service, _, userID, mediaID, _ := newTestSeriesService(t)
	for _, input := range []EpisodeProgressInput{
		{UserID: userID, MediaID: mediaID, SeasonNumber: 1, Action: EpisodeProgressSingle, Source: SourceManual, EpisodeRefs: []EpisodeReference{{SeasonNumber: 1, EpisodeNumber: 1, AbsoluteNumber: 1}}, TotalEpisodes: 1},
		{UserID: userID, MediaID: mediaID, SeasonNumber: 1, Action: EpisodeProgressSingle, Source: SourceManual, EpisodeRefs: []EpisodeReference{{SourceID: "1", SeasonNumber: 2, EpisodeNumber: 1, AbsoluteNumber: 1}}, TotalEpisodes: 1},
		{UserID: userID, MediaID: mediaID, SeasonNumber: 1, Action: EpisodeProgressSingle, Source: SourceManual, EpisodeRefs: []EpisodeReference{{SourceID: "1", SeasonNumber: 1, EpisodeNumber: 1, AbsoluteNumber: 2}}, TotalEpisodes: 1},
		{UserID: userID, MediaID: mediaID, SeasonNumber: 1, Action: EpisodeProgressSingle, Source: SourceManual, EpisodeRefs: []EpisodeReference{{SourceID: "1", SeasonNumber: 1, EpisodeNumber: 1, AbsoluteNumber: 1}, {SourceID: "1", SeasonNumber: 1, EpisodeNumber: 1, AbsoluteNumber: 1}}, TotalEpisodes: 1},
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
