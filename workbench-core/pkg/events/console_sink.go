package events

import (
	"context"
	"encoding/json"
	"log"
)

// ConsoleSink prints compact JSON event lines to the terminal.
type ConsoleSink struct{}

func (ConsoleSink) Emit(_ context.Context, _ string, event Event) error {
	if !enabled(event.Console) {
		return nil
	}
	line := struct {
		Type    string            `json:"type"`
		Message string            `json:"message"`
		Data    map[string]string `json:"data,omitempty"`
	}{
		Type:    event.Type,
		Message: event.Message,
		Data:    event.Data,
	}
	b, err := json.Marshal(line)
	if err != nil {
		log.Printf("%s %s", event.Type, event.Message)
		return nil
	}
	log.Printf("%s", string(b))
	return nil
}
