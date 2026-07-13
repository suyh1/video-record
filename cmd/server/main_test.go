package main

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"video-record/internal/integrations"
	"video-record/internal/storage"
)

func TestNewSyncRuntimeInitializesEnabledAccountJobs(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(ctx, db))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	_, err = db.Writer().ExecContext(ctx, `
		INSERT INTO users (id, username, password_hash, role, active, created_at)
		VALUES ('runtime-user', 'runtime-owner', 'synthetic-hash', 'admin', 1, 0)
	`)
	require.NoError(t, err)
	key := bytes.Repeat([]byte{0x61}, 32)
	accounts := integrations.NewAccountRepository(
		db, integrations.NewCredentialCipher(key), integrations.AccountRepositoryOptions{},
	)
	_, err = accounts.Create(ctx, integrations.CreateAccountInput{
		UserID: "runtime-user", Provider: "plex", Name: "Home",
		BaseURL:     "https://plex.example.test",
		Credentials: []byte(`{"token":"synthetic-token","accountId":42}`),
		Enabled:     true,
	})
	require.NoError(t, err)

	runtime, err := newSyncRuntime(ctx, db, key)

	require.NoError(t, err)
	require.NotNil(t, runtime.candidates)
	require.NotNil(t, runtime.scheduler)
	var jobs int
	require.NoError(t, db.Reader().QueryRowContext(ctx, "SELECT COUNT(*) FROM sync_jobs").Scan(&jobs))
	require.Equal(t, 2, jobs)
}
