package store

import (
	"context"

	"github.com/tinoosan/workbench-core/internal/types"
)

// ProfileStore is the host-side storage interface backing the virtual VFS mount "/profile".
//
// Profile is global (shared across runs and sessions) and is intended for durable user facts
// and preferences (e.g. timezone, writing style, birthday).
//
// VFS layout exposed by the corresponding resource:
//   - /profile/profile.md     (committed profile, host-managed; agent can read)
//   - /profile/update.md      (staging file, agent can write; host evaluates + commits)
//   - /profile/commits.jsonl  (audit log, host-managed; readable for debugging)
//
// This mirrors the /memory contract but with a different scope and lifecycle.
type ProfileStore interface {
	// GetProfile returns the full committed profile.md contents.
	GetProfile(ctx context.Context) (string, error)

	// AppendProfile appends a committed profile entry.
	//
	// The host decides what to append (policy, formatting, timestamps).
	AppendProfile(ctx context.Context, text string) error

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
