# Workbench

Workbench Core is a local agentic runtime that exposes an interactive CLI for launching sessions, resuming previous runs, and inspecting every artifact the agent creates. It builds on a virtual filesystem (VFS) abstraction so tooling remains explicit, auditable, and reproducible.

## Table of Contents

- [Quick start](#quick-start)
- [Core concepts](#core-concepts)
- [Commands & workflows](#commands--workflows)
- [Configuration](#configuration)
- [Inspecting runtime state](#inspecting-runtime-state)
- [Troubleshooting](#troubleshooting)
- [Developer resources](#developer-resources)

## Quick start

1. **Build** the CLI (Go toolchain required):

   ```sh
   go build ./cmd/workbench
   ```

2. **Start a fresh interactive session**:

   ```sh
   ./workbench
   ```

   The Bubble Tea-powered UI treats each user message as an agent turn. The embedded system prompt lists builtin capabilities (shell, HTTP, trace, etc.), while `/tools` exposes discoverable external tools via `tool.run`.

3. **Resume work or inspect state** after exiting the TUI:

   ```sh
   ./workbench list sessions             # show session IDs + metadata
   ./workbench resume <sessionId>        # start a new run in an existing session
   ./workbench show run <runId>          # view run metadata (JSON)
   ./workbench show history <sessionId>  # print the JSONL operation log
   ```

## Core concepts

### Agentic File System (AFS)

Workbench exposes a virtual filesystem inside each run. Key mounts include:

- `/project` – your host workspace (defaults to the current working directory; overridable via `--workdir`).
- `/scratch` – run-local workspace mapped to `dataDir/runs/<runId>/scratch`.
- `/results`, `/log`, `/memory`, `/profile`, `/tools` – mounted read-only to expose structured outputs, telemetry, memory proposals, and discovered tool manifests.

Every command that manipulates project files must operate through this explicit surface, which keeps tooling auditable and reproducible.

### Sessions vs. runs

- **Sessions** (data stored under `dataDir/sessions/<sessionId>`) hold stable context/goal and track the latest run index.
- **Runs** (stored under `dataDir/runs/<runId>`) represent a single agent execution with its workspace (`/scratch`), logs, artifacts, and metadata.

You can resume an existing session with `workbench resume <sessionId>` and inspect artifacts using the CLI or by exploring the data directory (see [docs/data-layout.md](docs/data-layout.md)).

## Commands & workflows

Most entrypoints live under `cmd/workbench/cmd` and use Cobra. Important workflows include:

| Command | Description |
| ------- | ----------- |
| `workbench` | Start a new session + run with default context. |
| `workbench resume <sessionId>` | Open a run that reuses session context. |
| `workbench list sessions` | List stored session IDs + metadata. |
| `workbench list runs <sessionId>` | Show run history for a session (statuses, timestamps). |
| `workbench show session <sessionId>` | Dump session metadata (`session.json`). |
| `workbench show run <runId>` | Dump run metadata (`run.json`). |
| `workbench show history <sessionId>` | Print the operation log (`history/history.jsonl`). |
| `workbench --help` | Get command + flag help (Cobra-generated). |

## Configuration

Runtime configuration resolves in this order: CLI flags → environment variables → defaults.

| Flag | Env | Description |
| ---- | --- | ----------- |
| `--data-dir` | `WORKBENCH_DATA_DIR` | Base directory containing `sessions`, `runs`, `tools`, `profile`, `agent`. Defaults to `~/.workbench` or `$XDG_STATE_HOME/workbench`. |
| `--workdir` | `WORKBENCH_WORKDIR` | Host directory mounted under `/project`. Defaults to the current working directory. |
| `--context-bytes` | — | How many bytes of context to persist (`run.maxBytesForContext`; default `8*1024`). Must be > 0. |
| `--model` | `OPENROUTER_MODEL` | Default model ID for LLM calls (overrides session defaults). |
| `--trace-bytes` | — | Byte budget for the ContextUpdater trace (default `8*1024`). |
| `--memory-bytes` | — | Memory injection budget per step (default `8*1024`). |
| `--profile-bytes` | — | Budget for profiling data (default `4*1024`). |
| `--history-pairs` | — | Number of recent (user, agent) pairs included from `/history` (default `8`). |
| `--include-history-ops` | `WORKBENCH_INCLUDE_HISTORY_OPS` | Whether to include environment/host operations from `/history` (default: enabled). |
| `--approvals-mode` | `WORKBENCH_APPROVALS_MODE` | Approval policy: `enabled` (default) or `disabled`. |
| `--plan-mode` | `WORKBENCH_PLAN_MODE` | When enabled, the agent must produce a structured plan for the first step. |

Helpers in `internal/config/effectiveConfig()` resolve the final configuration before each command runs. See [docs/cli-usage.md](docs/cli-usage.md) for deeper context on flag interactions, environment variables, and examples.

## Inspecting runtime state

The CLI stores persistent state under the configured `dataDir`:

- `dataDir/sessions/<sessionId>/session.json` (metadata) and `/history/history.jsonl` (operation log).
- `dataDir/runs/<runId>/` (containing `run.json`, `events.jsonl`, `scratch`, `artifacts`, `log`, `memory`, `profile`).

Refer to [docs/data-layout.md](docs/data-layout.md) for a guided walkthrough, sample commands, and tips on manually inspecting sessions, runs, and agent mounts.

## Troubleshooting

- Logs live under `dataDir/runs/<runId>/log` (JSON/trace artifacts). Use `./workbench show run <runId>` to understand failure reasons.
- Re-run `./workbench resume <sessionId>` with `--context-bytes`/`--trace-bytes` overrides to debug context issues.
- The [Troubleshooting guide](docs/troubleshooting.md) covers common problems (build issues, stuck runs, missing artifacts) and quick remediations.

## Developer resources

- Inspect `internal/app` for session orchestrators and `internal/store` for persistence logic.
- The [Developer guide](docs/developer-guide.md) explains how configuration, session/run lifecycles, and telemetry hooks fit together.
- Use `/tools` manifests and `tool.run` to extend Workbench with custom tooling following the AFS expectations.

Contributions are welcome—submit documentation fixes or enhancements alongside code changes to keep the docs aligned with evolving runtime behavior.
