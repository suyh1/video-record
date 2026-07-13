package integrations

import (
	"context"
	"errors"
	"time"
)

type MediaType string

const (
	MediaMovie   MediaType = "movie"
	MediaEpisode MediaType = "episode"
)

var (
	ErrInvalidHistory  = errors.New("invalid provider history")
	ErrAuthentication  = errors.New("provider authentication failed")
	ErrRateLimited     = errors.New("provider rate limited")
	ErrUnavailable     = errors.New("provider unavailable")
	ErrInvalidResponse = errors.New("invalid provider response")
)

type Provider interface {
	CheckAuthentication(context.Context) error
	History(context.Context, HistoryRequest) (HistoryPage, error)
}

type HistoryRequest struct {
	Cursor string
	Limit  int
	Since  time.Time
	Until  time.Time
}

type HistoryPage struct {
	Events     []HistoryEvent
	NextCursor string
}

type HistoryEvent struct {
	ID              string
	PlayedAt        time.Time
	DurationSeconds int
	PositionSeconds int
	Item            ItemIdentity
}

type ItemIdentity struct {
	ProviderItemID string
	TMDBID         string
	IMDbID         string
	TVDBID         string
	MediaType      MediaType
	Title          string
	OriginalTitle  string
	Year           int
	SeasonNumber   int
	EpisodeNumber  int
}

func ValidateHistoryPage(page HistoryPage) error {
	seen := make(map[string]struct{}, len(page.Events))
	for _, event := range page.Events {
		if event.ID == "" || event.PlayedAt.IsZero() || !validItemIdentity(event.Item) {
			return ErrInvalidHistory
		}
		if _, exists := seen[event.ID]; exists {
			return ErrInvalidHistory
		}
		seen[event.ID] = struct{}{}
	}
	return nil
}

func validItemIdentity(item ItemIdentity) bool {
	if item.MediaType != MediaMovie && item.MediaType != MediaEpisode {
		return false
	}
	if item.ProviderItemID == "" && item.TMDBID == "" && item.IMDbID == "" && item.TVDBID == "" {
		return false
	}
	return item.Title != ""
}

type ErrorKind string

const (
	ErrorAuthentication  ErrorKind = "authentication"
	ErrorRateLimited     ErrorKind = "rate_limited"
	ErrorUnavailable     ErrorKind = "unavailable"
	ErrorInvalidResponse ErrorKind = "invalid_response"
)

type ProviderError struct {
	Kind       ErrorKind
	RetryDelay time.Duration
}

func NewProviderError(kind ErrorKind, retryAfter time.Duration, _ error) *ProviderError {
	return &ProviderError{Kind: kind, RetryDelay: retryAfter}
}

func (err *ProviderError) Error() string {
	switch err.Kind {
	case ErrorAuthentication:
		return ErrAuthentication.Error()
	case ErrorRateLimited:
		return ErrRateLimited.Error()
	case ErrorInvalidResponse:
		return ErrInvalidResponse.Error()
	default:
		return ErrUnavailable.Error()
	}
}

func (err *ProviderError) Unwrap() error {
	switch err.Kind {
	case ErrorAuthentication:
		return ErrAuthentication
	case ErrorRateLimited:
		return ErrRateLimited
	case ErrorInvalidResponse:
		return ErrInvalidResponse
	default:
		return ErrUnavailable
	}
}

func IsRetryable(err error) bool {
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		return false
	}
	return providerErr.Kind == ErrorRateLimited || providerErr.Kind == ErrorUnavailable
}

func RetryAfter(err error) time.Duration {
	var providerErr *ProviderError
	if errors.As(err, &providerErr) {
		return providerErr.RetryDelay
	}
	return 0
}
