package records

import (
	"context"
	"strings"
)

const (
	DefaultLibraryLimit = 40
	MaxLibraryLimit     = 100
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

type LibraryQuery struct {
	Status    Status
	Cursor    string
	Limit     int
	MediaType string // "", "movie", "tv"
	Sort      string // "updated" (default), "title", "rating", "watched"
	Query     string
	Tag       string
}

type LibraryPage struct {
	Items      []CatalogItem
	NextCursor string
}

func (service *Service) State(ctx context.Context, userID, mediaID string) (State, bool, error) {
	if userID == "" || mediaID == "" {
		return State{}, false, ErrInvalidRecord
	}
	return service.repository.FindState(ctx, userID, mediaID)
}

func (service *Service) Library(ctx context.Context, userID string, status Status) ([]CatalogItem, error) {
	page, err := service.LibraryPage(ctx, userID, LibraryQuery{Status: status, Limit: MaxLibraryLimit})
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

func (service *Service) LibraryPage(ctx context.Context, userID string, query LibraryQuery) (LibraryPage, error) {
	if userID == "" {
		return LibraryPage{}, ErrInvalidRecord
	}
	if query.Status != "" && ValidateStatus(query.Status) != nil {
		return LibraryPage{}, ErrInvalidRecord
	}
	query.MediaType = strings.TrimSpace(query.MediaType)
	if query.MediaType != "" && query.MediaType != "movie" && query.MediaType != "tv" {
		return LibraryPage{}, ErrInvalidRecord
	}
	query.Sort = strings.TrimSpace(query.Sort)
	if query.Sort == "" {
		query.Sort = "updated"
	}
	switch query.Sort {
	case "updated", "title", "rating", "watched":
	default:
		return LibraryPage{}, ErrInvalidRecord
	}
	query.Query = strings.TrimSpace(query.Query)
	query.Tag = strings.TrimSpace(query.Tag)
	limit := query.Limit
	if limit == 0 {
		limit = DefaultLibraryLimit
	}
	if limit < 1 || limit > MaxLibraryLimit {
		return LibraryPage{}, ErrInvalidRecord
	}
	query.Limit = limit
	return service.repository.Library(ctx, userID, query)
}

func (service *Service) SearchMedia(ctx context.Context, userID, query string) ([]CatalogItem, error) {
	query = strings.TrimSpace(query)
	if userID == "" || query == "" {
		return nil, ErrInvalidRecord
	}
	return service.repository.SearchMedia(ctx, userID, query)
}
