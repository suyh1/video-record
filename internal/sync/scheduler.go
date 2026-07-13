package sync

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

type Runner interface {
	Run(context.Context, Job) (RunResult, error)
}

type RunnerFunc func(context.Context, Job) (RunResult, error)

func (runner RunnerFunc) Run(ctx context.Context, job Job) (RunResult, error) {
	return runner(ctx, job)
}

type RunSummary struct {
	Fetched    int `json:"fetched"`
	Imported   int `json:"imported"`
	Candidates int `json:"candidates"`
}

type RunResult struct {
	Cursor  string
	Summary RunSummary
}

type runDueError struct {
	providerErrors  []error
	schedulerErrors []error
}

func (err *runDueError) Error() string {
	if len(err.schedulerErrors) > 0 {
		return "sync scheduler operation failed"
	}
	return "sync provider run failed"
}

func (err *runDueError) Unwrap() []error {
	causes := make([]error, 0, len(err.providerErrors)+len(err.schedulerErrors))
	causes = append(causes, err.providerErrors...)
	causes = append(causes, err.schedulerErrors...)
	return causes
}

func (err *runDueError) fatal() bool {
	return len(err.schedulerErrors) > 0
}

type SchedulerOptions struct {
	Owner         string
	LeaseDuration time.Duration
	PollInterval  time.Duration
}

type Scheduler struct {
	service       *Service
	runner        Runner
	owner         string
	leaseDuration time.Duration
	pollInterval  time.Duration
}

func NewScheduler(service *Service, runner Runner, options SchedulerOptions) *Scheduler {
	owner := options.Owner
	if owner == "" {
		owner = uuid.NewString()
	}
	leaseDuration := options.LeaseDuration
	if leaseDuration <= 0 {
		leaseDuration = 5 * time.Minute
	}
	pollInterval := options.PollInterval
	if pollInterval <= 0 {
		pollInterval = time.Minute
	}
	return &Scheduler{
		service: service, runner: runner, owner: owner,
		leaseDuration: leaseDuration, pollInterval: pollInterval,
	}
}

func (scheduler *Scheduler) RunDue(ctx context.Context) (int, error) {
	jobs, err := scheduler.service.ClaimDue(ctx, scheduler.owner, 16, scheduler.leaseDuration)
	if err != nil {
		return 0, err
	}
	var providerErrors []error
	var schedulerErrors []error
	for _, job := range jobs {
		runID, beginErr := scheduler.service.BeginRun(ctx, job)
		if beginErr != nil {
			schedulerErrors = append(schedulerErrors, beginErr)
			if err := scheduler.service.Complete(ctx, job.ID, scheduler.owner, RunResult{}, beginErr); err != nil {
				schedulerErrors = append(schedulerErrors, err)
			}
			continue
		}
		result, runErr := scheduler.runner.Run(ctx, job)
		if err := scheduler.service.FinishRun(ctx, runID, result, runErr); err != nil {
			schedulerErrors = append(schedulerErrors, err)
		}
		if err := scheduler.service.Complete(ctx, job.ID, scheduler.owner, result, runErr); err != nil {
			schedulerErrors = append(schedulerErrors, err)
		}
		if runErr != nil {
			providerErrors = append(providerErrors, runErr)
		}
	}
	if len(providerErrors) == 0 && len(schedulerErrors) == 0 {
		return len(jobs), nil
	}
	return len(jobs), &runDueError{
		providerErrors: providerErrors, schedulerErrors: schedulerErrors,
	}
}

func (scheduler *Scheduler) Start(ctx context.Context) <-chan error {
	done := make(chan error, 1)
	go func() {
		defer close(done)
		ticker := time.NewTicker(scheduler.pollInterval)
		defer ticker.Stop()
		for {
			_, runErr := scheduler.RunDue(ctx)
			if ctx.Err() != nil {
				done <- ctx.Err()
				return
			}
			if runErr != nil && !errors.Is(runErr, context.Canceled) {
				var dueErr *runDueError
				if !errors.As(runErr, &dueErr) || dueErr.fatal() {
					done <- runErr
					return
				}
			}
			select {
			case <-ctx.Done():
				done <- ctx.Err()
				return
			case <-ticker.C:
			}
		}
	}()
	return done
}
