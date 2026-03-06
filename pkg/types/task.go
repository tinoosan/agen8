package types

import (
	"strings"
	"time"
)

// TaskStatus represents the lifecycle state of a Task.
type TaskStatus string

const (
	TaskStatusPending       TaskStatus = "pending"
	TaskStatusActive        TaskStatus = "active"
	TaskStatusReviewPending TaskStatus = "review_pending"
	TaskStatusSucceeded     TaskStatus = "succeeded"
	TaskStatusFailed        TaskStatus = "failed"
	TaskStatusCanceled      TaskStatus = "canceled"
	TaskStatusDelegated     TaskStatus = "delegated"  // Workers spawned, parent execution ended
	TaskStatusResumed       TaskStatus = "resumed"    // All callbacks processed, ready for finalization
)

// Task is the canonical DB-backed unit of work for autonomous agents.
type Task struct {
	TaskID            string         `json:"taskId"`
	SessionID         string         `json:"sessionId,omitempty"`
	RunID             string         `json:"runId,omitempty"`
	SourceTeamID      string         `json:"sourceTeamId,omitempty"`
	DestinationTeamID string         `json:"destinationTeamId,omitempty"`
	TeamID            string         `json:"teamId,omitempty"`
	AssignedRole      string         `json:"assignedRole,omitempty"`
	AssignedToType    string         `json:"assignedToType,omitempty"` // team | role | agent
	AssignedTo     string       `json:"assignedTo,omitempty"`     // team id, role name, or agent id
	ClaimedByAgentID string     `json:"claimedByAgentId,omitempty"`
	RoleSnapshot string         `json:"roleSnapshot,omitempty"`
	TaskKind     string         `json:"taskKind,omitempty"`
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

func (t *Task) NormalizeTeamFields() {
	if t == nil {
		return
	}
	t.SourceTeamID = strings.TrimSpace(t.SourceTeamID)
	t.DestinationTeamID = strings.TrimSpace(t.DestinationTeamID)
	t.TeamID = strings.TrimSpace(t.TeamID)
	if t.DestinationTeamID == "" && t.TeamID != "" {
		t.DestinationTeamID = t.TeamID
	}
	t.TeamID = t.DestinationTeamID
}

// TaskResult captures the outcome of a task.
// It is persisted in SQLite and surfaced through task/artifact views.
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
