package types

import "time"

// Task is a unit of work assigned to a run.
// It is serialized to JSON and delivered via /inbox.
type Task struct {
	TaskID          string            `json:"taskId"`
	AssignedToRunID string            `json:"assignedToRunId"`
	Goal            string            `json:"goal"`
	Inputs          map[string]any    `json:"inputs,omitempty"`
	WaitFor         []string          `json:"waitFor,omitempty"`
	Priority        string            `json:"priority,omitempty"`
	Status          string            `json:"status,omitempty"`
	CreatedAt       *time.Time        `json:"createdAt,omitempty"`
	StartedAt       *time.Time        `json:"startedAt,omitempty"`
	CompletedAt     *time.Time        `json:"completedAt,omitempty"`
	Error           string            `json:"error,omitempty"`
	Deadline        *time.Time        `json:"deadline,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

// TaskResult captures the outcome of a task.
// It is serialized to JSON and delivered via /outbox.
type TaskResult struct {
	TaskID      string     `json:"taskId"`
	RunID       string     `json:"runId"`
	Status      string     `json:"status"`
	Summary     string     `json:"summary,omitempty"`
	Artifacts   []string   `json:"artifacts,omitempty"`
	Error       string     `json:"error,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}
