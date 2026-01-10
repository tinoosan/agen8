package types

import (
	"time"

	"github.com/google/uuid"
)

// Event represents a single recorded action or state change within a workbench run.
// Events are persisted in an append-only JSONL format for auditability.
type Event struct {
	// EventId is a unique identifier for this specific event (e.g., "event-<uuid>").
	EventId string `json:"eventId"`
	// RunId is the identifier of the run this event belongs to.
	RunId string `json:"runId"`
	// Timestamp is when the event was recorded.
	Timestamp time.Time `json:"timestamp"`
	// Type is the category of the event (e.g., "action_start", "result").
	Type string `json:"type"`
	// Message is a human-readable description of the event.
	Message string `json:"message"`
	// Data contains additional structured metadata related to the event.
	Data map[string]string `json:"data,omitempty"`
}

// NewEvent initializes a new Event instance with a unique ID and the current timestamp.
func NewEvent(runId, eventType, message string, data map[string]string) Event {
	return Event{
		EventId:   "event-" + uuid.NewString(),
		RunId:     runId,
		Timestamp: time.Now(),
		Type:      eventType,
		Message:   message,
		Data:      data,
	}
}
