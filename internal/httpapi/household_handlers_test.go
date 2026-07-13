package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"video-record/internal/auth"
	"video-record/internal/household"
	"video-record/internal/media"
	"video-record/internal/records"
	"video-record/internal/storage"
)

func TestPolicyHandlersNeverExposePrivateNotesAndRequireExplicitSharing(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(ctx, db))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	authService := auth.NewService(auth.NewRepository(db), auth.ServiceOptions{})
	admin, err := authService.Initialize(ctx, "owner", "correct horse battery staple")
	require.NoError(t, err)
	householdService := household.NewService(household.NewRepository(db))
	member, err := householdService.CreateMember(ctx, admin.ID, "family", "family password 123")
	require.NoError(t, err)
	mediaService := media.NewService(media.NewRepository(db))
	movie, err := mediaService.CreateCustom(ctx, media.CreateCustomInput{MediaType: media.MediaTypeMovie, Title: "家庭电影"})
	require.NoError(t, err)
	recordService := records.NewService(records.NewRepository(db))
	rating := 91
	note := "管理员也不能读取"
	state, event, err := recordService.RecordStatus(ctx, records.RecordStatusInput{
		UpdateStateInput: records.UpdateStateInput{
			UserID: member.ID, MediaID: movie.ID, Status: records.StatusCompleted,
			Rating: &rating, Note: &note, Source: records.SourceManual, ExpectedVersion: 0,
		},
	})
	require.NoError(t, err)
	_, err = db.Writer().ExecContext(ctx, "INSERT INTO watch_event_participants (event_id, user_id) VALUES (?, ?)", event.ID, admin.ID)
	require.NoError(t, err)

	router := NewRouter(Dependencies{
		Storage: db, Auth: authService, Records: recordService, Household: householdService,
	})
	adminCookie, _ := loginForHTTPTest(t, router)
	memberLogin := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/auth/login", map[string]string{
		"username": "family", "password": "family password 123",
	}, map[string]string{"Origin": "http://example.test"})
	require.Equal(t, http.StatusOK, memberLogin.Code)
	memberCookie := memberLogin.Result().Cookies()[0]
	var memberBody struct {
		CSRFToken string `json:"csrfToken"`
	}
	require.NoError(t, json.Unmarshal(memberLogin.Body.Bytes(), &memberBody))

	private := performJSONRequest(router, http.MethodGet,
		"http://example.test/api/v1/household/records/"+member.ID+"/"+movie.ID,
		nil, map[string]string{"Cookie": adminCookie.String()},
	)
	require.Equal(t, http.StatusOK, private.Code)
	require.Contains(t, private.Body.String(), `"rating":null`)
	require.Contains(t, private.Body.String(), `"privateNote":null`)
	require.NotContains(t, private.Body.String(), note)

	shared := performJSONRequest(router, http.MethodPut,
		"http://example.test/api/v1/household/records/"+movie.ID+"/sharing",
		map[string]any{"shareRating": true, "shareReview": true, "sharedReview": "值得一起看", "expectedVersion": state.Version},
		map[string]string{
			"Cookie": memberCookie.String(), "Origin": "http://example.test",
			"X-CSRF-Token": memberBody.CSRFToken, "Idempotency-Key": "sharing-1",
		},
	)
	require.Equal(t, http.StatusOK, shared.Code)

	visible := performJSONRequest(router, http.MethodGet,
		"http://example.test/api/v1/household/records/"+member.ID+"/"+movie.ID,
		nil, map[string]string{"Cookie": adminCookie.String()},
	)
	require.Equal(t, http.StatusOK, visible.Code)
	require.Contains(t, visible.Body.String(), `"rating":9.1`)
	require.Contains(t, visible.Body.String(), `"sharedReview":"值得一起看"`)
	require.Contains(t, visible.Body.String(), `"privateNote":null`)

	events := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/household/events", nil, map[string]string{
		"Cookie": adminCookie.String(),
	})
	require.Equal(t, http.StatusOK, events.Code)
	require.Contains(t, events.Body.String(), `"participants":["family","owner"]`)
	require.NotContains(t, events.Body.String(), "note")

	participants := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/household/participants", nil, map[string]string{
		"Cookie": memberCookie.String(),
	})
	require.Equal(t, http.StatusOK, participants.Code)
	require.Contains(t, participants.Body.String(), `"username":"owner"`)
	require.NotContains(t, participants.Body.String(), `"username":"family"`)

	members := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/household/members", nil, map[string]string{
		"Cookie": adminCookie.String(),
	})
	require.Equal(t, http.StatusOK, members.Code)
	forbidden := performJSONRequest(router, http.MethodGet, "http://example.test/api/v1/household/members", nil, map[string]string{
		"Cookie": memberCookie.String(),
	})
	require.Equal(t, http.StatusForbidden, forbidden.Code)
}
