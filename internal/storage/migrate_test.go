package storage

import (
	"context"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

func TestMigrateIsIdempotent(t *testing.T) {
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	require.NoError(t, Migrate(context.Background(), db))
	require.NoError(t, Migrate(context.Background(), db))

	var applied int
	require.NoError(t, db.Writer().QueryRowContext(
		context.Background(),
		"SELECT COUNT(*) FROM schema_migrations WHERE version = 1",
	).Scan(&applied))
	require.Equal(t, 1, applied)
}

func TestMigrateRejectsChangedAppliedMigration(t *testing.T) {
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	first := fstest.MapFS{
		"0001_core.sql": {Data: []byte("CREATE TABLE app_metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL);")},
	}
	changed := fstest.MapFS{
		"0001_core.sql": {Data: []byte("CREATE TABLE changed_metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL);")},
	}

	require.NoError(t, migrate(context.Background(), db.Writer(), first))
	err = migrate(context.Background(), db.Writer(), changed)

	require.ErrorIs(t, err, ErrMigrationChecksumMismatch)
}
