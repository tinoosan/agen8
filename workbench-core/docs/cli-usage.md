# Workbench CLI Usage

The `workbench` binary exposes a Cobra-powered CLI that orchestrates sessions, runs, and data
storage. Most commands live under `cmd/workbench/cmd`.

## Primary commands

| Command | Description |
| ------- | ----------- |
| `workbench` | Start a new session + run and open the interactive TUI (default). |
| `workbench resume <sessionId>` | Create a new run in an existing session (workspace per run). |
| `workbench list sessions` | List session IDs along with metadata stored under `<dataDir>/sessions`. |
| `workbench list runs <sessionId>` | Show the runs inside a session with their statuses. |
| `workbench show session <sessionId>` | Print the session JSON metadata. |
| `workbench show run <runId>` | Print the run JSON metadata. |
| `workbench show history <sessionId>` | Display the history JSONL for a session (operation log). |

## Global Flags

| Flag | Environment | Description |
| ---- | ----------- | ----------- |
| `--data-dir` | `WORKBENCH_DATA_DIR` | Base directory for `sessions`, `runs`, `tools`, `profile`, `agent`. Defaults to `~/.workbench` or `$XDG_STATE_HOME/workbench`. |
| `--workdir` | `WORKBENCH_WORKDIR` | Host directory mounted under `/project`. Defaults to current working directory. |
| `--context-bytes` | — | Controls `run.maxBytesForContext` persisted in `run.json`. Must be > 0; default `8*1024`. |
| `--title` | — | Default title for new sessions (default `workbench`). |
| `--goal` | — | Default goal for new sessions (default `interactive chat`). |
| `--mouse` | `WORKBENCH_MOUSE` | Enable Bubble Tea mouse capture (opt-in; may disable native selection). |
| `--activity` | `WORKBENCH_ACTIVITY` | Show the activity panel by default within the TUI. |
| `--pricing-file` | `WORKBENCH_PRICING_FILE` | Optional pricing JSON to customize per-model cost estimates. |
| `--model` | `OPENROUTER_MODEL` | Default model ID for LLM requests. Setting this updates CLI prompts and overrides session defaults. |
| `--max-steps` | — | Maximum agent steps per turn (default `200`). |
| `--trace-bytes` | — | Trace budget in bytes for ContextUpdater (default `8*1024`). |
| `--memory-bytes` | — | Memory budget in bytes injected per step (default `8*1024`). |
| `--profile-bytes` | — | Profile budget in bytes (default `4*1024`). |
| `--history-pairs` | — | Number of recent (user, agent) pairs included from `/history` (default `8`). |
| `--user-id` | `WORKBENCH_USER_ID` | Optional stable user identifier recorded in history/events. |
| `--include-history-ops` | `WORKBENCH_INCLUDE_HISTORY_OPS` | If true (default), the prompt contains environment/host operations from `/history`. |
| `--price-in-per-m` | `WORKBENCH_PRICE_IN_PER_M` | USD per 1M input tokens for cost estimation (default 0). |
| `--price-out-per-m` | `WORKBENCH_PRICE_OUT_PER_M` | USD per 1M output tokens for cost estimation (default 0). |
| `--approvals-mode` | `WORKBENCH_APPROVALS_MODE` | Approval policy: `enabled` (default) or `disabled`. |
| `--plan-mode` | `WORKBENCH_PLAN_MODE` | When enabled, the agent must produce a structured plan for the first step. |

## Configuration Flow

1. CLI flags override environment variables (e.g., `--data-dir` > `WORKBENCH_DATA_DIR`).
2. Environment variables used by default when flags are absent.
3. Defaults (in code) only apply when both flag and env value are missing (e.g., `workbench` title/goal defaults).

`cmd/workbench/root.go` wires flags, env helpers, and the `app.RunChatOption` helpers in `internal/app/chat.go`. The CLI constructs `RunChatOptions` and passes them to `app.RunNewChatTUI`, which creates sessions/runs in `internal/store`.
