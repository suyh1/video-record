package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

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

func TestTMDBSearchReturnsCamelCaseMovieAndTVResults(t *testing.T) {
	router, cookie := newTMDBTestRouter(t, "synthetic-token", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/search/multi", r.URL.Path)
		_, _ = w.Write([]byte(`{
			"page":1,
			"results":[
				{"id":329865,"media_type":"movie","title":"降临","original_title":"Arrival","release_date":"2016-11-10"},
				{"id":1399,"media_type":"tv","name":"权力的游戏","original_name":"Game of Thrones","first_air_date":"2011-04-17"}
			],
			"total_pages":1,
			"total_results":2
		}`))
	}))

	response := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/tmdb/search?q=arrival", nil, map[string]string{
		"Cookie": cookie.String(),
	})

	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(t, `{
		"page":1,
		"results":[
			{"id":329865,"mediaType":"movie","title":"降临","originalTitle":"Arrival","year":"2016","posterPath":"","overview":""},
			{"id":1399,"mediaType":"tv","title":"权力的游戏","originalTitle":"Game of Thrones","year":"2011","posterPath":"","overview":""}
		],
		"totalPages":1,
		"totalResults":2
	}`, response.Body.String())
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
			_, _ = w.Write([]byte(`{"id":329865,"title":"降临","original_title":"Arrival","release_date":"2016-11-10","runtime":116}`))
		case "/tv/1399":
			_, _ = w.Write([]byte(`{
				"id":1399,"name":"权力的游戏","original_name":"Game of Thrones",
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
			_, _ = w.Write([]byte(`{"cast":[{"id":1,"name":"肖恩·宾","character":"艾德·史塔克","profile_path":"/sean.jpg","order":0},{"id":2,"name":"米歇尔·菲尔利","character":"凯特琳·史塔克","profile_path":"","order":1}]}`))
		default:
			http.NotFound(w, r)
		}
	}))

	for _, test := range []struct {
		path     string
		expected string
	}{
		{path: "/api/v1/tmdb/movie/329865", expected: `"originalTitle":"Arrival"`},
		{path: "/api/v1/tmdb/tv/1399", expected: `"seasons":[{"id":3624,"name":"第 1 季","overview":"冬天将至","posterPath":"/season.jpg","airDate":"2011-04-17","seasonNumber":1,"episodeCount":10}]`},
		{path: "/api/v1/tmdb/tv/1399/season/1", expected: `"stillPath":"/winter.jpg"`},
		{path: "/api/v1/tmdb/tv/1399/season/1/episode/1", expected: `"stillPath":"/winter.jpg"`},
		{path: "/api/v1/tmdb/tv/1399/credits", expected: `"character":"艾德·史塔克"`},
	} {
		response := performJSONRequest(router, http.MethodGet, "http://example.test"+test.path, nil, map[string]string{
			"Cookie": cookie.String(),
		})
		require.Equal(t, http.StatusOK, response.Code)
		require.Contains(t, response.Body.String(), test.expected)
	}

	invalidCredits := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/tmdb/person/1399/credits", nil, map[string]string{
		"Cookie": cookie.String(),
	})
	require.Equal(t, http.StatusBadRequest, invalidCredits.Code)
	require.Contains(t, invalidCredits.Body.String(), `"code":"invalid_media_type"`)
}

func newTMDBTestRouter(t *testing.T, token string, upstream http.Handler) (http.Handler, *http.Cookie) {
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
		BaseURL: server.URL,
		Token:   token,
		Cache:   tmdb.NewCache(db, nil),
	})
	router := NewRouter(Dependencies{Storage: db, Auth: authService, TMDB: tmdbClient})
	cookie, _ := loginForHTTPTest(t, router)
	return router, cookie
}
