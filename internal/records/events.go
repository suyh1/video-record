package records

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidWatchEvent  = errors.New("invalid watch event")
	ErrWatchEventNotFound = errors.New("watch event not found")
)

type WatchEvent struct {
	ID              string
	UserID          string
	MediaID         string
	EpisodeID       string
	WatchedAt       time.Time
	ViewingMethod   string
	Source          Source
	ExternalEventID string
	Completion      int
	Note            string
	ParticipantIDs  []string
}

type RecordStatusInput struct {
	UpdateStateInput
	WatchedAt      time.Time
	ViewingMethod  string
	ParticipantIDs []string
}

type CreateWatchEventInput struct {
	UserID          string
	MediaID         string
	EpisodeID       string
	WatchedAt       time.Time
	ViewingMethod   string
	Source          Source
	ExternalEventID string
	Completion      int
	Note            string
	ParticipantIDs  []string
}

func (service *Service) RecordStatus(ctx context.Context, input RecordStatusInput) (State, *WatchEvent, error) {
	update, err := service.prepareStateUpdate(ctx, input.UpdateStateInput)
	if err != nil {
		return update.current, nil, err
	}
	createsEvent := update.changed && update.next.Status == StatusCompleted &&
		(!update.exists || update.current.Status != StatusCompleted)
	if !createsEvent {
		state, err := service.persistStateUpdate(ctx, update)
		return state, nil, err
	}
	event, err := newWatchEvent(CreateWatchEventInput{
		UserID: input.UserID, MediaID: input.MediaID, WatchedAt: input.WatchedAt,
		ViewingMethod: input.ViewingMethod, Source: input.Source, Completion: 100,
		ParticipantIDs: input.ParticipantIDs,
	})
	if err != nil {
		return update.current, nil, err
	}
	applied, err := service.repository.ApplyStateAndEvent(
		ctx, update.next, update.current.Version, update.exists, event,
	)
	if err != nil {
		return update.current, nil, err
	}
	if !applied {
		return update.current, nil, ErrVersionConflict
	}
	state, _, err := service.repository.FindState(ctx, input.UserID, input.MediaID)
	if err != nil {
		return State{}, nil, err
	}
	return state, &event, nil
}

func (service *Service) AddRewatch(ctx context.Context, input CreateWatchEventInput) (WatchEvent, error) {
	state, exists, err := service.repository.FindState(ctx, input.UserID, input.MediaID)
	if err != nil {
		return WatchEvent{}, err
	}
	if !exists || state.Status != StatusCompleted {
		return WatchEvent{}, ErrInvalidWatchEvent
	}
	event, err := newWatchEvent(input)
	if err != nil {
		return WatchEvent{}, err
	}
	if err := service.repository.CreateWatchEvent(ctx, event); err != nil {
		return WatchEvent{}, err
	}
	return event, nil
}

func (service *Service) WatchEvents(ctx context.Context, userID, mediaID string) ([]WatchEvent, error) {
	if userID == "" || mediaID == "" {
		return nil, ErrInvalidWatchEvent
	}
	return service.repository.WatchEvents(ctx, userID, mediaID)
}

func (service *Service) DeleteWatchEvent(ctx context.Context, userID, eventID string) error {
	if userID == "" || eventID == "" {
		return ErrInvalidWatchEvent
	}
	return service.repository.DeleteWatchEvent(ctx, userID, eventID)
}

func newWatchEvent(input CreateWatchEventInput) (WatchEvent, error) {
	if input.UserID == "" || input.MediaID == "" || sourcePriority(input.Source) == 0 ||
		input.Completion < 0 || input.Completion > 100 {
		return WatchEvent{}, ErrInvalidWatchEvent
	}
	if input.WatchedAt.IsZero() {
		input.WatchedAt = time.Now().UTC()
	}
	participants := make([]string, 0, len(input.ParticipantIDs)+1)
	seen := make(map[string]struct{}, len(input.ParticipantIDs)+1)
	for _, participantID := range append([]string{input.UserID}, input.ParticipantIDs...) {
		participantID = strings.TrimSpace(participantID)
		if participantID == "" {
			return WatchEvent{}, ErrInvalidWatchEvent
		}
		if _, exists := seen[participantID]; exists {
			continue
		}
		seen[participantID] = struct{}{}
		participants = append(participants, participantID)
	}
	completion := input.Completion
	if completion == 0 {
		completion = 100
	}
	return WatchEvent{
		ID: uuid.NewString(), UserID: input.UserID, MediaID: input.MediaID,
		EpisodeID: input.EpisodeID, WatchedAt: input.WatchedAt.UTC(),
		ViewingMethod: input.ViewingMethod, Source: input.Source,
		ExternalEventID: input.ExternalEventID, Completion: completion, Note: input.Note,
		ParticipantIDs: participants,
	}, nil
}
