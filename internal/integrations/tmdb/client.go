package tmdb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBaseURL   = "https://api.themoviedb.org/3"
	defaultTimeout   = 8 * time.Second
	searchCacheTTL   = 6 * time.Hour
	detailsCacheTTL  = 7 * 24 * time.Hour
	maxResponseBytes = 5 << 20
)

var (
	ErrNotConfigured       = errors.New("tmdb is not configured")
	ErrRateLimited         = errors.New("tmdb rate limited")
	ErrUpstreamTimeout     = errors.New("tmdb request timed out")
	ErrUpstreamUnavailable = errors.New("tmdb is unavailable")
)

type ClientError struct {
	Kind       error
	RetryAfter time.Duration
}

func (err *ClientError) Error() string {
	return err.Kind.Error()
}

func (err *ClientError) Unwrap() error {
	return err.Kind
}

type ClientOptions struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
	Cache      *Cache
	Logger     *slog.Logger
	Timeout    time.Duration
}

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
	cache      *Cache
	logger     *slog.Logger
	timeout    time.Duration
}

func NewClient(options ClientOptions) *Client {
	baseURL := strings.TrimRight(options.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	logger := options.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &Client{
		baseURL:    baseURL,
		token:      strings.TrimSpace(options.Token),
		httpClient: httpClient,
		cache:      options.Cache,
		logger:     logger,
		timeout:    timeout,
	}
}

func (client *Client) Configured() bool {
	return client.token != ""
}

func (client *Client) Search(ctx context.Context, query, language string) (SearchResponse, error) {
	values := url.Values{"query": {query}, "language": {language}}
	var response SearchResponse
	if err := client.get(ctx, "/search/multi", values, searchCacheTTL, &response); err != nil {
		return SearchResponse{}, err
	}
	filtered := response.Results[:0]
	for _, result := range response.Results {
		if result.MediaType == "movie" || result.MediaType == "tv" {
			filtered = append(filtered, result)
		}
	}
	response.Results = filtered
	response.TotalResults = len(filtered)
	return response, nil
}

func (client *Client) MovieDetails(ctx context.Context, id int, language string) (MovieDetails, error) {
	var response MovieDetails
	err := client.get(ctx, fmt.Sprintf("/movie/%d", id), languageQuery(language), detailsCacheTTL, &response)
	return response, err
}

func (client *Client) TVDetails(ctx context.Context, id int, language string) (TVDetails, error) {
	var response TVDetails
	err := client.get(ctx, fmt.Sprintf("/tv/%d", id), languageQuery(language), detailsCacheTTL, &response)
	return response, err
}

func (client *Client) SeasonDetails(ctx context.Context, tvID, season int, language string) (SeasonDetails, error) {
	var response SeasonDetails
	err := client.get(ctx, fmt.Sprintf("/tv/%d/season/%d", tvID, season), languageQuery(language), detailsCacheTTL, &response)
	return response, err
}

func (client *Client) EpisodeDetails(ctx context.Context, tvID, season, episode int, language string) (EpisodeDetails, error) {
	var response EpisodeDetails
	err := client.get(
		ctx,
		fmt.Sprintf("/tv/%d/season/%d/episode/%d", tvID, season, episode),
		languageQuery(language),
		detailsCacheTTL,
		&response,
	)
	return response, err
}

func (client *Client) get(
	ctx context.Context,
	path string,
	query url.Values,
	ttl time.Duration,
	destination any,
) error {
	key := requestCacheKey(path, query)
	if client.cache != nil {
		cached, found, err := client.cache.Get(ctx, key)
		if err != nil {
			return err
		}
		if found {
			if err := json.Unmarshal(cached, destination); err == nil {
				return nil
			}
			if err := client.cache.Delete(ctx, key); err != nil {
				return err
			}
		}
	}
	if client.token == "" {
		return &ClientError{Kind: ErrNotConfigured}
	}

	endpoint, err := url.Parse(client.baseURL + path)
	if err != nil {
		return &ClientError{Kind: ErrUpstreamUnavailable}
	}
	endpoint.RawQuery = query.Encode()
	requestCtx, cancel := context.WithTimeout(ctx, client.timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return &ClientError{Kind: ErrUpstreamUnavailable}
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+client.token)

	response, err := client.httpClient.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(requestCtx.Err(), context.DeadlineExceeded) {
			client.logFailure(ctx, "timeout", 0)
			return &ClientError{Kind: ErrUpstreamTimeout}
		}
		client.logFailure(ctx, "unavailable", 0)
		return &ClientError{Kind: ErrUpstreamUnavailable}
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusTooManyRequests {
		retryAfter := parseRetryAfter(response.Header.Get("Retry-After"), time.Now())
		client.logFailure(ctx, "rate_limited", response.StatusCode)
		return &ClientError{Kind: ErrRateLimited, RetryAfter: retryAfter}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		client.logFailure(ctx, "unavailable", response.StatusCode)
		return &ClientError{Kind: ErrUpstreamUnavailable}
	}

	contents, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || len(contents) > maxResponseBytes || json.Unmarshal(contents, destination) != nil {
		client.logFailure(ctx, "invalid_response", response.StatusCode)
		return &ClientError{Kind: ErrUpstreamUnavailable}
	}
	if client.cache != nil {
		normalized, err := json.Marshal(destination)
		if err != nil {
			return &ClientError{Kind: ErrUpstreamUnavailable}
		}
		if err := client.cache.Put(ctx, key, normalized, ttl); err != nil {
			return err
		}
	}
	return nil
}

func (client *Client) logFailure(ctx context.Context, code string, status int) {
	client.logger.WarnContext(ctx, "tmdb request failed",
		slog.String("code", code),
		slog.Int("status", status),
	)
}

func requestCacheKey(path string, query url.Values) string {
	hash := sha256.Sum256([]byte(path + "?" + query.Encode()))
	return hex.EncodeToString(hash[:])
}

func languageQuery(language string) url.Values {
	return url.Values{"language": {language}}
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if retryAt, err := http.ParseTime(value); err == nil && retryAt.After(now) {
		return retryAt.Sub(now)
	}
	return 0
}
