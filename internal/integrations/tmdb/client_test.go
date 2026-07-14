package tmdb

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"video-record/internal/storage"
)

func TestClientSendsBearerTokenWithoutLeakingUpstreamDetails(t *testing.T) {
	const token = "synthetic-tmdb-token"
	var logs bytes.Buffer
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer "+token, r.Header.Get("Authorization"))
		http.Error(w, "upstream accidentally echoed "+token, http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	client := NewClient(ClientOptions{
		BaseURL: server.URL,
		Token:   token,
		Logger:  slog.New(slog.NewJSONHandler(&logs, nil)),
	})

	_, err := client.Search(context.Background(), "arrival", "zh-CN")

	require.ErrorIs(t, err, ErrUpstreamUnavailable)
	require.NotContains(t, err.Error(), token)
	require.NotContains(t, err.Error(), "upstream accidentally echoed")
	require.NotContains(t, logs.String(), token)
}

func TestCacheStoresNormalizedResponseWithoutUnknownTokenMaterial(t *testing.T) {
	const token = "synthetic-tmdb-token"
	cache := newTestCache(t, time.Now)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"page":1,
			"results":[{"id":329865,"media_type":"movie","title":"降临"}],
			"total_pages":1,
			"total_results":1,
			"debug_token":"synthetic-tmdb-token"
		}`))
	}))
	t.Cleanup(server.Close)
	client := NewClient(ClientOptions{BaseURL: server.URL, Token: token, Cache: cache})

	_, err := client.Search(context.Background(), "arrival", "zh-CN")
	require.NoError(t, err)

	var cached []byte
	require.NoError(t, cache.db.Reader().QueryRowContext(
		context.Background(),
		"SELECT response_json FROM tmdb_cache LIMIT 1",
	).Scan(&cached))
	require.NotContains(t, string(cached), token)
	require.NotContains(t, string(cached), "debug_token")
}

func TestSearchCacheExpiresAfterSixHours(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, time.July, 13, 12, 0, 0, 0, time.UTC)}
	cache := newTestCache(t, clock.Now)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		require.Equal(t, "/search/multi", r.URL.Path)
		require.Equal(t, "arrival", r.URL.Query().Get("query"))
		require.Equal(t, "zh-CN", r.URL.Query().Get("language"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"page":1,"results":[{"id":329865,"media_type":"movie","title":"降临"},{"id":42,"media_type":"person","name":"演员"}],"total_pages":1,"total_results":2}`))
	}))
	t.Cleanup(server.Close)
	client := NewClient(ClientOptions{BaseURL: server.URL, Token: "synthetic-token", Cache: cache})

	first, err := client.Search(context.Background(), "arrival", "zh-CN")
	require.NoError(t, err)
	second, err := client.Search(context.Background(), "arrival", "zh-CN")
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.Len(t, first.Results, 1)
	require.Equal(t, "movie", first.Results[0].MediaType)
	require.Equal(t, int32(1), requests.Load())

	clock.Advance(6*time.Hour + time.Millisecond)
	_, err = client.Search(context.Background(), "arrival", "zh-CN")
	require.NoError(t, err)
	require.Equal(t, int32(2), requests.Load())
}

func TestClientFetchesLiveTVSeasonAndCredits(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "zh-CN", r.URL.Query().Get("language"))
		switch r.URL.Path {
		case "/tv/1399":
			_, _ = w.Write([]byte(`{
				"id":1399,
				"name":"权力的游戏",
				"first_air_date":"2011-04-17",
				"episode_run_time":[57],
				"seasons":[{"id":3624,"name":"第 1 季","season_number":1,"episode_count":10,"poster_path":"/season.jpg"}]
			}`))
		case "/tv/1399/season/1":
			_, _ = w.Write([]byte(`{
				"id":3624,
				"name":"第 1 季",
				"overview":"维斯特洛的冬天将至。",
				"poster_path":"/season.jpg",
				"air_date":"2011-04-17",
				"season_number":1,
				"episodes":[{"id":63056,"season_number":1,"episode_number":1,"name":"凛冬将至","still_path":"/winter.jpg"}]
			}`))
		case "/tv/1399/season/1/episode/1":
			_, _ = w.Write([]byte(`{"id":63056,"season_number":1,"episode_number":1,"name":"凛冬将至","still_path":"/winter.jpg"}`))
		case "/tv/1399/credits":
			_, _ = w.Write([]byte(`{
				"cast":[
					{"id":1,"name":"肖恩·宾","character":"艾德·史塔克","profile_path":"/sean.jpg","order":0},
					{"id":2,"name":"米歇尔·菲尔利","character":"凯特琳·史塔克","profile_path":"","order":1}
				]
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	client := NewClient(ClientOptions{BaseURL: server.URL, Token: "synthetic-token"})

	tv, err := client.TVDetails(context.Background(), 1399, "zh-CN")
	require.NoError(t, err)
	require.Equal(t, 1399, tv.ID)
	require.Equal(t, []int{57}, tv.EpisodeRunTime)
	require.Len(t, tv.Seasons, 1)
	require.Equal(t, 10, tv.Seasons[0].EpisodeCount)
	season, err := client.SeasonDetails(context.Background(), 1399, 1, "zh-CN")
	require.NoError(t, err)
	require.Equal(t, 1, season.SeasonNumber)
	require.Equal(t, "/season.jpg", season.PosterPath)
	require.Equal(t, "/winter.jpg", season.Episodes[0].StillPath)
	episode, err := client.EpisodeDetails(context.Background(), 1399, 1, 1, "zh-CN")
	require.NoError(t, err)
	require.Equal(t, 1, episode.EpisodeNumber)
	require.Equal(t, "/winter.jpg", episode.StillPath)
	credits, err := client.Credits(context.Background(), "tv", 1399, "zh-CN")
	require.NoError(t, err)
	require.Len(t, credits.Cast, 2)
	require.Equal(t, "艾德·史塔克", credits.Cast[0].Character)
}

func TestLiveDetailsCacheExpiresAfterSixHours(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, time.July, 13, 12, 0, 0, 0, time.UTC)}
	cache := newTestCache(t, clock.Now)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		require.Equal(t, "/movie/329865", r.URL.Path)
		_, _ = w.Write([]byte(`{"id":329865,"title":"降临","release_date":"2016-11-10"}`))
	}))
	t.Cleanup(server.Close)
	client := NewClient(ClientOptions{BaseURL: server.URL, Token: "synthetic-token", Cache: cache})

	_, err := client.MovieDetails(context.Background(), 329865, "zh-CN")
	require.NoError(t, err)
	_, err = client.MovieDetails(context.Background(), 329865, "zh-CN")
	require.NoError(t, err)
	require.Equal(t, int32(1), requests.Load())

	clock.Advance(6*time.Hour + time.Millisecond)
	_, err = client.MovieDetails(context.Background(), 329865, "zh-CN")
	require.NoError(t, err)
	require.Equal(t, int32(2), requests.Load())
}

func TestClientMapsRetryAfterAndTimeoutToStableErrors(t *testing.T) {
	t.Run("rate limited", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Retry-After", "120")
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		t.Cleanup(server.Close)
		client := NewClient(ClientOptions{BaseURL: server.URL, Token: "synthetic-token"})

		_, err := client.Search(context.Background(), "arrival", "zh-CN")

		require.ErrorIs(t, err, ErrRateLimited)
		var clientError *ClientError
		require.ErrorAs(t, err, &clientError)
		require.Equal(t, 2*time.Minute, clientError.RetryAfter)
	})

	t.Run("deadline", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			time.Sleep(100 * time.Millisecond)
		}))
		t.Cleanup(server.Close)
		client := NewClient(ClientOptions{
			BaseURL: server.URL,
			Token:   "synthetic-token",
			Timeout: 20 * time.Millisecond,
		})

		_, err := client.Search(context.Background(), "arrival", "zh-CN")

		require.ErrorIs(t, err, ErrUpstreamTimeout)
	})

	client := NewClient(ClientOptions{BaseURL: "https://example.test", Token: "synthetic-token"})
	require.Equal(t, 8*time.Second, client.timeout)
}

func TestClientConnectivityChecksConfigurationWithoutCache(t *testing.T) {
	const token = "synthetic-connectivity-token"
	cache := newTestCache(t, time.Now)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		require.Equal(t, "/configuration", r.URL.Path)
		require.Empty(t, r.URL.RawQuery)
		require.Equal(t, "Bearer "+token, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"images":{"secure_base_url":"https://image.tmdb.org/t/p/"}}`))
	}))
	t.Cleanup(server.Close)
	client := NewClient(ClientOptions{BaseURL: server.URL, Token: token, Cache: cache})

	require.NoError(t, client.TestConnectivity(context.Background()))
	require.NoError(t, client.TestConnectivity(context.Background()))
	require.Equal(t, int32(2), requests.Load())

	var cachedResponses int
	require.NoError(t, cache.db.Reader().QueryRowContext(
		context.Background(),
		"SELECT COUNT(*) FROM tmdb_cache",
	).Scan(&cachedResponses))
	require.Zero(t, cachedResponses)
}

func TestClientConnectivitySeparatesRejectedCredentials(t *testing.T) {
	for _, test := range []struct {
		name       string
		statusCode int
		expected   error
	}{
		{name: "unauthorized", statusCode: http.StatusUnauthorized, expected: ErrUnauthorized},
		{name: "forbidden", statusCode: http.StatusForbidden, expected: ErrUnauthorized},
		{name: "upstream failure", statusCode: http.StatusInternalServerError, expected: ErrUpstreamUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.statusCode)
			}))
			t.Cleanup(server.Close)
			client := NewClient(ClientOptions{BaseURL: server.URL, Token: "synthetic-token"})

			err := client.TestConnectivity(context.Background())

			require.ErrorIs(t, err, test.expected)
		})
	}
}

func newTestCache(t *testing.T, now func() time.Time) *Cache {
	t.Helper()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(context.Background(), db))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return NewCache(db, now)
}

type fakeClock struct {
	now time.Time
}

func (clock *fakeClock) Now() time.Time {
	return clock.now
}

func (clock *fakeClock) Advance(duration time.Duration) {
	clock.now = clock.now.Add(duration)
}
