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

	// ActiveModel is the model identifier that should be used when (re)starting runs
	// in this session, unless overridden by host config or a runtime /model command.
	//
	// This is session-scoped on purpose:
	// - It makes "resume session" deterministic.
	// - It keeps model provenance stable across sub-agent runs.
	//
	// Example: "openai/gpt-5.2".
	ActiveModel string `json:"activeModel,omitempty"`

	// CreatedAt is when the session was created.
	CreatedAt *time.Time `json:"createdAt"`

	// CurrentRunID is the run the host considers "active" for resume/navigation.
	CurrentRunID string `json:"currentRunId,omitempty"`

	// CurrentGoal is the current user-facing objective for this session.
	//
	// This is a host-maintained field used to make "resume session" coherent:
	// the host injects it into the system prompt so the agent can continue without
	// rereading the entire history.
	CurrentGoal string `json:"currentGoal,omitempty"`

	// Plan is an optional short plan for the current goal.
	//
	// This enables "plan mode" patterns where a planner agent writes a plan and
	// then delegates to sub-agent runs. The host should treat this as advisory state
	// and keep provenance in /history.
	Plan string `json:"plan,omitempty"`

	// Summary is a compact, host-maintained recap of what happened so far.
	//
	// This is NOT a replacement for /history (the source of truth). It is a
	// bounded, human+agent friendly digest to reduce token cost on resume.
	Summary string `json:"summary,omitempty"`

	// UpdatedAt is the last time the session state was updated by the host.
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`

	// Runs is an ordered list of run IDs created in this session.
	//
	// Runs are stored separately under data/runs/<runId>/; this list is an index.
	Runs []string `json:"runs,omitempty"`

	// HistoryCursor is the host-maintained cursor used for incremental history retrieval.
	//
	// History is session-scoped, so this cursor persists across runs to support resume,
	// constructor/context building, and UI polling.
	//
	// Cursor is treated as opaque at the module boundary.
	HistoryCursor string `json:"historyCursor,omitempty"`
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
