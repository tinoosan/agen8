package protocol

import (
	"time"
)

type Task struct {
	ID               string    `json:"id"`
	ThreadID         ThreadID  `json:"threadId"`
	RunID            RunID     `json:"runId"`
	TeamID           string    `json:"teamId,omitempty"`
	TaskKind         string    `json:"taskKind,omitempty"`
	AssignedToType   string    `json:"assignedToType,omitempty"`
	AssignedTo       string    `json:"assignedTo,omitempty"`
	AssignedRole     string    `json:"assignedRole,omitempty"`
	ClaimedByAgentID string    `json:"claimedByAgentId,omitempty"`
	RoleSnapshot     string    `json:"roleSnapshot,omitempty"`
	Goal             string    `json:"goal"`
	Status           string    `json:"status"`
	Summary          string    `json:"summary,omitempty"`
	Error            string    `json:"error,omitempty"`
	Artifacts        []string  `json:"artifacts,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
	CompletedAt      time.Time `json:"completedAt,omitempty"`
}

type TaskListParams struct {
	ThreadID ThreadID `json:"threadId"`
	View     string   `json:"view,omitempty"` // inbox|outbox
	TeamID   string   `json:"teamId,omitempty"`
	Assignee string   `json:"assignee,omitempty"`
	Limit    int      `json:"limit,omitempty"`
}

type TaskListResult struct {
	Tasks []Task `json:"tasks"`
}

type TaskCreateParams struct {
	ThreadID       ThreadID `json:"threadId"`
	Goal           string   `json:"goal"`
	TaskKind       string   `json:"taskKind,omitempty"`
	AssignedToType string   `json:"assignedToType,omitempty"`
	AssignedTo     string   `json:"assignedTo,omitempty"`
	AssignedRole   string   `json:"assignedRole,omitempty"`
	Priority       int      `json:"priority,omitempty"`
}

type TaskCreateResult struct {
	Task Task `json:"task"`
}

type TaskClaimParams struct {
	ThreadID ThreadID `json:"threadId"`
	TaskID   string   `json:"taskId"`
	AgentID  string   `json:"agentId,omitempty"`
}

type TaskClaimResult struct {
	Task Task `json:"task"`
}

type TaskCompleteParams struct {
	ThreadID  ThreadID `json:"threadId"`
	TeamID    string   `json:"teamId,omitempty"`
	TaskID    string   `json:"taskId"`
	Summary   string   `json:"summary,omitempty"`
	Artifacts []string `json:"artifacts,omitempty"`
	Error     string   `json:"error,omitempty"`
	Status    string   `json:"status,omitempty"` // succeeded|failed|canceled
}

type TaskCompleteResult struct {
	Task Task `json:"task"`
}
