package protocol

import "time"

// TurnID uniquely identifies a turn.
type TurnID string

// TurnStatus represents the lifecycle state of a turn.
type TurnStatus string

const (
	TurnStatusPending    TurnStatus = "pending"
	TurnStatusInProgress TurnStatus = "in_progress"
	TurnStatusCompleted  TurnStatus = "completed"
	TurnStatusFailed     TurnStatus = "failed"
	TurnStatusCanceled   TurnStatus = "canceled"
)

// Turn is a user -> agent execution cycle within a thread.
type Turn struct {
	ID        TurnID     `json:"id"`
	ThreadID  ThreadID   `json:"threadId"`
	Status    TurnStatus `json:"status"`
	CreatedAt time.Time  `json:"createdAt"`

	// StepCount is an optional UI-friendly counter of agent steps completed.
	StepCount int `json:"stepCount,omitempty"`

	// Error is present when Status is failed.
	Error *TurnError `json:"error,omitempty"`
}

// TurnError describes a turn failure.
type TurnError struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}
