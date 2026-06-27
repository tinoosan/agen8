---
name: agen8-project-coordination
description: Project-specific coordination workflow for the agen8 repository. Use when orchestrating multi-agent work in this repo with Agen8, subagents, in-repo .worktrees scratch checkouts, pull requests, reviewer agents, CI gates, rework routing, merge readiness, and cleanup. This skill is local to this project and is not part of the installable Agen8 skill library.
---

# Agen8 Project Coordination

Use this project-local skill after loading the generic `agen8`, `agen8-graph`, and `agen8-coordination` skills. Those skills define the durable work model; this skill defines this repository's coordination policy.

## Non-Negotiables

- Register every coordinator, worker, and reviewer to Agen8.
- Subagents must register using the system-assigned nickname shown when they are spawned.
- Register against the canonical main project root: `/Users/santinoonyeme/personal/dev/Projects/agen8-mcp-server`.
- Do not register worker scratch folders as separate Agen8 projects.
- Do not use `git worktree`. It changes project location and fragments Agen8 visibility.
- Use ordinary in-repo scratch checkouts or copies under `.worktrees/`.
- Keep `.worktrees/` ignored and clean it up after merge, cancellation, or handoff.
- Use medium reasoning for subagents unless the user explicitly requests otherwise.
- Do not use low-capability/haiku-class agents for implementation or review.
- Reuse existing open subagents for compatible follow-up work; spawn a new subagent only when role separation, parallelism, or stale context makes reuse unsafe.
- Keep the user out of the loop unless blocked by missing information, external access, or a decision the coordinator cannot safely make.
- Agen8 task creation requires an active assignee; when no active member exists for a role, spawn/register the subagent first, then create the assigned task.

## Scratch Checkout Pattern

For implementation work, create a task-scoped folder under `.worktrees/`, for example:

```text
.worktrees/task-abc123-question-decision-port/
```

Prefer a local clone or copy that can create its own branch and PR without touching the hot-reload main checkout. Do not use the git worktree feature. Workers may work inside the scratch checkout, but all Agen8 registration, task claiming, decisions, and evidence must reference the canonical main project.

When the main checkout is dirty or hot-reloading, base scratch clones from the fetched remote branch, usually `origin/main`, rather than the main checkout's current working tree.

Worker prompts must explicitly say:

```text
Register to Agen8 using your system-assigned subagent nickname.
Register to Agen8 using project_root=/Users/santinoonyeme/personal/dev/Projects/agen8-mcp-server.
Do not register the .worktrees folder as a project.
Work only in .worktrees/<task-folder>.
```

## Branch Naming

Keep branch names human-readable. Agen8 already tracks durable mission and task
ids, so do not put long task ids in branch names unless parallel work would
otherwise be ambiguous.

Use these prefixes:

```text
feat/<short-topic>
fix/<short-topic>
docs/<short-topic>
ci/<short-topic>
chore/<short-topic>
release/<version>
hotfix/<version-or-topic>
```

Examples for this repo:

```text
feat/helm-chart
feat/mission-kr-reopen-flow
fix/kubernetes-ingress-host
fix/write-file-reader-root
docs/self-hosting-setup
ci/ghcr-release-image
chore/ignore-design-drafts
release/v0.1.0
hotfix/v0.0.2-auth-token
```

Default to branching from the latest `origin/main`. For urgent production fixes,
branch from the released tag only when `main` contains unrelated unreleased work
that must not ship with the fix.

## Release Convention

`main` should remain releasable. Normal feature and fix work lands through PRs
and squash merges. Create release tags from green `main` unless a release needs
a short stabilization branch.

Use SemVer tags:

- Bug fixes and small safe polish after the first seed release use `v0.0.x`.
- Meaningful feature baselines use `v0.x.0`.
- Breaking product, MCP, or storage contract changes before stability use the
  next `v0.x.0` minor.
- A stable public compatibility promise starts at `v1.0.0`.

Before tagging, confirm the release-relevant CI and artifact workflows are green.
For Kubernetes-capable releases, confirm the container image workflow has either
already been proven for the tag or is explicitly part of the tag verification
mission.

## Release Tracking

After the first release, every merged product, MCP contract, storage, packaging,
deployment, or operator-facing change must be tracked for the next release.

Use the existing release artifacts instead of burying release facts in chat:

- update the relevant release notes or create the next `docs/release-notes-vX.Y.Z.html`
- record an Agen8 decision when a change affects setup, deployment, MCP/tool
  contracts, data shape, auth/token behavior, or Kubernetes/homelab operation
- link merged PRs, validation output, and known caveats to the release mission
- create follow-up tasks for release blockers instead of leaving them as TODOs
- verify `cmd/agen8/version.go`, tags, and release workflow expectations before
  cutting or announcing a release

Release missions should include KRs for code readiness, artifact/container
readiness, docs/operator notes, and post-release verification. A release is not
done until the tag, generated artifacts, release notes, and Agen8 mission state
agree on what shipped.

## PR Delivery Loop

For coding tasks, the worker owns implementation through PR creation.

1. Coordinator creates/activates the mission/KR/task and assigns one task to one worker.
2. Worker registers to Agen8, claims the task, works in `.worktrees/<task-folder>`, validates locally, pushes a branch, and opens a PR.
3. Worker submits the task with PR URL, changed files, validation output, residual risks, and any notes.
4. Coordinator starts a separate reviewer agent.
5. Reviewer registers to Agen8, reads the task and PR, performs review, polls CI, watches comments, and records review notes.
6. If changes are required, reviewer records PR comments/review notes and coordinator routes the task back to the worker.
7. Worker implements requested changes on the same PR and resubmits evidence.
8. Reviewer repeats until approval and CI are green.
9. Coordinator confirms merge readiness, merges the PR, updates KRs, logs decisions, closes tasks, closes agents, cleans `.worktrees/<task-folder>`, and prunes deleted branch refs.

PR comments are review notes. The reviewer must make comments specific enough that the worker knows what to change.

## Worker Prompt Requirements

Include all of this in worker prompts:

- role/name and task id
- instruction to register with the system-assigned subagent nickname
- mission id and KR id
- canonical Agen8 project root
- scratch folder path under `.worktrees/`
- branch name using the convention in this skill
- PR requirement
- exact acceptance criteria
- validation commands/checks, including the repo's CI-equivalent formatting/lint checks for coding tasks
- evidence required on submit
- stop conditions
- reminder not to create stray missions/KRs/tasks
- reminder not to register the scratch folder as an Agen8 project

The worker should stop and submit a blocker if the task requires expanding beyond the owned scope.

## Reviewer Prompt Requirements

Use a separate reviewer agent when a worker agent produced implementation.

Reviewer prompts must include:

- task id, PR URL, mission id, KR id
- acceptance criteria
- canonical Agen8 project root
- instruction to register to Agen8 with the system-assigned subagent nickname
- instruction to poll CI and unresolved PR comments/review threads
- instruction to perform an independent review of the PR diff
- instruction to treat PR comments as review notes
- instruction to request changes through PR comments/Agen8 notes instead of silently implementing worker changes

Reviewer approval requires:

- acceptance criteria satisfied
- PR comments resolved or intentionally deferred
- CI passing
- no unexplained broad refactors
- tests or evidence adequate for risk
- residual risks recorded

## Coordinator Responsibilities

The coordinator must actively manage the workflow:

- create missions/KRs/tasks with observable outcomes
- create or select explicit Agen8 roster members for worker/reviewer roles before creating assigned tasks
- verify selected assignee members are active before creating assigned tasks
- assign workers and reviewers, preferring existing open subagents for compatible follow-up work
- monitor claimed tasks and stale work
- send check-in messages when workers/reviewers are quiet
- route review feedback back to the worker
- decide whether a follow-up task is better than scope expansion
- verify CI and reviewer status before merge
- merge only after review success
- clean `.worktrees/` folders and close subagents
- prune deleted remote branch refs after merged PRs
- update KR progress and final mission state

Do not end a coordination turn with live worker/reviewer agents running unless the user explicitly asks to leave them running and the task state records the handoff.

## Anti-Slop Checklist

Before approving work, verify:

- the change solves the stated task, not an adjacent invented task
- the PR is smaller than the problem allows, not larger
- the worker ran repo-level CI-equivalent preflight checks, not only scoped package tests
- interfaces and names are domain-specific
- no hidden fallbacks or silent no-ops were added
- tests prove behavior, not implementation trivia
- docs or comments changed only when they reduce future confusion
- review notes mention what is not covered
- follow-up work is captured as tasks, not buried in prose

## Cleanup

After merge, cancellation, or abandoned work:

- delete `.worktrees/<task-folder>` and any reviewer-created `.worktrees/review-*` folders
- delete local temporary branches when safe
- run `git fetch --prune origin` and confirm merged task branches no longer appear in local, remote-tracking, or remote-head listings
- close worker and reviewer agents
- update Agen8 task status
- log any reason cleanup could not be completed

The scratch folder is never durable state.
