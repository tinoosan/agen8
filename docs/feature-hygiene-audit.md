# Feature Hygiene Audit (Agen8)

SECTION 1 — Repo surface map (feature-focused)

- Entry points (`cmd/*`) and activation:
  - `cmd/agen8/main.go:5-7` -> `cmd.Execute()`.
  - `cmd/agen8/cmd/root.go:35-137` defines root command and persistent flags, then registers commands (`daemon`, `monitor`, `profiles`, plus command files with their own `init()` hooks).
  - Session/workflow features are activated by subcommands in `cmd/agen8/cmd/workflow_commands.go`, `events_commands.go`, `tasks.go`, `dashboard_command.go`, `sessions_commands.go`, `utility_commands.go`, `monitor.go`, `activity_tui.go`.
- Config/flag surface area:
  - CLI flags are defined in `cmd/agen8/cmd/root.go:102-133` and command-local `Flags()` calls (for example `cmd/agen8/cmd/dashboard_command.go:164-168`, `cmd/agen8/cmd/events_commands.go:184-190`).
  - Runtime config parsing is in `internal/app/runtime_config.go:84-215` (`AGEN8_CONFIG`, `config.toml`, defaults/env/skills/code_exec/path_access/obsidian sections).
  - Data-dir config surface is in `pkg/config/data_dir.go:12-63` (`AGEN8_DATA_DIR`, `XDG_STATE_HOME`).
- Registries/plugin points/toggles:
  - RPC method registration is centralized via `internal/app/rpc_server.go:129-159` (`methodRegistry`, `methodRegistrar`, `buildMethodRegistry`) and wired in `internal/app/rpc_server.go:233-243`.
  - Protocol method names are defined in `pkg/protocol/request.go` constants and consumed in RPC handlers/clients.
  - Feature toggles are mostly env/flag gates (`root.go` + `runtime_config.go`), not dynamic plugin loading.

SECTION 2 — Candidate features list (max 30, high-signal only)

| ID | Category | Severity | Location(s) | How it is activated | Proof summary | Recommendation | Risk | Approval |
|---|---|---|---|---|---|---|---|---|
| F001 | Dead | High | `cmd/agen8/cmd/coordinator_shell.go` `runCoordinatorShell`, `handleCoordinatorCommand`, `rpcSessionActionWithRecovery`, `submitCoordinatorGoal`, `resolveCoordinatorState`, `rpcLatestSeq` (pre-delete lines 24-239) | No activation path found | - Symbol search only showed self-references inside `coordinator_shell.go`.<br>- `cmd/agen8/cmd/workflow_commands.go:191-193` routes `runCoordinatorShellFn` to `coordinator.Run(...)`, not `runCoordinatorShell`. | Delete dead shell stack; retain shared helper `isRetryableLiveError`. | Low | No |
| F002 | Dead | Medium | `cmd/agen8/cmd/events_commands.go` `filterRunIDsByRole` (pre-delete lines 184-212) | No activation path found | - No call sites outside declaration.<br>- `logs` flags in `events_commands.go` expose no role filter input, so the helper had no reachable path. | Delete | Low | No |
| F003 | Overbuilt | Medium | `internal/app/daemon_protocol_initializer.go` `ProtocolInitializer` type/methods (pre-delete lines 16-150) | No activation path found | - No references to `ProtocolInitializer`/`newProtocolInitializer` outside file.<br>- Daemon path calls `shouldEnableProtocolStdio` directly in `internal/app/daemon.go:62`. | Delete abstraction, keep `shouldEnableProtocolStdio`. | Low | No |
| F004 | Dead | Low | `internal/app/task_lifecycle.go` `cancelActiveTasksForRun` (pre-delete lines 10-24) | No activation path found | - No call sites for `cancelActiveTasksForRun`.<br>- Task cancellation behavior is handled directly in supervisor methods, not through this helper. | Delete | Low | No |
| F005 | Dead | Medium | `internal/app/options.go` unused constructors: `WithSubagentModel` (pre-delete lines 81-89), `WithReasoningEffort` (168-176), `WithReasoningSummary` (178-186), `WithWebSearch` (188-193), `WithPriceInPerMTokensUSD` (223-229), `WithPriceOutPerMTokensUSD` (230-236), `WithEnvWebSearch` (237-248) | No activation path found | - Symbol search found only declarations for these constructors.<br>- Actual daemon option wiring in `cmd/agen8/cmd/daemon.go:28-40` does not pass them. | Delete unused constructors only. | Low | No |
| F006 | Dead | Low | `internal/app/team_daemon.go` `ptrNowUTC` (pre-delete lines 243-246) | No activation path found | - No call sites found for `ptrNowUTC`.<br>- Removing it did not affect runtime behavior/tests. | Delete | Low | No |
| F007 | Dead | Low | `internal/app/team_daemon.go` `newTeamOrderedEmitter` (pre-delete lines 248-269) | No activation path found | - No call sites found for `newTeamOrderedEmitter`.<br>- Team daemon runtime path already uses other emit/broadcast setup; this helper was detached. | Delete | Low | No |
| F008 | Incomplete | Medium | `internal/app/code_exec_security.go` `emitCodeExecProvisioningSecurityWarning`, `boolToString`, `emitCodeExecConfigWarning` (pre-delete lines 10-38) | No activation path found | - `emitCodeExecProvisioningSecurityWarning` was explicit no-op (`_ = ctx/_ = cfg/_ = emit`).<br>- Other helpers had no runtime call sites and only dead/no-op tests. | Delete dead scaffold + dead tests. | Low | No |
| F009 | Incomplete | Medium | `cmd/agen8/cmd/dashboard_command.go` `dashboardInterval` at lines `18`, `167` | `agen8 dashboard --interval` | - `dashboardInterval` appears only in declaration and flag binding.<br>- `runDashboardFlow`/`dashboardtui.Run` path does not read/pass this value, so flag is accepted but ignored. | Leave alone for now; requires explicit approval to alter user-visible CLI behavior. | Med | Requires Approval |

SECTION 3 — Deletion + quarantine plan (staged)

- Stage 1: Safe deletes (provably dead, low risk)
  - Apply F001, F002, F003, F004, F005, F006, F007, F008.
  - Verification per patch: targeted tests first, then `go test ./...` and `go vet ./...`.
- Stage 2: Quarantine experiments
  - No quarantine needed after Stage 1; all implemented items were fully dead and safely removable.
- Stage 3: Consolidate duplicates (proposal only)
  - No duplicate consolidation executed in this pass.
- Stage 4: Incomplete features (proposal only)
  - F009 remains intentionally unchanged pending approval.
  - Next options (requires approval): wire `--interval` into live dashboard refresh or deprecate/remove flag.

SECTION 4 — Implementation (safe only)

Patch 1
- Commit-style title: `cmd: remove dead legacy coordinator shell stack`
- Feature IDs addressed: F001
- What changed + why:
  - Removed dead coordinator shell implementation in `cmd/agen8/cmd/coordinator_shell.go`.
  - Kept only `isRetryableLiveError`, which is still used by live-follow flows.
  - This removes unreachable command logic without behavior change.
- Exact file edits:
  - `cmd/agen8/cmd/coordinator_shell.go`: removed `coordinatorSessionState` and dead command handlers; retained shared retry helper.
- Verification steps:
  - `go test ./cmd/agen8/cmd ./internal/tui/coordinator`
  - `go test ./...`
  - `go vet ./...`

Patch 2
- Commit-style title: `cmd/app: delete dead run-filter and lifecycle helpers`
- Feature IDs addressed: F002, F004, F006, F007
- What changed + why:
  - Removed unused `filterRunIDsByRole`.
  - Deleted dead `internal/app/task_lifecycle.go`.
  - Removed unused `ptrNowUTC` and `newTeamOrderedEmitter`.
  - This reduces disconnected helper surface and dead maintenance burden.
- Exact file edits:
  - `cmd/agen8/cmd/events_commands.go`: removed `filterRunIDsByRole`.
  - `internal/app/task_lifecycle.go`: deleted.
  - `internal/app/team_daemon.go`: removed two dead helpers and cleaned unused import.
- Verification steps:
  - `go test ./cmd/agen8/cmd ./internal/app`
  - `go test ./...`
  - `go vet ./...`

Patch 3
- Commit-style title: `app: remove unused ProtocolInitializer abstraction`
- Feature IDs addressed: F003
- What changed + why:
  - Removed unused `ProtocolInitializer` struct and methods.
  - Preserved `shouldEnableProtocolStdio`, which is still used and tested.
  - Eliminates overbuilt abstraction with no runtime wiring.
- Exact file edits:
  - `internal/app/daemon_protocol_initializer.go`: reduced to `shouldEnableProtocolStdio`.
- Verification steps:
  - `go test ./internal/app`
  - `go test ./...`
  - `go vet ./...`

Patch 4
- Commit-style title: `app: remove unused RunChatOption constructors`
- Feature IDs addressed: F005
- What changed + why:
  - Deleted only unused option constructor functions.
  - Kept `RunChatOptions` fields and active constructors untouched.
  - Removed dead config API surface that was never called.
- Exact file edits:
  - `internal/app/options.go`: deleted seven unused constructor funcs; removed now-unused `strconv` import.
- Verification steps:
  - `go test ./internal/app ./cmd/agen8/cmd`
  - `go test ./...`
  - `go vet ./...`

Patch 5
- Commit-style title: `app: remove dead code-exec security no-op scaffold`
- Feature IDs addressed: F008
- What changed + why:
  - Deleted dead/no-op code-exec security helpers and dead tests tied only to no-op behavior.
  - No runtime caller depended on these functions.
- Exact file edits:
  - `internal/app/code_exec_security.go`: deleted.
  - `internal/app/code_exec_security_test.go`: deleted.
- Verification steps:
  - `go test ./internal/app`
  - `go test ./...`
  - `go vet ./...`

Patch 6
- Commit-style title: `docs/audit: publish feature-hygiene report`
- Feature IDs addressed: F001-F009
- What changed + why:
  - Added this report with evidence, staged plan, safe-delete implementation, and explicit non-implemented approval-gated item (F009).
- Exact file edits:
  - `docs/feature-hygiene-audit.md`: added.
- Verification steps:
  - Report includes required SECTION 1-6 structure and approval labels.

SECTION 5 — Verification checklist

- Commands run:
  - Baseline: `go test ./...`, `go vet ./...`.
  - After each patch: targeted package tests, then full `go test ./...` and `go vet ./...`.
- Manual smoke checks to run:
  - `agen8 --help`
  - `agen8 daemon --help`
  - `agen8 coordinator --help`
  - `agen8 logs --help`
  - `agen8 dashboard --help`
- Risks to watch in CI:
  - Hidden references in integration-only build tags.
  - Command registration drift from dead code deletion in `cmd` package.
  - User-visible behavior changes around CLI flags (none intended in this patch set).

SECTION 6 — Self-critique

- What I avoided deleting and why:
  - F009 (`dashboard --interval`) was intentionally not changed because it is a public CLI flag; removing/wiring it changes user-visible behavior and needs approval.
- Where evidence was weak:
  - No weak-evidence deletions were implemented; all removals had direct no-callsite proof plus missing activation path proof.
- Next-best steps for unclear/incomplete items:
  - Add a focused CLI test asserting whether `--interval` impacts dashboard refresh.
  - Decide one of two approved outcomes for F009: functional wiring into dashboard live loop, or explicit flag deprecation/removal with release note.
