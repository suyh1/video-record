package sync

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"video-record/internal/storage"
)

func TestPersistedJobsCatchUpAfterRestartAndUseSingleRunLeases(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(ctx, db))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	accountID := insertSchedulerAccount(t, db)
	now := time.Date(2026, 7, 13, 8, 0, 0, 0, time.UTC)
	service := NewService(db, ServiceOptions{Now: func() time.Time { return now }})
	require.NoError(t, service.EnsureJobs(ctx, accountID))

	now = now.Add(2 * time.Hour)
	restarted := NewService(db, ServiceOptions{Now: func() time.Time { return now }})
	ownerAJobs, err := restarted.ClaimDue(ctx, "scheduler-a", 10, 5*time.Minute)
	require.NoError(t, err)
	require.Len(t, ownerAJobs, 2)
	require.ElementsMatch(t, []JobKind{JobIncremental, JobCompensation}, []JobKind{ownerAJobs[0].Kind, ownerAJobs[1].Kind})
	ownerBJobs, err := restarted.ClaimDue(ctx, "scheduler-b", 10, 5*time.Minute)
	require.NoError(t, err)
	require.Empty(t, ownerBJobs)

	require.NoError(t, restarted.Complete(ctx, ownerAJobs[0].ID, "scheduler-a", RunResult{}, nil))
	jobs, err := restarted.Jobs(ctx, accountID)
	require.NoError(t, err)
	completed := findJob(t, jobs, ownerAJobs[0].ID)
	expectedInterval := 15 * time.Minute
	if completed.Kind == JobCompensation {
		expectedInterval = 24 * time.Hour
	}
	require.Equal(t, now.Add(expectedInterval), completed.NextRunAt)
	require.Empty(t, completed.LeaseOwner)

	now = now.Add(6 * time.Minute)
	reclaimed, err := restarted.ClaimDue(ctx, "scheduler-b", 10, 5*time.Minute)
	require.NoError(t, err)
	require.Len(t, reclaimed, 1)
	require.NotEqual(t, ownerAJobs[0].ID, reclaimed[0].ID)
}

func TestCompleteRejectsExpiredLeaseAndAllowsReclaim(t *testing.T) {
	ctx := context.Background()
	db := openSchedulerDB(t)
	accountID := insertSchedulerAccount(t, db)
	now := time.Date(2026, 7, 13, 8, 0, 0, 0, time.UTC)
	service := NewService(db, ServiceOptions{Now: func() time.Time { return now }})
	require.NoError(t, service.EnsureJobs(ctx, accountID))
	claimed, err := service.ClaimDue(ctx, "expired-owner", 1, time.Minute)
	require.NoError(t, err)
	require.Len(t, claimed, 1)

	now = now.Add(2 * time.Minute)
	require.ErrorIs(t, service.Complete(
		ctx, claimed[0].ID, "expired-owner", RunResult{Cursor: "stale-cursor"}, nil,
	), ErrLeaseLost)
	reclaimed, err := service.ClaimDue(ctx, "replacement-owner", 1, time.Minute)
	require.NoError(t, err)
	require.Len(t, reclaimed, 1)
	require.Equal(t, claimed[0].ID, reclaimed[0].ID)
	require.Empty(t, reclaimed[0].Cursor)
}

func TestSchedulerStartsNonBlockingAndPersistsSuccessfulRuns(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	db := openSchedulerDB(t)
	accountID := insertSchedulerAccount(t, db)
	service := NewService(db, ServiceOptions{})
	require.NoError(t, service.EnsureJobs(ctx, accountID))
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	var runs atomic.Int64
	scheduler := NewScheduler(service, RunnerFunc(func(context.Context, Job) (RunResult, error) {
		started <- struct{}{}
		<-release
		runs.Add(1)
		return RunResult{Cursor: "cursor-next", Summary: RunSummary{Fetched: 2, Imported: 1}}, nil
	}), SchedulerOptions{Owner: "scheduler-start"})
	done := scheduler.Start(ctx)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("scheduler did not start a due job")
	}
	close(release)
	require.Eventually(t, func() bool { return runs.Load() == 2 }, time.Second, 10*time.Millisecond)
	var successful int
	require.Eventually(t, func() bool {
		return db.Reader().QueryRowContext(context.Background(), `
			SELECT COUNT(*) FROM sync_runs
			WHERE account_id = ? AND status = 'succeeded' AND cursor = 'cursor-next'
		`, accountID).Scan(&successful) == nil && successful == 2
	}, time.Second, 10*time.Millisecond)
	jobs, err := service.Jobs(context.Background(), accountID)
	require.NoError(t, err)
	for _, job := range jobs {
		require.Equal(t, "cursor-next", job.Cursor)
	}
	var summary string
	require.NoError(t, db.Reader().QueryRowContext(ctx, `
		SELECT summary_json FROM sync_runs
		WHERE account_id = ? AND status = 'succeeded'
		ORDER BY started_at, id LIMIT 1
	`, accountID).Scan(&summary))
	require.JSONEq(t, `{"fetched":2,"imported":1,"candidates":0}`, summary)
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
}

func TestNewSchedulerGeneratesLeaseOwnerWhenOmitted(t *testing.T) {
	ctx := context.Background()
	db := openSchedulerDB(t)
	accountID := insertSchedulerAccount(t, db)
	service := NewService(db, ServiceOptions{})
	require.NoError(t, service.EnsureJobs(ctx, accountID))
	scheduler := NewScheduler(service, RunnerFunc(func(context.Context, Job) (RunResult, error) {
		return RunResult{}, nil
	}), SchedulerOptions{})

	count, err := scheduler.RunDue(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, count)
}

func TestSchedulerPersistsFailuresAndServiceRejectsLostLeases(t *testing.T) {
	ctx := context.Background()
	db := openSchedulerDB(t)
	accountID := insertSchedulerAccount(t, db)
	now := time.Date(2026, 7, 13, 9, 0, 0, 0, time.UTC)
	service := NewService(db, ServiceOptions{Now: func() time.Time { return now }})
	require.NoError(t, service.EnsureJobs(ctx, accountID))
	runFailure := errors.New("synthetic run failure containing provider-secret")
	scheduler := NewScheduler(service, RunnerFunc(func(context.Context, Job) (RunResult, error) {
		return RunResult{
			Cursor:  "must-not-advance",
			Summary: RunSummary{Fetched: 3, Imported: 1, Candidates: 2},
		}, runFailure
	}), SchedulerOptions{Owner: "scheduler-failure", LeaseDuration: time.Minute, PollInterval: time.Hour})
	count, err := scheduler.RunDue(ctx)
	require.Equal(t, 2, count)
	require.ErrorIs(t, err, runFailure)
	require.NotContains(t, err.Error(), "provider-secret")
	jobs, err := service.Jobs(ctx, accountID)
	require.NoError(t, err)
	for _, job := range jobs {
		require.Equal(t, "run_failed", job.LastErrorCode)
		require.Empty(t, job.LeaseOwner)
		require.Empty(t, job.Cursor)
	}
	var failed int
	require.NoError(t, db.Reader().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sync_runs WHERE account_id = ? AND status = 'failed'
	`, accountID).Scan(&failed))
	require.Equal(t, 2, failed)
	rows, err := db.Reader().QueryContext(ctx, `
		SELECT cursor, summary_json FROM sync_runs
		WHERE account_id = ? AND status = 'failed'
	`, accountID)
	require.NoError(t, err)
	defer func() { require.NoError(t, rows.Close()) }()
	for rows.Next() {
		var cursor *string
		var summary string
		require.NoError(t, rows.Scan(&cursor, &summary))
		require.Nil(t, cursor)
		require.JSONEq(t, `{"fetched":3,"imported":1,"candidates":2}`, summary)
		require.False(t, strings.Contains(summary, "provider-secret"))
	}
	require.NoError(t, rows.Err())
	require.Error(t, service.EnsureJobs(ctx, "missing-account"))
	_, err = service.ClaimDue(ctx, "", 0, 0)
	require.Error(t, err)
	require.ErrorIs(t, service.Complete(ctx, "missing-job", "missing-owner", RunResult{}, nil), ErrLeaseLost)
	require.Error(t, service.FinishRun(ctx, "missing-run", RunResult{}, nil))
}

func TestServiceReturnsStorageErrorsAfterClose(t *testing.T) {
	ctx := context.Background()
	db := openSchedulerDB(t)
	accountID := insertSchedulerAccount(t, db)
	service := NewService(db, ServiceOptions{})
	require.NoError(t, service.EnsureJobs(ctx, accountID))
	require.NoError(t, db.Close())

	require.Error(t, service.EnsureJobs(ctx, accountID))
	_, err := service.ClaimDue(ctx, "closed-storage", 1, time.Minute)
	require.Error(t, err)
	require.Error(t, service.Complete(ctx, "job", "closed-storage", RunResult{}, nil))
	_, err = service.BeginRun(ctx, Job{AccountID: accountID, Kind: JobIncremental})
	require.Error(t, err)
	require.Error(t, service.FinishRun(ctx, "run", RunResult{}, nil))
	_, err = service.Jobs(ctx, accountID)
	require.Error(t, err)
}

func TestSchedulerContinuesPollingAfterProviderFailures(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	db := openSchedulerDB(t)
	accountID := insertSchedulerAccount(t, db)
	initial := time.Date(2026, 7, 13, 11, 0, 0, 0, time.UTC)
	var clock atomic.Int64
	clock.Store(initial.UnixMilli())
	service := NewService(db, ServiceOptions{Now: func() time.Time {
		return time.UnixMilli(clock.Load()).UTC()
	}})
	require.NoError(t, service.EnsureJobs(ctx, accountID))
	var runs atomic.Int64
	scheduler := NewScheduler(service, RunnerFunc(func(context.Context, Job) (RunResult, error) {
		runs.Add(1)
		return RunResult{}, errors.New("temporary provider failure containing provider-secret")
	}), SchedulerOptions{
		Owner: "resilient-scheduler", LeaseDuration: time.Minute, PollInterval: 10 * time.Millisecond,
	})
	done := scheduler.Start(ctx)
	require.Eventually(t, func() bool { return runs.Load() == 2 }, 3*time.Second, 10*time.Millisecond)
	clock.Store(initial.Add(25 * time.Hour).UnixMilli())
	require.Eventually(t, func() bool { return runs.Load() >= 4 }, 3*time.Second, 10*time.Millisecond)
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
}

func openSchedulerDB(t *testing.T) *storage.DB {
	t.Helper()
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(ctx, db))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db
}

func insertSchedulerAccount(t *testing.T, db *storage.DB) string {
	t.Helper()
	ctx := context.Background()
	userID := "scheduler-user"
	accountID := "scheduler-account"
	_, err := db.Writer().ExecContext(ctx, `
		INSERT INTO users (id, username, password_hash, role, active, created_at)
		VALUES (?, 'scheduler-owner', 'synthetic-hash', 'admin', 1, 0)
	`, userID)
	require.NoError(t, err)
	_, err = db.Writer().ExecContext(ctx, `
		INSERT INTO external_accounts (
			id, user_id, provider, name, base_url, credential_ciphertext,
			credential_nonce, credential_version, credential_fingerprint,
			enabled, created_at, updated_at
		) VALUES (?, ?, 'jellyfin', 'Home', 'https://media.example.test', x'01', x'02', 1, 'fingerprint', 1, 0, 0)
	`, accountID, userID)
	require.NoError(t, err)
	return accountID
}

func findJob(t *testing.T, jobs []Job, id string) Job {
	t.Helper()
	for _, job := range jobs {
		if job.ID == id {
			return job
		}
	}
	t.Fatalf("job %s not found", id)
	return Job{}
}
