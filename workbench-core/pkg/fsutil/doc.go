// Package fsutil provides filesystem utility functions for the workbench.
//
// It offers helpers for path manipulation, sanitization, and safe atomic writes that
// are used throughout runtime initialization or when recording artifacts.
//
// # Helpers
//
//   - `GetAgentsSkillsDir`: Resolves the open-standard skills path under
//     `~/.agents/skills` for runtime mounts.
//   - `AtomicFileWrite`: Ensures files are written via temporary files + rename to
//     avoid partial writes.
//   - `Paths` helpers: Validate that runtime-generated paths stay within expected roots.
//
// These utilities are stable and intended for reuse wherever writes touch runtime
// storage or skills data.
package fsutil
