package emby

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"video-record/internal/integrations"
	"video-record/internal/integrations/providertest"
)

const (
	testUserID = "emby-user-1"
	testToken  = "synthetic-emby-token"
)

func TestProviderConformance(t *testing.T) {
	providertest.Run(t, func(t *testing.T, scenario string) integrations.Provider {
		return newScenarioClient(t, scenario)
	}, providertest.Expectations{
		FirstCursor:  "2026-07-13|202",
		SecondCursor: "2026-07-13|203",
		Secret:       testToken,
	})
}

func TestHistoryMapsEmbyMovieEpisodeAndRepeatRows(t *testing.T) {
	page, err := newScenarioClient(t, "success").History(
		context.Background(), integrations.HistoryRequest{Limit: 10},
	)

	require.NoError(t, err)
	require.Len(t, page.Events, 3)
	require.Equal(t, "2026-07-13|203", page.NextCursor)
	movie := page.Events[0]
	require.Equal(t, "emby:201", movie.ID)
	require.Equal(t, integrations.MediaMovie, movie.Item.MediaType)
	require.Equal(t, "movie-emby-1", movie.Item.ProviderItemID)
	require.Equal(t, "603", movie.Item.TMDBID)
	require.Equal(t, "tt0133093", movie.Item.IMDbID)
	require.Equal(t, 8160, movie.DurationSeconds)
	require.Equal(t, time.Date(2026, 7, 13, 9, 15, 0, 0, time.UTC), movie.PlayedAt)

	episode := page.Events[1]
	require.Equal(t, integrations.MediaEpisode, episode.Item.MediaType)
	require.Equal(t, "Emby Synthetic Series", episode.Item.Title)
	require.Equal(t, 1, episode.Item.SeasonNumber)
	require.Equal(t, 4, episode.Item.EpisodeNumber)
	require.Equal(t, "76543", episode.Item.TVDBID)

	repeat := page.Events[2]
	require.Equal(t, movie.Item, repeat.Item)
	require.NotEqual(t, movie.ID, repeat.ID)
	require.True(t, repeat.PlayedAt.After(movie.PlayedAt))
}

func TestEmbyHistoryCursorPaginationAndDateRange(t *testing.T) {
	client := newScenarioClient(t, "success")
	first, err := client.History(context.Background(), integrations.HistoryRequest{Limit: 2})
	require.NoError(t, err)
	require.Len(t, first.Events, 2)
	second, err := client.History(context.Background(), integrations.HistoryRequest{
		Cursor: first.NextCursor, Limit: 2,
	})
	require.NoError(t, err)
	require.Len(t, second.Events, 1)
	require.Equal(t, "emby:203", second.Events[0].ID)

	_, err = client.History(context.Background(), integrations.HistoryRequest{
		Since: time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC),
		Until: time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
	})
	require.ErrorIs(t, err, integrations.ErrInvalidResponse)
}

func TestEmbyHistoryInterpretsPluginClockInConfiguredServerLocation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/emby/user_usage_stats/emby-user-1/2026-07-13/GetItems":
			writeFixture(t, w, "history.json")
		case "/emby/Items/movie-emby-1":
			writeFixture(t, w, "movie.json")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	location := time.FixedZone("server-east", 8*60*60)
	client := NewClient(ClientOptions{
		BaseURL: server.URL + "/emby", Token: testToken, UserID: testUserID, Location: location,
	})

	page, err := client.History(context.Background(), integrations.HistoryRequest{
		Since: time.Date(2026, 7, 12, 17, 0, 0, 0, time.UTC), Limit: 1,
	})

	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 7, 13, 1, 15, 0, 0, time.UTC), page.Events[0].PlayedAt)
}

func TestEmbyDeletedUsersMalformedResponsesAndFailures(t *testing.T) {
	t.Run("deleted user", func(t *testing.T) {
		err := newScenarioClient(t, "deleted_user").CheckAuthentication(context.Background())
		require.ErrorIs(t, err, integrations.ErrAuthentication)
	})
	for _, scenario := range []string{"malformed_history", "null_activity", "mismatched_user", "malformed_item"} {
		t.Run(scenario, func(t *testing.T) {
			_, err := newScenarioClient(t, scenario).History(
				context.Background(), integrations.HistoryRequest{Limit: 2},
			)
			require.ErrorIs(t, err, integrations.ErrInvalidResponse)
		})
	}
	t.Run("server failure", func(t *testing.T) {
		_, err := newScenarioClient(t, "server_failure").History(
			context.Background(), integrations.HistoryRequest{Limit: 2},
		)
		require.ErrorIs(t, err, integrations.ErrUnavailable)
		require.True(t, integrations.IsRetryable(err))
		require.NotContains(t, err.Error(), testToken)
		require.NotContains(t, err.Error(), "upstream echoed")
	})
}

func TestEmbyMissingConfigurationCancellationAndMalformedCursor(t *testing.T) {
	unconfigured := NewClient(ClientOptions{})
	require.ErrorIs(t, unconfigured.CheckAuthentication(context.Background()), integrations.ErrAuthentication)
	_, err := unconfigured.History(context.Background(), integrations.HistoryRequest{})
	require.ErrorIs(t, err, integrations.ErrAuthentication)

	client := NewClient(ClientOptions{
		BaseURL: "https://emby.example.test/emby", Token: testToken, UserID: testUserID,
	})
	_, err = client.History(context.Background(), integrations.HistoryRequest{Cursor: "invalid"})
	require.ErrorIs(t, err, integrations.ErrInvalidResponse)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.History(canceled, integrations.HistoryRequest{})
	require.ErrorIs(t, err, context.Canceled)
}

func TestEmbyRetryAfterAndTimeoutUseStableErrors(t *testing.T) {
	_, err := newScenarioClient(t, "rate_limited").History(
		context.Background(), integrations.HistoryRequest{Limit: 1},
	)
	require.Error(t, err)
	require.Equal(t, 60*time.Second, integrations.RetryAfter(err))
	require.False(t, errors.Is(err, context.Canceled))

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		time.Sleep(100 * time.Millisecond)
	}))
	t.Cleanup(server.Close)
	client := NewClient(ClientOptions{
		BaseURL: server.URL + "/emby", Token: testToken, UserID: testUserID, Timeout: 10 * time.Millisecond,
	})
	_, err = client.History(context.Background(), integrations.HistoryRequest{})
	require.ErrorIs(t, err, integrations.ErrUnavailable)
}

func newScenarioClient(t *testing.T, scenario string) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, testToken, r.Header.Get("X-Emby-Token"))
		require.Empty(t, r.URL.Query().Get("api_key"))
		if scenario == "deleted_user" && strings.HasPrefix(r.URL.Path, "/emby/Users/") {
			http.NotFound(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/emby/Users/") {
			writeFixture(t, w, "user.json")
			return
		}
		if strings.Contains(r.URL.Path, "/GetItems") {
			require.Equal(t, "/emby/user_usage_stats/emby-user-1/2026-07-13/GetItems", r.URL.Path)
			require.Equal(t, "Movie,Episode", r.URL.Query().Get("Filter"))
			switch scenario {
			case "rate_limited":
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(http.StatusTooManyRequests)
			case "server_failure":
				http.Error(w, "upstream echoed "+testToken, http.StatusBadGateway)
			case "malformed_history":
				writeFixture(t, w, "malformed.json")
			case "null_activity":
				_, _ = w.Write([]byte(`{"user_id":"emby-user-1","activity":null}`))
			case "mismatched_user":
				_, _ = w.Write([]byte(`{"user_id":"other-user","activity":[]}`))
			default:
				writeFixture(t, w, "history.json")
			}
			return
		}
		if strings.HasPrefix(r.URL.Path, "/emby/Items/") {
			if scenario == "malformed_item" {
				writeFixture(t, w, "malformed.json")
				return
			}
			switch r.URL.Path {
			case "/emby/Items/movie-emby-1":
				writeFixture(t, w, "movie.json")
			case "/emby/Items/episode-emby-1":
				writeFixture(t, w, "episode.json")
			default:
				http.NotFound(w, r)
			}
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	return NewClient(ClientOptions{
		BaseURL: server.URL + "/emby", Token: testToken, UserID: testUserID,
		Now: func() time.Time { return time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC) },
	})
}

func writeFixture(t *testing.T, w http.ResponseWriter, name string) {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(contents)
	require.NoError(t, err)
}
