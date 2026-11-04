package queue

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type DB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

type WorkerConfig struct {
	Concurrency  int
	WorkerID     string
	PollInterval time.Duration
}

func DefaultWorkerConfig(workerID string) WorkerConfig {
	return WorkerConfig{
		Concurrency:  4,
		WorkerID:     workerID,
		PollInterval: 500 * time.Millisecond,
	}
}

type WorkerPool struct {
	db       DB
	cfg      WorkerConfig
	handlers map[string]Handler
	mu       sync.RWMutex
	wg       sync.WaitGroup
}

func NewWorkerPool(db DB, cfg WorkerConfig) *WorkerPool {
	if cfg.Concurrency < 1 {
		cfg.Concurrency = 1
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 500 * time.Millisecond
	}
	return &WorkerPool{
		db:       db,
		cfg:      cfg,
		handlers: make(map[string]Handler),
	}
}

func (p *WorkerPool) RegisterHandler(taskType string, h Handler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handlers[taskType] = h
}

func (p *WorkerPool) getHandler(taskType string) (Handler, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	h, exists := p.handlers[taskType]
	return h, exists
}

func (p *WorkerPool) Enqueue(
	ctx context.Context,
	tenantID int64,
	workflowID *int64,
	taskType string,
	payload []byte,
	runAt time.Time,
	maxAttempts int,
) (*Job, error) {
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	if runAt.IsZero() {
		runAt = time.Now()
	}

	job := &Job{}
	row := p.db.QueryRow(ctx, EnqueueJobSQL, tenantID, workflowID, taskType, payload, runAt, maxAttempts)
	err := row.Scan(
		&job.ID,
		&job.TenantID,
		&job.WorkflowID,
		&job.TaskType,
		&job.Payload,
		&job.Status,
		&job.RunAt,
		&job.Attempts,
		&job.MaxAttempts,
		&job.LastError,
		&job.LockedBy,
		&job.LockedAt,
		&job.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("enqueue job: %w", err)
	}

	return job, nil
}

func (p *WorkerPool) Claim(ctx context.Context) (*Job, error) {
	job := &Job{}
	row := p.db.QueryRow(ctx, ClaimJobSQL, p.cfg.WorkerID)
	err := row.Scan(
		&job.ID,
		&job.TenantID,
		&job.WorkflowID,
		&job.TaskType,
		&job.Payload,
		&job.Status,
		&job.RunAt,
		&job.Attempts,
		&job.MaxAttempts,
		&job.LastError,
		&job.LockedBy,
		&job.LockedAt,
		&job.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNoJobAvailable
		}
		return nil, fmt.Errorf("claim job: %w", err)
	}
	return job, nil
}

func (p *WorkerPool) safeRunHandler(ctx context.Context, h Handler, job *Job) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("task handler panic: %v", r)
		}
	}()
	return h(ctx, job)
}

func (p *WorkerPool) ExecuteJob(ctx context.Context, job *Job) error {
	handler, exists := p.getHandler(job.TaskType)
	if !exists {
		errStr := fmt.Sprintf("%v: %s", ErrUnregisteredTask, job.TaskType)
		_, _ = p.db.Exec(ctx, FailJobSQL, job.ID, errStr)
		return fmt.Errorf("%w: %s", ErrUnregisteredTask, job.TaskType)
	}

	runErr := p.safeRunHandler(ctx, handler, job)
	if runErr == nil {
		_, err := p.db.Exec(ctx, CompleteJobSQL, job.ID)
		if err != nil {
			return fmt.Errorf("mark completed: %w", err)
		}
		return nil
	}

	// Task failed
	job.Attempts++
	errStr := runErr.Error()

	if job.Attempts >= job.MaxAttempts {
		_, err := p.db.Exec(ctx, FailJobSQL, job.ID, errStr)
		if err != nil {
			return fmt.Errorf("mark failed: %w", err)
		}
		return fmt.Errorf("job failed permanently (%d/%d attempts): %w", job.Attempts, job.MaxAttempts, runErr)
	}

	// Retry with exponential backoff
	backoff := ComputeBackoff(job.Attempts)
	nextRun := time.Now().Add(backoff)
	_, err := p.db.Exec(ctx, RetryJobSQL, job.ID, nextRun, errStr)
	if err != nil {
		return fmt.Errorf("schedule retry: %w", err)
	}

	return fmt.Errorf("job scheduled for retry in %s: %w", backoff, runErr)
}

func (p *WorkerPool) Start(ctx context.Context) {
	for i := 0; i < p.cfg.Concurrency; i++ {
		p.wg.Add(1)
		go p.workerLoop(ctx, i)
	}
}

func (p *WorkerPool) Stop() {
	p.wg.Wait()
}

func (p *WorkerPool) workerLoop(ctx context.Context, workerNum int) {
	defer p.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		job, err := p.Claim(ctx)
		if err != nil {
			if errors.Is(err, ErrNoJobAvailable) {
				select {
				case <-ctx.Done():
					return
				case <-time.After(p.cfg.PollInterval):
					continue
				}
			}
			slog.Error("error claiming job", "worker", workerNum, "err", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(p.cfg.PollInterval):
				continue
			}
		}

		if err := p.ExecuteJob(ctx, job); err != nil {
			slog.Warn("job execution failed", "job_id", job.ID, "task_type", job.TaskType, "err", err)
		}
	}
}
