# Workbench Developer Guide

This guide explains how the CLI, configuration, and internal services collaborate to manage sessions and runs.

## Architecture overview

- `cmd/workbench/cmd` contains Cobra commands that parse CLI flags, resolve configuration, and invoke `app.RunNewChatTUI`.
- `internal/config` exposes `effectiveConfig()` which merges CLI flags, environment variables, and defaults. Components call this helper before starting new sessions/runs.
- `internal/app` orchestrates chat sessions (user/agent turns) and wires agents into the TUI/runtime.
- `internal/store` persists sessions, runs, history, and artifacts under the configured `dataDir`.
- `internal/fsutil` provides helper functions to resolve deterministic paths (`GetRunDir`, `GetArtifactDir`, etc.).

## Session/run lifecycle

1. CLI command resolves configuration and calls `app.RunNewChatTUI` (or a similar helper for `resume`).
2. `app` creates a session (if needed) and `internal/store` manages directories under `<dataDir>/sessions` and `<dataDir>/runs`.
3. Each run spawns a sandbox with mounts (`/project`, `/workspace`, `/log`, `/memory`, `/results`, `/tools`). `/memory` is shared across runs; agents interact with `/workspace` and request tools via `tool.run`.
4. Artifact outputs (logs, artifacts, results) stream into the run directory and can be inspected via the CLI or host filesystem.

## Tooling & extensions

- Custom tools live under `<dataDir>/tools`. The agent discovers them through `/tools` manifests.
- Builtin tools (shell, HTTP, trace) are defined directly in the system prompt and do not require manifest discovery.

## Debugging tips for contributors

- Inspect `internal/store` tests/examples to understand how sessions/runs/history are persisted.
- Use `go test ./...` to validate changes; target `internal/store` and `internal/app` when adjusting persistence logic.
- Update `docs/cli-usage.md` and `docs/data-layout.md` whenever new flags or directories are added.
