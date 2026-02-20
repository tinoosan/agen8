package protocol

import "time"

// ThreadID uniquely identifies a thread.
type ThreadID string

// Thread is the durable container for turns (maps to a agen8 session).
type Thread struct {
	ID        ThreadID  `json:"id"`
	Title     string    `json:"title,omitempty"`
	CreatedAt time.Time `json:"createdAt"`

	// ActiveModel is the model identifier in use for this thread (e.g. "openai/gpt-5.2").
	ActiveModel string `json:"activeModel,omitempty"`
	// ActiveReasoningEffort is the active reasoning effort for the selected model.
	ActiveReasoningEffort string `json:"activeReasoningEffort,omitempty"`
	// ActiveReasoningSummary is the active reasoning summary level for the selected model.
	ActiveReasoningSummary string `json:"activeReasoningSummary,omitempty"`

	// ActiveRunID identifies the current agent-instance run served by the app server.
	ActiveRunID RunID `json:"activeRunId,omitempty"`

	// Best-effort aggregates for client display.
	InputTokens  int     `json:"inputTokens,omitempty"`
	OutputTokens int     `json:"outputTokens,omitempty"`
	TotalTokens  int     `json:"totalTokens,omitempty"`
	CostUSD      float64 `json:"costUSD,omitempty"`
}
