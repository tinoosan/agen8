package state

import (
	"context"
	"errors"
	"fmt"
	"time"

	pkgstore "github.com/tinoosan/agen8/pkg/store"
	"github.com/tinoosan/agen8/pkg/types"
)

// Task storage contracts
//
// Task IDs
//   - Most task IDs are canonicalized to the form "task-<id>" by ingress (webhook/RPC/task tool ingress).
//   - Heartbeat tasks use "heartbeat-..." IDs and are exempt from "task-" normalization.
//   - Ingestion may preserve an unprefixed/original ID in task metadata as "originalTaskId".
//
// Errors
//   - ErrTaskNotFound wraps pkg/store.ErrNotFound so callers can use errors.Is(err, pkg/store.ErrNotFound).
//   - Implementations should return ErrTaskNotFound (or an error wrapping it) when a task is missing.
//   - Lease operations may return ErrTaskClaimed / ErrTaskTerminal for expected contention/terminal states.
//
// Leases
//   - ClaimTask should be treated as "acquire lease" and should fail with ErrTaskClaimed if held elsewhere.
//   - ExtendLease should be idempotent for the current lease holder and should not revive terminal tasks.
//   - RecoverExpiredLeases should mark tasks failed when their lease has elapsed without completion.
//
// State machine invariants (coordination / re-claim prevention)
//   - Terminal (succeeded, failed, canceled) must never transition back to pending.
//   - CompleteTask: only allowed when current status is active, pending, or delegated; never overwrite terminal.
//   - ResumeTask: only allowed when status is delegated; idempotent when already resumed or not delegated.
//   - ReleaseLease: only allowed when status is active; must not move terminal tasks to pending.
//
// Filtering/pagination
//   - ListTasks should honor Limit/Offset for pagination and SortBy/SortDesc for ordering.
//   - Invalid filters/sort keys should return ErrInvalidFilter.
//   - SortBy values are implementation-defined but typically include: created_at, finished_at (or completed_at), cost_usd.

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

	// ReleaseLease releases the lease on an active task so it goes back to Pending (e.g. for yield-for-callback).
	ReleaseLease(ctx context.Context, taskID string) error

	// DelegateTask transitions a task from active to delegated (workers spawned, parent execution ended).
	DelegateTask(ctx context.Context, taskID string) error

	// ResumeTask transitions a task from delegated to pending (all callbacks processed, ready for finalization).
	ResumeTask(ctx context.Context, taskID string) error

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

// MessageStore persists runtime messages for bus-driven processing.
type MessageStore interface {
	PublishMessage(ctx context.Context, msg types.AgentMessage) (types.AgentMessage, error)
	ClaimNextMessage(ctx context.Context, filter MessageClaimFilter, ttl time.Duration, consumerID string) (types.AgentMessage, error)
	AckMessage(ctx context.Context, messageID string, result MessageAckResult) error
	NackMessage(ctx context.Context, messageID string, reason string, retryAt *time.Time) error
	RequeueExpiredClaims(ctx context.Context) error
	GetMessage(ctx context.Context, messageID string) (types.AgentMessage, error)
	ListMessages(ctx context.Context, filter MessageFilter) ([]types.AgentMessage, error)
	CountMessages(ctx context.Context, filter MessageFilter) (int, error)
}

// TaskFilter specifies query criteria for ListTasks.
type TaskFilter struct {
	SessionID      string // Filter by session
	RunID          string // Filter by run
	TeamID         string // Filter by team
	AssignedRole   string // Filter by assigned role
	AssignedToType string // Filter by assignee type: team|role|agent
	AssignedTo     string // Filter by assignee id/name
	ClaimedBy      string // Filter by claimed_by agent id
	TaskKind       string // Filter by task_kind
	View           string // Logical view: inbox|outbox
	UnassignedOnly bool   // Filter tasks where assigned_role is empty
	Status         []types.TaskStatus
	FromDate       *time.Time // Created after this time
	ToDate         *time.Time // Created before this time

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

// MessageClaimFilter constrains claim/list operations for bus consumers.
type MessageClaimFilter struct {
	ThreadID string
	RunID    string
	TeamID   string
	Channel  string
	Kinds    []string
}

// MessageFilter specifies query criteria for list/count operations.
type MessageFilter struct {
	ThreadID string
	RunID    string
	TeamID   string
	Channel  string
	Kinds    []string
	Statuses []string

	Limit  int
	Offset int

	SortBy   string // created_at|visible_at|processed_at|priority
	SortDesc bool
}

// MessageAckResult captures optional terminal metadata when acknowledging a message.
type MessageAckResult struct {
	Status    string
	Error     string
	Metadata  map[string]any
	Processed *time.Time
}

var (
	// ErrTaskNotFound indicates the requested task does not exist.
	// Wraps pkgstore.ErrNotFound so callers can use errors.Is(err, pkgstore.ErrNotFound).
	ErrTaskNotFound  = fmt.Errorf("%w: task", pkgstore.ErrNotFound)
	ErrTaskClaimed   = errors.New("task already claimed by another worker")
	ErrTaskTerminal  = errors.New("task is in terminal state (completed/failed/canceled)")
	ErrInvalidFilter = errors.New("invalid task filter")

	// ErrMessageNotFound indicates the requested message does not exist.
	// Wraps pkgstore.ErrNotFound so callers can use errors.Is(err, pkgstore.ErrNotFound).
	ErrMessageNotFound  = fmt.Errorf("%w: message", pkgstore.ErrNotFound)
	ErrMessageClaimed   = errors.New("message already claimed by another worker")
	ErrMessageTerminal  = errors.New("message is in terminal state")
	ErrInvalidMsgFilter = errors.New("invalid message filter")
)
