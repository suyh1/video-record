package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"video-record/internal/storage"
)

func TestHealthz(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	NewRouter(Dependencies{}).ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"status":"ok"}`, rec.Body.String())
}

func TestReadyzBeforeStorageInitialization(t *testing.T) {
	rec := requestHealthEndpoint(NewRouter(Dependencies{}), "/readyz")

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.JSONEq(t, `{
		"type":"about:blank",
		"title":"Service Unavailable",
		"status":503,
		"code":"not_ready",
		"requestId":"`+rec.Header().Get(RequestIDHeader)+`"
	}`, rec.Body.String())
}

func TestReadyzRejectsUnmigratedStorage(t *testing.T) {
	db := openTestStorage(t)

	rec := requestHealthEndpoint(NewRouter(Dependencies{Storage: db}), "/readyz")

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestReadyzAfterMigratedStorage(t *testing.T) {
	db := openTestStorage(t)
	require.NoError(t, storage.Migrate(context.Background(), db))

	rec := requestHealthEndpoint(NewRouter(Dependencies{Storage: db}), "/readyz")

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"status":"ok"}`, rec.Body.String())
}

func TestReadyzRejectsClosedStorage(t *testing.T) {
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(context.Background(), db))
	require.NoError(t, db.Close())

	rec := requestHealthEndpoint(NewRouter(Dependencies{Storage: db}), "/readyz")

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func openTestStorage(t *testing.T) *storage.DB {
	t.Helper()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db
}

func requestHealthEndpoint(handler http.Handler, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
