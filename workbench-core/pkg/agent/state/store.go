package state

import (
	"context"
	"time"

	"github.com/tinoosan/workbench-core/pkg/types"
)

type Status string

const (
	StatusActive      Status = "active"
	StatusSucceeded   Status = "succeeded"
	StatusFailed      Status = "failed"
	StatusCanceled    Status = "canceled"
	StatusQuarantined Status = "quarantined"
)

type Record struct {
	TaskID     string
	Status     Status
	Attempts   int
	LeaseUntil time.Time
	UpdatedAt  time.Time

	Result *types.TaskResult
	Error  string
}

type ClaimResult struct {
	Claimed    bool
	Attempts   int
	LeaseUntil time.Time
}

type Store interface {
	// RecoverExpired marks tasks with expired leases as failed so they can be re-queued or inspected.
	RecoverExpired(ctx context.Context, now time.Time) error

	// Claim attempts to acquire a lease on a task. It returns Claimed=false if another worker holds the lease
	// or the task is terminal/quarantined.
	Claim(ctx context.Context, taskID string, ttl time.Duration) (ClaimResult, error)

	// Extend extends a task's lease (for long-running tasks).
	Extend(ctx context.Context, taskID string, ttl time.Duration) error

	// Complete records a task result and marks the task terminal.
	Complete(ctx context.Context, taskID string, result types.TaskResult) error

	// Quarantine marks a task quarantined and records an error.
	Quarantine(ctx context.Context, taskID string, errMsg string) error

	// Get returns the current record, if present.
	Get(ctx context.Context, taskID string) (Record, bool, error)
}

