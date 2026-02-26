# Phase 2 Plan: Codex External Orchestration With Dynamic Spawn + Assign

## Summary
Phase 2 makes `codex-cli` a true orchestrator harness by introducing a strict coordinator action contract that Agen8 validates and applies to its own state.

Locked decisions:
- Output contract: strict JSON envelope.
- Apply path: session post-adapter.
- Orchestration scope: dynamic spawn + assign.

## Scope
In scope:
- Parse/validate coordinator actions from external harness output.
- Apply actions via Agen8 task/run services.
- Support `create_role_task` and `spawn_worker_task`.
- Add idempotency + safety limits.
- Add observability events for parse/apply/reject.

Out of scope:
- External `task_review` execution (Phase 2.1).
- New external adapters beyond `codex-cli`.

## Architecture
### 1) Action contract package
Add package:
- `pkg/harness/actions/types.go`
- `pkg/harness/actions/parse.go`
- `pkg/harness/actions/validate.go`
- `pkg/harness/actions/idempotency.go`

Envelope:
- `version: "agen8.actions.v1"`
- `mode: "coordinator"`
- `actions: []`

Action types:
- `create_role_task`: `{goal, assignedRole, priority?, harnessId?}`
- `spawn_worker_task`: `{goal, priority?, harnessId?}`

### 2) Session-side apply hook
File:
- `pkg/agent/session/session.go`

Add config dependency:
- `ExternalCoordinatorActionApplier` interface

Flow:
- For non-native harness + coordinator + action mode enabled:
  - parse adapter text as strict envelope
  - validate all actions
  - apply actions through applier
  - emit action events
- If parse/validate/apply fails:
  - fail current task with explicit reason
  - no partial duplicate side effects

### 3) App bridge (state mutation owner)
Add:
- `internal/app/external_coordinator_bridge.go`

Responsibilities:
- role validation against team context
- `task.create` via task manager
- dynamic child run spawn (deterministic)
- first-task routing to spawned child run
- idempotent apply on retries

### 4) Supervisor spawn reuse
File:
- `internal/app/daemon_runtime_supervisor.go`

Reuse existing spawn logic (`makeSpawnWorkerFunc`) by exposing deterministic helper:
- load-or-create child run by deterministic id
- preserve `ParentRunID`, `SpawnIndex`, runtime model/profile/harness

### 5) Codex coordinator prompt envelope
Add:
- `pkg/harness/codexcli/coordinator_envelope.go`

Behavior:
- wrap coordinator goals with strict “JSON-only output” instructions
- include valid action schema + limits + valid roles
- keep non-coordinator external tasks unchanged

### 6) Config additions
File:
- `internal/app/runtime_config.go`
- docs updates in `docs/config-toml.md` and `docs/config.toml.example`

Add optional `[harness]` keys:
- `codex_action_mode = "disabled" | "coordinator_actions_v1"`
- `codex_actions_require_json = true|false`
- `codex_actions_max_actions = <int>`
- `codex_actions_max_spawn_actions = <int>`
- `codex_actions_max_goal_chars = <int>`

Default:
- `codex_action_mode = "disabled"`

## Idempotency and safety
- deterministic action key from `parentTaskID + actionIndex + normalizedAction`
- deterministic ids for created tasks/runs from action key
- if existing id found, treat as already-applied success
- hard limits from config for action count/spawn count/goal length
- unknown roles rejected before apply

## Observability
Emit:
- `coordinator.actions.parsed`
- `coordinator.actions.applied`
- `coordinator.actions.rejected`
- `coordinator.spawn.created`

Payload includes:
- `taskId`, `runId`, `harnessId`, `actionCount`, `appliedCount`, `rejectedCount`, `spawnedRunCount`, `reason`

## Tests
Unit:
- parser/validator/idempotency determinism in `pkg/harness/actions/*_test.go`

Session:
- successful apply path
- malformed envelope failure
- non-coordinator bypass path

Integration:
- dynamic spawn creates child run once under retry
- spawned child receives first task
- role task creation visible in `task.list`
- unknown role rejected with no side effects

Regression gates:
- `task.create`, `task.list`, `logs.query`, `dashboard --once`
- no implicit bootstrap on clean startup
- `go test ./...` all green
