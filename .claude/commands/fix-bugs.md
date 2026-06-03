Fix open bug issues one at a time. For each bug, the full lifecycle is below. Do not deviate.

## Setup

Read these files before starting. They are the law — not guidelines, not suggestions.
- `CLAUDE.md`
- `AGENTS.md`
- `docs/ai-development-workflow.md`

List open bug issues:
```
gh issue list --label bug --state open --json number,title --jq 'sort_by(.number) | .[] | "#\(.number) — \(.title)"'
```

Pick the lowest-numbered issue. If the user specifies issues, work those in the given order.

## Per-issue lifecycle

### 1. Read the issue spec
```
gh issue view <number>
```
The acceptance criteria are the spec. Satisfy all of them. Do not add scope. Do not reduce scope.

### 2. Research — read everything you will touch
- Read every file you will modify, fully.
- Grep for every function you plan to change — read all callers, all test files, all wiring sites.
- For tool changes: read the tool definition, Execute method, callback wiring in `daemon_supervisor_spawn.go`, and the service it delegates to.
- For frontend changes: read the component, its parent, and any hooks/stores it consumes.
- For prompt changes: read the full prompt rules method and any tests that assert prompt content.
- Look at what already works in the same area. Apply the same patterns — do not invent new ones.

### 3. Write the failing test FIRST (TDD)
- Before changing any production code, write a test that reproduces the bug. Run it. It must fail. If it passes, your test is wrong — it is not testing the bug.
- The test must exercise real code paths — no mocks. Use real implementations, real stores (in-memory SQLite is fine), real services. If a dependency is hard to construct, that is a design signal, not a reason to mock.
- No `_ =` in test setup. Use `require.NoError`.
- Test doubles (stubs, spies, fakes) are acceptable ONLY when they implement a real interface with a real in-memory backing. A struct that returns canned values and ignores inputs is a mock — do not use it.
- Test doubles that do exist must capture ALL arguments. No `_` for domain params.

### 4. Implement the fix
- Run the failing test after each change to see it go green. Stop when it passes and all existing tests still pass.
- Minimum change to satisfy acceptance criteria. Nothing more.
- No refactoring of surrounding code.
- No added comments, docstrings, or type annotations on unchanged code.
- No "while I'm here" improvements.
- No silent fallbacks, nil guards that return nil, or error swallowing. Invalid input returns an error. Always.
- If you change a function signature, update every caller. Grep first.

### 5. Validate
```
go test ./<changed-packages>/... -count=1 -timeout 60s
go vet ./<changed-packages>/...
```
Frontend:
```
cd web && npm run lint && npm run build && npm test
```
If any test fails, fix it. Do not skip, comment out, or weaken assertions. If a pre-existing test breaks, understand why before deciding to update or revert.

### 6. Commit in a worktree

The main checkout stays on `dev`. Never switch it.

```
git stash
git worktree add .worktrees/fix/<slug> -b fix/<slug> dev
```
Apply changes in the worktree (pop stash or re-apply), then:
```
git add <specific files>
git commit -m "fix: <what and why>"
git push -u origin fix/<slug>
```

### 7. PR, CI, merge, cleanup
```
gh pr create --base dev --title "fix: ..." --body "Closes #<issue>. ..."
gh pr checks <PR> --watch
gh pr merge <PR> --squash --delete-branch
```
If CI fails: read the error, fix in the worktree, push, watch again.

After merge:
```
cd <project-root>
git worktree remove --force .worktrees/fix/<slug>
git fetch --prune
git pull --ff-only origin dev
```

### 8. Confirm and move on
Verify the merge landed: `git log --oneline -3`
If the PR body had `Closes #<number>`, the issue auto-closes. Otherwise:
```
gh issue close <number> --comment "Fixed in #<PR>"
```

Repeat from step 1 with the next issue.

## When stuck

If the same approach fails 3 times, do not retry it. Write what you learned and what went wrong to the `Lessons learned` section of `AGENTS.md` so future runs benefit from it. Then try a different approach or ask the user.

## Start now

Read the project docs, list the open bugs, and begin fixing the first one.
