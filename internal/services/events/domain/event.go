package domain

import (
	"errors"
	"fmt"
	"strings"

	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

var ErrRunIDRequired = errors.New("runID is required")

// EventFilter specifies filtering and pagination for event queries.
type EventFilter struct {
	RunID string // required: filter by run

	// Pagination
	Limit     int   // max results (default: 100, 0 = use default)
	Offset    int   // skip N events (for page-based pagination)
	AfterSeq  int64 // return events with seq > AfterSeq (for cursor-based pagination)
	BeforeSeq int64 // return events with seq < BeforeSeq (for reverse pagination)

	// Filtering by event type
	Types []string // filter to specific event types (empty = all types)

	// Sorting
	SortDesc bool // true = newest first (DESC), false = oldest first (ASC, default)

	// Ops log filters
	Severities []string // filter by severity: info, warning, error
	Categories []string // filter by category: task, agent, llm, system, tools
	Search     string   // server-side text search on message/type
	Origin     string   // filter by origin/role
}

// TailedEvent is a single event from the tail stream with its next offset.
type TailedEvent struct {
	Event      types.EventRecord
	NextOffset int64
}

// ValidateEvent checks that an event has the required fields.
func ValidateEvent(e types.EventRecord) error {
	if strings.TrimSpace(e.Type) == "" {
		return fmt.Errorf("event type is required")
	}
	if strings.TrimSpace(e.Message) == "" {
		return fmt.Errorf("event message is required")
	}
	return nil
}

// Enabled returns whether a flag pointer is enabled (nil defaults to true).
func Enabled(ptr *bool) bool {
	if ptr == nil {
		return true
	}
	return *ptr
}

// EventSeverity derives severity from event type and data.
func EventSeverity(eventType string, data map[string]string) string {
	if sev, ok := data["severity"]; ok && sev != "" {
		return sev
	}
	lower := strings.ToLower(eventType)
	if strings.Contains(lower, "error") || strings.Contains(lower, "fail") ||
		strings.Contains(lower, "quarantine") || strings.Contains(lower, "deadlock") {
		return "error"
	}
	if strings.Contains(lower, "warning") || strings.Contains(lower, "retry") ||
		strings.Contains(lower, "deferred") || strings.Contains(lower, "missing") {
		return "warning"
	}
	return "info"
}

// EventCategory derives category from event type prefix.
func EventCategory(eventType string) string {
	lower := strings.ToLower(eventType)
	switch {
	case strings.HasPrefix(lower, "task."):
		return "task"
	case strings.HasPrefix(lower, "agent.") || strings.HasPrefix(lower, "subagent.") || strings.HasPrefix(lower, "run."):
		return "agent"
	case strings.HasPrefix(lower, "llm.") || strings.HasPrefix(lower, "model") || strings.HasPrefix(lower, "cost."):
		return "llm"
	case strings.HasPrefix(lower, "audit.") || strings.HasPrefix(lower, "tool."):
		return "tools"
	case lower == "read_file" || lower == "write_file" || lower == "code_compile":
		return "tools"
	default:
		return "system"
	}
}
