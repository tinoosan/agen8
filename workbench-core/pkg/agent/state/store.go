package state

import (
	"context"
	"errors"
	"fmt"
	"time"

	pkgstore "github.com/tinoosan/workbench-core/pkg/store"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// TaskReader queries tasks from storage.
type TaskReader interface {
	// GetTask retrieves a single task by ID.
	GetTask(ctx context.Context, taskID string) (types.Task, error)

	// GetRunStats aggregates task statistics for a run.
	// This is intended to be efficient (single query) and should not require loading
	// all tasks into memory.
	GetRunStats(ctx context.Context, runID string) (RunStats, error)

	// ListTasks queries tasks with filtering, sorting, and pagination.
	ListTasks(ctx context.Context, filter TaskFilter) ([]types.Task, error)

	// CountTasks returns the total count matching the filter (for pagination).
	CountTasks(ctx context.Context, filter TaskFilter) (int, error)
}

// TaskWriter creates and deletes tasks.
type TaskWriter interface {
	// CreateTask inserts a new task.
	CreateTask(ctx context.Context, task types.Task) error

	// DeleteTask removes a task (for cleanup/testing).
	DeleteTask(ctx context.Context, taskID string) error
}

// TaskUpdater modifies existing task data.
type TaskUpdater interface {
	// UpdateTask updates task fields (full replacement). Use this for general
	// field updates (goal, priority, metadata, etc.).
	UpdateTask(ctx context.Context, task types.Task) error

	// CompleteTask marks a task as succeeded/failed/canceled and records the result.
	CompleteTask(ctx context.Context, taskID string, result types.TaskResult) error
}

// TaskLeaser manages task execution leases (for distributed execution).
type TaskLeaser interface {
	// ClaimTask attempts to acquire a lease for execution.
	ClaimTask(ctx context.Context, taskID string, ttl time.Duration) error

	// ExtendLease extends the lease for a long-running task.
	ExtendLease(ctx context.Context, taskID string, ttl time.Duration) error

	// RecoverExpiredLeases finds tasks with expired leases and marks them failed.
	RecoverExpiredLeases(ctx context.Context) error
}

// TaskStore combines all task storage operations.
type TaskStore interface {
	TaskReader
	TaskWriter
	TaskUpdater
	TaskLeaser
}

// TaskFilter specifies query criteria for ListTasks.
type TaskFilter struct {
	SessionID string // Filter by session
	RunID     string // Filter by run
	Status    []types.TaskStatus
	FromDate  *time.Time // Created after this time
	ToDate    *time.Time // Created before this time

	// Pagination
	Limit  int // Max results (default: 50)
	Offset int // Skip N results

	// Sorting
	SortBy   string // Field name: "created_at", "finished_at" (or "completed_at"), "cost_usd"
	SortDesc bool
}

// RunStats captures aggregated statistics for tasks in a run.
type RunStats struct {
	TotalTasks    int
	Succeeded     int
	Failed        int
	TotalCost     float64
	TotalTokens   int
	TotalDuration time.Duration
}

var (
	// ErrTaskNotFound indicates the requested task does not exist.
	// Wraps pkgstore.ErrNotFound so callers can use errors.Is(err, pkgstore.ErrNotFound).
	ErrTaskNotFound  = fmt.Errorf("%w: task", pkgstore.ErrNotFound)
	ErrTaskClaimed   = errors.New("task already claimed by another worker")
	ErrTaskTerminal  = errors.New("task is in terminal state (completed/failed/canceled)")
	ErrInvalidFilter = errors.New("invalid task filter")
)
