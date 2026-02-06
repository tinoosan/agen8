# Workbench Developer Guide

This guide explains how the CLI, configuration, and internal services collaborate to manage sessions and agents.

## Architecture overview

- `cmd/workbench/cmd` contains Cobra commands that parse CLI flags, resolve configuration, and invoke `app.RunDaemon` / `tui.RunMonitor`.
- `internal/config` exposes `effectiveConfig()` which merges CLI flags, environment variables, and defaults. Components call this helper before starting new sessions/runs.
- `internal/app` orchestrates the autonomous daemon session and wires agents into the runtime.
- `internal/store` persists sessions, agents, history, and artifacts under the configured `dataDir`.
- `pkg/fsutil` provides helper functions to resolve deterministic paths (`GetAgentDir`, etc.).

## Session/run lifecycle

1. CLI command resolves configuration and calls `app.RunDaemon`.
2. `app` creates a session and an agent ID; `internal/store` manages directories under `<dataDir>/sessions` and `<dataDir>/agents`.
3. Each agent instance mounts (`/project`, `/workspace`, `/log`, `/memory`, `/skills`, `/plan`, `/inbox`, `/outbox`) and processes tasks from `/inbox`.
4. Artifact outputs (logs, workspace files) stream into the agent directory and can be inspected via the CLI or host filesystem.

## Tooling & extensions

- Built-in host tools (fs_*, shell_exec, http_fetch, trace_run) are defined directly in the system prompt.

## Debugging tips for contributors

- Inspect `internal/store` tests/examples to understand how sessions/agents/history are persisted.
- Use `go test ./...` to validate changes; target `internal/store` and `internal/app` when adjusting persistence logic.
- Update `docs/cli-usage.md` and `docs/data-layout.md` whenever new flags or directories are added.
