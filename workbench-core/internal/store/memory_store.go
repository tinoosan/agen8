package store

// MemoryStore is the host-side storage interface backing the virtual VFS mount "/memory".
//
// Memory is intentionally simple and stable:
//   - /memory/memory.md       (committed memory, host-managed; agent can read)
//   - /memory/update.md       (staging file, agent can write; host evaluates + commits)
//   - /memory/commits.jsonl   (audit log, host-managed; readable for debugging)
//
// This interface allows you to swap storage backends later (disk, sqlite, etc)
// without changing the agent loop, VFS ops, or evaluation policy.
type MemoryStore interface {
	MemoryContentReader
	MemoryContentAppender
	StagingArea
	CommitLog
}
