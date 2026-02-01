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
	// RunID is the unique identifier for this run (e.g., "run-<uuid>").
	RunID string `json:"runId"`
	// SessionID is the session this run belongs to (e.g., "sess-<uuid>").
	//
	// A session groups multiple runs and provides shared, append-only history across them.
	SessionID string `json:"sessionId,omitempty"`
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

	// TotalTokensIn is the cumulative input tokens consumed by this run.
	TotalTokensIn int `json:"totalTokensIn,omitempty"`
	// TotalTokensOut is the cumulative output tokens consumed by this run.
	TotalTokensOut int `json:"totalTokensOut,omitempty"`
	// TotalTokens is the cumulative input + output tokens for this run.
	TotalTokens int `json:"totalTokens,omitempty"`
	// TotalCostUSD is the cumulative estimated cost for this run.
	TotalCostUSD float64 `json:"totalCostUsd,omitempty"`

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

	// Context budgets applied by the PromptUpdater per step.
	MaxTraceBytes   int `json:"maxTraceBytes,omitempty"`
	MaxMemoryBytes  int `json:"maxMemoryBytes,omitempty"`

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

	// ApprovalsMode records whether the agent required approvals during this run.
	ApprovalsMode string `json:"approvalsMode,omitempty"`
}

// NewRun initializes a new Run instance with a unique ID and the given parameters.
// It sets the initial status to StatusRunning and the start time to now.
func NewRun(goal string, maxBytesForContext int, sessionID string) Run {
	runID := "run-" + uuid.NewString()
	now := time.Now()
	return Run{
		RunID:              runID,
		SessionID:          sessionID,
		Goal:               goal,
		Status:             StatusRunning,
		StartedAt:          &now,
		MaxBytesForContext: maxBytesForContext,
		Error:              nil,
	}

}
