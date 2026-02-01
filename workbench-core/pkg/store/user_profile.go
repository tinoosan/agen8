package store

import "context"

// UserProfileContentReader reads committed global user profile content.
type UserProfileContentReader interface {
	GetUserProfile(ctx context.Context) (string, error)
}

// UserProfileContentAppender appends committed global user profile content.
type UserProfileContentAppender interface {
	AppendUserProfile(ctx context.Context, text string) error
}

// UserProfileVFSStore is the minimal store contract needed by the /user_profile VFS resource.
type UserProfileVFSStore interface {
	UserProfileContentReader
	StagingArea
	CommitLogReader
}

// UserProfileCommitter is the minimal store contract needed to commit user profile updates.
type UserProfileCommitter interface {
	UserProfileContentAppender
	CommitLogAppender
}

// UserProfileStore is the host-side storage interface backing the virtual VFS mount "/user_profile".
//
// User profile is global (shared across runs and sessions) and is intended for durable user facts
// and preferences (e.g. timezone, writing style, birthday).
//
// VFS layout exposed by the corresponding resource:
//   - /user_profile/user_profile.md     (committed profile, host-managed; agent can read)
//   - /user_profile/update.md           (staging file, agent can write/append; host evaluates + commits)
//   - /user_profile/commits.jsonl       (audit log, host-managed; readable for debugging)
//
// This mirrors the /memory contract but with a different scope and lifecycle.
type UserProfileStore interface {
	UserProfileContentReader
	UserProfileContentAppender
	StagingArea
	CommitLog
}

