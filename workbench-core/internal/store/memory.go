package store

import (
	"context"

	"github.com/tinoosan/workbench-core/internal/types"
)

// StagingArea manages the update.md staging file (agent-writable).
type StagingArea interface {
	GetUpdate(ctx context.Context) (string, error)
	SetUpdate(ctx context.Context, text string) error
	ClearUpdate(ctx context.Context) error
}

// CommitLogReader reads the commits.jsonl audit log (debug/audit).
type CommitLogReader interface {
	GetCommitLog(ctx context.Context) (string, error)
}

// CommitLogAppender appends one audit line to commits.jsonl.
type CommitLogAppender interface {
	AppendCommitLog(ctx context.Context, line types.MemoryCommitLine) error
}

// CommitLog composes reading and appending audit log lines.
type CommitLog interface {
	CommitLogReader
	CommitLogAppender
}

// MemoryContentReader reads committed run-scoped memory content.
type MemoryContentReader interface {
	GetMemory(ctx context.Context) (string, error)
}

// MemoryContentAppender appends committed run-scoped memory content.
type MemoryContentAppender interface {
	AppendMemory(ctx context.Context, text string) error
}

// MemoryVFSStore is the minimal store contract needed by the /memory VFS resource.
type MemoryVFSStore interface {
	MemoryContentReader
	StagingArea
	CommitLogReader
}

// MemoryCommitter is the minimal store contract needed to commit memory updates.
type MemoryCommitter interface {
	MemoryContentAppender
	CommitLogAppender
}

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
