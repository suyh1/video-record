package httpapi

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestImportExportHandlersUseSafeDownloadsAndProtectedStreamingUploads(t *testing.T) {
	router, cookie, csrfToken, mediaID, _, _ := newRecordsTestRouter(t)
	headers := map[string]string{
		"Cookie": cookie.String(), "Origin": "http://example.test",
		"X-CSRF-Token": csrfToken, "Idempotency-Key": "prepare-export-record", "If-Match": `"0"`,
	}
	updated := performJSONRequest(router, http.MethodPut, "http://example.test/api/v1/records/"+mediaID, map[string]any{
		"status": "wishlist", "note": "portable note",
	}, headers)
	require.Equal(t, http.StatusOK, updated.Code)

	exported := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/data/export?format=json", nil, map[string]string{
		"Cookie": cookie.String(),
	})
	require.Equal(t, http.StatusOK, exported.Code)
	require.Equal(t, "application/json", exported.Header().Get("Content-Type"))
	require.Equal(t, `attachment; filename="video-record-export.json"`, exported.Header().Get("Content-Disposition"))
	require.Equal(t, "nosniff", exported.Header().Get("X-Content-Type-Options"))
	require.Contains(t, exported.Body.String(), "portable note")

	imported := performMultipartImport(t, router, cookie, csrfToken, "records.json", exported.Body.Bytes(), "import-1")
	require.Equal(t, http.StatusOK, imported.Code)
	require.Contains(t, imported.Body.String(), `"importedRecords":1`)
	require.Contains(t, imported.Body.String(), `"failures":[]`)
	largeValidDocument := append(append([]byte(nil), exported.Body.Bytes()...), bytes.Repeat([]byte(" "), 2<<20)...)
	largeImport := performMultipartImport(t, router, cookie, csrfToken, "records.json", largeValidDocument, "import-large")
	require.Equal(t, http.StatusOK, largeImport.Code, largeImport.Body.String())

	invalid := performMultipartImport(t, router, cookie, csrfToken, "records.json", []byte(`{"version":99}`), "import-2")
	require.Equal(t, http.StatusBadRequest, invalid.Code)
	require.Contains(t, invalid.Body.String(), `"code":"invalid_import"`)
}

func performMultipartImport(
	t *testing.T,
	router http.Handler,
	cookie *http.Cookie,
	csrfToken, filename string,
	data []byte,
	idempotencyKey string,
) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = part.Write(data)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	req := httptest.NewRequest(http.MethodPost, "http://example.test/api/v1/data/import", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Cookie", cookie.String())
	req.Header.Set("Origin", "http://example.test")
	req.Header.Set("X-CSRF-Token", csrfToken)
	req.Header.Set("Idempotency-Key", idempotencyKey)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}
