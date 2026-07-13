package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIdempotencyReplaysOriginalEventResponse(t *testing.T) {
	router, cookie, csrfToken, mediaID, recordService, _ := newRecordsTestRouter(t)
	baseHeaders := map[string]string{
		"Cookie":       cookie.String(),
		"Origin":       "http://example.test",
		"X-CSRF-Token": csrfToken,
		"If-Match":     `"0"`,
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
		"If-Match": `"0"`,
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
