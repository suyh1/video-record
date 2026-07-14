package records

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidWatchEvent = errors.New("invalid watch event")
)

type WatchEvent struct {
	ID              string
	RoundID         string
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

type CreateWatchEventInput struct {
	RoundID         string
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

func (service *Service) WatchEvents(ctx context.Context, userID, mediaID string) ([]WatchEvent, error) {
	if userID == "" || mediaID == "" {
		return nil, ErrInvalidWatchEvent
	}
	return service.repository.WatchEvents(ctx, userID, mediaID)
}

func newWatchEvent(input CreateWatchEventInput) (WatchEvent, error) {
	if input.RoundID == "" || input.UserID == "" || input.MediaID == "" || sourcePriority(input.Source) == 0 ||
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
		ID: uuid.NewString(), RoundID: input.RoundID, UserID: input.UserID, MediaID: input.MediaID,
		EpisodeID: input.EpisodeID, WatchedAt: input.WatchedAt.UTC(),
		ViewingMethod: input.ViewingMethod, Source: input.Source,
		ExternalEventID: input.ExternalEventID, Completion: completion, Note: input.Note,
		ParticipantIDs: participants,
	}, nil
}
