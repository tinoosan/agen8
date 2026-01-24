# Workbench

Workbench Core is a local, agentic runtime that exposes an interactive CLI for running sessions, resuming previous runs, and inspecting the resulting artifacts. It is built around a virtual filesystem (VFS) model and stores scoped data in a configurable data directory.

## AFS abstraction

Workbench also exposes an Agentic File System (AFS) abstraction—the concrete VFS entries under `/project`, `/scratch`, `/log`, `/memory`, and `/results` (with `/tools` reserved for external/custom tools)—that agents learn to discover and manipulate rather than assuming hidden APIs. Every command that touches project files works through this AFS surface, so tooling remains explicit, auditable, and reproducible.

Each AFS mount has a clear role:

- `/project` maps to the real project workspace and should host user-visible artifacts.
- `/scratch` is temporary scratch space scoped to the run.
- `/tools` exposes discoverable manifests for external/custom tools (builtin tools are defined directly in the system prompt).
- `/log`, `/results`, `/memory`, and `/profile` provide debugging, telemetry, and memory utilities.

## Quick start

```sh
# Build the CLI
go build ./cmd/workbench

# Start a new interactive session
./workbench
```

The default run opens a Bubble Tea-powered TUI where each message you submit becomes an agent turn. Builtin capabilities (shell, HTTP, trace, etc.) are described directly in the system prompt, while `/tools` only surfaces external/custom tools that require discovery via `tool.run`. Agents still interact with run-scoped files under `/scratch`. Every run creates a session and writes results into the configured data directory (default: `~/.workbench` or `$XDG_STATE_HOME/workbench`).

## Commands

Most entrypoints live under `cmd/workbench/cmd` and use Cobra. Key commands are:

- `workbench` – starts a fresh interactive session with configurable context size.
- `workbench resume <sessionId>` – resumes an existing session by creating a new run.
- `workbench list sessions` – lists all session IDs stored under `data`.
- `workbench list runs <sessionId>` – shows runs (and their statuses) for one session.
- `workbench show session <sessionId>` – prints the `session.json` metadata for a session.
- `workbench show history <sessionId>` – emits the recent operation history (JSONL) for debugging.
- `workbench show run <runId>` – prints the `run.json` metadata for a run.

## Configuration

All runtime configuration (currently just `dataDir`) is defined in `internal/config/config.go`. The CLI supports:

- `--data-dir` – base directory where runs, sessions, results, workspace, and history live (priority: `--data-dir`, env `WORKBENCH_DATA_DIR`, default: `~/.workbench` or `$XDG_STATE_HOME/workbench`).
- `--workdir` / `WORKBENCH_WORKDIR` – override the directory mounted at `/project` inside the sandbox.
- `--context-bytes` – limits the token context saved per run (must be > 0).

The `effectiveConfig()` helper resolves the final config (including `dataDir`) before each command runs.

## Project layout

```
cmd/workbench/
  cmd/          # Cobra subcommands
internal/       # core services: app, store, tools, config, history
go.mod          # module definition and dependencies
```

You can inspect `internal/app` for the runtime orchestration (chat sessions, TUI hooks) and `internal/store` for how sessions/runs/history are persisted.

## Advantages

- **Control:** Runs entirely locally with configurable `dataDir`/`workdir`, so nothing depends on external services.
- **Transparency:** Every session and run is stored in the data directory, and you can inspect history, artifacts, or metadata via the CLI commands.
- **Reproducibility:** Sessions create structured state (`session`, `run`, `/scratch`, `/history`), making it easy to resume, replay, or audit work.
- **Explicit tooling:** Builtin capabilities (shell, HTTP, trace) are spelled out in the system prompt, and `/tools` is reserved for external/custom tools that must be discovered and invoked with `tool.run`, keeping integrations transparent.

## Further reading

- [`docs/data-layout.md`](docs/data-layout.md) – detailed runtime data directory layout (sessions, runs, agent mounts).
- [`docs/cli-usage.md`](docs/cli-usage.md) – command/flag reference for the `workbench` CLI and environment variables.
