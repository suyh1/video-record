package records

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEventStatusRewatchAndDeletionRules(t *testing.T) {
	service, _, userID, mediaID := newTestRecordsService(t)
	ctx := context.Background()
	rating := 92
	note := "保留的私人笔记"

	wishlist, event, err := service.RecordStatus(ctx, RecordStatusInput{
		UpdateStateInput: UpdateStateInput{
			UserID: userID, MediaID: mediaID, Status: StatusWishlist,
			Rating: &rating, Note: &note, Source: SourceManual, ExpectedVersion: 0,
		},
		WatchedAt: time.Date(2026, 7, 10, 20, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.Equal(t, StatusWishlist, wishlist.Status)
	require.Nil(t, event)
	require.Empty(t, mustWatchEvents(t, service, userID, mediaID))

	firstWatchedAt := time.Date(2026, 7, 11, 21, 0, 0, 0, time.UTC)
	completed, first, err := service.RecordStatus(ctx, RecordStatusInput{
		UpdateStateInput: UpdateStateInput{
			UserID: userID, MediaID: mediaID, Status: StatusCompleted,
			Source: SourceManual, ExpectedVersion: wishlist.Version,
		},
		WatchedAt: firstWatchedAt,
	})
	require.NoError(t, err)
	require.NotNil(t, first)
	require.Equal(t, StatusCompleted, completed.Status)
	require.Equal(t, SourceManual, first.Source)

	secondWatchedAt := time.Date(2026, 7, 12, 22, 0, 0, 0, time.UTC)
	second, err := service.AddRewatch(ctx, CreateWatchEventInput{
		UserID: userID, MediaID: mediaID, WatchedAt: secondWatchedAt,
		Source: SourceManual, ExternalEventID: "manual-rewatch-2",
	})
	require.NoError(t, err)
	require.NotEqual(t, first.ID, second.ID)
	require.Equal(t, "manual-rewatch-2", second.ExternalEventID)
	require.Len(t, mustWatchEvents(t, service, userID, mediaID), 2)

	afterRewatch, exists, err := service.repository.FindState(ctx, userID, mediaID)
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, firstWatchedAt, *afterRewatch.StartedAt)
	require.Equal(t, secondWatchedAt, *afterRewatch.CompletedAt)
	require.Equal(t, rating, *afterRewatch.Rating)
	require.Equal(t, note, *afterRewatch.Note)

	require.NoError(t, service.DeleteWatchEvent(ctx, userID, second.ID))
	afterDelete, exists, err := service.repository.FindState(ctx, userID, mediaID)
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, firstWatchedAt, *afterDelete.StartedAt)
	require.Equal(t, firstWatchedAt, *afterDelete.CompletedAt)
	require.Equal(t, rating, *afterDelete.Rating)
	require.Equal(t, note, *afterDelete.Note)

	require.NoError(t, service.DeleteWatchEvent(ctx, userID, first.ID))
	afterAllDeleted, exists, err := service.repository.FindState(ctx, userID, mediaID)
	require.NoError(t, err)
	require.True(t, exists)
	require.Nil(t, afterAllDeleted.StartedAt)
	require.Nil(t, afterAllDeleted.CompletedAt)
	require.Equal(t, rating, *afterAllDeleted.Rating)
	require.Equal(t, note, *afterAllDeleted.Note)
}

func TestEventValidationOwnershipAndExternalDeduplication(t *testing.T) {
	service, db, userID, mediaID := newTestRecordsService(t)
	ctx := context.Background()
	_, err := service.AddRewatch(ctx, CreateWatchEventInput{
		UserID: userID, MediaID: mediaID, Source: SourceManual,
	})
	require.ErrorIs(t, err, ErrInvalidWatchEvent)

	completed, first, err := service.RecordStatus(ctx, RecordStatusInput{
		UpdateStateInput: UpdateStateInput{
			UserID: userID, MediaID: mediaID, Status: StatusCompleted,
			Source: SourceManual, ExpectedVersion: 0,
		},
		WatchedAt: time.Date(2026, 7, 11, 21, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.NotNil(t, first)

	unchanged, repeated, err := service.RecordStatus(ctx, RecordStatusInput{
		UpdateStateInput: UpdateStateInput{
			UserID: userID, MediaID: mediaID, Status: StatusCompleted,
			Source: SourceManual, ExpectedVersion: completed.Version,
		},
	})
	require.NoError(t, err)
	require.Nil(t, repeated)
	require.Equal(t, completed.Version, unchanged.Version)

	rewatch := CreateWatchEventInput{
		UserID: userID, MediaID: mediaID,
		WatchedAt: time.Date(2026, 7, 12, 21, 0, 0, 0, time.UTC),
		Source:    SourceConfirmedSync, ExternalEventID: "provider-event-42",
	}
	second, err := service.AddRewatch(ctx, rewatch)
	require.NoError(t, err)
	_, err = service.AddRewatch(ctx, rewatch)
	require.Error(t, err)
	require.Len(t, mustWatchEvents(t, service, userID, mediaID), 2)

	secondUserID := insertTestUser(t, db, "event-outsider")
	require.ErrorIs(t, service.DeleteWatchEvent(ctx, secondUserID, second.ID), ErrWatchEventNotFound)
	require.Len(t, mustWatchEvents(t, service, userID, mediaID), 2)

	_, err = service.WatchEvents(ctx, "", mediaID)
	require.ErrorIs(t, err, ErrInvalidWatchEvent)
	require.ErrorIs(t, service.DeleteWatchEvent(ctx, userID, ""), ErrInvalidWatchEvent)
	_, err = newWatchEvent(CreateWatchEventInput{UserID: userID, MediaID: mediaID, Source: "unknown"})
	require.ErrorIs(t, err, ErrInvalidWatchEvent)
	_, err = newWatchEvent(CreateWatchEventInput{
		UserID: userID, MediaID: mediaID, Source: SourceManual, Completion: 101,
	})
	require.ErrorIs(t, err, ErrInvalidWatchEvent)
	_, err = newWatchEvent(CreateWatchEventInput{
		UserID: userID, MediaID: mediaID, Source: SourceManual, ParticipantIDs: []string{""},
	})
	require.ErrorIs(t, err, ErrInvalidWatchEvent)
}

func TestEventDefaultsAndRecordStatusConflicts(t *testing.T) {
	service, _, userID, mediaID := newTestRecordsService(t)
	ctx := context.Background()
	event, err := newWatchEvent(CreateWatchEventInput{
		UserID: userID, MediaID: mediaID, Source: SourceManual,
	})
	require.NoError(t, err)
	require.False(t, event.WatchedAt.IsZero())
	require.Equal(t, 100, event.Completion)
	require.Equal(t, []string{userID}, event.ParticipantIDs)

	_, _, err = service.RecordStatus(ctx, RecordStatusInput{UpdateStateInput: UpdateStateInput{
		UserID: userID, MediaID: mediaID, Status: StatusWishlist, Source: "unknown",
	}})
	require.ErrorIs(t, err, ErrInvalidRecord)

	wishlist, _, err := service.RecordStatus(ctx, RecordStatusInput{UpdateStateInput: UpdateStateInput{
		UserID: userID, MediaID: mediaID, Status: StatusWishlist,
		Source: SourceManual, ExpectedVersion: 0,
	}})
	require.NoError(t, err)
	current, created, err := service.RecordStatus(ctx, RecordStatusInput{UpdateStateInput: UpdateStateInput{
		UserID: userID, MediaID: mediaID, Status: StatusCompleted,
		Source: SourceManual, ExpectedVersion: 0,
	}})
	require.ErrorIs(t, err, ErrVersionConflict)
	require.Nil(t, created)
	require.Equal(t, wishlist.Version, current.Version)
	require.Empty(t, mustWatchEvents(t, service, userID, mediaID))
	require.ErrorIs(t, service.DeleteWatchEvent(ctx, userID, "missing-event"), ErrWatchEventNotFound)
}

func TestCompletedEventIncludesSelectedHouseholdParticipants(t *testing.T) {
	service, db, userID, mediaID := newTestRecordsService(t)
	participantID := insertTestUser(t, db, "co-watcher")
	_, event, err := service.RecordStatus(context.Background(), RecordStatusInput{
		UpdateStateInput: UpdateStateInput{
			UserID: userID, MediaID: mediaID, Status: StatusCompleted,
			Source: SourceManual, ExpectedVersion: 0,
		},
		ParticipantIDs: []string{participantID, participantID},
	})
	require.NoError(t, err)
	require.NotNil(t, event)

	var participants int
	require.NoError(t, db.Reader().QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM watch_event_participants WHERE event_id = ?
	`, event.ID).Scan(&participants))
	require.Equal(t, 2, participants)
}

func mustWatchEvents(t *testing.T, service *Service, userID, mediaID string) []WatchEvent {
	t.Helper()
	events, err := service.WatchEvents(context.Background(), userID, mediaID)
	require.NoError(t, err)
	return events
}
