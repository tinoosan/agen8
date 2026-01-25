Here’s documentation for the `pkg/skills/resource.go` module, written with the new **code-documentation** skill. This is aimed at Workbench engineers who need to understand or extend how the `/skills` virtual filesystem is exposed.

---
# Skills Resource (`pkg/skills/resource.go`)

## Audience
Backend engineers or contributors working on Workbench’s VFS/skill integrations. You should read this before modifying how `/skills` behaves, adding new skill capabilities, or troubleshooting why your skill files aren’t visible.

## Purpose Summary
`SkillsResource` is the VFS-backed layer that exposes every skill directory to agents via the `/skills` mount. It translates VFS paths into the actual skill directories and files on disk, while enforcing safety checks, sorting, and refresh semantics so the agent always sees the current skill tree.

## High-Level Structure

```
Agent request                SkillsResource          Skill Manager         Real Skill Dir
---------------            ------------------      -----------------     --------------
/skills/                     -> List() / Read()   -> Find entry metadata  -> /skills/<name>/...
/skills/my-skill/file.md     -> read file content
/skills/my-skill/subdir/     -> list dir entries
```

### Key Components

| Component | Role |
|---|---|
| `SkillsResource` | Handles VFS requests (`List`, `Read`, `Write`, `Append`) and delegates to the manager. |
| `Manager` | Tracks registered skill directories, resolves writable paths, rescans when files change. |
| `parseSkillResourcePath` | Normalizes and validates agent-supplied paths to prevent escaping skill directories. |
| `listSkillDir`, `listNamespaces` | Enumerate directories/files, sort them lexicographically, and provide metadata for the VFS response. |

## Usage Patterns

| Action | VFS Path | Behavior |
|---|---|---|
| List available skills | `/skills` | `List("")` -> `SkillDirs()` returns sorted namespaces as directories. |
| Inspect a skill file | `/skills/<name>/SKILL.md` | `Read()` ensures the skill exists, unlocks safe path via `vfsutil.SafeJoinBaseDir`, and returns file bytes. |
| Modify skill file | `Write` or `Append` to `/skills/<name>/...` | Delegates to manager’s writable path (usually inside `/skills` workspace). After modification, `rescan()` refreshes the manager so the new content is immediately visible. |

#### Example
To show the README of the `explain-code` skill:
```
GET /skills/explain-code/README.md
-> SkillsResource.Read()
   -> manager.Get("explain-code")
   -> SafeJoinBaseDir(skill.Path, "README.md")
```

## Implementation Details & Gotchas

- **Safety first:** `vfsutil.SafeJoinBaseDir` ensures paths can’t escape the skill root, preventing directory traversal even if the agent submits something like `/skills/../secrets`.
- **Rescan after write:** Both `Write` and `Append` call `r.rescan()` to refresh the manager’s skill map. Without this, new files wouldn’t be reflected in subsequent reads or directory listings.
- **Sorting:** Directory listings are sorted lexicographically (`sort.Slice`), so the agent sees deterministic order.
- **Error surfacing:** Every filesystem error (e.g., `os.Stat`, `os.ReadFile`) is wrapped with contextual text to make troubleshooting simpler when viewing logs.

### Gotchas
- `parseSkillResourcePath` insists on two segments (`<skill>/<file>`); forgetting the filename results in a `"path must target a file under /skills/<name>"` error.
- Writable paths are resolved relative to the skill’s writable directory. For read-only/external skills, writes are rejected.
- Modifications require `fsutil.WriteFileAtomic` to avoid partial writes—this means the agent cannot append data atomically without abiding by this wrapper.

## Navigation Aids
| Method | Purpose |
|---|---|
| `List(path string)` | Entry point for any `/skills` directory listing. |
| `Read(path string)` | Reads files; requires validated path and returns file bytes. |
| `Write(path string, data []byte)` | Handles skill edits (used when the agent creates or updates skill files). |
| `Append(...)` | Appends to a skill file and triggers rescan. |
| `listSkillDir` | Internal helper to enumerate entries inside a skill directory. |
| `parseSkillResourcePath` | Normalizes user input and guards against invalid skill names. |

## Summary
`pkg/skills/resource.go` is your gateway between the `/skills` virtual mount and the physical skill directories. It enforces safe paths, deterministic listings, and auto-refreshing of the skill catalog after writes. When you update skill behavior or add new directories, follow this flow: register the skill via the manager, rely on `SkillsResource` for safe access, and remember to rescan so the agent sees the latest files.

Let me know if you’d like a diagram or further breakdown of the manager interactions!
