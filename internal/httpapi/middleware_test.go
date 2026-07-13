package httpapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRequestIDIsAddedToContextAndResponse(t *testing.T) {
	var contextID string
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contextID = RequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.NotEmpty(t, contextID)
	require.Equal(t, contextID, rec.Header().Get(RequestIDHeader))
}

func TestLoggerUsesProductionJSONAndRedactsSensitiveValues(t *testing.T) {
	var output bytes.Buffer
	logger := NewLogger("production", &output, "synthetic-tmdb-token", "synthetic-media-token")

	logger.Info("request",
		slog.String("Authorization", "Bearer synthetic-auth-secret"),
		slog.String("Cookie", "session=synthetic-cookie"),
		slog.String("detail", "tmdb=synthetic-tmdb-token media=synthetic-media-token"),
	)

	logOutput := output.String()
	for _, secret := range []string{
		"synthetic-auth-secret",
		"synthetic-cookie",
		"synthetic-tmdb-token",
		"synthetic-media-token",
	} {
		require.NotContains(t, logOutput, secret)
	}
	require.Contains(t, logOutput, "[REDACTED]")

	var entry map[string]any
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(output.Bytes()), &entry))
	require.Equal(t, "INFO", entry["level"])
}

func TestLoggerUsesTextLocally(t *testing.T) {
	var output bytes.Buffer
	NewLogger("development", &output).Info("ready")

	require.Contains(t, output.String(), "level=INFO")
	require.Contains(t, output.String(), "msg=ready")
	var entry map[string]any
	require.Error(t, json.Unmarshal(bytes.TrimSpace(output.Bytes()), &entry))
}

func TestRequestLoggerIncludesRequestIDWithoutQueryValues(t *testing.T) {
	var output bytes.Buffer
	logger := NewLogger("production", &output)
	handler := RequestID(RequestLogger(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/items?access_token=synthetic-query-secret", nil))

	var entry map[string]any
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(output.Bytes()), &entry))
	require.Equal(t, rec.Header().Get(RequestIDHeader), entry["requestId"])
	require.Equal(t, "/items", entry["path"])
	require.EqualValues(t, http.StatusNoContent, entry["status"])
	require.NotContains(t, output.String(), "synthetic-query-secret")
}

func TestRecoveryReturnsGenericProblemWithoutPanicDetails(t *testing.T) {
	const panicSecret = "synthetic-panic-secret"
	var output bytes.Buffer
	logger := NewLogger("production", &output)
	handler := RequestID(Recoverer(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(panicSecret)
	})))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/panic", nil))

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	require.NotContains(t, rec.Body.String(), panicSecret)
	require.NotContains(t, output.String(), panicSecret)
	require.NotContains(t, output.String(), "goroutine")

	var problem map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))
	require.Equal(t, "about:blank", problem["type"])
	require.Equal(t, "Internal Server Error", problem["title"])
	require.EqualValues(t, http.StatusInternalServerError, problem["status"])
	require.Equal(t, "internal_error", problem["code"])
	require.Equal(t, rec.Header().Get(RequestIDHeader), problem["requestId"])
	require.True(t, strings.Contains(output.String(), "panic recovered"))
}
