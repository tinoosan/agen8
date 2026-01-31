package types

import "time"

// Task is a unit of work delivered via /inbox for a single autonomous agent.
type Task struct {
	TaskID    string         `json:"taskId"`
	Goal      string         `json:"goal"`
	Inputs    map[string]any `json:"inputs,omitempty"`
	Priority  int            `json:"priority,omitempty"` // 0 = highest
	Status    string         `json:"status,omitempty"`   // pending, active, succeeded, failed, canceled
	CreatedAt *time.Time     `json:"createdAt,omitempty"`
	StartedAt *time.Time     `json:"startedAt,omitempty"`
	CompletedAt *time.Time   `json:"completedAt,omitempty"`
	Error     string         `json:"error,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// TaskResult captures the outcome of a task.
// It is serialized to JSON and delivered via /outbox.
type TaskResult struct {
	TaskID      string     `json:"taskId"`
	Status      string     `json:"status"`
	Summary     string     `json:"summary,omitempty"`
	Artifacts   []string   `json:"artifacts,omitempty"`
	Error       string     `json:"error,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}
