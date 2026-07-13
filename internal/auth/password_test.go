package auth

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPasswordHashUsesRecordedArgon2idParameters(t *testing.T) {
	encoded, err := HashPassword("correct horse battery staple")

	require.NoError(t, err)
	require.Contains(t, encoded, "$argon2id$v=19$m=65536,t=3,p=2$")
	require.NotContains(t, encoded, "correct horse battery staple")
}

func TestPasswordHashVerifiesOnlyMatchingPassword(t *testing.T) {
	encoded, err := HashPassword("correct horse battery staple")
	require.NoError(t, err)

	matches, err := VerifyPassword(encoded, "correct horse battery staple")
	require.NoError(t, err)
	require.True(t, matches)

	matches, err = VerifyPassword(encoded, "wrong password")
	require.NoError(t, err)
	require.False(t, matches)
}

func TestPasswordHashRejectsMalformedValuesSafely(t *testing.T) {
	for _, encoded := range []string{
		"",
		"not-a-hash",
		"$argon2id$v=19$m=bad,t=3,p=2$c2FsdA$aGFzaA",
		"$argon2id$v=18$m=65536,t=3,p=2$c2FsdA$aGFzaA",
	} {
		matches, err := VerifyPassword(encoded, "password")
		require.False(t, matches)
		require.ErrorIs(t, err, ErrInvalidPasswordHash)
	}
}
