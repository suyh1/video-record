package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql/driver"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	modernsqlite "modernc.org/sqlite"
)

const recoveryMigrationVersion = 9999

func TestMigrationRecoverySmoke(t *testing.T) {
	if os.Getenv("VIDEO_RECORD_RECOVERY_SMOKE") != "1" {
		t.Skip("run through scripts/recovery-smoke.sh")
	}
	directory := t.TempDir()
	databasePath := filepath.Join(directory, "video-record.db")
	prepareMigrationRecoveryDatabase(t, databasePath)
	signalPath := filepath.Join(directory, "ready")
	command := exec.Command(os.Args[0], "-test.run=^TestMigrationRecoveryHelperProcess$", "-test.v")
	command.Env = append(os.Environ(),
		"VIDEO_RECORD_MIGRATION_RECOVERY_HELPER=1",
		"VIDEO_RECORD_RECOVERY_DATABASE="+databasePath,
		"VIDEO_RECORD_RECOVERY_SIGNAL="+signalPath,
	)
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	require.NoError(t, command.Start())
	cleanupRecoveryProcess(t, command)
	waitForRecoverySignal(t, signalPath, &output)
	require.NoError(t, command.Process.Kill())
	require.Error(t, command.Wait())

	ctx := context.Background()
	db, err := Open(ctx, databasePath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.NoError(t, Migrate(ctx, db))
	require.NoError(t, db.Ready(ctx))
	var integrity string
	require.NoError(t, db.Reader().QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity))
	require.Equal(t, "ok", integrity)
	var count int
	require.NoError(t, db.Reader().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sqlite_master
		WHERE type = 'table' AND name = 'recovery_migration_probe'
	`).Scan(&count))
	require.Zero(t, count)
	require.NoError(t, db.Reader().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM schema_migrations WHERE version = ?", recoveryMigrationVersion,
	).Scan(&count))
	require.Zero(t, count)
}

func TestMigrationRecoveryHelperProcess(t *testing.T) {
	if os.Getenv("VIDEO_RECORD_MIGRATION_RECOVERY_HELPER") != "1" {
		return
	}
	registerMigrationRecoveryPause(t)
	ctx := context.Background()
	db, err := Open(ctx, os.Getenv("VIDEO_RECORD_RECOVERY_DATABASE"))
	require.NoError(t, err)
	migrationSQL := `
		CREATE TABLE recovery_migration_probe (id INTEGER PRIMARY KEY) STRICT;
		SELECT video_record_migration_recovery_pause();
	`
	checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(migrationSQL)))
	err = applyMigration(ctx, db.Writer(), migration{
		version: recoveryMigrationVersion,
		name:    "9999_recovery_probe.sql", checksum: checksum, sql: migrationSQL,
	})
	t.Fatalf("recovery migration returned before process termination: %v", err)
}

func registerMigrationRecoveryPause(t *testing.T) {
	t.Helper()
	err := modernsqlite.RegisterScalarFunction(
		"video_record_migration_recovery_pause",
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

func prepareMigrationRecoveryDatabase(t *testing.T, path string) {
	t.Helper()
	ctx := context.Background()
	db, err := Open(ctx, path)
	require.NoError(t, err)
	require.NoError(t, Migrate(ctx, db))
	require.NoError(t, db.Close())
}

func cleanupRecoveryProcess(t *testing.T, command *exec.Cmd) {
	t.Helper()
	t.Cleanup(func() {
		if command.ProcessState == nil {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	})
}

func waitForRecoverySignal(t *testing.T, path string, output *bytes.Buffer) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("recovery helper did not signal readiness: %s", output.String())
}
