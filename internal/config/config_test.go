package config

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadDefaults(t *testing.T) {
	clearConfigEnvironment(t)

	cfg, err := Load()

	require.NoError(t, err)
	require.Equal(t, "development", cfg.Environment)
	require.Equal(t, 8080, cfg.Port)
	require.Equal(t, "/data", cfg.DataDir)
	require.False(t, cfg.CookieSecure)
}

func TestLoadEnablesSecureCookiesInProduction(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("APP_ENV", "production")

	cfg, err := Load()

	require.NoError(t, err)
	require.True(t, cfg.CookieSecure)
}

func TestLoadRejectsMalformedEncryptionKeyWithoutEchoingIt(t *testing.T) {
	clearConfigEnvironment(t)
	const malformedKey = "not-a-valid-key"
	t.Setenv("APP_ENCRYPTION_KEY", malformedKey)

	_, err := Load()

	require.ErrorIs(t, err, ErrInvalidEncryptionKey)
	require.NotContains(t, err.Error(), malformedKey)
}

func TestLoadUsesEnvironmentOverrides(t *testing.T) {
	clearConfigEnvironment(t)
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	t.Setenv("APP_ENV", "test")
	t.Setenv("APP_PORT", "9090")
	t.Setenv("DATA_DIR", t.TempDir())
	t.Setenv("APP_COOKIE_SECURE", "true")
	t.Setenv("APP_ENCRYPTION_KEY", key)
	t.Setenv("TMDB_READ_ACCESS_TOKEN", "synthetic-tmdb-token")

	cfg, err := Load()

	require.NoError(t, err)
	require.Equal(t, "test", cfg.Environment)
	require.Equal(t, 9090, cfg.Port)
	require.NotEqual(t, "/data", cfg.DataDir)
	require.True(t, cfg.CookieSecure)
	require.Len(t, cfg.EncryptionKey, 32)
	require.Equal(t, "synthetic-tmdb-token", cfg.TMDBReadAccessToken)
}

func clearConfigEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"APP_ENV",
		"APP_PORT",
		"DATA_DIR",
		"APP_COOKIE_SECURE",
		"APP_ENCRYPTION_KEY",
		"TMDB_READ_ACCESS_TOKEN",
	} {
		t.Setenv(key, "")
	}
}
