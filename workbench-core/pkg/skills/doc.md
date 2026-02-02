Here’s documentation for the `pkg/skills/resource.go` module, written with the new **code-documentation** skill. This is aimed at Workbench engineers who need to understand or extend how the `/skills` virtual filesystem is exposed.

---
# Skills Resource (`pkg/skills/resource.go`)

## Audience
Backend engineers or contributors working on Workbench’s VFS/skill integrations. You should read this before modifying how `/skills` behaves, adding new skill capabilities, or troubleshooting why your skill files aren’t visible.

## Purpose Summary
`SkillsResource` is the VFS-backed layer that exposes every skill markdown file to agents via the `/skills` mount. It translates VFS paths into the actual skill files on disk, while enforcing safety checks, sorting, and refresh semantics so the agent always sees the current skill set.

## High-Level Structure

```
Agent request                SkillsResource          Skill Manager         Real Skill Path
---------------            ------------------      -----------------     --------------
/skills/                     -> List() / Read()   -> Find entry metadata  -> <dataDir>/skills/<name>/SKILL.md
/skills/my-skill/SKILL.md    -> read file content
```

### Key Components

| Component | Role |
|---|---|
| `SkillsResource` | Handles VFS requests (`List`, `Read`, `Write`, `Append`) and delegates to the manager. |
| `Manager` | Tracks registered skill files, resolves writable paths, rescans when files change. |
| `parseSkillFilename` | Normalizes and validates agent-supplied filenames to prevent traversal. |
| `listSkillFiles` | Enumerates skill files, sorts them, and provides metadata for the VFS response. |

## Usage Patterns

| Action | VFS Path | Behavior |
|---|---|---|
| List available skills | `/skills` | `List("")` -> returns sorted skill directories. |
| Inspect a skill file | `/skills/<name>/SKILL.md` | `Read()` ensures the skill exists and returns file bytes. |
| Modify skill file | `Write` or `Append` to `/skills/<name>/SKILL.md` | Writes to `<dataDir>/skills/<name>/SKILL.md`, then rescans so the update is immediately visible. |

#### Example
To read the `explain-code` skill:
```
GET /skills/explain-code/SKILL.md
-> SkillsResource.Read()
   -> manager.Get("explain-code")
```

## Implementation Details & Gotchas

- **Safety first:** `vfsutil.SafeJoinBaseDir` ensures paths can’t escape the skill root, preventing directory traversal even if the agent submits something like `/skills/../secrets`.
- **Rescan after write:** Both `Write` and `Append` call `r.rescan()` to refresh the manager’s skill map. Without this, new files wouldn’t be reflected in subsequent reads or directory listings.
- **Sorting:** Directory listings are sorted lexicographically (`sort.Slice`), so the agent sees deterministic order.
- **Error surfacing:** Every filesystem error (e.g., `os.Stat`, `os.ReadFile`) is wrapped with contextual text to make troubleshooting simpler when viewing logs.

### Gotchas
- Skills are directories under `/skills` and `SKILL.md` is the entrypoint.
- Writable paths are resolved under the configured writable root (`<dataDir>/skills`). For read-only/external skills, writes are rejected.
- Modifications require `fsutil.WriteFileAtomic` to avoid partial writes—this means the agent cannot append data atomically without abiding by this wrapper.

## Navigation Aids
| Method | Purpose |
|---|---|
| `List(path string)` | Entry point for any `/skills` directory listing. |
| `Read(path string)` | Reads files; requires validated path and returns file bytes. |
| `Write(path string, data []byte)` | Handles skill edits (used when the agent creates or updates skill files). |
| `Append(...)` | Appends to a skill file and triggers rescan. |
| `listSkillFiles` | Internal helper to enumerate discovered skill files. |
| `parseSkillFilename` | Normalizes user input and guards against invalid skill names. |

## Summary
`pkg/skills/resource.go` is your gateway between the `/skills` virtual mount and the physical skill files. It enforces safe paths, deterministic listings, and auto-refreshing of the skill catalog after writes. When you update skill behavior or add new skills, follow this flow: register the skill via the manager, rely on `SkillsResource` for safe access, and remember to rescan so the agent sees the latest files.

Let me know if you’d like a diagram or further breakdown of the manager interactions!
