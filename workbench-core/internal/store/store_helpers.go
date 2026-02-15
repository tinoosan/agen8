package store

import (
	"encoding/json"
	"time"

	pkgstore "github.com/tinoosan/workbench-core/pkg/store"
	"github.com/tinoosan/workbench-core/pkg/types"
)

func normalizeHistoryLimits(maxBytes, limit int) (int, int) {
	if maxBytes <= 0 {
		maxBytes = 16 * 1024
	}
	if limit <= 0 {
		limit = 200
	}
	return maxBytes, limit
}

func normalizeTraceLimits(maxBytes, limit, defaultMax, defaultLimit int) (int, int) {
	if maxBytes <= 0 {
		maxBytes = defaultMax
	}
	if limit <= 0 {
		limit = defaultLimit
	}
	return maxBytes, limit
}

func parseTraceEvent(raw string) (pkgstore.TraceEvent, bool) {
	var ev types.EventRecord
	if err := json.Unmarshal([]byte(raw), &ev); err != nil {
		return pkgstore.TraceEvent{}, false
	}
	return pkgstore.TraceEvent{
		Timestamp: ev.Timestamp.UTC().Format(time.RFC3339Nano),
		Type:      ev.Type,
		Message:   ev.Message,
		Data:      ev.Data,
	}, true
}
