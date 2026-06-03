package app

import (
	"context"
	"log/slog"

	"github.com/tinoosan/agen8-mcp-server/internal/services/events/domain"
)

// consoleDataValueLimit is the maximum character length for any single value
// in the Data map when rendering console output. Values exceeding this limit
// are truncated with a "…" suffix. This prevents model-produced text
// (reasoning summaries, task results, patch previews, etc.) from flooding
// the daemon log.
const consoleDataValueLimit = 200

// ConsoleSink logs events through slog so they match the daemon's configured
// log format (text on TTY, JSON otherwise).
type ConsoleSink struct{}

func (c ConsoleSink) Emit(_ context.Context, msg Message) error {
	event := msg.Payload
	if !domain.Enabled(event.Console) {
		return nil
	}
	data := truncateDataValues(event.Data, consoleDataValueLimit)
	attrs := make([]any, 0, 2+len(data)*2)
	attrs = append(attrs, "type", event.Type)
	for k, v := range data {
		attrs = append(attrs, k, v)
	}
	slog.Info(event.Message, attrs...)
	return nil
}

// truncateDataValues returns a copy of data with all string values truncated
// to maxLen runes. If a value is truncated, "…" is appended.
func truncateDataValues(data map[string]string, maxLen int) map[string]string {
	if len(data) == 0 {
		return data
	}
	out := make(map[string]string, len(data))
	for k, v := range data {
		if len(v) > maxLen {
			// Truncate at rune boundary to avoid splitting multi-byte chars.
			runes := []rune(v)
			if len(runes) > maxLen {
				v = string(runes[:maxLen]) + "…"
			}
		}
		out[k] = v
	}
	return out
}
