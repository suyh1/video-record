package sync

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"video-record/internal/integrations"
	"video-record/internal/storage"
)

func TestProviderRunnerDecryptsCredentialsPaginatesAndIngestsCandidates(t *testing.T) {
	ctx := context.Background()
	db, accountRepository, candidateService, account := newProviderRunnerServices(t)
	insertCandidateMedia(t, db, "runner-exact", "movie", "Runner Exact", "2026")
	insertCandidateExternalID(t, db, "runner-exact", "tmdb", "901", "movie")
	exact := candidateMovieEvent("runner-event-exact", "runner-provider-exact", "Runner Exact", 2026)
	exact.Item.TMDBID = "901"
	unmatched := candidateMovieEvent("runner-event-unmatched", "runner-provider-unmatched", "Runner Unknown", 2025)

	provider := &scriptedProvider{pages: map[string]integrations.HistoryPage{
		"":       {Events: []integrations.HistoryEvent{exact}, NextCursor: "page-2"},
		"page-2": {Events: []integrations.HistoryEvent{unmatched}, NextCursor: "done"},
		"done":   {Events: []integrations.HistoryEvent{}, NextCursor: "done"},
	}}
	var factoryAccount integrations.Account
	var factoryCredentials []byte
	runner := NewProviderRunner(
		db,
		accountRepository,
		candidateService,
		ProviderFactoryFunc(func(account integrations.Account, credentials []byte) (integrations.Provider, error) {
			factoryAccount = account
			factoryCredentials = append([]byte(nil), credentials...)
			return provider, nil
		}),
		ProviderRunnerOptions{Now: func() time.Time {
			return time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
		}},
	)

	result, err := runner.Run(ctx, Job{AccountID: account.ID, Kind: JobIncremental})

	require.NoError(t, err)
	require.Equal(t, "done", result.Cursor)
	require.Equal(t, RunSummary{Fetched: 2, Imported: 1, Candidates: 1}, result.Summary)
	require.Equal(t, account.ID, factoryAccount.ID)
	require.Equal(t, []byte(`{"token":"synthetic-provider-token","userId":"provider-user"}`), factoryCredentials)
	require.Equal(t, 1, provider.authenticationChecks)
	require.Equal(t, []string{"", "page-2", "done"}, provider.cursors)
	require.Equal(t, 2, countRows(t, db, "sync_candidates"))
	require.Equal(t, 1, countRows(t, db, "watch_events"))
}

func TestProviderRunnerUsesCompensationWindowWithoutAdvancingCursor(t *testing.T) {
	ctx := context.Background()
	db, accountRepository, candidateService, account := newProviderRunnerServices(t)
	provider := &scriptedProvider{pages: map[string]integrations.HistoryPage{
		"": {Events: []integrations.HistoryEvent{}},
	}}
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	runner := NewProviderRunner(
		db,
		accountRepository,
		candidateService,
		ProviderFactoryFunc(func(integrations.Account, []byte) (integrations.Provider, error) {
			return provider, nil
		}),
		ProviderRunnerOptions{
			Now: func() time.Time { return now }, PageLimit: 17, CompensationWindow: 48 * time.Hour,
		},
	)

	result, err := runner.Run(ctx, Job{
		AccountID: account.ID, Kind: JobCompensation, Cursor: "incremental-cursor",
	})

	require.NoError(t, err)
	require.Empty(t, result.Cursor)
	require.Len(t, provider.requests, 1)
	require.Empty(t, provider.requests[0].Cursor)
	require.Equal(t, now.Add(-48*time.Hour), provider.requests[0].Since)
	require.Equal(t, now, provider.requests[0].Until)
	require.Equal(t, 17, provider.requests[0].Limit)
}

func TestProviderRunnerRejectsLockedAccountsAndUnsafeProviderResults(t *testing.T) {
	t.Run("invalid job", func(t *testing.T) {
		db, accounts, candidates, _ := newProviderRunnerServices(t)
		runner := NewProviderRunner(db, accounts, candidates, ProviderFactoryFunc(func(
			integrations.Account, []byte,
		) (integrations.Provider, error) {
			return &scriptedProvider{}, nil
		}), ProviderRunnerOptions{})

		_, err := runner.Run(context.Background(), Job{Kind: JobKind("unknown")})
		require.ErrorIs(t, err, ErrProviderAccountUnavailable)
	})

	t.Run("disabled account", func(t *testing.T) {
		db, accounts, candidates, account := newProviderRunnerServices(t)
		_, err := db.Writer().ExecContext(context.Background(), `
			UPDATE external_accounts SET enabled = 0 WHERE id = ?
		`, account.ID)
		require.NoError(t, err)
		runner := NewProviderRunner(db, accounts, candidates, ProviderFactoryFunc(func(
			integrations.Account, []byte,
		) (integrations.Provider, error) {
			return &scriptedProvider{}, nil
		}), ProviderRunnerOptions{})

		_, err = runner.Run(context.Background(), Job{AccountID: account.ID, Kind: JobIncremental})
		require.ErrorIs(t, err, ErrProviderAccountUnavailable)
	})

	t.Run("locked credentials", func(t *testing.T) {
		db, _, candidates, account := newProviderRunnerServices(t)
		lockedAccounts := integrations.NewAccountRepository(
			db,
			integrations.NewCredentialCipher(bytes.Repeat([]byte{0x72}, 32)),
			integrations.AccountRepositoryOptions{},
		)
		runner := NewProviderRunner(db, lockedAccounts, candidates, ProviderFactoryFunc(func(
			integrations.Account, []byte,
		) (integrations.Provider, error) {
			return &scriptedProvider{}, nil
		}), ProviderRunnerOptions{})

		_, err := runner.Run(context.Background(), Job{AccountID: account.ID, Kind: JobIncremental})
		require.ErrorIs(t, err, integrations.ErrCredentialsLocked)
	})

	t.Run("factory and authentication errors", func(t *testing.T) {
		db, accounts, candidates, account := newProviderRunnerServices(t)
		factoryFailure := NewProviderRunner(db, accounts, candidates, ProviderFactoryFunc(func(
			integrations.Account, []byte,
		) (integrations.Provider, error) {
			return nil, ErrInvalidProviderConfiguration
		}), ProviderRunnerOptions{})
		_, err := factoryFailure.Run(context.Background(), Job{AccountID: account.ID, Kind: JobIncremental})
		require.ErrorIs(t, err, ErrInvalidProviderConfiguration)

		authFailure := NewProviderRunner(db, accounts, candidates, ProviderFactoryFunc(func(
			integrations.Account, []byte,
		) (integrations.Provider, error) {
			return &scriptedProvider{authenticationError: integrations.ErrAuthentication}, nil
		}), ProviderRunnerOptions{})
		_, err = authFailure.Run(context.Background(), Job{AccountID: account.ID, Kind: JobIncremental})
		require.ErrorIs(t, err, integrations.ErrAuthentication)
	})

	t.Run("invalid and stalled pages", func(t *testing.T) {
		db, accounts, candidates, account := newProviderRunnerServices(t)
		event := candidateMovieEvent("duplicate-event", "duplicate-provider", "Duplicate", 2026)
		invalidPage := NewProviderRunner(db, accounts, candidates, ProviderFactoryFunc(func(
			integrations.Account, []byte,
		) (integrations.Provider, error) {
			return &scriptedProvider{pages: map[string]integrations.HistoryPage{
				"": {Events: []integrations.HistoryEvent{event, event}},
			}}, nil
		}), ProviderRunnerOptions{})
		_, err := invalidPage.Run(context.Background(), Job{AccountID: account.ID, Kind: JobIncremental})
		require.ErrorIs(t, err, integrations.ErrInvalidHistory)

		stalledPage := NewProviderRunner(db, accounts, candidates, ProviderFactoryFunc(func(
			integrations.Account, []byte,
		) (integrations.Provider, error) {
			return &scriptedProvider{pages: map[string]integrations.HistoryPage{
				"stalled": {Events: []integrations.HistoryEvent{event}, NextCursor: "stalled"},
			}}, nil
		}), ProviderRunnerOptions{})
		_, err = stalledPage.Run(context.Background(), Job{
			AccountID: account.ID, Kind: JobIncremental, Cursor: "stalled",
		})
		require.ErrorIs(t, err, integrations.ErrInvalidHistory)
	})
}

func TestDefaultProviderFactoryUsesStrictProviderSpecificCredentials(t *testing.T) {
	factory := NewDefaultProviderFactory(DefaultProviderFactoryOptions{})
	for name, testCase := range map[string]struct {
		account     integrations.Account
		credentials string
	}{
		"jellyfin": {
			account:     integrations.Account{Provider: "jellyfin", BaseURL: "https://jellyfin.example.test"},
			credentials: `{"token":"synthetic-token","userId":"provider-user"}`,
		},
		"emby": {
			account:     integrations.Account{Provider: "emby", BaseURL: "https://emby.example.test"},
			credentials: `{"token":"synthetic-token","userId":"provider-user","timezone":"Asia/Shanghai"}`,
		},
		"plex": {
			account:     integrations.Account{Provider: "plex", BaseURL: "https://plex.example.test"},
			credentials: `{"token":"synthetic-token","accountId":42}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			provider, err := factory.New(testCase.account, []byte(testCase.credentials))
			require.NoError(t, err)
			require.NotNil(t, provider)
		})
	}

	_, err := factory.New(
		integrations.Account{Provider: "jellyfin", BaseURL: "https://jellyfin.example.test"},
		[]byte(`{"token":"must-not-appear","userId":"provider-user","unexpected":true}`),
	)
	require.ErrorIs(t, err, ErrInvalidProviderConfiguration)
	require.NotContains(t, err.Error(), "must-not-appear")
	_, err = factory.New(
		integrations.Account{Provider: "emby", BaseURL: "https://emby.example.test"},
		[]byte(`{"token":"must-not-appear","userId":"provider-user","timezone":"Not/A_Zone"}`),
	)
	require.ErrorIs(t, err, ErrInvalidProviderConfiguration)
	_, err = factory.New(
		integrations.Account{Provider: "plex", BaseURL: "https://plex.example.test"},
		[]byte(`{"token":"must-not-appear","accountId":0}`),
	)
	require.ErrorIs(t, err, ErrInvalidProviderConfiguration)
	_, err = factory.New(
		integrations.Account{Provider: "jellyfin", BaseURL: "https://jellyfin.example.test"},
		[]byte(`{"token":"must-not-appear","userId":"provider-user"}{}`),
	)
	require.ErrorIs(t, err, ErrInvalidProviderConfiguration)
	_, err = factory.New(integrations.Account{Provider: "unknown"}, []byte(`{"token":"must-not-appear"}`))
	require.ErrorIs(t, err, ErrInvalidProviderConfiguration)
	require.NotContains(t, err.Error(), "must-not-appear")
}

func TestEnsureEnabledAccountJobsSkipsDisabledAccounts(t *testing.T) {
	ctx := context.Background()
	db, accountRepository, _, enabled := newProviderRunnerServices(t)
	disabled, err := accountRepository.Create(ctx, integrations.CreateAccountInput{
		UserID: enabled.UserID, Provider: "emby", Name: "Disabled",
		BaseURL: "https://disabled.example.test", Credentials: []byte(`{"token":"synthetic","userId":"disabled"}`),
		Enabled: false,
	})
	require.NoError(t, err)
	service := NewService(db, ServiceOptions{})

	require.NoError(t, service.EnsureEnabledAccountJobs(ctx))

	enabledJobs, err := service.Jobs(ctx, enabled.ID)
	require.NoError(t, err)
	require.Len(t, enabledJobs, 2)
	disabledJobs, err := service.Jobs(ctx, disabled.ID)
	require.NoError(t, err)
	require.Empty(t, disabledJobs)
}

type scriptedProvider struct {
	pages                map[string]integrations.HistoryPage
	authenticationError  error
	authenticationChecks int
	cursors              []string
	requests             []integrations.HistoryRequest
}

func (provider *scriptedProvider) CheckAuthentication(context.Context) error {
	provider.authenticationChecks++
	return provider.authenticationError
}

func (provider *scriptedProvider) History(
	_ context.Context,
	request integrations.HistoryRequest,
) (integrations.HistoryPage, error) {
	provider.cursors = append(provider.cursors, request.Cursor)
	provider.requests = append(provider.requests, request)
	page, ok := provider.pages[request.Cursor]
	if !ok {
		return integrations.HistoryPage{}, errors.New("unexpected synthetic cursor")
	}
	return page, nil
}

func newProviderRunnerServices(
	t *testing.T,
) (*storage.DB, *integrations.AccountRepository, *CandidateService, integrations.Account) {
	t.Helper()
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(ctx, db))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	userID := "provider-runner-user"
	_, err = db.Writer().ExecContext(ctx, `
		INSERT INTO users (id, username, password_hash, role, active, created_at)
		VALUES (?, 'provider-runner-owner', 'synthetic-hash', 'admin', 1, 0)
	`, userID)
	require.NoError(t, err)
	key := bytes.Repeat([]byte{0x71}, 32)
	accountRepository := integrations.NewAccountRepository(
		db, integrations.NewCredentialCipher(key), integrations.AccountRepositoryOptions{},
	)
	account, err := accountRepository.Create(ctx, integrations.CreateAccountInput{
		UserID: userID, Provider: "jellyfin", Name: "Home",
		BaseURL:     "https://media.example.test",
		Credentials: []byte(`{"token":"synthetic-provider-token","userId":"provider-user"}`),
		Enabled:     true,
	})
	require.NoError(t, err)
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	return db, accountRepository, NewCandidateService(db, CandidateServiceOptions{
		Now: func() time.Time { return now },
	}), account
}
