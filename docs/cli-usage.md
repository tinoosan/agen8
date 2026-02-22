# Agen8 CLI Usage

The `agen8` binary exposes a Cobra-powered CLI for managing sessions, runs, and diagnostics. Most commands live under `cmd/agen8/cmd` and follow the same configuration/resolution order described in [docs/config-toml.md](docs/config-toml.md).

## Quick start

1. **Build the CLI** (Go 1.24+ toolchain required):

   ```sh
   go build ./cmd/agen8
   ```

2. **Launch an interactive run** (default bubbletea TUI):

   ```sh
   ./agen8
   ```

   Every line you enter becomes an agent turn. The embedded system prompt lists built-in capabilities (shell, HTTP, trace, etc.) and guides what the agent can do inside the VFS mounts.

3. **Inspect or resume after exiting**:

   ```sh
   ./agen8 list sessions             # show session IDs, modes, timestamps
   ./agen8 resume <sessionId>        # continue the latest run; add --new-run for a fresh workspace
   ./agen8 show run <runId>          # inspect run metadata, status, proposals
   ./agen8 show history <sessionId>  # print the JSONL operation log stored in the data directory
   ```

## Recommended workflow

A typical onboarding workflow:

1. `./agen8 init` to create a local `.agen8` workspace and default config.
2. `./agen8 new --mode team --profile startup_team` to spin up a team session with the desired profile.
3. `./agen8 coordinator` or `./agen8 dashboard` to stay in the observer console while the agent runs.
4. Use `./agen8 logs --follow` / `./agen8 activity --follow` to tail live activity streams.
5. When ready to inspect artifacts, combine `agen8 show run` with `ls`/`cat` inside `dataDir/agents/<agentId>/workspace`.

## Session lifecycle commands

| Command | Purpose |
| ------- | ------- |
| `agen8 init` | Initialize `.agen8` and default configuration.
| `agen8 new --mode <mode> --profile <profile>` | Create a session tuned to the requested mode (standalone, team, coordinator, etc.).
| `agen8` | Launch a default run using the session/workspace configured in `.agen8`.
| `agen8 resume <sessionId>` | Continue the latest run in a session; add `--new-run` for an isolated new run.
| `agen8 list sessions` | List every stored session with its mode, timestamps, and status.
| `agen8 list runs <sessionId>` | Show run history, durations, and agent IDs for the specified session.

## Observability & coordination commands

| Command | Purpose |
| ------- | ------- |
| `agen8 coordinator` | Attach to a coordinator-focused chat view and oversee subordinate agents.
| `agen8 dashboard` | Read-only observability for sessions, runs, tasks, and cost signals.
| `agen8 monitor` | Observer UI that attaches to a running agent (start the daemon first).
| `agen8 logs` | Query structured logs and filter by `--agent-id`, `--level`, or `--follow`.
| `agen8 activity` | Tail the activity stream of proposals and tool calls.
| `agen8 show session/run/history` | Dump metadata or the JSONL operation log for debugging.

## Support commands

- `agen8 attach <sessionId>` – Attach to an arbitrary session even after the daemon shuts down gracefully.
- `agen8 stop <sessionId>` – Stop a session gracefully while preserving artifacts and history.
- `agen8 --help` – Print Cobra-generated command/flag help.

## Configuration helpers

Runtime configuration resolves in this order: CLI flags → environment variables → `${AGEN8_DATA_DIR}/config.toml` → built-in defaults. Important flags you will see often:

| Flag | Env | Description |
|------|-----|-------------|
| `--data-dir` | `AGEN8_DATA_DIR` | Directory for `agen8.db`, `sessions`, `agents`, and shared `memory`. Defaults to `~/.agen8` or `$XDG_STATE_HOME/agen8`.
| `--workdir` | `AGEN8_WORKDIR` | Host directory mounted as `/project`.
| `--context-bytes` | `AGEN8_CONTEXT_BYTES` | Maximum bytes of user/agent history sent to the model.
| `--include-history-ops` | `AGEN8_INCLUDE_HISTORY_OPS` | Whether operations from `/history` appear in prompts (default: enabled).

For complete flag context and environment-variable guidance, see [docs/cli-usage.md](docs/cli-usage.md) (this document) complemented by [docs/config-toml.md](docs/config-toml.md).

## Troubleshooting CLI sessions

- Artifact directories live under `dataDir/agents/<agentId>` (`workspace`, `artifacts`, `log`). Use `./agen8 show run <agentId>` to inspect the latest status.
- When a session stalls, check `./agen8 logs --follow --agent-id <agentId>` for trace events or use `./agen8 activity --follow` for proposal-level insights.
- If context truncation is evident, lower `--context-bytes` or raise `--history-pairs` when starting a new run.
- Missing artifacts often mean the agent cleaned up after a review; inspect `dataDir/agents/<agentId>/workspace/subagents/<childRunId>` for worker outputs.

## Why the CLI matters

Each CLI command maps directly to CLI handlers in `cmd/agen8/cmd`. Reviewing them helps you understand how the runtime initializes configuration, resolves mounts, and invokes internal services such as `pkg/services/session` and `internal/store`.
