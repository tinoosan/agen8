package store

import (
	"context"

	"github.com/tinoosan/workbench-core/pkg/types"
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
