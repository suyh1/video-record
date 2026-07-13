package jellyfin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
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
	dateLayout       = "2006-01-02"
)

type ClientOptions struct {
	BaseURL    string
	Token      string
	UserID     string
	HTTPClient *http.Client
	Timeout    time.Duration
	Now        func() time.Time
}

type Client struct {
	baseURL    string
	token      string
	userID     string
	httpClient *http.Client
	timeout    time.Duration
	now        func() time.Time
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
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(options.BaseURL), "/"),
		token:   strings.TrimSpace(options.Token), userID: strings.TrimSpace(options.UserID),
		httpClient: httpClient, timeout: timeout, now: now,
	}
}

func (client *Client) CheckAuthentication(ctx context.Context) error {
	if err := client.configured(); err != nil {
		return err
	}
	var user userResponse
	if err := client.getJSON(
		ctx, "/Users/"+url.PathEscape(client.userID), nil, true, &user,
	); err != nil {
		return err
	}
	if user.ID == "" || user.ID != client.userID {
		return integrations.NewProviderError(integrations.ErrorInvalidResponse, 0, nil)
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
	limit := request.Limit
	if limit <= 0 {
		limit = defaultPageLimit
	}
	if limit > maxPageLimit {
		limit = maxPageLimit
	}
	startDate, startRowID, err := client.historyStart(request)
	if err != nil {
		return integrations.HistoryPage{}, err
	}
	endDate := startDate
	if !request.Until.IsZero() {
		endDate = truncateDate(request.Until)
	}
	if endDate.Before(startDate) {
		return integrations.HistoryPage{}, invalidResponse()
	}

	events := make([]integrations.HistoryEvent, 0, limit)
	itemCache := make(map[string]itemResponse)
	nextCursor := request.Cursor
	for date := startDate; !date.After(endDate); date = date.AddDate(0, 0, 1) {
		rows, err := client.activityRows(ctx, date)
		if err != nil {
			return integrations.HistoryPage{}, err
		}
		sort.Slice(rows, func(left, right int) bool {
			return rows[left].numericRowID < rows[right].numericRowID
		})
		minimumRowID := int64(0)
		if date.Equal(startDate) {
			minimumRowID = startRowID
		}
		for _, row := range rows {
			if row.numericRowID <= minimumRowID {
				continue
			}
			item, found := itemCache[row.ItemID]
			if !found {
				item, err = client.item(ctx, row.ItemID)
				if err != nil {
					return integrations.HistoryPage{}, err
				}
				itemCache[row.ItemID] = item
			}
			event, err := mapHistoryEvent(row, item)
			if err != nil {
				return integrations.HistoryPage{}, invalidResponse()
			}
			events = append(events, event)
			nextCursor = formatCursor(date, row.numericRowID)
			if len(events) == limit {
				page := integrations.HistoryPage{Events: events, NextCursor: nextCursor}
				if err := integrations.ValidateHistoryPage(page); err != nil {
					return integrations.HistoryPage{}, invalidResponse()
				}
				return page, nil
			}
		}
		if len(events) == 0 && date.Before(endDate) {
			nextCursor = formatCursor(date.AddDate(0, 0, 1), 0)
		}
	}
	page := integrations.HistoryPage{Events: events, NextCursor: nextCursor}
	if err := integrations.ValidateHistoryPage(page); err != nil {
		return integrations.HistoryPage{}, invalidResponse()
	}
	return page, nil
}

func (client *Client) activityRows(ctx context.Context, date time.Time) ([]activityRow, error) {
	path := fmt.Sprintf(
		"/user_usage_stats/%s/%s/GetItems",
		url.PathEscape(client.userID), date.Format(dateLayout),
	)
	query := url.Values{"filter": {"Movie,Episode"}, "timezoneOffset": {"0"}}
	var rows []activityRow
	if err := client.getJSON(ctx, path, query, false, &rows); err != nil {
		return nil, err
	}
	if rows == nil {
		return nil, invalidResponse()
	}
	for index := range rows {
		rowID, err := strconv.ParseInt(rows[index].RowID, 10, 64)
		if err != nil || rowID <= 0 || rows[index].ItemID == "" {
			return nil, invalidResponse()
		}
		rows[index].numericRowID = rowID
		rows[index].historyDate = date
	}
	return rows, nil
}

func (client *Client) item(ctx context.Context, itemID string) (itemResponse, error) {
	query := url.Values{
		"UserId": {client.userID},
		"Fields": {"ProviderIds,OriginalTitle,ProductionYear,RunTimeTicks,ParentIndexNumber,IndexNumber,SeriesName"},
	}
	var item itemResponse
	if err := client.getJSON(ctx, "/Items/"+url.PathEscape(itemID), query, false, &item); err != nil {
		return itemResponse{}, err
	}
	if item.ID == "" || item.ID != itemID {
		return itemResponse{}, invalidResponse()
	}
	return item, nil
}

func (client *Client) configured() error {
	if client.baseURL == "" || client.token == "" || client.userID == "" {
		return integrations.NewProviderError(integrations.ErrorAuthentication, 0, nil)
	}
	return nil
}

func (client *Client) historyStart(request integrations.HistoryRequest) (time.Time, int64, error) {
	if request.Cursor != "" {
		date, rowID, err := parseCursor(request.Cursor)
		if err != nil {
			return time.Time{}, 0, invalidResponse()
		}
		return date, rowID, nil
	}
	if !request.Since.IsZero() {
		return truncateDate(request.Since), 0, nil
	}
	return truncateDate(client.now()), 0, nil
}

func (client *Client) getJSON(
	ctx context.Context,
	path string,
	query url.Values,
	authenticationCheck bool,
	destination any,
) error {
	endpoint, err := url.Parse(client.baseURL + path)
	if err != nil {
		return invalidResponse()
	}
	endpoint.RawQuery = query.Encode()
	requestCtx, cancel := context.WithTimeout(ctx, client.timeout)
	defer cancel()
	httpRequest, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return invalidResponse()
	}
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("X-Emby-Token", client.token)
	response, err := client.httpClient.Do(httpRequest)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(requestCtx.Err(), context.DeadlineExceeded) {
			return integrations.NewProviderError(integrations.ErrorUnavailable, 0, nil)
		}
		return integrations.NewProviderError(integrations.ErrorUnavailable, 0, nil)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusTooManyRequests {
		return integrations.NewProviderError(
			integrations.ErrorRateLimited,
			parseRetryAfter(response.Header.Get("Retry-After"), time.Now()),
			nil,
		)
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden ||
		(authenticationCheck && response.StatusCode == http.StatusNotFound) {
		return integrations.NewProviderError(integrations.ErrorAuthentication, 0, nil)
	}
	if response.StatusCode >= http.StatusInternalServerError {
		return integrations.NewProviderError(integrations.ErrorUnavailable, 0, nil)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return invalidResponse()
	}
	contents, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || len(contents) > maxResponseBytes || json.Unmarshal(contents, destination) != nil {
		return invalidResponse()
	}
	return nil
}

func parseCursor(value string) (time.Time, int64, error) {
	parts := strings.Split(value, "|")
	if len(parts) != 2 {
		return time.Time{}, 0, errors.New("invalid cursor")
	}
	date, err := time.Parse(dateLayout, parts[0])
	if err != nil {
		return time.Time{}, 0, err
	}
	rowID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || rowID < 0 {
		return time.Time{}, 0, errors.New("invalid cursor")
	}
	return date.UTC(), rowID, nil
}

func formatCursor(date time.Time, rowID int64) string {
	return date.UTC().Format(dateLayout) + "|" + strconv.FormatInt(rowID, 10)
}

func truncateDate(value time.Time) time.Time {
	year, month, day := value.UTC().Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
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
