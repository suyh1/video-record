package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"video-record/internal/auth"
	"video-record/internal/integrations/tmdb"
	"video-record/internal/storage"
)

type publicTMDBHighlightsTestResponse struct {
	Items []struct {
		ID            int    `json:"id"`
		MediaType     string `json:"mediaType"`
		Title         string `json:"title"`
		OriginalTitle string `json:"originalTitle"`
		Year          string `json:"year"`
		Overview      string `json:"overview"`
		BackdropURL   string `json:"backdropURL"`
	} `json:"items"`
}

type publicTMDBRequestSnapshot struct {
	path          string
	authorization string
	language      string
}

func TestPublicTMDBHighlightsAllowAnonymousAccessAndPreservePrivateBoundary(t *testing.T) {
	const token = "synthetic-public-token"
	requests := make(chan publicTMDBRequestSnapshot, 2)
	router, _ := newPublicTMDBTestRouter(t, token, 0, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- publicTMDBRequestSnapshot{
			path:          r.URL.Path,
			authorization: r.Header.Get("Authorization"),
			language:      r.URL.Query().Get("language"),
		}
		switch r.URL.Path {
		case "/movie/popular":
			_, _ = w.Write([]byte(`{
				"page":1,
				"results":[
					{"id":1,"title":"电影一","original_title":"Movie One","release_date":"2026-01-02","overview":"电影简介一","backdrop_path":"/movie-one.jpg"},
					{"id":99,"title":"无背景电影","backdrop_path":""},
					{"id":2,"title":"电影二","backdrop_path":"/movie-two.jpg"},
					{"id":3,"title":"电影三","backdrop_path":"/movie-three.jpg"},
					{"id":4,"title":"电影四","backdrop_path":"/movie-four.jpg"},
					{"id":5,"title":"电影五","backdrop_path":"/movie-five.jpg"},
					{"id":6,"title":"电影六","backdrop_path":"/movie-six.jpg"}
				]
			}`))
		case "/tv/popular":
			_, _ = w.Write([]byte(`{
				"page":1,
				"results":[
					{"id":11,"name":"剧集一","original_name":"TV One","first_air_date":"2025-03-04","overview":"剧集简介一","backdrop_path":"/tv-one.webp"},
					{"id":12,"name":"剧集二","backdrop_path":"/tv-two.jpg"},
					{"id":13,"name":"剧集三","backdrop_path":"/tv-three.jpg"},
					{"id":14,"name":"剧集四","backdrop_path":"/tv-four.jpg"},
					{"id":15,"name":"剧集五","backdrop_path":"/tv-five.jpg"},
					{"id":16,"name":"剧集六","backdrop_path":"/tv-six.jpg"}
				]
			}`))
		default:
			http.NotFound(w, r)
		}
	}))

	before := time.Now()
	response := performJSONRequest(router, http.MethodGet,
		"http://example.test/api/v1/public/tmdb/highlights", nil, nil)
	after := time.Now()

	require.Equal(t, http.StatusOK, response.Code)
	var body publicTMDBHighlightsTestResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.Len(t, body.Items, 10)
	require.Equal(t,
		[]string{"movie", "tv", "movie", "tv", "movie", "tv", "movie", "tv", "movie", "tv"},
		mediaTypesFromHighlights(body),
	)
	require.Equal(t, 1, body.Items[0].ID)
	require.Equal(t, "电影一", body.Items[0].Title)
	require.Equal(t, "Movie One", body.Items[0].OriginalTitle)
	require.Equal(t, "2026", body.Items[0].Year)
	require.Equal(t, "电影简介一", body.Items[0].Overview)
	require.Equal(t, 11, body.Items[1].ID)
	require.Equal(t, "剧集一", body.Items[1].Title)
	require.Equal(t, "TV One", body.Items[1].OriginalTitle)
	require.Equal(t, "2025", body.Items[1].Year)
	require.Equal(t, "剧集简介一", body.Items[1].Overview)
	for _, item := range body.Items {
		parsed := parseSignedImageURL(t, item.BackdropURL)
		expires, err := strconv.ParseInt(parsed.Query().Get("expires"), 10, 64)
		require.NoError(t, err)
		require.NotEmpty(t, parsed.Query().Get("signature"))
		expiresAt := time.Unix(expires, 0)
		require.False(t, expiresAt.Before(before.Add(24*time.Hour-time.Second)))
		require.False(t, expiresAt.After(after.Add(24*time.Hour)))
	}
	requestedPaths := make([]string, 0, 2)
	for range 2 {
		request := <-requests
		requestedPaths = append(requestedPaths, request.path)
		require.Equal(t, "Bearer "+token, request.authorization)
		require.Equal(t, "zh-CN", request.language)
	}
	require.ElementsMatch(t, []string{"/movie/popular", "/tv/popular"}, requestedPaths)
	require.NotContains(t, response.Body.String(), token)
	require.NotContains(t, response.Body.String(), "无背景电影")

	privateResponse := performJSONRequest(router, http.MethodGet,
		"http://example.test/api/v1/tmdb/search?q=test", nil, nil)
	require.Equal(t, http.StatusUnauthorized, privateResponse.Code)
	require.Contains(t, privateResponse.Body.String(), `"code":"unauthenticated"`)
}

func TestPublicTMDBImageAllowsAnonymousAccessWithGeneratedURL(t *testing.T) {
	const imageContents = "synthetic-jpeg-contents"
	imageAuthorization := make(chan string, 1)
	router, _ := newPublicTMDBTestRouter(t, "synthetic-token", 0, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/movie/popular":
			_, _ = w.Write([]byte(`{"results":[{"id":1,"title":"电影","backdrop_path":"/arrival.jpg"}]}`))
		case "/tv/popular":
			_, _ = w.Write([]byte(`{"results":[]}`))
		case "/t/p/w1280/arrival.jpg":
			imageAuthorization <- r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte(imageContents))
		default:
			http.NotFound(w, r)
		}
	}))

	highlights := performJSONRequest(router, http.MethodGet,
		"http://example.test/api/v1/public/tmdb/highlights", nil, nil)
	require.Equal(t, http.StatusOK, highlights.Code)
	var body publicTMDBHighlightsTestResponse
	require.NoError(t, json.Unmarshal(highlights.Body.Bytes(), &body))
	require.Len(t, body.Items, 1)
	require.Equal(t, "/api/v1/public/tmdb/images/w1280/arrival.jpg", parseSignedImageURL(t, body.Items[0].BackdropURL).Path)

	image := performJSONRequest(router, http.MethodGet,
		"http://example.test"+body.Items[0].BackdropURL, nil, nil)
	require.Equal(t, http.StatusOK, image.Code)
	require.Equal(t, "image/jpeg", image.Header().Get("Content-Type"))
	require.Equal(t, "public, max-age=86400, immutable", image.Header().Get("Cache-Control"))
	require.Equal(t, "nosniff", image.Header().Get("X-Content-Type-Options"))
	require.Equal(t, imageContents, image.Body.String())
	require.Empty(t, <-imageAuthorization)
}

func TestPublicTMDBHighlightsRejectNonTMDBImageURLs(t *testing.T) {
	router, _ := newPublicTMDBTestRouter(t, "synthetic-token", 0, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/movie/popular":
			_, _ = w.Write([]byte(`{"results":[
				{"id":1,"title":"外部图片","backdrop_path":"https://cdn.example.test/external.jpg"},
				{"id":2,"title":"TMDB 图片","backdrop_path":"/trusted.jpg"}
			]}`))
		case "/tv/popular":
			_, _ = w.Write([]byte(`{"results":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))

	response := performJSONRequest(router, http.MethodGet,
		"http://example.test/api/v1/public/tmdb/highlights", nil, nil)

	require.Equal(t, http.StatusOK, response.Code)
	var body publicTMDBHighlightsTestResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.Len(t, body.Items, 1)
	require.Equal(t, 2, body.Items[0].ID)
	require.Equal(t, "TMDB 图片", body.Items[0].Title)
	require.NotContains(t, response.Body.String(), "cdn.example.test")
	require.NotContains(t, response.Body.String(), "https://image.tmdb.org")
}

func TestPublicTMDBImageRejectsInvalidAndTamperedRequests(t *testing.T) {
	unexpectedRequests := make(chan string, 16)
	router, client := newPublicTMDBTestRouter(t, "synthetic-token", 0, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		unexpectedRequests <- r.URL.String()
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("unexpected image"))
	}))
	expires := time.Now().Add(time.Hour).Truncate(time.Second)
	signature, err := client.SignImage("w1280", "/arrival.jpg", expires)
	require.NoError(t, err)
	validQuery := "?expires=" + strconv.FormatInt(expires.Unix(), 10) + "&signature=" + url.QueryEscape(signature)

	tests := []struct {
		name           string
		path           string
		expectedStatus int
		expectedCode   string
	}{
		{name: "missing parameters", path: "/api/v1/public/tmdb/images/w1280/arrival.jpg", expectedStatus: http.StatusBadRequest, expectedCode: "invalid_image_parameters"},
		{name: "invalid expiry", path: "/api/v1/public/tmdb/images/w1280/arrival.jpg?expires=tomorrow&signature=x", expectedStatus: http.StatusBadRequest, expectedCode: "invalid_image_parameters"},
		{name: "invalid size", path: "/api/v1/public/tmdb/images/original/arrival.jpg" + validQuery, expectedStatus: http.StatusBadRequest, expectedCode: "invalid_image_request"},
		{name: "invalid filename", path: "/api/v1/public/tmdb/images/w1280/arrival.gif" + validQuery, expectedStatus: http.StatusBadRequest, expectedCode: "invalid_image_request"},
		{name: "expired", path: "/api/v1/public/tmdb/images/w1280/arrival.jpg?expires=" + strconv.FormatInt(time.Now().Add(-time.Minute).Unix(), 10) + "&signature=" + url.QueryEscape(signature), expectedStatus: http.StatusGone, expectedCode: "image_url_expired"},
		{name: "tampered signature", path: "/api/v1/public/tmdb/images/w1280/arrival.jpg?expires=" + strconv.FormatInt(expires.Unix(), 10) + "&signature=" + strings.Repeat("0", 64), expectedStatus: http.StatusForbidden, expectedCode: "invalid_image_signature"},
		{name: "tampered path", path: "/api/v1/public/tmdb/images/w1280/changed.jpg" + validQuery, expectedStatus: http.StatusForbidden, expectedCode: "invalid_image_signature"},
		{name: "tampered future expiry", path: "/api/v1/public/tmdb/images/w1280/arrival.jpg?expires=" + strconv.FormatInt(expires.Add(time.Second).Unix(), 10) + "&signature=" + url.QueryEscape(signature), expectedStatus: http.StatusForbidden, expectedCode: "invalid_image_signature"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := performJSONRequest(router, http.MethodGet, "http://example.test"+test.path, nil, nil)
			require.Equal(t, test.expectedStatus, response.Code)
			require.Equal(t, "application/problem+json", response.Header().Get("Content-Type"))
			require.Contains(t, response.Body.String(), `"code":"`+test.expectedCode+`"`)
		})
	}
	require.Empty(t, unexpectedRequests)
}

func TestPublicTMDBImageReturnsGoneWhenExpiryPassesDuringSignatureVerification(t *testing.T) {
	client := tmdb.NewClient(tmdb.ClientOptions{Token: "synthetic-token"})
	expires := time.Now().Add(time.Hour).Truncate(time.Second)
	clockCalls := 0
	handlers := publicTMDBHandlers{
		client: client,
		now: func() time.Time {
			clockCalls++
			if clockCalls == 1 {
				return expires.Add(-time.Nanosecond)
			}
			return expires.Add(time.Nanosecond)
		},
	}
	router := chi.NewRouter()
	router.Get("/api/v1/public/tmdb/images/{size}/{filename}", handlers.image)
	requestURL := fmt.Sprintf(
		"http://example.test/api/v1/public/tmdb/images/w1280/arrival.jpg?expires=%d&signature=%s",
		expires.Unix(), strings.Repeat("0", 64),
	)

	response := performJSONRequest(router, http.MethodGet, requestURL, nil, nil)

	require.Equal(t, http.StatusGone, response.Code)
	require.Contains(t, response.Body.String(), `"code":"image_url_expired"`)
}

func TestPublicTMDBImageQueuesUntilSlotAvailable(t *testing.T) {
	firstStarted := make(chan struct{}, 1)
	releaseFirst := make(chan struct{}, 1)
	secondStarted := make(chan struct{}, 1)
	var upstreamRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestNumber := upstreamRequests.Add(1)
		if requestNumber == 1 {
			firstStarted <- struct{}{}
			<-releaseFirst
		} else {
			secondStarted <- struct{}{}
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("queued-image"))
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() {
		select {
		case releaseFirst <- struct{}{}:
		default:
		}
	})
	client := tmdb.NewClient(tmdb.ClientOptions{
		ImageBaseURL: server.URL,
		Token:        "synthetic-token",
		Timeout:      2 * time.Second,
	})
	handlers := publicTMDBHandlers{
		client:        client,
		now:           time.Now,
		imageSlots:    make(chan struct{}, 1),
		imageRequests: make(chan struct{}, 2),
	}
	router := chi.NewRouter()
	router.Get("/api/v1/public/tmdb/images/{size}/{filename}", handlers.image)
	expires := time.Now().Add(time.Hour).Truncate(time.Second)
	signature, err := client.SignImage("w1280", "/arrival.jpg", expires)
	require.NoError(t, err)
	requestURL := fmt.Sprintf(
		"http://example.test/api/v1/public/tmdb/images/w1280/arrival.jpg?expires=%d&signature=%s",
		expires.Unix(), url.QueryEscape(signature),
	)

	firstResponse := httptest.NewRecorder()
	firstDone := make(chan struct{})
	go func() {
		router.ServeHTTP(firstResponse, httptest.NewRequest(http.MethodGet, requestURL, nil))
		close(firstDone)
	}()
	requireChannelSignal(t, firstStarted, "first upstream image request did not start")

	secondResponse := httptest.NewRecorder()
	secondDone := make(chan struct{})
	go func() {
		router.ServeHTTP(secondResponse, httptest.NewRequest(http.MethodGet, requestURL, nil))
		close(secondDone)
	}()
	require.Eventually(t, func() bool { return len(handlers.imageRequests) == 2 }, time.Second, 10*time.Millisecond)
	select {
	case <-secondStarted:
		t.Fatal("queued image request reached upstream before a slot was released")
	case <-secondDone:
		t.Fatal("queued image request returned before a slot was released")
	default:
	}

	releaseFirst <- struct{}{}
	requireChannelSignal(t, firstDone, "first public image handler did not return")
	requireChannelSignal(t, secondStarted, "queued image request did not reach upstream")
	requireChannelSignal(t, secondDone, "queued public image handler did not return")
	require.Equal(t, http.StatusOK, firstResponse.Code)
	require.Equal(t, http.StatusOK, secondResponse.Code)
	require.EqualValues(t, 2, upstreamRequests.Load())
	require.Empty(t, handlers.imageSlots)
	require.Empty(t, handlers.imageRequests)
}

func TestPublicTMDBImageRejectsWhenAdmissionLimitIsFull(t *testing.T) {
	firstStarted := make(chan struct{}, 1)
	firstCanceled := make(chan struct{}, 1)
	var upstreamRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber := upstreamRequests.Add(1)
		if requestNumber == 1 {
			firstStarted <- struct{}{}
			<-r.Context().Done()
			firstCanceled <- struct{}{}
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("released-slot-image"))
	}))
	t.Cleanup(server.Close)
	client := tmdb.NewClient(tmdb.ClientOptions{
		ImageBaseURL: server.URL,
		Token:        "synthetic-token",
		Timeout:      2 * time.Second,
	})
	handlers := publicTMDBHandlers{
		client:        client,
		now:           time.Now,
		imageSlots:    make(chan struct{}, 1),
		imageRequests: make(chan struct{}, 1),
	}
	router := chi.NewRouter()
	router.Get("/api/v1/public/tmdb/images/{size}/{filename}", handlers.image)
	expires := time.Now().Add(time.Hour).Truncate(time.Second)
	signature, err := client.SignImage("w1280", "/arrival.jpg", expires)
	require.NoError(t, err)
	requestURL := fmt.Sprintf(
		"http://example.test/api/v1/public/tmdb/images/w1280/arrival.jpg?expires=%d&signature=%s",
		expires.Unix(), url.QueryEscape(signature),
	)

	firstContext, cancelFirst := context.WithCancel(context.Background())
	defer cancelFirst()
	firstRequest := httptest.NewRequest(http.MethodGet, requestURL, nil).WithContext(firstContext)
	firstResponse := httptest.NewRecorder()
	firstDone := make(chan struct{})
	go func() {
		router.ServeHTTP(firstResponse, firstRequest)
		close(firstDone)
	}()
	requireChannelSignal(t, firstStarted, "first upstream image request did not start")

	secondResponse := performJSONRequest(router, http.MethodGet, requestURL, nil, nil)
	requestsAfterSecond := upstreamRequests.Load()
	cancelFirst()
	requireChannelSignal(t, firstCanceled, "active upstream image request was not canceled")
	requireChannelSignal(t, firstDone, "canceled public image handler did not return")

	require.Equal(t, http.StatusTooManyRequests, secondResponse.Code)
	require.Equal(t, "1", secondResponse.Header().Get("Retry-After"))
	require.Contains(t, secondResponse.Body.String(), `"code":"tmdb_image_busy"`)
	require.EqualValues(t, 1, requestsAfterSecond)

	thirdResponse := performJSONRequest(router, http.MethodGet, requestURL, nil, nil)
	require.Equal(t, http.StatusOK, thirdResponse.Code)
	require.Equal(t, "released-slot-image", thirdResponse.Body.String())
	require.EqualValues(t, 2, upstreamRequests.Load())
	require.Empty(t, handlers.imageSlots)
	require.Empty(t, handlers.imageRequests)
}

func TestPublicTMDBImageReleasesQueuedAdmissionAfterCancellation(t *testing.T) {
	var upstreamRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamRequests.Add(1)
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("unexpected-image"))
	}))
	t.Cleanup(server.Close)
	client := tmdb.NewClient(tmdb.ClientOptions{
		ImageBaseURL: server.URL,
		Token:        "synthetic-token",
		Timeout:      2 * time.Second,
	})
	handlers := publicTMDBHandlers{
		client:        client,
		now:           time.Now,
		imageSlots:    make(chan struct{}, 1),
		imageRequests: make(chan struct{}, 1),
	}
	handlers.imageSlots <- struct{}{}
	router := chi.NewRouter()
	router.Get("/api/v1/public/tmdb/images/{size}/{filename}", handlers.image)
	expires := time.Now().Add(time.Hour).Truncate(time.Second)
	signature, err := client.SignImage("w1280", "/arrival.jpg", expires)
	require.NoError(t, err)
	requestURL := fmt.Sprintf(
		"http://example.test/api/v1/public/tmdb/images/w1280/arrival.jpg?expires=%d&signature=%s",
		expires.Unix(), url.QueryEscape(signature),
	)

	requestContext, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()
	request := httptest.NewRequest(http.MethodGet, requestURL, nil).WithContext(requestContext)
	response := httptest.NewRecorder()
	requestDone := make(chan struct{})
	go func() {
		router.ServeHTTP(response, request)
		close(requestDone)
	}()
	require.Eventually(t, func() bool { return len(handlers.imageRequests) == 1 }, time.Second, 10*time.Millisecond)
	cancelRequest()
	requireChannelSignal(t, requestDone, "canceled queued image request did not return")

	require.Zero(t, upstreamRequests.Load())
	require.Empty(t, handlers.imageRequests)
	require.Len(t, handlers.imageSlots, 1)
	<-handlers.imageSlots
}

func TestPublicTMDBHighlightsMapUpstreamErrorsWithoutLeaks(t *testing.T) {
	tests := []struct {
		name           string
		token          string
		timeout        time.Duration
		upstream       http.Handler
		expectedStatus int
		expectedCode   string
	}{
		{name: "not configured", expectedStatus: http.StatusServiceUnavailable, expectedCode: "tmdb_not_configured"},
		{name: "unauthorized", token: "secret-unauthorized-token", upstream: statusHandler(http.StatusUnauthorized), expectedStatus: http.StatusBadGateway, expectedCode: "tmdb_unauthorized"},
		{name: "rate limited", token: "secret-rate-token", upstream: statusHandler(http.StatusTooManyRequests), expectedStatus: http.StatusServiceUnavailable, expectedCode: "tmdb_rate_limited"},
		{name: "unavailable", token: "secret-unavailable-token", upstream: statusHandler(http.StatusInternalServerError), expectedStatus: http.StatusBadGateway, expectedCode: "tmdb_unavailable"},
		{
			name: "timeout", token: "secret-timeout-token", timeout: 20 * time.Millisecond,
			upstream:       http.HandlerFunc(func(http.ResponseWriter, *http.Request) { time.Sleep(100 * time.Millisecond) }),
			expectedStatus: http.StatusGatewayTimeout, expectedCode: "tmdb_timeout",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			upstream := test.upstream
			var unexpectedRequests chan string
			if upstream == nil {
				unexpectedRequests = make(chan string, 2)
				upstream = http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
					unexpectedRequests <- r.URL.String()
				})
			}
			router, _ := newPublicTMDBTestRouter(t, test.token, test.timeout, upstream)
			response := performJSONRequest(router, http.MethodGet,
				"http://example.test/api/v1/public/tmdb/highlights", nil, nil)

			require.Equal(t, test.expectedStatus, response.Code)
			require.Contains(t, response.Body.String(), `"code":"`+test.expectedCode+`"`)
			require.NotContains(t, response.Body.String(), "secret-")
			require.NotContains(t, response.Body.String(), "upstream secret response")
			if unexpectedRequests != nil {
				require.Empty(t, unexpectedRequests)
			}
		})
	}
}

func TestPublicTMDBHighlightsFailWithoutPartialItemsWhenOneFeedFails(t *testing.T) {
	router, _ := newPublicTMDBTestRouter(t, "synthetic-token", 0, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/movie/popular":
			_, _ = w.Write([]byte(`{"results":[{"id":1,"title":"不应泄露的电影","backdrop_path":"/movie.jpg"}]}`))
		case "/tv/popular":
			http.Error(w, "upstream secret response", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))

	response := performJSONRequest(router, http.MethodGet,
		"http://example.test/api/v1/public/tmdb/highlights", nil, nil)

	require.Equal(t, http.StatusBadGateway, response.Code)
	require.Contains(t, response.Body.String(), `"code":"tmdb_unavailable"`)
	require.NotContains(t, response.Body.String(), `"items"`)
	require.NotContains(t, response.Body.String(), "不应泄露的电影")
	require.NotContains(t, response.Body.String(), "upstream secret response")
}

func TestPublicTMDBImageMapsUpstreamErrorsWithoutLeaks(t *testing.T) {
	for _, test := range []struct {
		name           string
		status         int
		expectedStatus int
		expectedCode   string
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, expectedStatus: http.StatusBadGateway, expectedCode: "tmdb_unauthorized"},
		{name: "rate limited", status: http.StatusTooManyRequests, expectedStatus: http.StatusServiceUnavailable, expectedCode: "tmdb_rate_limited"},
		{name: "unavailable", status: http.StatusNotFound, expectedStatus: http.StatusBadGateway, expectedCode: "tmdb_unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			router, client := newPublicTMDBTestRouter(t, "secret-image-token", 0, statusHandler(test.status))
			expires := time.Now().Add(time.Hour).Truncate(time.Second)
			signature, err := client.SignImage("w1280", "/arrival.jpg", expires)
			require.NoError(t, err)
			requestURL := fmt.Sprintf(
				"http://example.test/api/v1/public/tmdb/images/w1280/arrival.jpg?expires=%d&signature=%s",
				expires.Unix(), url.QueryEscape(signature),
			)

			response := performJSONRequest(router, http.MethodGet, requestURL, nil, nil)

			require.Equal(t, test.expectedStatus, response.Code)
			require.Contains(t, response.Body.String(), `"code":"`+test.expectedCode+`"`)
			require.NotContains(t, response.Body.String(), "secret-image-token")
			require.NotContains(t, response.Body.String(), "upstream secret response")
		})
	}
}

func newPublicTMDBTestRouter(
	t *testing.T,
	token string,
	timeout time.Duration,
	upstream http.Handler,
) (http.Handler, *tmdb.Client) {
	t.Helper()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(context.Background(), db))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	authService := auth.NewService(auth.NewRepository(db), auth.ServiceOptions{})
	_, err = authService.Initialize(context.Background(), "owner", "correct horse battery staple")
	require.NoError(t, err)
	server := httptest.NewServer(upstream)
	t.Cleanup(server.Close)
	client := tmdb.NewClient(tmdb.ClientOptions{
		BaseURL:      server.URL,
		ImageBaseURL: server.URL + "/t/p",
		Token:        token,
		Cache:        tmdb.NewCache(db, nil),
		Timeout:      timeout,
	})
	return NewRouter(Dependencies{Storage: db, Auth: authService, TMDB: client}), client
}

func mediaTypesFromHighlights(response publicTMDBHighlightsTestResponse) []string {
	mediaTypes := make([]string, 0, len(response.Items))
	for _, item := range response.Items {
		mediaTypes = append(mediaTypes, item.MediaType)
	}
	return mediaTypes
}

func parseSignedImageURL(t *testing.T, value string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(value)
	require.NoError(t, err)
	require.Empty(t, parsed.Scheme)
	require.Empty(t, parsed.Host)
	require.Equal(t, "w1280", strings.Split(parsed.Path, "/")[6])
	require.NotEmpty(t, parsed.Query().Get("expires"))
	require.NotEmpty(t, parsed.Query().Get("signature"))
	return parsed
}

func statusHandler(status int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte("upstream secret response"))
	})
}

func requireChannelSignal(t *testing.T, signal <-chan struct{}, failureMessage string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatal(failureMessage)
	}
}
