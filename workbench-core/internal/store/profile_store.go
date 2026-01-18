package store

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
