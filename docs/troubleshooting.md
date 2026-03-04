# Troubleshooting Agen8

This guide captures common issues and diagnostics that surface when running Agen8 locally.

## General checks

- **Permissions**: Ensure `AGEN8_DATA_DIR` (or default `~/.agen8`) is writable by your user; otherwise the CLI cannot persist agents/sessions.
- **Go toolchain**: `go version` should match the version declared in `go.mod`. Rebuild the CLI if dependencies change.
- **Environment variables**: Unset conflicting vars (e.g., `AGEN8_WORKDIR` pointing to a nonexistent path) before running the CLI.

## Session/run issues

- **Stuck agent/resume**: Use `./agen8 show run <agentId>` to inspect `status`, `error`, or `lastTrace`. Check `<dataDir>/agents/<agentId>/log/events.jsonl` for the precise host/agent steps.
- **Missing artifacts**: Look under `<dataDir>/agents/<agentId>/workspace` and `<dataDir>/agents/<agentId>/artifacts`. If paths are missing, inspect agent operations via the TUI history or logs.
- **Context truncation**: Reduce `--context-bytes` or `--history-pairs`, then start a fresh run. Context overruns show up in trace logs (look for `PromptUpdater` warnings in `log/events.jsonl`).

## Configuration problems

- **Data directory not honored**: CLI flags > env vars > defaults. Explicitly pass `--data-dir` when starting Agen8 or set `AGEN8_DATA_DIR`. Use `./agen8 --help` to confirm recognized flags.
- **Workdir mismatch**: `--workdir` controls the `/project` mount. Verify the host path exists before launching runs; otherwise the mount fails and the CLI logs an error.

## Retries and timeouts

- **Retries**: LLM calls use exponential backoff (see `pkg/llm/retry`). Tool runs and store writes are not retried; they use a single attempt with timeout where applicable.
- **Timeouts**: Tool invocations respect the `timeoutMs` passed in the host op; the runtime does not add retries for transient tool failures.

## Advanced diagnostics

- **Trace budget exceeded**: When trace budget (`--trace-bytes`) is exhausted, the agent may stop writing further trace output. Increase the flag or disable tracing to continue.
- **Memory budget issues**: If the agent stops due to memory injection limits, raise `--memory-bytes` or inspect `/memory` proposals stored under `<dataDir>/memory`.

## fs_patch diagnostics

- **Validate first with dry-run**: use `tools.fs_patch(path=\"...\", text=\"...\", dryRun=true, verbose=true)` to check hunks without writing.
- **Failure fields**: when a patch fails, inspect `patchFailureReason`, `patchFailedHunk`, `patchTargetLine`, `patchHunkHeader`, `patchExpectedContext`, `patchActualContext`, and `patchSuggestion` in activity details/events.
- **Recommended loop**:
  1. `dryRun=true` to validate.
  2. If failure, re-open the target file and adjust context lines/hunk header.
  3. Re-run dry-run until clean.
  4. Re-run with `dryRun=false` to apply.

## fs_write verification/checksum

- **Verify critical writes**: use `tools.fs_write(path="...", text="...", verify=true)` to force read-back validation.
- **Integrity hashes**: set `checksum` to `md5`, `sha1`, or `sha256` to get a deterministic digest in the response/activity details.
- **Checksum parameter names**:
  - Canonical form:
    - `checksum`: algorithm name (`md5`, `sha1`, `sha256`)
    - `checksumExpected`: expected hex digest for the selected algorithm
  - Compatibility aliases (equivalent to setting `checksum` + `checksumExpected`):
    - `checksumMd5`
    - `checksumSha1`
    - `checksumSha256`
  - Rules:
    - Use either canonical fields or one alias field.
    - If both are set, they must agree on algorithm and digest.
- **Mismatch triage**: on verification failure, inspect `writeMismatchAt`, `writeExpectedBytes`, and `writeActualBytes` fields.
- **Recommended loop**:
  1. Run with `verify=true` (+ `checksum="sha256"` for audit trails).
  2. If mismatch occurs, re-read the file and compare against source payload.
  3. Re-write and verify again before downstream operations.

Example (canonical):

```python
tools.fs_write(
    path="/workspace/file.txt",
    text="Hello World",
    checksum="sha256",
    checksumExpected="a591a6d40bf420404a011733cfb7b190d62c65bf0bcda32b57b277d9ad9f146e",
)
```

Example (alias):

```python
tools.fs_write(
    path="/workspace/file.txt",
    text="Hello World",
    checksumMd5="b10a8db164e0754105b7a99be72e3fe5",
)
```

## When to seek help

Collect the following before raising an issue:

1. CLI command and flags you ran.
2. `runId`, `sessionId`, and the timestamps when the problem occurred.
3. Relevant logs under `<dataDir>/agents/<agentId>/log/events.jsonl`.
4. Any overrides (`AGEN8_DATA_DIR`, `AGEN8_WORKDIR`, `OPENROUTER_MODEL`).

Share these details in the issue or in-team discussion for faster triage.
