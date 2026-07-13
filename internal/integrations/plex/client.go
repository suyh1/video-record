package plex

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"video-record/internal/integrations"
)

const (
	defaultTimeout   = 8 * time.Second
	defaultPageLimit = 100
	maxPageLimit     = 200
	maxResponseBytes = 5 << 20
)

type ClientOptions struct {
	BaseURL    string
	Token      string
	AccountID  int
	HTTPClient *http.Client
	Timeout    time.Duration
}

type Client struct {
	baseURL    string
	token      string
	accountID  int
	httpClient *http.Client
	timeout    time.Duration
}

func NewClient(options ClientOptions) *Client {
	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(options.BaseURL), "/"),
		token:   strings.TrimSpace(options.Token), accountID: options.AccountID,
		httpClient: httpClient, timeout: timeout,
	}
}

func (client *Client) CheckAuthentication(ctx context.Context) error {
	if err := client.configured(); err != nil {
		return err
	}
	contents, contentType, err := client.get(ctx, "/library/sections", nil, true)
	if err != nil {
		return err
	}
	if err := validateEnvelope(contents, contentType); err != nil {
		return invalidResponse()
	}
	return nil
}

func (client *Client) History(
	ctx context.Context,
	request integrations.HistoryRequest,
) (integrations.HistoryPage, error) {
	if err := ctx.Err(); err != nil {
		return integrations.HistoryPage{}, err
	}
	if err := client.configured(); err != nil {
		return integrations.HistoryPage{}, err
	}
	offset, err := parseCursor(request.Cursor)
	if err != nil {
		return integrations.HistoryPage{}, invalidResponse()
	}
	if !request.Since.IsZero() && !request.Until.IsZero() && request.Until.Before(request.Since) {
		return integrations.HistoryPage{}, invalidResponse()
	}
	limit := request.Limit
	if limit <= 0 {
		limit = defaultPageLimit
	}
	if limit > maxPageLimit {
		limit = maxPageLimit
	}
	query := url.Values{
		"accountID":              {strconv.Itoa(client.accountID)},
		"sort":                   {"viewedAt:asc"},
		"includeGuids":           {"1"},
		"X-Plex-Container-Start": {strconv.Itoa(offset)},
		"X-Plex-Container-Size":  {strconv.Itoa(limit)},
	}
	if !request.Since.IsZero() {
		query.Set("viewedAt>=", strconv.FormatInt(request.Since.UTC().Unix(), 10))
	}
	if !request.Until.IsZero() {
		query.Set("viewedAt<=", strconv.FormatInt(request.Until.UTC().Unix(), 10))
	}
	contents, contentType, err := client.get(
		ctx, "/status/sessions/history/all", query, false,
	)
	if err != nil {
		return integrations.HistoryPage{}, err
	}
	container, err := decodeHistory(contents, contentType)
	if err != nil || container.Offset != offset || container.Size != len(container.Metadata) ||
		container.Size > limit || container.TotalSize < offset+container.Size {
		return integrations.HistoryPage{}, invalidResponse()
	}
	events := make([]integrations.HistoryEvent, 0, len(container.Metadata))
	for _, video := range container.Metadata {
		event, err := mapHistoryEvent(video)
		if err != nil {
			return integrations.HistoryPage{}, invalidResponse()
		}
		events = append(events, event)
	}
	page := integrations.HistoryPage{
		Events: events, NextCursor: strconv.Itoa(offset + len(events)),
	}
	if err := integrations.ValidateHistoryPage(page); err != nil {
		return integrations.HistoryPage{}, invalidResponse()
	}
	return page, nil
}

func (client *Client) configured() error {
	if client.baseURL == "" || client.token == "" || client.accountID <= 0 {
		return integrations.NewProviderError(integrations.ErrorAuthentication, 0, nil)
	}
	return nil
}

func (client *Client) get(
	ctx context.Context,
	path string,
	query url.Values,
	authenticationCheck bool,
) ([]byte, string, error) {
	endpoint, err := url.Parse(client.baseURL + path)
	if err != nil {
		return nil, "", invalidResponse()
	}
	endpoint.RawQuery = query.Encode()
	requestCtx, cancel := context.WithTimeout(ctx, client.timeout)
	defer cancel()
	httpRequest, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, "", invalidResponse()
	}
	httpRequest.Header.Set("Accept", "application/json, application/xml;q=0.9")
	httpRequest.Header.Set("X-Plex-Token", client.token)
	httpRequest.Header.Set("X-Plex-Product", "video-record")
	httpRequest.Header.Set("X-Plex-Client-Identifier", "video-record-server")
	httpRequest.Header.Set("X-Plex-Version", "1")
	response, err := client.httpClient.Do(httpRequest)
	if err != nil {
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		return nil, "", integrations.NewProviderError(integrations.ErrorUnavailable, 0, nil)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusTooManyRequests {
		return nil, "", integrations.NewProviderError(
			integrations.ErrorRateLimited,
			parseRetryAfter(response.Header.Get("Retry-After"), time.Now()),
			nil,
		)
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden ||
		(authenticationCheck && response.StatusCode == http.StatusNotFound) {
		return nil, "", integrations.NewProviderError(integrations.ErrorAuthentication, 0, nil)
	}
	if response.StatusCode >= http.StatusInternalServerError {
		return nil, "", integrations.NewProviderError(integrations.ErrorUnavailable, 0, nil)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, "", invalidResponse()
	}
	contents, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || len(contents) == 0 || len(contents) > maxResponseBytes {
		return nil, "", invalidResponse()
	}
	return contents, response.Header.Get("Content-Type"), nil
}

func validateEnvelope(contents []byte, contentType string) error {
	format := responseFormat(contents, contentType)
	switch format {
	case "xml":
		var envelope struct {
			XMLName xml.Name `xml:"MediaContainer"`
		}
		return xml.Unmarshal(contents, &envelope)
	case "json":
		var envelope struct {
			MediaContainer json.RawMessage `json:"MediaContainer"`
		}
		if err := json.Unmarshal(contents, &envelope); err != nil ||
			len(envelope.MediaContainer) == 0 || bytes.Equal(envelope.MediaContainer, []byte("null")) {
			return errors.New("invalid plex envelope")
		}
		return nil
	default:
		return errors.New("invalid plex response format")
	}
}

func decodeHistory(contents []byte, contentType string) (historyContainer, error) {
	switch responseFormat(contents, contentType) {
	case "xml":
		var container historyContainer
		if err := xml.Unmarshal(contents, &container); err != nil {
			return historyContainer{}, err
		}
		return container, nil
	case "json":
		var response struct {
			MediaContainer *historyContainer `json:"MediaContainer"`
		}
		if err := json.Unmarshal(contents, &response); err != nil || response.MediaContainer == nil {
			return historyContainer{}, errors.New("invalid plex history")
		}
		return *response.MediaContainer, nil
	default:
		return historyContainer{}, errors.New("invalid plex response format")
	}
}

func responseFormat(contents []byte, contentType string) string {
	trimmed := bytes.TrimSpace(contents)
	if strings.Contains(strings.ToLower(contentType), "json") ||
		(len(trimmed) > 0 && trimmed[0] == '{') {
		return "json"
	}
	if strings.Contains(strings.ToLower(contentType), "xml") ||
		(len(trimmed) > 0 && trimmed[0] == '<') {
		return "xml"
	}
	return ""
}

func parseCursor(value string) (int, error) {
	if value == "" {
		return 0, nil
	}
	offset, err := strconv.Atoi(value)
	if err != nil || offset < 0 {
		return 0, errors.New("invalid cursor")
	}
	return offset, nil
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

func invalidResponse() error {
	return integrations.NewProviderError(integrations.ErrorInvalidResponse, 0, nil)
}
