# Agen8 CLI Usage

The `agen8` binary exposes a Cobra-powered CLI that orchestrates sessions, runs, and data storage. Most commands live under `cmd/agen8/cmd` and surface functionality via intuitive subcommands.

## Common workflows

### Modular workflow (recommended)

```sh
./agen8 init                            # initialize .agent8 in this project
./agen8 new --mode team --profile startup_team
./agen8 coordinator                     # focused coordinator chat/attach
./agen8 dashboard                       # high-level observability
./agen8 logs --follow                   # structured event stream
./agen8 activity --follow               # live activity stream
./agen8 sessions list                   # session lifecycle visibility
```

### 1. Start a new responsive session

```sh
./agen8
```

The CLI boots the default Bubble Tea-powered TUI. Every line you send becomes an agent turn. Built-in capabilities (shell, HTTP, tracing, etc.) appear directly in the system prompt.

### 2. Resume progress in an existing session

```sh
./agen8 list sessions              # see available session IDs
./agen8 resume <sessionId>         # continue the last run in that session
```

Session metadata lives in `agen8.db` under the data directory. Resuming reuses the same goal/context and continues the last run workspace (use `--new-run` to start fresh under `/workspace`).

### 3. Inspect metadata, history, and artifacts

```sh
./agen8 show session <sessionId>   # print session metadata (SQLite-backed)
./agen8 show history <sessionId>   # dump history as JSONL
./agen8 show run <runId>           # inspect run metadata + status
```

Pair these commands with `ls`/`cat` inside the `dataDir` to debug agent behavior, inspect artifacts, or replay the operation log.

## Primary commands

| Command                              | Description                                                          |
| ------------------------------------ | -------------------------------------------------------------------- |
| `agen8`                          | Start a fresh session + run (default).                               |
| `agen8 init`                     | Initialize a project-local `.agent8` workspace.                      |
| `agen8 new`                      | Create a new session and optionally auto-attach.                     |
| `agen8 coordinator`              | Attach to the coordinator-focused session view.                      |
| `agen8 attach <session-id>`      | Attach to a specific existing session.                               |
| `agen8 dashboard`                | Show read-only overview of sessions/runs/tasks/cost.                 |
| `agen8 logs`                     | Query structured logs/events with filters.                           |
| `agen8 activity`                 | Tail live activity stream.                                           |
| `agen8 sessions`                 | List/manage sessions (`list|attach|pause|resume|stop|delete`).       |
| `agen8 resume <sessionId>`       | Continue the last run in a session (use `--new-run` to start fresh). |
| `agen8 list sessions`            | List session IDs along with metadata stored in SQLite.               |
| `agen8 list runs <sessionId>`    | Show runs tied to a session, including statuses and timestamps.      |
| `agen8 show session <sessionId>` | Print the session metadata.                                          |
| `agen8 show run <runId>`         | Print the run metadata.                                              |
| `agen8 show history <sessionId>` | Display the session-scoped operation history as JSONL.               |

Run `agen8 <command> --help` to get per-command flag details (Cobra auto-generates these descriptions).

## Migration note

- Existing `agen8 monitor` behavior is retained for compatibility.
- New modular commands are built on the same daemon/RPC control plane, so observability remains available while enabling tmux-friendly workflows.

## Global flags & environment variables

All flags have equivalent environment variables. CLI flags take precedence over env vars, which in turn override defaults defined in code.

| Flag                    | Env var                         | Default                                       | Description                                                                                                            |
| ----------------------- | ------------------------------- | --------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| `--data-dir`            | `AGEN8_DATA_DIR`            | `~/.agen8` or `$XDG_STATE_HOME/agen8` | Base directory for `sessions`, `runs`, `tools`, `memory`, `agent`. Overrides where persistent data resides. |
| `--workdir`             | `AGEN8_WORKDIR`             | Current working directory                     | Host directory mounted at `/project` inside the sandbox. Useful for debugging or running in a different workspace.     |
| `--context-bytes`       | —                               | `8*1024`                                      | Byte budget persisted as `run.maxBytesForContext`. Must be > 0. A smaller budget tightens agent memory.                |
| `--model`               | `OPENROUTER_MODEL`              | (model configured by agent/system prompt)     | Default LLM ID for remote requests. Overrides session defaults and updates CLI prompts.                                |
| `--trace-bytes`         | —                               | `8*1024`                                      | Budget for PromptUpdater tracing mode. Limits how much trace data is recorded.                                         |
| `--memory-bytes`        | —                               | `8*1024`                                      | Budget for memory injected via `/memory`. Affects per-step lookback.                                                   |
| `--history-pairs`       | —                               | `8`                                           | Number of recent (user, agent) pairs included from `/history`. Reducing this limits prompt size.                       |
| `--include-history-ops` | `AGEN8_INCLUDE_HISTORY_OPS` | `true` (enabled)                              | Whether to include environment/host operations from `/history`. Disable to reduce noise.                               |

Use `effectiveConfig()` in `internal/config` to trace how each runtime flag/env resolves at startup.

## Configuration flow

1. **CLI flags** override everything (including environment variables).
2. **Environment variables** are applied when flags are absent.
3. **Defaults** from `internal/config/config.go` and `effectiveConfig()` fill in missing values.

The CLI constructs `RunChatOptions` from resolved configuration and passes them into `app.RunNewChatTUI`, which manages sessions/runs via `internal/store`. When flags or env vars change, rerun the CLI to refresh the active configuration.

For daemon runtime `config.toml`, API key onboarding/keychain behavior, and headless server setup, see [docs/config-toml.md](docs/config-toml.md).

## Troubleshooting tips

- Ensure `$AGEN8_DATA_DIR` is writable; otherwise, sessions/runs fail to persist.
- `./agen8 show run <agentId>` + `cat <dataDir>/agents/<agentId>/log/events.jsonl` reveal runtime problems.
- If the CLI exits immediately, check `~/.agen8/.trace.json` (if present) for tracing context budget violations.
- Use `./agen8 --help` to confirm new flags introduced in newer releases.

For deeper diagnostics, refer to [docs/troubleshooting.md](docs/troubleshooting.md).
