package records

import (
	"context"
	"errors"
	"strings"
	"time"
)

var (
	ErrCollectionNotFound = errors.New("collection not found")
	ErrInvalidRecord      = errors.New("invalid record")
	ErrVersionConflict    = errors.New("record version conflict")
)

type State struct {
	UserID       string
	MediaID      string
	Status       Status
	Rating       *int
	Note         *string
	Version      int
	StatusSource Source
	RatingSource Source
	NoteSource   Source
	StartedAt    *time.Time
	CompletedAt  *time.Time
}

type UpdateStateInput struct {
	UserID          string
	MediaID         string
	Status          Status
	Rating          *int
	RatingSet       bool
	Note            *string
	NoteSet         bool
	Source          Source
	ExpectedVersion int
}

type Collection struct {
	ID     string
	UserID string
	Name   string
	Items  []string
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (service *Service) UpdateState(ctx context.Context, input UpdateStateInput) (State, error) {
	update, err := service.prepareStateUpdate(ctx, input)
	if err != nil {
		return update.current, err
	}
	return service.persistStateUpdate(ctx, update)
}

type preparedStateUpdate struct {
	current State
	next    State
	exists  bool
	changed bool
}

func (service *Service) prepareStateUpdate(ctx context.Context, input UpdateStateInput) (preparedStateUpdate, error) {
	if input.UserID == "" || input.MediaID == "" || ValidateStatus(input.Status) != nil || sourcePriority(input.Source) == 0 {
		return preparedStateUpdate{}, ErrInvalidRecord
	}
	ratingProvided := input.RatingSet || input.Rating != nil
	noteProvided := input.NoteSet || input.Note != nil
	if ratingProvided && input.Rating != nil && (*input.Rating < 0 || *input.Rating > 100) {
		return preparedStateUpdate{}, ErrInvalidRating
	}
	current, exists, err := service.repository.FindState(ctx, input.UserID, input.MediaID)
	if err != nil {
		return preparedStateUpdate{}, err
	}
	if input.ExpectedVersion != current.Version {
		return preparedStateUpdate{current: current}, ErrVersionConflict
	}

	next := current
	changed := false
	if !exists || CanOverwrite(input.Source, current.StatusSource) {
		changed = changed || next.Status != input.Status || next.StatusSource != input.Source
		next.Status, next.StatusSource = input.Status, input.Source
	}
	if ratingProvided && (!exists || CanOverwrite(input.Source, current.RatingSource)) {
		changed = changed || !equalIntPointers(next.Rating, input.Rating) || next.RatingSource != input.Source
		next.Rating = cloneIntPointer(input.Rating)
		next.RatingSource = input.Source
	}
	if noteProvided && (!exists || CanOverwrite(input.Source, current.NoteSource)) {
		changed = changed || !equalStringPointers(next.Note, input.Note) || next.NoteSource != input.Source
		next.Note = cloneStringPointer(input.Note)
		next.NoteSource = input.Source
	}
	next.UserID, next.MediaID = input.UserID, input.MediaID
	if changed {
		next.Version = current.Version + 1
	}
	return preparedStateUpdate{current: current, next: next, exists: exists, changed: changed}, nil
}

func (service *Service) persistStateUpdate(ctx context.Context, update preparedStateUpdate) (State, error) {
	if !update.changed {
		return update.current, nil
	}
	if !update.exists {
		if err := service.repository.InsertState(ctx, update.next); err != nil {
			return State{}, err
		}
		return update.next, nil
	}
	updated, err := service.repository.UpdateState(ctx, update.next, update.current.Version)
	if err != nil {
		return State{}, err
	}
	if !updated {
		return update.current, ErrVersionConflict
	}
	return update.next, nil
}

func (service *Service) SetTags(ctx context.Context, userID, mediaID string, names []string) error {
	if userID == "" || mediaID == "" {
		return ErrInvalidRecord
	}
	return service.repository.SetTags(ctx, userID, mediaID, names)
}

func (service *Service) SetTagsVersioned(
	ctx context.Context,
	userID, mediaID string,
	names []string,
	expectedVersion int,
) (State, error) {
	if userID == "" || mediaID == "" || expectedVersion < 0 {
		return State{}, ErrInvalidRecord
	}
	state, exists, err := service.repository.FindState(ctx, userID, mediaID)
	if err != nil {
		return State{}, err
	}
	if !exists || state.Version != expectedVersion {
		return state, ErrVersionConflict
	}
	updated, err := service.repository.SetTagsVersioned(ctx, userID, mediaID, names, expectedVersion)
	if err != nil {
		return State{}, err
	}
	if !updated {
		return state, ErrVersionConflict
	}
	state.Version++
	return state, nil
}

func (service *Service) Tags(ctx context.Context, userID, mediaID string) ([]string, error) {
	return service.repository.Tags(ctx, userID, mediaID)
}

func (service *Service) CreateCollection(ctx context.Context, userID, name string) (Collection, error) {
	name = strings.TrimSpace(name)
	if userID == "" || name == "" {
		return Collection{}, ErrInvalidRecord
	}
	return service.repository.CreateCollection(ctx, userID, name)
}

func (service *Service) AddCollectionItem(ctx context.Context, userID, collectionID, mediaID string) error {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(collectionID) == "" || strings.TrimSpace(mediaID) == "" {
		return ErrInvalidRecord
	}
	return service.repository.AddCollectionItem(ctx, userID, collectionID, mediaID)
}

func (service *Service) ReplaceCollectionItems(
	ctx context.Context,
	userID, collectionID string,
	mediaIDs []string,
) error {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(collectionID) == "" {
		return ErrInvalidRecord
	}
	seen := make(map[string]struct{}, len(mediaIDs))
	normalized := make([]string, 0, len(mediaIDs))
	for _, mediaID := range mediaIDs {
		mediaID = strings.TrimSpace(mediaID)
		if mediaID == "" {
			return ErrInvalidRecord
		}
		if _, exists := seen[mediaID]; exists {
			return ErrInvalidRecord
		}
		seen[mediaID] = struct{}{}
		normalized = append(normalized, mediaID)
	}
	return service.repository.ReplaceCollectionItems(ctx, userID, collectionID, normalized)
}

func (service *Service) Collections(ctx context.Context, userID string) ([]Collection, error) {
	return service.repository.Collections(ctx, userID)
}

func equalIntPointers(left, right *int) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func equalStringPointers(left, right *string) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func cloneIntPointer(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
