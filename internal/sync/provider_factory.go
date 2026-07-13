package sync

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"video-record/internal/integrations"
	"video-record/internal/integrations/emby"
	"video-record/internal/integrations/jellyfin"
	"video-record/internal/integrations/plex"
)

var ErrInvalidProviderConfiguration = errors.New("invalid provider configuration")

type ProviderFactory interface {
	New(integrations.Account, []byte) (integrations.Provider, error)
}

type ProviderFactoryFunc func(integrations.Account, []byte) (integrations.Provider, error)

func (factory ProviderFactoryFunc) New(
	account integrations.Account,
	credentials []byte,
) (integrations.Provider, error) {
	return factory(account, credentials)
}

type DefaultProviderFactoryOptions struct {
	HTTPClient *http.Client
	Now        func() time.Time
}

type DefaultProviderFactory struct {
	httpClient *http.Client
	now        func() time.Time
}

func NewDefaultProviderFactory(options DefaultProviderFactoryOptions) *DefaultProviderFactory {
	return &DefaultProviderFactory{httpClient: options.HTTPClient, now: options.Now}
}

func (factory *DefaultProviderFactory) New(
	account integrations.Account,
	credentials []byte,
) (integrations.Provider, error) {
	switch account.Provider {
	case "jellyfin":
		var value jellyfinCredentials
		if decodeProviderCredentials(credentials, &value) != nil ||
			strings.TrimSpace(value.Token) == "" || strings.TrimSpace(value.UserID) == "" {
			return nil, ErrInvalidProviderConfiguration
		}
		return jellyfin.NewClient(jellyfin.ClientOptions{
			BaseURL: account.BaseURL, Token: value.Token, UserID: value.UserID,
			HTTPClient: factory.httpClient, Now: factory.now,
		}), nil
	case "emby":
		var value embyCredentials
		if decodeProviderCredentials(credentials, &value) != nil ||
			strings.TrimSpace(value.Token) == "" || strings.TrimSpace(value.UserID) == "" {
			return nil, ErrInvalidProviderConfiguration
		}
		location := time.UTC
		if strings.TrimSpace(value.Timezone) != "" {
			parsed, err := time.LoadLocation(value.Timezone)
			if err != nil {
				return nil, ErrInvalidProviderConfiguration
			}
			location = parsed
		}
		return emby.NewClient(emby.ClientOptions{
			BaseURL: account.BaseURL, Token: value.Token, UserID: value.UserID,
			HTTPClient: factory.httpClient, Now: factory.now, Location: location,
		}), nil
	case "plex":
		var value plexCredentials
		if decodeProviderCredentials(credentials, &value) != nil ||
			strings.TrimSpace(value.Token) == "" || value.AccountID <= 0 {
			return nil, ErrInvalidProviderConfiguration
		}
		return plex.NewClient(plex.ClientOptions{
			BaseURL: account.BaseURL, Token: value.Token, AccountID: value.AccountID,
			HTTPClient: factory.httpClient,
		}), nil
	default:
		return nil, ErrInvalidProviderConfiguration
	}
}

type jellyfinCredentials struct {
	Token  string `json:"token"`
	UserID string `json:"userId"`
}

type embyCredentials struct {
	Token    string `json:"token"`
	UserID   string `json:"userId"`
	Timezone string `json:"timezone,omitempty"`
}

type plexCredentials struct {
	Token     string `json:"token"`
	AccountID int    `json:"accountId"`
}

func decodeProviderCredentials(contents []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return ErrInvalidProviderConfiguration
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ErrInvalidProviderConfiguration
	}
	return nil
}
