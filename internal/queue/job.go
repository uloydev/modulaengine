package queue

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"time"
)

var (
	ErrNoJobAvailable = errors.New("no pending job available")
	ErrUnregisteredTask = errors.New("no handler registered for task type")
	ErrInvalidJobState = errors.New("invalid job state")
)

type JobStatus string

const (
	StatusPending   JobStatus = "PENDING"
	StatusRunning   JobStatus = "RUNNING"
	StatusCompleted JobStatus = "COMPLETED"
	StatusFailed    JobStatus = "FAILED"
)

func (s JobStatus) IsValid() bool {
	switch s {
	case StatusPending, StatusRunning, StatusCompleted, StatusFailed:
		return true
	default:
		return false
	}
}

type Job struct {
	ID          int64           `json:"id"`
	TenantID    int64           `json:"tenant_id"`
	WorkflowID  *int64          `json:"workflow_id,omitempty"`
	TaskType    string          `json:"task_type"`
	Payload     json.RawMessage `json:"payload"`
	Status      JobStatus       `json:"status"`
	RunAt       time.Time       `json:"run_at"`
	Attempts    int             `json:"attempts"`
	MaxAttempts int             `json:"max_attempts"`
	LastError   *string         `json:"last_error,omitempty"`
	LockedBy    *string         `json:"locked_by,omitempty"`
	LockedAt    *time.Time      `json:"locked_at,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
}

type Handler func(ctx context.Context, job *Job) error

func ComputeBackoff(attempts int) time.Duration {
	if attempts <= 0 {
		return 10 * time.Second
	}
	// Exponential backoff: 10s * 2^(attempts-1), capped at 1 hour
	secs := float64(10) * math.Pow(2, float64(attempts-1))
	if secs > 3600 {
		secs = 3600
	}
	return time.Duration(secs) * time.Second
}
