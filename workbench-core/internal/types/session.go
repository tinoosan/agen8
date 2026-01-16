package types

import (
	"time"

	"github.com/google/uuid"
)

// Session represents a durable container that groups multiple runs.
//
// # High-level model
//
// - A Run is one agent execution thread (one loop, one tool runner, one workspace).
// - A Session groups runs that belong to the same "conversation"/workspace timeline.
//
// Workspaces are run-scoped (data/runs/<runId>/workspace), but a session can still provide:
// - shared history (append-only provenance across runs)
// - navigation (which run to resume from)
//
// Sub-agents are modeled as runs spawned by another run:
// - child run has ParentRunID set
// - both runs share the same SessionID
type Session struct {
	// SessionID is the unique identifier for this session (e.g., "sess-<uuid>").
	SessionID string `json:"sessionId"`

	// Title is an optional short label for the session.
	Title string `json:"title,omitempty"`

	// CreatedAt is when the session was created.
	CreatedAt *time.Time `json:"createdAt"`

	// CurrentRunID is the run the host considers "active" for resume/navigation.
	CurrentRunID string `json:"currentRunId,omitempty"`

	// Runs is an ordered list of run IDs created in this session.
	//
	// Runs are stored separately under data/runs/<runId>/; this list is an index.
	Runs []string `json:"runs,omitempty"`
}

// NewSession creates a new session with a unique ID.
func NewSession(title string) Session {
	now := time.Now()
	return Session{
		SessionID: "sess-" + uuid.NewString(),
		Title:     title,
		CreatedAt: &now,
		Runs:      nil,
	}
}
