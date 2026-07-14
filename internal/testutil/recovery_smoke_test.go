package testutil_test

import (
	"bytes"
	"context"
	"database/sql/driver"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	modernsqlite "modernc.org/sqlite"

	"video-record/internal/storage"
	syncdomain "video-record/internal/sync"
	"video-record/internal/testutil"
)

func TestRecoverySmoke(t *testing.T) {
	if os.Getenv("VIDEO_RECORD_RECOVERY_SMOKE") != "1" {
		t.Skip("run through scripts/recovery-smoke.sh")
	}
	for _, mode := range []string{"write", "sync", "backup"} {
		t.Run(mode, func(t *testing.T) {
			directory := t.TempDir()
			databasePath := filepath.Join(directory, "video-record.db")
			prepareRecoveryDatabase(t, databasePath)
			signalPath := filepath.Join(directory, "ready")
			command := exec.Command(os.Args[0], "-test.run=^TestRecoveryHelperProcess$", "-test.v")
			command.Env = append(os.Environ(),
				"VIDEO_RECORD_RECOVERY_HELPER=1",
				"VIDEO_RECORD_RECOVERY_MODE="+mode,
				"VIDEO_RECORD_RECOVERY_DATABASE="+databasePath,
				"VIDEO_RECORD_RECOVERY_SIGNAL="+signalPath,
			)
			var output bytes.Buffer
			command.Stdout = &output
			command.Stderr = &output
			require.NoError(t, command.Start())
			cleanupRecoveryHelper(t, command)
			waitForRecoveryHelper(t, signalPath, &output)
			require.NoError(t, command.Process.Kill())
			require.Error(t, command.Wait())
			verifyRecoveredDatabase(t, databasePath, mode)
		})
	}
}

func TestRecoveryHelperProcess(t *testing.T) {
	if os.Getenv("VIDEO_RECORD_RECOVERY_HELPER") != "1" {
		return
	}
	ctx := context.Background()
	mode := os.Getenv("VIDEO_RECORD_RECOVERY_MODE")
	if mode == "sync" {
		registerSyncRecoveryPause(t)
	}
	db, err := storage.Open(ctx, os.Getenv("VIDEO_RECORD_RECOVERY_DATABASE"))
	require.NoError(t, err)
	if mode == "backup" {
		manager := storage.NewBackupManager(db, storage.BackupOptions{
			BackupsDir: filepath.Join(filepath.Dir(db.Path()), "backups"),
			AfterSnapshot: func() error {
				signalRecoveryHelper(t)
				return nil
			},
		})
		_, _ = manager.Create(ctx)
		return
	}
	if mode == "sync" {
		_, err = db.Writer().ExecContext(ctx, `
			CREATE TEMP TRIGGER recovery_sync_pause
			BEFORE INSERT ON sync_candidates
			BEGIN
				SELECT video_record_sync_recovery_pause();
			END
		`)
		require.NoError(t, err)
		_, err = syncdomain.NewCandidateService(db, syncdomain.CandidateServiceOptions{}).
			Ingest(ctx, "recovery-account", performanceHistoryEvent("recovery", 0, 0))
		t.Fatalf("sync ingest returned before process termination: %v", err)
	}
	tx, err := db.Writer().BeginTx(ctx, nil)
	require.NoError(t, err)
	switch mode {
	case "write":
		_, err = tx.ExecContext(ctx, `
			INSERT INTO media_items (
				id, media_type, external_title, original_title, release_date,
				external_overview, poster_path, backdrop_path, custom_title,
				created_at, updated_at
			) VALUES ('recovery-write-probe', 'movie', '', '', '', '', '', '', 'Probe', 0, 0)
		`)
	default:
		t.Fatalf("unknown recovery mode %q", mode)
	}
	require.NoError(t, err)
	signalRecoveryHelper(t)
}

func signalRecoveryHelper(t *testing.T) {
	t.Helper()
	require.NoError(t, os.WriteFile(os.Getenv("VIDEO_RECORD_RECOVERY_SIGNAL"), []byte("ready"), 0o600))
	select {}
}

func registerSyncRecoveryPause(t *testing.T) {
	t.Helper()
	err := modernsqlite.RegisterScalarFunction(
		"video_record_sync_recovery_pause",
		0,
		func(_ *modernsqlite.FunctionContext, _ []driver.Value) (driver.Value, error) {
			if err := os.WriteFile(os.Getenv("VIDEO_RECORD_RECOVERY_SIGNAL"), []byte("ready"), 0o600); err != nil {
				return nil, err
			}
			select {}
		},
	)
	require.NoError(t, err)
}

func cleanupRecoveryHelper(t *testing.T, command *exec.Cmd) {
	t.Helper()
	t.Cleanup(func() {
		if command.ProcessState == nil {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	})
}

func waitForRecoveryHelper(t *testing.T, signalPath string, output *bytes.Buffer) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(signalPath); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("recovery helper did not signal readiness: %s", output.String())
}

func prepareRecoveryDatabase(t *testing.T, databasePath string) {
	t.Helper()
	ctx := context.Background()
	db, err := storage.Open(ctx, databasePath)
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(ctx, db))
	seeded, err := testutil.Seed(ctx, db, testutil.SeedOptions{
		Users: 1, MediaItems: 1, WatchEvents: 1,
		Password: stringsJoin("Synthetic", "recovery", "password"),
		Now:      time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	_, err = db.Writer().ExecContext(ctx, `
		INSERT INTO external_accounts (
			id, user_id, provider, name, base_url, credential_ciphertext, credential_nonce,
			credential_version, credential_fingerprint, enabled, created_at, updated_at
		) VALUES ('recovery-account', ?, 'jellyfin', 'Synthetic recovery provider',
			'http://127.0.0.1', X'01', X'02', 1, 'synthetic-fingerprint', 1, 0, 0)
	`, seeded.UserIDs[0])
	require.NoError(t, err)
	require.NoError(t, db.Close())
}

func verifyRecoveredDatabase(t *testing.T, databasePath, mode string) {
	t.Helper()
	ctx := context.Background()
	db, err := storage.Open(ctx, databasePath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.NoError(t, storage.Migrate(ctx, db))
	require.NoError(t, db.Ready(ctx))
	var integrity string
	require.NoError(t, db.Reader().QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity))
	require.Equal(t, "ok", integrity)
	var mediaCount int
	require.NoError(t, db.Reader().QueryRowContext(ctx, "SELECT COUNT(*) FROM media_items").Scan(&mediaCount))
	require.Equal(t, 1, mediaCount)
	switch mode {
	case "write":
		var count int
		require.NoError(t, db.Reader().QueryRowContext(ctx,
			"SELECT COUNT(*) FROM media_items WHERE id = 'recovery-write-probe'").Scan(&count))
		require.Zero(t, count)
	case "sync":
		for query, expected := range map[string]int{
			"SELECT COUNT(*) FROM sync_candidates":                              0,
			"SELECT COUNT(*) FROM external_media_mappings":                      0,
			"SELECT COUNT(*) FROM watch_events":                                 1,
			"SELECT COUNT(*) FROM watch_events WHERE source = 'confirmed_sync'": 0,
			"SELECT COUNT(*) FROM watch_event_participants":                     1,
		} {
			var count int
			require.NoError(t, db.Reader().QueryRowContext(ctx, query).Scan(&count))
			require.Equal(t, expected, count, query)
		}
		var source string
		require.NoError(t, db.Reader().QueryRowContext(ctx,
			"SELECT status_source FROM watch_rounds WHERE media_id = 'perf-media-00000' AND archived_at IS NULL",
		).Scan(&source))
		require.Equal(t, "external_default", source)
	case "backup":
		manager := storage.NewBackupManager(db, storage.BackupOptions{
			BackupsDir: filepath.Join(filepath.Dir(databasePath), "backups"),
		})
		require.NoError(t, manager.CleanupIncomplete(ctx))
		entries, err := os.ReadDir(filepath.Join(filepath.Dir(databasePath), "backups"))
		require.NoError(t, err)
		require.Empty(t, entries)
		artifacts, err := manager.List(ctx)
		require.NoError(t, err)
		require.Empty(t, artifacts)
	}
}
