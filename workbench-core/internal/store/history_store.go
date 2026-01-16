package store

import "context"

// HistoryStore is the host-side storage interface backing the VFS mount "/history".
//
// History is a verifiable, append-only source of truth for raw interactions between:
//   - users
//   - agents
//   - the environment/host
//
// Unlike /trace (curated agent-facing events) and /memory (curated host-governed notes),
// history is immutable and append-only.
//
// The agent can read history via VFS, but cannot write to it.
type HistoryStore interface {
	// ReadAll returns the full history JSONL bytes (0+ lines).
	ReadAll(ctx context.Context) ([]byte, error)

	// AppendLine appends one JSON object as a JSONL line.
	//
	// line should be a single JSON object. The store will ensure a trailing newline.
	AppendLine(ctx context.Context, line []byte) error
}
