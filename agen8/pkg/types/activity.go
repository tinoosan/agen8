package types

import (
	"strings"
	"time"
)

// ActivityStatus is the high-level state of an agent operation as shown in the Activity feed.
//
// Activities are UI-oriented projections of the event stream. They are persisted as a
// convenience index (see internal/store) to enable pagination and history browsing.
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
type Activity struct {
	ID string `json:"id"`

	// Kind is the operation type (fs_read, fs_write, shell_exec, ...).
	Kind string `json:"kind"`

	// Title is the short human label shown in the list (e.g. "Read /project/main.go").
	Title string `json:"title"`

	// Status is pending/ok/error.
	Status ActivityStatus `json:"status"`

	// Timing info: derived from host event receipt time (not wall-clock tool time).
	StartedAt  time.Time     `json:"startedAt"`
	FinishedAt *time.Time    `json:"finishedAt,omitempty"`
	Duration   time.Duration `json:"duration,omitempty"`

	// Inputs are the relevant operation inputs. Keep small and readable.
	From     string `json:"from,omitempty"`
	To       string `json:"to,omitempty"`
	Path     string `json:"path,omitempty"`
	MaxBytes string `json:"maxBytes,omitempty"`

	// For fs_write/fs_append, this is a small preview of the payload that was written.
	TextPreview   string `json:"textPreview,omitempty"`
	TextTruncated bool   `json:"textTruncated,omitempty"`
	TextRedacted  bool   `json:"textRedacted,omitempty"`
	TextIsJSON    bool   `json:"textIsJSON,omitempty"`
	TextBytes     string `json:"textBytes,omitempty"`

	// Outputs are small summaries suitable for a details panel.
	Ok            string `json:"ok,omitempty"`
	Error         string `json:"error,omitempty"`
	OutputPreview string `json:"outputPreview,omitempty"`

	// Response metadata.
	BytesLen  string `json:"bytesLen,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`

	// Data holds raw event data for extended display.
	Data map[string]string `json:"data,omitempty"`
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
	return strings.TrimSpace(a.TextPreview) != "" ||
		strings.TrimSpace(a.OutputPreview) != "" ||
		strings.TrimSpace(a.Error) != ""
}
