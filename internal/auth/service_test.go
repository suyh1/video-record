package auth

import (
	"context"
	"crypto/sha256"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"video-record/internal/storage"
)

func TestInitializeCreatesOnlyOneAdministrator(t *testing.T) {
	service, db, _ := newTestService(t)

	user, err := service.Initialize(context.Background(), "owner", "correct horse battery staple")

	require.NoError(t, err)
	require.Equal(t, RoleAdmin, user.Role)
	require.True(t, user.Active)
	initialized, err := service.IsInitialized(context.Background())
	require.NoError(t, err)
	require.True(t, initialized)

	_, err = service.Initialize(context.Background(), "second", "another secure password")
	require.ErrorIs(t, err, ErrInitializationClosed)
	var users int
	require.NoError(t, db.Reader().QueryRowContext(context.Background(), "SELECT COUNT(*) FROM users").Scan(&users))
	require.Equal(t, 1, users)
}

func TestLoginStoresOnlyTokenHashAndRotatesSessions(t *testing.T) {
	service, db, _ := newTestService(t)
	_, err := service.Initialize(context.Background(), "owner", "correct horse battery staple")
	require.NoError(t, err)

	first, err := service.Login(context.Background(), "owner", "correct horse battery staple", "client-1")
	require.NoError(t, err)
	require.NotEmpty(t, first.Token)
	require.NotEmpty(t, first.CSRFToken)
	require.NotContains(t, first.Token, "owner")

	var storedHash []byte
	require.NoError(t, db.Reader().QueryRowContext(
		context.Background(),
		"SELECT token_hash FROM sessions WHERE id = ?",
		first.ID,
	).Scan(&storedHash))
	expectedHash := sha256.Sum256([]byte(first.Token))
	require.Equal(t, expectedHash[:], storedHash)
	require.NotEqual(t, []byte(first.Token), storedHash)

	second, err := service.Login(context.Background(), "owner", "correct horse battery staple", "client-1")
	require.NoError(t, err)
	require.NotEqual(t, first.Token, second.Token)
	_, err = service.Authenticate(context.Background(), first.Token)
	require.ErrorIs(t, err, ErrInvalidSession)
	identity, err := service.Authenticate(context.Background(), second.Token)
	require.NoError(t, err)
	require.Equal(t, "owner", identity.User.Username)
}

func TestLoginRateLimitsRepeatedFailures(t *testing.T) {
	service, _, clock := newTestService(t)
	_, err := service.Initialize(context.Background(), "owner", "correct horse battery staple")
	require.NoError(t, err)

	for range defaultLoginFailureLimit {
		_, err = service.Login(context.Background(), "owner", "wrong password", "client-1")
		require.ErrorIs(t, err, ErrInvalidCredentials)
	}
	_, err = service.Login(context.Background(), "owner", "correct horse battery staple", "client-1")
	require.ErrorIs(t, err, ErrRateLimited)

	clock.Advance(defaultLoginBlockDuration + time.Second)
	_, err = service.Login(context.Background(), "owner", "correct horse battery staple", "client-1")
	require.NoError(t, err)
}

func TestSessionExpiryRevocationAndLastSeenThrottle(t *testing.T) {
	service, db, clock := newTestService(t)
	_, err := service.Initialize(context.Background(), "owner", "correct horse battery staple")
	require.NoError(t, err)
	session, err := service.Login(context.Background(), "owner", "correct horse battery staple", "client-1")
	require.NoError(t, err)

	initialLastSeen := session.LastSeenAt
	clock.Advance(time.Minute)
	_, err = service.Authenticate(context.Background(), session.Token)
	require.NoError(t, err)
	require.Equal(t, initialLastSeen, sessionLastSeen(t, db, session.ID))

	clock.Advance(defaultLastSeenInterval)
	_, err = service.Authenticate(context.Background(), session.Token)
	require.NoError(t, err)
	require.True(t, sessionLastSeen(t, db, session.ID).After(initialLastSeen))

	require.NoError(t, service.Revoke(context.Background(), session.Token))
	_, err = service.Authenticate(context.Background(), session.Token)
	require.ErrorIs(t, err, ErrInvalidSession)

	replacement, err := service.Login(context.Background(), "owner", "correct horse battery staple", "client-1")
	require.NoError(t, err)
	clock.Advance(defaultSessionTTL + time.Second)
	_, err = service.Authenticate(context.Background(), replacement.Token)
	require.ErrorIs(t, err, ErrInvalidSession)
}

type testClock struct {
	now time.Time
}

func (clock *testClock) Now() time.Time {
	return clock.now
}

func (clock *testClock) Advance(duration time.Duration) {
	clock.now = clock.now.Add(duration)
}

func newTestService(t *testing.T) (*Service, *storage.DB, *testClock) {
	t.Helper()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(context.Background(), db))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	clock := &testClock{now: time.Date(2026, time.July, 13, 12, 0, 0, 0, time.UTC)}
	service := NewService(NewRepository(db), ServiceOptions{Now: clock.Now})
	return service, db, clock
}

func sessionLastSeen(t *testing.T, db *storage.DB, sessionID string) time.Time {
	t.Helper()
	var milliseconds int64
	require.NoError(t, db.Reader().QueryRowContext(
		context.Background(),
		"SELECT last_seen_at FROM sessions WHERE id = ?",
		sessionID,
	).Scan(&milliseconds))
	return time.UnixMilli(milliseconds).UTC()
}
