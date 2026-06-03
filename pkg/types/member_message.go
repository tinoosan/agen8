package types

import (
	"time"

	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
)

const (
	MessageChannelInbox  = "inbox"
	MessageChannelOutbox = "outbox"
)

const (
	MessageKindTask      = "task"
	MessageKindUserInput = "user_input"
	MessageKindInform    = "inform"
	MessageKindQuery     = "query"
	MessageKindAck       = "ack"
	MessageKindResponse  = "response"
	MessageKindTimeout   = "timeout"
	MessageKindSystem    = "system"
)

const (
	MessageStatusQueued   = "queued"
	MessageStatusConsumed = "consumed"
)

// MemberMessage is the queue-backed bus envelope that carries a routed message
// with an optional TaskRef to the backing task record.
//
// Routing fields (AssignedToType, AssignedTo, AssignedRole) live on the
// envelope itself so the claim query never needs to JOIN the tasks table.
type MemberMessage struct {
	MessageID MessageID `json:"messageId"`

	IntentID      string `json:"intentId"`
	CorrelationID string `json:"correlationId"`
	CausationID   string `json:"causationId,omitempty"`
	Producer      string `json:"producer,omitempty"`

	SpaceID        spacedomain.SpaceID `json:"spaceId"`
	ActorMemberID  string              `json:"actorMemberId,omitempty"`
	TargetMemberID string              `json:"targetMemberId,omitempty"`
	RunID          RunID               `json:"runId,omitempty"`
	Channel        string              `json:"channel"`
	Kind           string              `json:"kind"`

	// Payload.
	Message *Message `json:"message,omitempty"`
	TaskRef string   `json:"taskRef,omitempty"`

	// Self-contained routing — no task table JOIN needed.
	AssignedToType string `json:"assignedToType,omitempty"` // "member", "role", "space"
	AssignedTo     string `json:"assignedTo,omitempty"`
	AssignedRole   string `json:"assignedRole,omitempty"`

	Body map[string]any `json:"body,omitempty"`

	// Delivery mechanics. Messages are immutable events: they move from queued
	// to consumed exactly once.
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
