# HANDOVER

Date: 2026-02-26
Branch: `codex/pivot-harness-orchestrator`
Upstream: `origin/codex/pivot-harness-orchestrator`
PR: https://github.com/tinoosan/agen8/pull/16

## 1. What was completed before this handoff
Recent commits already on branch:
- `b945944` Pivot to multi-harness orchestrator with harness routing and observability
- `27074aa` Fix adapter total token preservation and costs RPC error handling
- `f6ec975` Stop persisting/emitting external harness cost
- `311a6c8` Add harness defaults to config.toml runtime config
- `3539817` Document harness config.toml defaults and CLI observability aliases

Implemented baseline:
- Harness contract + registry + selector.
- Native path wired as adapter-backed.
- `codex-cli` adapter registered and executable.
- Harness selection precedence implemented.
- Harness metadata surfaced in task RPC/list/state.
- Observability aliases (`status`, `feed`, `trace`, `costs`) added.
- Config-first harness defaults documented and wired.

## 2. Clarification reached today (critical)
The user clarified orchestration intent:
- External harness must not be “text only”.
- Coordinator must be able to orchestrate by creating work and spawning workers dynamically.
- This requires an Agen8 action-application bridge, not just adapter execution.

Current reality:
- External `codex-cli` path runs per task and returns text/usage.
- It does not directly invoke Agen8 host tools in external mode.
- Therefore, without Phase 2 bridge, external coordinator cannot truly orchestrate autonomously.

## 3. Phase 2 decisions locked
- Output contract: strict JSON envelope.
- Apply path: session post-adapter (adapter remains execution-only).
- Spawn model: dynamic spawn + assign required in Phase 2.

## 4. Phase 2 implementation plan file
Planned canonical plan file:
- `docs/plans/phase2-external-coordinator-actions.md`

Core components to add:
- `pkg/harness/actions/*` (contract/parser/validator/idempotency)
- `internal/app/external_coordinator_bridge.go`
- session hook in `pkg/agent/session/session.go`
- codex envelope builder in `pkg/harness/codexcli/coordinator_envelope.go`
- runtime config additions in `internal/app/runtime_config.go`

## 5. Repo state at handoff
`git status --short --branch` at handoff:
- `## codex/pivot-harness-orchestrator...origin/codex/pivot-harness-orchestrator`
- `M AGENT.md`
- `M agen8`

Notes:
- `AGENT.md` was updated with explicit external orchestration semantics and bridge guardrails.
- `agen8` appears as a modified built binary artifact in working tree.

## 6. Immediate next steps for implementing agent
1. Commit/keep `AGENT.md` update as intended guardrail.
2. Add phase-2 plan file under `docs/plans/`.
3. Implement `pkg/harness/actions` with strict validation + deterministic ids.
4. Add session post-adapter apply hook behind config gate.
5. Implement supervisor-backed bridge for task apply + deterministic spawn.
6. Add codex coordinator JSON-only envelope prompt builder.
7. Extend runtime config + docs with action-mode controls.
8. Add unit/session/integration tests and run `go test ./...`.

## 7. Acceptance criteria for next phase
- External coordinator can emit valid actions that result in:
  - role task creation
  - spawned worker run creation
  - initial task assignment to spawned run
- Retries are idempotent (no duplicate runs/tasks).
- Invalid envelope/action set fails safely with explicit error.
- Existing orchestrator behavior remains unchanged when action mode is disabled.

## 8. Risks to watch
- Duplicate side effects on retry/replay without deterministic ids.
- Role-validation gaps causing misrouted tasks.
- Coupling adapter to DB mutation (must avoid; bridge owns mutations).
- Action parsing fragility if output is not forced JSON-only.
