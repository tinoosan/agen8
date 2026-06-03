package types

// EscalationData carries structured information when a parent escalates a sub-agent task.
type EscalationData struct {
	Reason         string   `json:"reason"`
	AttemptSummary string   `json:"attemptSummary"`
	Recommendation string   `json:"recommendation"`
	Artifacts      []string `json:"artifacts,omitempty"`
	OriginalGoal   string   `json:"originalGoal"`
	RetryCount     int      `json:"retryCount"`
	SourceRunID    string   `json:"sourceRunId"`
	SourceTaskID   string   `json:"sourceTaskId"`
}
