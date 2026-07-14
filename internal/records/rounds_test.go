package records

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"video-record/internal/media"
)

func TestCurrentRoundReturnsEmptyAndCreatesFirstMovieRound(t *testing.T) {
	_, db, userID, movieID := newTestRecordsService(t)
	now := time.Date(2026, 7, 14, 12, 30, 45, 0, time.UTC)
	service := NewService(NewRepository(db), ServiceOptions{Now: func() time.Time { return now }})
	scope := RoundScope{UserID: userID, MediaID: movieID}

	current, err := service.CurrentRound(context.Background(), scope)
	require.NoError(t, err)
	require.Empty(t, current.ID)
	require.Equal(t, 1, current.RoundNumber)
	require.Equal(t, StatusNone, current.Status)
	require.Zero(t, current.Version)

	rating := 87
	note := "第一轮"
	method := "家庭电视"
	saved, err := service.UpdateRound(context.Background(), UpdateRoundInput{
		Scope: scope, Status: StatusWatching,
		Rating: &rating, RatingSet: true, Note: &note, NoteSet: true,
		ViewingMethod: &method, ViewingMethodSet: true,
		Source: SourceManual, ExpectedVersion: 0,
	})
	require.NoError(t, err)
	require.NotEmpty(t, saved.ID)
	require.Equal(t, 1, saved.RoundNumber)
	require.Equal(t, 1, saved.Version)
	require.Equal(t, StatusWatching, saved.Status)
	require.Equal(t, rating, *saved.Rating)
	require.Equal(t, note, *saved.Note)
	require.Equal(t, method, *saved.ViewingMethod)

	reloaded, err := service.CurrentRound(context.Background(), scope)
	require.NoError(t, err)
	require.Equal(t, saved, reloaded)
}

func TestCurrentRoundIsolatesSeasonsAndUsers(t *testing.T) {
	_, db, userID, movieID := newTestRecordsService(t)
	_, err := db.Writer().ExecContext(context.Background(), "DELETE FROM media_items WHERE id = ?", movieID)
	require.NoError(t, err)
	mediaService := media.NewService(media.NewRepository(db))
	series, err := mediaService.CreateCustom(context.Background(), media.CreateCustomInput{
		MediaType: media.MediaTypeTV, Title: "测试剧集",
	})
	require.NoError(t, err)
	otherUserID := insertTestUser(t, db, "round-viewer")
	service := NewService(NewRepository(db))
	seasonOne, seasonTwo := 1, 2

	one, err := service.UpdateRound(context.Background(), UpdateRoundInput{
		Scope:  RoundScope{UserID: userID, MediaID: series.ID, SeasonNumber: &seasonOne},
		Status: StatusCompleted, Source: SourceManual, ExpectedVersion: 0,
		CompletedAt: timePointer(time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)),
	})
	require.NoError(t, err)
	two, err := service.UpdateRound(context.Background(), UpdateRoundInput{
		Scope:  RoundScope{UserID: userID, MediaID: series.ID, SeasonNumber: &seasonTwo},
		Status: StatusWatching, Source: SourceManual, ExpectedVersion: 0,
	})
	require.NoError(t, err)
	require.NotEqual(t, one.ID, two.ID)
	require.Equal(t, StatusCompleted, one.Status)
	require.Equal(t, StatusWatching, two.Status)

	other, err := service.CurrentRound(context.Background(), RoundScope{
		UserID: otherUserID, MediaID: series.ID, SeasonNumber: &seasonOne,
	})
	require.NoError(t, err)
	require.Zero(t, other.Version)
	require.Equal(t, StatusNone, other.Status)
}

func TestRoundScopeArchiveAndFutureTimeValidation(t *testing.T) {
	_, db, userID, movieID := newTestRecordsService(t)
	mediaService := media.NewService(media.NewRepository(db))
	series, err := mediaService.CreateCustom(context.Background(), media.CreateCustomInput{
		MediaType: media.MediaTypeTV, Title: "测试剧集",
	})
	require.NoError(t, err)
	now := time.Date(2026, 7, 14, 12, 30, 45, 0, time.UTC)
	service := NewService(NewRepository(db), ServiceOptions{Now: func() time.Time { return now }})
	seasonOne := 1

	_, err = service.CurrentRound(context.Background(), RoundScope{
		UserID: userID, MediaID: movieID, SeasonNumber: &seasonOne,
	})
	require.ErrorIs(t, err, ErrInvalidRoundScope)
	_, err = service.CurrentRound(context.Background(), RoundScope{UserID: userID, MediaID: series.ID})
	require.ErrorIs(t, err, ErrInvalidRoundScope)

	_, err = service.UpdateRound(context.Background(), UpdateRoundInput{
		Scope:  RoundScope{UserID: userID, MediaID: movieID},
		Status: StatusCompleted, Source: SourceManual,
		CompletedAt: timePointer(now.Add(time.Second)),
	})
	require.ErrorIs(t, err, ErrInvalidWatchedAt)

	saved, err := service.UpdateRound(context.Background(), UpdateRoundInput{
		Scope:  RoundScope{UserID: userID, MediaID: movieID},
		Status: StatusWatching, Source: SourceManual,
	})
	require.NoError(t, err)
	_, err = db.Writer().ExecContext(context.Background(), `
		UPDATE watch_rounds SET archived_at = ? WHERE id = ?
	`, formatEventTime(now), saved.ID)
	require.NoError(t, err)
	_, err = service.UpdateRound(context.Background(), UpdateRoundInput{
		Scope:  RoundScope{UserID: userID, MediaID: movieID},
		Status: StatusCompleted, Source: SourceManual, ExpectedVersion: saved.Version,
		CompletedAt: &now,
	})
	require.ErrorIs(t, err, ErrRoundArchived)
}

func TestRewatchRoundArchivesCompletedMovieAndStartsBlankRound(t *testing.T) {
	service, db, userID, mediaID := newTestRecordsService(t)
	ctx := context.Background()
	completedAt := time.Date(2026, 7, 13, 21, 2, 3, 0, time.UTC)
	rating := 91
	note := "第一轮笔记"
	method := "家庭投影"
	completed, err := service.UpdateRound(ctx, UpdateRoundInput{
		Scope:  RoundScope{UserID: userID, MediaID: mediaID},
		Status: StatusCompleted, Rating: &rating, RatingSet: true,
		Note: &note, NoteSet: true, ViewingMethod: &method, ViewingMethodSet: true,
		CompletedAt: &completedAt, Source: SourceManual,
	})
	require.NoError(t, err)

	result, err := service.StartRewatch(ctx, RewatchInput{
		Scope:           RoundScope{UserID: userID, MediaID: mediaID},
		ExpectedVersion: completed.Version,
	})
	require.NoError(t, err)
	require.Equal(t, 1, result.Archived.RoundNumber)
	require.Equal(t, completedAt, *result.Archived.CompletedAt)
	require.NotNil(t, result.Archived.ArchivedAt)
	require.Equal(t, rating, *result.Archived.Rating)
	require.Equal(t, note, *result.Archived.Note)
	require.Equal(t, 2, result.Current.RoundNumber)
	require.Equal(t, StatusWatching, result.Current.Status)
	require.Nil(t, result.Current.Rating)
	require.Nil(t, result.Current.Note)
	require.Nil(t, result.Current.ViewingMethod)
	require.Nil(t, result.Current.CompletedAt)

	history, err := service.RoundHistory(ctx, RoundScope{UserID: userID, MediaID: mediaID})
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.Equal(t, result.Archived.ID, history[0].ID)
	detail, err := service.RoundDetail(ctx, RoundScope{UserID: userID, MediaID: mediaID}, result.Archived.ID)
	require.NoError(t, err)
	require.Equal(t, note, *detail.Round.Note)
	require.Empty(t, detail.Episodes)

	otherUserID := insertTestUser(t, db, "round-history-outsider")
	otherHistory, err := service.RoundHistory(ctx, RoundScope{UserID: otherUserID, MediaID: mediaID})
	require.NoError(t, err)
	require.Empty(t, otherHistory)
	_, err = service.RoundDetail(ctx, RoundScope{UserID: otherUserID, MediaID: mediaID}, result.Archived.ID)
	require.ErrorIs(t, err, ErrRoundNotFound)
}

func TestRewatchRoundRequiresCompletedCurrentRound(t *testing.T) {
	service, _, userID, mediaID := newTestRecordsService(t)
	ctx := context.Background()
	watching, err := service.UpdateRound(ctx, UpdateRoundInput{
		Scope:  RoundScope{UserID: userID, MediaID: mediaID},
		Status: StatusWatching, Source: SourceManual,
	})
	require.NoError(t, err)

	_, err = service.StartRewatch(ctx, RewatchInput{
		Scope:           RoundScope{UserID: userID, MediaID: mediaID},
		ExpectedVersion: watching.Version,
	})
	require.ErrorIs(t, err, ErrRoundNotCompleted)
	history, historyErr := service.RoundHistory(ctx, RoundScope{UserID: userID, MediaID: mediaID})
	require.NoError(t, historyErr)
	require.Empty(t, history)
}

func TestRewatchRoundArchivesSeasonEpisodesWithoutChangingOtherSeason(t *testing.T) {
	service, _, userID, mediaID, seasons := newTestSeriesService(t)
	ctx := context.Background()
	watchedAt := time.Date(2026, 7, 13, 20, 1, 2, 0, time.UTC)
	seasonOne, err := service.UpdateEpisodeProgress(ctx, EpisodeProgressInput{
		UserID: userID, MediaID: mediaID, SeasonNumber: 1,
		Action: EpisodeProgressSingle, EpisodeID: seasons[0][0],
		WatchedAt: watchedAt, Source: SourceManual,
	})
	require.NoError(t, err)
	seasonTwo, err := service.UpdateEpisodeProgress(ctx, EpisodeProgressInput{
		UserID: userID, MediaID: mediaID, SeasonNumber: 2,
		Action: EpisodeProgressSeason, WatchedAt: watchedAt.Add(time.Hour),
		Source: SourceManual, TotalEpisodes: 3,
	})
	require.NoError(t, err)
	require.Equal(t, StatusCompleted, seasonTwo.Status)

	result, err := service.StartRewatch(ctx, RewatchInput{
		Scope:           RoundScope{UserID: userID, MediaID: mediaID, SeasonNumber: integerPointer(2)},
		ExpectedVersion: seasonTwo.Version,
	})
	require.NoError(t, err)
	require.Equal(t, 2, *result.Archived.SeasonNumber)
	detail, err := service.RoundDetail(
		ctx,
		RoundScope{UserID: userID, MediaID: mediaID, SeasonNumber: integerPointer(2)},
		result.Archived.ID,
	)
	require.NoError(t, err)
	require.Len(t, detail.Episodes, 3)
	for _, episode := range detail.Episodes {
		require.True(t, episode.Watched)
		require.Equal(t, watchedAt.Add(time.Hour), *episode.WatchedAt)
	}
	currentSeasonTwo, err := service.EpisodeProgress(ctx, userID, mediaID, 2)
	require.NoError(t, err)
	require.Equal(t, result.Current.ID, currentSeasonTwo.RoundID)
	require.Zero(t, currentSeasonTwo.WatchedEpisodes)
	currentSeasonOne, err := service.EpisodeProgress(ctx, userID, mediaID, 1)
	require.NoError(t, err)
	require.Equal(t, seasonOne.RoundID, currentSeasonOne.RoundID)
	require.Equal(t, 1, currentSeasonOne.WatchedEpisodes)
}

func TestRewatchRoundRollsBackArchiveWhenNextRoundInsertFails(t *testing.T) {
	service, db, userID, mediaID := newTestRecordsService(t)
	ctx := context.Background()
	completedAt := time.Date(2026, 7, 13, 21, 2, 3, 0, time.UTC)
	completed, err := service.UpdateRound(ctx, UpdateRoundInput{
		Scope:  RoundScope{UserID: userID, MediaID: mediaID},
		Status: StatusCompleted, CompletedAt: &completedAt, Source: SourceManual,
	})
	require.NoError(t, err)
	_, err = db.Writer().ExecContext(ctx, `
		CREATE TRIGGER fail_second_round
		BEFORE INSERT ON watch_rounds
		WHEN NEW.round_number = 2
		BEGIN SELECT RAISE(ABORT, 'injected rewatch failure'); END
	`)
	require.NoError(t, err)

	_, err = service.StartRewatch(ctx, RewatchInput{
		Scope:           RoundScope{UserID: userID, MediaID: mediaID},
		ExpectedVersion: completed.Version,
	})
	require.Error(t, err)
	current, currentErr := service.CurrentRound(ctx, RoundScope{UserID: userID, MediaID: mediaID})
	require.NoError(t, currentErr)
	require.Equal(t, completed.ID, current.ID)
	require.Equal(t, StatusCompleted, current.Status)
	require.Nil(t, current.ArchivedAt)
	var count int
	require.NoError(t, db.Reader().QueryRowContext(ctx, "SELECT COUNT(*) FROM watch_rounds").Scan(&count))
	require.Equal(t, 1, count)
}

func integerPointer(value int) *int {
	return &value
}

func timePointer(value time.Time) *time.Time {
	return &value
}
