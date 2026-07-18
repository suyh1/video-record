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

type Collection struct {
	ID     string
	UserID string
	Name   string
	Items  []string
}

type Service struct {
	repository Repository
	now        func() time.Time
}

type ServiceOptions struct {
	Now func() time.Time
}

func NewService(repository Repository, options ...ServiceOptions) *Service {
	now := time.Now
	if len(options) > 0 && options[0].Now != nil {
		now = options[0].Now
	}
	return &Service{repository: repository, now: now}
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
	profile, exists, err := service.repository.FindProfile(ctx, userID, mediaID)
	if err != nil {
		return State{}, err
	}
	if exists && profile.Version != expectedVersion || !exists && expectedVersion != 0 {
		return State{UserID: userID, MediaID: mediaID, Status: profile.Status, Version: profile.Version}, ErrVersionConflict
	}
	updated, err := service.repository.SetTagsVersioned(ctx, userID, mediaID, names, expectedVersion)
	if err != nil {
		return State{}, err
	}
	if !updated {
		latest, _, findErr := service.repository.FindProfile(ctx, userID, mediaID)
		if findErr != nil {
			return State{}, findErr
		}
		return State{UserID: userID, MediaID: mediaID, Status: latest.Status, Version: latest.Version}, ErrVersionConflict
	}
	latest, _, err := service.repository.FindProfile(ctx, userID, mediaID)
	if err != nil {
		return State{}, err
	}
	return State{UserID: userID, MediaID: mediaID, Status: latest.Status, Version: latest.Version}, nil
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

func (service *Service) CollectionItems(
	ctx context.Context,
	userID, collectionID string,
	status Status,
) ([]CatalogItem, error) {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(collectionID) == "" {
		return nil, ErrInvalidRecord
	}
	if status != "" && ValidateStatus(status) != nil {
		return nil, ErrInvalidRecord
	}
	return service.repository.CollectionItems(ctx, userID, collectionID, status)
}

func (service *Service) RenameCollection(ctx context.Context, userID, collectionID, name string) (Collection, error) {
	name = strings.TrimSpace(name)
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(collectionID) == "" || name == "" {
		return Collection{}, ErrInvalidRecord
	}
	return service.repository.RenameCollection(ctx, userID, collectionID, name)
}

func (service *Service) DeleteCollection(ctx context.Context, userID, collectionID string) error {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(collectionID) == "" {
		return ErrInvalidRecord
	}
	return service.repository.DeleteCollection(ctx, userID, collectionID)
}

func (service *Service) UserTags(ctx context.Context, userID string) ([]string, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, ErrInvalidRecord
	}
	return service.repository.UserTags(ctx, userID)
}

const DefaultViewingMethodLimit = 8

func (service *Service) ViewingMethods(ctx context.Context, userID string) ([]string, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, ErrInvalidRecord
	}
	return service.repository.ViewingMethods(ctx, userID, DefaultViewingMethodLimit)
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
