package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"video-record/internal/auth"
	"video-record/internal/household"
	"video-record/internal/integrations"
	"video-record/internal/storage"
	syncdomain "video-record/internal/sync"
)

func TestSyncHandlersScopeCandidatesAndRequireProtectedWrites(t *testing.T) {
	ctx := context.Background()
	db, authService, syncService, adminID, adminAccountID := newSyncHTTPServices(t)
	householdService := household.NewService(household.NewRepository(db))
	member, err := householdService.CreateMember(ctx, adminID, "other-sync-user", "correct horse battery staple")
	require.NoError(t, err)
	otherAccountID := insertSyncHTTPAccount(t, db, member.ID, "other-account", "emby")
	adminCandidate, err := syncService.Ingest(ctx, adminAccountID, syncHTTPMovieEvent("admin-event", "Admin Candidate"))
	require.NoError(t, err)
	otherCandidate, err := syncService.Ingest(ctx, otherAccountID, syncHTTPMovieEvent("other-event", "Other Private Candidate"))
	require.NoError(t, err)
	router := NewRouter(Dependencies{Storage: db, Auth: authService, Sync: syncService})
	cookie, csrfToken := loginForHTTPTest(t, router)

	list := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/sync/candidates", nil, map[string]string{
		"Cookie": cookie.String(),
	})
	require.Equal(t, http.StatusOK, list.Code)
	require.Contains(t, list.Body.String(), adminCandidate.ID)
	require.Contains(t, list.Body.String(), "Admin Candidate")
	require.Contains(t, list.Body.String(), "未找到可用的外部 ID")
	require.NotContains(t, list.Body.String(), otherCandidate.ID)
	require.NotContains(t, list.Body.String(), "Other Private Candidate")
	require.NotContains(t, list.Body.String(), "credential")
	require.NotContains(t, list.Body.String(), "synthetic-token")

	status := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/sync/status", nil, map[string]string{
		"Cookie": cookie.String(),
	})
	require.Equal(t, http.StatusOK, status.Code)
	require.Contains(t, status.Body.String(), `"provider":"jellyfin"`)
	require.Contains(t, status.Body.String(), `"pendingCandidates":1`)
	require.NotContains(t, status.Body.String(), otherAccountID)

	unsafeHeaders := map[string]string{"Cookie": cookie.String(), "X-CSRF-Token": csrfToken, "Idempotency-Key": "unsafe-ignore"}
	missingOrigin := performJSONRequest(router, http.MethodPost,
		"http://example.test/api/v1/sync/candidates/"+adminCandidate.ID+"/ignore", map[string]any{}, unsafeHeaders)
	require.Equal(t, http.StatusForbidden, missingOrigin.Code)
	missingIdempotency := performJSONRequest(router, http.MethodPost,
		"http://example.test/api/v1/sync/candidates/"+adminCandidate.ID+"/ignore", map[string]any{}, map[string]string{
			"Cookie": cookie.String(), "Origin": "http://example.test", "X-CSRF-Token": csrfToken,
		})
	require.Equal(t, http.StatusBadRequest, missingIdempotency.Code)

	ignored := performJSONRequest(router, http.MethodPost,
		"http://example.test/api/v1/sync/candidates/"+adminCandidate.ID+"/ignore", map[string]any{}, map[string]string{
			"Cookie": cookie.String(), "Origin": "http://example.test",
			"X-CSRF-Token": csrfToken, "Idempotency-Key": "ignore-admin-candidate",
		})
	require.Equal(t, http.StatusOK, ignored.Code)
	require.Contains(t, ignored.Body.String(), `"status":"ignored"`)

	forbidden := performJSONRequest(router, http.MethodPost,
		"http://example.test/api/v1/sync/candidates/"+otherCandidate.ID+"/ignore", map[string]any{}, map[string]string{
			"Cookie": cookie.String(), "Origin": "http://example.test",
			"X-CSRF-Token": csrfToken, "Idempotency-Key": "ignore-other-candidate",
		})
	require.Equal(t, http.StatusNotFound, forbidden.Code)
}

func TestSyncHandlersConfirmRematchAndCreateCustomCandidates(t *testing.T) {
	ctx := context.Background()
	db, authService, syncService, _, accountID := newSyncHTTPServices(t)
	insertSyncHTTPMedia(t, db, "confirm-media", "movie", "Confirm Title", "2024")
	confirmCandidate, err := syncService.Ingest(ctx, accountID, syncHTTPMovieEvent("confirm-event", "Confirm Title"))
	require.NoError(t, err)
	insertSyncHTTPMedia(t, db, "rematch-a", "movie", "Ambiguous", "2024")
	insertSyncHTTPMedia(t, db, "rematch-b", "movie", "Ambiguous", "2024")
	rematchCandidate, err := syncService.Ingest(ctx, accountID, syncHTTPMovieEvent("rematch-event", "Ambiguous"))
	require.NoError(t, err)
	customEvent := integrations.HistoryEvent{
		ID: "custom-event", PlayedAt: time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC),
		DurationSeconds: 3600, PositionSeconds: 3600,
		Item: integrations.ItemIdentity{
			ProviderItemID: "provider-custom", MediaType: integrations.MediaEpisode,
			Title: "New Custom Series", Year: 2026, SeasonNumber: 1, EpisodeNumber: 2,
		},
	}
	customCandidate, err := syncService.Ingest(ctx, accountID, customEvent)
	require.NoError(t, err)
	router := NewRouter(Dependencies{Storage: db, Auth: authService, Sync: syncService})
	cookie, csrfToken := loginForHTTPTest(t, router)
	headers := func(key string) map[string]string {
		return map[string]string{
			"Cookie": cookie.String(), "Origin": "http://example.test",
			"X-CSRF-Token": csrfToken, "Idempotency-Key": key,
		}
	}

	confirmed := performJSONRequest(router, http.MethodPost,
		"http://example.test/api/v1/sync/candidates/"+confirmCandidate.ID+"/confirm",
		map[string]any{}, headers("confirm-candidate"))
	require.Equal(t, http.StatusOK, confirmed.Code)
	require.Contains(t, confirmed.Body.String(), `"mediaId":"confirm-media"`)

	rematched := performJSONRequest(router, http.MethodPost,
		"http://example.test/api/v1/sync/candidates/"+rematchCandidate.ID+"/rematch",
		map[string]string{"mediaId": "rematch-b"}, headers("rematch-candidate"))
	require.Equal(t, http.StatusOK, rematched.Code)
	require.Contains(t, rematched.Body.String(), `"mediaId":"rematch-b"`)

	custom := performJSONRequest(router, http.MethodPost,
		"http://example.test/api/v1/sync/candidates/"+customCandidate.ID+"/custom",
		map[string]string{"title": "New Custom Series", "mediaType": "tv", "year": "2026"},
		headers("custom-candidate"))
	require.Equal(t, http.StatusOK, custom.Code)
	require.Contains(t, custom.Body.String(), `"status":"confirmed"`)
	require.Contains(t, custom.Body.String(), `"title":"New Custom Series"`)
}

func newSyncHTTPServices(
	t *testing.T,
) (*storage.DB, *auth.Service, *syncdomain.CandidateService, string, string) {
	t.Helper()
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(ctx, db))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	authService := auth.NewService(auth.NewRepository(db), auth.ServiceOptions{})
	admin, err := authService.Initialize(ctx, "owner", "correct horse battery staple")
	require.NoError(t, err)
	accountID := insertSyncHTTPAccount(t, db, admin.ID, "admin-account", "jellyfin")
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	return db, authService, syncdomain.NewCandidateService(db, syncdomain.CandidateServiceOptions{
		Now: func() time.Time { return now },
	}), admin.ID, accountID
}

func insertSyncHTTPAccount(t *testing.T, db *storage.DB, userID, accountID, provider string) string {
	t.Helper()
	_, err := db.Writer().ExecContext(context.Background(), `
		INSERT INTO external_accounts (
			id, user_id, provider, name, base_url, credential_ciphertext,
			credential_nonce, credential_version, credential_fingerprint,
			enabled, created_at, updated_at
		) VALUES (?, ?, ?, 'Home', 'https://media.example.test',
		          x'01', x'02', 1, 'fingerprint', 1, 0, 0)
	`, accountID, userID, provider)
	require.NoError(t, err)
	return accountID
}

func insertSyncHTTPMedia(t *testing.T, db *storage.DB, id, mediaType, title, year string) {
	t.Helper()
	_, err := db.Writer().ExecContext(context.Background(), `
		INSERT INTO media_items (
			id, media_type, external_title, original_title, release_date,
			external_overview, poster_path, backdrop_path, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, '', '', '', 0, 0)
	`, id, mediaType, title, title, year+"-01-01")
	require.NoError(t, err)
}

func syncHTTPMovieEvent(id, title string) integrations.HistoryEvent {
	return integrations.HistoryEvent{
		ID: id, PlayedAt: time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC),
		DurationSeconds: 7200, PositionSeconds: 7200,
		Item: integrations.ItemIdentity{
			ProviderItemID: "provider-" + id, MediaType: integrations.MediaMovie,
			Title: title, Year: 2024,
		},
	}
}

func decodeSyncCandidateResponse(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(body, &decoded))
	return decoded
}
