package plex

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"video-record/internal/integrations"
	"video-record/internal/integrations/providertest"
)

const testToken = "synthetic-plex-token"

func TestProviderConformance(t *testing.T) {
	providertest.Run(t, func(t *testing.T, scenario string) integrations.Provider {
		return newScenarioClient(t, scenario)
	}, providertest.Expectations{
		FirstCursor: "2", SecondCursor: "3", Secret: testToken,
	})
}

func TestHistoryMapsStableRatingKeysMoviesEpisodesAndRepeatPlays(t *testing.T) {
	page, err := newScenarioClient(t, "all_xml").History(
		context.Background(), integrations.HistoryRequest{Limit: 10},
	)

	require.NoError(t, err)
	require.Len(t, page.Events, 3)
	require.Equal(t, "3", page.NextCursor)
	movie := page.Events[0]
	require.Equal(t, "plex:/status/sessions/history/301", movie.ID)
	require.Equal(t, integrations.MediaMovie, movie.Item.MediaType)
	require.Equal(t, "movie-rating-key", movie.Item.ProviderItemID)
	require.Equal(t, "603", movie.Item.TMDBID)
	require.Equal(t, "tt0133093", movie.Item.IMDbID)
	require.Equal(t, 8160, movie.DurationSeconds)
	require.Equal(t, 8000, movie.PositionSeconds)
	require.Equal(t, time.Unix(1783933200, 0).UTC(), movie.PlayedAt)

	episode := page.Events[1]
	require.Equal(t, integrations.MediaEpisode, episode.Item.MediaType)
	require.Equal(t, "Plex Synthetic Series", episode.Item.Title)
	require.Equal(t, "episode-rating-key", episode.Item.ProviderItemID)
	require.Equal(t, 3, episode.Item.SeasonNumber)
	require.Equal(t, 7, episode.Item.EpisodeNumber)
	require.Equal(t, "76543", episode.Item.TVDBID)

	repeat := page.Events[2]
	require.Equal(t, movie.Item, repeat.Item)
	require.NotEqual(t, movie.ID, repeat.ID)
	require.True(t, repeat.PlayedAt.After(movie.PlayedAt))
}

func TestHistoryParsesPlexJSONWithoutPropagatingUnknownFields(t *testing.T) {
	page, err := newScenarioClient(t, "all_json").History(
		context.Background(), integrations.HistoryRequest{Limit: 10},
	)

	require.NoError(t, err)
	require.Len(t, page.Events, 3)
	require.Equal(t, "movie-rating-key", page.Events[0].Item.ProviderItemID)
	require.Equal(t, "603", page.Events[0].Item.TMDBID)
}

func TestHistoryUsesPlexContainerPaginationAndTimeFilters(t *testing.T) {
	client := newScenarioClient(t, "success")
	first, err := client.History(context.Background(), integrations.HistoryRequest{
		Limit: 2,
		Since: time.Unix(1783930000, 0).UTC(),
		Until: time.Unix(1783940000, 0).UTC(),
	})
	require.NoError(t, err)
	require.Len(t, first.Events, 2)
	require.Equal(t, "2", first.NextCursor)
	second, err := client.History(context.Background(), integrations.HistoryRequest{
		Cursor: first.NextCursor, Limit: 2,
	})
	require.NoError(t, err)
	require.Len(t, second.Events, 1)
	require.Equal(t, "3", second.NextCursor)
}

func TestPlexRejectsDuplicateEventsMalformedResponsesAndInvalidCursor(t *testing.T) {
	for _, scenario := range []string{"duplicate", "malformed_xml", "malformed_json", "missing_history_key"} {
		t.Run(scenario, func(t *testing.T) {
			_, err := newScenarioClient(t, scenario).History(
				context.Background(), integrations.HistoryRequest{Limit: 10},
			)
			require.ErrorIs(t, err, integrations.ErrInvalidResponse)
		})
	}
	client := NewClient(ClientOptions{BaseURL: "https://plex.example.test", Token: testToken, AccountID: 42})
	_, err := client.History(context.Background(), integrations.HistoryRequest{Cursor: "invalid"})
	require.ErrorIs(t, err, integrations.ErrInvalidResponse)
}

func TestPlexAuthenticationRateLimitServerFailureAndCancellation(t *testing.T) {
	err := newScenarioClient(t, "unauthorized").CheckAuthentication(context.Background())
	require.ErrorIs(t, err, integrations.ErrAuthentication)
	_, err = newScenarioClient(t, "rate_limited").History(
		context.Background(), integrations.HistoryRequest{Limit: 1},
	)
	require.ErrorIs(t, err, integrations.ErrRateLimited)
	require.Equal(t, 90*time.Second, integrations.RetryAfter(err))
	_, err = newScenarioClient(t, "server_failure").History(
		context.Background(), integrations.HistoryRequest{Limit: 1},
	)
	require.ErrorIs(t, err, integrations.ErrUnavailable)
	require.True(t, integrations.IsRetryable(err))
	require.NotContains(t, err.Error(), testToken)
	require.NotContains(t, err.Error(), "upstream echoed")

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = newScenarioClient(t, "success").History(canceled, integrations.HistoryRequest{})
	require.ErrorIs(t, err, context.Canceled)
}

func TestPlexAuthenticationParsesJSONAndRejectsInvalidEnvelopes(t *testing.T) {
	require.NoError(t, newScenarioClient(t, "json_auth").CheckAuthentication(context.Background()))
	for _, scenario := range []string{"null_auth", "wrong_xml_root"} {
		t.Run(scenario, func(t *testing.T) {
			err := newScenarioClient(t, scenario).CheckAuthentication(context.Background())
			require.ErrorIs(t, err, integrations.ErrInvalidResponse)
		})
	}
}

func TestPlexTimeoutAndHTTPDateRetryAfter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		time.Sleep(100 * time.Millisecond)
	}))
	t.Cleanup(server.Close)
	client := NewClient(ClientOptions{
		BaseURL: server.URL, Token: testToken, AccountID: 42, Timeout: 10 * time.Millisecond,
	})
	_, err := client.History(context.Background(), integrations.HistoryRequest{})
	require.ErrorIs(t, err, integrations.ErrUnavailable)

	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	require.Equal(t, 30*time.Second, parseRetryAfter(now.Add(30*time.Second).Format(http.TimeFormat), now))
	require.Zero(t, parseRetryAfter("invalid", now))
}

func TestPlexRejectsInconsistentContainerMetadata(t *testing.T) {
	_, err := newScenarioClient(t, "bad_container").History(
		context.Background(), integrations.HistoryRequest{Limit: 10},
	)
	require.ErrorIs(t, err, integrations.ErrInvalidResponse)
}

func TestPlexRequiresServerTokenAndAccount(t *testing.T) {
	client := NewClient(ClientOptions{})
	require.ErrorIs(t, client.CheckAuthentication(context.Background()), integrations.ErrAuthentication)
	_, err := client.History(context.Background(), integrations.HistoryRequest{})
	require.ErrorIs(t, err, integrations.ErrAuthentication)
}

func newScenarioClient(t *testing.T, scenario string) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, testToken, r.Header.Get("X-Plex-Token"))
		require.Empty(t, r.URL.Query().Get("X-Plex-Token"))
		if r.URL.Path == "/library/sections" {
			switch scenario {
			case "unauthorized":
				w.WriteHeader(http.StatusUnauthorized)
				return
			case "json_auth":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"MediaContainer":{"size":0}}`))
				return
			case "null_auth":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"MediaContainer":null}`))
				return
			case "wrong_xml_root":
				w.Header().Set("Content-Type", "application/xml")
				_, _ = w.Write([]byte(`<Other />`))
				return
			}
			writeFixture(t, w, "library.xml", "application/xml")
			return
		}
		if r.URL.Path != "/status/sessions/history/all" {
			http.NotFound(w, r)
			return
		}
		require.Equal(t, "42", r.URL.Query().Get("accountID"))
		require.Equal(t, "viewedAt:asc", r.URL.Query().Get("sort"))
		require.Equal(t, "1", r.URL.Query().Get("includeGuids"))
		if value := r.URL.Query().Get("viewedAt>="); value != "" {
			require.Equal(t, "1783930000", value)
			require.Equal(t, "1783940000", r.URL.Query().Get("viewedAt<="))
		}
		switch scenario {
		case "rate_limited":
			w.Header().Set("Retry-After", "90")
			w.WriteHeader(http.StatusTooManyRequests)
		case "server_failure":
			http.Error(w, "upstream echoed "+testToken, http.StatusServiceUnavailable)
		case "all_json":
			writeFixture(t, w, "history.json", "application/json")
		case "all_xml":
			writeFixture(t, w, "history-all.xml", "application/xml")
		case "duplicate":
			writeFixture(t, w, "history-duplicate.xml", "application/xml")
		case "missing_history_key":
			writeFixture(t, w, "history-missing-key.xml", "application/xml")
		case "bad_container":
			writeFixture(t, w, "history-bad-container.xml", "application/xml")
		case "malformed_xml":
			_, _ = w.Write([]byte("<MediaContainer><Video>"))
		case "malformed_json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"MediaContainer":null}`))
		default:
			start, _ := strconv.Atoi(r.URL.Query().Get("X-Plex-Container-Start"))
			if start == 0 {
				writeFixture(t, w, "history-page-1.xml", "application/xml")
			} else {
				writeFixture(t, w, "history-page-2.xml", "application/xml")
			}
		}
	}))
	t.Cleanup(server.Close)
	return NewClient(ClientOptions{
		BaseURL: server.URL, Token: testToken, AccountID: 42,
	})
}

func writeFixture(t *testing.T, w http.ResponseWriter, name, contentType string) {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	w.Header().Set("Content-Type", contentType)
	_, err = w.Write(contents)
	require.NoError(t, err)
}

func TestPlexLegacyAgentGUIDsMapToExternalIDs(t *testing.T) {
	identity, err := mapIdentity(historyVideo{
		RatingKey: "legacy", Type: "movie", Title: "Legacy",
		GUIDs: []plexGUID{
			{ID: "com.plexapp.agents.themoviedb://123?lang=en"},
			{ID: "com.plexapp.agents.imdb://tt1234567?lang=en"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, "123", identity.TMDBID)
	require.Equal(t, "tt1234567", identity.IMDbID)
}

func TestPlexEventIDsDoNotUseOnlyRatingKeys(t *testing.T) {
	page, err := newScenarioClient(t, "all_xml").History(context.Background(), integrations.HistoryRequest{Limit: 10})
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(page.Events[0].ID, "plex:/status/sessions/history/"))
	require.NotEqual(t, page.Events[0].ID, page.Events[2].ID)
}
