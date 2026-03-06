package events

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// ConsoleSink prints compact JSON event lines to the terminal.
// If Writer is nil, output goes to os.Stderr.
type ConsoleSink struct {
	Writer io.Writer
}

func (c ConsoleSink) w() io.Writer {
	if c.Writer != nil {
		return c.Writer
	}
	return os.Stderr
}

func (c ConsoleSink) Emit(_ context.Context, msg Message) error {
	event := msg.Payload
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
		fmt.Fprintf(c.w(), "%s %s\n", event.Type, event.Message)
		return nil
	}
	fmt.Fprintf(c.w(), "%s\n", string(b))
	return nil
}
