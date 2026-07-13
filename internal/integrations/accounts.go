package integrations

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"video-record/internal/storage"
)

var (
	ErrAccountNotFound = errors.New("integration account not found")
	ErrInvalidAccount  = errors.New("invalid integration account")
)

type Account struct {
	ID                    string
	UserID                string
	Provider              string
	Name                  string
	BaseURL               string
	CredentialFingerprint string
	Enabled               bool
	Locked                bool
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type CreateAccountInput struct {
	UserID      string
	Provider    string
	Name        string
	BaseURL     string
	Credentials []byte
	Enabled     bool
}

type AccountRepositoryOptions struct {
	Now func() time.Time
}

type AccountRepository struct {
	db     *storage.DB
	cipher *CredentialCipher
	now    func() time.Time
}

func NewAccountRepository(
	db *storage.DB,
	cipher *CredentialCipher,
	options AccountRepositoryOptions,
) *AccountRepository {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &AccountRepository{db: db, cipher: cipher, now: now}
}

func (repository *AccountRepository) Create(ctx context.Context, input CreateAccountInput) (Account, error) {
	input.UserID = strings.TrimSpace(input.UserID)
	input.Provider = strings.TrimSpace(input.Provider)
	input.Name = strings.TrimSpace(input.Name)
	input.BaseURL = strings.TrimSpace(input.BaseURL)
	if input.UserID == "" || !validProvider(input.Provider) || input.Name == "" ||
		!validBaseURL(input.BaseURL) || len(input.Credentials) == 0 {
		return Account{}, ErrInvalidAccount
	}
	encrypted, err := repository.cipher.Encrypt(input.Credentials)
	if err != nil {
		return Account{}, err
	}
	now := repository.now().UTC()
	account := Account{
		ID: uuid.NewString(), UserID: input.UserID, Provider: input.Provider,
		Name: input.Name, BaseURL: input.BaseURL,
		CredentialFingerprint: encrypted.Fingerprint,
		Enabled:               input.Enabled, CreatedAt: now, UpdatedAt: now,
	}
	_, err = repository.db.Writer().ExecContext(ctx, `
		INSERT INTO external_accounts (
			id, user_id, provider, name, base_url, credential_ciphertext,
			credential_nonce, credential_version, credential_fingerprint,
			enabled, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, account.ID, account.UserID, account.Provider, account.Name, account.BaseURL,
		encrypted.Ciphertext, encrypted.Nonce, encrypted.Version, encrypted.Fingerprint,
		account.Enabled, now.UnixMilli(), now.UnixMilli())
	if err != nil {
		return Account{}, err
	}
	return account, nil
}

func (repository *AccountRepository) List(ctx context.Context, userID string) ([]Account, error) {
	rows, err := repository.db.Reader().QueryContext(ctx, `
		SELECT id, user_id, provider, name, base_url, credential_ciphertext,
		       credential_nonce, credential_version, credential_fingerprint,
		       enabled, created_at, updated_at
		FROM external_accounts
		WHERE user_id = ?
		ORDER BY provider, name, id
	`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	accounts := make([]Account, 0)
	for rows.Next() {
		account, encrypted, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		if _, err := repository.cipher.Decrypt(encrypted); err != nil {
			account.Locked = true
		}
		accounts = append(accounts, account)
	}
	return accounts, rows.Err()
}

func (repository *AccountRepository) Credentials(ctx context.Context, userID, accountID string) ([]byte, error) {
	var encrypted EncryptedCredential
	err := repository.db.Reader().QueryRowContext(ctx, `
		SELECT credential_ciphertext, credential_nonce, credential_version, credential_fingerprint
		FROM external_accounts
		WHERE id = ? AND user_id = ?
	`, accountID, userID).Scan(
		&encrypted.Ciphertext, &encrypted.Nonce, &encrypted.Version, &encrypted.Fingerprint,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAccountNotFound
	}
	if err != nil {
		return nil, err
	}
	plaintext, err := repository.cipher.Decrypt(encrypted)
	if err != nil {
		return nil, ErrCredentialsLocked
	}
	return plaintext, nil
}

func validProvider(provider string) bool {
	return provider == "jellyfin" || provider == "emby" || provider == "plex"
}

func validBaseURL(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" ||
		parsed.Fragment != "" || parsed.Opaque != "" {
		return false
	}
	return strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https")
}

type accountScanner interface {
	Scan(...any) error
}

func scanAccount(row accountScanner) (Account, EncryptedCredential, error) {
	var account Account
	var encrypted EncryptedCredential
	var createdAt, updatedAt int64
	if err := row.Scan(
		&account.ID, &account.UserID, &account.Provider, &account.Name, &account.BaseURL,
		&encrypted.Ciphertext, &encrypted.Nonce, &encrypted.Version, &account.CredentialFingerprint,
		&account.Enabled, &createdAt, &updatedAt,
	); err != nil {
		return Account{}, EncryptedCredential{}, err
	}
	encrypted.Fingerprint = account.CredentialFingerprint
	account.CreatedAt = time.UnixMilli(createdAt).UTC()
	account.UpdatedAt = time.UnixMilli(updatedAt).UTC()
	return account, encrypted, nil
}
