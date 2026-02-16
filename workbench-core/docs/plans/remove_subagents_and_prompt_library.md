# Remove subagent functionality in team mode + prompt library

**Overview:** Extract built-in prompts into a dedicated prompt library (`pkg/prompts`), remove subagent functionality from team mode, and introduce a dedicated team-mode system prompt so co-agents are not instructed about subagents. Standalone mode keeps current behavior.

**Clean break:** No backwards compatibility required. All call sites are updated in this work; choose the clearest API and naming for the prompt library (and any tool schema changes) without preserving old names or behavior for existing code.

---

## Current state

- **Team mode** ([team_daemon.go](internal/app/team_daemon.go)): All roles use `agent.DefaultAutonomousSystemPrompt()`, which includes subagent rules (spawn_worker, task_review, callbacks, /deliverables/subagents/, etc.). Co-agents are told to use subagents even though team mode has no child runs.
- **Standalone mode** ([daemon_runtime_supervisor.go](internal/app/daemon_runtime_supervisor.go)): Top-level runs use `DefaultAutonomousSystemPrompt()`; child runs use `DefaultSubAgentSystemPrompt()`. Subagent flow (spawn_worker → callback → task_review) is only here.
- **task_create** ([task_create.go](pkg/agent/hosttools/task_create.go)): In team daemon, `SpawnWorker` is not set (nil), so `spawn_worker=true` already returns "spawn_worker is not available in this context". The tool definition and description still advertise spawnWorker and subagents.
- **task_review**: Registered in team daemon; in team mode there are no subagent callbacks, so it would only ever error. Team callbacks use source=team.callback and are reviewed by the coordinator via normal task completion, not the task_review tool.

## Goal

1. **Prompt library**: Extract all built-in system prompts into a new `pkg/prompts` package so prompts live in one place and new prompts (e.g. per feature or mode) are easy to add.
2. **Prompts**: Separate team-mode prompt from standalone autonomous prompt so team co-agents are never instructed about subagents, spawn_worker, task_review, or callbacks.
3. **Tools**: In team mode, do not expose task_review; expose task_create without spawnWorker in schema/description so the model is not nudged toward subagent behavior.
4. **Robustness**: Keep standalone behavior unchanged; only team mode is restricted and re-prompted.

---

## To-dos (commit after successful tests)

Each item: implement the change, run the relevant tests (and full suite where noted), then **commit if green**.

| #   | To-do                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               | Commit message (suggestion)                                               |
| --- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------- |
| 1   | **Prompt library** — Add `pkg/prompts`: `doc.go`, `base.go` (DefaultSystemPrompt + baseWithoutRecursiveDelegation), `modes.go` (DefaultAutonomousSystemPrompt, DefaultSubAgentSystemPrompt, DefaultTeamModeSystemPrompt). Add `pkg/prompts/prompts_test.go` (team prompt excludes subagent wording; base contains e.g. fs_list). Run `go test ./pkg/prompts/...`; commit.                                                                                                                                                                                                                           | `pkg/prompts: add prompt library (base, autonomous, subagent, team mode)` |
| 2   | **Wire prompts in agent and app** — In `loop.go` remove all prompt strings and the four prompt functions; use `prompts.DefaultSystemPrompt()` when base empty. In `constructors.go`, `context_constructor.go` use `prompts.DefaultSystemPrompt()`. In `team_daemon.go` use `prompts.DefaultTeamModeSystemPrompt()`. In `daemon_runtime_supervisor.go` use `prompts.DefaultAutonomousSystemPrompt()` and `prompts.DefaultSubAgentSystemPrompt()`. Update `agent_prompts_test.go` to use `prompts.DefaultSystemPrompt()`. Run `go build ./... && go test ./pkg/agent/... ./internal/app/...`; commit. | `agent, app: use pkg/prompts for system prompts`                          |
| 3   | **Team daemon: drop task_review** — Remove registration of `hosttools.TaskReviewTool` for team roles in `team_daemon.go`. Run `go test ./internal/app/...`; commit.                                                                                                                                                                                                                                                                                                                                                                                                                                 | `app: do not register task_review in team mode`                           |
| 4   | **task_create: team definition** — In `task_create.go` `Definition()`, when `t.SpawnWorker == nil` return a tool definition without `spawnWorker` in params and with a team-only description. Add or extend hosttools test that when SpawnWorker is nil, Definition() does not include spawnWorker. Run `go test ./pkg/agent/... ./internal/app/...`; commit.                                                                                                                                                                                                                                       | `hosttools: task_create team definition without spawnWorker`              |
| 5   | **Full suite** — Run `go test ./...` from workbench-core; fix any remaining failures; commit if any minor fixes.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    | (only if needed)                                                          |

---

## 0. Extract prompts into a prompt library (pkg/prompts)

Create a new package **`pkg/prompts`** that owns all built-in system prompt content. This becomes the single source of truth and makes it easy to add new prompts as features grow.

### Package layout

- **`doc.go`** (or package comment): One-line description plus a short **"How to add a prompt"** note: add a new exported function that returns `string`; build on `Base()` or `BaseWithoutRecursiveDelegation()` as needed; keep mode-specific content in the same file or a dedicated file (e.g. `team.go`).
- **`base.go`**: Move the full base system prompt from [loop.go](pkg/agent/loop.go) (identity, planning, capabilities, VFS, memory, operating_rules) into **`DefaultSystemPrompt()`** (or **`Base()`**). Add **`baseWithoutRecursiveDelegation()`** (unexported) that returns the same content with the `<recursive_delegation>...</recursive_delegation>` block removed (string replace or regex).
- **`modes.go`** (or split into `autonomous.go`, `subagent.go`, `team.go`): Move **`DefaultAutonomousSystemPrompt()`** and **`DefaultSubAgentSystemPrompt()`** from loop.go into this package; they continue to call the base and append their mode blocks. Add **`DefaultTeamModeSystemPrompt()`** (see section 1) which uses `baseWithoutRecursiveDelegation()` + `<team_autonomous_mode>`.

### Exported API

Clean break: use whatever names are clearest. For example:

- `DefaultSystemPrompt() string` — base (identity, planning, capabilities, VFS, memory)
- `DefaultAutonomousSystemPrompt() string` — standalone daemon / task runner
- `DefaultSubAgentSystemPrompt() string` — spawned child agents
- `DefaultTeamModeSystemPrompt() string` — team co-agents (no subagent rules)

Or shorter: `Base()`, `Autonomous()`, `SubAgent()`, `TeamMode()`. All call sites are updated in this plan; no need to preserve old names.

Internal helpers (unexported): e.g. `baseWithoutRecursiveDelegation() string`.

### Call-site updates

- [pkg/agent/loop.go](pkg/agent/loop.go): When base is empty, use `prompts.DefaultSystemPrompt()` (import `github.com/tinoosan/workbench-core/pkg/prompts`).
- [pkg/agent/constructors.go](pkg/agent/constructors.go): Use `prompts.DefaultSystemPrompt()` in DefaultConfig and NewAgent/NewDefaultAgent.
- [pkg/agent/context_constructor.go](pkg/agent/context_constructor.go): Use `prompts.DefaultSystemPrompt()`.
- [internal/app/team_daemon.go](internal/app/team_daemon.go): Use `prompts.DefaultTeamModeSystemPrompt()`.
- [internal/app/daemon_runtime_supervisor.go](internal/app/daemon_runtime_supervisor.go): Use `prompts.DefaultAutonomousSystemPrompt()` and `prompts.DefaultSubAgentSystemPrompt()`.
- [pkg/agent/agent_prompts_test.go](pkg/agent/agent_prompts_test.go): Use `prompts.DefaultSystemPrompt()`.

After this step, **delete** all prompt string content and the four prompt functions from `pkg/agent/loop.go` (agent no longer defines prompts; it only consumes them).

---

## 1. Add team-mode system prompt in the library (pkg/prompts)

- In **`pkg/prompts`**, add **`DefaultTeamModeSystemPrompt()`** (see section 0; it lives in the library, not in agent):
  - Uses **`baseWithoutRecursiveDelegation()`** (no `<recursive_delegation>` block).
  - Appends a **`<team_autonomous_mode>`** block with:
    - **not_chat** – task runner, not chat.
    - **scope** – single-goal focus, complete and report; **delegate only by creating tasks with assignedRole** (per the existing `<team>` block in session/prompt.go), not by spawning workers.
    - **honest_reporting**, **state_persistence**, **initiative** – same as autonomous.
    - **reporting** – final_answer/artifacts requirements; keep or relax email step for team as desired.
    - **no_sleep** – process tasks, don’t wait.
  - **Do not include**: coordination_principle (callbacks), subagents, subagent_examples, spawn_review, no_duplicate_delegated, callback_rule, no_poll_for_callbacks, final_report_and_plan (subagent), or any mention of task_review / spawn_worker / /deliverables/subagents/ / callbacks.

---

## 2. Use team prompt in team daemon (internal/app/team_daemon.go)

- Replace:
  - `agentCfg.SystemPrompt = agent.DefaultAutonomousSystemPrompt()`
- With:
  - `agentCfg.SystemPrompt = prompts.DefaultTeamModeSystemPrompt()`
- Add import: `github.com/tinoosan/workbench-core/pkg/prompts`.

No other daemon logic change here; the existing `<team>` block is still injected by [buildSystemPrompt](pkg/agent/session/prompt.go) (profile, memory, team block).

---

## 3. task_create: team-only definition when SpawnWorker is nil (pkg/agent/hosttools/task_create.go)

Clean break: when spawn is disabled, the tool definition is team-only (no spawnWorker, no subagent wording); no need to keep the old description.

- In **`Definition()`**: when **`t.SpawnWorker == nil`** (team mode today; could be any context that disables spawning):
  - Return a **different** tool definition:
    - **Description**: Focus on creating tasks and, in team mode, assigning to roles via `assignedRole`. Do **not** mention spawnWorker, subagents, workers, callbacks, or task_review.
    - **Parameters**: Omit the **`spawnWorker`** property entirely so the model cannot request it.
  - When `SpawnWorker != nil`, keep the current definition (including spawnWorker and current description).

- **Execute()** is already correct: when `SpawnWorker == nil` and `payload.SpawnWorker` is true, it returns an error. Changing the definition only avoids suggesting spawn_worker in team mode.

---

## 4. Do not register task_review in team mode (internal/app/team_daemon.go)

- **Remove** the registration of `hosttools.TaskReviewTool` for team roles (the block that registers `TaskReviewTool` with `Store`, `SessionID`, `RunID`, `Supervisor: teamSupervisor`).
- Team agents will no longer see or call task_review. Coordinator review of worker results continues via normal task completion (team.callback tasks) and optional follow-up delegation.

---

## Summary of behavior after changes

| Area        | Team mode                                                                          | Standalone                                             |
| ----------- | ---------------------------------------------------------------------------------- | ------------------------------------------------------ |
| Base prompt | `prompts.DefaultTeamModeSystemPrompt()` (no subagent rules)                        | `prompts.DefaultAutonomousSystemPrompt()` (unchanged)  |
| task_create | Schema/description without spawnWorker; Execute already errors if spawnWorker=true | Unchanged (spawnWorker available when SpawnWorker set) |
| task_review | Not registered                                                                     | Registered                                             |
| Callbacks   | Only team.callback (coordinator review); no subagent.callback                      | subagent.callback + task_review (unchanged)            |

---

## Files to touch

| File                                        | Change                                                                                                                    |
| ------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------- |
| **New** `pkg/prompts/doc.go`                | Package comment + "How to add a prompt" (add function returning string; build on Base or baseWithoutRecursiveDelegation). |
| **New** `pkg/prompts/base.go`               | `DefaultSystemPrompt()` (Base), `baseWithoutRecursiveDelegation()` (internal helper).                                     |
| **New** `pkg/prompts/modes.go`              | `DefaultAutonomousSystemPrompt()`, `DefaultSubAgentSystemPrompt()`, `DefaultTeamModeSystemPrompt()`.                      |
| `pkg/agent/loop.go`                         | Remove all prompt content and the four prompt functions; use `prompts.DefaultSystemPrompt()` when base is empty.          |
| `pkg/agent/constructors.go`                 | Use `prompts.DefaultSystemPrompt()`; add import for `pkg/prompts`.                                                        |
| `pkg/agent/context_constructor.go`          | Use `prompts.DefaultSystemPrompt()`; add import for `pkg/prompts`.                                                        |
| `internal/app/team_daemon.go`               | Use `prompts.DefaultTeamModeSystemPrompt()`; remove TaskReviewTool registration; add import for `pkg/prompts`.            |
| `internal/app/daemon_runtime_supervisor.go` | Use `prompts.DefaultAutonomousSystemPrompt()` and `prompts.DefaultSubAgentSystemPrompt()`; add import for `pkg/prompts`.  |
| `pkg/agent/agent_prompts_test.go`           | Use `prompts.DefaultSystemPrompt()`.                                                                                      |
| `pkg/agent/hosttools/task_create.go`        | In `Definition()`, when `SpawnWorker == nil`, return description and params without spawnWorker.                          |

---

## Testing

- **Unit (prompts)**: Add `pkg/prompts/prompts_test.go`: test that `DefaultTeamModeSystemPrompt()` does not contain "spawn_worker", "task_review", "subagent", or "callback" (beyond generic "task" wording); test that `DefaultSystemPrompt()` contains expected core content (e.g. "fs_list").
- **Unit (agent)**: Keep or adjust `agent_prompts_test.go` to use `prompts.DefaultSystemPrompt()`.
- **Unit (hosttools)**: Test that when `SpawnWorker == nil`, `TaskCreateTool.Definition()` does not include a `spawnWorker` property.
- **Integration / manual**: Run team mode and confirm only task_create (with assignedRole) is used for delegation; run standalone and confirm spawn_worker and task_review still work.

No change to session callback logic ([session.go](pkg/agent/session/session.go) `maybeCreateCoordinatorCallback`): team path already creates team.callback only; subagent path unchanged.
