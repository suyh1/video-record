package media

import (
	"context"
	"errors"
	"strings"
)

var (
	ErrExternalIdentityConflict = errors.New("external identity belongs to another media item")
	ErrInvalidMedia             = errors.New("invalid media item")
	ErrMediaTypeMismatch        = errors.New("media type mismatch")
)

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (service *Service) UpsertExternal(ctx context.Context, snapshot ExternalSnapshot) (Item, error) {
	if err := validateExternalSnapshot(snapshot); err != nil {
		return Item{}, err
	}
	return service.repository.UpsertExternal(ctx, snapshot)
}

func (service *Service) CreateCustom(ctx context.Context, input CreateCustomInput) (Item, error) {
	input.Title = strings.TrimSpace(input.Title)
	if !validMediaType(input.MediaType) || input.Title == "" {
		return Item{}, ErrInvalidMedia
	}
	return service.repository.CreateCustom(ctx, input)
}

func (service *Service) LinkExternal(ctx context.Context, itemID string, snapshot ExternalSnapshot) (Item, error) {
	if itemID == "" || validateExternalSnapshot(snapshot) != nil {
		return Item{}, ErrInvalidMedia
	}
	return service.repository.LinkExternal(ctx, itemID, snapshot)
}

func (service *Service) FindByID(ctx context.Context, id string) (Item, error) {
	return service.repository.FindByID(ctx, id)
}

func validateExternalSnapshot(snapshot ExternalSnapshot) error {
	if strings.TrimSpace(snapshot.Source) == "" ||
		strings.TrimSpace(snapshot.SourceID) == "" ||
		strings.TrimSpace(snapshot.Title) == "" ||
		!validMediaType(snapshot.MediaType) || snapshot.RuntimeMinutes < 0 {
		return ErrInvalidMedia
	}
	for _, genre := range snapshot.Genres {
		if strings.TrimSpace(genre.ID) == "" || strings.TrimSpace(genre.Name) == "" {
			return ErrInvalidMedia
		}
	}
	return nil
}

func validMediaType(mediaType MediaType) bool {
	return mediaType == MediaTypeMovie || mediaType == MediaTypeTV
}
