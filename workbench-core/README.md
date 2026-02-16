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

   The Bubble Tea-powered UI treats each user message as an agent turn. The embedded system prompt lists built-in capabilities (shell, HTTP, trace, etc.).

3. **Resume work or inspect state** after exiting the TUI:

   ```sh
   ./workbench list sessions             # show session IDs + metadata
   ./workbench resume <sessionId>        # continue the last run in an existing session
   ./workbench show run <runId>          # view run metadata (JSON)
   ./workbench show history <sessionId>  # print the JSONL operation log
   ```

## The Vision: Kubernetes for Agents

Workbench is designed as a **declarative runtime for autonomous agents**. Just as Kubernetes manages container lifecycles through declarative YAML manifests, Workbench manages agent lifecycles through **Profiles** and **Skills**.

It shifts the paradigm from "building a chain" to "configuring a workstation":

- **Declarative Identity**: define what an agent _is_ (roles, goals, models) in simple YAML.
- **Portable Capabilities**: define what an agent _can do_ in standard Markdown (`SKILL.md`).
- **Autonomous Lifecycles**: specify how often an agent should "wake up" to process its inbox or clean its workspace via `heartbeats`.

## Core concepts

### Agent-as-Config

Workbench treats agents as configuration rather than imperative code. By isolating behavior into profiles and skills, you gain:

- **Portability**: Move an agent profile from your laptop to a server without changing code.
- **Auditability**: Every instruction and capability is version-controllable in plain text.
- **Interoperability**: Skills follow an open standard, allowing them to be shared across different agent instances.

### Agentic File System (AFS)

Workbench exposes a virtual filesystem inside each run. Key mounts include:

- `/project` – your host workspace (defaults to the current working directory; overridable via `--workdir`).
- `/workspace` – agent-local workspace mapped to `dataDir/agents/<agentId>/workspace`.
- `/log` – run event stream and trace excerpts.
- `/skills` – user-defined skills (read `/skills/<skill_name>/SKILL.md`).
- `/plan` – planning workspace (`HEAD.md` + `CHECKLIST.md`).
- `/memory` – shared agent memory (`MEMORY.MD` + daily `YYYY-MM-DD-memory.md` files).

Every command that manipulates project files must operate through this explicit surface, which keeps tooling auditable and reproducible.

### Sessions vs. runs

- **Sessions** (data stored under `dataDir/sessions/<sessionId>`) hold stable context/goal and track the latest run index.
- **Agents** (stored under `dataDir/agents/<agentId>`) represent a single autonomous runtime instance with its workspace (`/workspace`), logs, artifacts, and metadata.

You can resume an existing session with `workbench resume <sessionId>` to continue the last run (use `--new-run` to force a fresh run) and inspect artifacts using the CLI or by exploring the data directory (see [docs/data-layout.md](docs/data-layout.md)).

## Commands & workflows

Most entrypoints live under `cmd/workbench/cmd` and use Cobra. Important workflows include:

| Command                              | Description                                                                                                                                                                        |
| ------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `workbench`                          | Start a new session + run with default context (daemon).                                                                                                                           |
| `workbench monitor`                  | Open the monitoring TUI. Start the daemon first, then run monitor so it attaches to the active agent; use `--agent-id <id>` with the agent ID printed at daemon startup if needed. |
| `workbench resume <sessionId>`       | Continue the last run in a session (use `--new-run` to start fresh).                                                                                                               |
| `workbench list sessions`            | List stored session IDs + metadata.                                                                                                                                                |
| `workbench list runs <sessionId>`    | Show run history for a session (statuses, timestamps).                                                                                                                             |
| `workbench show session <sessionId>` | Dump session metadata.                                                                                                                                                             |
| `workbench show run <runId>`         | Dump run metadata.                                                                                                                                                                 |
| `workbench show history <sessionId>` | Print the operation log as JSONL.                                                                                                                                                  |
| `workbench --help`                   | Get command + flag help (Cobra-generated).                                                                                                                                         |

## Configuration

Runtime configuration resolves in this order: CLI flags → environment variables → defaults.

| Flag                    | Env                             | Description                                                                                                                                     |
| ----------------------- | ------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| `--data-dir`            | `WORKBENCH_DATA_DIR`            | Base directory containing `workbench.db`, `sessions`, `agents`, and shared `memory`. Defaults to `~/.workbench` or `$XDG_STATE_HOME/workbench`. |
| `--workdir`             | `WORKBENCH_WORKDIR`             | Host directory mounted under `/project`. Defaults to the current working directory.                                                             |
| `--context-bytes`       | —                               | How many bytes of context to persist (`run.maxBytesForContext`; default `8*1024`). Must be > 0.                                                 |
| `--model`               | `OPENROUTER_MODEL`              | Default model ID for LLM calls (overrides session defaults).                                                                                    |
| `--trace-bytes`         | —                               | Byte budget for the PromptUpdater trace (default `8*1024`).                                                                                     |
| `--memory-bytes`        | —                               | Memory injection budget per step (default `8*1024`).                                                                                            |
| `--history-pairs`       | —                               | Number of recent (user, agent) pairs included from `/history` (default `8`).                                                                    |
| `--include-history-ops` | `WORKBENCH_INCLUDE_HISTORY_OPS` | Whether to include environment/host operations from `/history` (default: enabled).                                                              |

Helpers in `internal/config/effectiveConfig()` resolve the final configuration before each command runs. See [docs/cli-usage.md](docs/cli-usage.md) for deeper context on flag interactions, environment variables, and examples.

## Email notifications (Gmail OAuth2)

Workbench can send **plain-text** email notifications through Gmail using OAuth2 (XOAUTH2 over SMTP).

### Setup

Workbench loads variables from the real environment first; if missing, it falls back to a `.env` file in your session/workdir root.

Set these environment variables (or put them in `.env`):

```sh
export GMAIL_USER="you@gmail.com"
export GMAIL_FROM="you@gmail.com"                 # optional; defaults to GMAIL_USER

export GOOGLE_OAUTH_CLIENT_ID="..."
export GOOGLE_OAUTH_CLIENT_SECRET="..."
export GOOGLE_OAUTH_REFRESH_TOKEN="..."

# Optional (debug only): use an access token directly instead of refreshing.
export GOOGLE_OAUTH_ACCESS_TOKEN="..."
```

Notes:

- You must create an OAuth client in Google Cloud, enable Gmail access, and generate a refresh token for the `https://mail.google.com/` scope.
- Workbench uses STARTTLS on port 587; implicit TLS on port 465 is not supported.

### Agent usage

The built-in tool name is `email(to, subject, body)`. You can explicitly ask the agent to send an email, or configure autonomous mode to send completion summaries.

If SMTP is not configured, email requests fail with a clear error and the agent continues normally.

## Inspecting runtime state

The CLI stores persistent state under the configured `dataDir`:

- `dataDir/workbench.db` (sessions, runs, events, history).
- `dataDir/agents/<agentId>/` (containing `workspace`, `artifacts`, `log`, `inbox`, `outbox`).
- `dataDir/memory/` (shared memory across runs: `MEMORY.MD`, plus daily `YYYY-MM-DD-memory.md` files).

Refer to [docs/data-layout.md](docs/data-layout.md) for a guided walkthrough, sample commands, and tips on manually inspecting sessions, runs, and agent mounts.

## Troubleshooting

- Logs live under `dataDir/agents/<agentId>/log` (JSON/trace artifacts). Use `./workbench show run <agentId>` to understand failure reasons.
- Re-run `./workbench resume <sessionId>` with `--context-bytes`/`--trace-bytes` overrides to debug context issues.
- The [Troubleshooting guide](docs/troubleshooting.md) covers common problems (build issues, stuck runs, missing artifacts) and quick remediations.

## Developer resources

- Inspect `internal/app` for session runtime wiring and `internal/store` for persistence logic.
- The [Developer guide](docs/developer-guide.md) explains how configuration, session/run lifecycles, and telemetry hooks fit together.
- The [Execution model](docs/execution-model.md) (PRD) defines sub-agents, teams, hierarchy, review gate, retry/escalation, and daemon responsibilities; it is the authoritative spec for orchestration behaviour.
- Inspect `pkg/agent/hosttools` for the built-in host tool surface and `pkg/tools/builtins` for host-side implementations.

Contributions are welcome—submit documentation fixes or enhancements alongside code changes to keep the docs aligned with evolving runtime behavior.
