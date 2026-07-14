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
	router, cookie, csrfToken, mediaID, recordService, _ := newRecordsTestRouter(t)
	headers := map[string]string{
		"Cookie":          cookie.String(),
		"Origin":          "http://example.test",
		"X-CSRF-Token":    csrfToken,
		"Idempotency-Key": "update-record",
		"If-Match":        `"0"`,
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

	conflictHeaders := cloneHeaders(headers)
	conflictHeaders["Idempotency-Key"] = "conflicting-record-update"
	conflict := performJSONRequest(router, http.MethodPut, "http://example.test/api/v1/records/"+mediaID, map[string]any{
		"status": "wishlist",
	}, conflictHeaders)
	require.Equal(t, http.StatusConflict, conflict.Code)
	require.Contains(t, conflict.Body.String(), `"code":"version_conflict"`)

	tagsHeaders := cloneHeaders(headers)
	tagsHeaders["If-Match"] = `"1"`
	tagsHeaders["Idempotency-Key"] = "set-record-tags"
	tags := performJSONRequest(router, http.MethodPut, "http://example.test/api/v1/records/"+mediaID+"/tags", map[string]any{
		"tags": []string{"科幻", "家庭"},
	}, tagsHeaders)
	require.Equal(t, http.StatusNoContent, tags.Code)
	require.Equal(t, `"2"`, tags.Header().Get("ETag"))

	readTags := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/records/"+mediaID+"/tags", nil, map[string]string{
		"Cookie": cookie.String(),
	})
	require.Equal(t, http.StatusOK, readTags.Code)
	require.Equal(t, `"2"`, readTags.Header().Get("ETag"))
	require.JSONEq(t, `{"tags":["家庭","科幻"]}`, readTags.Body.String())

	staleTagsHeaders := cloneHeaders(tagsHeaders)
	staleTagsHeaders["Idempotency-Key"] = "stale-record-tags"
	staleTags := performJSONRequest(router, http.MethodPut, "http://example.test/api/v1/records/"+mediaID+"/tags", map[string]any{
		"tags": []string{"覆盖失败"},
	}, staleTagsHeaders)
	require.Equal(t, http.StatusConflict, staleTags.Code)
	require.Equal(t, `"2"`, staleTags.Header().Get("ETag"))
	storedTags, err := recordService.Tags(context.Background(), currentUserID(t, router, cookie), mediaID)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"科幻", "家庭"}, storedTags)

	collectionHeaders := cloneHeaders(tagsHeaders)
	collectionHeaders["Idempotency-Key"] = "create-collection"
	collection := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/collections", map[string]string{
		"name": "周末电影",
	}, collectionHeaders)
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
	itemHeaders := cloneHeaders(tagsHeaders)
	itemHeaders["Idempotency-Key"] = "add-collection-item"
	added := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/collections/"+collectionBody.ID+"/items", map[string]string{
		"mediaId": mediaID,
	}, itemHeaders)
	require.Equal(t, http.StatusNoContent, added.Code)
	reorderHeaders := cloneHeaders(tagsHeaders)
	reorderHeaders["Idempotency-Key"] = "reorder-collection-items"
	reordered := performJSONRequest(router, http.MethodPut, "http://example.test/api/v1/collections/"+collectionBody.ID+"/items", map[string]any{
		"mediaIds": []string{},
	}, reorderHeaders)
	require.Equal(t, http.StatusNoContent, reordered.Code)

	listed := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/collections", nil, map[string]string{
		"Cookie": cookie.String(),
	})
	require.Equal(t, http.StatusOK, listed.Code)
	require.NotContains(t, listed.Body.String(), mediaID)
}

func TestRecordTagsReturnsEmptyArrayWhenNoTagsExist(t *testing.T) {
	router, cookie, csrfToken, mediaID, _, _ := newRecordsTestRouter(t)
	created := performJSONRequest(router, http.MethodPut, "http://example.test/api/v1/records/"+mediaID, map[string]any{
		"status": "wishlist",
	}, map[string]string{
		"Cookie":          cookie.String(),
		"Origin":          "http://example.test",
		"X-CSRF-Token":    csrfToken,
		"Idempotency-Key": "create-record-without-tags",
		"If-Match":        `"0"`,
	})
	require.Equal(t, http.StatusOK, created.Code)

	response := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/records/"+mediaID+"/tags", nil, map[string]string{
		"Cookie": cookie.String(),
	})
	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(t, `{"tags":[]}`, response.Body.String())
}

func TestRecordHandlerClearsExplicitNullRatingAndNote(t *testing.T) {
	router, cookie, csrfToken, mediaID, _, _ := newRecordsTestRouter(t)
	headers := map[string]string{
		"Cookie":          cookie.String(),
		"Origin":          "http://example.test",
		"X-CSRF-Token":    csrfToken,
		"Idempotency-Key": "create-record-with-fields",
		"If-Match":        `"0"`,
	}
	created := performJSONRequest(router, http.MethodPut, "http://example.test/api/v1/records/"+mediaID, map[string]any{
		"status": "completed",
		"rating": 8.7,
		"note":   "很喜欢",
	}, headers)
	require.Equal(t, http.StatusOK, created.Code)

	headers["If-Match"] = `"1"`
	headers["Idempotency-Key"] = "clear-record-fields"
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

func TestRecordReadLibraryAndLocalSearchSupportTheUI(t *testing.T) {
	router, cookie, csrfToken, mediaID, _, _ := newRecordsTestRouter(t)
	headers := map[string]string{
		"Cookie": cookie.String(), "Origin": "http://example.test", "X-CSRF-Token": csrfToken,
		"Idempotency-Key": "prepare-record-read",
		"If-Match":        `"0"`,
	}
	updated := performJSONRequest(router, http.MethodPut, "http://example.test/api/v1/records/"+mediaID, map[string]any{
		"status": "completed", "rating": 8.7, "note": "保留笔记",
		"watchedAt": "2026-07-13T20:30:00Z", "viewingMethod": "家庭投影",
	}, headers)
	require.Equal(t, http.StatusOK, updated.Code)

	read := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/records/"+mediaID, nil, map[string]string{
		"Cookie": cookie.String(),
	})
	require.Equal(t, http.StatusOK, read.Code)
	require.Contains(t, read.Body.String(), `"status":"completed"`)
	require.Contains(t, read.Body.String(), `"watchedAt":"2026-07-13T20:30:00Z"`)
	require.Contains(t, read.Body.String(), `"viewingMethod":"家庭投影"`)

	library := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/library?status=completed", nil, map[string]string{
		"Cookie": cookie.String(),
	})
	require.Equal(t, http.StatusOK, library.Code)
	require.Contains(t, library.Body.String(), mediaID)
	require.Contains(t, library.Body.String(), `"title":"测试电影"`)
	require.Contains(t, library.Body.String(), `"status":"completed"`)

	search := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/media/search?q=测试", nil, map[string]string{
		"Cookie": cookie.String(),
	})
	require.Equal(t, http.StatusOK, search.Code)
	require.Contains(t, search.Body.String(), mediaID)
	require.Contains(t, search.Body.String(), `"source":"local"`)

	events := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/records/"+mediaID+"/events", nil, map[string]string{
		"Cookie": cookie.String(),
	})
	require.Equal(t, http.StatusOK, events.Code)
	require.Contains(t, events.Body.String(), `"viewingMethod":"家庭投影"`)
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
