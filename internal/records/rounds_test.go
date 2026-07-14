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

func timePointer(value time.Time) *time.Time {
	return &value
}
