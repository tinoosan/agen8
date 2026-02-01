# Troubleshooting Workbench

This guide captures common issues and diagnostics that surface when running Workbench locally.

## General checks

- **Permissions**: Ensure `WORKBENCH_DATA_DIR` (or default `~/.workbench`) is writable by your user; otherwise the CLI cannot persist runs/sessions.
- **Go toolchain**: `go version` should match the version declared in `go.mod`. Rebuild the CLI if dependencies change.
- **Environment variables**: Unset conflicting vars (e.g., `WORKBENCH_WORKDIR` pointing to a nonexistent path) before running the CLI.

## Session/run issues

- **Stuck run/resume**: Use `./workbench show run <runId>` to inspect `status`, `error`, or `lastTrace`. Check `<dataDir>/runs/<runId>/log/events.jsonl` for the precise host/agent steps.
- **Missing artifacts**: Look under `<dataDir>/runs/<runId>/artifacts` and `/results` to verify tool outputs. If paths are missing, inspect agent operations via the TUI history or logs.
- **Context truncation**: Reduce `--context-bytes` or `--history-pairs`, then start a fresh run. Context overruns show up in trace logs (look for `PromptUpdater` warnings in `log/events.jsonl`).

## Configuration problems

- **Data directory not honored**: CLI flags > env vars > defaults. Explicitly pass `--data-dir` when starting Workbench or set `WORKBENCH_DATA_DIR`. Use `./workbench --help` to confirm recognized flags.
- **Workdir mismatch**: `--workdir` controls the `/project` mount. Verify the host path exists before launching runs; otherwise the mount fails and the CLI logs an error.

## Retries and timeouts

- **Retries**: LLM calls use exponential backoff (see `pkg/llm/retry`). Tool runs and store writes are not retried; they use a single attempt with timeout where applicable.
- **Timeouts**: Tool invocations respect the `timeoutMs` passed in the host op; the runtime does not add retries for transient tool failures.

## Advanced diagnostics

- **Trace budget exceeded**: When trace budget (`--trace-bytes`) is exhausted, the agent may stop writing further trace output. Increase the flag or disable tracing to continue.
- **Memory budget issues**: If the agent stops due to memory injection limits, raise `--memory-bytes` or inspect `/memory` proposals stored under `<dataDir>/memory`.
- **Approval gating**: With `--approvals-mode enabled`, every tool/action needs explicit approval. Switch to `disabled` temporarily while reproducing issues.

## When to seek help

Collect the following before raising an issue:

1. CLI command and flags you ran.
2. `runId`, `sessionId`, and the timestamps when the problem occurred.
3. Relevant logs under `<dataDir>/runs/<runId>/log/events.jsonl`.
4. Any overrides (`WORKBENCH_DATA_DIR`, `WORKBENCH_WORKDIR`, `OPENROUTER_MODEL`).

Share these details in the issue or in-team discussion for faster triage.
