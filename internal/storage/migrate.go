package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

var ErrMigrationChecksumMismatch = errors.New("applied migration checksum mismatch")

//go:embed migrations/*.sql
var embeddedMigrations embed.FS

type migration struct {
	version  int
	name     string
	checksum string
	sql      string
}

func Migrate(ctx context.Context, db *DB) error {
	return migrateEmbedded(ctx, db, true)
}

func migrateEmbedded(ctx context.Context, db *DB, createBackup bool) error {
	migrations, err := fs.Sub(embeddedMigrations, "migrations")
	if err != nil {
		return err
	}
	ordered, err := loadMigrations(migrations)
	if err != nil {
		return err
	}
	if createBackup {
		required, err := preMigrationBackupRequired(ctx, db.Reader(), ordered)
		if err != nil {
			return err
		}
		if required {
			if _, err := NewBackupManager(db, BackupOptions{}).Create(ctx); err != nil {
				return fmt.Errorf("create pre-migration backup: %w", err)
			}
		}
	}
	return migrateLoaded(ctx, db.Writer(), ordered)
}

func preMigrationBackupRequired(ctx context.Context, db *sql.DB, ordered []migration) (bool, error) {
	var exists int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations'
	`).Scan(&exists); err != nil {
		return false, err
	}
	if exists == 0 {
		return false, nil
	}
	rows, err := db.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	applied := make(map[int]struct{})
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return false, err
		}
		applied[version] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	if len(applied) == 0 {
		return false, nil
	}
	for _, item := range ordered {
		if _, exists := applied[item.version]; !exists {
			return true, nil
		}
	}
	return false, nil
}

func migrate(ctx context.Context, db *sql.DB, migrations fs.FS) error {
	ordered, err := loadMigrations(migrations)
	if err != nil {
		return err
	}
	return migrateLoaded(ctx, db, ordered)
}

func migrateLoaded(ctx context.Context, db *sql.DB, ordered []migration) error {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			checksum TEXT NOT NULL,
			applied_at TEXT NOT NULL
		) STRICT
	`); err != nil {
		return err
	}

	for _, item := range ordered {
		var name, checksum string
		err := db.QueryRowContext(ctx,
			"SELECT name, checksum FROM schema_migrations WHERE version = ?",
			item.version,
		).Scan(&name, &checksum)
		switch {
		case err == nil:
			if name != item.name || checksum != item.checksum {
				return fmt.Errorf("%w: version %04d", ErrMigrationChecksumMismatch, item.version)
			}
			continue
		case !errors.Is(err, sql.ErrNoRows):
			return err
		}

		if err := applyMigration(ctx, db, item); err != nil {
			return fmt.Errorf("apply migration %04d: %w", item.version, err)
		}
	}
	return nil
}

func loadMigrations(migrations fs.FS) ([]migration, error) {
	paths, err := fs.Glob(migrations, "*.sql")
	if err != nil {
		return nil, err
	}
	ordered := make([]migration, 0, len(paths))
	seenVersions := make(map[int]struct{}, len(paths))
	for _, path := range paths {
		contents, err := fs.ReadFile(migrations, path)
		if err != nil {
			return nil, err
		}
		name := filepath.Base(path)
		separator := strings.IndexByte(name, '_')
		if separator < 1 || !strings.HasSuffix(name, ".sql") {
			return nil, fmt.Errorf("invalid migration filename %q", name)
		}
		version, err := strconv.Atoi(name[:separator])
		if err != nil {
			return nil, fmt.Errorf("invalid migration filename %q", name)
		}
		if _, exists := seenVersions[version]; exists {
			return nil, fmt.Errorf("duplicate migration version %04d", version)
		}
		seenVersions[version] = struct{}{}
		ordered = append(ordered, migration{
			version:  version,
			name:     name,
			checksum: fmt.Sprintf("%x", sha256.Sum256(contents)),
			sql:      string(contents),
		})
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].version < ordered[j].version })
	return ordered, nil
}

func applyMigration(ctx context.Context, db *sql.DB, item migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, item.sql); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO schema_migrations (version, name, checksum, applied_at)
		VALUES (?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
	`, item.version, item.name, item.checksum); err != nil {
		return err
	}
	return tx.Commit()
}
