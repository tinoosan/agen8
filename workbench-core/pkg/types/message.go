package types

import "time"

// Message is a lightweight envelope for agent-to-agent or agent-to-orchestrator communication.
// It is serialized to JSON and typically delivered via /outbox.
type Message struct {
	MessageID   string            `json:"messageId"`
	FromRunID   string            `json:"fromRunId"`
	ToRunID     string            `json:"toRunId,omitempty"`
	TaskID      string            `json:"taskId,omitempty"`
	Kind        string            `json:"kind,omitempty"`
	Title       string            `json:"title,omitempty"`
	Body        string            `json:"body,omitempty"`
	Attachments []string          `json:"attachments,omitempty"`
	CreatedAt   *time.Time        `json:"createdAt,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}
