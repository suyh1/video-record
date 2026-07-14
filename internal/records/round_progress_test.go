package records

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSeasonRoundProgressMarksEditsAndUndoesEpisodeTime(t *testing.T) {
	service, db, userID, mediaID, seasons := newTestSeriesService(t)
	ctx := context.Background()
	firstTime := time.Date(2026, 7, 13, 12, 0, 1, 0, time.UTC)

	progress, err := service.UpdateEpisodeProgress(ctx, EpisodeProgressInput{
		UserID: userID, MediaID: mediaID, SeasonNumber: 1,
		Action: EpisodeProgressSingle, EpisodeID: seasons[0][0],
		WatchedAt: firstTime, Source: SourceManual, TotalEpisodes: 3,
	})
	require.NoError(t, err)
	require.Equal(t, 1, progress.SeasonNumber)
	require.NotEmpty(t, progress.RoundID)
	require.Equal(t, StatusWatching, progress.Status)
	require.Equal(t, 1, progress.WatchedEpisodes)
	require.Equal(t, firstTime, *progress.Episodes[0].WatchedAt)

	editedTime := firstTime.Add(2*time.Hour + 3*time.Second)
	progress, err = service.UpdateEpisodeProgress(ctx, EpisodeProgressInput{
		UserID: userID, MediaID: mediaID, SeasonNumber: 1,
		Action: EpisodeProgressSetTime, EpisodeID: seasons[0][0],
		WatchedAt: editedTime, Source: SourceManual, TotalEpisodes: 3,
		ExpectedVersion: progress.Version,
	})
	require.NoError(t, err)
	require.Equal(t, editedTime, *progress.Episodes[0].WatchedAt)
	var progressTime, eventTime string
	require.NoError(t, db.Reader().QueryRowContext(ctx, `
		SELECT progress.watched_at, event.watched_at
		FROM round_episode_progress progress
		JOIN watch_events event ON event.id = progress.watch_event_id
		WHERE progress.round_id = ? AND progress.episode_id = ?
	`, progress.RoundID, seasons[0][0]).Scan(&progressTime, &eventTime))
	require.Equal(t, formatEventTime(editedTime), progressTime)
	require.Equal(t, progressTime, eventTime)

	progress, err = service.UpdateEpisodeProgress(ctx, EpisodeProgressInput{
		UserID: userID, MediaID: mediaID, SeasonNumber: 1,
		Action: EpisodeProgressUndo, EpisodeID: seasons[0][0],
		Source: SourceManual, TotalEpisodes: 3, ExpectedVersion: progress.Version,
	})
	require.NoError(t, err)
	require.Equal(t, StatusNone, progress.Status)
	require.Zero(t, progress.WatchedEpisodes)
	require.Zero(t, tableCountForRecords(t, db, "round_episode_progress"))
	require.Zero(t, tableCountForRecords(t, db, "watch_events"))
}

func TestSeasonRoundProgressSetTimeMarksUnwatchedAndRejectsFuture(t *testing.T) {
	_, db, userID, mediaID, seasons := newTestSeriesService(t)
	now := time.Date(2026, 7, 14, 12, 30, 45, 0, time.UTC)
	service := NewService(NewRepository(db), ServiceOptions{Now: func() time.Time { return now }})
	ctx := context.Background()

	progress, err := service.UpdateEpisodeProgress(ctx, EpisodeProgressInput{
		UserID: userID, MediaID: mediaID, SeasonNumber: 1,
		Action: EpisodeProgressSetTime, EpisodeID: seasons[0][1],
		WatchedAt: now.Add(-time.Second), Source: SourceManual, TotalEpisodes: 3,
	})
	require.NoError(t, err)
	require.True(t, progress.Episodes[1].Watched)

	_, err = service.UpdateEpisodeProgress(ctx, EpisodeProgressInput{
		UserID: userID, MediaID: mediaID, SeasonNumber: 1,
		Action: EpisodeProgressSetTime, EpisodeID: seasons[0][2],
		WatchedAt: now.Add(time.Second), Source: SourceManual, TotalEpisodes: 3,
		ExpectedVersion: progress.Version,
	})
	require.ErrorIs(t, err, ErrInvalidWatchedAt)
	require.Equal(t, 1, tableCountForRecords(t, db, "round_episode_progress"))
	require.Equal(t, 1, tableCountForRecords(t, db, "watch_events"))
}

func TestSeasonRoundProgressCompletesOnlySelectedSeason(t *testing.T) {
	service, _, userID, mediaID, seasons := newTestSeriesService(t)
	ctx := context.Background()
	watchedAt := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)

	seasonOne, err := service.UpdateEpisodeProgress(ctx, EpisodeProgressInput{
		UserID: userID, MediaID: mediaID, SeasonNumber: 1,
		Action: EpisodeProgressRange, EpisodeID: seasons[0][0], ThroughEpisodeID: seasons[0][2],
		WatchedAt: watchedAt, Source: SourceManual, TotalEpisodes: 3,
	})
	require.NoError(t, err)
	require.Equal(t, StatusCompleted, seasonOne.Status)
	require.Equal(t, 3, seasonOne.WatchedEpisodes)

	seasonTwo, err := service.EpisodeProgress(ctx, userID, mediaID, 2)
	require.NoError(t, err)
	require.Equal(t, StatusNone, seasonTwo.Status)
	require.Zero(t, seasonTwo.WatchedEpisodes)
	require.Len(t, seasonTwo.Episodes, 3)
}

func tableCountForRecords(t *testing.T, db interface {
	Reader() *sql.DB
}, table string) int {
	t.Helper()
	var count int
	require.NoError(t, db.Reader().QueryRowContext(
		context.Background(), "SELECT COUNT(*) FROM "+table,
	).Scan(&count))
	return count
}
