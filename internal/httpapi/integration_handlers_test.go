package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"video-record/internal/auth"
	"video-record/internal/integrations"
	"video-record/internal/storage"
	syncdomain "video-record/internal/sync"
)

func TestIntegrationAccountHandlersEncryptProvisionListAndDisconnect(t *testing.T) {
	router, cookie, csrfToken, db := newIntegrationTestRouter(t, bytes.Repeat([]byte{0x71}, 32))
	headers := map[string]string{
		"Cookie": cookie.String(), "Origin": "http://example.test",
		"X-CSRF-Token": csrfToken, "Idempotency-Key": "create-integration-account",
	}
	created := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/integrations/accounts", map[string]any{
		"provider": "jellyfin", "name": "客厅 Jellyfin", "baseUrl": "https://jellyfin.example.test",
		"token": "synthetic-jellyfin-account-token", "userId": "jellyfin-user-id",
	}, headers)
	require.Equal(t, http.StatusCreated, created.Code)
	require.Contains(t, created.Body.String(), `"provider":"jellyfin"`)
	require.Contains(t, created.Body.String(), `"name":"客厅 Jellyfin"`)
	require.NotContains(t, created.Body.String(), "synthetic-jellyfin-account-token")
	require.NotContains(t, created.Body.String(), "jellyfin.example.test")
	accountID := jsonStringField(t, created.Body.Bytes(), "id")

	var ciphertext []byte
	require.NoError(t, db.Reader().QueryRowContext(context.Background(), `
		SELECT credential_ciphertext FROM external_accounts WHERE id = ?
	`, accountID).Scan(&ciphertext))
	require.NotContains(t, string(ciphertext), "synthetic-jellyfin-account-token")
	var jobs int
	require.NoError(t, db.Reader().QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM sync_jobs WHERE account_id = ?", accountID,
	).Scan(&jobs))
	require.Equal(t, 2, jobs)

	listed := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/integrations/accounts", nil, map[string]string{
		"Cookie": cookie.String(),
	})
	require.Equal(t, http.StatusOK, listed.Code)
	require.Contains(t, listed.Body.String(), accountID)
	require.NotContains(t, listed.Body.String(), "synthetic-jellyfin-account-token")
	require.NotContains(t, listed.Body.String(), "jellyfin.example.test")

	deleteHeaders := cloneHeaders(headers)
	deleteHeaders["Idempotency-Key"] = "disconnect-integration-account"
	disconnected := performJSONRequest(router, http.MethodDelete,
		"http://example.test/api/v1/integrations/accounts/"+accountID, nil, deleteHeaders)
	require.Equal(t, http.StatusNoContent, disconnected.Code)
	require.NoError(t, db.Reader().QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM external_accounts WHERE id = ?", accountID,
	).Scan(&jobs))
	require.Zero(t, jobs)
}

func TestIntegrationAccountHandlersRejectInvalidConfigurationAndMissingKey(t *testing.T) {
	router, cookie, csrfToken, _ := newIntegrationTestRouter(t, nil)
	headers := map[string]string{
		"Cookie": cookie.String(), "Origin": "http://example.test",
		"X-CSRF-Token": csrfToken, "Idempotency-Key": "locked-integration-account",
	}
	locked := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/integrations/accounts", map[string]any{
		"provider": "plex", "name": "Plex", "baseUrl": "https://plex.example.test",
		"token": "synthetic-plex-account-token", "accountId": 1,
	}, headers)
	require.Equal(t, http.StatusLocked, locked.Code)
	require.Contains(t, locked.Body.String(), `"code":"integrations_locked"`)
	require.NotContains(t, locked.Body.String(), "synthetic-plex-account-token")

	invalidRouter, invalidCookie, invalidCSRF, _ := newIntegrationTestRouter(t, bytes.Repeat([]byte{0x72}, 32))
	headers["Cookie"] = invalidCookie.String()
	headers["X-CSRF-Token"] = invalidCSRF
	headers["Idempotency-Key"] = "invalid-integration-account"
	invalid := performJSONRequest(invalidRouter, http.MethodPost, "http://example.test/api/v1/integrations/accounts", map[string]any{
		"provider": "emby", "name": "Emby", "baseUrl": "https://emby.example.test",
		"token": "synthetic-emby-account-token", "userId": "emby-user", "timezone": "Not/A_Timezone",
	}, headers)
	require.Equal(t, http.StatusBadRequest, invalid.Code)
	require.Contains(t, invalid.Body.String(), `"code":"invalid_integration_account"`)
	require.NotContains(t, invalid.Body.String(), "synthetic-emby-account-token")
}

func newIntegrationTestRouter(t *testing.T, key []byte) (http.Handler, *http.Cookie, string, *storage.DB) {
	t.Helper()
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(ctx, db))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	authService := auth.NewService(auth.NewRepository(db), auth.ServiceOptions{})
	_, err = authService.Initialize(ctx, "owner", "correct horse battery staple")
	require.NoError(t, err)
	router := NewRouter(Dependencies{
		Storage: db,
		Auth:    authService,
		IntegrationAccounts: integrations.NewAccountRepository(
			db, integrations.NewCredentialCipher(key), integrations.AccountRepositoryOptions{},
		),
		SyncJobs: syncdomain.NewService(db, syncdomain.ServiceOptions{}),
	})
	cookie, csrfToken := loginForHTTPTest(t, router)
	return router, cookie, csrfToken, db
}

func jsonStringField(t *testing.T, contents []byte, field string) string {
	t.Helper()
	var value map[string]any
	require.NoError(t, json.Unmarshal(contents, &value))
	text, ok := value[field].(string)
	require.True(t, ok)
	require.NotEmpty(t, text)
	return text
}
