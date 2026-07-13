package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"video-record/internal/auth"
	"video-record/internal/integrations/tmdb"
	"video-record/internal/media"
	"video-record/internal/storage"
)

func TestMediaHandlersCreateReadAndLinkTMDBItems(t *testing.T) {
	router, cookie, csrfToken := newMediaTestRouter(t)
	headers := map[string]string{
		"Cookie":          cookie.String(),
		"Origin":          "http://example.test",
		"X-CSRF-Token":    csrfToken,
		"Idempotency-Key": "create-tmdb-media",
	}

	created := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/media/tmdb/movie/329865", nil, headers)
	require.Equal(t, http.StatusOK, created.Code)
	var createdBody struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &createdBody))
	require.NotEmpty(t, createdBody.ID)
	require.NotEqual(t, "329865", createdBody.ID)
	require.Equal(t, "降临", createdBody.Title)
	require.Contains(t, created.Body.String(), `"runtimeMinutes":116`)
	require.Contains(t, created.Body.String(), `"genres":["剧情"]`)

	read := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/media/"+createdBody.ID, nil, map[string]string{
		"Cookie": cookie.String(),
	})
	require.Equal(t, http.StatusOK, read.Code)
	require.Contains(t, read.Body.String(), `"originalTitle":"Arrival"`)

	customHeaders := cloneHeaders(headers)
	customHeaders["Idempotency-Key"] = "create-custom-media"
	custom := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/media/custom", map[string]string{
		"mediaType": "movie",
		"title":     "我的译名",
		"overview":  "私人简介",
		"year":      "2016",
	}, customHeaders)
	require.Equal(t, http.StatusCreated, custom.Code)
	var customBody struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(custom.Body.Bytes(), &customBody))

	linkHeaders := cloneHeaders(headers)
	linkHeaders["Idempotency-Key"] = "link-custom-media"
	linked := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/media/"+customBody.ID+"/tmdb/movie/329866", nil, linkHeaders)
	require.Equal(t, http.StatusOK, linked.Code)
	require.Contains(t, linked.Body.String(), `"title":"我的译名"`)
	require.Contains(t, linked.Body.String(), `"externalTitle":"外部更新"`)
}

func newMediaTestRouter(t *testing.T) (http.Handler, *http.Cookie, string) {
	t.Helper()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(context.Background(), db))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	authService := auth.NewService(auth.NewRepository(db), auth.ServiceOptions{})
	_, err = authService.Initialize(context.Background(), "owner", "correct horse battery staple")
	require.NoError(t, err)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/movie/329865":
			_, _ = w.Write([]byte(`{"id":329865,"title":"降临","original_title":"Arrival","release_date":"2016-11-10","overview":"外部简介","poster_path":"/arrival.jpg","runtime":116,"genres":[{"id":18,"name":"剧情"}]}`))
		case "/movie/329866":
			_, _ = w.Write([]byte(`{"id":329866,"title":"外部更新","original_title":"Updated","release_date":"2016-11-10","overview":"外部简介 v2","poster_path":"/updated.jpg"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(upstream.Close)
	tmdbClient := tmdb.NewClient(tmdb.ClientOptions{
		BaseURL: upstream.URL,
		Token:   "synthetic-token",
		Cache:   tmdb.NewCache(db, nil),
	})
	router := NewRouter(Dependencies{
		Storage: db,
		Auth:    authService,
		TMDB:    tmdbClient,
		Media:   media.NewService(media.NewRepository(db)),
	})
	cookie, csrfToken := loginForHTTPTest(t, router)
	return router, cookie, csrfToken
}
