package jellyfin

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
	testUserID = "user-1"
	testToken  = "synthetic-jellyfin-token"
)

func TestProviderConformance(t *testing.T) {
	providertest.Run(t, func(t *testing.T, scenario string) integrations.Provider {
		return newScenarioClient(t, scenario)
	}, providertest.Expectations{
		FirstCursor:  "2026-07-13|102",
		SecondCursor: "2026-07-13|103",
		Secret:       testToken,
	})
}

func TestHistoryMapsMoviesEpisodesAndRepeatPlays(t *testing.T) {
	client := newScenarioClient(t, "success")

	page, err := client.History(context.Background(), integrations.HistoryRequest{Limit: 10})

	require.NoError(t, err)
	require.Len(t, page.Events, 3)
	require.Equal(t, "2026-07-13|103", page.NextCursor)
	movie := page.Events[0]
	require.Equal(t, "jellyfin:101", movie.ID)
	require.Equal(t, integrations.MediaMovie, movie.Item.MediaType)
	require.Equal(t, "movie-1", movie.Item.ProviderItemID)
	require.Equal(t, "329865", movie.Item.TMDBID)
	require.Equal(t, "tt2543164", movie.Item.IMDbID)
	require.Equal(t, 6960, movie.DurationSeconds)
	require.Equal(t, 7000, movie.PositionSeconds)
	require.Equal(t, time.Date(2026, 7, 13, 9, 0, 0, 0, time.UTC), movie.PlayedAt)

	episode := page.Events[1]
	require.Equal(t, integrations.MediaEpisode, episode.Item.MediaType)
	require.Equal(t, "Synthetic Series", episode.Item.Title)
	require.Equal(t, 2, episode.Item.SeasonNumber)
	require.Equal(t, 3, episode.Item.EpisodeNumber)
	require.Equal(t, "episode-1", episode.Item.ProviderItemID)
	require.Equal(t, "66732", episode.Item.TVDBID)

	repeat := page.Events[2]
	require.Equal(t, movie.Item, repeat.Item)
	require.NotEqual(t, movie.ID, repeat.ID)
	require.True(t, repeat.PlayedAt.After(movie.PlayedAt))
}

func TestHistoryUsesStableRowCursorForPagination(t *testing.T) {
	client := newScenarioClient(t, "success")
	first, err := client.History(context.Background(), integrations.HistoryRequest{Limit: 2})
	require.NoError(t, err)
	require.Len(t, first.Events, 2)

	second, err := client.History(context.Background(), integrations.HistoryRequest{
		Cursor: first.NextCursor,
		Limit:  2,
	})

	require.NoError(t, err)
	require.Len(t, second.Events, 1)
	require.Equal(t, "jellyfin:103", second.Events[0].ID)
	require.Equal(t, "2026-07-13|103", second.NextCursor)
}

func TestDeletedUsersMalformedResponsesAndUpstreamFailuresUseStableErrors(t *testing.T) {
	t.Run("deleted user", func(t *testing.T) {
		err := newScenarioClient(t, "deleted_user").CheckAuthentication(context.Background())
		require.ErrorIs(t, err, integrations.ErrAuthentication)
		require.NotContains(t, err.Error(), testToken)
	})

	t.Run("malformed history", func(t *testing.T) {
		_, err := newScenarioClient(t, "malformed_history").History(
			context.Background(), integrations.HistoryRequest{Limit: 2},
		)
		require.ErrorIs(t, err, integrations.ErrInvalidResponse)
	})

	t.Run("null history", func(t *testing.T) {
		_, err := newScenarioClient(t, "null_history").History(
			context.Background(), integrations.HistoryRequest{Limit: 2},
		)
		require.ErrorIs(t, err, integrations.ErrInvalidResponse)
	})

	t.Run("malformed item", func(t *testing.T) {
		_, err := newScenarioClient(t, "malformed_item").History(
			context.Background(), integrations.HistoryRequest{Limit: 2},
		)
		require.ErrorIs(t, err, integrations.ErrInvalidResponse)
	})

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

func TestHistoryRejectsMalformedCursorWithoutMakingRequest(t *testing.T) {
	client := NewClient(ClientOptions{
		BaseURL: "https://media.example.test", Token: testToken, UserID: testUserID,
	})

	_, err := client.History(context.Background(), integrations.HistoryRequest{
		Cursor: "not-a-valid-cursor", Limit: 2,
	})

	require.ErrorIs(t, err, integrations.ErrInvalidResponse)
}

func TestHistoryTraversesRequestedUTCDateRange(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, testToken, r.Header.Get("X-Emby-Token"))
		switch r.URL.Path {
		case "/user_usage_stats/user-1/2026-07-12/GetItems":
			_, _ = w.Write([]byte("[]"))
		case "/user_usage_stats/user-1/2026-07-13/GetItems":
			writeFixture(t, w, "history.json")
		case "/Items/movie-1":
			writeFixture(t, w, "movie.json")
		case "/Items/episode-1":
			writeFixture(t, w, "episode.json")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	client := NewClient(ClientOptions{BaseURL: server.URL, Token: testToken, UserID: testUserID})

	page, err := client.History(context.Background(), integrations.HistoryRequest{
		Since: time.Date(2026, 7, 12, 8, 0, 0, 0, time.FixedZone("west", -7*60*60)),
		Until: time.Date(2026, 7, 13, 23, 0, 0, 0, time.UTC),
		Limit: 500,
	})

	require.NoError(t, err)
	require.Len(t, page.Events, 3)
	require.Equal(t, "2026-07-13|103", page.NextCursor)
	_, err = client.History(context.Background(), integrations.HistoryRequest{
		Since: time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC),
		Until: time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
	})
	require.ErrorIs(t, err, integrations.ErrInvalidResponse)
}

func TestClientHandlesMissingConfigurationTimeoutAndHTTPFailures(t *testing.T) {
	unconfigured := NewClient(ClientOptions{})
	require.ErrorIs(t, unconfigured.CheckAuthentication(context.Background()), integrations.ErrAuthentication)
	_, err := unconfigured.History(context.Background(), integrations.HistoryRequest{})
	require.ErrorIs(t, err, integrations.ErrAuthentication)

	t.Run("timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			time.Sleep(100 * time.Millisecond)
		}))
		t.Cleanup(server.Close)
		client := NewClient(ClientOptions{
			BaseURL: server.URL, Token: testToken, UserID: testUserID, Timeout: 10 * time.Millisecond,
		})
		_, err := client.History(context.Background(), integrations.HistoryRequest{})
		require.ErrorIs(t, err, integrations.ErrUnavailable)
		require.True(t, integrations.IsRetryable(err))
	})

	for name, status := range map[string]int{
		"unauthorized": http.StatusUnauthorized,
		"invalid":      http.StatusTeapot,
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
			}))
			t.Cleanup(server.Close)
			client := NewClient(ClientOptions{BaseURL: server.URL, Token: testToken, UserID: testUserID})
			_, err := client.History(context.Background(), integrations.HistoryRequest{})
			if status == http.StatusUnauthorized {
				require.ErrorIs(t, err, integrations.ErrAuthentication)
			} else {
				require.ErrorIs(t, err, integrations.ErrInvalidResponse)
			}
		})
	}
}

func TestMapperRejectsMalformedPlaybackFacts(t *testing.T) {
	validRow := activityRow{
		PlayedAt: "2026-07-13T09:00:00Z", ItemID: "movie-1", Type: "Movie",
		Duration: "100", RowID: "1", numericRowID: 1,
	}
	validItem := itemResponse{ID: "movie-1", Name: "Movie", Type: "Movie", RunTimeTicks: ticksPerSecond}
	for name, mutate := range map[string]func(*activityRow, *itemResponse){
		"invalid time":     func(row *activityRow, _ *itemResponse) { row.PlayedAt = "not-a-time" },
		"invalid duration": func(row *activityRow, _ *itemResponse) { row.Duration = "-1" },
		"item mismatch":    func(_ *activityRow, item *itemResponse) { item.ID = "other" },
		"type mismatch":    func(_ *activityRow, item *itemResponse) { item.Type = "Episode" },
		"unknown type": func(row *activityRow, item *itemResponse) {
			row.Type = "Audio"
			item.Type = "Audio"
		},
		"negative runtime": func(_ *activityRow, item *itemResponse) { item.RunTimeTicks = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			row, item := validRow, validItem
			mutate(&row, &item)
			_, err := mapHistoryEvent(row, item)
			require.Error(t, err)
		})
	}
}

func TestRetryAfterAcceptsHTTPDates(t *testing.T) {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	require.Equal(t, 30*time.Second, parseRetryAfter(now.Add(30*time.Second).Format(http.TimeFormat), now))
	require.Zero(t, parseRetryAfter("invalid", now))
}

func newScenarioClient(t *testing.T, scenario string) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, testToken, r.Header.Get("X-Emby-Token"))
		require.Empty(t, r.URL.Query().Get("api_key"))
		if scenario == "deleted_user" && strings.HasPrefix(r.URL.Path, "/Users/") {
			http.NotFound(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/Users/") {
			writeFixture(t, w, "user.json")
			return
		}
		if strings.Contains(r.URL.Path, "/GetItems") {
			require.Equal(t, "/user_usage_stats/user-1/2026-07-13/GetItems", r.URL.Path)
			require.Equal(t, "Movie,Episode", r.URL.Query().Get("filter"))
			switch scenario {
			case "rate_limited":
				w.Header().Set("Retry-After", "45")
				w.WriteHeader(http.StatusTooManyRequests)
			case "server_failure":
				http.Error(w, "upstream echoed "+testToken, http.StatusServiceUnavailable)
			case "malformed_history":
				writeFixture(t, w, "malformed.json")
			case "null_history":
				_, _ = w.Write([]byte("null"))
			default:
				writeFixture(t, w, "history.json")
			}
			return
		}
		if strings.HasPrefix(r.URL.Path, "/Items/") {
			if scenario == "malformed_item" {
				writeFixture(t, w, "malformed.json")
				return
			}
			switch r.URL.Path {
			case "/Items/movie-1":
				writeFixture(t, w, "movie.json")
			case "/Items/episode-1":
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
		BaseURL: server.URL,
		Token:   testToken,
		UserID:  testUserID,
		Now: func() time.Time {
			return time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
		},
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

func TestProviderErrorsPreserveRetryDelayWithoutRawCauses(t *testing.T) {
	client := newScenarioClient(t, "rate_limited")
	_, err := client.History(context.Background(), integrations.HistoryRequest{Limit: 1})
	require.Error(t, err)
	require.Equal(t, 45*time.Second, integrations.RetryAfter(err))
	require.False(t, errors.Is(err, context.Canceled))
}
