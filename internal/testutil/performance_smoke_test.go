package testutil_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"video-record/internal/auth"
	"video-record/internal/httpapi"
	"video-record/internal/integrations"
	"video-record/internal/records"
	"video-record/internal/storage"
	syncdomain "video-record/internal/sync"
	"video-record/internal/testutil"
)

const (
	performanceMediaItems    = 10_000
	performanceWatchEvents   = 50_000
	performanceCalendarItems = 301
	performanceLibraryItems  = 100
)

func TestPerformanceSmoke(t *testing.T) {
	if os.Getenv("VIDEO_RECORD_PERF_SMOKE") != "1" {
		t.Skip("run through scripts/perf-smoke.sh")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(ctx, db))
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	password := stringsJoin("Synthetic", "performance", "password")
	seeded, err := testutil.Seed(ctx, db, testutil.SeedOptions{
		Users: 5, MediaItems: performanceMediaItems, WatchEvents: performanceWatchEvents,
		Password: password,
		Now:      now,
	})
	require.NoError(t, err)

	authService := auth.NewService(auth.NewRepository(db), auth.ServiceOptions{})
	session, err := authService.Login(ctx, seeded.Username, password, "performance-smoke")
	require.NoError(t, err)
	recordService := records.NewService(records.NewRepository(db))
	router := httpapi.NewRouter(httpapi.Dependencies{Storage: db, Auth: authService, Records: recordService})

	account, scheduler, setProvider := newPerformanceScheduler(t, ctx, db, seeded.UserIDs[0], now)
	initialProvider := newPerformanceProvider("initial", 0, performanceMediaItems)
	setProvider(initialProvider)
	initialStarted := time.Now()
	initialDone := make(chan schedulerResult, 1)
	go func() {
		processed, runErr := scheduler.RunDue(ctx)
		initialDone <- schedulerResult{processed: processed, err: runErr}
	}()
	select {
	case <-initialProvider.started:
	case <-time.After(15 * time.Second):
		cancel()
		<-initialDone
		t.Fatal("initial sync did not reach provider history")
	}

	calendarP95 := measureHTTPP95(t, router, session.Token,
		"/api/v1/calendar?month=2026-07&timezone=UTC&filter=all",
		"events", performanceCalendarItems, 30)
	libraryP95 := measureHTTPP95(t, router, session.Token,
		"/api/v1/library?status=completed", "items", performanceLibraryItems, 50)
	initialResult := <-initialDone
	initialDuration := time.Since(initialStarted)
	require.NoError(t, initialResult.err)
	require.Equal(t, 1, initialResult.processed)
	require.Less(t, calendarP95, 200*time.Millisecond, "hot local API p95 during sync")
	require.Less(t, libraryP95, 300*time.Millisecond, "library filter p95 during sync")
	require.Less(t, initialDuration, 5*time.Minute, "10,000-item initial sync")
	require.Equal(t, 1, initialProvider.authenticationChecks)
	require.Equal(t, 51, initialProvider.historyCalls)
	assertPerformanceSyncState(t, db, performanceMediaItems, performanceWatchEvents+performanceMediaItems)
	assertPerformanceRun(t, db, account.ID, "10000", syncdomain.RunSummary{
		Fetched: performanceMediaItems, Imported: performanceMediaItems,
	})

	incrementalProvider := newPerformanceProvider("incremental", performanceMediaItems, 100)
	setProvider(incrementalProvider)
	_, err = db.Writer().ExecContext(ctx, `
		UPDATE sync_jobs SET next_run_at = ?
		WHERE account_id = ? AND kind = 'incremental'
	`, now.UnixMilli(), account.ID)
	require.NoError(t, err)
	incrementalStarted := time.Now()
	processed, err := scheduler.RunDue(ctx)
	incrementalDuration := time.Since(incrementalStarted)
	require.NoError(t, err)
	require.Equal(t, 1, processed)
	require.Less(t, incrementalDuration, time.Minute, "100-item incremental sync")
	require.Equal(t, 1, incrementalProvider.authenticationChecks)
	require.Equal(t, 2, incrementalProvider.historyCalls)
	assertPerformanceSyncState(t, db, performanceMediaItems+100, performanceWatchEvents+performanceMediaItems+100)
	assertPerformanceRun(t, db, account.ID, "10100", syncdomain.RunSummary{Fetched: 100, Imported: 100})

	performMeasuredRequest(t, router, session.Token,
		"/api/v1/calendar?month=2026-07&timezone=UTC&filter=all",
		"events", performanceCalendarItems)
	performMeasuredRequest(t, router, session.Token,
		"/api/v1/library?status=completed", "items", performanceLibraryItems)
	t.Logf("calendar_p95=%s library_p95=%s initial_sync=%s incremental_sync=%s",
		calendarP95, libraryP95, initialDuration, incrementalDuration)
}

type schedulerResult struct {
	processed int
	err       error
}

func newPerformanceScheduler(
	t *testing.T,
	ctx context.Context,
	db *storage.DB,
	userID string,
	now time.Time,
) (integrations.Account, *syncdomain.Scheduler, func(integrations.Provider)) {
	t.Helper()
	key := bytes.Repeat([]byte{0x41}, 32)
	credentials := []byte(stringsJoin("synthetic", "performance", "credentials"))
	accounts := integrations.NewAccountRepository(
		db,
		integrations.NewCredentialCipher(key),
		integrations.AccountRepositoryOptions{Now: func() time.Time { return now }},
	)
	account, err := accounts.Create(ctx, integrations.CreateAccountInput{
		UserID: userID, Provider: "jellyfin", Name: "Synthetic performance provider",
		BaseURL: "http://127.0.0.1", Credentials: credentials, Enabled: true,
	})
	require.NoError(t, err)
	candidates := syncdomain.NewCandidateService(db, syncdomain.CandidateServiceOptions{
		Now: func() time.Time { return now },
	})
	var activeProvider integrations.Provider
	runner := syncdomain.NewProviderRunner(
		db,
		accounts,
		candidates,
		syncdomain.ProviderFactoryFunc(func(
			factoryAccount integrations.Account,
			factoryCredentials []byte,
		) (integrations.Provider, error) {
			if factoryAccount.ID != account.ID || !bytes.Equal(factoryCredentials, credentials) || activeProvider == nil {
				return nil, errors.New("invalid synthetic provider factory input")
			}
			return activeProvider, nil
		}),
		syncdomain.ProviderRunnerOptions{
			Now: func() time.Time { return now }, PageLimit: 200,
		},
	)
	service := syncdomain.NewService(db, syncdomain.ServiceOptions{Now: func() time.Time { return now }})
	require.NoError(t, service.EnsureJobs(ctx, account.ID))
	_, err = db.Writer().ExecContext(ctx, `
		UPDATE sync_jobs SET next_run_at = ?
		WHERE account_id = ? AND kind = 'compensation'
	`, now.Add(24*time.Hour).UnixMilli(), account.ID)
	require.NoError(t, err)
	scheduler := syncdomain.NewScheduler(service, runner, syncdomain.SchedulerOptions{
		Owner: "performance-smoke", LeaseDuration: 5 * time.Minute,
	})
	return account, scheduler, func(provider integrations.Provider) { activeProvider = provider }
}

func measureHTTPP95(
	t *testing.T,
	handler http.Handler,
	token, path, collection string,
	expectedItems, samples int,
) time.Duration {
	t.Helper()
	for index := 0; index < 3; index++ {
		performMeasuredRequest(t, handler, token, path, collection, expectedItems)
	}
	durations := make([]time.Duration, 0, samples)
	for index := 0; index < samples; index++ {
		started := time.Now()
		body := performRequest(t, handler, token, path)
		durations = append(durations, time.Since(started))
		requireCollectionCount(t, body, collection, expectedItems)
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	position := (len(durations)*95 + 99) / 100
	return durations[position-1]
}

func performMeasuredRequest(
	t *testing.T,
	handler http.Handler,
	token, path, collection string,
	expectedItems int,
) {
	t.Helper()
	requireCollectionCount(t, performRequest(t, handler, token, path), collection, expectedItems)
}

func performRequest(t *testing.T, handler http.Handler, token, path string) []byte {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "http://example.test"+path, nil)
	request.AddCookie(&http.Cookie{Name: httpapi.SessionCookieName, Value: token})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	return append([]byte(nil), response.Body.Bytes()...)
}

func requireCollectionCount(t *testing.T, body []byte, collection string, expected int) {
	t.Helper()
	var envelope map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &envelope))
	encoded, exists := envelope[collection]
	require.True(t, exists, "missing response collection %q", collection)
	var items []json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &items))
	require.Len(t, items, expected, collection)
}

func assertPerformanceSyncState(t *testing.T, db *storage.DB, candidates, watchEvents int) {
	t.Helper()
	expected := map[string]int{
		"SELECT COUNT(*) FROM sync_candidates":                                          candidates,
		"SELECT COUNT(*) FROM sync_candidates WHERE status = 'confirmed'":               candidates,
		"SELECT COUNT(*) FROM watch_events":                                             watchEvents,
		"SELECT COUNT(*) FROM watch_event_participants":                                 watchEvents,
		"SELECT COUNT(*) FROM external_media_mappings":                                  performanceMediaItems,
		"SELECT COUNT(*) FROM user_media_states WHERE status_source = 'confirmed_sync'": performanceMediaItems,
	}
	for query, count := range expected {
		var actual int
		require.NoError(t, db.Reader().QueryRow(query).Scan(&actual))
		require.Equal(t, count, actual, query)
	}
}

func assertPerformanceRun(
	t *testing.T,
	db *storage.DB,
	accountID, cursor string,
	expected syncdomain.RunSummary,
) {
	t.Helper()
	var status, summaryJSON string
	require.NoError(t, db.Reader().QueryRow(`
		SELECT status, summary_json FROM sync_runs
		WHERE account_id = ? AND cursor = ?
	`, accountID, cursor).Scan(&status, &summaryJSON))
	require.Equal(t, "succeeded", status)
	var summary syncdomain.RunSummary
	require.NoError(t, json.Unmarshal([]byte(summaryJSON), &summary))
	require.Equal(t, expected, summary)
	var jobCursor string
	require.NoError(t, db.Reader().QueryRow(`
		SELECT cursor FROM sync_jobs WHERE account_id = ? AND kind = 'incremental'
	`, accountID).Scan(&jobCursor))
	require.Equal(t, cursor, jobCursor)
}

type performanceProvider struct {
	prefix               string
	startCursor          int
	eventCount           int
	started              chan struct{}
	authenticationChecks int
	historyCalls         int
}

func newPerformanceProvider(prefix string, startCursor, eventCount int) *performanceProvider {
	return &performanceProvider{
		prefix: prefix, startCursor: startCursor, eventCount: eventCount,
		started: make(chan struct{}),
	}
}

func (provider *performanceProvider) CheckAuthentication(context.Context) error {
	provider.authenticationChecks++
	return nil
}

func (provider *performanceProvider) History(
	ctx context.Context,
	request integrations.HistoryRequest,
) (integrations.HistoryPage, error) {
	if err := ctx.Err(); err != nil {
		return integrations.HistoryPage{}, err
	}
	provider.historyCalls++
	if provider.historyCalls == 1 {
		close(provider.started)
	}
	cursor := provider.startCursor
	if request.Cursor != "" {
		parsed, err := strconv.Atoi(request.Cursor)
		if err != nil {
			return integrations.HistoryPage{}, integrations.ErrInvalidHistory
		}
		cursor = parsed
	}
	end := provider.startCursor + provider.eventCount
	if cursor < provider.startCursor || cursor > end || request.Limit < 1 {
		return integrations.HistoryPage{}, integrations.ErrInvalidHistory
	}
	if cursor == end {
		return integrations.HistoryPage{}, nil
	}
	pageEnd := min(cursor+request.Limit, end)
	events := make([]integrations.HistoryEvent, 0, pageEnd-cursor)
	for eventIndex := cursor; eventIndex < pageEnd; eventIndex++ {
		mediaIndex := (eventIndex - provider.startCursor) % performanceMediaItems
		events = append(events, performanceHistoryEvent(provider.prefix, eventIndex, mediaIndex))
	}
	return integrations.HistoryPage{Events: events, NextCursor: strconv.Itoa(pageEnd)}, nil
}

func performanceHistoryEvent(prefix string, eventIndex, mediaIndex int) integrations.HistoryEvent {
	return integrations.HistoryEvent{
		ID:              fmt.Sprintf("%s-event-%05d", prefix, eventIndex),
		PlayedAt:        time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC).Add(time.Duration(eventIndex) * time.Second),
		DurationSeconds: 7_200,
		PositionSeconds: 7_100,
		Item: integrations.ItemIdentity{
			ProviderItemID: fmt.Sprintf("provider-item-%05d", mediaIndex),
			TMDBID:         fmt.Sprintf("%d", mediaIndex+1),
			MediaType:      integrations.MediaMovie,
			Title:          fmt.Sprintf("Synthetic movie %05d", mediaIndex),
			Year:           2000 + mediaIndex%25,
		},
	}
}
