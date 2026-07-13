package providertest

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"video-record/internal/integrations"
)

type Factory func(t *testing.T, scenario string) integrations.Provider

type Expectations struct {
	FirstCursor  string
	SecondCursor string
	Secret       string
}

func Run(t *testing.T, factory Factory, expected Expectations) {
	t.Helper()
	t.Run("authentication", func(t *testing.T) {
		if err := factory(t, "success").CheckAuthentication(context.Background()); err != nil {
			t.Fatalf("authentication check failed: %v", err)
		}
	})
	t.Run("paginated stable history", func(t *testing.T) {
		provider := factory(t, "success")
		first, err := provider.History(context.Background(), integrations.HistoryRequest{Limit: 2})
		if err != nil {
			t.Fatalf("first history page failed: %v", err)
		}
		if err := integrations.ValidateHistoryPage(first); err != nil {
			t.Fatalf("first history page is invalid: %v", err)
		}
		if first.NextCursor != expected.FirstCursor {
			t.Fatalf("first cursor = %q, want %q", first.NextCursor, expected.FirstCursor)
		}
		replayed, err := provider.History(context.Background(), integrations.HistoryRequest{Limit: 2})
		if err != nil {
			t.Fatalf("replayed history page failed: %v", err)
		}
		if !reflect.DeepEqual(first.Events, replayed.Events) {
			t.Fatalf("replayed events differ: got %#v, want %#v", replayed.Events, first.Events)
		}
		second, err := provider.History(context.Background(), integrations.HistoryRequest{Cursor: first.NextCursor, Limit: 2})
		if err != nil {
			t.Fatalf("second history page failed: %v", err)
		}
		if err := integrations.ValidateHistoryPage(second); err != nil {
			t.Fatalf("second history page is invalid: %v", err)
		}
		if second.NextCursor != expected.SecondCursor {
			t.Fatalf("second cursor = %q, want %q", second.NextCursor, expected.SecondCursor)
		}
	})
	t.Run("cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := factory(t, "success").History(ctx, integrations.HistoryRequest{Limit: 1})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation error = %v, want context.Canceled", err)
		}
	})
	t.Run("redacted retry", func(t *testing.T) {
		_, err := factory(t, "rate_limited").History(context.Background(), integrations.HistoryRequest{Limit: 1})
		if err == nil {
			t.Fatal("rate-limited history returned nil error")
		}
		if !integrations.IsRetryable(err) {
			t.Fatalf("rate-limited error is not retryable: %v", err)
		}
		if strings.Contains(err.Error(), expected.Secret) {
			t.Fatal("rate-limited error leaked the provider secret")
		}
		if !errors.Is(err, integrations.ErrRateLimited) {
			t.Fatalf("rate-limited error = %v, want ErrRateLimited", err)
		}
	})
}
