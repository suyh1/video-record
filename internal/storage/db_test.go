package storage

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenConfiguresSQLiteConnections(t *testing.T) {
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	require.Equal(t, 1, db.Writer().Stats().MaxOpenConnections)
	require.Equal(t, maxReaderConnections, db.Reader().Stats().MaxOpenConnections)
	requirePragmaValue(t, db.Writer(), "foreign_keys", "1")
	requirePragmaValue(t, db.Reader(), "foreign_keys", "1")
	requirePragmaValue(t, db.Writer(), "journal_mode", "wal")
	requirePragmaValue(t, db.Writer(), "busy_timeout", "5000")
}

func TestOpenCreatesPrivateDataDirectory(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "nested", "data")
	db, err := Open(context.Background(), filepath.Join(dataDir, "video-record.db"))
	require.NoError(t, err)
	require.NoError(t, db.Close())

	info, err := os.Stat(dataDir)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o700), info.Mode().Perm())
}

func TestReadyRequiresAppliedMigrationsAndWritableStorage(t *testing.T) {
	ctx := context.Background()
	require.ErrorIs(t, (&DB{}).Ready(ctx), sql.ErrConnDone)
	db, err := Open(ctx, filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	_, err = db.Writer().ExecContext(ctx, `
		CREATE TABLE schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at INTEGER NOT NULL
		)
	`)
	require.NoError(t, err)
	require.ErrorIs(t, db.Ready(ctx), ErrNotMigrated)
	_, err = db.Writer().ExecContext(ctx, "INSERT INTO schema_migrations (version, applied_at) VALUES (1, 0)")
	require.NoError(t, err)
	require.NoError(t, db.Ready(ctx))
	require.NoError(t, db.Writer().Close())
	require.Error(t, db.Ready(ctx))
}

func requirePragmaValue(t *testing.T, db *sql.DB, pragma, expected string) {
	t.Helper()
	var value string
	require.NoError(t, db.QueryRowContext(context.Background(), "PRAGMA "+pragma).Scan(&value))
	require.Equal(t, expected, value)
}
