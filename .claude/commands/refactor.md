You are executing architectural refactoring work for agen8. Every issue you work on is part of a larger cleanup effort. You MUST understand the full picture before touching any code — refactors that fix one problem while creating two new ones are worse than doing nothing.

## Setup

**Stay current with dev:**
```
git fetch origin dev
git log HEAD..origin/dev --oneline
```
If there are new commits, fast-forward: `git pull --ff-only origin dev`.

**Read these first — they are the law:**
- `CLAUDE.md` — critical rules, zero-tolerance policies
- `AGENTS.md` — no-fallback error policy, backend completion rules
- `docs/architecture/` — all files (if they exist)

## The architectural vision

agen8 is a multi-agent orchestration platform. The codebase must be clean enough that:

1. **An agent working on agen8 can add a feature by modifying 1-2 files, not 8-15.** If extension points aren't localized, every feature introduction is error-prone.
2. **An internal SDK could be extracted from `pkg/`.** If it can't, the boundaries are wrong. `pkg/` = interfaces, domain types, SDK surface. `internal/` = implementations, services, wiring.
3. **Package boundaries are attention boundaries.** Smaller, focused packages mean less context per change — critical when LLMs author the code.
4. **The compiler enforces as much as possible.** Convention-based rules get violated. Import restrictions, interface satisfaction, and construction-time validation don't.

## Before starting any issue

### 1. Read the issue AND its parent epic
```
gh issue view <number>
```
If the issue references a parent epic, read that too. Understand where this issue fits in the larger plan — what comes before it, what it unblocks, what the end state looks like.

### 2. Research the full blast radius

Before changing ANY code:
- Read every file you will modify, fully
- For every type you plan to move: `grep -rn 'TypeName' pkg/ internal/ --include='*.go'` — find ALL usages
- For every import you plan to change: find ALL importers of that package
- For every interface you plan to modify: find ALL implementations and ALL callers
- Map the dependency chain — who imports what, who uses what
- Check for type assertions (`\.(*ConcreteType)`) that will break
- If moving a package: verify no circular imports result

**Do NOT start coding until you have the full picture.**

### 3. State your plan before executing

Write down:
- What files you will modify
- What types/imports/interfaces change
- What the expected blast radius is
- How you will verify nothing broke

## Rules for architectural refactors

### Moving types between packages
- Shared types used across boundaries → `pkg/types/`
- NEVER duplicate a type across packages
- NEVER create wrapper types or re-export packages
- Update ALL references in a single PR — no "fix the rest later"

### Moving packages between `pkg/` and `internal/`
- Move shared types OUT first (to `pkg/types/`), then move the package
- Verify the build after each move, not just at the end
- Use find-and-replace on the full import path — don't miss any

### Changing interfaces
- Interface lives with the CONSUMER, not the provider (standard Go DI)
- Adding methods to an interface: grep for ALL implementations, update them all
- If an interface is satisfied implicitly: check with `var _ Interface = (*Impl)(nil)` after changes

### Splitting files
- Same package, same behavior — just file boundaries
- Functions that call each other frequently stay in the same file
- Group by concern, not by alphabetical order
- Each file should have a single reason to change

### General
- `internal/app/` (or its sub-packages) is the ONLY place that wires concrete implementations together
- No silent fallbacks, no nil guards that return nil, no `_ =` error discards (CLAUDE.md zero tolerance)
- Test doubles must capture all arguments (no `_` for domain params)

## Verification — run BEFORE committing

```bash
# Build check — catches import cycles and missing types
go build ./...

# Full test suite
go test ./... -count=1 -timeout 120s
go vet ./...
```

Frontend (if applicable):
```bash
cd web && npm run lint && npm run build && npm test
```

If ANY check fails, fix before committing. A refactor that breaks the build or tests is not a refactor.

## Commit and PR lifecycle

### Commit in a worktree
Main checkout stays on `dev`. Never switch it.

```
git stash
git worktree add .worktrees/refactor/<slug> -b refactor/<slug> dev
```

Apply changes, then:
```
git add <specific files>
git commit -m "refactor: <what and why>"
git push -u origin refactor/<slug>
```

### PR
```
gh pr create --base dev --title "refactor: ..." --body "Closes #<issue>. Part of #<epic>."
gh pr checks <PR> --watch
gh pr merge <PR> --squash --delete-branch
```

### Cleanup after merge
```
cd <project-root>
git worktree remove --force .worktrees/refactor/<slug>
git fetch --prune
git pull --ff-only origin dev
```

## Anti-patterns — things that have gone wrong before

1. **Fixing one violation while creating new ones.** Always run boundary checks after changes.
2. **Moving a type without updating all references.** Grep the ENTIRE codebase, not just the files you think use it.
3. **Splitting files along wrong seams.** If you're unsure where a function belongs, leave it and move on.
4. **"Temporary" violations.** There is no temporary. If a change introduces a new illegal import, fix it now or revert.
5. **Massive PRs.** If a refactor touches more than 15 files, break it into smaller steps that each compile and pass tests independently.
6. **Refactoring without reading.** If you haven't read every file in the blast radius, you are not ready to refactor.

## When stuck

If the same approach fails 3 times, stop. Write what you learned to the `Lessons learned` section of `AGENTS.md`. Then try a different approach or ask the user.

## Start now

List open refactoring issues:
```
gh issue list --label refactor --state open --json number,title --jq 'sort_by(.number) | .[] | "#\(.number) — \(.title)"'
```

Pick the lowest-numbered issue (or work the user's specified issues). Read the issue, read its epic, research the blast radius, then execute.
