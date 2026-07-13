package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"video-record/internal/storage"
)

var errUserNotFound = errors.New("user not found")

type Repository interface {
	IsInitialized(context.Context) (bool, error)
	CreateInitialAdmin(context.Context, User, string) error
	FindUserByUsername(context.Context, string) (User, string, error)
	LoginAttempt(context.Context, string) (loginAttempt, error)
	SaveLoginAttempt(context.Context, string, loginAttempt) error
	ClearLoginAttempt(context.Context, string) error
	RotateSession(context.Context, sessionRecord, time.Time) error
	FindSession(context.Context, []byte) (Identity, error)
	TouchSession(context.Context, string, time.Time, time.Time) error
	RevokeSession(context.Context, []byte, time.Time) error
}

type SQLiteRepository struct {
	db *storage.DB
}

type loginAttempt struct {
	Failures      int
	WindowStarted time.Time
	BlockedUntil  time.Time
}

type sessionRecord struct {
	ID            string
	UserID        string
	TokenHash     []byte
	CSRFTokenHash []byte
	ExpiresAt     time.Time
	LastSeenAt    time.Time
}

func NewRepository(db *storage.DB) *SQLiteRepository {
	return &SQLiteRepository{db: db}
}

func (repository *SQLiteRepository) IsInitialized(ctx context.Context) (bool, error) {
	var users int
	if err := repository.db.Reader().QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&users); err != nil {
		return false, err
	}
	return users > 0, nil
}

func (repository *SQLiteRepository) CreateInitialAdmin(ctx context.Context, user User, passwordHash string) error {
	tx, err := repository.db.Writer().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var users int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&users); err != nil {
		return err
	}
	if users > 0 {
		return ErrInitializationClosed
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO users (id, username, password_hash, role, active, created_at)
		VALUES (?, ?, ?, ?, 1, ?)
	`, user.ID, user.Username, passwordHash, user.Role, user.CreatedAt.UnixMilli()); err != nil {
		return err
	}
	return tx.Commit()
}

func (repository *SQLiteRepository) FindUserByUsername(ctx context.Context, username string) (User, string, error) {
	var user User
	var passwordHash string
	var active int
	var createdAt int64
	err := repository.db.Reader().QueryRowContext(ctx, `
		SELECT id, username, password_hash, role, active, created_at
		FROM users
		WHERE username = ? COLLATE NOCASE
	`, username).Scan(&user.ID, &user.Username, &passwordHash, &user.Role, &active, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, "", errUserNotFound
	}
	if err != nil {
		return User{}, "", err
	}
	user.Active = active == 1
	user.CreatedAt = time.UnixMilli(createdAt).UTC()
	return user, passwordHash, nil
}

func (repository *SQLiteRepository) LoginAttempt(ctx context.Context, key string) (loginAttempt, error) {
	var attempt loginAttempt
	var windowStarted, blockedUntil int64
	err := repository.db.Reader().QueryRowContext(ctx, `
		SELECT failures, window_started, blocked_until
		FROM login_attempts
		WHERE bucket_key = ?
	`, key).Scan(&attempt.Failures, &windowStarted, &blockedUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return loginAttempt{}, nil
	}
	if err != nil {
		return loginAttempt{}, err
	}
	attempt.WindowStarted = time.UnixMilli(windowStarted).UTC()
	if blockedUntil > 0 {
		attempt.BlockedUntil = time.UnixMilli(blockedUntil).UTC()
	}
	return attempt, nil
}

func (repository *SQLiteRepository) SaveLoginAttempt(ctx context.Context, key string, attempt loginAttempt) error {
	var blockedUntil int64
	if !attempt.BlockedUntil.IsZero() {
		blockedUntil = attempt.BlockedUntil.UnixMilli()
	}
	_, err := repository.db.Writer().ExecContext(ctx, `
		INSERT INTO login_attempts (bucket_key, failures, window_started, blocked_until)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(bucket_key) DO UPDATE SET
			failures = excluded.failures,
			window_started = excluded.window_started,
			blocked_until = excluded.blocked_until
	`, key, attempt.Failures, attempt.WindowStarted.UnixMilli(), blockedUntil)
	return err
}

func (repository *SQLiteRepository) ClearLoginAttempt(ctx context.Context, key string) error {
	_, err := repository.db.Writer().ExecContext(ctx, "DELETE FROM login_attempts WHERE bucket_key = ?", key)
	return err
}

func (repository *SQLiteRepository) RotateSession(ctx context.Context, session sessionRecord, now time.Time) error {
	tx, err := repository.db.Writer().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
		UPDATE sessions SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL
	`, now.UnixMilli(), session.UserID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO sessions (
			id, user_id, token_hash, csrf_token_hash, expires_at, last_seen_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`,
		session.ID,
		session.UserID,
		session.TokenHash,
		session.CSRFTokenHash,
		session.ExpiresAt.UnixMilli(),
		session.LastSeenAt.UnixMilli(),
		now.UnixMilli(),
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (repository *SQLiteRepository) FindSession(ctx context.Context, tokenHash []byte) (Identity, error) {
	var identity Identity
	var active int
	var createdAt, expiresAt, lastSeenAt int64
	err := repository.db.Reader().QueryRowContext(ctx, `
		SELECT
			s.id, s.csrf_token_hash, s.expires_at, s.last_seen_at,
			u.id, u.username, u.role, u.active, u.created_at
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ? AND s.revoked_at IS NULL AND u.active = 1
	`, tokenHash).Scan(
		&identity.SessionID,
		&identity.CSRFTokenHash,
		&expiresAt,
		&lastSeenAt,
		&identity.User.ID,
		&identity.User.Username,
		&identity.User.Role,
		&active,
		&createdAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Identity{}, ErrInvalidSession
	}
	if err != nil {
		return Identity{}, err
	}
	identity.User.Active = active == 1
	identity.User.CreatedAt = time.UnixMilli(createdAt).UTC()
	identity.ExpiresAt = time.UnixMilli(expiresAt).UTC()
	identity.LastSeenAt = time.UnixMilli(lastSeenAt).UTC()
	return identity, nil
}

func (repository *SQLiteRepository) TouchSession(ctx context.Context, id string, seenAt, cutoff time.Time) error {
	_, err := repository.db.Writer().ExecContext(ctx, `
		UPDATE sessions
		SET last_seen_at = ?
		WHERE id = ? AND revoked_at IS NULL AND last_seen_at <= ?
	`, seenAt.UnixMilli(), id, cutoff.UnixMilli())
	return err
}

func (repository *SQLiteRepository) RevokeSession(ctx context.Context, tokenHash []byte, now time.Time) error {
	_, err := repository.db.Writer().ExecContext(ctx, `
		UPDATE sessions SET revoked_at = ? WHERE token_hash = ? AND revoked_at IS NULL
	`, now.UnixMilli(), tokenHash)
	return err
}
