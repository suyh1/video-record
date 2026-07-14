package storage

import (
	"context"
	"database/sql"
	"io/fs"
	"os"
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

func TestMigrateReportsClosedDatabase(t *testing.T) {
	database, err := sql.Open("sqlite", sqliteDSN(filepath.Join(t.TempDir(), "closed.db"), false))
	require.NoError(t, err)
	require.NoError(t, database.Close())

	require.Error(t, migrate(context.Background(), database, fstest.MapFS{}))
}

func TestMigrateCreatesPreUpgradeBackup(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	db, err := Open(ctx, filepath.Join(dataDir, "video-record.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	currentVersion := migrateToPreviousEmbeddedVersion(t, ctx, db)
	_, err = db.Writer().ExecContext(ctx, `
		INSERT INTO app_metadata (key, value) VALUES ('pre-upgrade-marker', 'preserved')
	`)
	require.NoError(t, err)

	require.NoError(t, Migrate(ctx, db))

	manager := NewBackupManager(db, BackupOptions{})
	artifacts, err := manager.List(ctx)
	require.NoError(t, err)
	require.Len(t, artifacts, 1)
	require.Equal(t, currentVersion, artifacts[0].Manifest.SchemaVersion)
	archive, err := manager.Read(artifacts[0].Filename)
	require.NoError(t, err)
	_, databaseBytes, err := parseBackupArchive(archive)
	require.NoError(t, err)
	backupDatabase := filepath.Join(t.TempDir(), "pre-upgrade.db")
	require.NoError(t, os.WriteFile(backupDatabase, databaseBytes, 0o600))
	snapshot, err := sql.Open("sqlite", sqliteDSN(backupDatabase, true))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, snapshot.Close()) })
	var marker string
	require.NoError(t, snapshot.QueryRowContext(ctx,
		"SELECT value FROM app_metadata WHERE key = 'pre-upgrade-marker'",
	).Scan(&marker))
	require.Equal(t, "preserved", marker)
}

func TestMigrateStopsBeforeUpgradeWhenBackupFails(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	db, err := Open(ctx, filepath.Join(dataDir, "video-record.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	previousVersion := migrateToPreviousEmbeddedVersion(t, ctx, db)
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, "backups"), []byte("blocked"), 0o600))

	err = Migrate(ctx, db)

	require.Error(t, err)
	var version int
	require.NoError(t, db.Reader().QueryRowContext(ctx,
		"SELECT COALESCE(MAX(version), 0) FROM schema_migrations",
	).Scan(&version))
	require.Equal(t, previousVersion, version)
}

func migrateToPreviousEmbeddedVersion(t *testing.T, ctx context.Context, db *DB) int {
	t.Helper()
	migrationFS, err := fs.Sub(embeddedMigrations, "migrations")
	require.NoError(t, err)
	ordered, err := loadMigrations(migrationFS)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(ordered), 2)
	previous := fstest.MapFS{}
	for _, item := range ordered[:len(ordered)-1] {
		previous[item.name] = &fstest.MapFile{Data: []byte(item.sql)}
	}
	require.NoError(t, migrate(ctx, db.Writer(), previous))
	return ordered[len(ordered)-2].version
}
