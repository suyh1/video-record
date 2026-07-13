package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"video-record/internal/auth"
	"video-record/internal/storage"
)

const maxRestoreArchiveBytes int64 = 4 << 30

type backupHandlers struct {
	manager     *storage.BackupManager
	idempotency *idempotencyMiddleware
}

func (handlers backupHandlers) list(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	artifacts, err := handlers.manager.List(r.Context())
	if err != nil {
		writeProblem(w, r, http.StatusInternalServerError, "Internal Server Error", "internal_error")
		return
	}
	writeJSON(w, http.StatusOK, artifacts)
}

func (handlers backupHandlers) create(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	artifact, err := handlers.manager.Create(r.Context())
	if err != nil {
		writeProblem(w, r, http.StatusInternalServerError, "Internal Server Error", "backup_failed")
		return
	}
	writeJSON(w, http.StatusCreated, artifact)
}

func (handlers backupHandlers) download(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	filename := chi.URLParam(r, "filename")
	file, info, err := handlers.manager.Open(filename)
	if err != nil {
		if errors.Is(err, storage.ErrInvalidBackup) || errors.Is(err, io.EOF) {
			writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_backup")
			return
		}
		writeProblem(w, r, http.StatusNotFound, "Not Found", "backup_not_found")
		return
	}
	defer func() { _ = file.Close() }()
	w.Header().Set("Content-Type", "application/vnd.video-record.backup")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, filename))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, filename, info.ModTime(), file)
}

func (handlers backupHandlers) restore(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	if _, ok := idempotencyKey(r); !ok {
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_idempotency_key")
		return
	}
	reader, err := r.MultipartReader()
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_backup")
		return
	}
	var staged *storage.StagedRestore
	for {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil || part.FormName() != "file" || part.FileName() == "" || staged != nil {
			if part != nil {
				_ = part.Close()
			}
			writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_backup")
			return
		}
		upload, stageErr := handlers.manager.StageRestore(r.Context(), part, maxRestoreArchiveBytes)
		_ = part.Close()
		if errors.Is(stageErr, storage.ErrBackupTooLarge) {
			writeProblem(w, r, http.StatusRequestEntityTooLarge, "Content Too Large", "backup_too_large")
			return
		}
		if stageErr != nil {
			writeProblem(w, r, http.StatusInternalServerError, "Internal Server Error", "restore_failed")
			return
		}
		staged = &upload
		defer staged.Remove()
	}
	if staged == nil || staged.Bytes == 0 {
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_backup")
		return
	}
	handlers.idempotency.handleHashBestEffort(w, r, staged.SHA256, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlers.applyRestore(w, r, *staged)
	}))
}

func (handlers backupHandlers) applyRestore(w http.ResponseWriter, r *http.Request, staged storage.StagedRestore) {
	identity, _ := IdentityFromContext(r.Context())
	result, err := handlers.manager.RestoreStaged(r.Context(), staged, func(ctx context.Context, db *storage.DB) error {
		return writeRestoreAudit(ctx, db, identity.User.ID)
	})
	if err != nil {
		writeRestoreError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func writeRestoreAudit(ctx context.Context, db *storage.DB, actorID string) error {
	var actor any
	var actorExists int
	if err := db.Reader().QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE id = ?", actorID).Scan(&actorExists); err != nil {
		return err
	}
	if actorExists > 0 {
		actor = actorID
	}
	metadata, err := json.Marshal(map[string]string{"result": "committed"})
	if err != nil {
		return err
	}
	_, err = db.Writer().ExecContext(ctx, `
		INSERT INTO audit_events (
			id, actor_user_id, action, target_type, target_id, metadata_json, created_at
		) VALUES (?, ?, 'backup.restore', 'database', 'video-record', ?, ?)
	`, uuid.NewString(), actor, string(metadata), time.Now().UTC().UnixMilli())
	return err
}

func requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return false
	}
	if identity.User.Role != auth.RoleAdmin {
		writeProblem(w, r, http.StatusForbidden, "Forbidden", "forbidden")
		return false
	}
	return true
}

func writeRestoreError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, storage.ErrBackupChecksum):
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "backup_checksum_mismatch")
	case errors.Is(err, storage.ErrIncompatibleBackup):
		writeProblem(w, r, http.StatusConflict, "Conflict", "incompatible_backup")
	case errors.Is(err, storage.ErrInsufficientSpace):
		writeProblem(w, r, http.StatusInsufficientStorage, "Insufficient Storage", "insufficient_storage")
	case errors.Is(err, storage.ErrInvalidBackup):
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_backup")
	case errors.Is(err, storage.ErrMaintenance):
		writeProblem(w, r, http.StatusServiceUnavailable, "Service Unavailable", "maintenance_mode")
	default:
		writeProblem(w, r, http.StatusInternalServerError, "Internal Server Error", "restore_failed")
	}
}
