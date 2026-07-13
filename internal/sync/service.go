package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"video-record/internal/storage"
)

const (
	incrementalInterval  = 15 * time.Minute
	compensationInterval = 24 * time.Hour
)

var ErrLeaseLost = errors.New("sync job lease lost")

type JobKind string

const (
	JobIncremental  JobKind = "incremental"
	JobCompensation JobKind = "compensation"
)

type Job struct {
	ID             string
	AccountID      string
	Kind           JobKind
	NextRunAt      time.Time
	LeaseOwner     string
	LeaseExpiresAt *time.Time
	LastErrorCode  string
	Cursor         string
}

type ServiceOptions struct {
	Now func() time.Time
}

type Service struct {
	db  *storage.DB
	now func() time.Time
}

func NewService(db *storage.DB, options ServiceOptions) *Service {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &Service{db: db, now: now}
}

func (service *Service) EnsureJobs(ctx context.Context, accountID string) error {
	now := service.now().UTC().UnixMilli()
	tx, err := service.db.Writer().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, kind := range []JobKind{JobIncremental, JobCompensation} {
		if _, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO sync_jobs (
				id, account_id, kind, next_run_at, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?)
		`, uuid.NewString(), accountID, kind, now, now, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (service *Service) ClaimDue(
	ctx context.Context,
	owner string,
	limit int,
	leaseDuration time.Duration,
) ([]Job, error) {
	if owner == "" || limit < 1 || leaseDuration <= 0 {
		return nil, errors.New("invalid sync lease")
	}
	now := service.now().UTC()
	tx, err := service.db.Writer().BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `
		SELECT job.id, job.account_id, job.kind, job.next_run_at, job.cursor,
		       job.lease_owner, job.lease_expires_at, job.last_error_code
		FROM sync_jobs job
		JOIN external_accounts account ON account.id = job.account_id AND account.enabled = 1
		WHERE job.next_run_at <= ?
		  AND (job.lease_expires_at IS NULL OR job.lease_expires_at <= ?)
		ORDER BY job.next_run_at, job.id
		LIMIT ?
	`, now.UnixMilli(), now.UnixMilli(), limit)
	if err != nil {
		return nil, err
	}
	jobs := make([]Job, 0, limit)
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		jobs = append(jobs, job)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for index := range jobs {
		expiresAt := now.Add(leaseDuration)
		result, err := tx.ExecContext(ctx, `
			UPDATE sync_jobs SET lease_owner = ?, lease_expires_at = ?, updated_at = ?
			WHERE id = ? AND (lease_expires_at IS NULL OR lease_expires_at <= ?)
		`, owner, expiresAt.UnixMilli(), now.UnixMilli(), jobs[index].ID, now.UnixMilli())
		if err != nil {
			return nil, err
		}
		claimed, err := result.RowsAffected()
		if err != nil {
			return nil, err
		}
		if claimed != 1 {
			return nil, ErrLeaseLost
		}
		jobs[index].LeaseOwner = owner
		jobs[index].LeaseExpiresAt = &expiresAt
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return jobs, nil
}

func (service *Service) Complete(ctx context.Context, jobID, owner string, result RunResult, runErr error) error {
	now := service.now().UTC()
	var kind JobKind
	err := service.db.Reader().QueryRowContext(ctx, `
		SELECT kind FROM sync_jobs
		WHERE id = ? AND lease_owner = ? AND lease_expires_at > ?
	`, jobID, owner, now.UnixMilli()).Scan(&kind)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrLeaseLost
	}
	if err != nil {
		return err
	}
	interval := incrementalInterval
	if kind == JobCompensation {
		interval = compensationInterval
	}
	var errorCode any
	if runErr != nil {
		errorCode = "run_failed"
	}
	var cursor any
	if runErr == nil && result.Cursor != "" {
		cursor = result.Cursor
	}
	execResult, err := service.db.Writer().ExecContext(ctx, `
		UPDATE sync_jobs SET
			next_run_at = ?, lease_owner = NULL, lease_expires_at = NULL,
			last_error_code = ?, cursor = COALESCE(?, cursor), updated_at = ?
		WHERE id = ? AND lease_owner = ? AND lease_expires_at > ?
	`, now.Add(interval).UnixMilli(), errorCode, cursor, now.UnixMilli(), jobID, owner, now.UnixMilli())
	if err != nil {
		return err
	}
	updated, err := execResult.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return ErrLeaseLost
	}
	return nil
}

func (service *Service) BeginRun(ctx context.Context, job Job) (string, error) {
	runID := uuid.NewString()
	_, err := service.db.Writer().ExecContext(ctx, `
		INSERT INTO sync_runs (
			id, account_id, job_kind, status, summary_json, started_at
		) VALUES (?, ?, ?, 'running', '{}', ?)
	`, runID, job.AccountID, job.Kind, service.now().UTC().UnixMilli())
	if err != nil {
		return "", err
	}
	return runID, nil
}

func (service *Service) FinishRun(ctx context.Context, runID string, result RunResult, runErr error) error {
	status := "succeeded"
	var cursor any = result.Cursor
	if runErr != nil {
		status = "failed"
		cursor = nil
	}
	summary, err := json.Marshal(result.Summary)
	if err != nil {
		return err
	}
	execResult, err := service.db.Writer().ExecContext(ctx, `
		UPDATE sync_runs SET status = ?, cursor = ?, summary_json = ?, finished_at = ?
		WHERE id = ? AND status = 'running'
	`, status, cursor, string(summary), service.now().UTC().UnixMilli(), runID)
	if err != nil {
		return err
	}
	updated, err := execResult.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return errors.New("sync run not found")
	}
	return nil
}

func (service *Service) Jobs(ctx context.Context, accountID string) ([]Job, error) {
	rows, err := service.db.Reader().QueryContext(ctx, `
		SELECT id, account_id, kind, next_run_at, cursor, lease_owner, lease_expires_at, last_error_code
		FROM sync_jobs WHERE account_id = ? ORDER BY kind
	`, accountID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	jobs := make([]Job, 0, 2)
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

type rowScanner interface {
	Scan(...any) error
}

func scanJob(row rowScanner) (Job, error) {
	var job Job
	var nextRunAt int64
	var cursor, leaseOwner, lastError sql.NullString
	var leaseExpires sql.NullInt64
	if err := row.Scan(
		&job.ID, &job.AccountID, &job.Kind, &nextRunAt, &cursor,
		&leaseOwner, &leaseExpires, &lastError,
	); err != nil {
		return Job{}, err
	}
	job.NextRunAt = time.UnixMilli(nextRunAt).UTC()
	job.LeaseOwner = leaseOwner.String
	job.LastErrorCode = lastError.String
	job.Cursor = cursor.String
	if leaseExpires.Valid {
		value := time.UnixMilli(leaseExpires.Int64).UTC()
		job.LeaseExpiresAt = &value
	}
	return job, nil
}
