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
	router, cookie, csrfToken, mediaService := newMediaTestRouter(t)
	headers := map[string]string{
		"Cookie":          cookie.String(),
		"Origin":          "http://example.test",
		"X-CSRF-Token":    csrfToken,
		"Idempotency-Key": "create-tmdb-media",
	}

	created := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/media/tmdb/movie/329865", nil, headers)
	require.Equal(t, http.StatusOK, created.Code)
	var createdBody struct {
		ID           string `json:"id"`
		Title        string `json:"title"`
		TMDBID       *int   `json:"tmdbId"`
		PosterPath   string `json:"posterPath"`
		BackdropPath string `json:"backdropPath"`
	}
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &createdBody))
	require.NotEmpty(t, createdBody.ID)
	require.NotEqual(t, "329865", createdBody.ID)
	require.Equal(t, "降临", createdBody.Title)
	require.NotNil(t, createdBody.TMDBID)
	require.Equal(t, 329865, *createdBody.TMDBID)
	require.Contains(t, created.Body.String(), `"runtimeMinutes":116`)
	require.Contains(t, created.Body.String(), `"genres":["剧情"]`)
	requireSignedTMDBImageURL(t, createdBody.PosterPath, "w342", "arrival.jpg")
	requireSignedTMDBImageURL(t, createdBody.BackdropPath, "w1280", "arrival-bg.jpg")
	require.NotContains(t, created.Body.String(), `"posterPath":"/arrival.jpg"`)
	require.NotContains(t, created.Body.String(), "https://image.tmdb.org")

	read := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/media/"+createdBody.ID, nil, map[string]string{
		"Cookie": cookie.String(),
	})
	require.Equal(t, http.StatusOK, read.Code)
	require.Contains(t, read.Body.String(), `"originalTitle":"Arrival"`)
	var readBody mediaItemResponse
	require.NoError(t, json.Unmarshal(read.Body.Bytes(), &readBody))
	requireSignedTMDBImageURL(t, readBody.PosterPath, "w342", "arrival.jpg")
	requireSignedTMDBImageURL(t, readBody.BackdropPath, "w1280", "arrival-bg.jpg")

	customHeaders := cloneHeaders(headers)
	customHeaders["Idempotency-Key"] = "create-custom-media"
	custom := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/media/custom", map[string]string{
		"mediaType": "movie",
		"title":     "我的译名",
		"overview":  "私人简介",
		"year":      "2016",
	}, customHeaders)
	require.Equal(t, http.StatusCreated, custom.Code)
	require.Contains(t, custom.Body.String(), `"tmdbId":null`)
	var customBody struct {
		ID           string `json:"id"`
		PosterPath   string `json:"posterPath"`
		BackdropPath string `json:"backdropPath"`
	}
	require.NoError(t, json.Unmarshal(custom.Body.Bytes(), &customBody))
	require.Empty(t, customBody.PosterPath)
	require.Empty(t, customBody.BackdropPath)

	linkHeaders := cloneHeaders(headers)
	linkHeaders["Idempotency-Key"] = "link-custom-media"
	linked := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/media/"+customBody.ID+"/tmdb/movie/329866", nil, linkHeaders)
	require.Equal(t, http.StatusOK, linked.Code)
	require.Contains(t, linked.Body.String(), `"title":"我的译名"`)
	require.Contains(t, linked.Body.String(), `"externalTitle":"外部更新"`)
	require.Contains(t, linked.Body.String(), `"tmdbId":329866`)
	var linkedBody mediaItemResponse
	require.NoError(t, json.Unmarshal(linked.Body.Bytes(), &linkedBody))
	require.Empty(t, linkedBody.PosterPath)
	require.Empty(t, linkedBody.BackdropPath)
	require.NotContains(t, linked.Body.String(), "cdn.example.test")

	linkedRead := performJSONRequest(router, http.MethodGet,
		"http://example.test/api/v1/media/"+customBody.ID, nil,
		map[string]string{"Cookie": cookie.String()})
	require.Equal(t, http.StatusOK, linkedRead.Code)
	var linkedReadBody mediaItemResponse
	require.NoError(t, json.Unmarshal(linkedRead.Body.Bytes(), &linkedReadBody))
	require.NotNil(t, linkedReadBody.TMDBID)
	require.Empty(t, linkedReadBody.PosterPath)
	require.Empty(t, linkedReadBody.BackdropPath)
	require.NotContains(t, linkedRead.Body.String(), "cdn.example.test")

	customImageItem, err := mediaService.CreateCustom(context.Background(), media.CreateCustomInput{
		MediaType: media.MediaTypeMovie, Title: "自定义图片",
	})
	require.NoError(t, err)
	customImageItem, err = mediaService.LinkExternal(context.Background(), customImageItem.ID, media.ExternalSnapshot{
		Source: "custom-provider", SourceID: "custom-image", MediaType: media.MediaTypeMovie,
		Title: "自定义图片", PosterPath: "https://cdn.example.test/custom-poster.jpg",
		BackdropPath: "http://cdn.example.test/custom-backdrop.webp",
	})
	require.NoError(t, err)
	customImageResponse := performJSONRequest(router, http.MethodGet,
		"http://example.test/api/v1/media/"+customImageItem.ID, nil,
		map[string]string{"Cookie": cookie.String()})
	require.Equal(t, http.StatusOK, customImageResponse.Code)
	var customImageBody mediaItemResponse
	require.NoError(t, json.Unmarshal(customImageResponse.Body.Bytes(), &customImageBody))
	require.Equal(t, "https://cdn.example.test/custom-poster.jpg", customImageBody.PosterPath)
	require.Equal(t, "http://cdn.example.test/custom-backdrop.webp", customImageBody.BackdropPath)
}

func TestMediaHandlerRejectsTMDBCustomImageHostAliases(t *testing.T) {
	router, cookie, _, mediaService := newMediaTestRouter(t)
	item, err := mediaService.CreateCustom(context.Background(), media.CreateCustomInput{
		MediaType: media.MediaTypeMovie, Title: "TMDB 域名别名",
	})
	require.NoError(t, err)
	item, err = mediaService.LinkExternal(context.Background(), item.ID, media.ExternalSnapshot{
		Source: "custom-provider", SourceID: "tmdb-host-alias", MediaType: media.MediaTypeMovie,
		Title:        "TMDB 域名别名",
		PosterPath:   "https://ImAgE.TmDb.OrG./t/p/w342/poster.jpg",
		BackdropPath: "http://image.tmdb.org../t/p/w1280/backdrop.jpg",
	})
	require.NoError(t, err)

	response := performJSONRequest(router, http.MethodGet,
		"http://example.test/api/v1/media/"+item.ID, nil,
		map[string]string{"Cookie": cookie.String()})

	require.Equal(t, http.StatusOK, response.Code)
	var body mediaItemResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.Nil(t, body.TMDBID)
	require.Empty(t, body.PosterPath)
	require.Empty(t, body.BackdropPath)
	require.NotContains(t, response.Body.String(), "TmDb.OrG")
	require.NotContains(t, response.Body.String(), "tmdb.org")
}

func newMediaTestRouter(t *testing.T) (http.Handler, *http.Cookie, string, *media.Service) {
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
			_, _ = w.Write([]byte(`{"id":329865,"title":"降临","original_title":"Arrival","release_date":"2016-11-10","overview":"外部简介","poster_path":"/arrival.jpg","backdrop_path":"/arrival-bg.jpg","runtime":116,"genres":[{"id":18,"name":"剧情"}]}`))
		case "/movie/329866":
			_, _ = w.Write([]byte(`{"id":329866,"title":"外部更新","original_title":"Updated","release_date":"2016-11-10","overview":"外部简介 v2","poster_path":"https://cdn.example.test/untrusted-poster.jpg","backdrop_path":"http://cdn.example.test/untrusted-backdrop.jpg"}`))
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
	mediaService := media.NewService(media.NewRepository(db))
	router := NewRouter(Dependencies{
		Storage: db,
		Auth:    authService,
		TMDB:    tmdbClient,
		Media:   mediaService,
	})
	cookie, csrfToken := loginForHTTPTest(t, router)
	return router, cookie, csrfToken, mediaService
}
