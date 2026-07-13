package household

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"video-record/internal/auth"
	"video-record/internal/media"
	"video-record/internal/records"
	"video-record/internal/storage"
)

func TestHouseholdMembersPrivacySharingAndSharedEvents(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(ctx, db))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	authService := auth.NewService(auth.NewRepository(db), auth.ServiceOptions{})
	admin, err := authService.Initialize(ctx, "owner", "correct horse battery staple")
	require.NoError(t, err)
	service := NewService(NewRepository(db))
	member, err := service.CreateMember(ctx, admin.ID, "family", "family password 123")
	require.NoError(t, err)
	require.Equal(t, auth.RoleMember, member.Role)
	require.True(t, member.Active)
	_, err = service.CreateMember(ctx, member.ID, "forbidden", "forbidden password")
	require.ErrorIs(t, err, ErrForbidden)
	members, err := service.Members(ctx, admin.ID)
	require.NoError(t, err)
	require.Len(t, members, 2)

	mediaService := media.NewService(media.NewRepository(db))
	movie, err := mediaService.CreateCustom(ctx, media.CreateCustomInput{
		MediaType: media.MediaTypeMovie, Title: "家庭电影",
	})
	require.NoError(t, err)
	recordService := records.NewService(records.NewRepository(db))
	rating := 91
	privateNote := "只有我能看到的长笔记"
	recordState, event, err := recordService.RecordStatus(ctx, records.RecordStatusInput{
		UpdateStateInput: records.UpdateStateInput{
			UserID: member.ID, MediaID: movie.ID, Status: records.StatusCompleted,
			Rating: &rating, Note: &privateNote, Source: records.SourceManual, ExpectedVersion: 0,
		},
		WatchedAt: time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.NotNil(t, event)
	_, err = db.Writer().ExecContext(ctx, `
		INSERT INTO watch_event_participants (event_id, user_id) VALUES (?, ?)
	`, event.ID, admin.ID)
	require.NoError(t, err)

	private, err := service.VisibleRecord(ctx, admin.ID, member.ID, movie.ID)
	require.NoError(t, err)
	require.Nil(t, private.Rating)
	require.Nil(t, private.PrivateNote)
	require.Nil(t, private.SharedReview)
	ownerView, err := service.VisibleRecord(ctx, member.ID, member.ID, movie.ID)
	require.NoError(t, err)
	require.Equal(t, privateNote, *ownerView.PrivateNote)

	_, err = service.UpdateSharing(ctx, admin.ID, member.ID, movie.ID, SharingInput{
		ShareRating: true, ShareReview: true, SharedReview: "值得一起看", ExpectedVersion: recordState.Version,
	})
	require.ErrorIs(t, err, ErrForbidden)
	shared, err := service.UpdateSharing(ctx, member.ID, member.ID, movie.ID, SharingInput{
		ShareRating: true, ShareReview: true, SharedReview: "值得一起看", ExpectedVersion: recordState.Version,
	})
	require.NoError(t, err)
	require.True(t, shared.ShareRating)

	visible, err := service.VisibleRecord(ctx, admin.ID, member.ID, movie.ID)
	require.NoError(t, err)
	require.Equal(t, rating, *visible.Rating)
	require.Nil(t, visible.PrivateNote)
	require.Equal(t, "值得一起看", *visible.SharedReview)

	events, err := service.SharedEvents(ctx, admin.ID)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, movie.ID, events[0].MediaID)
	require.Equal(t, []string{"family", "owner"}, events[0].Participants)
	encoded, err := json.Marshal(events[0])
	require.NoError(t, err)
	require.NotContains(t, string(encoded), privateNote)
	require.NotContains(t, string(encoded), "note")
}

func TestAdminResetAndDeactivateMemberRevokeSessionsAndWriteAudit(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(ctx, db))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	authService := auth.NewService(auth.NewRepository(db), auth.ServiceOptions{})
	admin, err := authService.Initialize(ctx, "owner", "correct horse battery staple")
	require.NoError(t, err)
	service := NewService(NewRepository(db))
	member, err := service.CreateMember(ctx, admin.ID, "family", "family password 123")
	require.NoError(t, err)
	session, err := authService.Login(ctx, "family", "family password 123", "test")
	require.NoError(t, err)

	require.NoError(t, service.ResetPassword(ctx, admin.ID, member.ID, "replacement password 456"))
	_, err = authService.Authenticate(ctx, session.Token)
	require.ErrorIs(t, err, auth.ErrInvalidSession)
	_, err = authService.Login(ctx, "family", "family password 123", "old")
	require.ErrorIs(t, err, auth.ErrInvalidCredentials)
	_, err = authService.Login(ctx, "family", "replacement password 456", "new")
	require.NoError(t, err)

	require.NoError(t, service.DeactivateMember(ctx, admin.ID, member.ID))
	_, err = authService.Login(ctx, "family", "replacement password 456", "inactive")
	require.ErrorIs(t, err, auth.ErrInvalidCredentials)

	var auditActions int
	require.NoError(t, db.Reader().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM audit_events
		WHERE actor_user_id = ? AND action IN ('member.create', 'member.reset_password', 'member.deactivate')
	`, admin.ID).Scan(&auditActions))
	require.Equal(t, 3, auditActions)
}

func TestParticipantsReturnOtherActiveHouseholdMembers(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(ctx, db))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	authService := auth.NewService(auth.NewRepository(db), auth.ServiceOptions{})
	admin, err := authService.Initialize(ctx, "owner", "correct horse battery staple")
	require.NoError(t, err)
	service := NewService(NewRepository(db))
	active, err := service.CreateMember(ctx, admin.ID, "family", "family password 123")
	require.NoError(t, err)
	inactive, err := service.CreateMember(ctx, admin.ID, "away", "inactive password 123")
	require.NoError(t, err)
	require.NoError(t, service.DeactivateMember(ctx, admin.ID, inactive.ID))

	participants, err := service.Participants(ctx, admin.ID)
	require.NoError(t, err)
	require.Len(t, participants, 1)
	require.Equal(t, active.ID, participants[0].ID)
	require.Equal(t, active.Username, participants[0].Username)
	require.Equal(t, active.Role, participants[0].Role)
	require.True(t, participants[0].Active)

	_, err = service.Participants(ctx, inactive.ID)
	require.ErrorIs(t, err, ErrForbidden)
}

func TestHouseholdValidationAuthorizationAndSharingConflicts(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(ctx, db))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	authService := auth.NewService(auth.NewRepository(db), auth.ServiceOptions{})
	admin, err := authService.Initialize(ctx, "owner", "correct horse battery staple")
	require.NoError(t, err)
	service := NewService(NewRepository(db))
	member, err := service.CreateMember(ctx, admin.ID, "family", "family password 123")
	require.NoError(t, err)

	_, err = service.CreateMember(ctx, admin.ID, " ", "family password 123")
	require.ErrorIs(t, err, ErrInvalidMember)
	_, err = service.CreateMember(ctx, admin.ID, "short-password", "too short")
	require.ErrorIs(t, err, ErrInvalidMember)
	_, err = service.CreateMember(ctx, admin.ID, "family", "duplicate password 123")
	require.Error(t, err)
	_, err = service.Members(ctx, member.ID)
	require.ErrorIs(t, err, ErrForbidden)
	require.ErrorIs(t, service.ResetPassword(ctx, admin.ID, admin.ID, "replacement password 456"), ErrForbidden)
	require.ErrorIs(t, service.DeactivateMember(ctx, admin.ID, admin.ID), ErrForbidden)
	require.ErrorIs(t, service.ResetPassword(ctx, admin.ID, "missing", "replacement password 456"), ErrMemberNotFound)
	_, err = service.Participants(ctx, "missing")
	require.ErrorIs(t, err, ErrMemberNotFound)
	_, err = service.SharedEvents(ctx, "missing")
	require.ErrorIs(t, err, ErrMemberNotFound)

	mediaService := media.NewService(media.NewRepository(db))
	movie, err := mediaService.CreateCustom(ctx, media.CreateCustomInput{
		MediaType: media.MediaTypeMovie, Title: "家庭电影",
	})
	require.NoError(t, err)
	recordService := records.NewService(records.NewRepository(db))
	rating := 88
	state, _, err := recordService.RecordStatus(ctx, records.RecordStatusInput{UpdateStateInput: records.UpdateStateInput{
		UserID: member.ID, MediaID: movie.ID, Status: records.StatusWishlist,
		Rating: &rating, Source: records.SourceManual, ExpectedVersion: 0,
	}})
	require.NoError(t, err)

	_, err = service.UpdateSharing(ctx, member.ID, member.ID, "", SharingInput{ExpectedVersion: state.Version})
	require.ErrorIs(t, err, ErrInvalidMember)
	_, err = service.UpdateSharing(ctx, member.ID, member.ID, movie.ID, SharingInput{
		ShareReview: true, ExpectedVersion: state.Version,
	})
	require.ErrorIs(t, err, ErrInvalidMember)
	_, err = service.UpdateSharing(ctx, member.ID, member.ID, movie.ID, SharingInput{
		ShareReview: true, SharedReview: strings.Repeat("长", 501), ExpectedVersion: state.Version,
	})
	require.ErrorIs(t, err, ErrInvalidMember)
	_, err = service.UpdateSharing(ctx, member.ID, member.ID, movie.ID, SharingInput{
		ShareRating: true, ExpectedVersion: state.Version + 1,
	})
	require.ErrorIs(t, err, ErrVersionConflict)
	_, err = service.UpdateSharing(ctx, member.ID, member.ID, "missing", SharingInput{ExpectedVersion: 1})
	require.ErrorIs(t, err, ErrRecordNotFound)

	shared, err := service.UpdateSharing(ctx, member.ID, member.ID, movie.ID, SharingInput{
		ShareRating: true, ShareReview: true, SharedReview: "向家庭公开", ExpectedVersion: state.Version,
	})
	require.NoError(t, err)
	cleared, err := service.UpdateSharing(ctx, member.ID, member.ID, movie.ID, SharingInput{
		ExpectedVersion: shared.Version,
	})
	require.NoError(t, err)
	require.False(t, cleared.ShareRating)
	require.False(t, cleared.ShareReview)
	require.Nil(t, cleared.SharedReview)

	require.NoError(t, service.DeactivateMember(ctx, admin.ID, member.ID))
	require.ErrorIs(t, service.ResetPassword(ctx, admin.ID, member.ID, "replacement password 456"), ErrMemberNotFound)
}
