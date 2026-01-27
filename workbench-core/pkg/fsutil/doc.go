// Package fsutil provides filesystem utility functions for the workbench.
//
// It offers helpers for path manipulation, sanitization, and safe atomic writes that
// are used throughout runtime initialization or when recording artifacts.
//
// # Helpers
//
//   - `GetSkillsDir`/`GetDataDir`: Centralizes how the runtime locates standardized
//     directories within `config.DataDir` so other code does not duplicate path logic.
//   - `AtomicFileWrite`: Ensures files are written via temporary files + rename to
//     avoid partial writes.
//   - `Paths` helpers: Validate that runtime-generated paths stay within expected roots.
//
// These utilities are stable and intended for reuse wherever writes touch runtime
// storage or skills data.
package fsutil
