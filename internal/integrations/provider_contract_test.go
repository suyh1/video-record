package integrations

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHistoryPageValidationRequiresStableIDsAndIdentityHints(t *testing.T) {
	valid := HistoryPage{
		Events: []HistoryEvent{{
			ID: "event-1", PlayedAt: time.Now().UTC(),
			Item: ItemIdentity{ProviderItemID: "item-1", MediaType: MediaMovie, Title: "Synthetic Movie", Year: 2026},
		}},
		NextCursor: "cursor-2",
	}
	require.NoError(t, ValidateHistoryPage(valid))
	require.ErrorIs(t, ValidateHistoryPage(HistoryPage{Events: []HistoryEvent{{Item: valid.Events[0].Item}}}), ErrInvalidHistory)
	missingHints := valid
	missingHints.Events = []HistoryEvent{{ID: "event-1", PlayedAt: time.Now().UTC(), Item: ItemIdentity{MediaType: MediaMovie}}}
	require.ErrorIs(t, ValidateHistoryPage(missingHints), ErrInvalidHistory)
	duplicate := valid
	duplicate.Events = append(duplicate.Events, duplicate.Events[0])
	require.ErrorIs(t, ValidateHistoryPage(duplicate), ErrInvalidHistory)
}

func TestProviderErrorsClassifyRetriesWithoutLeakingSecrets(t *testing.T) {
	secret := "synthetic-provider-token"
	err := NewProviderError(ErrorRateLimited, 30*time.Second, errors.New("upstream echoed "+secret))
	require.True(t, IsRetryable(err))
	require.Equal(t, 30*time.Second, RetryAfter(err))
	require.NotContains(t, err.Error(), secret)
	require.ErrorIs(t, err, ErrRateLimited)
	require.False(t, IsRetryable(NewProviderError(ErrorAuthentication, 0, errors.New(secret))))
	require.False(t, IsRetryable(context.Canceled))
	for _, testCase := range []struct {
		kind     ErrorKind
		sentinel error
		retry    bool
	}{
		{ErrorAuthentication, ErrAuthentication, false},
		{ErrorRateLimited, ErrRateLimited, true},
		{ErrorUnavailable, ErrUnavailable, true},
		{ErrorInvalidResponse, ErrInvalidResponse, false},
	} {
		classified := NewProviderError(testCase.kind, time.Second, errors.New(secret))
		require.ErrorIs(t, classified, testCase.sentinel)
		require.Equal(t, testCase.retry, IsRetryable(classified))
		require.NotContains(t, classified.Error(), secret)
	}
	require.Zero(t, RetryAfter(context.Canceled))
}
