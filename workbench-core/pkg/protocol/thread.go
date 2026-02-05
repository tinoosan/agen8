package protocol

import "time"

// ThreadID uniquely identifies a thread.
type ThreadID string

// Thread is the durable container for turns (maps to a workbench session).
type Thread struct {
	ID        ThreadID  `json:"id"`
	Title     string    `json:"title,omitempty"`
	CreatedAt time.Time `json:"createdAt"`

	// ActiveModel is the model identifier in use for this thread (e.g. "openai/gpt-5.2").
	ActiveModel string `json:"activeModel,omitempty"`

	// ActiveRunID identifies the current agent-instance run served by the app server.
	ActiveRunID RunID `json:"activeRunId,omitempty"`

	// Best-effort aggregates for client display.
	TotalTokensIn  int     `json:"totalTokensIn,omitempty"`
	TotalTokensOut int     `json:"totalTokensOut,omitempty"`
	TotalTokens    int     `json:"totalTokens,omitempty"`
	TotalCostUSD   float64 `json:"totalCostUsd,omitempty"`
}
