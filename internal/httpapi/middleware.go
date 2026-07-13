package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode"

	"video-record/internal/storage"
)

const RequestIDHeader = "X-Request-ID"

type requestIDContextKey struct{}

type problemDetails struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	Code      string `json:"code"`
	RequestID string `json:"requestId"`
}

func NewLogger(environment string, output io.Writer, secrets ...string) *slog.Logger {
	if output == nil {
		output = io.Discard
	}

	options := &slog.HandlerOptions{ReplaceAttr: redactingReplaceAttr(secrets)}
	var handler slog.Handler = slog.NewTextHandler(output, options)
	if strings.EqualFold(environment, "production") {
		handler = slog.NewJSONHandler(output, options)
	}
	return slog.New(handler)
}

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := rand.Text()
		w.Header().Set(RequestIDHeader, requestID)
		ctx := context.WithValue(r.Context(), requestIDContextKey{}, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func RequestIDFromContext(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDContextKey{}).(string)
	return requestID
}

func RequestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	logger = loggerOrDiscard(logger)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			startedAt := time.Now()
			response := &statusResponseWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(response, r)
			logger.InfoContext(r.Context(), "request completed",
				slog.String("requestId", RequestIDFromContext(r.Context())),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", response.status),
				slog.Duration("duration", time.Since(startedAt)),
			)
		})
	}
}

func Recoverer(logger *slog.Logger) func(http.Handler) http.Handler {
	logger = loggerOrDiscard(logger)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recover() == nil {
					return
				}
				logger.ErrorContext(r.Context(), "panic recovered",
					slog.String("requestId", RequestIDFromContext(r.Context())),
				)
				writeProblem(w, r, http.StatusInternalServerError, "Internal Server Error", "internal_error")
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func MaintenanceMode(db *storage.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && r.URL.Path == "/api/v1/restore" {
				if db.IsMaintenance() {
					writeProblem(w, r, http.StatusServiceUnavailable, "Service Unavailable", "maintenance_mode")
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			if !db.BeginRequest() {
				writeProblem(w, r, http.StatusServiceUnavailable, "Service Unavailable", "maintenance_mode")
				return
			}
			defer db.EndRequest()
			next.ServeHTTP(w, r)
		})
	}
}

type statusResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func writeProblem(w http.ResponseWriter, r *http.Request, status int, title, code string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(problemDetails{
		Type:      "about:blank",
		Title:     title,
		Status:    status,
		Code:      code,
		RequestID: RequestIDFromContext(r.Context()),
	})
}

func redactingReplaceAttr(secrets []string) func([]string, slog.Attr) slog.Attr {
	return func(_ []string, attr slog.Attr) slog.Attr {
		if isSensitiveLogKey(attr.Key) {
			return slog.String(attr.Key, "[REDACTED]")
		}

		switch attr.Value.Kind() {
		case slog.KindString:
			return slog.String(attr.Key, redactKnownSecrets(attr.Value.String(), secrets))
		case slog.KindAny:
			original := fmt.Sprint(attr.Value.Any())
			redacted := redactKnownSecrets(original, secrets)
			if redacted != original {
				return slog.String(attr.Key, redacted)
			}
		}
		return attr
	}
}

func redactKnownSecrets(value string, secrets []string) string {
	for _, secret := range secrets {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[REDACTED]")
		}
	}
	return value
}

func isSensitiveLogKey(key string) bool {
	normalized := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, key)
	for _, marker := range []string{"authorization", "cookie", "credential", "password", "secret", "token"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func loggerOrDiscard(logger *slog.Logger) *slog.Logger {
	if logger != nil {
		return logger
	}
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
