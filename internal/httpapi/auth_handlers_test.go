package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"video-record/internal/auth"
	"video-record/internal/integrations/tmdb"
	"video-record/internal/storage"
)

func TestInitializeClosesSetupAfterFirstAdministrator(t *testing.T) {
	router, _ := newAuthTestRouter(t, false)

	status := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/setup/status", nil, nil)
	require.Equal(t, http.StatusOK, status.Code)
	require.JSONEq(t, `{"initialized":false,"storageReady":true,"tmdbConfigured":false}`, status.Body.String())

	created := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/setup/admin", map[string]string{
		"username": "owner",
		"password": "correct horse battery staple",
	}, map[string]string{"Origin": "http://example.test"})
	require.Equal(t, http.StatusCreated, created.Code)
	require.Contains(t, created.Body.String(), `"role":"admin"`)

	closed := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/setup/admin", map[string]string{
		"username": "second",
		"password": "another secure password",
	}, map[string]string{"Origin": "http://example.test"})
	require.Equal(t, http.StatusConflict, closed.Code)
	require.Contains(t, closed.Body.String(), `"code":"initialization_closed"`)
}

func TestSetupStatusReportsConfiguredTMDBWithoutExposingCredentials(t *testing.T) {
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(context.Background(), db))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	service := auth.NewService(auth.NewRepository(db), auth.ServiceOptions{})
	router := NewRouter(Dependencies{
		Storage: db,
		Auth:    service,
		TMDB:    tmdb.NewClient(tmdb.ClientOptions{Token: "synthetic-test-token"}),
	})

	status := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/setup/status", nil, nil)
	require.Equal(t, http.StatusOK, status.Code)
	require.JSONEq(t, `{"initialized":false,"storageReady":true,"tmdbConfigured":true}`, status.Body.String())
	require.NotContains(t, status.Body.String(), "synthetic-test-token")
}

func TestLoginSetsOpaqueSessionCookieWithConditionalSecurity(t *testing.T) {
	for _, secure := range []bool{false, true} {
		t.Run(map[bool]string{false: "development", true: "production"}[secure], func(t *testing.T) {
			router, service := newAuthTestRouter(t, secure)
			_, err := service.Initialize(context.Background(), "owner", "correct horse battery staple")
			require.NoError(t, err)

			login := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/auth/login", map[string]string{
				"username": "owner",
				"password": "correct horse battery staple",
			}, map[string]string{"Origin": "http://example.test"})

			require.Equal(t, http.StatusOK, login.Code)
			cookies := login.Result().Cookies()
			require.Len(t, cookies, 1)
			cookie := cookies[0]
			require.Equal(t, SessionCookieName, cookie.Name)
			require.NotEmpty(t, cookie.Value)
			require.NotContains(t, cookie.Value, "owner")
			require.True(t, cookie.HttpOnly)
			require.Equal(t, http.SameSiteLaxMode, cookie.SameSite)
			require.Equal(t, secure, cookie.Secure)
			require.Equal(t, "/", cookie.Path)
			_, err = service.Authenticate(context.Background(), cookie.Value)
			require.NoError(t, err)
			require.Contains(t, login.Body.String(), `"csrfToken":"`)
		})
	}
}

func TestLoginAcceptsHTTPSOriginForwardedByProxy(t *testing.T) {
	tests := map[string]map[string]string{
		"forwarded": {
			"Origin":    "https://example.test",
			"Forwarded": `for=192.0.2.1;proto="https", for=172.18.0.1;proto=http`,
		},
		"x-forwarded-proto": {
			"Origin":            "https://example.test",
			"X-Forwarded-Proto": "https, http",
		},
	}
	for name, headers := range tests {
		t.Run(name, func(t *testing.T) {
			router, service := newAuthTestRouter(t, true)
			_, err := service.Initialize(context.Background(), "owner", "correct horse battery staple")
			require.NoError(t, err)

			response := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/auth/login", map[string]string{
				"username": "owner",
				"password": "correct horse battery staple",
			}, headers)

			require.Equal(t, http.StatusOK, response.Code)
		})
	}
}

func TestLoginRejectsInvalidForwardedOrigin(t *testing.T) {
	tests := map[string]map[string]string{
		"conflicting proxy protocols": {
			"Origin":            "https://example.test",
			"Forwarded":         "for=192.0.2.1;proto=https",
			"X-Forwarded-Proto": "http",
		},
		"unsupported proxy protocol": {
			"Origin":            "https://example.test",
			"X-Forwarded-Proto": "websocket",
		},
		"wrong origin host": {
			"Origin":            "https://attacker.test",
			"X-Forwarded-Proto": "https",
		},
		"missing origin": {
			"X-Forwarded-Proto": "https",
		},
	}
	for name, headers := range tests {
		t.Run(name, func(t *testing.T) {
			router, service := newAuthTestRouter(t, true)
			_, err := service.Initialize(context.Background(), "owner", "correct horse battery staple")
			require.NoError(t, err)

			response := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/auth/login", map[string]string{
				"username": "owner",
				"password": "correct horse battery staple",
			}, headers)

			require.Equal(t, http.StatusForbidden, response.Code)
			require.Contains(t, response.Body.String(), `"code":"invalid_origin"`)
		})
	}
}

func TestLoginAcceptsMatchingDirectTLSOrigin(t *testing.T) {
	router, service := newAuthTestRouter(t, true)
	_, err := service.Initialize(context.Background(), "owner", "correct horse battery staple")
	require.NoError(t, err)

	response := performJSONRequest(router, http.MethodPost, "https://example.test/api/v1/auth/login", map[string]string{
		"username": "owner",
		"password": "correct horse battery staple",
	}, map[string]string{"Origin": "https://example.test"})

	require.Equal(t, http.StatusOK, response.Code)
}

func TestCSRFAndOriginProtectLogout(t *testing.T) {
	router, service := newAuthTestRouter(t, false)
	_, err := service.Initialize(context.Background(), "owner", "correct horse battery staple")
	require.NoError(t, err)
	cookie, csrfToken := loginForHTTPTest(t, router)

	withoutOrigin := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/auth/logout", nil, map[string]string{
		"Cookie": cookie.String(),
	})
	require.Equal(t, http.StatusForbidden, withoutOrigin.Code)

	withoutCSRF := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/auth/logout", nil, map[string]string{
		"Cookie": cookie.String(),
		"Origin": "http://example.test",
	})
	require.Equal(t, http.StatusForbidden, withoutCSRF.Code)

	wrongOrigin := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/auth/logout", nil, map[string]string{
		"Cookie":       cookie.String(),
		"Origin":       "https://attacker.test",
		"X-CSRF-Token": csrfToken,
	})
	require.Equal(t, http.StatusForbidden, wrongOrigin.Code)

	logout := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/auth/logout", nil, map[string]string{
		"Cookie":       cookie.String(),
		"Origin":       "http://example.test",
		"X-CSRF-Token": csrfToken,
	})
	require.Equal(t, http.StatusNoContent, logout.Code)

	me := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/auth/me", nil, map[string]string{
		"Cookie": cookie.String(),
	})
	require.Equal(t, http.StatusUnauthorized, me.Code)
}

func newAuthTestRouter(t *testing.T, cookieSecure bool) (http.Handler, *auth.Service) {
	t.Helper()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(context.Background(), db))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	service := auth.NewService(auth.NewRepository(db), auth.ServiceOptions{})
	return NewRouter(Dependencies{Storage: db, Auth: service, CookieSecure: cookieSecure}), service
}

func loginForHTTPTest(t *testing.T, router http.Handler) (*http.Cookie, string) {
	t.Helper()
	response := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/auth/login", map[string]string{
		"username": "owner",
		"password": "correct horse battery staple",
	}, map[string]string{"Origin": "http://example.test"})
	require.Equal(t, http.StatusOK, response.Code)
	require.Len(t, response.Result().Cookies(), 1)
	var body struct {
		CSRFToken string `json:"csrfToken"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.NotEmpty(t, body.CSRFToken)
	return response.Result().Cookies()[0], body.CSRFToken
}

func performJSONRequest(
	handler http.Handler,
	method, target string,
	body any,
	headers map[string]string,
) *httptest.ResponseRecorder {
	var encoded bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&encoded).Encode(body)
	}
	req := httptest.NewRequest(method, target, &encoded)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
