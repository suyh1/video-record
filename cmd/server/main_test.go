package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"video-record/internal/integrations"
	"video-record/internal/storage"
)

func TestNewBackupManagerCleansInterruptedArtifacts(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	db, err := storage.Open(ctx, filepath.Join(dataDir, "video-record.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	backupsDir := filepath.Join(dataDir, "backups")
	require.NoError(t, os.MkdirAll(backupsDir, 0o700))
	stalePath := filepath.Join(backupsDir, ".snapshot-interrupted.db")
	require.NoError(t, os.WriteFile(stalePath, []byte("synthetic"), 0o600))

	manager, err := newBackupManager(ctx, db, backupsDir, false)

	require.NoError(t, err)
	require.NotNil(t, manager)
	_, err = os.Stat(stalePath)
	require.ErrorIs(t, err, os.ErrNotExist)
}

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
