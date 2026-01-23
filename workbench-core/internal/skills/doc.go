package skills

// Package skills manages the lifecycle of agent-facing skills that live under
// the /skills virtual mount. It wires together discovery (Manager), exposure
// (SkillsResource), and abstraction points (SkillsProvider) so contributors can
// reason about what files are visible to agents, how skills are written, and how
// future extensions can reuse the same metadata.
//
// # Key Concepts
//
// ## Manager
// - Responsible for scanning each configured root directory for skill
//   directories. Each candidate directory must contain a `SKILL.md`, whose YAML
//   front-matter is parsed to populate the Skill metadata (name, description,
//   and filesystem path). The first-discovered directory wins when multiple roots
//   share the same name, so ordering matters when roots overlap.
// - Maintains an `entries` map keyed by the directory name. `Entries()` returns a
//   sorted slice of `SkillEntry` values so callers can present deterministic
//   listings, while `Get()` performs fast lookups during reads or lists.
// - Handles writable skills. When `WritableRoot` is set, the manager sanitizes
//   skill names (no dots, slashes, or empty names) and resolves writable paths
//   using `vfsutil.SafeJoinBaseDir` to avoid escaping the configured directory.
//   All writes go through `resolveWritablePath`, which also enforces non-empty
//   relative segments (e.g., you cannot write to `/skills/foo/../bar`).
// - Provides helper methods such as `AddSkill` (writes a new `SKILL.md` atomically
//   via `fsutil.WriteFileAtomic`) and `Scan` to refresh the registry after any
//   change.
//
// ## SkillsProvider
// - An interface (currently just `Entries() []SkillEntry`) that lets other
//   components—like `ContextConstructor`—consume the cached skill metadata
//   without depending directly on the manager implementation. This keeps skill
//   enumeration pluggable and testable.
//
// ## SkillsResource
// - Acts as the VFS bridge for `/skills`. It exposes `List`, `Read`, `Write`, and
//   `Append` operations. Each method first ensures the manager exists and then
//   trims, splits, and validates the incoming path. Paths are normalized with
//   `strings.Trim` and `SplitN`, requiring at least `<skill>/<file>` to proceed.
// - `List` checks whether the request targets the root (empty path) or a
//   specific skill directory. Root listings call `listNamespaces`, which simply
//   converts the manager's directory keys into sorted `vfs.Entry` instances. For
//   nested paths, `listSkillDir` renders the file entries, fetching metadata such
//   as size and mod time, and preserving deterministic order via `sort.Slice`.
// - `Read` resolves the skill entry with `manager.Get`, joins the requested file
//   path with `vfsutil.SafeJoinBaseDir`, and streams the file contents with
//   `os.ReadFile`. Errors are annotated to clarify whether the issue occurred
//   during lookup, safe join, or file read.
// - `Write` and `Append` both use `parseSkillResourcePath` to normalize the
//   path, `resolveWritablePath` to compute an atomic destination under
//   `WritableRoot`, and `fsutil` helpers for atomic writes/appends. `Write` uses
//   `fsutil.WriteFileAtomic` to avoid partial data, while `Append` opens the file
//   with O_APPEND semantics. Both operations call `rescan()` so the manager
//   immediately reflects the new content.
//
// ## Path Safety Helpers
// - `parseSkillResourcePath` enforces the `<skill>/<file>` shape. It disallows
//   single-segment paths (e.g., `/skills/foo`) and trims extra slashes. The
//   skill name is passed through `sanitizeSkillName` to reject `.`/`..` or names
//   containing path separators, guarding against directory traversal.
// - `sanitizeSkillName` is reused by the manager when resolving writable roots to
//   ensure consistency between reads and writes.
//
// ## Workflow Summary
// 1. Configure skill roots and optional writable directory via the manager.
// 2. Call `Manager.Scan()` to discover `SKILL.md` files and populate the cache.
// 3. The VFS (`SkillsResource`) exposes `/skills` requests, delegating to the
//    manager for lookups and resolving paths safely before performing filesystem
//    work.
// 4. When an agent writes or appends skill files, the manager resolves a
//    secure writable path, persists atomically, and rescans the registry to
//    update the VFS view.
//
// ## Navigation Tips
// - To add new skill directories, place them under one of the configured roots
//   and ensure `SKILL.md` includes the proper front matter (`name`,
//   `description`). The manager will pick them up on the next scan.
// - Any modification performed through the `/skills` VFS automatically triggers
//   a rescan, so there is no stale metadata after writes.
// - The provider abstraction keeps documentation tooling (like context
//   constructors) decoupled from the concrete manager, enabling mocks or
//   alternate discovery strategies in the future.
//
// This document should help contributors understand the interplay between the
// manager, provider, and resource so they can safely extend skill discovery,
// add writable directories, or surface skill metadata to other systems without
// diving into the implementation details each time.

//
// Additional Insights:
// - `SkillsResource` list and read operations layer on top of `Manager.Get` and
//   `listNamespaces`. This means any change in how the manager indexes directories
//   or handles conflicting names is immediately reflected in the virtual mount.
// - Sorting, metadata collection, and error wrapping in `listSkillDir` keep the
//   VFS consumer from needing to know the underlying filesystem quirks.
// - For writable skills, always ensure `WritableRoot` is set; otherwise writes
//   fail early with a clear error message. The Manager centralizes this check so
//   both CLI commands and the VFS share the same expectations.
