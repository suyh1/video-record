package assets

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

func TestHandlerServesStaticAssetsAndSPAFallback(t *testing.T) {
	files := fstest.MapFS{
		"index.html":             {Data: []byte("<!doctype html><div id=\"root\"></div>")},
		"assets/app-12345678.js": {Data: []byte("console.log('synthetic')")},
	}
	api := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	handler := newHandler(api, files)

	for _, requestPath := range []string{"/", "/library", "/settings/sync"} {
		response := performAssetRequest(t, handler, http.MethodGet, requestPath)
		require.Equal(t, http.StatusOK, response.Code)
		require.Contains(t, response.Body.String(), "id=\"root\"")
		require.Equal(t, "no-cache", response.Header().Get("Cache-Control"))
	}

	asset := performAssetRequest(t, handler, http.MethodGet, "/assets/app-12345678.js")
	require.Equal(t, http.StatusOK, asset.Code)
	require.Equal(t, "public, max-age=31536000, immutable", asset.Header().Get("Cache-Control"))
	require.Contains(t, asset.Header().Get("Content-Type"), "javascript")
	require.Contains(t, asset.Body.String(), "synthetic")

	head := performAssetRequest(t, handler, http.MethodHead, "/assets/app-12345678.js")
	require.Equal(t, http.StatusOK, head.Code)
	require.Empty(t, head.Body.String())
}

func TestHandlerDelegatesAPIHealthAndNonReadRequests(t *testing.T) {
	files := fstest.MapFS{"index.html": {Data: []byte("index")}}
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = io.WriteString(w, r.Method+" "+r.URL.Path)
	})
	handler := newHandler(api, files)

	for _, test := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/setup/status"},
		{http.MethodGet, "/healthz"},
		{http.MethodGet, "/healthz/"},
		{http.MethodGet, "/readyz"},
		{http.MethodGet, "/readyz/"},
		{http.MethodPost, "/library"},
	} {
		response := performAssetRequest(t, handler, test.method, test.path)
		require.Equal(t, http.StatusTeapot, response.Code)
		require.Equal(t, test.method+" "+test.path, response.Body.String())
	}
}

func performAssetRequest(t *testing.T, handler http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, "http://example.test"+path, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
