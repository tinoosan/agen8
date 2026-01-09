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
)

// RunStatuses maps status strings to their typed RunStatus values.
var RunStatuses = map[string]RunStatus{
	string(StatusRunning): StatusRunning,
	string(StatusDone):    StatusDone,
	string(StatusFailed):  StatusFailed,
}

// Run represents the state and metadata of a single workbench execution.
type Run struct {
	// RunId is the unique identifier for this run (e.g., "run-<uuid>").
	RunId string `json:"runId"`
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
func NewRun(goal string, maxBytesForContext int) Run {
	runId := "run-" + uuid.NewString()
	now := time.Now()
	return Run{
		RunId:              runId,
		Goal:               goal,
		Status:             StatusRunning,
		StartedAt:          &now,
		MaxBytesForContext: maxBytesForContext,
		Error:              nil,
	}

}
