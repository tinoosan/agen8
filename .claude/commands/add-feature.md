Implement open feature requests one at a time. For each feature, the full lifecycle is below. Do not deviate.

## Philosophy — read this first

**Small, surgical, shipped.** A feature is not measured by how many lines it touches. A single well-placed line of code that unlocks a capability is a feature. 30 small shipped features that each work end-to-end beat one sprawling epic every time. Think in sprints: each feature should be shippable in isolation, build on the last, and leave foundations for the next.

**Full-stack features, not layers.** There is no "backend PR then frontend PR." If the feature requires both backend and frontend changes to be useful, that is ONE feature. Ship it as one unit. A backend service with no UI is not a feature — it's half a feature. A UI that calls a nonexistent endpoint is not a feature — it's a broken page. Identify the smallest vertical slice that delivers value to the user and implement that.

**End-to-end correctness over unit coverage.** A unit test that passes while the system falls apart is worse than no test — it gives false confidence. Every feature needs three classes of tests where applicable:

1. **Behavior tests** — Does the feature work as the user expects? Drive the real entry points (RPC handler, UI component with real store, CLI command). These are the most important tests.
2. **Regression tests** — Does the feature break existing behavior? Test the edges: nil inputs, concurrent access, missing dependencies, invalid state transitions. These catch bugs before users do.
3. **Performance/benchmark tests** — Does the feature perform acceptably? Only where applicable: hot paths, batch operations, queries that touch large datasets, rendering lists. Use `b.Run()` in Go, measure render times in React.

A feature without a behavior test is not done. The other two classes are required where they apply.

## Setup

Read these files before starting. They are the law — not guidelines, not suggestions.
- `CLAUDE.md`
- `AGENTS.md`
- `docs/ai-development-workflow.md`

List open feature issues:
```
gh issue list --label enhancement --state open --json number,title,createdAt --jq 'sort_by(.number) | .[] | "#\(.number) — \(.title) (created \(.createdAt[:10]))"'
```

Pick the lowest-numbered issue. If the user specifies issues, work those in the given order.

## Governing workflow

All work follows `docs/ai-development-workflow.md`. Key rules that apply throughout:
- One agent, one task, one branch, one worktree (Section 2)
- Declare intended subsystems before broad edits (Section 1, step 2)
- Search the codebase before implementing — reuse existing logic (Section 8)
- PRs must link the governing issue and use `pull_request_template.md` (Section 4)
- All PRs target `dev` (Section 4)
- Never push directly to `dev` or `main` (Section 4)
- Partial implementations must be removed, not hidden behind TODOs (Section 8)
- No silent fallbacks — see no-fallback error policy (Section 8)

## Sync with origin/dev — run BEFORE every issue and BETWEEN steps

Other agents may be merging bug fixes and refactors into `dev` while you work.
Stale branches cause merge conflicts and wasted effort. Stay current.

**Before starting each issue:**
```
git fetch origin dev
git log --oneline HEAD..origin/dev --  # see what changed
```
If `origin/dev` has new commits, rebase your worktree branch:
```
git rebase origin/dev
```
If rebase produces conflicts, resolve them in the worktree (never in the shared checkout). If the conflict is non-trivial, rerun validation after resolving.

**Between implementation steps (research -> test -> implement -> validate):**
If you've been working for a while, do a quick check:
```
git fetch origin dev --dry-run 2>&1 | grep -q "origin/dev" && echo "dev has new commits — rebase before continuing"
```
This prevents discovering conflicts only at PR time.

**After every PR merge:**
```
git fetch --prune
git pull --ff-only origin dev
```

## Per-issue lifecycle

### 1. Staleness check — MANDATORY before any implementation

Read the full issue:
```
gh issue view <number>
```

Before writing a single line of code, determine whether the feature is still relevant:

**Architecture drift check:**
- Read `docs/architecture/` files for the affected subsystem
- Read existing PRDs in `docs/prd/` that touch the same area
- Grep for key types, functions, or packages the issue references — do they still exist? Have they been renamed or replaced?
- Check if the issue references files, APIs, or patterns that no longer exist in the codebase
- Check the issue's comments for any "superseded by", "no longer needed", or "see instead" signals

**Scope overlap check:**
- Search closed issues and merged PRs for work that may have already addressed this feature partially or fully
- Check if another open issue or in-progress branch covers the same ground

**Staleness signals — if ANY of these are true, STOP:**
1. The types/packages/files the issue references no longer exist or have been fundamentally restructured
2. A newer issue or merged PR explicitly supersedes this one
3. The feature's user outcome is already achievable through existing functionality
4. The acceptance criteria reference components, patterns, or APIs that have been replaced
5. The PRD the issue links to has been deprecated or significantly revised

**If stale:** Comment on the issue explaining what changed, suggest closing, and move to the next issue. Do not implement stale features.

**If partially stale:** The acceptance criteria must be revised. Comment with what still applies and what doesn't. Ask the user (via AskUserQuestion) whether to proceed with the reduced scope or skip.

### 2. Scope validation — reject if not focused

A feature is only implementable if it meets ALL of these criteria:
- **Single concern:** The issue addresses one capability, not a bundle of loosely related changes
- **Clear boundaries:** Acceptance criteria are testable and finite — no "explore", "consider", or "investigate"
- **Contained blast radius:** The change touches at most 2-3 packages/subsystems. If it requires changes across 5+ packages, it needs to be split first
- **No missing prerequisites:** Every dependency the feature needs already exists or is explicitly listed as a non-goal
- **Measurable done:** You can write a test that passes only when the feature works
- **Full-stack completeness:** If the feature needs both backend and frontend, the issue must cover both. If it only covers one layer, comment asking for the full vertical slice or split it into a complete deliverable

If the issue fails scope validation: comment explaining what needs to be tightened, and move to the next issue.

### 3. Identify the vertical slice

Before coding, define the smallest end-to-end unit that delivers value:
- What does the user see or do differently when this ships?
- What backend change enables it?
- What frontend change exposes it?
- What RPC/protocol change connects them?

If the answer is "backend only" or "frontend only," ask: is this actually useful to anyone yet? If not, find the smallest slice that IS useful and implement that instead. A feature ships when a user can use it, not when a package compiles.

### 4. Research — read everything you will touch

- Read every file you will modify, fully.
- Grep for every function you plan to change — read all callers, all test files, all wiring sites.
- For tool changes: read the tool definition, Execute method, callback wiring in `daemon_supervisor_spawn.go`, and the service it delegates to.
- For frontend changes: read the component, its parent, and any hooks/stores it consumes.
- For RPC changes: read the protocol types, handler registration, and any existing tests.
- For service changes: read the domain model, repository interface, application service, and production wiring in `space_daemon.go`.
- Look at what already works in the same area. Apply the same patterns — do not invent new ones.

### 5. Write tests FIRST (TDD) — all three classes

Before changing any production code, write failing tests. They must fail because the feature doesn't exist yet.

**Behavior test (REQUIRED):**
- Exercise the feature through the real entry point the user hits: RPC handler, UI component with real store, CLI command
- No mocks. Use real implementations, real stores (in-memory SQLite is fine), real services
- If the feature spans backend and frontend: the backend behavior test exercises the RPC round-trip; the frontend behavior test renders the component and verifies it displays/interacts correctly
- No `_ =` in test setup. Use `require.NoError`
- Test doubles must capture ALL arguments. No `_` for domain params

**Regression test (REQUIRED where edge cases exist):**
- What happens with nil/empty/invalid input?
- What if the dependency is missing or returns an error?
- What if this is called concurrently?
- What if the entity is in an unexpected state?
- If the feature modifies existing behavior, add a test that the OLD behavior still works for unaffected cases

**Performance/benchmark test (REQUIRED for hot paths):**
- Add `func BenchmarkX(b *testing.B)` for: batch operations, queries over large datasets, serialization of large payloads, frequently called helpers
- For frontend: measure render performance for lists/tables with realistic data volumes
- Skip this class for one-off admin operations, configuration endpoints, or rarely-called paths

Run all tests. They must fail (because the feature doesn't exist yet). If they pass, your tests are wrong.

### 6. Implement the feature

- Run the failing tests after each change to see them go green. Stop when they pass and all existing tests still pass.
- Follow existing patterns exactly. If the codebase uses `Set*Func()` callbacks, use that pattern. If it uses interfaces, use interfaces.
- Wire new backend features through real app entry points (see CLAUDE.md "Wire new backend features" rule).
- No silent fallbacks or error swallowing. Invalid input returns an error. Always.
- If you add a new RPC method, register it in the production RPC server.
- If you add a new service dependency, wire it in `space_daemon.go`.
- If the feature has a frontend component, implement it in the same change. Backend + frontend = one feature.
- Minimum change to satisfy acceptance criteria. Nothing more.

### 7. Validate end-to-end

Run the full validation for every layer you touched:

Backend:
```
go test ./<changed-packages>/... -count=1 -timeout 60s
go vet ./<changed-packages>/...
```
Frontend:
```
cd web && npm run lint && npm run build && npm test
```
Both (if the feature spans layers):
```
make test
make lint
```

If any test fails, fix it. Do not skip, comment out, or weaken assertions.

After tests pass, sanity-check: trace the feature from entry point to user-visible output. Can a request actually reach the new code through the production wiring? If not, the feature is not done.

### 8. Commit in a worktree

Follow the worktree-first workflow from `docs/ai-development-workflow.md` Section 3.
The main checkout stays on `dev`. Never switch it.

```
git stash
git fetch origin dev
git worktree add .worktrees/feat/<slug> -b feat/<slug> origin/dev
```
Apply changes in the worktree (pop stash or re-apply), then:
```
git add <specific files>
git commit -m "feat: <what and why>"
git push -u origin feat/<slug>
```

### 9. PR, CI, merge, cleanup

Create the PR following `docs/ai-development-workflow.md` Section 4 and `.github/pull_request_template.md`:
```
gh pr create --base dev --title "feat: ..." --body "Closes #<issue>.

## Summary
...
## Problem
...
## Task reference
#<issue>
## Local validation
- go test, go vet, make lint (list what you ran)
## Architectural impact
..."
gh pr checks <PR> --watch
gh pr merge <PR> --squash --delete-branch
```
If CI fails: read the error, fix in the worktree, push, watch again.

After merge:
```
cd <project-root>
git worktree remove --force .worktrees/feat/<slug>
git fetch --prune
git pull --ff-only origin dev
```

### 10. Confirm and move on
Verify the merge landed: `git log --oneline -3`
If the PR body had `Closes #<number>`, the issue auto-closes. Otherwise:
```
gh issue close <number> --comment "Implemented in #<PR>"
```

Repeat from step 1 with the next issue.

## When stuck

If the same approach fails 3 times, do not retry it. Write what you learned and what went wrong to the `Lessons learned` section of `AGENTS.md` so future runs benefit from it. Then try a different approach or ask the user.

## Start now

Read the project docs, list the open feature requests, run the staleness check on the first one, and begin implementing if it passes.
