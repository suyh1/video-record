package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"video-record/internal/auth"
	"video-record/internal/integrations/tmdb"
	"video-record/internal/storage"
)

func TestTMDBStatusNeverReturnsTokenMaterial(t *testing.T) {
	for _, test := range []struct {
		name       string
		token      string
		configured bool
	}{
		{name: "configured", token: "synthetic-tmdb-token", configured: true},
		{name: "unconfigured", token: "", configured: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			router, cookie := newTMDBTestRouter(t, test.token, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))

			response := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/tmdb/status", nil, map[string]string{
				"Cookie": cookie.String(),
			})

			require.Equal(t, http.StatusOK, response.Code)
			require.JSONEq(t, map[bool]string{true: `{"configured":true}`, false: `{"configured":false}`}[test.configured], response.Body.String())
			if test.token != "" {
				require.NotContains(t, response.Body.String(), test.token)
			}
		})
	}
}

func TestTMDBConnectivityReturnsConnectedWithoutLeakingTokenMaterial(t *testing.T) {
	const token = "synthetic-connectivity-token"
	router, cookie := newTMDBTestRouter(t, token, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/configuration", r.URL.Path)
		require.Equal(t, "Bearer "+token, r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"images":{"debug_token":"synthetic-connectivity-token"}}`))
	}))

	response := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/tmdb/connectivity", nil, map[string]string{
		"Cookie": cookie.String(),
	})

	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(t, `{"connected":true}`, response.Body.String())
	require.NotContains(t, response.Body.String(), token)
}

func TestTMDBConnectivityReturnsActionableProblemCodes(t *testing.T) {
	for _, test := range []struct {
		name           string
		upstreamStatus int
		expectedStatus int
		expectedCode   string
	}{
		{name: "unauthorized", upstreamStatus: http.StatusUnauthorized, expectedStatus: http.StatusBadGateway, expectedCode: "tmdb_unauthorized"},
		{name: "forbidden", upstreamStatus: http.StatusForbidden, expectedStatus: http.StatusBadGateway, expectedCode: "tmdb_unauthorized"},
		{name: "rate limited", upstreamStatus: http.StatusTooManyRequests, expectedStatus: http.StatusServiceUnavailable, expectedCode: "tmdb_rate_limited"},
		{name: "unavailable", upstreamStatus: http.StatusInternalServerError, expectedStatus: http.StatusBadGateway, expectedCode: "tmdb_unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			router, cookie := newTMDBTestRouter(t, "synthetic-token", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "upstream secret response", test.upstreamStatus)
			}))

			response := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/tmdb/connectivity", nil, map[string]string{
				"Cookie": cookie.String(),
			})

			require.Equal(t, test.expectedStatus, response.Code)
			require.Contains(t, response.Body.String(), `"code":"`+test.expectedCode+`"`)
			require.NotContains(t, response.Body.String(), "upstream secret response")
			require.NotContains(t, response.Body.String(), "synthetic-token")
		})
	}
}

func TestTMDBConnectivityReturnsTimeoutProblem(t *testing.T) {
	router, cookie := newTMDBTestRouterWithTimeout(t, "synthetic-token", 20*time.Millisecond, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))

	response := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/tmdb/connectivity", nil, map[string]string{
		"Cookie": cookie.String(),
	})

	require.Equal(t, http.StatusGatewayTimeout, response.Code)
	require.Contains(t, response.Body.String(), `"code":"tmdb_timeout"`)
}

func TestTMDBSearchReturnsCamelCaseMovieAndTVResultsWithProxiedPosters(t *testing.T) {
	router, cookie := newTMDBTestRouter(t, "synthetic-token", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search/multi":
			_, _ = w.Write([]byte(`{
				"page":1,
				"results":[
					{"id":329865,"media_type":"movie","title":"降临","original_title":"Arrival","release_date":"2016-11-10","poster_path":"/arrival.jpg"},
					{"id":1399,"media_type":"tv","name":"权力的游戏","original_name":"Game of Thrones","first_air_date":"2011-04-17","poster_path":""},
					{"id":2,"media_type":"movie","title":"自定义海报","poster_path":"https://cdn.example.test/custom.jpg"},
					{"id":3,"media_type":"movie","title":"非法海报","poster_path":"/nested/poster.jpg"}
				],
				"total_pages":1,
				"total_results":4
			}`))
		case "/t/p/w342/arrival.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("proxied-search-poster"))
		default:
			http.NotFound(w, r)
		}
	}))

	response := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/tmdb/search?q=arrival", nil, map[string]string{
		"Cookie": cookie.String(),
	})

	require.Equal(t, http.StatusOK, response.Code)
	var body tmdbSearchResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.Len(t, body.Results, 4)
	require.Equal(t, "降临", body.Results[0].Title)
	require.Equal(t, "2016", body.Results[0].Year)
	requireSignedTMDBImageURL(t, body.Results[0].PosterPath, "w342", "arrival.jpg")
	require.Empty(t, body.Results[1].PosterPath)
	require.Empty(t, body.Results[2].PosterPath)
	require.Empty(t, body.Results[3].PosterPath)
	require.NotContains(t, response.Body.String(), `"posterPath":"/arrival.jpg"`)
	require.NotContains(t, response.Body.String(), "https://image.tmdb.org")

	imageResponse := performJSONRequest(router, http.MethodGet,
		"http://example.test"+body.Results[0].PosterPath, nil, nil)
	require.Equal(t, http.StatusOK, imageResponse.Code)
	require.Equal(t, "proxied-search-poster", imageResponse.Body.String())
}

func TestTMDBRateLimitReturnsStableProblemAndRetryAfter(t *testing.T) {
	router, cookie := newTMDBTestRouter(t, "synthetic-token", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "90")
		w.WriteHeader(http.StatusTooManyRequests)
	}))

	response := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/tmdb/search?q=arrival", nil, map[string]string{
		"Cookie": cookie.String(),
	})

	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.Equal(t, "90", response.Header().Get("Retry-After"))
	require.Contains(t, response.Body.String(), `"code":"tmdb_rate_limited"`)
}

func TestTMDBDetailsRoutesReturnCamelCaseSnapshots(t *testing.T) {
	router, cookie := newTMDBTestRouter(t, "synthetic-token", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/movie/329865":
			_, _ = w.Write([]byte(`{"id":329865,"title":"降临","original_title":"Arrival","release_date":"2016-11-10","poster_path":"/arrival.jpg","backdrop_path":"/arrival-bg.webp","runtime":116}`))
		case "/tv/1399":
			_, _ = w.Write([]byte(`{
				"id":1399,"name":"权力的游戏","original_name":"Game of Thrones",
				"poster_path":"/got.jpg","backdrop_path":"/got-bg.jpg",
				"first_air_date":"2011-04-17","number_of_seasons":8,"number_of_episodes":73,
				"episode_run_time":[57],"genres":[{"id":18,"name":"剧情"}],
				"seasons":[{"id":3624,"name":"第 1 季","overview":"冬天将至","poster_path":"/season.jpg","air_date":"2011-04-17","season_number":1,"episode_count":10}]
			}`))
		case "/tv/1399/season/1":
			_, _ = w.Write([]byte(`{
				"id":3624,"name":"第 1 季","overview":"冬天将至","poster_path":"/season.jpg","air_date":"2011-04-17","season_number":1,
				"episodes":[{"id":63056,"name":"凛冬将至","season_number":1,"episode_number":1,"runtime":62,"still_path":"/winter.jpg"}]
			}`))
		case "/tv/1399/season/1/episode/1":
			_, _ = w.Write([]byte(`{"id":63056,"name":"凛冬将至","season_number":1,"episode_number":1,"runtime":62,"still_path":"/winter.jpg"}`))
		case "/tv/1399/credits":
			_, _ = w.Write([]byte(`{"cast":[{"id":1,"name":"肖恩·宾","character":"艾德·史塔克","profile_path":"/sean.jpg","order":0},{"id":2,"name":"米歇尔·菲尔利","character":"凯特琳·史塔克","profile_path":"","order":1},{"id":3,"name":"非法头像","profile_path":"/nested/profile.jpg","order":2}]}`))
		default:
			http.NotFound(w, r)
		}
	}))

	headers := map[string]string{"Cookie": cookie.String()}
	movie := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/tmdb/movie/329865", nil, headers)
	require.Equal(t, http.StatusOK, movie.Code)
	var movieBody tmdbMovieResponse
	require.NoError(t, json.Unmarshal(movie.Body.Bytes(), &movieBody))
	require.Equal(t, "Arrival", movieBody.OriginalTitle)
	requireSignedTMDBImageURL(t, movieBody.PosterPath, "w342", "arrival.jpg")
	requireSignedTMDBImageURL(t, movieBody.BackdropPath, "w1280", "arrival-bg.webp")

	tvResponse := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/tmdb/tv/1399", nil, headers)
	require.Equal(t, http.StatusOK, tvResponse.Code)
	var tvBody tmdbTVResponse
	require.NoError(t, json.Unmarshal(tvResponse.Body.Bytes(), &tvBody))
	requireSignedTMDBImageURL(t, tvBody.PosterPath, "w342", "got.jpg")
	requireSignedTMDBImageURL(t, tvBody.BackdropPath, "w1280", "got-bg.jpg")
	require.Len(t, tvBody.Seasons, 1)
	requireSignedTMDBImageURL(t, tvBody.Seasons[0].PosterPath, "w342", "season.jpg")

	season := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/tmdb/tv/1399/season/1", nil, headers)
	require.Equal(t, http.StatusOK, season.Code)
	var seasonBody tmdbSeasonResponse
	require.NoError(t, json.Unmarshal(season.Body.Bytes(), &seasonBody))
	requireSignedTMDBImageURL(t, seasonBody.PosterPath, "w342", "season.jpg")
	require.Len(t, seasonBody.Episodes, 1)
	requireSignedTMDBImageURL(t, seasonBody.Episodes[0].StillPath, "w780", "winter.jpg")

	episode := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/tmdb/tv/1399/season/1/episode/1", nil, headers)
	require.Equal(t, http.StatusOK, episode.Code)
	var episodeBody tmdbEpisodeResponse
	require.NoError(t, json.Unmarshal(episode.Body.Bytes(), &episodeBody))
	requireSignedTMDBImageURL(t, episodeBody.StillPath, "w780", "winter.jpg")

	credits := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/tmdb/tv/1399/credits", nil, headers)
	require.Equal(t, http.StatusOK, credits.Code)
	var creditsBody tmdbCreditsResponse
	require.NoError(t, json.Unmarshal(credits.Body.Bytes(), &creditsBody))
	require.Len(t, creditsBody.Cast, 3)
	requireSignedTMDBImageURL(t, creditsBody.Cast[0].ProfilePath, "w300", "sean.jpg")
	require.Empty(t, creditsBody.Cast[1].ProfilePath)
	require.Empty(t, creditsBody.Cast[2].ProfilePath)

	for _, response := range []*httptest.ResponseRecorder{movie, tvResponse, season, episode, credits} {
		require.NotContains(t, response.Body.String(), "https://image.tmdb.org")
		require.NotRegexp(t,
			`"(?:posterPath|backdropPath|stillPath|profilePath)":"/(?:arrival|got|season|winter|sean)`,
			response.Body.String(),
		)
	}

	invalidCredits := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/tmdb/person/1399/credits", nil, map[string]string{
		"Cookie": cookie.String(),
	})
	require.Equal(t, http.StatusBadRequest, invalidCredits.Code)
	require.Contains(t, invalidCredits.Body.String(), `"code":"invalid_media_type"`)
}

func requireSignedTMDBImageURL(t *testing.T, value, size, filename string) {
	t.Helper()
	parsed, err := url.Parse(value)
	require.NoError(t, err)
	require.Empty(t, parsed.Scheme)
	require.Empty(t, parsed.Host)
	require.Equal(t, "/api/v1/public/tmdb/images/"+size+"/"+filename, parsed.Path)
	require.NotEmpty(t, parsed.Query().Get("expires"))
	require.NotEmpty(t, parsed.Query().Get("signature"))
}

func newTMDBTestRouter(t *testing.T, token string, upstream http.Handler) (http.Handler, *http.Cookie) {
	return newTMDBTestRouterWithTimeout(t, token, 0, upstream)
}

func newTMDBTestRouterWithTimeout(t *testing.T, token string, timeout time.Duration, upstream http.Handler) (http.Handler, *http.Cookie) {
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
	tmdbClient := tmdb.NewClient(tmdb.ClientOptions{
		BaseURL:      server.URL,
		ImageBaseURL: server.URL + "/t/p",
		Token:        token,
		Cache:        tmdb.NewCache(db, nil),
		Timeout:      timeout,
	})
	router := NewRouter(Dependencies{Storage: db, Auth: authService, TMDB: tmdbClient})
	cookie, _ := loginForHTTPTest(t, router)
	return router, cookie
}
