package records

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCalendarHonorsTimezoneBoundariesRepeatedWatchesAndParticipants(t *testing.T) {
	service, db, userID, mediaID := newTestRecordsService(t)
	ctx := context.Background()
	firstTime := time.Date(2026, 6, 30, 16, 0, 0, 0, time.UTC)
	guestID := insertTestUser(t, db, "calendar-guest")
	firstRound, err := service.UpdateRound(ctx, UpdateRoundInput{
		Scope:  RoundScope{UserID: userID, MediaID: mediaID},
		Status: StatusCompleted, CompletedAt: &firstTime, Source: SourceManual,
		ParticipantIDs: []string{guestID},
	})
	require.NoError(t, err)
	events := mustWatchEvents(t, service, userID, mediaID)
	require.Len(t, events, 1)
	first := events[0]
	rewatch, err := service.StartRewatch(ctx, RewatchInput{
		Scope: RoundScope{UserID: userID, MediaID: mediaID}, ExpectedVersion: firstRound.Version,
	})
	require.NoError(t, err)
	secondTime := firstTime.Add(2 * time.Hour)
	_, err = service.UpdateRound(ctx, UpdateRoundInput{
		Scope:  RoundScope{UserID: userID, MediaID: mediaID},
		Status: StatusCompleted, CompletedAt: &secondTime, Source: SourceManual,
		ExpectedVersion: rewatch.Current.Version,
	})
	require.NoError(t, err)
	events = mustWatchEvents(t, service, userID, mediaID)
	require.Len(t, events, 2)
	second := events[1]

	calendar, err := service.CalendarMonth(ctx, CalendarMonthInput{
		UserID: userID, Year: 2026, Month: time.July,
		Timezone: "Asia/Shanghai", Filter: CalendarFilterAll,
	})
	require.NoError(t, err)
	require.Len(t, calendar.Events, 2)
	require.Equal(t, "2026-07-01", calendar.Events[0].LocalDate)
	require.Equal(t, first.ID, calendar.Events[0].ID)
	require.Equal(t, second.ID, calendar.Events[1].ID)
	require.Equal(t, []string{"calendar-guest", "owner"}, calendar.Events[0].Participants)
	require.Equal(t, StatusCompleted, calendar.Events[0].Status)

	guestCalendar, err := service.CalendarMonth(ctx, CalendarMonthInput{
		UserID: guestID, Year: 2026, Month: time.July,
		Timezone: "Asia/Shanghai", Filter: CalendarFilterAll,
	})
	require.NoError(t, err)
	require.Len(t, guestCalendar.Events, 1)
	require.Equal(t, first.ID, guestCalendar.Events[0].ID)
}

func TestCalendarFiltersCompletedAndInProgressAndValidatesInput(t *testing.T) {
	service, _, userID, mediaID := newTestRecordsService(t)
	ctx := context.Background()
	completedAt := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	completed, err := service.UpdateRound(ctx, UpdateRoundInput{
		Scope:  RoundScope{UserID: userID, MediaID: mediaID},
		Status: StatusCompleted, CompletedAt: &completedAt, Source: SourceManual,
	})
	require.NoError(t, err)
	watching, err := service.StartRewatch(ctx, RewatchInput{
		Scope: RoundScope{UserID: userID, MediaID: mediaID}, ExpectedVersion: completed.Version,
	})
	require.NoError(t, err)
	require.Equal(t, StatusWatching, watching.Current.Status)

	inProgress, err := service.CalendarMonth(ctx, CalendarMonthInput{
		UserID: userID, Year: 2026, Month: time.July,
		Timezone: "UTC", Filter: CalendarFilterInProgress,
	})
	require.NoError(t, err)
	require.Empty(t, inProgress.Events)

	completedOnly, err := service.CalendarMonth(ctx, CalendarMonthInput{
		UserID: userID, Year: 2026, Month: time.July,
		Timezone: "UTC", Filter: CalendarFilterCompleted,
	})
	require.NoError(t, err)
	require.Len(t, completedOnly.Events, 1)

	invalid := []CalendarMonthInput{
		{Year: 2026, Month: time.July, Timezone: "UTC", Filter: CalendarFilterAll},
		{UserID: userID, Year: 0, Month: time.July, Timezone: "UTC", Filter: CalendarFilterAll},
		{UserID: userID, Year: 2026, Month: 13, Timezone: "UTC", Filter: CalendarFilterAll},
		{UserID: userID, Year: 2026, Month: time.July, Timezone: "Mars/Olympus", Filter: CalendarFilterAll},
		{UserID: userID, Year: 2026, Month: time.July, Timezone: "UTC", Filter: "unknown"},
	}
	for _, input := range invalid {
		_, err := service.CalendarMonth(ctx, input)
		require.ErrorIs(t, err, ErrInvalidCalendarQuery)
	}
}

func TestCalendarProjectsEpisodeNumbersAndViewingMethod(t *testing.T) {
	service, db, userID, mediaID, seasons := newTestSeriesService(t)
	progress, err := service.UpdateEpisodeProgress(context.Background(), EpisodeProgressInput{
		UserID: userID, MediaID: mediaID, SeasonNumber: 2, Action: EpisodeProgressSingle,
		EpisodeID: seasons[1][1], WatchedAt: time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC),
		Source: SourceManual,
	})
	require.NoError(t, err)
	events := mustWatchEvents(t, service, userID, mediaID)
	require.Len(t, events, 1)
	_, err = db.Writer().ExecContext(context.Background(), `
		UPDATE watch_events SET viewing_method = '家庭电视' WHERE id = ?
	`, events[0].ID)
	require.NoError(t, err)

	calendar, err := service.CalendarMonth(context.Background(), CalendarMonthInput{
		UserID: userID, Year: 2026, Month: time.July,
		Timezone: "UTC", Filter: CalendarFilterInProgress,
	})
	require.NoError(t, err)
	require.Len(t, calendar.Events, 1)
	event := calendar.Events[0]
	require.Equal(t, seasons[1][1], *event.EpisodeID)
	require.Equal(t, 2, *event.SeasonNumber)
	require.Equal(t, 2, *event.EpisodeNumber)
	require.Equal(t, 5, *event.AbsoluteNumber)
	require.Equal(t, "家庭电视", *event.ViewingMethod)
	require.Equal(t, progress.Status, event.Status)
}

func TestCalendarProjectionIncludesArchivedAndCurrentRoundEvents(t *testing.T) {
	service, db, userID, mediaID := newTestRecordsService(t)
	ctx := context.Background()
	guestID := insertTestUser(t, db, "calendar-round-guest")
	firstTime := time.Date(2026, 7, 10, 12, 1, 2, 0, time.UTC)
	first, err := service.UpdateRound(ctx, UpdateRoundInput{
		Scope:  RoundScope{UserID: userID, MediaID: mediaID},
		Status: StatusCompleted, CompletedAt: &firstTime, Source: SourceManual,
		ParticipantIDs: []string{guestID},
	})
	require.NoError(t, err)
	rewatch, err := service.StartRewatch(ctx, RewatchInput{
		Scope: RoundScope{UserID: userID, MediaID: mediaID}, ExpectedVersion: first.Version,
	})
	require.NoError(t, err)
	secondTime := firstTime.Add(24 * time.Hour)
	second, err := service.UpdateRound(ctx, UpdateRoundInput{
		Scope:  RoundScope{UserID: userID, MediaID: mediaID},
		Status: StatusCompleted, CompletedAt: &secondTime, Source: SourceManual,
		ExpectedVersion: rewatch.Current.Version,
	})
	require.NoError(t, err)
	_, err = service.StartRewatch(ctx, RewatchInput{
		Scope: RoundScope{UserID: userID, MediaID: mediaID}, ExpectedVersion: second.Version,
	})
	require.NoError(t, err)

	completed, err := service.CalendarMonth(ctx, CalendarMonthInput{
		UserID: userID, Year: 2026, Month: time.July,
		Timezone: "UTC", Filter: CalendarFilterCompleted,
	})
	require.NoError(t, err)
	require.Len(t, completed.Events, 2)
	require.Equal(t, []string{"calendar-round-guest", "owner"}, completed.Events[0].Participants)
	require.Equal(t, StatusCompleted, completed.Events[0].Status)
	inProgress, err := service.CalendarMonth(ctx, CalendarMonthInput{
		UserID: userID, Year: 2026, Month: time.July,
		Timezone: "UTC", Filter: CalendarFilterInProgress,
	})
	require.NoError(t, err)
	require.Empty(t, inProgress.Events)
}
