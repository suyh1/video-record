package records

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewWatchEventRequiresRoundAndNormalizesParticipants(t *testing.T) {
	watchedAt := time.Date(2026, 7, 13, 20, 30, 45, 0, time.FixedZone("UTC+8", 8*60*60))
	event, err := newWatchEvent(CreateWatchEventInput{
		RoundID: "round-1", UserID: "user-1", MediaID: "movie-1",
		WatchedAt: watchedAt, Source: SourceManual,
		ParticipantIDs: []string{"guest-1", "user-1", "guest-1"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, event.ID)
	require.Equal(t, "round-1", event.RoundID)
	require.Equal(t, watchedAt.UTC(), event.WatchedAt)
	require.Equal(t, 100, event.Completion)
	require.Equal(t, []string{"user-1", "guest-1"}, event.ParticipantIDs)

	for _, input := range []CreateWatchEventInput{
		{UserID: "user-1", MediaID: "movie-1", Source: SourceManual},
		{RoundID: "round-1", UserID: "user-1", MediaID: "movie-1", Source: "unknown"},
		{RoundID: "round-1", UserID: "user-1", MediaID: "movie-1", Source: SourceManual, Completion: 101},
		{RoundID: "round-1", UserID: "user-1", MediaID: "movie-1", Source: SourceManual, ParticipantIDs: []string{" "}},
	} {
		_, err := newWatchEvent(input)
		require.ErrorIs(t, err, ErrInvalidWatchEvent)
	}
}

func TestWatchEventsReadsCompletedRoundForEveryParticipant(t *testing.T) {
	service, db, userID, mediaID := newTestRecordsService(t)
	_, err := service.WatchEvents(context.Background(), "", mediaID)
	require.ErrorIs(t, err, ErrInvalidWatchEvent)
	guestID := insertTestUser(t, db, "round-event-guest")
	otherID := insertTestUser(t, db, "round-event-other")
	watchedAt := time.Date(2026, 7, 13, 12, 30, 45, 0, time.UTC)

	round, err := service.UpdateRound(context.Background(), UpdateRoundInput{
		Scope: RoundScope{UserID: userID, MediaID: mediaID}, Status: StatusCompleted,
		CompletedAt: &watchedAt, Source: SourceManual, ParticipantIDs: []string{guestID},
	})
	require.NoError(t, err)

	for _, participantID := range []string{userID, guestID} {
		events, err := service.WatchEvents(context.Background(), participantID, mediaID)
		require.NoError(t, err)
		require.Len(t, events, 1)
		require.Equal(t, round.ID, events[0].RoundID)
		require.Equal(t, watchedAt, events[0].WatchedAt)
	}
	otherEvents, err := service.WatchEvents(context.Background(), otherID, mediaID)
	require.NoError(t, err)
	require.Empty(t, otherEvents)
}

func mustWatchEvents(t *testing.T, service *Service, userID, mediaID string) []WatchEvent {
	t.Helper()
	events, err := service.WatchEvents(context.Background(), userID, mediaID)
	require.NoError(t, err)
	return events
}
