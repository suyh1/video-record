package integrations

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"video-record/internal/storage"
)

func TestCredentialCipherRoundTripTamperVersionAndMissingKey(t *testing.T) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	plaintext := []byte(`{"baseUrl":"https://media.example.test","token":"synthetic-token"}`)
	cipher := NewCredentialCipher(key)
	encrypted, err := cipher.Encrypt(plaintext)
	require.NoError(t, err)
	require.NotEmpty(t, encrypted.Ciphertext)
	require.NotEmpty(t, encrypted.Nonce)
	require.Equal(t, CredentialVersion1, encrypted.Version)
	require.NotEmpty(t, encrypted.Fingerprint)
	require.False(t, bytes.Contains(encrypted.Ciphertext, []byte("synthetic-token")))

	decrypted, err := cipher.Decrypt(encrypted)
	require.NoError(t, err)
	require.Equal(t, plaintext, decrypted)

	tampered := encrypted
	tampered.Ciphertext = append([]byte(nil), encrypted.Ciphertext...)
	tampered.Ciphertext[len(tampered.Ciphertext)-1] ^= 0xff
	_, err = cipher.Decrypt(tampered)
	require.ErrorIs(t, err, ErrCredentialTampered)
	unsupported := encrypted
	unsupported.Version++
	_, err = cipher.Decrypt(unsupported)
	require.ErrorIs(t, err, ErrCredentialVersion)
	_, err = NewCredentialCipher(nil).Decrypt(encrypted)
	require.ErrorIs(t, err, ErrCredentialsLocked)
	_, err = NewCredentialCipher(make([]byte, 31)).Encrypt(plaintext)
	require.ErrorIs(t, err, ErrInvalidCredentialKey)
	require.False(t, errors.Is(err, ErrCredentialTampered))
	invalidNonce := encrypted
	invalidNonce.Nonce = []byte{1}
	_, err = cipher.Decrypt(invalidNonce)
	require.ErrorIs(t, err, ErrCredentialTampered)
	wrongKey := make([]byte, 32)
	_, err = rand.Read(wrongKey)
	require.NoError(t, err)
	_, err = NewCredentialCipher(wrongKey).Decrypt(encrypted)
	require.ErrorIs(t, err, ErrCredentialTampered)
}

func TestAccountRepositoryStoresOnlyEncryptedCredentials(t *testing.T) {
	ctx := context.Background()
	db := openIntegrationDB(t)
	insertIntegrationUser(t, db, "user-a", "owner-a")
	key := bytes.Repeat([]byte{0x42}, 32)
	now := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	repository := NewAccountRepository(db, NewCredentialCipher(key), AccountRepositoryOptions{
		Now: func() time.Time { return now },
	})
	credentials := []byte(`{"token":"synthetic-media-token"}`)

	account, err := repository.Create(ctx, CreateAccountInput{
		UserID: "user-a", Provider: "jellyfin", Name: "Home",
		BaseURL: "https://media.example.test", Credentials: credentials, Enabled: true,
	})
	require.NoError(t, err)
	require.NotEmpty(t, account.ID)
	require.Equal(t, "user-a", account.UserID)
	require.Equal(t, "jellyfin", account.Provider)
	require.NotEmpty(t, account.CredentialFingerprint)
	require.False(t, account.Locked)

	var ciphertext, nonce []byte
	var version int
	var fingerprint string
	require.NoError(t, db.Reader().QueryRowContext(ctx, `
		SELECT credential_ciphertext, credential_nonce, credential_version, credential_fingerprint
		FROM external_accounts WHERE id = ?
	`, account.ID).Scan(&ciphertext, &nonce, &version, &fingerprint))
	require.NotContains(t, string(ciphertext), "synthetic-media-token")
	require.NotEmpty(t, nonce)
	require.Equal(t, CredentialVersion1, version)
	require.Equal(t, account.CredentialFingerprint, fingerprint)

	decrypted, err := repository.Credentials(ctx, "user-a", account.ID)
	require.NoError(t, err)
	require.Equal(t, credentials, decrypted)
}

func TestAccountRepositoryReportsLockedCredentialsWithoutMutatingData(t *testing.T) {
	ctx := context.Background()
	db := openIntegrationDB(t)
	insertIntegrationUser(t, db, "user-a", "owner-a")
	key := bytes.Repeat([]byte{0x21}, 32)
	repository := NewAccountRepository(db, NewCredentialCipher(key), AccountRepositoryOptions{})
	account, err := repository.Create(ctx, CreateAccountInput{
		UserID: "user-a", Provider: "emby", Name: "Living Room",
		BaseURL: "https://emby.example.test", Credentials: []byte("synthetic-secret"), Enabled: true,
	})
	require.NoError(t, err)

	for name, cipher := range map[string]*CredentialCipher{
		"missing key": NewCredentialCipher(nil),
		"changed key": NewCredentialCipher(bytes.Repeat([]byte{0x22}, 32)),
	} {
		t.Run(name, func(t *testing.T) {
			lockedRepository := NewAccountRepository(db, cipher, AccountRepositoryOptions{})
			accounts, err := lockedRepository.List(ctx, "user-a")
			require.NoError(t, err)
			require.Len(t, accounts, 1)
			require.True(t, accounts[0].Locked)
			_, err = lockedRepository.Credentials(ctx, "user-a", account.ID)
			require.ErrorIs(t, err, ErrCredentialsLocked)
		})
	}

	var tamperedCiphertext []byte
	require.NoError(t, db.Reader().QueryRowContext(ctx, `
		SELECT credential_ciphertext FROM external_accounts WHERE id = ?
	`, account.ID).Scan(&tamperedCiphertext))
	tamperedCiphertext[0] ^= 0xff
	_, err = db.Writer().ExecContext(ctx, `
		UPDATE external_accounts SET credential_ciphertext = ? WHERE id = ?
	`, tamperedCiphertext, account.ID)
	require.NoError(t, err)
	accounts, err := repository.List(ctx, "user-a")
	require.NoError(t, err)
	require.True(t, accounts[0].Locked)
	_, err = repository.Credentials(ctx, "user-a", account.ID)
	require.ErrorIs(t, err, ErrCredentialsLocked)

	var userCount, accountCount int
	require.NoError(t, db.Reader().QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE id = 'user-a'").Scan(&userCount))
	require.NoError(t, db.Reader().QueryRowContext(ctx, "SELECT COUNT(*) FROM external_accounts WHERE id = ?", account.ID).Scan(&accountCount))
	require.Equal(t, 1, userCount)
	require.Equal(t, 1, accountCount)
}

func TestAccountRepositoryScopesCredentialsToUser(t *testing.T) {
	ctx := context.Background()
	db := openIntegrationDB(t)
	insertIntegrationUser(t, db, "user-a", "owner-a")
	insertIntegrationUser(t, db, "user-b", "owner-b")
	repository := NewAccountRepository(db, NewCredentialCipher(bytes.Repeat([]byte{0x31}, 32)), AccountRepositoryOptions{})
	account, err := repository.Create(ctx, CreateAccountInput{
		UserID: "user-a", Provider: "plex", Name: "Private",
		BaseURL: "https://plex.example.test", Credentials: []byte("synthetic-secret"), Enabled: true,
	})
	require.NoError(t, err)

	accounts, err := repository.List(ctx, "user-b")
	require.NoError(t, err)
	require.Empty(t, accounts)
	_, err = repository.Credentials(ctx, "user-b", account.ID)
	require.ErrorIs(t, err, ErrAccountNotFound)
}

func TestAccountRepositoryRejectsBaseURLsThatCouldPersistPlaintextCredentials(t *testing.T) {
	ctx := context.Background()
	db := openIntegrationDB(t)
	insertIntegrationUser(t, db, "user-a", "owner-a")
	repository := NewAccountRepository(db, NewCredentialCipher(bytes.Repeat([]byte{0x51}, 32)), AccountRepositoryOptions{})

	for _, baseURL := range []string{
		"https://user:synthetic-secret@media.example.test",
		"https://media.example.test?token=synthetic-secret",
		"https://media.example.test/#synthetic-secret",
		"file:///tmp/media",
		"https://",
	} {
		_, err := repository.Create(ctx, CreateAccountInput{
			UserID: "user-a", Provider: "jellyfin", Name: "Unsafe",
			BaseURL: baseURL, Credentials: []byte("synthetic-secret"), Enabled: true,
		})
		require.ErrorIs(t, err, ErrInvalidAccount, baseURL)
	}

	account, err := repository.Create(ctx, CreateAccountInput{
		UserID: "user-a", Provider: "jellyfin", Name: "Subpath",
		BaseURL: "http://media.internal:8096/jellyfin", Credentials: []byte("synthetic-secret"), Enabled: true,
	})
	require.NoError(t, err)
	require.Equal(t, "http://media.internal:8096/jellyfin", account.BaseURL)
}

func openIntegrationDB(t *testing.T) *storage.DB {
	t.Helper()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(context.Background(), db))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db
}

func insertIntegrationUser(t *testing.T, db *storage.DB, id, username string) {
	t.Helper()
	_, err := db.Writer().ExecContext(context.Background(), `
		INSERT INTO users (id, username, password_hash, role, active, created_at)
		VALUES (?, ?, 'synthetic-hash', 'member', 1, 0)
	`, id, username)
	require.NoError(t, err)
}
