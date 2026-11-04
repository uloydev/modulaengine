package queue

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

type ReaperConfig struct {
	CheckInterval  time.Duration
	LockTimeoutSec int
}

func DefaultReaperConfig() ReaperConfig {
	return ReaperConfig{
		CheckInterval:  60 * time.Second,
		LockTimeoutSec: 300, // 5 minutes
	}
}

type Reaper struct {
	db  DB
	cfg ReaperConfig
	wg  sync.WaitGroup
}

func NewReaper(db DB, cfg ReaperConfig) *Reaper {
	if cfg.CheckInterval <= 0 {
		cfg.CheckInterval = 60 * time.Second
	}
	if cfg.LockTimeoutSec <= 0 {
		cfg.LockTimeoutSec = 300
	}
	return &Reaper{
		db:  db,
		cfg: cfg,
	}
}

func (r *Reaper) ReapOnce(ctx context.Context) (int64, error) {
	tag, err := r.db.Exec(ctx, ReapStrandedJobsSQL, r.cfg.LockTimeoutSec)
	if err != nil {
		return 0, fmt.Errorf("reap stranded jobs: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (r *Reaper) Start(ctx context.Context) {
	r.wg.Add(1)
	go r.loop(ctx)
}

func (r *Reaper) Stop() {
	r.wg.Wait()
}

func (r *Reaper) loop(ctx context.Context) {
	defer r.wg.Done()

	ticker := time.NewTicker(r.cfg.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reaped, err := r.ReapOnce(ctx)
			if err != nil {
				slog.Error("reaper cycle failed", "err", err)
			} else if reaped > 0 {
				slog.Warn("reaped stranded jobs", "count", reaped)
			}
		}
	}
}
