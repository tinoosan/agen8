package types

import (
	"time"

	"github.com/google/uuid"
)

// RunStatus represents the current state of a workbench run.
type RunStatus string

const (
	// StatusRunning indicates the run is currently in progress.
	StatusRunning RunStatus = "running"
	// StatusDone indicates the run has completed successfully.
	StatusDone RunStatus = "done"
	// StatusFailed indicates the run stopped due to an error.
	StatusFailed RunStatus = "failed"
	// StatusCanceled indicates the run was interrupted by the user or host.
	//
	// This is a terminal state used when a run is intentionally stopped (e.g. SIGINT / Ctrl-C).
	StatusCanceled RunStatus = "canceled"
)

// RunStatuses maps status strings to their typed RunStatus values.
var RunStatuses = map[string]RunStatus{
	string(StatusRunning):  StatusRunning,
	string(StatusDone):     StatusDone,
	string(StatusFailed):   StatusFailed,
	string(StatusCanceled): StatusCanceled,
}

// Run represents the state and metadata of a single workbench execution.
type Run struct {
	// RunId is the unique identifier for this run (e.g., "run-<uuid>").
	RunId string `json:"runId"`
	// SessionID is the session this run belongs to (e.g., "sess-<uuid>").
	//
	// A session groups multiple runs and provides shared, append-only history across them.
	SessionID string `json:"sessionId,omitempty"`
	// ParentRunID is the run that spawned this run (empty for root runs).
	//
	// This is the basis for "sub-agents": a sub-agent is a child run in the same session.
	ParentRunID string `json:"parentRunId,omitempty"`
	// Goal is the high-level description of what this run is trying to accomplish.
	Goal string `json:"goal"`
	// Status is the current operating state of the run.
	Status RunStatus `json:"status"`
	// StartedAt is the timestamp when the run was initialized.
	StartedAt *time.Time `json:"startedAt"`
	// FinishedAt is the timestamp when the run reached a terminal state.
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	// MaxBytesForContext is the maximum token/byte limit allowed for agent context.
	MaxBytesForContext int `json:"maxBytesForContext"`
	// Error contains the failure message if the run status is StatusFailed.
	Error *string `json:"error,omitempty"`
}

// NewRun initializes a new Run instance with a unique ID and the given parameters.
// It sets the initial status to StatusRunning and the start time to now.
func NewRun(goal string, maxBytesForContext int, sessionID string, parentRunID string) Run {
	runId := "run-" + uuid.NewString()
	now := time.Now()
	return Run{
		RunId:              runId,
		SessionID:          sessionID,
		ParentRunID:        parentRunID,
		Goal:               goal,
		Status:             StatusRunning,
		StartedAt:          &now,
		MaxBytesForContext: maxBytesForContext,
		Error:              nil,
	}

}
