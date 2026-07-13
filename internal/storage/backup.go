package storage

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
	"modernc.org/sqlite"
)

const backupFormatVersion = 1

var (
	ErrBackupChecksum     = errors.New("backup checksum mismatch")
	ErrIncompatibleBackup = errors.New("incompatible backup")
	ErrInsufficientSpace  = errors.New("insufficient space for restore")
	ErrInvalidBackup      = errors.New("invalid backup")
	ErrMaintenance        = errors.New("maintenance already active")
	ErrBackupTooLarge     = errors.New("backup archive too large")
)

type BackupManifest struct {
	FormatVersion         int    `json:"formatVersion"`
	SchemaVersion         int    `json:"schemaVersion"`
	CreatedAt             string `json:"createdAt"`
	DatabaseSHA256        string `json:"databaseSha256"`
	DatabaseBytes         int64  `json:"databaseBytes"`
	RequiresEncryptionKey bool   `json:"requiresEncryptionKey"`
}

type BackupArtifact struct {
	Filename string         `json:"filename"`
	Path     string         `json:"-"`
	Bytes    int64          `json:"bytes"`
	Manifest BackupManifest `json:"manifest"`
}

type RestoreResult struct {
	PreRestoreBackup string   `json:"preRestoreBackup"`
	Warnings         []string `json:"warnings"`
}

type BackupOptions struct {
	BackupsDir             string
	Now                    func() time.Time
	AvailableBytes         func(string) (uint64, error)
	SameFilesystem         func(string, string) (bool, error)
	AfterSnapshot          func() error
	BeforeReplace          func(string) error
	AfterReplace           func() error
	CriticalTimeout        time.Duration
	EncryptionKeyAvailable bool
}

type BackupManager struct {
	db                     *DB
	backupsDir             string
	now                    func() time.Time
	availableBytes         func(string) (uint64, error)
	sameFilesystem         func(string, string) (bool, error)
	afterSnapshot          func() error
	afterReplace           func() error
	beforeReplace          func(string) error
	criticalTimeout        time.Duration
	encryptionKeyAvailable bool
}

func NewBackupManager(db *DB, options BackupOptions) *BackupManager {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	availableBytes := options.AvailableBytes
	if availableBytes == nil {
		availableBytes = filesystemAvailableBytes
	}
	sameFilesystemCheck := options.SameFilesystem
	if sameFilesystemCheck == nil {
		sameFilesystemCheck = sameFilesystem
	}
	backupsDir := options.BackupsDir
	if backupsDir == "" {
		backupsDir = filepath.Join(filepath.Dir(db.Path()), "backups")
	}
	criticalTimeout := options.CriticalTimeout
	if criticalTimeout <= 0 {
		criticalTimeout = 30 * time.Second
	}
	return &BackupManager{
		db: db, backupsDir: backupsDir, now: now, availableBytes: availableBytes,
		sameFilesystem:         sameFilesystemCheck,
		afterSnapshot:          options.AfterSnapshot,
		beforeReplace:          options.BeforeReplace,
		afterReplace:           options.AfterReplace,
		criticalTimeout:        criticalTimeout,
		encryptionKeyAvailable: options.EncryptionKeyAvailable,
	}
}

func (manager *BackupManager) CleanupIncomplete(ctx context.Context) error {
	entries, err := os.ReadDir(manager.backupsDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	removed := false
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() || !isIncompleteBackupArtifact(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		if err := os.Remove(filepath.Join(manager.backupsDir, entry.Name())); err != nil {
			return err
		}
		removed = true
	}
	if removed {
		return syncDirectory(manager.backupsDir)
	}
	return nil
}

func isIncompleteBackupArtifact(name string) bool {
	if strings.HasPrefix(name, ".snapshot-") {
		return strings.HasSuffix(name, ".db") || strings.HasSuffix(name, ".db-wal") ||
			strings.HasSuffix(name, ".db-shm")
	}
	return strings.HasPrefix(name, "video-record-") && strings.HasSuffix(name, ".vrbackup.partial") ||
		strings.HasPrefix(name, ".restore-upload-") && strings.HasSuffix(name, ".partial")
}

func (manager *BackupManager) Create(ctx context.Context) (BackupArtifact, error) {
	if err := os.MkdirAll(manager.backupsDir, 0o700); err != nil {
		return BackupArtifact{}, err
	}
	temporaryDatabase := filepath.Join(manager.backupsDir, ".snapshot-"+uuid.NewString()+".db")
	defer removeSQLiteFiles(temporaryDatabase)
	if err := manager.onlineBackup(ctx, temporaryDatabase); err != nil {
		return BackupArtifact{}, err
	}
	if manager.afterSnapshot != nil {
		if err := manager.afterSnapshot(); err != nil {
			return BackupArtifact{}, err
		}
	}
	if err := os.Chmod(temporaryDatabase, 0o600); err != nil {
		return BackupArtifact{}, err
	}
	databaseSHA256, databaseBytes, err := checksumFile(temporaryDatabase)
	if err != nil {
		return BackupArtifact{}, err
	}
	snapshot, err := sql.Open("sqlite", sqliteDSN(temporaryDatabase, true))
	if err != nil {
		return BackupArtifact{}, err
	}
	defer func() { _ = snapshot.Close() }()
	if err := snapshot.PingContext(ctx); err != nil {
		return BackupArtifact{}, err
	}
	schemaVersion, err := databaseSchemaVersion(ctx, snapshot)
	if err != nil {
		return BackupArtifact{}, err
	}
	requiresKey, err := databaseRequiresEncryptionKey(ctx, snapshot)
	if err != nil {
		return BackupArtifact{}, err
	}
	manifest := BackupManifest{
		FormatVersion: backupFormatVersion, SchemaVersion: schemaVersion,
		CreatedAt:      manager.now().UTC().Format(time.RFC3339Nano),
		DatabaseSHA256: databaseSHA256,
		DatabaseBytes:  databaseBytes, RequiresEncryptionKey: requiresKey,
	}
	filename := fmt.Sprintf("video-record-%d-%s.vrbackup", manager.now().UTC().UnixNano(), uuid.NewString())
	finalPath := filepath.Join(manager.backupsDir, filename)
	temporaryArchive := finalPath + ".partial"
	if err := writeArchiveFile(temporaryArchive, manifest, temporaryDatabase); err != nil {
		_ = os.Remove(temporaryArchive)
		return BackupArtifact{}, err
	}
	if err := os.Rename(temporaryArchive, finalPath); err != nil {
		_ = os.Remove(temporaryArchive)
		return BackupArtifact{}, err
	}
	if err := syncDirectory(manager.backupsDir); err != nil {
		return BackupArtifact{}, err
	}
	info, err := os.Stat(finalPath)
	if err != nil {
		return BackupArtifact{}, err
	}
	return BackupArtifact{Filename: filename, Path: finalPath, Bytes: info.Size(), Manifest: manifest}, nil
}

func (manager *BackupManager) List(_ context.Context) ([]BackupArtifact, error) {
	entries, err := os.ReadDir(manager.backupsDir)
	if errors.Is(err, os.ErrNotExist) {
		return []BackupArtifact{}, nil
	}
	if err != nil {
		return nil, err
	}
	artifacts := make([]BackupArtifact, 0)
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(entry.Name(), ".vrbackup") {
			continue
		}
		path := filepath.Join(manager.backupsDir, entry.Name())
		manifest, err := readBackupManifestFile(path)
		if err != nil {
			return nil, err
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, BackupArtifact{
			Filename: entry.Name(), Path: path, Bytes: info.Size(), Manifest: manifest,
		})
	}
	sort.Slice(artifacts, func(i, j int) bool {
		return artifacts[i].Manifest.CreatedAt > artifacts[j].Manifest.CreatedAt
	})
	return artifacts, nil
}

func (manager *BackupManager) Read(filename string) ([]byte, error) {
	file, _, err := manager.Open(filename)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	return io.ReadAll(file)
}

func (manager *BackupManager) Open(filename string) (*os.File, os.FileInfo, error) {
	if filename == "" || filepath.Base(filename) != filename || !strings.HasSuffix(filename, ".vrbackup") {
		return nil, nil, ErrInvalidBackup
	}
	path := filepath.Join(manager.backupsDir, filename)
	info, err := os.Lstat(path)
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, nil, ErrInvalidBackup
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	return file, info, nil
}

func (manager *BackupManager) onlineBackup(ctx context.Context, destination string) error {
	connection, err := manager.db.Reader().Conn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = connection.Close() }()
	return connection.Raw(func(driverConnection any) error {
		backuper, ok := driverConnection.(interface {
			NewBackup(string) (*sqlite.Backup, error)
		})
		if !ok {
			return errors.New("sqlite driver does not support online backup")
		}
		backup, err := backuper.NewBackup(destination)
		if err != nil {
			return err
		}
		for {
			more, stepErr := backup.Step(128)
			if stepErr != nil {
				_ = backup.Finish()
				return stepErr
			}
			if !more {
				return backup.Finish()
			}
			select {
			case <-ctx.Done():
				_ = backup.Finish()
				return ctx.Err()
			default:
			}
		}
	})
}

func writeArchiveFile(path string, manifest BackupManifest, databasePath string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	writer := zip.NewWriter(file)
	manifestEntry, err := writer.Create("manifest.json")
	if err == nil {
		err = json.NewEncoder(manifestEntry).Encode(manifest)
	}
	var databaseEntry io.Writer
	if err == nil {
		databaseEntry, err = writer.Create("video-record.db")
	}
	if err == nil {
		database, openErr := os.Open(databasePath)
		if openErr != nil {
			err = openErr
		} else {
			_, err = io.Copy(databaseEntry, database)
			err = errors.Join(err, database.Close())
		}
	}
	err = errors.Join(err, writer.Close())
	err = errors.Join(err, file.Sync())
	err = errors.Join(err, file.Close())
	return err
}

func checksumFile(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	hash := sha256.New()
	bytes, copyErr := io.Copy(hash, file)
	closeErr := file.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		return "", 0, err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), bytes, nil
}

func databaseSchemaVersion(ctx context.Context, database *sql.DB) (int, error) {
	var version int
	err := database.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version)
	return version, err
}

func databaseRequiresEncryptionKey(ctx context.Context, database *sql.DB) (bool, error) {
	var exists int
	if err := database.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'external_accounts'
	`).Scan(&exists); err != nil {
		return false, err
	}
	if exists == 0 {
		return false, nil
	}
	var count int
	if err := database.QueryRowContext(ctx, "SELECT COUNT(*) FROM external_accounts").Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func filesystemAvailableBytes(path string) (uint64, error) {
	var stats unix.Statfs_t
	if err := unix.Statfs(path, &stats); err != nil {
		return 0, err
	}
	return stats.Bavail * uint64(stats.Bsize), nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = directory.Close() }()
	return directory.Sync()
}

func removeSQLiteFiles(path string) {
	_ = os.Remove(path)
	_ = os.Remove(path + "-wal")
	_ = os.Remove(path + "-shm")
}
