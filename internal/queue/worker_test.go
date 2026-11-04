package queue_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/uloydev/modulaengine/internal/queue"
)

type mockRow struct {
	scanFunc func(dest ...any) error
}

func (m *mockRow) Scan(dest ...any) error {
	return m.scanFunc(dest...)
}

type mockDB struct {
	queryRowFunc func(ctx context.Context, sql string, args ...any) pgx.Row
	execFunc     func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (m *mockDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return m.queryRowFunc(ctx, sql, args...)
}

func (m *mockDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return m.execFunc(ctx, sql, args...)
}

func TestWorkerPool_ExecuteJob_Success(t *testing.T) {
	t.Parallel()

	completedJobId := int64(0)
	db := &mockDB{
		execFunc: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			if sql == queue.CompleteJobSQL {
				completedJobId = args[0].(int64)
			}
			return pgconn.NewCommandTag("UPDATE 1"), nil
		},
	}

	pool := queue.NewWorkerPool(db, queue.DefaultWorkerConfig("worker-1"))
	handled := false
	pool.RegisterHandler("notification:email", func(ctx context.Context, job *queue.Job) error {
		handled = true
		return nil
	})

	job := &queue.Job{
		ID:          100,
		TaskType:    "notification:email",
		Attempts:    0,
		MaxAttempts: 3,
	}

	err := pool.ExecuteJob(context.Background(), job)
	if err != nil {
		t.Fatalf("unexpected error executing job: %v", err)
	}

	if !handled {
		t.Errorf("expected handler to be invoked")
	}
	if completedJobId != 100 {
		t.Errorf("expected completed job ID 100, got %d", completedJobId)
	}
}

func TestWorkerPool_ExecuteJob_Retry(t *testing.T) {
	t.Parallel()

	retriedJobId := int64(0)
	db := &mockDB{
		execFunc: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			if sql == queue.RetryJobSQL {
				retriedJobId = args[0].(int64)
			}
			return pgconn.NewCommandTag("UPDATE 1"), nil
		},
	}

	pool := queue.NewWorkerPool(db, queue.DefaultWorkerConfig("worker-1"))
	pool.RegisterHandler("webhook:send", func(ctx context.Context, job *queue.Job) error {
		return errors.New("connection timeout")
	})

	job := &queue.Job{
		ID:          200,
		TaskType:    "webhook:send",
		Attempts:    1,
		MaxAttempts: 3,
	}

	err := pool.ExecuteJob(context.Background(), job)
	if err == nil {
		t.Fatalf("expected error from failed job, got nil")
	}

	if retriedJobId != 200 {
		t.Errorf("expected retry on job ID 200, got %d", retriedJobId)
	}
}

func TestWorkerPool_ExecuteJob_MaxAttemptsReached(t *testing.T) {
	t.Parallel()

	failedJobId := int64(0)
	db := &mockDB{
		execFunc: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			if sql == queue.FailJobSQL {
				failedJobId = args[0].(int64)
			}
			return pgconn.NewCommandTag("UPDATE 1"), nil
		},
	}

	pool := queue.NewWorkerPool(db, queue.DefaultWorkerConfig("worker-1"))
	pool.RegisterHandler("webhook:send", func(ctx context.Context, job *queue.Job) error {
		return errors.New("500 internal server error")
	})

	job := &queue.Job{
		ID:          300,
		TaskType:    "webhook:send",
		Attempts:    2,
		MaxAttempts: 3,
	}

	err := pool.ExecuteJob(context.Background(), job)
	if err == nil {
		t.Fatalf("expected error from permanently failed job, got nil")
	}

	if failedJobId != 300 {
		t.Errorf("expected job ID 300 to be marked failed, got %d", failedJobId)
	}
}

func TestWorkerPool_ExecuteJob_UnregisteredTask(t *testing.T) {
	t.Parallel()

	failedJobId := int64(0)
	db := &mockDB{
		execFunc: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			if sql == queue.FailJobSQL {
				failedJobId = args[0].(int64)
			}
			return pgconn.NewCommandTag("UPDATE 1"), nil
		},
	}

	pool := queue.NewWorkerPool(db, queue.DefaultWorkerConfig("worker-1"))

	job := &queue.Job{
		ID:       400,
		TaskType: "unregistered:task",
	}

	err := pool.ExecuteJob(context.Background(), job)
	if !errors.Is(err, queue.ErrUnregisteredTask) {
		t.Fatalf("expected ErrUnregisteredTask, got: %v", err)
	}

	if failedJobId != 400 {
		t.Errorf("expected unregistered job 400 to be marked failed, got: %d", failedJobId)
	}
}

func TestWorkerPool_GracefulShutdown(t *testing.T) {
	t.Parallel()

	db := &mockDB{
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			return &mockRow{
				scanFunc: func(dest ...any) error {
					return pgx.ErrNoRows
				},
			}
		},
	}

	cfg := queue.DefaultWorkerConfig("worker-test")
	cfg.Concurrency = 2
	cfg.PollInterval = 10 * time.Millisecond

	pool := queue.NewWorkerPool(db, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	pool.Start(ctx)

	// Let workers run for a brief moment then cancel
	time.Sleep(25 * time.Millisecond)
	cancel()

	done := make(chan struct{})
	go func() {
		pool.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Clean exit
	case <-time.After(2 * time.Second):
		t.Fatalf("worker pool did not shut down gracefully within deadline")
	}
}

func TestReaper_ReapOnce(t *testing.T) {
	t.Parallel()

	db := &mockDB{
		execFunc: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("UPDATE 3"), nil
		},
	}

	reaper := queue.NewReaper(db, queue.DefaultReaperConfig())
	reaped, err := reaper.ReapOnce(context.Background())
	if err != nil {
		t.Fatalf("ReapOnce error: %v", err)
	}

	if reaped != 3 {
		t.Errorf("expected 3 reaped jobs, got %d", reaped)
	}
}

func TestWorkerPool_ExecuteJob_PanicRecovery(t *testing.T) {
	t.Parallel()

	retriedJobId := int64(0)
	db := &mockDB{
		execFunc: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			if sql == queue.RetryJobSQL {
				retriedJobId = args[0].(int64)
			}
			return pgconn.NewCommandTag("UPDATE 1"), nil
		},
	}

	pool := queue.NewWorkerPool(db, queue.DefaultWorkerConfig("worker-panic-test"))
	pool.RegisterHandler("task:panic", func(ctx context.Context, job *queue.Job) error {
		panic("nil pointer dereference inside handler")
	})

	job := &queue.Job{
		ID:          555,
		TaskType:    "task:panic",
		Attempts:    0,
		MaxAttempts: 3,
	}

	err := pool.ExecuteJob(context.Background(), job)
	if err == nil {
		t.Fatalf("expected error from recovered panic, got nil")
	}

	if retriedJobId != 555 {
		t.Errorf("expected panicked job 555 to be scheduled for retry, got: %d", retriedJobId)
	}
}
