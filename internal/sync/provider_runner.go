package sync

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"video-record/internal/integrations"
	"video-record/internal/storage"
)

var ErrProviderAccountUnavailable = errors.New("provider account unavailable")

const maxProviderPagesPerRun = 10_000

type ProviderRunnerOptions struct {
	Now                func() time.Time
	PageLimit          int
	CompensationWindow time.Duration
}

type ProviderRunner struct {
	db                 *storage.DB
	accounts           *integrations.AccountRepository
	candidates         *CandidateService
	factory            ProviderFactory
	now                func() time.Time
	pageLimit          int
	compensationWindow time.Duration
}

func NewProviderRunner(
	db *storage.DB,
	accounts *integrations.AccountRepository,
	candidates *CandidateService,
	factory ProviderFactory,
	options ProviderRunnerOptions,
) *ProviderRunner {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	pageLimit := options.PageLimit
	if pageLimit <= 0 || pageLimit > 200 {
		pageLimit = 200
	}
	compensationWindow := options.CompensationWindow
	if compensationWindow <= 0 {
		compensationWindow = 24 * time.Hour
	}
	return &ProviderRunner{
		db: db, accounts: accounts, candidates: candidates, factory: factory,
		now: now, pageLimit: pageLimit, compensationWindow: compensationWindow,
	}
}

func (runner *ProviderRunner) Run(ctx context.Context, job Job) (RunResult, error) {
	if job.AccountID == "" || job.Kind != JobIncremental && job.Kind != JobCompensation ||
		runner.db == nil || runner.accounts == nil || runner.candidates == nil || runner.factory == nil {
		return RunResult{}, ErrProviderAccountUnavailable
	}
	account, err := runner.account(ctx, job.AccountID)
	if err != nil {
		return RunResult{}, err
	}
	credentials, err := runner.accounts.Credentials(ctx, account.UserID, account.ID)
	if err != nil {
		return RunResult{}, err
	}
	provider, err := runner.factory.New(account, credentials)
	if err != nil {
		return RunResult{}, err
	}
	if err := provider.CheckAuthentication(ctx); err != nil {
		return RunResult{}, err
	}

	now := runner.now().UTC()
	request := integrations.HistoryRequest{
		Cursor: job.Cursor, Limit: runner.pageLimit, Until: now,
	}
	if job.Kind == JobCompensation {
		request.Cursor = ""
		request.Since = now.Add(-runner.compensationWindow)
	}
	result := RunResult{}
	for pageNumber := 0; pageNumber < maxProviderPagesPerRun; pageNumber++ {
		if err := ctx.Err(); err != nil {
			return RunResult{}, err
		}
		page, err := provider.History(ctx, request)
		if err != nil {
			return RunResult{}, err
		}
		if err := integrations.ValidateHistoryPage(page); err != nil {
			return RunResult{}, err
		}
		for _, event := range page.Events {
			candidate, err := runner.candidates.Ingest(ctx, account.ID, event)
			if err != nil {
				return RunResult{}, err
			}
			result.Summary.Fetched++
			switch candidate.Status {
			case CandidateConfirmed:
				result.Summary.Imported++
			case CandidateExact, CandidatePossible, CandidateUnmatched, CandidateConflict:
				result.Summary.Candidates++
			}
		}
		if len(page.Events) == 0 || page.NextCursor == "" {
			if job.Kind == JobIncremental {
				result.Cursor = request.Cursor
			}
			return result, nil
		}
		if page.NextCursor == request.Cursor {
			return RunResult{}, integrations.ErrInvalidHistory
		}
		request.Cursor = page.NextCursor
	}
	return RunResult{}, integrations.ErrInvalidHistory
}

func (runner *ProviderRunner) account(ctx context.Context, accountID string) (integrations.Account, error) {
	var account integrations.Account
	var createdAt, updatedAt int64
	err := runner.db.Reader().QueryRowContext(ctx, `
		SELECT id, user_id, provider, name, base_url, credential_fingerprint,
		       enabled, created_at, updated_at
		FROM external_accounts
		WHERE id = ? AND enabled = 1
	`, accountID).Scan(
		&account.ID, &account.UserID, &account.Provider, &account.Name, &account.BaseURL,
		&account.CredentialFingerprint, &account.Enabled, &createdAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return integrations.Account{}, ErrProviderAccountUnavailable
	}
	if err != nil {
		return integrations.Account{}, err
	}
	account.CreatedAt = time.UnixMilli(createdAt).UTC()
	account.UpdatedAt = time.UnixMilli(updatedAt).UTC()
	return account, nil
}
