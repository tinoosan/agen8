package tui

import (
	"strings"
	"time"
)

// ActivityStatus is the high-level state of an agent operation as shown in the Activity feed.
//
// Activities are UI-only projections of the event stream. They do not change the agent loop
// contract and they are not persisted as a primary record (that remains the on-disk events
// and history logs).
type ActivityStatus string

const (
	ActivityPending ActivityStatus = "pending"
	ActivityOK      ActivityStatus = "ok"
	ActivityError   ActivityStatus = "error"
)

// Activity is a UI-friendly aggregated view of "what the agent is doing".
//
// A single Activity is created from an op request event (agent.op.request) and updated when the
// corresponding op response event arrives (agent.op.response).
//
// Telemetry (turn counters, bytes, cursors, etc.) is intentionally excluded from Activity so the
// feed remains user-facing. When telemetry is enabled, the UI may surface additional fields in
// the details panel, but Activity.Kind/Title remain stable.
type Activity struct {
	ID string

	// Kind is the operation type (fs.read, fs.write, tool.run, ...).
	Kind string

	// Title is the short human label shown in the list (e.g. "Read /tools/builtin.bash").
	Title string

	// Status is pending/ok/error.
	Status ActivityStatus

	// Timing info: derived from host event receipt time (not wall-clock tool time).
	StartedAt  time.Time
	FinishedAt *time.Time
	Duration   time.Duration

	// Inputs are the relevant operation inputs. Keep small and readable.
	From      string
	To        string
	Path      string
	MaxBytes  string
	ToolID    string
	ActionID  string
	InputJSON string // sanitized one-line JSON from the event (tool.run only)
	Command   string // effective command line (tool.run only)

	// For fs.write/fs.append, this is a small preview of the payload that was written.
	// The host provides this as an event field so the UI can show "what changed"
	// without needing to read the file back.
	TextPreview   string
	TextTruncated bool
	TextRedacted  bool
	TextIsJSON    bool
	TextBytes     string // telemetry-only

	// Outputs are small summaries suitable for a details panel.
	CallID        string
	Ok            string
	Error         string
	OutputPreview string // e.g. stdout/stderr preview for builtin.bash when available

	// Response metadata (telemetry only).
	BytesLen  string
	Truncated bool
}

func (a Activity) ShortStatus() string {
	switch a.Status {
	case ActivityOK:
		return "✓"
	case ActivityError:
		return "✗"
	default:
		return "…"
	}
}

func (a Activity) HasDetails() bool {
	return strings.TrimSpace(a.InputJSON) != "" ||
		strings.TrimSpace(a.TextPreview) != "" ||
		strings.TrimSpace(a.OutputPreview) != "" ||
		strings.TrimSpace(a.Error) != "" ||
		strings.TrimSpace(a.CallID) != ""
}
