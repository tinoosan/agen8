package types

import (
	"time"

	"github.com/google/uuid"
)

// EventRecord is the canonical append-only operational log for a run.
//
// Event records capture raw runtime occurrences such as host operations, logs,
// and state changes. They are the source material from which narrower read models
// such as agent space entries may be derived.
type EventRecord struct {
	// EventID is a unique identifier for this specific event (e.g., "event-<uuid>").
	EventID EventID `json:"eventId"`
	// RunID is the identifier of the run this event belongs to.
	RunID RunID `json:"runId"`
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
func NewEventRecord(runID RunID, eventType, message string, data map[string]string) EventRecord {
	return EventRecord{
		EventID:   EventID("event-" + uuid.NewString()),
		RunID:     runID,
		Timestamp: time.Now(),
		Type:      eventType,
		Message:   message,
		Data:      data,
	}
}
