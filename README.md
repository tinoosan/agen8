# Agen8 MCP Server

Agen8 MCP Server is an MCP-first work-context layer for AI harnesses.

Codex, Claude Code, and other harnesses remain the primary work surfaces. Agen8
provides the durable structured record behind them: projects, members, missions,
key results, tasks, decisions, files, credentials, HTTP actions, and the context
map.

This repository is being reshaped from selected Agen8 code. It intentionally
does not continue the old product as a replacement chat UI or as a default
harness session manager.

## Current Baseline

- Binary: `agen8-mcp`
- Default local data directory: `~/.agen8` (`--data-dir` or
  `AGEN8_DATA_DIR` can point isolated runs somewhere else)
- Development daemon: `make dev remote`
- Build command: `make build-go`
- Version check: `./bin/agen8-mcp version`
- Health check: `GET http://127.0.0.1:7777/healthz`
- MCP endpoint: `http://127.0.0.1:7777/mcp?token=<token>`
- Retained MCP tools: `project`, `mission`, `task`, `decision`,
  `graph_query`, and `http`

See `docs/release-baseline.html` for the setup, verification, and release
baseline checklist.

## Local Setup

Start the development daemon with:

```sh
make dev remote
```

On a fresh data directory, the daemon prints a setup URL that includes the
one-time setup token. Open that URL, create the first local account, then create
or open a project in the web UI.

Runtime data lives outside the repository. On macOS the default is
`~/.agen8`; use `DATA_DIR=...`, `AGEN8_DATA_DIR`, or `--data-dir` only when an
isolated verification run is intentional. `make clean` removes build artifacts
and logs, not the live SQLite database. Database resets are explicit release or
operator actions, not part of routine cleanup.

Connect a harness to:

```text
http://127.0.0.1:7777/mcp?token=<api-key>
```

Use `.mcp.example.json` as the checked-in template. Keep the real `.mcp.json`
local; it is ignored because it contains machine-specific MCP server entries
and the user's local Agen8 API key.

Then call `project.register` with the project root and a readable
`display_name`, such as `backend engineer` or `frontend reviewer`, so tasks,
decisions, and graph records remain understandable.

## Initial Repository Shape

```text
cmd/agen8-mcp/               # daemon CLI entrypoint
internal/app/                # retained service graph construction
internal/daemon/             # HTTP daemon, setup, web serving, MCP mount
internal/mcp/                # retained MCP tool definitions and dispatch
internal/services/           # service-owned domains, apps, repos, RPC adapters
internal/store/              # SQLite baseline schema and preserving checks
web/                         # focused local UI
docs/                        # HTML architecture and release docs
skills/agen8/                # installable Agen8 workflow skill template
```
