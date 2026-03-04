# Agen8 Developer Guide

This guide explains how the CLI, configuration, and internal services collaborate to manage sessions and agents. For the **authoritative execution model** (sub-agents, teams, hierarchy, retry, escalation, cleanup), see [Execution model (PRD)](execution-model.md).

## Architecture overview

- `cmd/agen8/cmd` contains Cobra commands that parse CLI flags, resolve configuration, and invoke `app.RunDaemon` / `tui.RunMonitor`.
- `internal/config` exposes `effectiveConfig()` which merges CLI flags, environment variables, and defaults. Components call this helper before starting new sessions/runs.
- `internal/app` orchestrates the autonomous daemon session and wires agents into the runtime. The daemon enforces the [execution model](execution-model.md): tree-based hierarchy, single-parent sub-agents, review gate, cleanup, and RPC-level coordination so that TUI and future Web UI share the same semantics.
- **Session service** (`pkg/services/session`): the daemon and RPC access session and run data only through this service (load/save session, start/delete session, runs, activities). See [Session Service (architecture)](architecture/pkg-services-session.md).
- `internal/store` persists sessions, agents, history, and artifacts under the configured `dataDir`; only the session service Manager and the runtime supervisor use it for session/run data.
- `pkg/fsutil` provides helper functions to resolve deterministic paths (`GetAgentDir`, `GetWorkspaceDirForRun`, etc.).

## Session/run lifecycle

1. CLI command resolves configuration and calls `app.RunDaemon`.
2. `app` creates a session and an agent ID; `internal/store` manages directories under `<dataDir>/sessions` and `<dataDir>/agents`.
3. Each agent instance mounts (`/project`, `/workspace`, `/log`, `/memory`, `/skills`, `/plan`) and processes tasks from SQLite-backed routing. Sub-agent runs use a workspace under the parent’s workspace (`workspace/subagents/<childRunId>/`) so artifacts are attributable and the parent can review them; see [Data layout](data-layout.md#sub-agent-workspace).
4. Artifact outputs (logs, workspace files) stream into the agent directory and can be inspected via the CLI or host filesystem.

## Execution hierarchy (summary)

- **Standalone:** one root agent; it may spawn sub-agents. No coordinator.
- **Team:** one coordinator, one or more co-agents. Coordinator delegates to co-agents; co-agents may spawn sub-agents. **Coordinator cannot spawn sub-agents.**
- Sub-agents are leaves: single task (or retries), no spawning, no lateral communication. Parent must approve (review gate) before sub-agent work is considered complete; then cleanup removes the worker while preserving run history and artifacts. Full rules: [Execution model](execution-model.md).

## Tooling & extensions

- Built-in host tools (fs\_\*, shell_exec, http_fetch, trace_run) are defined directly in the system prompt.
- For new filesystem tools, follow [Adding `fs_*` tools](adding_fs_tools.md). `fs_*` tools are auto-allowlisted and do not require profile `allowed_tools` entries.

## Debugging tips for contributors

- Inspect `internal/store` tests/examples to understand how sessions/agents/history are persisted.
- Use `go test ./...` to validate changes; target `internal/store` and `internal/app` when adjusting persistence logic.
- Update `docs/cli-usage.md` and `docs/data-layout.md` whenever new flags or directories are added.
