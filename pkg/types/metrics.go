package types

// TokenUsage captures flat token and cost accounting shared across several
// domain and transport projections.
type TokenUsage struct {
	InputTokens  int     `json:"inputTokens,omitempty"`
	OutputTokens int     `json:"outputTokens,omitempty"`
	TotalTokens  int     `json:"totalTokens,omitempty"`
	CostUSD      float64 `json:"costUSD,omitempty"`
	// CacheReadInputTokens is the cumulative count of input tokens served from
	// the provider's prompt cache across all turns. A non-zero value indicates
	// the provider reused a cached prefix and those tokens were not freshly
	// computed (typically billed at a lower rate or not billed at all).
	CacheReadInputTokens int `json:"cacheReadInputTokens,omitempty"`
}

func (u TokenUsage) Add(other TokenUsage) TokenUsage {
	return TokenUsage{
		InputTokens:          u.InputTokens + other.InputTokens,
		OutputTokens:         u.OutputTokens + other.OutputTokens,
		TotalTokens:          u.TotalTokens + other.TotalTokens,
		CostUSD:              u.CostUSD + other.CostUSD,
		CacheReadInputTokens: u.CacheReadInputTokens + other.CacheReadInputTokens,
	}
}

func (u TokenUsage) IsZero() bool {
	return u.InputTokens == 0 && u.OutputTokens == 0 && u.TotalTokens == 0 && u.CostUSD == 0 && u.CacheReadInputTokens == 0
}
