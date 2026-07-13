package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"video-record/internal/storage"
)

const (
	idempotencyTTL     = 24 * time.Hour
	maxIdempotencyBody = 1 << 20
)

type idempotencyMiddleware struct {
	db  *storage.DB
	now func() time.Time
}

type storedHTTPResponse struct {
	Method      string
	Path        string
	RequestHash string
	Status      int
	ContentType string
	ETag        string
	RequestID   string
	Body        []byte
}

func newIdempotencyMiddleware(db *storage.DB) *idempotencyMiddleware {
	return &idempotencyMiddleware{db: db, now: time.Now}
}

func (middleware *idempotencyMiddleware) Handle(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, maxIdempotencyBody+1))
		if err != nil || len(body) > maxIdempotencyBody {
			writeProblem(w, r, http.StatusRequestEntityTooLarge, "Content Too Large", "request_too_large")
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		hash := sha256.Sum256(body)
		middleware.handleHash(w, r, hex.EncodeToString(hash[:]), next)
	})
}

func (middleware *idempotencyMiddleware) handleHash(
	w http.ResponseWriter,
	r *http.Request,
	requestHash string,
	next http.Handler,
) {
	middleware.handleHashWithPersistence(w, r, requestHash, next, true)
}

func (middleware *idempotencyMiddleware) handleHashBestEffort(
	w http.ResponseWriter,
	r *http.Request,
	requestHash string,
	next http.Handler,
) {
	middleware.handleHashWithPersistence(w, r, requestHash, next, false)
}

func (middleware *idempotencyMiddleware) handleHashWithPersistence(
	w http.ResponseWriter,
	r *http.Request,
	requestHash string,
	next http.Handler,
	persistenceRequired bool,
) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	key, ok := idempotencyKey(r)
	if !ok {
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_idempotency_key")
		return
	}

	stored, found, err := middleware.find(r.Context(), identity.User.ID, key)
	if err != nil {
		writeProblem(w, r, http.StatusInternalServerError, "Internal Server Error", "internal_error")
		return
	}
	if !found {
		err = middleware.reserve(
			r.Context(), identity.User.ID, key, r.Method, r.URL.Path, requestHash, RequestIDFromContext(r.Context()),
		)
		if err != nil {
			stored, found, err = middleware.find(r.Context(), identity.User.ID, key)
			if err != nil || !found {
				writeProblem(w, r, http.StatusInternalServerError, "Internal Server Error", "internal_error")
				return
			}
		}
	}
	if found {
		if stored.Method != r.Method || stored.Path != r.URL.Path || stored.RequestHash != requestHash {
			writeProblem(w, r, http.StatusConflict, "Conflict", "idempotency_key_conflict")
			return
		}
		if stored.Status == 0 {
			writeProblem(w, r, http.StatusConflict, "Conflict", "idempotency_in_progress")
			return
		}
		writeStoredResponse(w, stored, true)
		return
	}

	capture := newCapturedResponse()
	next.ServeHTTP(capture, r)
	response := capture.stored(r.Method, r.URL.Path, requestHash, RequestIDFromContext(r.Context()))
	if response.Status < http.StatusInternalServerError {
		err := middleware.complete(r.Context(), identity.User.ID, key, response)
		if err != nil && !persistenceRequired {
			err = middleware.upsertCompleted(r.Context(), identity.User.ID, key, response)
		}
		if err != nil {
			if persistenceRequired {
				writeProblem(w, r, http.StatusInternalServerError, "Internal Server Error", "internal_error")
				return
			}
		}
	} else {
		_ = middleware.release(r.Context(), identity.User.ID, key, response)
	}
	copyHeader(w.Header(), capture.header)
	w.WriteHeader(response.Status)
	_, _ = w.Write(response.Body)
}

func idempotencyKey(r *http.Request) (string, bool) {
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	return key, key != "" && len(key) <= 128
}

func (middleware *idempotencyMiddleware) find(
	ctx context.Context,
	userID, key string,
) (storedHTTPResponse, bool, error) {
	now := middleware.now().UTC().UnixMilli()
	if _, err := middleware.db.Writer().ExecContext(ctx, "DELETE FROM idempotency_keys WHERE expires_at <= ?", now); err != nil {
		return storedHTTPResponse{}, false, err
	}
	var response storedHTTPResponse
	err := middleware.db.Reader().QueryRowContext(ctx, `
		SELECT method, path, request_hash, status_code, content_type, etag, request_id, response_body
		FROM idempotency_keys WHERE user_id = ? AND key = ?
	`, userID, key).Scan(
		&response.Method, &response.Path, &response.RequestHash, &response.Status,
		&response.ContentType, &response.ETag, &response.RequestID, &response.Body,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return storedHTTPResponse{}, false, nil
	}
	return response, err == nil, err
}

func (middleware *idempotencyMiddleware) reserve(
	ctx context.Context,
	userID, key, method, path, requestHash, requestID string,
) error {
	now := middleware.now().UTC()
	_, err := middleware.db.Writer().ExecContext(ctx, `
		INSERT INTO idempotency_keys (
			user_id, key, method, path, request_hash, status_code,
			content_type, etag, request_id, response_body, created_at, expires_at
		) VALUES (?, ?, ?, ?, ?, 0, '', '', ?, ?, ?, ?)
	`, userID, key, method, path, requestHash, requestID, []byte{}, now.UnixMilli(), now.Add(idempotencyTTL).UnixMilli())
	return err
}

func (middleware *idempotencyMiddleware) complete(
	ctx context.Context,
	userID, key string,
	response storedHTTPResponse,
) error {
	result, err := middleware.db.Writer().ExecContext(ctx, `
		UPDATE idempotency_keys SET
			status_code = ?, content_type = ?, etag = ?, response_body = ?
		WHERE user_id = ? AND key = ? AND method = ? AND path = ?
		  AND request_hash = ? AND status_code = 0
	`, response.Status, response.ContentType, response.ETag, response.Body,
		userID, key, response.Method, response.Path, response.RequestHash)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return errors.New("idempotency reservation was not completed")
	}
	return nil
}

func (middleware *idempotencyMiddleware) upsertCompleted(
	ctx context.Context,
	userID, key string,
	response storedHTTPResponse,
) error {
	now := middleware.now().UTC()
	result, err := middleware.db.Writer().ExecContext(ctx, `
		INSERT INTO idempotency_keys (
			user_id, key, method, path, request_hash, status_code,
			content_type, etag, request_id, response_body, created_at, expires_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, key) DO UPDATE SET
			status_code = excluded.status_code,
			content_type = excluded.content_type,
			etag = excluded.etag,
			request_id = excluded.request_id,
			response_body = excluded.response_body
		WHERE idempotency_keys.method = excluded.method
		  AND idempotency_keys.path = excluded.path
		  AND idempotency_keys.request_hash = excluded.request_hash
		  AND idempotency_keys.status_code = 0
	`, userID, key, response.Method, response.Path, response.RequestHash, response.Status,
		response.ContentType, response.ETag, response.RequestID, response.Body,
		now.UnixMilli(), now.Add(idempotencyTTL).UnixMilli())
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return errors.New("idempotency result was not persisted")
	}
	return nil
}

func (middleware *idempotencyMiddleware) release(
	ctx context.Context,
	userID, key string,
	response storedHTTPResponse,
) error {
	_, err := middleware.db.Writer().ExecContext(ctx, `
		DELETE FROM idempotency_keys
		WHERE user_id = ? AND key = ? AND method = ? AND path = ?
		  AND request_hash = ? AND status_code = 0
	`, userID, key, response.Method, response.Path, response.RequestHash)
	return err
}

func writeStoredResponse(w http.ResponseWriter, response storedHTTPResponse, replayed bool) {
	if response.ContentType != "" {
		w.Header().Set("Content-Type", response.ContentType)
	}
	if response.ETag != "" {
		w.Header().Set("ETag", response.ETag)
	}
	if response.RequestID != "" {
		w.Header().Set(RequestIDHeader, response.RequestID)
	}
	if replayed {
		w.Header().Set("Idempotency-Replayed", "true")
	}
	w.WriteHeader(response.Status)
	_, _ = w.Write(response.Body)
}

type capturedResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func newCapturedResponse() *capturedResponse {
	return &capturedResponse{header: make(http.Header)}
}

func (response *capturedResponse) Header() http.Header {
	return response.header
}

func (response *capturedResponse) WriteHeader(status int) {
	if response.status == 0 {
		response.status = status
	}
}

func (response *capturedResponse) Write(body []byte) (int, error) {
	if response.status == 0 {
		response.status = http.StatusOK
	}
	return response.body.Write(body)
}

func (response *capturedResponse) stored(method, path, requestHash, requestID string) storedHTTPResponse {
	status := response.status
	if status == 0 {
		status = http.StatusOK
	}
	return storedHTTPResponse{
		Method: method, Path: path, RequestHash: requestHash, Status: status,
		ContentType: response.header.Get("Content-Type"), ETag: response.header.Get("ETag"),
		RequestID: requestID, Body: append([]byte{}, response.body.Bytes()...),
	}
}

func copyHeader(destination, source http.Header) {
	for key, values := range source {
		destination[key] = append([]string(nil), values...)
	}
}
