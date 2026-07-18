package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	defaultLoginFailureLimit  = 5
	defaultLoginWindow        = 15 * time.Minute
	defaultLoginBlockDuration = 15 * time.Minute
	defaultLastSeenInterval   = 5 * time.Minute
	defaultSessionTTL         = 30 * 24 * time.Hour
)

var (
	ErrInitializationClosed = errors.New("initialization is closed")
	ErrInvalidCredentials   = errors.New("invalid credentials")
	ErrInvalidInput         = errors.New("invalid input")
	ErrInvalidSession       = errors.New("invalid session")
	ErrRateLimited          = errors.New("login rate limited")
)

type ServiceOptions struct {
	Now func() time.Time
}

type Service struct {
	repository Repository
	now        func() time.Time
	dummyHash  string
}

func NewService(repository Repository, options ServiceOptions) *Service {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	dummyHash, _ := HashPassword("invalid-login-placeholder")
	return &Service{repository: repository, now: now, dummyHash: dummyHash}
}

func (service *Service) IsInitialized(ctx context.Context) (bool, error) {
	return service.repository.IsInitialized(ctx)
}

func (service *Service) Initialize(ctx context.Context, username, password string) (User, error) {
	initialized, err := service.repository.IsInitialized(ctx)
	if err != nil {
		return User{}, err
	}
	if initialized {
		return User{}, ErrInitializationClosed
	}
	username = strings.TrimSpace(username)
	if username == "" || len(password) < 12 {
		return User{}, ErrInvalidInput
	}
	passwordHash, err := HashPassword(password)
	if err != nil {
		return User{}, err
	}
	user := User{
		ID:        uuid.NewString(),
		Username:  username,
		Role:      RoleAdmin,
		Active:    true,
		CreatedAt: service.now().UTC(),
	}
	if err := service.repository.CreateInitialAdmin(ctx, user, passwordHash); err != nil {
		return User{}, err
	}
	return user, nil
}

func (service *Service) ChangePassword(ctx context.Context, userID, currentPassword, newPassword string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" || currentPassword == "" || len(newPassword) < 12 {
		return ErrInvalidInput
	}
	passwordHash, err := service.repository.FindUserPasswordHash(ctx, userID)
	if errors.Is(err, errUserNotFound) {
		return ErrInvalidCredentials
	}
	if err != nil {
		return err
	}
	matches, verifyErr := VerifyPassword(passwordHash, currentPassword)
	if verifyErr != nil || !matches {
		return ErrInvalidCredentials
	}
	nextHash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	return service.repository.UpdateUserPasswordHash(ctx, userID, nextHash)
}

func (service *Service) Login(ctx context.Context, username, password, clientBucket string) (Session, error) {
	now := service.now().UTC()
	bucketKey := loginBucketKey(username, clientBucket)
	attempt, err := service.repository.LoginAttempt(ctx, bucketKey)
	if err != nil {
		return Session{}, err
	}
	if attempt.BlockedUntil.After(now) {
		return Session{}, ErrRateLimited
	}
	if !attempt.BlockedUntil.IsZero() || (!attempt.WindowStarted.IsZero() && now.Sub(attempt.WindowStarted) >= defaultLoginWindow) {
		attempt = loginAttempt{}
		if err := service.repository.ClearLoginAttempt(ctx, bucketKey); err != nil {
			return Session{}, err
		}
	}

	user, passwordHash, findErr := service.repository.FindUserByUsername(ctx, strings.TrimSpace(username))
	if errors.Is(findErr, errUserNotFound) {
		passwordHash = service.dummyHash
	} else if findErr != nil {
		return Session{}, findErr
	}
	matches, verifyErr := VerifyPassword(passwordHash, password)
	if verifyErr != nil || findErr != nil || !matches || !user.Active {
		if err := service.recordLoginFailure(ctx, bucketKey, attempt, now); err != nil {
			return Session{}, err
		}
		return Session{}, ErrInvalidCredentials
	}
	if err := service.repository.ClearLoginAttempt(ctx, bucketKey); err != nil {
		return Session{}, err
	}

	token := rand.Text()
	csrfToken := rand.Text()
	tokenHash := sha256.Sum256([]byte(token))
	csrfHash := sha256.Sum256([]byte(csrfToken))
	session := Session{
		ID:         uuid.NewString(),
		UserID:     user.ID,
		Token:      token,
		CSRFToken:  csrfToken,
		ExpiresAt:  now.Add(defaultSessionTTL),
		LastSeenAt: now,
	}
	if err := service.repository.RotateSession(ctx, sessionRecord{
		ID:            session.ID,
		UserID:        session.UserID,
		TokenHash:     tokenHash[:],
		CSRFTokenHash: csrfHash[:],
		ExpiresAt:     session.ExpiresAt,
		LastSeenAt:    session.LastSeenAt,
	}, now); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (service *Service) Authenticate(ctx context.Context, token string) (Identity, error) {
	tokenHash := sha256.Sum256([]byte(token))
	identity, err := service.repository.FindSession(ctx, tokenHash[:])
	if err != nil {
		return Identity{}, err
	}
	now := service.now().UTC()
	if !identity.ExpiresAt.After(now) {
		return Identity{}, ErrInvalidSession
	}
	if now.Sub(identity.LastSeenAt) >= defaultLastSeenInterval {
		if err := service.repository.TouchSession(ctx, identity.SessionID, now, now.Add(-defaultLastSeenInterval)); err != nil {
			return Identity{}, err
		}
		identity.LastSeenAt = now
	}
	return identity, nil
}

func (service *Service) Revoke(ctx context.Context, token string) error {
	tokenHash := sha256.Sum256([]byte(token))
	return service.repository.RevokeSession(ctx, tokenHash[:], service.now().UTC())
}

func (service *Service) ValidateCSRF(identity Identity, token string) bool {
	actual := sha256.Sum256([]byte(token))
	return len(identity.CSRFTokenHash) == len(actual) &&
		subtle.ConstantTimeCompare(identity.CSRFTokenHash, actual[:]) == 1
}

func (service *Service) recordLoginFailure(ctx context.Context, key string, attempt loginAttempt, now time.Time) error {
	if attempt.WindowStarted.IsZero() {
		attempt.WindowStarted = now
	}
	attempt.Failures++
	if attempt.Failures >= defaultLoginFailureLimit {
		attempt.BlockedUntil = now.Add(defaultLoginBlockDuration)
	}
	return service.repository.SaveLoginAttempt(ctx, key, attempt)
}

func loginBucketKey(username, clientBucket string) string {
	hash := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(username)) + "\x00" + clientBucket))
	return hex.EncodeToString(hash[:])
}
