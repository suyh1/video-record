package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"video-record/internal/auth"
)

func TestIdempotencyReplaysOriginalEventResponse(t *testing.T) {
	router, cookie, csrfToken, mediaID, recordService, _ := newRecordsTestRouter(t)
	baseHeaders := map[string]string{
		"Cookie":          cookie.String(),
		"Origin":          "http://example.test",
		"X-CSRF-Token":    csrfToken,
		"Idempotency-Key": "complete-record",
		"If-Match":        `"0"`,
	}
	completed := performJSONRequest(router, http.MethodPut, "http://example.test/api/v1/records/"+mediaID, map[string]any{
		"status": "completed",
	}, baseHeaders)
	require.Equal(t, http.StatusOK, completed.Code)

	eventHeaders := cloneHeaders(baseHeaders)
	delete(eventHeaders, "If-Match")
	eventHeaders["Idempotency-Key"] = "rewatch-2026-07-13"
	body := map[string]any{
		"watchedAt":     "2026-07-13T20:30:00Z",
		"viewingMethod": "家庭影院",
	}
	first := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/records/"+mediaID+"/events", body, eventHeaders)
	require.Equal(t, http.StatusCreated, first.Code)
	require.Empty(t, first.Header().Get("Idempotency-Replayed"))

	second := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/records/"+mediaID+"/events", body, eventHeaders)
	require.Equal(t, first.Code, second.Code)
	require.Equal(t, first.Body.String(), second.Body.String())
	require.Equal(t, "true", second.Header().Get("Idempotency-Replayed"))

	events, err := recordService.WatchEvents(context.Background(), currentUserID(t, router, cookie), mediaID)
	require.NoError(t, err)
	require.Len(t, events, 2)
}

func TestIdempotencyRejectsMissingOrConflictingKeys(t *testing.T) {
	router, cookie, csrfToken, mediaID, _, _ := newRecordsTestRouter(t)
	completeRecordForIdempotencyTest(t, router, cookie, csrfToken, mediaID)
	headers := map[string]string{
		"Cookie": cookie.String(), "Origin": "http://example.test", "X-CSRF-Token": csrfToken,
	}
	body := map[string]any{"watchedAt": "2026-07-13T20:30:00Z"}
	missing := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/records/"+mediaID+"/events", body, headers)
	require.Equal(t, http.StatusBadRequest, missing.Code)
	require.Contains(t, missing.Body.String(), `"code":"invalid_idempotency_key"`)

	headers["Idempotency-Key"] = "one-logical-request"
	first := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/records/"+mediaID+"/events", body, headers)
	require.Equal(t, http.StatusCreated, first.Code)
	conflict := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/records/"+mediaID+"/events", map[string]any{
		"watchedAt": "2026-07-14T20:30:00Z",
	}, headers)
	require.Equal(t, http.StatusConflict, conflict.Code)
	require.Contains(t, conflict.Body.String(), `"code":"idempotency_key_conflict"`)
}

func TestIdempotencyIncludesQueryScope(t *testing.T) {
	router, cookie, csrfToken, _, _, _ := newRecordsTestRouter(t)
	headers := map[string]string{
		"Cookie": cookie.String(), "Origin": "http://example.test",
		"X-CSRF-Token": csrfToken, "Idempotency-Key": "query-scope-key",
	}
	body := map[string]any{"action": "next", "expectedVersion": 0}
	first := performJSONRequest(
		router, http.MethodPost,
		"http://example.test/api/v1/records/missing/progress?seasonNumber=1",
		body, headers,
	)
	require.NotEqual(t, http.StatusConflict, first.Code)
	second := performJSONRequest(
		router, http.MethodPost,
		"http://example.test/api/v1/records/missing/progress?seasonNumber=2",
		body, headers,
	)
	require.Equal(t, http.StatusConflict, second.Code)
	require.Contains(t, second.Body.String(), `"code":"idempotency_key_conflict"`)
}

func TestIdempotencyExpiresStoredResponses(t *testing.T) {
	router, cookie, csrfToken, mediaID, recordService, db := newRecordsTestRouter(t)
	userID := currentUserID(t, router, cookie)
	completeRecordForIdempotencyTest(t, router, cookie, csrfToken, mediaID)
	_, err := db.Writer().ExecContext(context.Background(), `
		INSERT INTO idempotency_keys (
			user_id, key, method, path, request_hash, status_code,
			content_type, etag, response_body, created_at, expires_at
		) VALUES (?, 'expired-key', 'POST', '/expired', 'old', 418, 'text/plain', '', ?, 0, 0)
	`, userID, []byte("old"))
	require.NoError(t, err)

	headers := map[string]string{
		"Cookie": cookie.String(), "Origin": "http://example.test", "X-CSRF-Token": csrfToken,
		"Idempotency-Key": "expired-key",
	}
	response := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/records/"+mediaID+"/events", map[string]any{
		"watchedAt": "2026-07-15T20:30:00Z",
	}, headers)
	require.Equal(t, http.StatusCreated, response.Code)
	events, err := recordService.WatchEvents(context.Background(), userID, mediaID)
	require.NoError(t, err)
	require.Len(t, events, 2)
}

func TestIdempotencyReplaysNoContentResponse(t *testing.T) {
	router, cookie, csrfToken, mediaID, _, _ := newRecordsTestRouter(t)
	completeRecordForIdempotencyTest(t, router, cookie, csrfToken, mediaID)
	headers := map[string]string{
		"Cookie":          cookie.String(),
		"Origin":          "http://example.test",
		"X-CSRF-Token":    csrfToken,
		"Idempotency-Key": "set-tags-no-content",
		"If-Match":        `"1"`,
	}
	body := map[string]any{"tags": []string{"科幻"}}
	target := "http://example.test/api/v1/records/" + mediaID + "/tags"

	first := performJSONRequest(router, http.MethodPut, target, body, headers)
	require.Equal(t, http.StatusNoContent, first.Code)
	require.Empty(t, first.Body.String())

	replayed := performJSONRequest(router, http.MethodPut, target, body, headers)
	require.Equal(t, http.StatusNoContent, replayed.Code)
	require.Equal(t, "true", replayed.Header().Get("Idempotency-Replayed"))
	require.Empty(t, replayed.Body.String())
}

func TestIdempotencyReservesKeyBeforeSideEffects(t *testing.T) {
	router, cookie, csrfToken, mediaID, _, db := newRecordsTestRouter(t)
	_, err := db.Writer().ExecContext(context.Background(), `
		CREATE TRIGGER fail_idempotency_completion
		BEFORE UPDATE OF status_code ON idempotency_keys
		WHEN NEW.key = 'completion-failure'
		BEGIN
			SELECT RAISE(FAIL, 'synthetic completion failure');
		END
	`)
	require.NoError(t, err)
	headers := map[string]string{
		"Cookie":          cookie.String(),
		"Origin":          "http://example.test",
		"X-CSRF-Token":    csrfToken,
		"Idempotency-Key": "completion-failure",
		"If-Match":        `"0"`,
	}
	target := "http://example.test/api/v1/records/" + mediaID
	body := map[string]any{"status": "wishlist"}

	first := performJSONRequest(router, http.MethodPut, target, body, headers)
	require.Equal(t, http.StatusInternalServerError, first.Code)

	retry := performJSONRequest(router, http.MethodPut, target, body, headers)
	require.Equal(t, http.StatusConflict, retry.Code)
	require.Contains(t, retry.Body.String(), `"code":"idempotency_in_progress"`)

	read := performJSONRequest(router, http.MethodGet, target, nil, map[string]string{"Cookie": cookie.String()})
	require.Equal(t, http.StatusOK, read.Code)
	require.Contains(t, read.Body.String(), `"status":"wishlist"`)
	require.Contains(t, read.Body.String(), `"version":1`)
}

func TestIdempotencyReservationIsSharedAcrossMiddlewareInstances(t *testing.T) {
	router, cookie, _, _, _, db := newRecordsTestRouter(t)
	userID := currentUserID(t, router, cookie)
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			close(started)
			<-release
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"created":true}`))
	})
	firstMiddleware := newIdempotencyMiddleware(db).Handle(handler)
	secondMiddleware := newIdempotencyMiddleware(db).Handle(handler)
	request := func() *http.Request {
		req := httptest.NewRequest(http.MethodPost, "http://example.test/api/v1/test", strings.NewReader(`{"value":1}`))
		req.Header.Set("Idempotency-Key", "shared-reservation")
		identity := auth.Identity{User: auth.User{ID: userID, Active: true}}
		return req.WithContext(context.WithValue(req.Context(), identityContextKey{}, identity))
	}

	firstResult := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		recorder := httptest.NewRecorder()
		firstMiddleware.ServeHTTP(recorder, request())
		firstResult <- recorder
	}()
	<-started
	second := httptest.NewRecorder()
	secondMiddleware.ServeHTTP(second, request())
	require.Equal(t, http.StatusConflict, second.Code)
	require.Contains(t, second.Body.String(), `"code":"idempotency_in_progress"`)
	require.EqualValues(t, 1, calls.Load())

	close(release)
	first := <-firstResult
	require.Equal(t, http.StatusCreated, first.Code)
	replayed := httptest.NewRecorder()
	secondMiddleware.ServeHTTP(replayed, request())
	require.Equal(t, http.StatusCreated, replayed.Code)
	require.Equal(t, "true", replayed.Header().Get("Idempotency-Replayed"))
	require.EqualValues(t, 1, calls.Load())
}

func TestIdempotencyReplayKeepsProblemRequestIDConsistent(t *testing.T) {
	router, cookie, csrfToken, mediaID, _, _ := newRecordsTestRouter(t)
	completeRecordForIdempotencyTest(t, router, cookie, csrfToken, mediaID)
	headers := map[string]string{
		"Cookie":          cookie.String(),
		"Origin":          "http://example.test",
		"X-CSRF-Token":    csrfToken,
		"Idempotency-Key": "cached-version-conflict",
		"If-Match":        `"0"`,
	}
	target := "http://example.test/api/v1/records/" + mediaID
	body := map[string]any{"status": "wishlist"}

	first := performJSONRequest(router, http.MethodPut, target, body, headers)
	require.Equal(t, http.StatusConflict, first.Code)
	replayed := performJSONRequest(router, http.MethodPut, target, body, headers)
	require.Equal(t, http.StatusConflict, replayed.Code)
	require.Equal(t, first.Body.String(), replayed.Body.String())
	require.Equal(t, first.Header().Get(RequestIDHeader), replayed.Header().Get(RequestIDHeader))
	require.Equal(t, "true", replayed.Header().Get("Idempotency-Replayed"))
}

func completeRecordForIdempotencyTest(
	t *testing.T,
	router http.Handler,
	cookie *http.Cookie,
	csrfToken, mediaID string,
) {
	t.Helper()
	completed := performJSONRequest(router, http.MethodPut, "http://example.test/api/v1/records/"+mediaID, map[string]any{
		"status": "completed",
	}, map[string]string{
		"Cookie": cookie.String(), "Origin": "http://example.test", "X-CSRF-Token": csrfToken,
		"Idempotency-Key": "complete-record",
		"If-Match":        `"0"`,
	})
	require.Equal(t, http.StatusOK, completed.Code)
}

func currentUserID(t *testing.T, router http.Handler, cookie *http.Cookie) string {
	t.Helper()
	response := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/auth/me", nil, map[string]string{
		"Cookie": cookie.String(),
	})
	require.Equal(t, http.StatusOK, response.Code)
	var body struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	return body.ID
}
