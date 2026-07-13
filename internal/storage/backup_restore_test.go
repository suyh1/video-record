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
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCleanupIncompleteBackupArtifactsAfterProcessTermination(t *testing.T) {
	ctx := context.Background()
	backupsDir := filepath.Join(t.TempDir(), "backups")
	require.NoError(t, os.MkdirAll(backupsDir, 0o700))
	stale := []string{
		".snapshot-crashed.db",
		".snapshot-crashed.db-wal",
		".snapshot-crashed.db-shm",
		"video-record-123-crashed.vrbackup.partial",
		".restore-upload-crashed.partial",
	}
	for _, name := range stale {
		require.NoError(t, os.WriteFile(filepath.Join(backupsDir, name), []byte("synthetic"), 0o600))
	}
	keep := []string{"notes.txt", "video-record-valid.vrbackup"}
	for _, name := range keep {
		require.NoError(t, os.WriteFile(filepath.Join(backupsDir, name), []byte("keep"), 0o600))
	}
	manager := NewBackupManager(nil, BackupOptions{BackupsDir: backupsDir})

	require.NoError(t, manager.CleanupIncomplete(ctx))

	for _, name := range stale {
		_, err := os.Stat(filepath.Join(backupsDir, name))
		require.ErrorIs(t, err, os.ErrNotExist, name)
	}
	for _, name := range keep {
		_, err := os.Stat(filepath.Join(backupsDir, name))
		require.NoError(t, err, name)
	}
	require.NoError(t, manager.CleanupIncomplete(ctx), "cleanup is idempotent")
}

func TestOnlineBackupProducesConsistentChecksummedSnapshotDuringWrites(t *testing.T) {
	ctx := context.Background()
	db := openBackupTestDB(t)
	_, err := db.Writer().ExecContext(ctx, `
		CREATE TABLE backup_probe (id INTEGER PRIMARY KEY, batch_id INTEGER NOT NULL)
	`)
	require.NoError(t, err)
	_, err = db.Writer().ExecContext(ctx, "INSERT INTO backup_probe (batch_id) VALUES (0), (0)")
	require.NoError(t, err)
	manager := NewBackupManager(db, BackupOptions{BackupsDir: filepath.Join(t.TempDir(), "backups")})
	stop := make(chan struct{})
	done := make(chan struct{})
	var batches atomic.Int64
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
			}
			batch := batches.Add(1)
			tx, beginErr := db.Writer().BeginTx(ctx, nil)
			if beginErr != nil {
				return
			}
			_, firstErr := tx.ExecContext(ctx, "INSERT INTO backup_probe (batch_id) VALUES (?)", batch)
			_, secondErr := tx.ExecContext(ctx, "INSERT INTO backup_probe (batch_id) VALUES (?)", batch)
			if firstErr != nil || secondErr != nil || tx.Commit() != nil {
				_ = tx.Rollback()
				return
			}
		}
	}()
	artifact, err := manager.Create(ctx)
	close(stop)
	<-done
	require.NoError(t, err)
	require.NotEmpty(t, artifact.Filename)
	require.FileExists(t, artifact.Path)

	archiveData, err := os.ReadFile(artifact.Path)
	require.NoError(t, err)
	manifest, database := readBackupArchive(t, archiveData)
	require.Equal(t, backupFormatVersion, manifest.FormatVersion)
	require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(database)), manifest.DatabaseSHA256)
	require.Equal(t, int64(len(database)), manifest.DatabaseBytes)
	extractedPath := filepath.Join(t.TempDir(), "snapshot.db")
	require.NoError(t, os.WriteFile(extractedPath, database, 0o600))
	snapshot, err := Open(ctx, extractedPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, snapshot.Close()) })
	var count, inconsistent int
	require.NoError(t, snapshot.Reader().QueryRowContext(ctx, "SELECT COUNT(*) FROM backup_probe").Scan(&count))
	require.NoError(t, snapshot.Reader().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM (
			SELECT batch_id FROM backup_probe GROUP BY batch_id HAVING COUNT(*) <> 2
		)
	`).Scan(&inconsistent))
	require.Greater(t, count, 0)
	require.Equal(t, 0, inconsistent)
	var integrity string
	require.NoError(t, snapshot.Reader().QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity))
	require.Equal(t, "ok", integrity)
}

func TestRestoreValidatesArchiveSnapshotsCurrentDataAndRollsBackFailures(t *testing.T) {
	ctx := context.Background()
	db := openBackupTestDB(t)
	_, err := db.Writer().ExecContext(ctx, `
		CREATE TABLE restore_probe (id INTEGER PRIMARY KEY, value TEXT NOT NULL)
	`)
	require.NoError(t, err)
	_, err = db.Writer().ExecContext(ctx, "INSERT INTO restore_probe (id, value) VALUES (1, 'before')")
	require.NoError(t, err)
	backupsDir := filepath.Join(t.TempDir(), "backups")
	manager := NewBackupManager(db, BackupOptions{BackupsDir: backupsDir})
	artifact, err := manager.Create(ctx)
	require.NoError(t, err)
	archiveData, err := os.ReadFile(artifact.Path)
	require.NoError(t, err)
	_, err = db.Writer().ExecContext(ctx, "UPDATE restore_probe SET value = 'after' WHERE id = 1")
	require.NoError(t, err)

	require.True(t, db.BeginRequest())
	restoreDone := make(chan error, 1)
	go func() {
		_, restoreErr := manager.Restore(ctx, archiveData)
		restoreDone <- restoreErr
	}()
	select {
	case err := <-restoreDone:
		t.Fatalf("restore did not wait for an active request: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	db.EndRequest()
	require.NoError(t, <-restoreDone)
	var value string
	require.NoError(t, db.Reader().QueryRowContext(ctx, "SELECT value FROM restore_probe WHERE id = 1").Scan(&value))
	require.Equal(t, "before", value)
	artifacts, err := manager.List(ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(artifacts), 2)

	manifest, database := readBackupArchive(t, archiveData)
	tamperedDatabase := append([]byte(nil), database...)
	tamperedDatabase[len(tamperedDatabase)/2] ^= 0xff
	_, err = manager.Restore(ctx, writeBackupArchive(t, manifest, tamperedDatabase, "video-record.db"))
	require.ErrorIs(t, err, ErrBackupChecksum)
	manifest.SchemaVersion++
	manifest.DatabaseSHA256 = fmt.Sprintf("%x", sha256.Sum256(database))
	_, err = manager.Restore(ctx, writeBackupArchive(t, manifest, database, "video-record.db"))
	require.ErrorIs(t, err, ErrIncompatibleBackup)
	_, err = manager.Restore(ctx, writeBackupArchive(t, manifest, database, "../escape.db"))
	require.ErrorIs(t, err, ErrInvalidBackup)

	noSpace := NewBackupManager(db, BackupOptions{
		BackupsDir:     backupsDir,
		AvailableBytes: func(string) (uint64, error) { return 0, nil },
	})
	manifest.SchemaVersion--
	_, err = noSpace.Restore(ctx, writeBackupArchive(t, manifest, database, "video-record.db"))
	require.ErrorIs(t, err, ErrInsufficientSpace)

	_, err = db.Writer().ExecContext(ctx, "UPDATE restore_probe SET value = 'current' WHERE id = 1")
	require.NoError(t, err)
	forced := NewBackupManager(db, BackupOptions{
		BackupsDir:   backupsDir,
		AfterReplace: func() error { return errors.New("forced replacement failure") },
	})
	_, err = forced.Restore(ctx, archiveData)
	require.Error(t, err)
	require.NoError(t, db.Reader().QueryRowContext(ctx, "SELECT value FROM restore_probe WHERE id = 1").Scan(&value))
	require.Equal(t, "current", value)

	manifest, database = readBackupArchive(t, archiveData)
	manifest.RequiresEncryptionKey = true
	missingKeyArchive := writeBackupArchive(t, manifest, database, "video-record.db")
	result, err := manager.Restore(ctx, missingKeyArchive)
	require.NoError(t, err)
	require.Equal(t, []string{"integrations_locked"}, result.Warnings)
}

func TestBackupManagerReadsRegularArtifactsAndRejectsSymlinks(t *testing.T) {
	ctx := context.Background()
	db := openBackupTestDB(t)
	backupsDir := filepath.Join(t.TempDir(), "backups")
	manager := NewBackupManager(db, BackupOptions{BackupsDir: backupsDir})
	artifact, err := manager.Create(ctx)
	require.NoError(t, err)

	data, err := manager.Read(artifact.Filename)
	require.NoError(t, err)
	require.NotEmpty(t, data)
	_, err = manager.Read("missing.vrbackup")
	require.ErrorIs(t, err, os.ErrNotExist)
	listed, err := manager.List(ctx)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, artifact.Filename, listed[0].Filename)

	secret := filepath.Join(t.TempDir(), "outside.vrbackup")
	require.NoError(t, os.WriteFile(secret, []byte("outside backup directory"), 0o600))
	require.NoError(t, os.Symlink(secret, filepath.Join(backupsDir, "linked.vrbackup")))
	_, err = manager.Read("linked.vrbackup")
	require.ErrorIs(t, err, ErrInvalidBackup)
	listed, err = manager.List(ctx)
	require.NoError(t, err)
	require.Len(t, listed, 1)
}

func TestMaintenanceLifecycleAndReadyState(t *testing.T) {
	ctx := context.Background()
	db := openBackupTestDB(t)
	require.NoError(t, db.Ready(ctx))
	require.False(t, db.IsMaintenance())
	require.NoError(t, db.BeginMaintenance(ctx))
	require.True(t, db.IsMaintenance())
	require.False(t, db.BeginRequest())
	require.ErrorIs(t, db.BeginMaintenance(ctx), ErrMaintenance)
	db.EndMaintenance()
	require.False(t, db.IsMaintenance())
	require.True(t, db.BeginRequest())
	db.EndRequest()
	require.NoError(t, db.Close())
	require.Error(t, db.Ready(ctx))
}

func TestBackupManifestMarksEncryptedIntegrationAccounts(t *testing.T) {
	ctx := context.Background()
	db := openBackupTestDB(t)
	backupsDir := filepath.Join(t.TempDir(), "backups")
	manager := NewBackupManager(db, BackupOptions{BackupsDir: backupsDir})
	listed, err := manager.List(ctx)
	require.NoError(t, err)
	require.Empty(t, listed)

	artifact, err := manager.Create(ctx)
	require.NoError(t, err)
	require.False(t, artifact.Manifest.RequiresEncryptionKey)
	require.NoError(t, insertBackupIntegrationUser(ctx, db, "backup-user", "backup-owner"))
	require.NoError(t, insertBackupIntegrationAccount(ctx, db, "synthetic-account", "backup-user"))
	artifact, err = manager.Create(ctx)
	require.NoError(t, err)
	require.True(t, artifact.Manifest.RequiresEncryptionKey)
	expectedSchema, err := databaseSchemaVersion(ctx, db.Reader())
	require.NoError(t, err)
	require.Equal(t, expectedSchema, artifact.Manifest.SchemaVersion)
	_, err = manager.Read("../outside.vrbackup")
	require.ErrorIs(t, err, ErrInvalidBackup)
}

func TestBackupAndRestoreReportInvalidIOAndArchiveFailures(t *testing.T) {
	ctx := context.Background()
	db := openBackupTestDB(t)
	blockedParent := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(blockedParent, []byte("blocked"), 0o600))
	blocked := NewBackupManager(db, BackupOptions{BackupsDir: filepath.Join(blockedParent, "backups")})
	_, err := blocked.Create(ctx)
	require.Error(t, err)
	_, err = blocked.StageRestore(ctx, bytes.NewReader([]byte("archive")), 1024)
	require.Error(t, err)
	_, _, err = checksumFile(filepath.Join(t.TempDir(), "missing.db"))
	require.Error(t, err)

	backupsDir := filepath.Join(t.TempDir(), "backups")
	manager := NewBackupManager(db, BackupOptions{BackupsDir: backupsDir})
	artifact, err := manager.Create(ctx)
	require.NoError(t, err)
	archive, err := os.ReadFile(artifact.Path)
	require.NoError(t, err)

	spaceFailure := errors.New("space probe failed")
	spaceProbe := NewBackupManager(db, BackupOptions{
		BackupsDir: backupsDir,
		AvailableBytes: func(string) (uint64, error) {
			return 0, spaceFailure
		},
	})
	_, err = spaceProbe.Restore(ctx, archive)
	require.ErrorIs(t, err, spaceFailure)

	manifest, _ := readBackupArchive(t, archive)
	corruptDatabase := []byte("not a sqlite database")
	manifest.DatabaseBytes = int64(len(corruptDatabase))
	manifest.DatabaseSHA256 = fmt.Sprintf("%x", sha256.Sum256(corruptDatabase))
	_, err = manager.Restore(ctx, writeBackupArchive(t, manifest, corruptDatabase, "video-record.db"))
	require.ErrorIs(t, err, ErrInvalidBackup)

	_, _, err = parseBackupArchive([]byte("not a zip archive"))
	require.ErrorIs(t, err, ErrInvalidBackup)
	manifest.FormatVersion = backupFormatVersion + 1
	_, _, err = parseBackupArchive(writeBackupArchive(t, manifest, corruptDatabase, "video-record.db"))
	require.ErrorIs(t, err, ErrInvalidBackup)

	corruptPath := filepath.Join(backupsDir, "corrupt.vrbackup")
	require.NoError(t, os.WriteFile(corruptPath, []byte("corrupt"), 0o600))
	_, err = manager.List(ctx)
	require.Error(t, err)

	require.Error(t, writeArchiveFile(corruptPath, BackupManifest{}, corruptPath))
	_, err = filesystemAvailableBytes(filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)
	require.Error(t, syncDirectory(filepath.Join(t.TempDir(), "missing")))
}

func TestOpenRejectsInvalidDatabasePaths(t *testing.T) {
	ctx := context.Background()
	blockedParent := filepath.Join(t.TempDir(), "blocked")
	require.NoError(t, os.WriteFile(blockedParent, []byte("blocked"), 0o600))
	_, err := Open(ctx, filepath.Join(blockedParent, "video-record.db"))
	require.Error(t, err)

	directoryPath := filepath.Join(t.TempDir(), "database-directory")
	require.NoError(t, os.Mkdir(directoryPath, 0o700))
	_, err = Open(ctx, directoryPath)
	require.Error(t, err)
}

func TestMaintenanceWaitCancellationReopensRequestGate(t *testing.T) {
	db := openBackupTestDB(t)
	require.True(t, db.BeginRequest())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, db.BeginMaintenance(ctx), context.Canceled)
	require.False(t, db.IsMaintenance())
	db.EndRequest()
	require.True(t, db.BeginRequest())
	db.EndRequest()
}

func TestBackupArchiveRejectsDuplicateAndMalformedEntries(t *testing.T) {
	ctx := context.Background()
	db := openBackupTestDB(t)
	manager := NewBackupManager(db, BackupOptions{BackupsDir: filepath.Join(t.TempDir(), "backups")})
	manifest := BackupManifest{
		FormatVersion:  backupFormatVersion,
		SchemaVersion:  1,
		CreatedAt:      time.Now().UTC().Format(time.RFC3339Nano),
		DatabaseBytes:  1,
		DatabaseSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte{1})),
	}
	manifestJSON, err := json.Marshal(manifest)
	require.NoError(t, err)

	cases := []struct {
		name    string
		entries []rawBackupEntry
	}{
		{name: "missing database", entries: []rawBackupEntry{{name: "manifest.json", data: manifestJSON}}},
		{name: "duplicate manifest", entries: []rawBackupEntry{
			{name: "manifest.json", data: manifestJSON}, {name: "manifest.json", data: manifestJSON},
		}},
		{name: "malformed manifest", entries: []rawBackupEntry{
			{name: "manifest.json", data: []byte("{")}, {name: "video-record.db", data: []byte{1}},
		}},
		{name: "unknown entry", entries: []rawBackupEntry{
			{name: "manifest.json", data: manifestJSON}, {name: "unexpected.db", data: []byte{1}},
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			archive := writeRawBackupArchive(t, testCase.entries)
			_, _, err := parseBackupArchive(archive)
			require.ErrorIs(t, err, ErrInvalidBackup)
			staged, err := manager.StageRestore(ctx, bytes.NewReader(archive), int64(len(archive)))
			require.NoError(t, err)
			defer staged.Remove()
			_, err = manager.RestoreStaged(ctx, staged, nil)
			require.ErrorIs(t, err, ErrInvalidBackup)
		})
	}
}

func TestBackupManagerDefaultsAndCanceledOperations(t *testing.T) {
	ctx := context.Background()
	db := openBackupTestDB(t)
	manager := NewBackupManager(db, BackupOptions{})
	require.Equal(t, filepath.Join(filepath.Dir(db.Path()), "backups"), manager.backupsDir)

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	require.ErrorIs(t, manager.onlineBackup(canceled, filepath.Join(t.TempDir(), "snapshot.db")), context.Canceled)

	closed, err := sql.Open("sqlite", sqliteDSN(filepath.Join(t.TempDir(), "closed.db"), true))
	require.NoError(t, err)
	require.NoError(t, closed.Close())
	_, err = databaseRequiresEncryptionKey(ctx, closed)
	require.Error(t, err)
}

func TestRestoreKeepsCanonicalDatabaseUntilAtomicCommit(t *testing.T) {
	ctx := context.Background()
	db := openBackupTestDB(t)
	_, err := db.Writer().ExecContext(ctx, "CREATE TABLE atomic_probe (value TEXT NOT NULL)")
	require.NoError(t, err)
	_, err = db.Writer().ExecContext(ctx, "INSERT INTO atomic_probe (value) VALUES ('before')")
	require.NoError(t, err)
	backupsDir := filepath.Join(t.TempDir(), "backups")
	baseline := NewBackupManager(db, BackupOptions{BackupsDir: backupsDir})
	artifact, err := baseline.Create(ctx)
	require.NoError(t, err)
	archive, err := baseline.Read(artifact.Filename)
	require.NoError(t, err)
	_, err = db.Writer().ExecContext(ctx, "UPDATE atomic_probe SET value = 'current'")
	require.NoError(t, err)
	stopBeforeCommit := errors.New("stop before atomic commit")
	manager := NewBackupManager(db, BackupOptions{
		BackupsDir: backupsDir,
		BeforeReplace: func(databasePath string) error {
			require.FileExists(t, databasePath)
			return stopBeforeCommit
		},
	})
	_, err = manager.Restore(ctx, archive)
	require.ErrorIs(t, err, stopBeforeCommit)
	var value string
	require.NoError(t, db.Reader().QueryRowContext(ctx, "SELECT value FROM atomic_probe").Scan(&value))
	require.Equal(t, "current", value)
}

func TestRestoreUsesIndependentCriticalContextAndRollsBackCommitFailures(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	db := openBackupTestDB(t)
	_, err := db.Writer().ExecContext(ctx, "CREATE TABLE critical_probe (value TEXT NOT NULL)")
	require.NoError(t, err)
	_, err = db.Writer().ExecContext(ctx, "INSERT INTO critical_probe (value) VALUES ('before')")
	require.NoError(t, err)
	backupsDir := filepath.Join(t.TempDir(), "backups")
	manager := NewBackupManager(db, BackupOptions{
		BackupsDir: backupsDir,
		AfterReplace: func() error {
			cancel()
			return nil
		},
	})
	artifact, err := manager.Create(ctx)
	require.NoError(t, err)
	archive, err := manager.Read(artifact.Filename)
	require.NoError(t, err)
	_, err = db.Writer().ExecContext(ctx, "UPDATE critical_probe SET value = 'current'")
	require.NoError(t, err)
	_, err = manager.Restore(ctx, archive)
	require.NoError(t, err)
	var value string
	require.NoError(t, db.Reader().QueryRowContext(context.Background(), "SELECT value FROM critical_probe").Scan(&value))
	require.Equal(t, "before", value)

	commitFailure := errors.New("audit commit failed")
	manager = NewBackupManager(db, BackupOptions{BackupsDir: backupsDir})
	_, err = db.Writer().ExecContext(context.Background(), "UPDATE critical_probe SET value = 'current'")
	require.NoError(t, err)
	_, err = manager.RestoreWithCommit(context.Background(), archive, func(context.Context, *DB) error {
		return commitFailure
	})
	require.ErrorIs(t, err, commitFailure)
	require.NoError(t, db.Reader().QueryRowContext(context.Background(), "SELECT value FROM critical_probe").Scan(&value))
	require.Equal(t, "current", value)
}

func TestRestoreSerializesConcurrentRequests(t *testing.T) {
	ctx := context.Background()
	db := openBackupTestDB(t)
	backupsDir := filepath.Join(t.TempDir(), "backups")
	baseline := NewBackupManager(db, BackupOptions{BackupsDir: backupsDir})
	artifact, err := baseline.Create(ctx)
	require.NoError(t, err)
	archive, err := baseline.Read(artifact.Filename)
	require.NoError(t, err)
	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	var replacements atomic.Int64
	manager := NewBackupManager(db, BackupOptions{
		BackupsDir: backupsDir,
		AfterReplace: func() error {
			if replacements.Add(1) == 1 {
				close(firstEntered)
				<-releaseFirst
			}
			return nil
		},
	})
	results := make(chan error, 2)
	go func() { _, restoreErr := manager.Restore(ctx, archive); results <- restoreErr }()
	<-firstEntered
	go func() { _, restoreErr := manager.Restore(ctx, archive); results <- restoreErr }()
	select {
	case err := <-results:
		t.Fatalf("second restore was not serialized: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(releaseFirst)
	require.NoError(t, <-results)
	require.NoError(t, <-results)
	require.Equal(t, int64(2), replacements.Load())
}

func TestRestoreRejectsManifestDatabaseSchemaMismatch(t *testing.T) {
	ctx := context.Background()
	db := openBackupTestDB(t)
	manager := NewBackupManager(db, BackupOptions{BackupsDir: filepath.Join(t.TempDir(), "backups")})
	artifact, err := manager.Create(ctx)
	require.NoError(t, err)
	archive, err := manager.Read(artifact.Filename)
	require.NoError(t, err)
	manifest, database := readBackupArchive(t, archive)
	manifest.SchemaVersion--
	_, err = manager.Restore(ctx, writeBackupArchive(t, manifest, database, "video-record.db"))
	require.ErrorIs(t, err, ErrInvalidBackup)
}

func TestStagedRestoreStreamsArchivesAndEnforcesCompressedLimit(t *testing.T) {
	ctx := context.Background()
	db := openBackupTestDB(t)
	_, err := db.Writer().ExecContext(ctx, "CREATE TABLE staged_probe (value TEXT NOT NULL)")
	require.NoError(t, err)
	_, err = db.Writer().ExecContext(ctx, "INSERT INTO staged_probe (value) VALUES ('before')")
	require.NoError(t, err)
	manager := NewBackupManager(db, BackupOptions{BackupsDir: filepath.Join(t.TempDir(), "backups")})
	artifact, err := manager.Create(ctx)
	require.NoError(t, err)
	archive, err := manager.Read(artifact.Filename)
	require.NoError(t, err)
	_, err = db.Writer().ExecContext(ctx, "UPDATE staged_probe SET value = 'after'")
	require.NoError(t, err)

	_, err = manager.StageRestore(ctx, bytes.NewReader(archive), int64(len(archive)-1))
	require.ErrorIs(t, err, ErrBackupTooLarge)
	var value string
	require.NoError(t, db.Reader().QueryRowContext(ctx, "SELECT value FROM staged_probe").Scan(&value))
	require.Equal(t, "after", value)

	staged, err := manager.StageRestore(ctx, bytes.NewReader(archive), int64(len(archive)))
	require.NoError(t, err)
	t.Cleanup(func() { staged.Remove() })
	require.NotEmpty(t, staged.SHA256)
	_, err = manager.RestoreStaged(ctx, staged, nil)
	require.NoError(t, err)
	require.NoError(t, db.Reader().QueryRowContext(ctx, "SELECT value FROM staged_probe").Scan(&value))
	require.Equal(t, "before", value)
}

func TestStagedRestoreRejectsUnsafePathsAndCanceledLocks(t *testing.T) {
	ctx := context.Background()
	db := openBackupTestDB(t)
	manager := NewBackupManager(db, BackupOptions{BackupsDir: filepath.Join(t.TempDir(), "backups")})
	_, err := manager.StageRestore(ctx, bytes.NewReader([]byte("archive")), 0)
	require.ErrorIs(t, err, ErrBackupTooLarge)
	readFailure := errors.New("upload read failed")
	_, err = manager.StageRestore(ctx, errorReader{err: readFailure}, 1024)
	require.ErrorIs(t, err, readFailure)
	_, err = manager.RestoreStaged(ctx, StagedRestore{path: filepath.Join(t.TempDir(), ".restore-upload-forged.partial")}, nil)
	require.ErrorIs(t, err, ErrInvalidBackup)
	_, err = manager.RestoreStaged(ctx, StagedRestore{path: filepath.Join(manager.backupsDir, ".restore-upload-missing.partial")}, nil)
	require.ErrorIs(t, err, ErrInvalidBackup)
	invalidArchive, err := manager.StageRestore(ctx, bytes.NewReader([]byte("not a zip archive")), 1024)
	require.NoError(t, err)
	defer invalidArchive.Remove()
	_, err = manager.RestoreStaged(ctx, invalidArchive, nil)
	require.ErrorIs(t, err, ErrInvalidBackup)

	artifact, err := manager.Create(ctx)
	require.NoError(t, err)
	archive, err := manager.Read(artifact.Filename)
	require.NoError(t, err)
	staged, err := manager.StageRestore(ctx, bytes.NewReader(archive), int64(len(archive)))
	require.NoError(t, err)
	defer staged.Remove()
	require.NoError(t, db.gate.beginRestore(ctx))
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = manager.RestoreStaged(canceled, staged, nil)
	require.ErrorIs(t, err, context.Canceled)
	db.gate.endRestore()
}

func TestRestoreRejectsChangedMigrationChecksumInsideDatabase(t *testing.T) {
	ctx := context.Background()
	db := openBackupTestDB(t)
	manager := NewBackupManager(db, BackupOptions{BackupsDir: filepath.Join(t.TempDir(), "backups")})
	artifact, err := manager.Create(ctx)
	require.NoError(t, err)
	archive, err := manager.Read(artifact.Filename)
	require.NoError(t, err)
	manifest, database := readBackupArchive(t, archive)
	databasePath := filepath.Join(t.TempDir(), "tampered.db")
	require.NoError(t, os.WriteFile(databasePath, database, 0o600))
	tampered, err := Open(ctx, databasePath)
	require.NoError(t, err)
	_, err = tampered.Writer().ExecContext(ctx, `
		UPDATE schema_migrations SET checksum = 'tampered' WHERE version = ?
	`, manifest.SchemaVersion)
	require.NoError(t, err)
	require.NoError(t, tampered.Close())
	database, err = os.ReadFile(databasePath)
	require.NoError(t, err)
	manifest.DatabaseBytes = int64(len(database))
	manifest.DatabaseSHA256 = fmt.Sprintf("%x", sha256.Sum256(database))
	_, err = manager.Restore(ctx, writeBackupArchive(t, manifest, database, "video-record.db"))
	require.ErrorIs(t, err, ErrIncompatibleBackup)
}

type errorReader struct {
	err error
}

func (reader errorReader) Read([]byte) (int, error) {
	return 0, reader.err
}

func TestRestoreMigratesPreviousSchemaBackup(t *testing.T) {
	ctx := context.Background()
	migrationFS, err := fs.Sub(embeddedMigrations, "migrations")
	require.NoError(t, err)
	migrations, err := loadMigrations(migrationFS)
	require.NoError(t, err)
	require.Greater(t, len(migrations), 1)
	previous := fstest.MapFS{}
	for _, item := range migrations[:len(migrations)-1] {
		previous[item.name] = &fstest.MapFile{Data: []byte(item.sql), Mode: 0o600}
	}
	oldDB, err := Open(ctx, filepath.Join(t.TempDir(), "old", "video-record.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, oldDB.Close()) })
	require.NoError(t, migrate(ctx, oldDB.Writer(), previous))
	oldManager := NewBackupManager(oldDB, BackupOptions{BackupsDir: filepath.Join(t.TempDir(), "old-backups")})
	artifact, err := oldManager.Create(ctx)
	require.NoError(t, err)
	archive, err := oldManager.Read(artifact.Filename)
	require.NoError(t, err)
	require.Equal(t, migrations[len(migrations)-2].version, artifact.Manifest.SchemaVersion)

	currentDB := openBackupTestDB(t)
	currentManager := NewBackupManager(currentDB, BackupOptions{BackupsDir: filepath.Join(t.TempDir(), "current-backups")})
	_, err = currentManager.Restore(ctx, archive)
	require.NoError(t, err)
	version, err := databaseSchemaVersion(ctx, currentDB.Reader())
	require.NoError(t, err)
	require.Equal(t, migrations[len(migrations)-1].version, version)
}

func TestRestoreRecoveryUsesFreshContextAfterCriticalTimeout(t *testing.T) {
	ctx := context.Background()
	db := openBackupTestDB(t)
	_, err := db.Writer().ExecContext(ctx, "CREATE TABLE timeout_probe (value TEXT NOT NULL)")
	require.NoError(t, err)
	_, err = db.Writer().ExecContext(ctx, "INSERT INTO timeout_probe (value) VALUES ('before')")
	require.NoError(t, err)
	backupsDir := filepath.Join(t.TempDir(), "backups")
	baseline := NewBackupManager(db, BackupOptions{BackupsDir: backupsDir})
	artifact, err := baseline.Create(ctx)
	require.NoError(t, err)
	archive, err := baseline.Read(artifact.Filename)
	require.NoError(t, err)
	_, err = db.Writer().ExecContext(ctx, "UPDATE timeout_probe SET value = 'current'")
	require.NoError(t, err)
	manager := NewBackupManager(db, BackupOptions{
		BackupsDir:      backupsDir,
		CriticalTimeout: time.Nanosecond,
		AfterReplace: func() error {
			time.Sleep(time.Millisecond)
			return nil
		},
	})
	_, err = manager.Restore(ctx, archive)
	require.Error(t, err)
	var value string
	require.NoError(t, db.Reader().QueryRowContext(ctx, "SELECT value FROM timeout_probe").Scan(&value))
	require.Equal(t, "current", value)
}

func TestRestoreSpaceCheckIncludesCurrentDatabaseBackupPeak(t *testing.T) {
	ctx := context.Background()
	db := openBackupTestDB(t)
	manager := NewBackupManager(db, BackupOptions{BackupsDir: filepath.Join(t.TempDir(), "backups")})
	artifact, err := manager.Create(ctx)
	require.NoError(t, err)
	archive, err := manager.Read(artifact.Filename)
	require.NoError(t, err)
	_, err = db.Writer().ExecContext(ctx, "CREATE TABLE large_current_database (payload BLOB NOT NULL)")
	require.NoError(t, err)
	_, err = db.Writer().ExecContext(ctx, "INSERT INTO large_current_database (payload) VALUES (randomblob(4194304))")
	require.NoError(t, err)
	available := uint64(artifact.Manifest.DatabaseBytes*2 + 1)
	limited := NewBackupManager(db, BackupOptions{
		BackupsDir: manager.backupsDir,
		AvailableBytes: func(string) (uint64, error) {
			return available, nil
		},
	})
	_, err = limited.Restore(ctx, archive)
	require.ErrorIs(t, err, ErrInsufficientSpace)
}

func TestBackupManifestReadsEncryptionRequirementFromSnapshot(t *testing.T) {
	ctx := context.Background()
	db := openBackupTestDB(t)
	require.NoError(t, insertBackupIntegrationUser(ctx, db, "snapshot-user", "snapshot-owner"))
	manager := NewBackupManager(db, BackupOptions{
		BackupsDir: filepath.Join(t.TempDir(), "backups"),
		AfterSnapshot: func() error {
			return insertBackupIntegrationAccount(ctx, db, "created-after-snapshot", "snapshot-user")
		},
	})
	artifact, err := manager.Create(ctx)
	require.NoError(t, err)
	require.False(t, artifact.Manifest.RequiresEncryptionKey)
}

func insertBackupIntegrationUser(ctx context.Context, db *DB, id, username string) error {
	_, err := db.Writer().ExecContext(ctx, `
		INSERT INTO users (id, username, password_hash, role, active, created_at)
		VALUES (?, ?, 'synthetic-hash', 'member', 1, 0)
	`, id, username)
	return err
}

func insertBackupIntegrationAccount(ctx context.Context, db *DB, id, userID string) error {
	_, err := db.Writer().ExecContext(ctx, `
		INSERT INTO external_accounts (
			id, user_id, provider, name, base_url, credential_ciphertext,
			credential_nonce, credential_version, credential_fingerprint,
			enabled, created_at, updated_at
		) VALUES (?, ?, 'jellyfin', 'Synthetic', 'https://media.example.test',
		          x'01', x'02', 1, 'fingerprint', 1, 0, 0)
	`, id, userID)
	return err
}

func TestRestoreSpaceCheckSeparatesFilesystemsAndReportsProbeFailures(t *testing.T) {
	ctx := context.Background()
	db := openBackupTestDB(t)
	backupsDir := filepath.Join(t.TempDir(), "backups")
	require.NoError(t, os.MkdirAll(backupsDir, 0o700))
	databaseDir := filepath.Dir(db.Path())
	probeFailure := errors.New("backup volume probe failed")

	manager := NewBackupManager(db, BackupOptions{
		BackupsDir: backupsDir,
		SameFilesystem: func(string, string) (bool, error) {
			return false, nil
		},
		AvailableBytes: func(path string) (uint64, error) {
			if path == databaseDir {
				return 0, nil
			}
			return ^uint64(0), nil
		},
	})
	require.ErrorIs(t, manager.checkRestoreSpace(ctx, 1), ErrInsufficientSpace)

	manager = NewBackupManager(db, BackupOptions{
		BackupsDir: backupsDir,
		SameFilesystem: func(string, string) (bool, error) {
			return false, nil
		},
		AvailableBytes: func(path string) (uint64, error) {
			if path == databaseDir {
				return ^uint64(0), nil
			}
			return 0, probeFailure
		},
	})
	require.ErrorIs(t, manager.checkRestoreSpace(ctx, 1), probeFailure)

	manager = NewBackupManager(db, BackupOptions{
		BackupsDir: backupsDir,
		SameFilesystem: func(string, string) (bool, error) {
			return false, nil
		},
		AvailableBytes: func(string) (uint64, error) {
			return ^uint64(0), nil
		},
	})
	require.NoError(t, manager.checkRestoreSpace(ctx, 1))

	identityFailure := errors.New("filesystem identity failed")
	manager = NewBackupManager(db, BackupOptions{
		BackupsDir: backupsDir,
		SameFilesystem: func(string, string) (bool, error) {
			return false, identityFailure
		},
	})
	require.ErrorIs(t, manager.checkRestoreSpace(ctx, 1), identityFailure)
	_, err := sameFilesystem(filepath.Join(t.TempDir(), "missing"), backupsDir)
	require.Error(t, err)
	_, err = sameFilesystem(databaseDir, filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)
}

type rawBackupEntry struct {
	name string
	data []byte
}

func writeRawBackupArchive(t *testing.T, entries []rawBackupEntry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, entry := range entries {
		part, err := writer.Create(entry.name)
		require.NoError(t, err)
		_, err = part.Write(entry.data)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	return buffer.Bytes()
}

func openBackupTestDB(t *testing.T) *DB {
	t.Helper()
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	require.NoError(t, Migrate(ctx, db))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db
}

func readBackupArchive(t *testing.T, data []byte) (BackupManifest, []byte) {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	require.Len(t, reader.File, 2)
	var manifest BackupManifest
	var database []byte
	for _, file := range reader.File {
		contents := readZipFile(t, file)
		switch file.Name {
		case "manifest.json":
			require.NoError(t, json.Unmarshal(contents, &manifest))
		case "video-record.db":
			database = contents
		default:
			t.Fatalf("unexpected backup entry %q", file.Name)
		}
	}
	require.NotEmpty(t, database)
	return manifest, database
}

func readZipFile(t *testing.T, file *zip.File) []byte {
	t.Helper()
	reader, err := file.Open()
	require.NoError(t, err)
	defer func() { require.NoError(t, reader.Close()) }()
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	return data
}

func writeBackupArchive(t *testing.T, manifest BackupManifest, database []byte, databaseName string) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	manifestEntry, err := writer.Create("manifest.json")
	require.NoError(t, err)
	require.NoError(t, json.NewEncoder(manifestEntry).Encode(manifest))
	databaseEntry, err := writer.Create(databaseName)
	require.NoError(t, err)
	_, err = databaseEntry.Write(database)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return output.Bytes()
}
