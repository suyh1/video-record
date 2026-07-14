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

func TestEpisodeIdentityMigrationBackfillsAbsoluteNumbers(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	migrateToEmbeddedVersion(t, ctx, db, 11)
	_, err = db.Writer().ExecContext(ctx, `
		INSERT INTO media_items (
			id, media_type, external_title, original_title, release_date,
			external_overview, poster_path, backdrop_path, created_at, updated_at
		) VALUES ('series', 'tv', '剧集', '', '', '', '', '', 1, 1);
		INSERT INTO seasons (id, media_id, season_number, name, overview, poster_path, air_date)
		VALUES ('season-1', 'series', 1, '第 1 季', '', '', '');
		INSERT INTO episodes (id, season_id, episode_number, name, overview, still_path, air_date)
		VALUES ('episode-1', 'season-1', 1, '', '', '', ''),
		       ('episode-2', 'season-1', 2, '', '', '', '');
	`)
	require.NoError(t, err)

	require.NoError(t, Migrate(ctx, db))
	var first, second int
	require.NoError(t, db.Reader().QueryRowContext(ctx, "SELECT absolute_number FROM episodes WHERE id = 'episode-1'").Scan(&first))
	require.NoError(t, db.Reader().QueryRowContext(ctx, "SELECT absolute_number FROM episodes WHERE id = 'episode-2'").Scan(&second))
	require.Equal(t, 1, first)
	require.Equal(t, 2, second)
}

func TestViewingRoundsMigrationClearsRecordFactsAndPreservesConfiguration(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	migrateToPreviousEmbeddedVersion(t, ctx, db)

	_, err = db.Writer().ExecContext(ctx, `
		INSERT INTO users (id, username, password_hash, role, active, created_at)
		VALUES ('user-1', 'tester', 'hash', 'admin', 1, 1);
		INSERT INTO media_items (
			id, media_type, external_title, original_title, release_date,
			external_overview, poster_path, backdrop_path, created_at, updated_at
		) VALUES ('series-1', 'tv', '剧集', '', '', '', '', '', 1, 1);
		INSERT INTO media_external_ids (media_id, source, source_id, media_type)
		VALUES ('series-1', 'tmdb', '1001', 'tv');
		INSERT INTO seasons (id, media_id, season_number, name, overview, poster_path, air_date)
		VALUES ('season-1', 'series-1', 1, '第 1 季', '', '', '');
		INSERT INTO episodes (id, season_id, episode_number, name, overview, still_path, air_date)
		VALUES ('episode-1', 'season-1', 1, '第一集', '', '', '');
		INSERT INTO user_media_states (
			user_id, media_id, status, rating, note, started_at, completed_at, version,
			status_source, rating_source, note_source, updated_at,
			share_rating, share_review, shared_review
		) VALUES (
			'user-1', 'series-1', 'completed', 90, '旧笔记',
			'2026-07-01T12:00:00.000000000Z', '2026-07-02T12:00:00.000000000Z',
			1, 'manual', 'manual', 'manual', 1, 1, 1, '旧公开短评'
		);
		INSERT INTO tags (id, user_id, name) VALUES ('tag-1', 'user-1', '旧标签');
		INSERT INTO user_media_tags (user_id, media_id, tag_id)
		VALUES ('user-1', 'series-1', 'tag-1');
		INSERT INTO collections (id, user_id, name) VALUES ('collection-1', 'user-1', '保留片单');
		INSERT INTO collection_items (collection_id, media_id, position)
		VALUES ('collection-1', 'series-1', 0);
		INSERT INTO watch_events (
			id, created_by_user_id, media_id, episode_id, watched_at,
			viewing_method, source, completion, note, created_at
		) VALUES (
			'event-1', 'user-1', 'series-1', 'episode-1',
			'2026-07-02T12:00:00.000000000Z', '电视', 'manual', 100, '旧事件', 1
		);
		INSERT INTO watch_event_participants (event_id, user_id) VALUES ('event-1', 'user-1');
		INSERT INTO episode_progress (
			user_id, media_id, episode_id, watched_at, source, watch_event_id, updated_at
		) VALUES (
			'user-1', 'series-1', 'episode-1',
			'2026-07-02T12:00:00.000000000Z', 'manual', 'event-1', 1
		);
		INSERT INTO external_accounts (
			id, user_id, provider, name, base_url, credential_ciphertext,
			credential_nonce, credential_version, credential_fingerprint,
			enabled, created_at, updated_at
		) VALUES (
			'account-1', 'user-1', 'plex', 'Plex', 'https://example.invalid',
			x'01', x'02', 1, 'fingerprint', 1, 1, 1
		);
		INSERT INTO sync_candidates (
			id, account_id, external_event_id, status, payload_json,
			media_id, episode_id, created_at, updated_at
		) VALUES (
			'candidate-1', 'account-1', 'external-1', 'confirmed', '{}',
			'series-1', 'episode-1', 1, 1
		);
	`)
	require.NoError(t, err)

	require.NoError(t, Migrate(ctx, db))

	require.Equal(t, 1, tableRowCount(t, db, "users"))
	require.Equal(t, 1, tableRowCount(t, db, "media_items"))
	require.Equal(t, 1, tableRowCount(t, db, "media_external_ids"))
	require.Equal(t, 1, tableRowCount(t, db, "tags"))
	require.Equal(t, 1, tableRowCount(t, db, "collections"))
	require.Equal(t, 1, tableRowCount(t, db, "collection_items"))
	require.Equal(t, 1, tableRowCount(t, db, "external_accounts"))
	require.Zero(t, tableRowCount(t, db, "user_media_tags"))
	require.Zero(t, tableRowCount(t, db, "sync_candidates"))
	require.Zero(t, tableRowCount(t, db, "user_media_profiles"))
	require.Zero(t, tableRowCount(t, db, "watch_rounds"))
	require.Zero(t, tableRowCount(t, db, "watch_events"))
	require.Zero(t, tableRowCount(t, db, "round_episode_progress"))
	requireNoForeignKeyViolations(t, db)
}

func TestViewingRoundsMigrationEnforcesCurrentRoundUniqueness(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.NoError(t, Migrate(ctx, db))

	_, err = db.Writer().ExecContext(ctx, `
		INSERT INTO users (id, username, password_hash, role, active, created_at)
		VALUES ('user-1', 'tester', 'hash', 'admin', 1, 1);
		INSERT INTO media_items (
			id, media_type, external_title, original_title, release_date,
			external_overview, poster_path, backdrop_path, created_at, updated_at
		) VALUES
			('movie-1', 'movie', '电影', '', '', '', '', '', 1, 1),
			('series-1', 'tv', '剧集', '', '', '', '', '', 1, 1);
		INSERT INTO watch_rounds (
			id, user_id, media_id, season_number, round_number, status, version,
			status_source, rating_source, note_source, created_at, updated_at
		) VALUES
			('movie-round-1', 'user-1', 'movie-1', NULL, 1, 'watching', 1,
			 'manual', 'manual', 'manual', 1, 1),
			('season-1-round-1', 'user-1', 'series-1', 1, 1, 'watching', 1,
			 'manual', 'manual', 'manual', 1, 1),
			('season-2-round-1', 'user-1', 'series-1', 2, 1, 'watching', 1,
			 'manual', 'manual', 'manual', 1, 1);
	`)
	require.NoError(t, err)

	_, err = db.Writer().ExecContext(ctx, `
		INSERT INTO watch_rounds (
			id, user_id, media_id, season_number, round_number, status, version,
			status_source, rating_source, note_source, created_at, updated_at
		) VALUES (
			'movie-round-2', 'user-1', 'movie-1', NULL, 2, 'watching', 1,
			'manual', 'manual', 'manual', 1, 1
		)
	`)
	require.Error(t, err)

	_, err = db.Writer().ExecContext(ctx, `
		INSERT INTO watch_rounds (
			id, user_id, media_id, season_number, round_number, status, version,
			status_source, rating_source, note_source, created_at, updated_at
		) VALUES (
			'season-1-round-2', 'user-1', 'series-1', 1, 2, 'watching', 1,
			'manual', 'manual', 'manual', 1, 1
		)
	`)
	require.Error(t, err)
	requireNoForeignKeyViolations(t, db)
}

func tableRowCount(t *testing.T, db *DB, table string) int {
	t.Helper()
	var count int
	require.NoError(t, db.Reader().QueryRowContext(
		context.Background(), "SELECT COUNT(*) FROM "+table,
	).Scan(&count))
	return count
}

func requireNoForeignKeyViolations(t *testing.T, db *DB) {
	t.Helper()
	rows, err := db.Reader().QueryContext(context.Background(), "PRAGMA foreign_key_check")
	require.NoError(t, err)
	defer func() { require.NoError(t, rows.Close()) }()
	require.False(t, rows.Next())
	require.NoError(t, rows.Err())
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

func migrateToEmbeddedVersion(t *testing.T, ctx context.Context, db *DB, targetVersion int) {
	t.Helper()
	migrationFS, err := fs.Sub(embeddedMigrations, "migrations")
	require.NoError(t, err)
	ordered, err := loadMigrations(migrationFS)
	require.NoError(t, err)
	selected := fstest.MapFS{}
	found := false
	for _, item := range ordered {
		if item.version > targetVersion {
			break
		}
		selected[item.name] = &fstest.MapFile{Data: []byte(item.sql)}
		found = found || item.version == targetVersion
	}
	require.True(t, found, "embedded migration version %d must exist", targetVersion)
	require.NoError(t, migrate(ctx, db.Writer(), selected))
}
