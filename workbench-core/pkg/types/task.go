package types

import (
	"strings"
	"time"
)

// TaskStatus represents the lifecycle state of a Task.
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusActive    TaskStatus = "active"
	TaskStatusSucceeded TaskStatus = "succeeded"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCanceled  TaskStatus = "canceled"
)

// Task is a unit of work delivered via /inbox for a single autonomous agent.
type Task struct {
	TaskID       string         `json:"taskId"`
	SessionID    string         `json:"sessionId,omitempty"`
	RunID        string         `json:"runId,omitempty"`
	TeamID       string         `json:"teamId,omitempty"`
	AssignedRole string         `json:"assignedRole,omitempty"`
	CreatedBy    string         `json:"createdBy,omitempty"`
	Goal         string         `json:"goal"`
	Inputs       map[string]any `json:"inputs,omitempty"`
	Priority     int            `json:"priority,omitempty"` // 0 = highest
	Status       TaskStatus     `json:"status,omitempty"`   // pending, active, succeeded, failed, canceled
	CreatedAt    *time.Time     `json:"createdAt,omitempty"`
	StartedAt    *time.Time     `json:"startedAt,omitempty"`
	CompletedAt  *time.Time     `json:"completedAt,omitempty"`
	UpdatedAt    *time.Time     `json:"updatedAt,omitempty"`
	LeaseUntil   *time.Time     `json:"leaseUntil,omitempty"`
	Attempts     int            `json:"attempts,omitempty"`
	Error        string         `json:"error,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	Summary      string         `json:"summary,omitempty"`
	Artifacts    []string       `json:"artifacts,omitempty"`

	// Best-effort LLM usage totals for the task (populated after completion).
	InputTokens     int     `json:"inputTokens,omitempty"`
	OutputTokens    int     `json:"outputTokens,omitempty"`
	TotalTokens     int     `json:"totalTokens,omitempty"`
	CostUSD         float64 `json:"costUSD,omitempty"`
	DurationSeconds int     `json:"durationSeconds,omitempty"`
}

// SortTime returns the time used for ordering tasks (CreatedAt if set, else zero time).
func (t Task) SortTime() time.Time {
	if t.CreatedAt != nil {
		return t.CreatedAt.UTC()
	}
	return time.Time{}
}

// NormalizeStatus lowercases and defaults the task status.
func (t *Task) NormalizeStatus() {
	if t == nil {
		return
	}
	status := strings.ToLower(strings.TrimSpace(string(t.Status)))
	if status == "" {
		t.Status = TaskStatusPending
		return
	}
	t.Status = TaskStatus(status)
}

// TaskResult captures the outcome of a task.
// It is serialized to JSON and delivered via /outbox.
type TaskResult struct {
	TaskID      string     `json:"taskId"`
	Status      TaskStatus `json:"status"`
	Summary     string     `json:"summary,omitempty"`
	Artifacts   []string   `json:"artifacts,omitempty"`
	Error       string     `json:"error,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`

	// Best-effort LLM usage totals for the task (if available).
	InputTokens  int     `json:"inputTokens,omitempty"`
	OutputTokens int     `json:"outputTokens,omitempty"`
	TotalTokens  int     `json:"totalTokens,omitempty"`
	CostUSD      float64 `json:"costUSD,omitempty"`
}
