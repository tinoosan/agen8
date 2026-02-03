# Workbench CLI Usage

The `workbench` binary exposes a Cobra-powered CLI that orchestrates sessions, runs, and data storage. Most commands live under `cmd/workbench/cmd` and surface functionality via intuitive subcommands.

## Common workflows

### 1. Start a new responsive session

```sh
./workbench
```

The CLI boots the default Bubble Tea-powered TUI. Every line you send becomes an agent turn. Built-in capabilities (shell, HTTP, tracing, etc.) appear directly in the system prompt.

### 2. Resume progress in an existing session

```sh
./workbench list sessions              # see available session IDs
./workbench resume <sessionId>         # continue the last run in that session
```

Session metadata lives in `workbench.db` under the data directory. Resuming reuses the same goal/context and continues the last run workspace (use `--new-run` to start fresh under `/workspace`).

### 3. Inspect metadata, history, and artifacts

```sh
./workbench show session <sessionId>   # print session metadata (SQLite-backed)
./workbench show history <sessionId>   # dump history as JSONL
./workbench show run <runId>           # inspect run metadata + status
```

Pair these commands with `ls`/`cat` inside the `dataDir` to debug agent behavior, inspect artifacts, or replay the operation log.

## Primary commands

| Command                              | Description                                                          |
| ------------------------------------ | -------------------------------------------------------------------- |
| `workbench`                          | Start a fresh session + run (default).                               |
| `workbench resume <sessionId>`       | Continue the last run in a session (use `--new-run` to start fresh). |
| `workbench list sessions`            | List session IDs along with metadata stored in SQLite.               |
| `workbench list runs <sessionId>`    | Show runs tied to a session, including statuses and timestamps.      |
| `workbench show session <sessionId>` | Print the session metadata.                                          |
| `workbench show run <runId>`         | Print the run metadata.                                              |
| `workbench show history <sessionId>` | Display the session-scoped operation history as JSONL.               |

Run `workbench <command> --help` to get per-command flag details (Cobra auto-generates these descriptions).

## Global flags & environment variables

All flags have equivalent environment variables. CLI flags take precedence over env vars, which in turn override defaults defined in code.

| Flag                    | Env var                         | Default                                       | Description                                                                                                            |
| ----------------------- | ------------------------------- | --------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| `--data-dir`            | `WORKBENCH_DATA_DIR`            | `~/.workbench` or `$XDG_STATE_HOME/workbench` | Base directory for `sessions`, `runs`, `tools`, `memory`, `agent`. Overrides where persistent data resides. |
| `--workdir`             | `WORKBENCH_WORKDIR`             | Current working directory                     | Host directory mounted at `/project` inside the sandbox. Useful for debugging or running in a different workspace.     |
| `--context-bytes`       | —                               | `8*1024`                                      | Byte budget persisted as `run.maxBytesForContext`. Must be > 0. A smaller budget tightens agent memory.                |
| `--model`               | `OPENROUTER_MODEL`              | (model configured by agent/system prompt)     | Default LLM ID for remote requests. Overrides session defaults and updates CLI prompts.                                |
| `--trace-bytes`         | —                               | `8*1024`                                      | Budget for PromptUpdater tracing mode. Limits how much trace data is recorded.                                         |
| `--memory-bytes`        | —                               | `8*1024`                                      | Budget for memory injected via `/memory`. Affects per-step lookback.                                                   |
| `--history-pairs`       | —                               | `8`                                           | Number of recent (user, agent) pairs included from `/history`. Reducing this limits prompt size.                       |
| `--include-history-ops` | `WORKBENCH_INCLUDE_HISTORY_OPS` | `true` (enabled)                              | Whether to include environment/host operations from `/history`. Disable to reduce noise.                               |

Use `effectiveConfig()` in `internal/config` to trace how each runtime flag/env resolves at startup.

## Configuration flow

1. **CLI flags** override everything (including environment variables).
2. **Environment variables** are applied when flags are absent.
3. **Defaults** from `internal/config/config.go` and `effectiveConfig()` fill in missing values.

The CLI constructs `RunChatOptions` from resolved configuration and passes them into `app.RunNewChatTUI`, which manages sessions/runs via `internal/store`. When flags or env vars change, rerun the CLI to refresh the active configuration.

## Troubleshooting tips

- Ensure `$WORKBENCH_DATA_DIR` is writable; otherwise, sessions/runs fail to persist.
- `./workbench show run <agentId>` + `cat <dataDir>/agents/<agentId>/log/events.jsonl` reveal runtime problems.
- If the CLI exits immediately, check `~/.workbench/.trace.json` (if present) for tracing context budget violations.
- Use `./workbench --help` to confirm new flags introduced in newer releases.

For deeper diagnostics, refer to [docs/troubleshooting.md](docs/troubleshooting.md).
