# Agen8 CLI Usage

The `agen8` binary manages a local runtime for your project. The public CLI is organized around `project`, `team`, `task`, and `view`, with `monitor` as the primary live operator surface and `logs` as the direct operational feed.

## Quick start

1. **Build the CLI** (Go 1.24+ toolchain required):

   ```sh
   go build ./cmd/agen8
   ```

2. **Start the runtime**:

   ```sh
   ./agen8 daemon start
   ```
3. **Bind the current project**:

   ```sh
   ./agen8 project init
   ```

   This creates the project-local `.agen8/` workspace, including `.agen8/agen8.yaml` for desired team state.

4. **Start work with a team**:

   ```sh
   ./agen8 team list
   ./agen8 team start startup_team
   ```

5. **Operate the live system**:

   ```sh
   ./agen8 monitor
   ./agen8 logs --follow
   ```

## Recommended workflow

A typical onboarding workflow:

1. `./agen8 daemon start` to bring up the local runtime.
2. `./agen8 project init` to enable Agen8 in the current project.
3. `./agen8 team list` to inspect the available profile-backed teams for this project/runtime.
4. `./agen8 team start <profile-ref>` to start work with that team definition.
5. `./agen8 monitor` to operate the live system.
6. Use `./agen8 logs --follow` for raw runtime visibility or `./agen8 view ...` for focused surfaces.

## Public commands

| Command | Purpose |
| ------- | ------- |
| `agen8 daemon start|status|stop` | Manage the local runtime process.
| `agen8 project init|status|apply` | Enable the current project, inspect its runtime state, or apply desired-state reconciliation immediately.
| `agen8 team list|start` | List available profile-backed teams and start work with one.
| `agen8 task send|list` | Send work to the active team and inspect active-team tasks.
| `agen8 monitor` | Open the current primary operator surface.
| `agen8 view dashboard|activity|mail` | Open focused views.
| `agen8 logs` | Query structured runtime logs and follow live activity.

## Configuration helpers

Runtime configuration resolves in this order: CLI flags → environment variables → `${AGEN8_DATA_DIR}/config.toml` → built-in defaults. Important flags you will see often:

| Flag | Env | Description |
|------|-----|-------------|
| `--data-dir` | `AGEN8_DATA_DIR` | Directory for `agen8.db`, `sessions`, `agents`, and shared `memory`. Defaults to `~/.agen8` or `$XDG_STATE_HOME/agen8`.
| `--workdir` | `AGEN8_WORKDIR` | Host directory mounted as `/project`.
| `--context-bytes` | `AGEN8_CONTEXT_BYTES` | Maximum bytes of user/agent history sent to the model.
| `--include-history-ops` | `AGEN8_INCLUDE_HISTORY_OPS` | Whether operations from `/history` appear in prompts (default: enabled).

For complete flag context and environment-variable guidance, see [docs/cli-usage.md](docs/cli-usage.md) (this document) complemented by [docs/config-toml.md](docs/config-toml.md).

For project desired-state configuration, see [docs/agen8-yaml.md](docs/agen8-yaml.md).

## Troubleshooting CLI sessions

- Artifact directories live under `dataDir/agents/<agentId>` (`workspace`, `artifacts`, `log`).
- When a team stalls, check `./agen8 logs --follow` for trace events or `./agen8 view activity` for the focused activity stream.
- If context truncation is evident, lower `--context-bytes` or raise `--history-pairs` before starting the runtime.
- Missing artifacts often mean the agent cleaned up after a review; inspect `dataDir/agents/<agentId>/workspace/subagents/<childRunId>` for worker outputs.

## Why the CLI matters

The public CLI intentionally hides most runtime internals. Sessions, runs, and coordinator-specific wiring still exist under the hood, but the user-facing model is project -> team -> task -> view.
