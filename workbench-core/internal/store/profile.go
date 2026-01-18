package store

import "context"

// ProfileContentReader reads committed global profile content.
type ProfileContentReader interface {
	GetProfile(ctx context.Context) (string, error)
}

// ProfileContentAppender appends committed global profile content.
type ProfileContentAppender interface {
	AppendProfile(ctx context.Context, text string) error
}

// ProfileVFSStore is the minimal store contract needed by the /profile VFS resource.
type ProfileVFSStore interface {
	ProfileContentReader
	StagingArea
	CommitLogReader
}

// ProfileCommitter is the minimal store contract needed to commit profile updates.
type ProfileCommitter interface {
	ProfileContentAppender
	CommitLogAppender
}

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
	ProfileContentReader
	ProfileContentAppender
	StagingArea
	CommitLog
}

