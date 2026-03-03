package types

import "time"

const (
	MessageChannelInbox  = "inbox"
	MessageChannelOutbox = "outbox"
)

const (
	MessageKindTask      = "task"
	MessageKindUserInput = "user_input"
)

const (
	MessageStatusPending    = "pending"
	MessageStatusClaimed    = "claimed"
	MessageStatusAcked      = "acked"
	MessageStatusNacked     = "nacked"
	MessageStatusDeadletter = "deadletter"
)

// AgentMessage is the runtime message envelope used by the message bus.
// Task payload is optional; when Kind is task, TaskRef should be set.
type AgentMessage struct {
	MessageID string `json:"messageId"`

	IntentID      string `json:"intentId"`
	CorrelationID string `json:"correlationId"`
	CausationID   string `json:"causationId,omitempty"`
	Producer      string `json:"producer,omitempty"`

	ThreadID string `json:"threadId"`
	RunID    string `json:"runId,omitempty"`
	TeamID   string `json:"teamId,omitempty"`
	Channel  string `json:"channel"`
	Kind     string `json:"kind"`

	Body    map[string]any `json:"body,omitempty"`
	TaskRef string         `json:"taskRef,omitempty"`
	Task    *Task          `json:"task,omitempty"`

	Status     string     `json:"status"`
	LeaseOwner string     `json:"leaseOwner,omitempty"`
	LeaseUntil *time.Time `json:"leaseUntil,omitempty"`
	Attempts   int        `json:"attempts,omitempty"`
	VisibleAt  time.Time  `json:"visibleAt"`
	Priority   int        `json:"priority,omitempty"`

	Error    string         `json:"error,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`

	CreatedAt   *time.Time `json:"createdAt,omitempty"`
	UpdatedAt   *time.Time `json:"updatedAt,omitempty"`
	ProcessedAt *time.Time `json:"processedAt,omitempty"`
}
