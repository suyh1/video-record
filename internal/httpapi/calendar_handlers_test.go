package httpapi

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"video-record/internal/records"
)

func TestCalendarHandlerReturnsLocalDatesAndStableValidationErrors(t *testing.T) {
	router, cookie, _, mediaID, service, _ := newRecordsTestRouter(t)
	userID := currentUserID(t, router, cookie)
	completedAt := time.Date(2026, 6, 30, 16, 0, 0, 0, time.UTC)
	_, err := service.UpdateRound(context.Background(), records.UpdateRoundInput{
		Scope:  records.RoundScope{UserID: userID, MediaID: mediaID},
		Status: records.StatusCompleted, CompletedAt: &completedAt, Source: records.SourceManual,
	})
	require.NoError(t, err)

	response := performJSONRequest(router, http.MethodGet,
		"http://example.test/api/v1/calendar?month=2026-07&timezone=Asia%2FShanghai&filter=all",
		nil, map[string]string{"Cookie": cookie.String()},
	)
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"year":2026`)
	require.Contains(t, response.Body.String(), `"month":7`)
	require.Contains(t, response.Body.String(), `"localDate":"2026-07-01"`)
	require.Contains(t, response.Body.String(), `"participants":["owner"]`)
	require.NotContains(t, response.Body.String(), "UserID")

	invalid := performJSONRequest(router, http.MethodGet,
		"http://example.test/api/v1/calendar?month=2026-13&timezone=UTC&filter=all",
		nil, map[string]string{"Cookie": cookie.String()},
	)
	require.Equal(t, http.StatusBadRequest, invalid.Code)
	require.Contains(t, invalid.Body.String(), `"code":"invalid_calendar_query"`)

	unauthenticated := performJSONRequest(router, http.MethodGet,
		"http://example.test/api/v1/calendar?month=2026-07&timezone=UTC&filter=all", nil, nil,
	)
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)
}
