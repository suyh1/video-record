package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"video-record/internal/auth"
	"video-record/internal/household"
	"video-record/internal/storage"
)

func TestBackupHandlersRequireAdminAndRestoreUploadedSnapshot(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(ctx, db))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	_, err = db.Writer().ExecContext(ctx, "CREATE TABLE backup_http_probe (value TEXT NOT NULL)")
	require.NoError(t, err)
	_, err = db.Writer().ExecContext(ctx, "INSERT INTO backup_http_probe (value) VALUES ('before')")
	require.NoError(t, err)
	_, err = db.Writer().ExecContext(ctx, "CREATE TABLE backup_http_large (payload BLOB NOT NULL)")
	require.NoError(t, err)
	_, err = db.Writer().ExecContext(ctx, "INSERT INTO backup_http_large (payload) VALUES (randomblob(2097152))")
	require.NoError(t, err)
	authService := auth.NewService(auth.NewRepository(db), auth.ServiceOptions{})
	_, err = authService.Initialize(ctx, "owner", "correct horse battery staple")
	require.NoError(t, err)
	manager := storage.NewBackupManager(db, storage.BackupOptions{BackupsDir: filepath.Join(t.TempDir(), "backups")})
	router := NewRouter(Dependencies{Storage: db, Auth: authService, Backup: manager})
	cookie, csrfToken := loginForHTTPTest(t, router)
	headers := map[string]string{
		"Cookie": cookie.String(), "Origin": "http://example.test",
		"X-CSRF-Token": csrfToken, "Idempotency-Key": "backup-create-1",
	}

	created := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/backups", map[string]any{}, headers)
	require.Equal(t, http.StatusCreated, created.Code)
	var artifact storage.BackupArtifact
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &artifact))
	require.NotEmpty(t, artifact.Filename)

	listed := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/backups", nil, map[string]string{
		"Cookie": cookie.String(),
	})
	require.Equal(t, http.StatusOK, listed.Code)
	require.Contains(t, listed.Body.String(), artifact.Filename)
	downloaded := performJSONRequest(router, http.MethodGet,
		"http://example.test/api/v1/backups/"+artifact.Filename, nil,
		map[string]string{"Cookie": cookie.String()},
	)
	require.Equal(t, http.StatusOK, downloaded.Code)
	require.Equal(t, "application/vnd.video-record.backup", downloaded.Header().Get("Content-Type"))
	require.Equal(t, "nosniff", downloaded.Header().Get("X-Content-Type-Options"))
	require.Greater(t, downloaded.Body.Len(), maxIdempotencyBody)

	_, err = db.Writer().ExecContext(ctx, "UPDATE backup_http_probe SET value = 'after'")
	require.NoError(t, err)
	restored := performMultipartRestore(t, router, cookie, csrfToken, artifact.Filename, downloaded.Body.Bytes())
	require.Equal(t, http.StatusOK, restored.Code)
	var value string
	require.NoError(t, db.Reader().QueryRowContext(ctx, "SELECT value FROM backup_http_probe").Scan(&value))
	require.Equal(t, "before", value)
	var auditCount int
	require.NoError(t, db.Reader().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM audit_events WHERE action = 'backup.restore'
	`).Scan(&auditCount))
	require.Equal(t, 1, auditCount)
	replayedRestore := performMultipartRestore(t, router, cookie, csrfToken, artifact.Filename, downloaded.Body.Bytes())
	require.Equal(t, http.StatusOK, replayedRestore.Code)
	require.Equal(t, "true", replayedRestore.Header().Get("Idempotency-Replayed"))
	require.Equal(t, restored.Body.String(), replayedRestore.Body.String())

	require.NoError(t, db.BeginMaintenance(ctx))
	maintenance := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/backups", nil, map[string]string{
		"Cookie": cookie.String(),
	})
	require.Equal(t, http.StatusServiceUnavailable, maintenance.Code)
	require.Contains(t, maintenance.Body.String(), `"code":"maintenance_mode"`)
	health := performJSONRequest(router, http.MethodGet, "http://example.test/healthz", nil, nil)
	require.Equal(t, http.StatusOK, health.Code)
	db.EndMaintenance()
}

func TestRestoreAcceptsBackupWhoseUsersDifferFromCurrentInstallation(t *testing.T) {
	ctx := context.Background()
	currentDB, err := storage.Open(ctx, filepath.Join(t.TempDir(), "current", "video-record.db"))
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(ctx, currentDB))
	t.Cleanup(func() { require.NoError(t, currentDB.Close()) })
	currentAuth := auth.NewService(auth.NewRepository(currentDB), auth.ServiceOptions{})
	_, err = currentAuth.Initialize(ctx, "owner", "correct horse battery staple")
	require.NoError(t, err)
	currentManager := storage.NewBackupManager(currentDB, storage.BackupOptions{BackupsDir: filepath.Join(t.TempDir(), "current-backups")})
	router := NewRouter(Dependencies{Storage: currentDB, Auth: currentAuth, Backup: currentManager})
	cookie, csrfToken := loginForHTTPTest(t, router)

	foreignDB, err := storage.Open(ctx, filepath.Join(t.TempDir(), "foreign", "video-record.db"))
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(ctx, foreignDB))
	t.Cleanup(func() { require.NoError(t, foreignDB.Close()) })
	foreignAuth := auth.NewService(auth.NewRepository(foreignDB), auth.ServiceOptions{})
	_, err = foreignAuth.Initialize(ctx, "foreign-owner", "correct horse battery staple")
	require.NoError(t, err)
	foreignManager := storage.NewBackupManager(foreignDB, storage.BackupOptions{BackupsDir: filepath.Join(t.TempDir(), "foreign-backups")})
	foreignArtifact, err := foreignManager.Create(ctx)
	require.NoError(t, err)
	foreignArchive, err := foreignManager.Read(foreignArtifact.Filename)
	require.NoError(t, err)

	restored := performMultipartRestore(t, router, cookie, csrfToken, foreignArtifact.Filename, foreignArchive)
	require.Equal(t, http.StatusOK, restored.Code)
	var username string
	require.NoError(t, currentDB.Reader().QueryRowContext(ctx, "SELECT username FROM users").Scan(&username))
	require.Equal(t, "foreign-owner", username)
}

func TestBackupHandlersRejectMembersAndUnsafeWrites(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(ctx, db))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	authService := auth.NewService(auth.NewRepository(db), auth.ServiceOptions{})
	admin, err := authService.Initialize(ctx, "owner", "correct horse battery staple")
	require.NoError(t, err)
	householdService := household.NewService(household.NewRepository(db))
	_, err = householdService.CreateMember(ctx, admin.ID, "family", "correct horse battery staple")
	require.NoError(t, err)
	manager := storage.NewBackupManager(db, storage.BackupOptions{BackupsDir: filepath.Join(t.TempDir(), "backups")})
	router := NewRouter(Dependencies{Storage: db, Auth: authService, Backup: manager})

	memberCookie, _ := loginAsBackupUser(t, router, "family")
	memberList := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/backups", nil, map[string]string{
		"Cookie": memberCookie.String(),
	})
	require.Equal(t, http.StatusForbidden, memberList.Code)

	adminCookie, csrfToken := loginAsBackupUser(t, router, "owner")
	missingOrigin := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/backups", map[string]any{}, map[string]string{
		"Cookie": adminCookie.String(), "X-CSRF-Token": csrfToken, "Idempotency-Key": "missing-origin",
	})
	require.Equal(t, http.StatusForbidden, missingOrigin.Code)
	missingCSRF := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/backups", map[string]any{}, map[string]string{
		"Cookie": adminCookie.String(), "Origin": "http://example.test", "Idempotency-Key": "missing-csrf",
	})
	require.Equal(t, http.StatusForbidden, missingCSRF.Code)
	missingIdempotency := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/backups", map[string]any{}, map[string]string{
		"Cookie": adminCookie.String(), "Origin": "http://example.test", "X-CSRF-Token": csrfToken,
	})
	require.Equal(t, http.StatusBadRequest, missingIdempotency.Code)
}

func loginAsBackupUser(t *testing.T, router http.Handler, username string) (*http.Cookie, string) {
	t.Helper()
	response := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/auth/login", map[string]string{
		"username": username,
		"password": "correct horse battery staple",
	}, map[string]string{"Origin": "http://example.test"})
	require.Equal(t, http.StatusOK, response.Code)
	require.Len(t, response.Result().Cookies(), 1)
	var body struct {
		CSRFToken string `json:"csrfToken"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.NotEmpty(t, body.CSRFToken)
	return response.Result().Cookies()[0], body.CSRFToken
}

func performMultipartRestore(
	t *testing.T,
	router http.Handler,
	cookie *http.Cookie,
	csrfToken, filename string,
	data []byte,
) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = part.Write(data)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	req := httptest.NewRequest(http.MethodPost, "http://example.test/api/v1/restore", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Cookie", cookie.String())
	req.Header.Set("Origin", "http://example.test")
	req.Header.Set("X-CSRF-Token", csrfToken)
	req.Header.Set("Idempotency-Key", "restore-1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}
