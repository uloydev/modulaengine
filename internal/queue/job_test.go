package queue_test

import (
	"testing"
	"time"

	"github.com/uloydev/modulaengine/internal/queue"
)

func TestComputeBackoff(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		attempts int
		expected time.Duration
	}{
		{name: "0 attempts", attempts: 0, expected: 10 * time.Second},
		{name: "1 attempt", attempts: 1, expected: 10 * time.Second},
		{name: "2 attempts", attempts: 2, expected: 20 * time.Second},
		{name: "3 attempts", attempts: 3, expected: 40 * time.Second},
		{name: "4 attempts", attempts: 4, expected: 80 * time.Second},
		{name: "high attempts capped", attempts: 20, expected: 3600 * time.Second},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			actual := queue.ComputeBackoff(tt.attempts)
			if actual != tt.expected {
				t.Errorf("ComputeBackoff(%d) = %v, want %v", tt.attempts, actual, tt.expected)
			}
		})
	}
}

func TestJobStatus_IsValid(t *testing.T) {
	t.Parallel()

	validStatuses := []queue.JobStatus{
		queue.StatusPending,
		queue.StatusRunning,
		queue.StatusCompleted,
		queue.StatusFailed,
	}

	for _, s := range validStatuses {
		if !s.IsValid() {
			t.Errorf("expected status %s to be valid", s)
		}
	}

	if queue.JobStatus("UNKNOWN").IsValid() {
		t.Errorf("expected UNKNOWN status to be invalid")
	}
}
