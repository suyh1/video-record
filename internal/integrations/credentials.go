package integrations

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
)

const CredentialVersion1 = 1

var (
	ErrCredentialsLocked    = errors.New("integration credentials are locked")
	ErrInvalidCredentialKey = errors.New("invalid credential encryption key")
	ErrCredentialTampered   = errors.New("encrypted credentials failed authentication")
	ErrCredentialVersion    = errors.New("unsupported credential version")
)

type EncryptedCredential struct {
	Ciphertext  []byte
	Nonce       []byte
	Version     int
	Fingerprint string
}

type CredentialCipher struct {
	key []byte
}

func NewCredentialCipher(key []byte) *CredentialCipher {
	return &CredentialCipher{key: append([]byte(nil), key...)}
}

func (credentialCipher *CredentialCipher) Encrypt(plaintext []byte) (EncryptedCredential, error) {
	aead, err := credentialCipher.aead()
	if err != nil {
		return EncryptedCredential{}, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return EncryptedCredential{}, err
	}
	ciphertext := aead.Seal(nil, nonce, plaintext, credentialAssociatedData(CredentialVersion1))
	return EncryptedCredential{
		Ciphertext:  ciphertext,
		Nonce:       nonce,
		Version:     CredentialVersion1,
		Fingerprint: credentialCipher.fingerprint(plaintext),
	}, nil
}

func (credentialCipher *CredentialCipher) Decrypt(encrypted EncryptedCredential) ([]byte, error) {
	if encrypted.Version != CredentialVersion1 {
		return nil, ErrCredentialVersion
	}
	if len(credentialCipher.key) == 0 {
		return nil, ErrCredentialsLocked
	}
	aead, err := credentialCipher.aead()
	if err != nil {
		return nil, err
	}
	if len(encrypted.Nonce) != aead.NonceSize() {
		return nil, ErrCredentialTampered
	}
	plaintext, err := aead.Open(nil, encrypted.Nonce, encrypted.Ciphertext, credentialAssociatedData(encrypted.Version))
	if err != nil {
		return nil, ErrCredentialTampered
	}
	return plaintext, nil
}

func (credentialCipher *CredentialCipher) aead() (cipher.AEAD, error) {
	if len(credentialCipher.key) != 32 {
		return nil, ErrInvalidCredentialKey
	}
	block, err := aes.NewCipher(credentialCipher.key)
	if err != nil {
		return nil, ErrInvalidCredentialKey
	}
	return cipher.NewGCM(block)
}

func (credentialCipher *CredentialCipher) fingerprint(plaintext []byte) string {
	hash := hmac.New(sha256.New, credentialCipher.key)
	_, _ = hash.Write(plaintext)
	return hex.EncodeToString(hash.Sum(nil)[:8])
}

func credentialAssociatedData(version int) []byte {
	return []byte{byte(version), 'v', 'i', 'd', 'e', 'o', '-', 'r', 'e', 'c', 'o', 'r', 'd'}
}
