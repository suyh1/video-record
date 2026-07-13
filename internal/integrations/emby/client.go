package emby

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
	Location   *time.Location
}

type Client struct {
	baseURL    string
	token      string
	userID     string
	httpClient *http.Client
	timeout    time.Duration
	now        func() time.Time
	location   *time.Location
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
	location := options.Location
	if location == nil {
		location = time.UTC
	}
	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(options.BaseURL), "/"),
		token:   strings.TrimSpace(options.Token), userID: strings.TrimSpace(options.UserID),
		httpClient: httpClient, timeout: timeout, now: now, location: location,
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
	if user.ID == "" || !strings.EqualFold(user.ID, client.userID) {
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
		endDate = truncateDate(request.Until, client.location)
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
		if sameDate(date, startDate) {
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
				return validatedPage(events, nextCursor)
			}
		}
		if len(events) == 0 && date.Before(endDate) {
			nextCursor = formatCursor(date.AddDate(0, 0, 1), 0)
		}
	}
	return validatedPage(events, nextCursor)
}

func validatedPage(events []integrations.HistoryEvent, cursor string) (integrations.HistoryPage, error) {
	page := integrations.HistoryPage{Events: events, NextCursor: cursor}
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
	query := url.Values{"Filter": {"Movie,Episode"}}
	var response historyResponse
	if err := client.getJSON(ctx, path, query, false, &response); err != nil {
		return nil, err
	}
	if !strings.EqualFold(response.UserID, client.userID) || response.Activity == nil {
		return nil, invalidResponse()
	}
	for index := range response.Activity {
		rowID, err := strconv.ParseInt(response.Activity[index].RowID, 10, 64)
		if err != nil || rowID <= 0 || response.Activity[index].ItemID == "" {
			return nil, invalidResponse()
		}
		response.Activity[index].numericRowID = rowID
		response.Activity[index].historyDate = date
	}
	return response.Activity, nil
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
	if item.ID == "" || !strings.EqualFold(item.ID, itemID) {
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
		date, rowID, err := parseCursor(request.Cursor, client.location)
		if err != nil {
			return time.Time{}, 0, invalidResponse()
		}
		return date, rowID, nil
	}
	if !request.Since.IsZero() {
		return truncateDate(request.Since, client.location), 0, nil
	}
	return truncateDate(client.now(), client.location), 0, nil
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

func parseCursor(value string, location *time.Location) (time.Time, int64, error) {
	parts := strings.Split(value, "|")
	if len(parts) != 2 {
		return time.Time{}, 0, errors.New("invalid cursor")
	}
	date, err := time.ParseInLocation(dateLayout, parts[0], location)
	if err != nil {
		return time.Time{}, 0, err
	}
	rowID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || rowID < 0 {
		return time.Time{}, 0, errors.New("invalid cursor")
	}
	return date, rowID, nil
}

func formatCursor(date time.Time, rowID int64) string {
	return date.Format(dateLayout) + "|" + strconv.FormatInt(rowID, 10)
}

func truncateDate(value time.Time, location *time.Location) time.Time {
	local := value.In(location)
	year, month, day := local.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, location)
}

func sameDate(left, right time.Time) bool {
	leftYear, leftMonth, leftDay := left.Date()
	rightYear, rightMonth, rightDay := right.Date()
	return leftYear == rightYear && leftMonth == rightMonth && leftDay == rightDay
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
