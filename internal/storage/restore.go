package storage

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
)

const (
	maxManifestBytes = 1 << 20
	maxDatabaseBytes = 4 << 30
)

func (manager *BackupManager) Restore(ctx context.Context, archive []byte) (RestoreResult, error) {
	staged, err := manager.StageRestore(ctx, bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return RestoreResult{}, err
	}
	defer staged.Remove()
	return manager.RestoreStaged(ctx, staged, nil)
}

func (manager *BackupManager) RestoreWithCommit(
	ctx context.Context,
	archive []byte,
	commit func(context.Context, *DB) error,
) (RestoreResult, error) {
	staged, err := manager.StageRestore(ctx, bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return RestoreResult{}, err
	}
	defer staged.Remove()
	return manager.RestoreStaged(ctx, staged, commit)
}

type StagedRestore struct {
	path   string
	SHA256 string
	Bytes  int64
}

func (staged StagedRestore) Remove() {
	_ = os.Remove(staged.path)
}

func (manager *BackupManager) StageRestore(ctx context.Context, source io.Reader, maxBytes int64) (StagedRestore, error) {
	if maxBytes < 1 {
		return StagedRestore{}, ErrBackupTooLarge
	}
	if err := os.MkdirAll(manager.backupsDir, 0o700); err != nil {
		return StagedRestore{}, err
	}
	path := filepath.Join(manager.backupsDir, ".restore-upload-"+uuid.NewString()+".partial")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return StagedRestore{}, err
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(source, maxBytes+1))
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(copyErr, syncErr, closeErr); err != nil {
		_ = os.Remove(path)
		return StagedRestore{}, err
	}
	if written > maxBytes {
		_ = os.Remove(path)
		return StagedRestore{}, ErrBackupTooLarge
	}
	if err := syncDirectory(manager.backupsDir); err != nil {
		_ = os.Remove(path)
		return StagedRestore{}, err
	}
	return StagedRestore{path: path, SHA256: fmt.Sprintf("%x", hash.Sum(nil)), Bytes: written}, nil
}

func (manager *BackupManager) RestoreStaged(
	ctx context.Context,
	staged StagedRestore,
	commit func(context.Context, *DB) error,
) (RestoreResult, error) {
	if filepath.Dir(staged.path) != filepath.Clean(manager.backupsDir) ||
		!strings.HasPrefix(filepath.Base(staged.path), ".restore-upload-") {
		return RestoreResult{}, ErrInvalidBackup
	}
	info, err := os.Lstat(staged.path)
	if err != nil || !info.Mode().IsRegular() {
		return RestoreResult{}, ErrInvalidBackup
	}
	if err := manager.db.gate.beginRestore(ctx); err != nil {
		return RestoreResult{}, err
	}
	defer manager.db.gate.endRestore()
	manifest, err := readBackupManifestFile(staged.path)
	if err != nil {
		return RestoreResult{}, err
	}
	currentSchema, err := databaseSchemaVersion(ctx, manager.db.Reader())
	if err != nil {
		return RestoreResult{}, err
	}
	if manifest.SchemaVersion > currentSchema {
		return RestoreResult{}, ErrIncompatibleBackup
	}
	if err := manager.checkRestoreSpace(ctx, uint64(manifest.DatabaseBytes)); err != nil {
		return RestoreResult{}, err
	}
	replacement := manager.db.Path() + ".restore-" + uuid.NewString()
	defer removeSQLiteFiles(replacement)
	if err := extractBackupDatabase(staged.path, replacement, manifest); err != nil {
		return RestoreResult{}, err
	}
	if err := validateReplacement(ctx, replacement, manifest.SchemaVersion, currentSchema); err != nil {
		return RestoreResult{}, err
	}
	preRestore, err := manager.Create(ctx)
	if err != nil {
		return RestoreResult{}, err
	}
	if err := manager.db.gate.begin(ctx); err != nil {
		return RestoreResult{}, err
	}
	defer manager.db.gate.endMaintenance()
	criticalCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), manager.criticalTimeout)
	defer cancel()
	if err := manager.replaceDatabase(criticalCtx, replacement, commit); err != nil {
		return RestoreResult{}, err
	}
	result := RestoreResult{PreRestoreBackup: preRestore.Filename, Warnings: []string{}}
	if manifest.RequiresEncryptionKey && !manager.encryptionKeyAvailable {
		result.Warnings = append(result.Warnings, "integrations_locked")
	}
	return result, nil
}

func (manager *BackupManager) checkRestoreSpace(ctx context.Context, incomingBytes uint64) error {
	currentBytes, err := databaseLogicalBytes(ctx, manager.db.Reader())
	if err != nil {
		return err
	}
	const archiveOverhead = uint64(1 << 20)
	if currentBytes > (^uint64(0)-archiveOverhead)/2 {
		return ErrInsufficientSpace
	}
	backupRequired := currentBytes*2 + archiveOverhead
	databaseDir := filepath.Dir(manager.db.Path())
	if err := os.MkdirAll(manager.backupsDir, 0o700); err != nil {
		return err
	}
	same, err := manager.sameFilesystem(databaseDir, manager.backupsDir)
	if err != nil {
		return err
	}
	databaseAvailable, err := manager.availableBytes(databaseDir)
	if err != nil {
		return err
	}
	if same {
		if incomingBytes > ^uint64(0)-backupRequired || databaseAvailable < incomingBytes+backupRequired {
			return ErrInsufficientSpace
		}
		return nil
	}
	if databaseAvailable < incomingBytes {
		return ErrInsufficientSpace
	}
	backupAvailable, err := manager.availableBytes(manager.backupsDir)
	if err != nil {
		return err
	}
	if backupAvailable < backupRequired {
		return ErrInsufficientSpace
	}
	return nil
}

func databaseLogicalBytes(ctx context.Context, database *sql.DB) (uint64, error) {
	var pageCount, pageSize uint64
	if err := database.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pageCount); err != nil {
		return 0, err
	}
	if err := database.QueryRowContext(ctx, "PRAGMA page_size").Scan(&pageSize); err != nil {
		return 0, err
	}
	return pageCount * pageSize, nil
}

func sameFilesystem(first, second string) (bool, error) {
	firstInfo, err := os.Stat(first)
	if err != nil {
		return false, err
	}
	secondInfo, err := os.Stat(second)
	if err != nil {
		return false, err
	}
	firstStat, firstOK := firstInfo.Sys().(*syscall.Stat_t)
	secondStat, secondOK := secondInfo.Sys().(*syscall.Stat_t)
	if !firstOK || !secondOK {
		return false, errors.New("filesystem identity unavailable")
	}
	return firstStat.Dev == secondStat.Dev, nil
}

func readBackupManifestFile(path string) (BackupManifest, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return BackupManifest{}, ErrInvalidBackup
	}
	defer func() { _ = reader.Close() }()
	manifestFile, databaseFile, err := backupArchiveEntries(reader.File)
	if err != nil {
		return BackupManifest{}, err
	}
	entry, err := manifestFile.Open()
	if err != nil {
		return BackupManifest{}, ErrInvalidBackup
	}
	contents, readErr := io.ReadAll(io.LimitReader(entry, maxManifestBytes+1))
	_ = entry.Close()
	var manifest BackupManifest
	if readErr != nil || len(contents) > maxManifestBytes || json.Unmarshal(contents, &manifest) != nil {
		return BackupManifest{}, ErrInvalidBackup
	}
	if manifest.FormatVersion != backupFormatVersion || manifest.SchemaVersion < 1 || manifest.DatabaseBytes < 1 ||
		databaseFile.UncompressedSize64 != uint64(manifest.DatabaseBytes) {
		return BackupManifest{}, ErrInvalidBackup
	}
	return manifest, nil
}

func extractBackupDatabase(path, destination string, manifest BackupManifest) error {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return ErrInvalidBackup
	}
	defer func() { _ = reader.Close() }()
	_, databaseFile, err := backupArchiveEntries(reader.File)
	if err != nil {
		return err
	}
	entry, err := databaseFile.Open()
	if err != nil {
		return ErrInvalidBackup
	}
	defer func() { _ = entry.Close() }()
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(entry, maxDatabaseBytes+1))
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(copyErr, syncErr, closeErr); err != nil {
		return err
	}
	if written != manifest.DatabaseBytes || written > maxDatabaseBytes {
		return ErrInvalidBackup
	}
	if manifest.DatabaseSHA256 != fmt.Sprintf("%x", hash.Sum(nil)) {
		return ErrBackupChecksum
	}
	return syncDirectory(filepath.Dir(destination))
}

func backupArchiveEntries(files []*zip.File) (*zip.File, *zip.File, error) {
	if len(files) != 2 {
		return nil, nil, ErrInvalidBackup
	}
	seen := make(map[string]struct{}, 2)
	var manifest, database *zip.File
	for _, file := range files {
		if filepath.Base(file.Name) != file.Name || strings.Contains(file.Name, `\`) {
			return nil, nil, ErrInvalidBackup
		}
		if _, exists := seen[file.Name]; exists {
			return nil, nil, ErrInvalidBackup
		}
		seen[file.Name] = struct{}{}
		switch file.Name {
		case "manifest.json":
			manifest = file
		case "video-record.db":
			database = file
		default:
			return nil, nil, ErrInvalidBackup
		}
	}
	if manifest == nil || database == nil {
		return nil, nil, ErrInvalidBackup
	}
	return manifest, database, nil
}

func parseBackupArchive(archive []byte) (BackupManifest, []byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil || len(reader.File) != 2 {
		return BackupManifest{}, nil, ErrInvalidBackup
	}
	var manifest BackupManifest
	var database []byte
	seen := make(map[string]struct{}, 2)
	for _, file := range reader.File {
		if filepath.Base(file.Name) != file.Name || strings.Contains(file.Name, `\`) {
			return BackupManifest{}, nil, ErrInvalidBackup
		}
		if _, exists := seen[file.Name]; exists {
			return BackupManifest{}, nil, ErrInvalidBackup
		}
		seen[file.Name] = struct{}{}
		entry, err := file.Open()
		if err != nil {
			return BackupManifest{}, nil, ErrInvalidBackup
		}
		switch file.Name {
		case "manifest.json":
			contents, readErr := io.ReadAll(io.LimitReader(entry, maxManifestBytes+1))
			_ = entry.Close()
			if readErr != nil || len(contents) > maxManifestBytes || json.Unmarshal(contents, &manifest) != nil {
				return BackupManifest{}, nil, ErrInvalidBackup
			}
		case "video-record.db":
			contents, readErr := io.ReadAll(io.LimitReader(entry, maxDatabaseBytes+1))
			_ = entry.Close()
			if readErr != nil || len(contents) > maxDatabaseBytes {
				return BackupManifest{}, nil, ErrInvalidBackup
			}
			database = contents
		default:
			_ = entry.Close()
			return BackupManifest{}, nil, ErrInvalidBackup
		}
	}
	if manifest.FormatVersion != backupFormatVersion || manifest.SchemaVersion < 1 ||
		manifest.DatabaseBytes != int64(len(database)) || len(database) == 0 {
		return BackupManifest{}, nil, ErrInvalidBackup
	}
	if manifest.DatabaseSHA256 != fmt.Sprintf("%x", sha256.Sum256(database)) {
		return BackupManifest{}, nil, ErrBackupChecksum
	}
	return manifest, database, nil
}

func validateReplacement(ctx context.Context, path string, manifestSchema, currentSchema int) error {
	replacement, err := Open(ctx, path)
	if err != nil {
		return ErrInvalidBackup
	}
	defer func() { _ = replacement.Close() }()
	databaseSchema, err := databaseSchemaVersion(ctx, replacement.Reader())
	if err != nil || databaseSchema != manifestSchema {
		return ErrInvalidBackup
	}
	if databaseSchema > currentSchema {
		return ErrIncompatibleBackup
	}
	if err := validateMigrationRows(ctx, replacement.Reader(), databaseSchema); err != nil {
		return err
	}
	if err := migrateEmbedded(ctx, replacement, false); err != nil {
		return ErrIncompatibleBackup
	}
	if _, err := replacement.Writer().ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return ErrInvalidBackup
	}
	var integrity string
	if err := replacement.Reader().QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil {
		return ErrInvalidBackup
	}
	if integrity != "ok" {
		return ErrInvalidBackup
	}
	return nil
}

func validateMigrationRows(ctx context.Context, database *sql.DB, schemaVersion int) error {
	migrationFS, err := fs.Sub(embeddedMigrations, "migrations")
	if err != nil {
		return ErrIncompatibleBackup
	}
	known, err := loadMigrations(migrationFS)
	if err != nil {
		return ErrIncompatibleBackup
	}
	expected := make(map[int]migration, len(known))
	for _, item := range known {
		if item.version <= schemaVersion {
			expected[item.version] = item
		}
	}
	rows, err := database.QueryContext(ctx, "SELECT version, name, checksum FROM schema_migrations")
	if err != nil {
		return ErrInvalidBackup
	}
	defer func() { _ = rows.Close() }()
	seen := 0
	for rows.Next() {
		var version int
		var name, checksum string
		if err := rows.Scan(&version, &name, &checksum); err != nil {
			return ErrInvalidBackup
		}
		item, ok := expected[version]
		if !ok || item.name != name || item.checksum != checksum {
			return ErrIncompatibleBackup
		}
		seen++
	}
	if err := rows.Err(); err != nil {
		return ErrInvalidBackup
	}
	if seen != len(expected) {
		return ErrInvalidBackup
	}
	return nil
}

func (manager *BackupManager) replaceDatabase(
	ctx context.Context,
	replacement string,
	commit func(context.Context, *DB) error,
) error {
	databasePath := manager.db.Path()
	oldPath := databasePath + ".restore-old"
	if _, err := manager.db.Writer().ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return err
	}
	manager.db.mu.Lock()
	closeErr := manager.db.closeConnectionsLocked()
	manager.db.mu.Unlock()
	if closeErr != nil {
		return errors.Join(closeErr, reopenForRecovery(manager.db))
	}
	removeSQLiteFiles(oldPath)
	removeSQLiteSidecars(databasePath)
	if err := os.Link(databasePath, oldPath); err != nil {
		return errors.Join(err, reopenForRecovery(manager.db))
	}
	if err := syncDirectory(filepath.Dir(databasePath)); err != nil {
		removeSQLiteFiles(oldPath)
		return errors.Join(err, reopenForRecovery(manager.db))
	}
	if manager.beforeReplace != nil {
		if err := manager.beforeReplace(databasePath); err != nil {
			removeSQLiteFiles(oldPath)
			return errors.Join(err, reopenForRecovery(manager.db))
		}
	}
	if err := os.Rename(replacement, databasePath); err != nil {
		removeSQLiteFiles(oldPath)
		return errors.Join(err, reopenForRecovery(manager.db))
	}
	if err := syncDirectory(filepath.Dir(databasePath)); err != nil {
		return errors.Join(err, rollbackDatabase(manager.db, databasePath, oldPath))
	}
	if manager.afterReplace != nil {
		if err := manager.afterReplace(); err != nil {
			return errors.Join(err, rollbackDatabase(manager.db, databasePath, oldPath))
		}
	}
	if err := manager.db.reopen(ctx); err != nil {
		return errors.Join(err, rollbackDatabase(manager.db, databasePath, oldPath))
	}
	if commit != nil {
		if err := commit(ctx, manager.db); err != nil {
			return errors.Join(err, rollbackDatabase(manager.db, databasePath, oldPath))
		}
	}
	removeSQLiteFiles(oldPath)
	_ = syncDirectory(filepath.Dir(databasePath))
	return nil
}

func rollbackDatabase(db *DB, databasePath, oldPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db.mu.Lock()
	closeErr := db.closeConnectionsLocked()
	db.mu.Unlock()
	removeSQLiteSidecars(databasePath)
	renameErr := os.Rename(oldPath, databasePath)
	syncErr := syncDirectory(filepath.Dir(databasePath))
	reopenErr := db.reopen(ctx)
	return errors.Join(closeErr, renameErr, syncErr, reopenErr)
}

func reopenForRecovery(db *DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return db.reopen(ctx)
}

func removeSQLiteSidecars(path string) {
	_ = os.Remove(path + "-wal")
	_ = os.Remove(path + "-shm")
}
