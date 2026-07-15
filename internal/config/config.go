package config

import (
	"encoding/base64"
	"errors"
	"os"
	"strconv"
	"strings"
)

var (
	ErrInvalidCookieSecure  = errors.New("invalid cookie secure setting")
	ErrInvalidEncryptionKey = errors.New("invalid application encryption key")
	ErrInvalidPort          = errors.New("invalid application port")
)

type Config struct {
	Environment         string
	Port                int
	DataDir             string
	CookieSecure        bool
	EncryptionKey       []byte
	TMDBAPIBaseURL      string
	TMDBImageBaseURL    string
	TMDBReadAccessToken string
}

func Load() (Config, error) {
	environment := valueOrDefault("APP_ENV", "development")
	port, err := parsePort(valueOrDefault("APP_PORT", "8080"))
	if err != nil {
		return Config{}, err
	}

	cookieSecure := environment == "production"
	if value := strings.TrimSpace(os.Getenv("APP_COOKIE_SECURE")); value != "" {
		cookieSecure, err = strconv.ParseBool(value)
		if err != nil {
			return Config{}, ErrInvalidCookieSecure
		}
	}

	encryptionKey, err := parseEncryptionKey(strings.TrimSpace(os.Getenv("APP_ENCRYPTION_KEY")))
	if err != nil {
		return Config{}, err
	}

	return Config{
		Environment:         environment,
		Port:                port,
		DataDir:             valueOrDefault("DATA_DIR", "/data"),
		CookieSecure:        cookieSecure,
		EncryptionKey:       encryptionKey,
		TMDBAPIBaseURL:      strings.TrimSpace(os.Getenv("TMDB_API_BASE_URL")),
		TMDBImageBaseURL:    strings.TrimSpace(os.Getenv("TMDB_IMAGE_BASE_URL")),
		TMDBReadAccessToken: strings.TrimSpace(os.Getenv("TMDB_READ_ACCESS_TOKEN")),
	}, nil
}

func parsePort(value string) (int, error) {
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, ErrInvalidPort
	}
	return port, nil
}

func parseEncryptionKey(value string) ([]byte, error) {
	if value == "" {
		return nil, nil
	}

	key, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(key) != 32 {
		return nil, ErrInvalidEncryptionKey
	}
	return key, nil
}

func valueOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
