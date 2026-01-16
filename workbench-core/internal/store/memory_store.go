package store

import (
	"context"

	"github.com/tinoosan/workbench-core/internal/types"
)

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
	// GetMemory returns the full committed memory.md contents.
	GetMemory(ctx context.Context) (string, error)

	// AppendMemory appends a committed memory entry.
	//
	// The host decides what to append (policy, formatting, timestamps).
	AppendMemory(ctx context.Context, text string) error

	// GetUpdate returns the current update.md contents.
	GetUpdate(ctx context.Context) (string, error)

	// SetUpdate replaces update.md contents.
	SetUpdate(ctx context.Context, text string) error

	// ClearUpdate clears update.md (equivalent to SetUpdate("").
	ClearUpdate(ctx context.Context) error

	// GetCommitLog returns the full commits.jsonl contents.
	GetCommitLog(ctx context.Context) (string, error)

	// AppendCommitLog appends one JSONL audit record to commits.jsonl.
	AppendCommitLog(ctx context.Context, line types.MemoryCommitLine) error
}
