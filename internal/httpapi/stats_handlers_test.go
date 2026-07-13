package httpapi

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"video-record/internal/auth"
	statsdomain "video-record/internal/stats"
	"video-record/internal/storage"
)

func TestStatsHandlerUsesCurrentUserAndValidatesTimezone(t *testing.T) {
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(context.Background(), db))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	authService := auth.NewService(auth.NewRepository(db), auth.ServiceOptions{})
	_, err = authService.Initialize(context.Background(), "owner", "correct horse battery staple")
	require.NoError(t, err)
	router := NewRouter(Dependencies{
		Storage: db,
		Auth:    authService,
		Stats:   statsdomain.NewService(statsdomain.NewRepository(db)),
	})
	cookie, _ := loginForHTTPTest(t, router)

	response := performJSONRequest(router, http.MethodGet,
		"http://example.test/api/v1/stats?timezone=Asia%2FShanghai", nil,
		map[string]string{"Cookie": cookie.String()},
	)
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"totalWatches":0`)
	require.Contains(t, response.Body.String(), `"uniqueMedia":0`)
	require.Contains(t, response.Body.String(), `"monthly":[]`)
	require.NotContains(t, response.Body.String(), "UserID")

	invalid := performJSONRequest(router, http.MethodGet,
		"http://example.test/api/v1/stats?timezone=Mars%2FOlympus", nil,
		map[string]string{"Cookie": cookie.String()},
	)
	require.Equal(t, http.StatusBadRequest, invalid.Code)
	require.Contains(t, invalid.Body.String(), `"code":"invalid_stats_query"`)

	unauthenticated := performJSONRequest(router, http.MethodGet,
		"http://example.test/api/v1/stats?timezone=UTC", nil, nil,
	)
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)
}
