package types

// MemoryCommitLine is a single audit record appended to /memory/commits.jsonl.
//
// The audit log is immutable and append-only within a run. This enables debugging:
// "what did the agent try to write to memory, and why was it accepted/rejected?"
//
// Storage note:
// - The host writes these records (not the agent).
// - The default DiskMemoryStore persists them under data/memory/commits.jsonl.
type MemoryCommitLine struct {
	Timestamp string `json:"timestamp"`
	Model     string `json:"model,omitempty"`
	Turn      int    `json:"turn"`

	// Scope indicates where this memory proposal was written (e.g. "memory", "profile").
	// It is optional for backward compatibility.
	Scope string `json:"scope,omitempty"`
	// SessionID is the session in which the proposal occurred (optional).
	SessionID string `json:"sessionId,omitempty"`
	// RunID is the run in which the proposal occurred (optional).
	RunID string `json:"runId,omitempty"`

	Accepted bool   `json:"accepted"`
	Reason   string `json:"reason"`

	Bytes  int    `json:"bytes"`
	SHA256 string `json:"sha256"`
}
