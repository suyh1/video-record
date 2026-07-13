package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"video-record/internal/auth"
	"video-record/internal/media"
	"video-record/internal/records"
	"video-record/internal/storage"
)

func TestRecordHandlersEnforceVersionAndCurrentUserOwnership(t *testing.T) {
	router, cookie, csrfToken, mediaID, _, _ := newRecordsTestRouter(t)
	headers := map[string]string{
		"Cookie":       cookie.String(),
		"Origin":       "http://example.test",
		"X-CSRF-Token": csrfToken,
		"If-Match":     `"0"`,
	}

	updated := performJSONRequest(router, http.MethodPut, "http://example.test/api/v1/records/"+mediaID, map[string]any{
		"status": "completed",
		"rating": 8.7,
		"note":   "很喜欢",
	}, headers)
	require.Equal(t, http.StatusOK, updated.Code)
	require.Equal(t, `"1"`, updated.Header().Get("ETag"))
	require.Contains(t, updated.Body.String(), `"rating":8.7`)
	require.Contains(t, updated.Body.String(), `"version":1`)

	conflict := performJSONRequest(router, http.MethodPut, "http://example.test/api/v1/records/"+mediaID, map[string]any{
		"status": "wishlist",
	}, headers)
	require.Equal(t, http.StatusConflict, conflict.Code)
	require.Contains(t, conflict.Body.String(), `"code":"version_conflict"`)

	tagsHeaders := cloneHeaders(headers)
	tagsHeaders["If-Match"] = `"1"`
	tags := performJSONRequest(router, http.MethodPut, "http://example.test/api/v1/records/"+mediaID+"/tags", map[string]any{
		"tags": []string{"科幻", "家庭"},
	}, tagsHeaders)
	require.Equal(t, http.StatusNoContent, tags.Code)

	collection := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/collections", map[string]string{
		"name": "周末电影",
	}, tagsHeaders)
	require.Equal(t, http.StatusCreated, collection.Code)
	var collectionFields map[string]any
	require.NoError(t, json.Unmarshal(collection.Body.Bytes(), &collectionFields))
	require.Contains(t, collectionFields, "id")
	require.Contains(t, collectionFields, "name")
	require.Contains(t, collectionFields, "items")
	require.NotContains(t, collectionFields, "userId")
	require.NotContains(t, collectionFields, "UserID")
	var collectionBody struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(collection.Body.Bytes(), &collectionBody))
	added := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/collections/"+collectionBody.ID+"/items", map[string]string{
		"mediaId": mediaID,
	}, tagsHeaders)
	require.Equal(t, http.StatusNoContent, added.Code)

	listed := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/collections", nil, map[string]string{
		"Cookie": cookie.String(),
	})
	require.Equal(t, http.StatusOK, listed.Code)
	require.Contains(t, listed.Body.String(), mediaID)
}

func TestRecordHandlerClearsExplicitNullRatingAndNote(t *testing.T) {
	router, cookie, csrfToken, mediaID, _, _ := newRecordsTestRouter(t)
	headers := map[string]string{
		"Cookie":       cookie.String(),
		"Origin":       "http://example.test",
		"X-CSRF-Token": csrfToken,
		"If-Match":     `"0"`,
	}
	created := performJSONRequest(router, http.MethodPut, "http://example.test/api/v1/records/"+mediaID, map[string]any{
		"status": "completed",
		"rating": 8.7,
		"note":   "很喜欢",
	}, headers)
	require.Equal(t, http.StatusOK, created.Code)

	headers["If-Match"] = `"1"`
	cleared := performJSONRequest(router, http.MethodPut, "http://example.test/api/v1/records/"+mediaID, map[string]any{
		"status": "completed",
		"rating": nil,
		"note":   nil,
	}, headers)
	require.Equal(t, http.StatusOK, cleared.Code)
	require.Equal(t, `"2"`, cleared.Header().Get("ETag"))
	require.Contains(t, cleared.Body.String(), `"rating":null`)
	require.Contains(t, cleared.Body.String(), `"note":null`)
}

func newRecordsTestRouter(t *testing.T) (http.Handler, *http.Cookie, string, string, *records.Service, *storage.DB) {
	t.Helper()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(context.Background(), db))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	authService := auth.NewService(auth.NewRepository(db), auth.ServiceOptions{})
	_, err = authService.Initialize(context.Background(), "owner", "correct horse battery staple")
	require.NoError(t, err)
	mediaService := media.NewService(media.NewRepository(db))
	item, err := mediaService.CreateCustom(context.Background(), media.CreateCustomInput{
		MediaType: media.MediaTypeMovie, Title: "测试电影",
	})
	require.NoError(t, err)
	recordService := records.NewService(records.NewRepository(db))
	router := NewRouter(Dependencies{
		Storage: db,
		Auth:    authService,
		Records: recordService,
	})
	cookie, csrfToken := loginForHTTPTest(t, router)
	return router, cookie, csrfToken, item.ID, recordService, db
}

func cloneHeaders(headers map[string]string) map[string]string {
	cloned := make(map[string]string, len(headers))
	for key, value := range headers {
		cloned[key] = value
	}
	return cloned
}
