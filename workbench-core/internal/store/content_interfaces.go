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

// ProfileContentReader reads committed global profile content.
type ProfileContentReader interface {
	GetProfile(ctx context.Context) (string, error)
}

// ProfileContentAppender appends committed global profile content.
type ProfileContentAppender interface {
	AppendProfile(ctx context.Context, text string) error
}

// MemoryVFSStore is the minimal store contract needed by the /memory VFS resource.
type MemoryVFSStore interface {
	MemoryContentReader
	StagingArea
	CommitLogReader
}

// ProfileVFSStore is the minimal store contract needed by the /profile VFS resource.
type ProfileVFSStore interface {
	ProfileContentReader
	StagingArea
	CommitLogReader
}

// MemoryCommitter is the minimal store contract needed to commit memory updates.
type MemoryCommitter interface {
	MemoryContentAppender
	CommitLogAppender
}

// ProfileCommitter is the minimal store contract needed to commit profile updates.
type ProfileCommitter interface {
	ProfileContentAppender
	CommitLogAppender
}

