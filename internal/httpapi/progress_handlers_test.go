package httpapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"video-record/internal/media"
	"video-record/internal/records"
)

func TestEpisodeProgressHandlersUseCurrentUserCSRFAndIdempotency(t *testing.T) {
	router, cookie, csrfToken, _, service, db := newRecordsTestRouter(t)
	mediaService := media.NewService(media.NewRepository(db))
	series, err := mediaService.CreateCustom(context.Background(), media.CreateCustomInput{
		MediaType: media.MediaTypeTV, Title: "测试剧集",
	})
	require.NoError(t, err)
	seasonID := uuid.NewString()
	episodeID := uuid.NewString()
	_, err = db.Writer().ExecContext(context.Background(), `
		INSERT INTO seasons (id, media_id, season_number, name, overview, poster_path, air_date)
		VALUES (?, ?, 2, '第 2 季', '', '', '')
	`, seasonID, series.ID)
	require.NoError(t, err)
	_, err = db.Writer().ExecContext(context.Background(), `
		INSERT INTO episodes (id, season_id, episode_number, name, overview, still_path, air_date)
		VALUES (?, ?, 3, '第三集', '', '', '')
	`, episodeID, seasonID)
	require.NoError(t, err)

	progressURL := "http://example.test/api/v1/records/" + series.ID + "/progress?seasonNumber=2"
	read := performJSONRequest(router, http.MethodGet, progressURL, nil, map[string]string{
		"Cookie": cookie.String(),
	})
	require.Equal(t, http.StatusOK, read.Code)
	require.Contains(t, read.Body.String(), `"seasonNumber":2`)
	require.Contains(t, read.Body.String(), `"roundId":""`)
	require.Contains(t, read.Body.String(), `"episodeNumber":3`)
	require.Contains(t, read.Body.String(), `"absoluteNumber":1`)
	require.NotContains(t, read.Body.String(), "UserID")

	headers := map[string]string{
		"Cookie": cookie.String(), "Origin": "http://example.test",
		"X-CSRF-Token": csrfToken, "Idempotency-Key": "episode-next-1",
	}
	body := map[string]any{
		"action": "next", "expectedVersion": 0, "watchedAt": "2026-07-13T12:00:00Z",
	}
	updated := performJSONRequest(router, http.MethodPost, progressURL, body, headers)
	require.Equal(t, http.StatusOK, updated.Code)
	require.Equal(t, `"1"`, updated.Header().Get("ETag"))
	require.Contains(t, updated.Body.String(), `"watchedEpisodes":1`)
	require.Contains(t, updated.Body.String(), `"status":"completed"`)
	require.NotContains(t, updated.Body.String(), `"roundId":""`)

	replayed := performJSONRequest(router, http.MethodPost, progressURL, body, headers)
	require.Equal(t, http.StatusOK, replayed.Code)
	require.Equal(t, "true", replayed.Header().Get("Idempotency-Replayed"))
	events, err := service.WatchEvents(context.Background(), currentUserID(t, router, cookie), series.ID)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, episodeID, events[0].EpisodeID)

	withoutCSRF := performJSONRequest(router, http.MethodPost, progressURL, body, map[string]string{
		"Cookie": cookie.String(), "Origin": "http://example.test", "Idempotency-Key": "episode-next-2",
	})
	require.Equal(t, http.StatusForbidden, withoutCSRF.Code)

	missingSeason := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/records/"+series.ID+"/progress", nil, map[string]string{
		"Cookie": cookie.String(),
	})
	require.Equal(t, http.StatusBadRequest, missingSeason.Code)
	require.Contains(t, missingSeason.Body.String(), `"code":"invalid_episode_progress"`)
}

func TestEpisodeProgressHandlersAcceptSparseExternalReferences(t *testing.T) {
	router, cookie, csrfToken, _, service, db := newRecordsTestRouter(t)
	mediaService := media.NewService(media.NewRepository(db))
	series, err := mediaService.UpsertExternal(context.Background(), media.ExternalSnapshot{
		Source: "tmdb", SourceID: "1399", MediaType: media.MediaTypeTV, Title: "权力的游戏",
	})
	require.NoError(t, err)
	headers := map[string]string{
		"Cookie": cookie.String(), "Origin": "http://example.test",
		"X-CSRF-Token": csrfToken, "Idempotency-Key": "sparse-progress-1",
	}
	body := map[string]any{
		"action": "range", "expectedVersion": 0, "totalEpisodes": 12,
		"watchedAt": "2026-07-14T12:00:00Z",
		"episodeRefs": []map[string]any{
			{"sourceId": "63056", "seasonNumber": 1, "episodeNumber": 1, "absoluteNumber": 1},
			{"sourceId": "63057", "seasonNumber": 1, "episodeNumber": 2, "absoluteNumber": 2},
		},
	}

	progressURL := "http://example.test/api/v1/records/" + series.ID + "/progress?seasonNumber=1"
	updated := performJSONRequest(router, http.MethodPost, progressURL, body, headers)
	require.Equal(t, http.StatusOK, updated.Code)
	require.Equal(t, `"1"`, updated.Header().Get("ETag"))
	require.Contains(t, updated.Body.String(), `"sourceId":"63056"`)
	require.Contains(t, updated.Body.String(), `"watchedEpisodes":2`)
	require.Contains(t, updated.Body.String(), `"totalEpisodes":12`)
	require.Len(t, mustHTTPWatchEvents(t, service, currentUserID(t, router, cookie), series.ID), 2)

	replayed := performJSONRequest(router, http.MethodPost, progressURL, body, headers)
	require.Equal(t, http.StatusOK, replayed.Code)
	require.Equal(t, "true", replayed.Header().Get("Idempotency-Replayed"))
	require.Len(t, mustHTTPWatchEvents(t, service, currentUserID(t, router, cookie), series.ID), 2)

	invalidHeaders := cloneHeaders(headers)
	invalidHeaders["Idempotency-Key"] = "sparse-progress-invalid"
	invalidBody := map[string]any{
		"action": "single", "expectedVersion": 1, "totalEpisodes": 12,
		"episodeRefs": []map[string]any{
			{"sourceId": "63058", "seasonNumber": 1, "episodeNumber": 3, "absoluteNumber": 3},
			{"sourceId": "63058", "seasonNumber": 1, "episodeNumber": 3, "absoluteNumber": 3},
		},
	}
	invalid := performJSONRequest(router, http.MethodPost, progressURL, invalidBody, invalidHeaders)
	require.Equal(t, http.StatusBadRequest, invalid.Code)
	require.Contains(t, invalid.Body.String(), `"code":"invalid_episode_progress"`)
}

func mustHTTPWatchEvents(t *testing.T, service interface {
	WatchEvents(context.Context, string, string) ([]records.WatchEvent, error)
}, userID, mediaID string) []records.WatchEvent {
	t.Helper()
	events, err := service.WatchEvents(context.Background(), userID, mediaID)
	require.NoError(t, err)
	return events
}
