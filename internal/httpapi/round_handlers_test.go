package httpapi

import (
	"context"
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
		"viewingMethod":null, "watchedAt":null, "version":0, "profileVersion":0
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
