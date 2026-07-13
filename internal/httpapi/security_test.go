package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"video-record/internal/household"
)

func TestSecurityProblemDetailsETagIdempotencyCSRFAndSessionRevocation(t *testing.T) {
	router, db := newFullContractRouter(t)
	insertSyncHTTPMedia(t, db, "security-media", "movie", "Security Film", "2026")

	unauthorized := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/sync/status", nil, nil)
	require.Equal(t, http.StatusUnauthorized, unauthorized.Code)
	require.Equal(t, "application/problem+json", unauthorized.Header().Get("Content-Type"))
	require.NotEmpty(t, unauthorized.Header().Get(RequestIDHeader))
	require.JSONEq(t, `{
		"type":"about:blank","title":"Unauthorized","status":401,
		"code":"unauthenticated","requestId":"`+unauthorized.Header().Get(RequestIDHeader)+`"
	}`, unauthorized.Body.String())

	cookie, csrfToken := loginForHTTPTest(t, router)
	baseHeaders := map[string]string{
		"Cookie": cookie.String(), "Origin": "http://example.test",
		"X-CSRF-Token": csrfToken, "If-Match": `"0"`,
		"Idempotency-Key": "security-record-update",
	}
	first := performJSONRequest(router, http.MethodPut,
		"http://example.test/api/v1/records/security-media", map[string]any{"status": "wishlist"}, baseHeaders)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	require.Equal(t, `"1"`, first.Header().Get("ETag"))
	library := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/library", nil, map[string]string{
		"Cookie": cookie.String(),
	})
	require.Equal(t, http.StatusOK, library.Code)
	var libraryPage map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(library.Body.Bytes(), &libraryPage))
	require.Contains(t, libraryPage, "items")
	require.Contains(t, libraryPage, "nextCursor")
	replayed := performJSONRequest(router, http.MethodPut,
		"http://example.test/api/v1/records/security-media", map[string]any{"status": "wishlist"}, baseHeaders)
	require.Equal(t, http.StatusOK, replayed.Code)
	require.Equal(t, "true", replayed.Header().Get("Idempotency-Replayed"))
	require.Equal(t, first.Body.String(), replayed.Body.String())
	require.Equal(t, first.Header().Get("ETag"), replayed.Header().Get("ETag"))

	missingCSRF := performJSONRequest(router, http.MethodPost,
		"http://example.test/api/v1/media/custom", map[string]any{"title": "Blocked", "mediaType": "movie"},
		map[string]string{
			"Cookie": cookie.String(), "Origin": "http://example.test", "Idempotency-Key": "missing-csrf",
		})
	require.Equal(t, http.StatusForbidden, missingCSRF.Code)
	missingKey := performJSONRequest(router, http.MethodPost,
		"http://example.test/api/v1/media/custom", map[string]any{"title": "Blocked", "mediaType": "movie"},
		map[string]string{
			"Cookie": cookie.String(), "Origin": "http://example.test", "X-CSRF-Token": csrfToken,
		})
	require.Equal(t, http.StatusBadRequest, missingKey.Code)
	require.Contains(t, missingKey.Body.String(), `"code":"invalid_idempotency_key"`)

	logout := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/auth/logout", nil, map[string]string{
		"Cookie": cookie.String(), "Origin": "http://example.test", "X-CSRF-Token": csrfToken,
	})
	require.Equal(t, http.StatusNoContent, logout.Code)
	revoked := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/auth/me", nil, map[string]string{
		"Cookie": cookie.String(),
	})
	require.Equal(t, http.StatusUnauthorized, revoked.Code)
}

func TestSecurityRequiresIdempotencyKeysOnProtectedWrites(t *testing.T) {
	router, _ := newFullContractRouter(t)
	cookie, csrfToken := loginForHTTPTest(t, router)
	headers := map[string]string{
		"Cookie": cookie.String(), "Origin": "http://example.test", "X-CSRF-Token": csrfToken,
	}
	for _, route := range protectedWriteRoutes {
		path := concreteSecurityPath(route.Path)
		t.Run(route.Method+" "+path, func(t *testing.T) {
			response := performJSONRequest(
				router, route.Method, "http://example.test"+path, map[string]any{}, headers,
			)
			require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
			require.Contains(t, response.Body.String(), `"code":"invalid_idempotency_key"`)
		})
	}
}

func TestSecurityEnforcesObjectAuthorizationAndRedactsLogs(t *testing.T) {
	ctx := context.Background()
	db, authService, syncService, adminID, _ := newSyncHTTPServices(t)
	householdService := household.NewService(household.NewRepository(db))
	member, err := householdService.CreateMember(ctx, adminID, "private-owner", "correct horse battery staple")
	require.NoError(t, err)
	accountID := insertSyncHTTPAccount(t, db, member.ID, "private-account", "emby")
	candidate, err := syncService.Ingest(ctx, accountID, syncHTTPMovieEvent("private-event", "Private Candidate"))
	require.NoError(t, err)
	router := NewRouter(Dependencies{Storage: db, Auth: authService, Sync: syncService})
	cookie, csrfToken := loginForHTTPTest(t, router)

	list := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/sync/candidates", nil, map[string]string{
		"Cookie": cookie.String(),
	})
	require.Equal(t, http.StatusOK, list.Code)
	require.NotContains(t, list.Body.String(), candidate.ID)
	mutation := performJSONRequest(router, http.MethodPost,
		"http://example.test/api/v1/sync/candidates/"+candidate.ID+"/ignore", map[string]any{}, map[string]string{
			"Cookie": cookie.String(), "Origin": "http://example.test",
			"X-CSRF-Token": csrfToken, "Idempotency-Key": "cross-user-candidate",
		})
	require.Equal(t, http.StatusNotFound, mutation.Code)

	const secret = "synthetic-security-secret"
	var logs bytes.Buffer
	logger := NewLogger("production", &logs, secret)
	logger.Info("security request",
		slog.String("Authorization", "Bearer "+secret),
		slog.String("providerToken", secret),
	)
	require.NotContains(t, logs.String(), secret)
	require.Contains(t, logs.String(), "[REDACTED]")
}

func TestSecurityReturnsProblemDetailsForAPIRoutingErrors(t *testing.T) {
	router, _ := newFullContractRouter(t)
	for _, testCase := range []struct {
		name   string
		method string
		path   string
		status int
		code   string
	}{
		{name: "not found", method: http.MethodGet, path: "/api/v1/not-a-route", status: http.StatusNotFound, code: "route_not_found"},
		{name: "method not allowed", method: http.MethodPatch, path: "/api/v1/setup/status", status: http.StatusMethodNotAllowed, code: "method_not_allowed"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			response := performJSONRequest(router, testCase.method, "http://example.test"+testCase.path, nil, nil)
			require.Equal(t, testCase.status, response.Code)
			require.Equal(t, "application/problem+json", response.Header().Get("Content-Type"))
			require.NotEmpty(t, response.Header().Get(RequestIDHeader))
			require.Contains(t, response.Body.String(), `"code":"`+testCase.code+`"`)
			require.Contains(t, response.Body.String(), `"requestId":"`+response.Header().Get(RequestIDHeader)+`"`)
		})
	}
}

func TestSecurityIsolatesRecordsCollectionsAndEventsAcrossUsers(t *testing.T) {
	router, ownerCookie, ownerCSRF, mediaID, _, db := newRecordsTestRouter(t)
	ownerID := currentUserID(t, router, ownerCookie)
	householdService := household.NewService(household.NewRepository(db))
	_, err := householdService.CreateMember(
		context.Background(), ownerID, "isolated-member", "correct horse battery staple",
	)
	require.NoError(t, err)
	completeRecordForIdempotencyTest(t, router, ownerCookie, ownerCSRF, mediaID)
	collection := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/collections",
		map[string]string{"name": "Owner Private Collection"}, map[string]string{
			"Cookie": ownerCookie.String(), "Origin": "http://example.test",
			"X-CSRF-Token": ownerCSRF, "Idempotency-Key": "owner-private-collection",
		})
	require.Equal(t, http.StatusCreated, collection.Code)
	ownerEvents := performJSONRequest(router, http.MethodGet,
		"http://example.test/api/v1/records/"+mediaID+"/events", nil,
		map[string]string{"Cookie": ownerCookie.String()})
	require.Equal(t, http.StatusOK, ownerEvents.Code)
	var events []struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(ownerEvents.Body.Bytes(), &events))
	require.Len(t, events, 1)

	memberLogin := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/auth/login",
		map[string]string{"username": "isolated-member", "password": "correct horse battery staple"},
		map[string]string{"Origin": "http://example.test"})
	require.Equal(t, http.StatusOK, memberLogin.Code)
	memberCookie := memberLogin.Result().Cookies()[0]
	var loginBody struct {
		CSRFToken string `json:"csrfToken"`
	}
	require.NoError(t, json.Unmarshal(memberLogin.Body.Bytes(), &loginBody))

	memberRecord := performJSONRequest(router, http.MethodGet,
		"http://example.test/api/v1/records/"+mediaID, nil,
		map[string]string{"Cookie": memberCookie.String()})
	require.Equal(t, http.StatusOK, memberRecord.Code)
	require.Contains(t, memberRecord.Body.String(), `"status":"none"`)
	require.Contains(t, memberRecord.Body.String(), `"note":null`)
	require.NotContains(t, memberRecord.Body.String(), `"status":"completed"`)
	memberCollections := performJSONRequest(router, http.MethodGet,
		"http://example.test/api/v1/collections", nil,
		map[string]string{"Cookie": memberCookie.String()})
	require.Equal(t, http.StatusOK, memberCollections.Code)
	require.JSONEq(t, `[]`, memberCollections.Body.String())
	memberEvents := performJSONRequest(router, http.MethodGet,
		"http://example.test/api/v1/records/"+mediaID+"/events", nil,
		map[string]string{"Cookie": memberCookie.String()})
	require.Equal(t, http.StatusOK, memberEvents.Code)
	require.JSONEq(t, `[]`, memberEvents.Body.String())
	deleteOwnerEvent := performJSONRequest(router, http.MethodDelete,
		"http://example.test/api/v1/records/"+mediaID+"/events/"+events[0].ID, nil,
		map[string]string{
			"Cookie": memberCookie.String(), "Origin": "http://example.test",
			"X-CSRF-Token": loginBody.CSRFToken, "Idempotency-Key": "cross-user-event-delete",
		})
	require.Equal(t, http.StatusNotFound, deleteOwnerEvent.Code)
}

func concreteSecurityPath(path string) string {
	replacer := strings.NewReplacer(
		"{mediaID}", "media-id",
		"{collectionID}", "collection-id",
		"{eventID}", "event-id",
		"{candidateID}", "candidate-id",
		"{memberID}", "member-id",
		"{mediaType}", "movie",
		"{externalID}", "1",
		"{id}", "media-id",
	)
	return replacer.Replace(path)
}
