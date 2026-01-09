package types

import (
	"time"

	"github.com/google/uuid"
)

type Event struct {
	EventId string `json:"eventId"`
	RunId string `json:"runId"`
	Timestamp time.Time `json:"timestamp"`
	Type string  `json:"type"`
	Message string `json:"message"`
	Data map[string]string `json:"data,omitempty"`
}

func NewEvent(runId, eventType, message string, data map[string]string) Event {
	return Event{
		EventId: "event-" + uuid.NewString(),
		RunId: runId,
		Timestamp: time.Now(),
		Type: eventType,
		Message: message,
		Data: data,
	}
}
