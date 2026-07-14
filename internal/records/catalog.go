package records

import (
	"context"
	"strings"
)

type CatalogItem struct {
	ID            string
	TMDBID        *int
	MediaType     string
	Title         string
	OriginalTitle string
	Year          string
	PosterPath    string
	Status        Status
}

func (service *Service) State(ctx context.Context, userID, mediaID string) (State, bool, error) {
	if userID == "" || mediaID == "" {
		return State{}, false, ErrInvalidRecord
	}
	return service.repository.FindState(ctx, userID, mediaID)
}

func (service *Service) Library(ctx context.Context, userID string, status Status) ([]CatalogItem, error) {
	if userID == "" || status != "" && ValidateStatus(status) != nil {
		return nil, ErrInvalidRecord
	}
	return service.repository.Library(ctx, userID, status)
}

func (service *Service) SearchMedia(ctx context.Context, userID, query string) ([]CatalogItem, error) {
	query = strings.TrimSpace(query)
	if userID == "" || query == "" {
		return nil, ErrInvalidRecord
	}
	return service.repository.SearchMedia(ctx, userID, query)
}
