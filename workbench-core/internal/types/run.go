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

	// Runtime captures host/runtime configuration used when executing this run.
	//
	// This is primarily for reproducibility and debugging ("why did the agent behave that way?").
	// It is optional and may be nil for older runs.
	Runtime *RunRuntimeConfig `json:"runtime,omitempty"`
}

// RunRuntimeConfig is host/runtime configuration persisted into run.json for reproducibility.
//
// This is not "agent memory". It is host configuration: budgets, model, and other knobs
// that materially affect run behavior.
type RunRuntimeConfig struct {
	// DataDir is the workbench data directory used by the host process.
	DataDir string `json:"dataDir,omitempty"`

	// Model is the configured LLM model identifier for this run.
	Model string `json:"model,omitempty"`

	// MaxSteps is the maximum number of agent loop steps allowed per user turn.
	MaxSteps int `json:"maxSteps,omitempty"`

	// Context budgets applied by the ContextUpdater per step.
	MaxTraceBytes   int `json:"maxTraceBytes,omitempty"`
	MaxMemoryBytes  int `json:"maxMemoryBytes,omitempty"`
	MaxProfileBytes int `json:"maxProfileBytes,omitempty"`

	// RecentHistoryPairs controls how much recent /history is injected on resume.
	RecentHistoryPairs int `json:"recentHistoryPairs,omitempty"`

	// IncludeHistoryOps controls whether environment/host ops are included when injecting
	// session history into the system prompt.
	IncludeHistoryOps bool `json:"includeHistoryOps,omitempty"`

	// Pricing configuration used to estimate per-turn cost (USD per 1M tokens).
	PriceInPerMTokensUSD  float64 `json:"priceInPerMTokensUsd,omitempty"`
	PriceOutPerMTokensUSD float64 `json:"priceOutPerMTokensUsd,omitempty"`

	// Reasoning configuration (best-effort; provider/model dependent).
	ReasoningEffort  string `json:"reasoningEffort,omitempty"`
	ReasoningSummary string `json:"reasoningSummary,omitempty"`
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
