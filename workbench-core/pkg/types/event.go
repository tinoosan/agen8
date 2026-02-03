package types

import (
	"time"

	"github.com/google/uuid"
)

// Event represents a single recorded action or state change within a workbench run.
// Events are persisted in an append-only JSONL format for auditability.
type EventRecord struct {
	// EventID is a unique identifier for this specific event (e.g., "event-<uuid>").
	EventID string `json:"eventId"`
	// RunID is the identifier of the run this event belongs to.
	RunID string `json:"runId"`
	// Timestamp is when the event was recorded.
	Timestamp time.Time `json:"timestamp"`
	// Type is the category of the event (e.g., "action_start", "result").
	Type string `json:"type"`
	// Message is a human-readable description of the event.
	Message string `json:"message"`
	// Data contains additional structured metadata related to the event.
	Data map[string]string `json:"data,omitempty"`
	// Origin identifies the source of the event (e.g. "agent", "user", "env").
	Origin string `json:"origin,omitempty"`

	// Emission control (not persisted)
	StoreData map[string]string `json:"-"`
	Console   *bool             `json:"-"`
	Store     *bool             `json:"-"`
	History   *bool             `json:"-"`
}

// NewEventRecord initializes a new EventRecord with a unique ID and the current timestamp.
func NewEventRecord(runID, eventType, message string, data map[string]string) EventRecord {
	return EventRecord{
		EventID:   "event-" + uuid.NewString(),
		RunID:     runID,
		Timestamp: time.Now(),
		Type:      eventType,
		Message:   message,
		Data:      data,
	}
}
