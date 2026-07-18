package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"video-record/internal/media"
)

func TestCurrentRoundHandlersReadAndWriteMovie(t *testing.T) {
	router, cookie, csrfToken, mediaID, _, _ := newRecordsTestRouter(t)
	url := "http://example.test/api/v1/records/" + mediaID + "/rounds/current"

	read := performJSONRequest(router, http.MethodGet, url, nil, map[string]string{
		"Cookie": cookie.String(),
	})
	require.Equal(t, http.StatusOK, read.Code)
	require.Equal(t, `"0"`, read.Header().Get("ETag"))
	require.JSONEq(t, `{
		"roundId":"", "mediaId":"`+mediaID+`", "seasonNumber":null,
		"roundNumber":1, "status":"none", "rating":null, "note":null,
		"viewingMethod":null, "watchedAt":null, "version":0, "profileVersion":0,
		"participantIds":[]
	}`, read.Body.String())

	updated := performJSONRequest(router, http.MethodPut, url, map[string]any{
		"status": "completed", "rating": 8.7, "note": "第一轮",
		"viewingMethod": "家庭投影", "watchedAt": "2026-07-13T20:30:45Z",
	}, map[string]string{
		"Cookie": cookie.String(), "Origin": "http://example.test",
		"X-CSRF-Token": csrfToken, "Idempotency-Key": "movie-round-1",
		"If-Match": `"0"`,
	})
	require.Equal(t, http.StatusOK, updated.Code)
	require.Equal(t, `"1"`, updated.Header().Get("ETag"))
	require.Contains(t, updated.Body.String(), `"status":"completed"`)
	require.Contains(t, updated.Body.String(), `"rating":8.7`)
	require.Contains(t, updated.Body.String(), `"viewingMethod":"家庭投影"`)
	require.Contains(t, updated.Body.String(), `"watchedAt":"2026-07-13T20:30:45Z"`)
	require.Contains(t, updated.Body.String(), `"profileVersion":1`)
	require.Contains(t, updated.Body.String(), `"participantIds":[]`)

	replayed := performJSONRequest(router, http.MethodPut, url, map[string]any{
		"status": "completed", "rating": 8.7, "note": "第一轮",
		"viewingMethod": "家庭投影", "watchedAt": "2026-07-13T20:30:45Z",
	}, map[string]string{
		"Cookie": cookie.String(), "Origin": "http://example.test",
		"X-CSRF-Token": csrfToken, "Idempotency-Key": "movie-round-1",
		"If-Match": `"0"`,
	})
	require.Equal(t, http.StatusOK, replayed.Code)
	require.Equal(t, "true", replayed.Header().Get("Idempotency-Replayed"))
}

func TestCurrentRoundHandlersRequireCorrectSeasonScopeAndSecurity(t *testing.T) {
	router, cookie, csrfToken, movieID, _, db := newRecordsTestRouter(t)
	mediaService := media.NewService(media.NewRepository(db))
	series, err := mediaService.CreateCustom(context.Background(), media.CreateCustomInput{
		MediaType: media.MediaTypeTV, Title: "测试剧集",
	})
	require.NoError(t, err)
	seriesURL := "http://example.test/api/v1/records/" + series.ID + "/rounds/current"

	missingSeason := performJSONRequest(router, http.MethodGet, seriesURL, nil, map[string]string{
		"Cookie": cookie.String(),
	})
	require.Equal(t, http.StatusBadRequest, missingSeason.Code)
	require.Contains(t, missingSeason.Body.String(), `"code":"invalid_round_scope"`)

	season := performJSONRequest(router, http.MethodGet, seriesURL+"?seasonNumber=2", nil, map[string]string{
		"Cookie": cookie.String(),
	})
	require.Equal(t, http.StatusOK, season.Code)
	require.Contains(t, season.Body.String(), `"seasonNumber":2`)

	movieWithSeason := performJSONRequest(
		router, http.MethodGet,
		"http://example.test/api/v1/records/"+movieID+"/rounds/current?seasonNumber=1",
		nil, map[string]string{"Cookie": cookie.String()},
	)
	require.Equal(t, http.StatusBadRequest, movieWithSeason.Code)
	require.Contains(t, movieWithSeason.Body.String(), `"code":"invalid_round_scope"`)

	withoutCSRF := performJSONRequest(router, http.MethodPut, seriesURL+"?seasonNumber=2", map[string]any{
		"status": "watching",
	}, map[string]string{
		"Cookie": cookie.String(), "Origin": "http://example.test",
		"Idempotency-Key": "season-round-no-csrf", "If-Match": `"0"`,
	})
	require.Equal(t, http.StatusForbidden, withoutCSRF.Code)

	created := performJSONRequest(router, http.MethodPut, seriesURL+"?seasonNumber=2", map[string]any{
		"status": "watching",
	}, map[string]string{
		"Cookie": cookie.String(), "Origin": "http://example.test",
		"X-CSRF-Token": csrfToken, "Idempotency-Key": "season-round-2",
		"If-Match": `"0"`,
	})
	require.Equal(t, http.StatusOK, created.Code)
	require.Contains(t, created.Body.String(), `"status":"watching"`)
}

func TestCurrentRoundHandlersPreserveExistingCompletionTimeWhenUpdatingPrivateFields(t *testing.T) {
	router, cookie, csrfToken, _, _, db := newRecordsTestRouter(t)
	mediaService := media.NewService(media.NewRepository(db))
	series, err := mediaService.CreateCustom(context.Background(), media.CreateCustomInput{
		MediaType: media.MediaTypeTV, Title: "测试剧集",
	})
	require.NoError(t, err)
	url := "http://example.test/api/v1/records/" + series.ID + "/rounds/current?seasonNumber=2"

	completed := performJSONRequest(router, http.MethodPut, url, map[string]any{
		"status": "completed", "watchedAt": "2026-07-13T20:30:45Z",
	}, map[string]string{
		"Cookie": cookie.String(), "Origin": "http://example.test",
		"X-CSRF-Token": csrfToken, "Idempotency-Key": "complete-season-round",
		"If-Match": `"0"`,
	})
	require.Equal(t, http.StatusOK, completed.Code, completed.Body.String())

	updated := performJSONRequest(router, http.MethodPut, url, map[string]any{
		"status": "completed", "rating": 9.1, "note": "只更新私人记录",
	}, map[string]string{
		"Cookie": cookie.String(), "Origin": "http://example.test",
		"X-CSRF-Token": csrfToken, "Idempotency-Key": "update-completed-season-record",
		"If-Match": `"1"`,
	})
	require.Equal(t, http.StatusOK, updated.Code, updated.Body.String())
	require.Contains(t, updated.Body.String(), `"rating":9.1`)
	require.Contains(t, updated.Body.String(), `"note":"只更新私人记录"`)
	require.Contains(t, updated.Body.String(), `"watchedAt":"2026-07-13T20:30:45Z"`)
}

func TestCurrentRoundHandlersRejectFutureTimeAndStaleVersion(t *testing.T) {
	router, cookie, csrfToken, mediaID, _, _ := newRecordsTestRouter(t)
	url := "http://example.test/api/v1/records/" + mediaID + "/rounds/current"
	headers := map[string]string{
		"Cookie": cookie.String(), "Origin": "http://example.test",
		"X-CSRF-Token": csrfToken, "Idempotency-Key": "future-round",
		"If-Match": `"0"`,
	}

	future := performJSONRequest(router, http.MethodPut, url, map[string]any{
		"status": "completed", "watchedAt": "2999-01-01T00:00:00Z",
	}, headers)
	require.Equal(t, http.StatusBadRequest, future.Code)
	require.Contains(t, future.Body.String(), `"code":"invalid_watched_at"`)

	headers["Idempotency-Key"] = "create-current-round"
	created := performJSONRequest(router, http.MethodPut, url, map[string]any{
		"status": "watching",
	}, headers)
	require.Equal(t, http.StatusOK, created.Code)

	headers["Idempotency-Key"] = "stale-current-round"
	stale := performJSONRequest(router, http.MethodPut, url, map[string]any{
		"status": "dropped",
	}, headers)
	require.Equal(t, http.StatusConflict, stale.Code)
	require.Equal(t, `"1"`, stale.Header().Get("ETag"))
}

func TestViewingMethodsHandlerReturnsTopMethods(t *testing.T) {
	router, cookie, csrfToken, mediaID, _, db := newRecordsTestRouter(t)
	mediaService := media.NewService(media.NewRepository(db))
	second, err := mediaService.CreateCustom(context.Background(), media.CreateCustomInput{
		MediaType: media.MediaTypeMovie, Title: "第二部电影",
	})
	require.NoError(t, err)

	first := performJSONRequest(router, http.MethodPut, "http://example.test/api/v1/records/"+mediaID+"/rounds/current", map[string]any{
		"status": "completed", "viewingMethod": "影院", "watchedAt": "2026-07-13T20:30:45Z",
	}, map[string]string{
		"Cookie": cookie.String(), "Origin": "http://example.test",
		"X-CSRF-Token": csrfToken, "Idempotency-Key": "method-1", "If-Match": `"0"`,
	})
	require.Equal(t, http.StatusOK, first.Code)
	secondSave := performJSONRequest(router, http.MethodPut, "http://example.test/api/v1/records/"+second.ID+"/rounds/current", map[string]any{
		"status": "watching", "viewingMethod": "家庭电视",
	}, map[string]string{
		"Cookie": cookie.String(), "Origin": "http://example.test",
		"X-CSRF-Token": csrfToken, "Idempotency-Key": "method-2", "If-Match": `"0"`,
	})
	require.Equal(t, http.StatusOK, secondSave.Code)

	response := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/records/viewing-methods", nil, map[string]string{
		"Cookie": cookie.String(),
	})
	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(t, `{"methods":["家庭电视","影院"]}`, response.Body.String())
}

func TestRewatchRoundAndRoundHistoryHandlers(t *testing.T) {
	router, cookie, csrfToken, mediaID, _, _ := newRecordsTestRouter(t)
	currentURL := "http://example.test/api/v1/records/" + mediaID + "/rounds/current"
	completed := performJSONRequest(router, http.MethodPut, currentURL, map[string]any{
		"status": "completed", "rating": 9.1, "note": "第一轮",
		"watchedAt": "2026-07-13T20:30:45Z",
	}, map[string]string{
		"Cookie": cookie.String(), "Origin": "http://example.test",
		"X-CSRF-Token": csrfToken, "Idempotency-Key": "complete-before-rewatch",
		"If-Match": `"0"`,
	})
	require.Equal(t, http.StatusOK, completed.Code)

	rewatchURL := currentURL + "/rewatch"
	headers := map[string]string{
		"Cookie": cookie.String(), "Origin": "http://example.test",
		"X-CSRF-Token": csrfToken, "Idempotency-Key": "rewatch-movie-1",
		"If-Match": `"1"`,
	}
	rewatched := performJSONRequest(router, http.MethodPost, rewatchURL, map[string]any{}, headers)
	require.Equal(t, http.StatusOK, rewatched.Code)
	require.Contains(t, rewatched.Body.String(), `"roundNumber":2`)
	require.Contains(t, rewatched.Body.String(), `"status":"watching"`)
	require.Contains(t, rewatched.Body.String(), `"note":"第一轮"`)

	replayed := performJSONRequest(router, http.MethodPost, rewatchURL, map[string]any{}, headers)
	require.Equal(t, http.StatusOK, replayed.Code)
	require.Equal(t, "true", replayed.Header().Get("Idempotency-Replayed"))
	require.Equal(t, rewatched.Body.String(), replayed.Body.String())

	historyURL := "http://example.test/api/v1/records/" + mediaID + "/rounds"
	history := performJSONRequest(router, http.MethodGet, historyURL, nil, map[string]string{
		"Cookie": cookie.String(),
	})
	require.Equal(t, http.StatusOK, history.Code)
	require.Contains(t, history.Body.String(), `"roundNumber":1`)
	var payload struct {
		Rounds []struct {
			RoundID string `json:"roundId"`
		} `json:"rounds"`
	}
	require.NoError(t, json.Unmarshal(history.Body.Bytes(), &payload))
	require.Len(t, payload.Rounds, 1)

	detail := performJSONRequest(
		router, http.MethodGet, historyURL+"/"+payload.Rounds[0].RoundID, nil,
		map[string]string{"Cookie": cookie.String()},
	)
	require.Equal(t, http.StatusOK, detail.Code)
	require.Contains(t, detail.Body.String(), `"note":"第一轮"`)
	require.Contains(t, detail.Body.String(), `"episodes":[]`)
}

func TestRewatchRoundHandlersRequireCompletionAndSecurity(t *testing.T) {
	router, cookie, csrfToken, mediaID, _, _ := newRecordsTestRouter(t)
	currentURL := "http://example.test/api/v1/records/" + mediaID + "/rounds/current"
	watching := performJSONRequest(router, http.MethodPut, currentURL, map[string]any{
		"status": "watching",
	}, map[string]string{
		"Cookie": cookie.String(), "Origin": "http://example.test",
		"X-CSRF-Token": csrfToken, "Idempotency-Key": "watching-before-rewatch",
		"If-Match": `"0"`,
	})
	require.Equal(t, http.StatusOK, watching.Code)

	rewatchURL := currentURL + "/rewatch"
	withoutCSRF := performJSONRequest(router, http.MethodPost, rewatchURL, map[string]any{}, map[string]string{
		"Cookie": cookie.String(), "Origin": "http://example.test",
		"Idempotency-Key": "rewatch-no-csrf", "If-Match": `"1"`,
	})
	require.Equal(t, http.StatusForbidden, withoutCSRF.Code)

	incomplete := performJSONRequest(router, http.MethodPost, rewatchURL, map[string]any{}, map[string]string{
		"Cookie": cookie.String(), "Origin": "http://example.test",
		"X-CSRF-Token": csrfToken, "Idempotency-Key": "rewatch-incomplete",
		"If-Match": `"1"`,
	})
	require.Equal(t, http.StatusConflict, incomplete.Code)
	require.Contains(t, incomplete.Body.String(), `"code":"round_not_completed"`)
}
