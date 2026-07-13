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
	state, first, err := service.RecordStatus(ctx, RecordStatusInput{
		UpdateStateInput: UpdateStateInput{
			UserID: userID, MediaID: mediaID, Status: StatusCompleted,
			Source: SourceManual, ExpectedVersion: 0,
		},
		WatchedAt: firstTime,
	})
	require.NoError(t, err)
	require.NotNil(t, first)
	second, err := service.AddRewatch(ctx, CreateWatchEventInput{
		UserID: userID, MediaID: mediaID,
		WatchedAt: firstTime.Add(2 * time.Hour), Source: SourceManual,
	})
	require.NoError(t, err)
	_, err = service.AddRewatch(ctx, CreateWatchEventInput{
		UserID: userID, MediaID: mediaID,
		WatchedAt: time.Date(2026, 7, 31, 16, 0, 0, 0, time.UTC), Source: SourceManual,
	})
	require.NoError(t, err)

	guestID := insertTestUser(t, db, "calendar-guest")
	_, err = db.Writer().ExecContext(ctx, `
		INSERT INTO watch_event_participants (event_id, user_id) VALUES (?, ?)
	`, first.ID, guestID)
	require.NoError(t, err)

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
	require.Equal(t, state.Status, calendar.Events[0].Status)

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
	completed, _, err := service.RecordStatus(ctx, RecordStatusInput{
		UpdateStateInput: UpdateStateInput{
			UserID: userID, MediaID: mediaID, Status: StatusCompleted,
			Source: SourceManual, ExpectedVersion: 0,
		},
		WatchedAt: time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	watching, err := service.UpdateState(ctx, UpdateStateInput{
		UserID: userID, MediaID: mediaID, Status: StatusWatching,
		Source: SourceManual, ExpectedVersion: completed.Version,
	})
	require.NoError(t, err)
	require.Equal(t, StatusWatching, watching.Status)

	inProgress, err := service.CalendarMonth(ctx, CalendarMonthInput{
		UserID: userID, Year: 2026, Month: time.July,
		Timezone: "UTC", Filter: CalendarFilterInProgress,
	})
	require.NoError(t, err)
	require.Len(t, inProgress.Events, 1)

	completedOnly, err := service.CalendarMonth(ctx, CalendarMonthInput{
		UserID: userID, Year: 2026, Month: time.July,
		Timezone: "UTC", Filter: CalendarFilterCompleted,
	})
	require.NoError(t, err)
	require.Empty(t, completedOnly.Events)

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
		UserID: userID, MediaID: mediaID, Action: EpisodeProgressSingle,
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
