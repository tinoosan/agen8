# Agen8

Agen8 is an MCP-first work-context layer for AI harnesses.

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
- Build command: `make build` (full binary with embedded UI; `make build-go`
  recompiles only the Go side)
- Version check: `./bin/agen8-mcp version`
- Health check: `GET http://127.0.0.1:7777/healthz`
- MCP endpoint: `http://127.0.0.1:7777/mcp?token=<token>`
- Retained MCP tools: `project`, `mission`, `task`, `decision`,
  `graph_query`, and `http`
- Pre-push gate: clean git status, Go tests, frontend lint/tests/build,
  `npm audit`, and `govulncheck`

See `docs/release-baseline.html` for the setup, verification, and release
baseline checklist.

## Getting Started

Agen8 ships as a single self-contained binary: the web UI is compiled into it,
so once it is built there is nothing else to serve. Run it locally, create an
account in the browser, then point your AI harness at it.

### 1. Prerequisites

- **Go 1.25+** — to compile the daemon.
- **Node.js and npm** — to build the web UI assets that get embedded in the
  binary.
- An MCP-capable harness to connect (e.g. Codex or Claude Code). Optional, but
  it is what Agen8 exists to support.

### 2. Build

```sh
make build
```

This builds the web UI and compiles the daemon with that UI embedded, producing
the binary at `./bin/agen8-mcp`. (Check the version any time with
`./bin/agen8-mcp version`.)

### 3. Run

```sh
./bin/agen8-mcp
```

That starts the daemon (equivalent to `./bin/agen8-mcp daemon start`). By
default it listens on `127.0.0.1:7777` and stores its data in `~/.agen8`.
Override either if you need to:

```sh
./bin/agen8-mcp daemon start --http-addr 127.0.0.1:8080 --data-dir /path/to/data
```

Confirm it is up:

```sh
curl http://127.0.0.1:7777/healthz
```

### 4. Create your account

On the **first run with a fresh data directory**, the daemon prints a setup URL
to the terminal that includes a one-time setup token. Open that URL in a
browser, create the first local account, then create or open a project.

First-run setup also returns an initial **API key** — this is the token your
harness will use to talk to Agen8. Copy it somewhere safe.

### 5. Connect a harness

Add an MCP server entry to your harness pointing at the daemon, using the API
key from the previous step as the token:

```text
http://127.0.0.1:7777/mcp?token=<your-agen8-api-key>
```

`.mcp.example.json` at the repo root is a ready-to-copy template. Copy it to
`.mcp.json` and drop in your key. Keep the real `.mcp.json` local — it is
gitignored because it holds your machine-specific server entry and API key.

### 6. Install the Agen8 workflow skill

Install the workflow skill into your harness so it knows how to drive Agen8
(register, plan missions/key results, run tasks, log decisions):

```sh
./bin/agen8-mcp skill install --harness codex
# or, for Claude Code:
./bin/agen8-mcp skill install --harness claude-cli
```

Re-run the same command any time to refresh the installed skill.

### 7. Start working

From inside your harness, call `project.register` with the project root and a
readable `display_name` such as `Atlas (Backend Engineer)` or
`Iris (Frontend Reviewer)`, so tasks, decisions, and graph records stay
understandable. From there you create missions and key results, then run tasks
against them.

Your runtime data lives outside the repository (default `~/.agen8`). It is never
removed by routine cleanup — resetting the database is an explicit, deliberate
action.

## Development

The steps above run the *built* binary. For working **on** Agen8 itself, use the
hot-reloading development daemon instead:

```sh
make dev remote
```

This runs the Go daemon behind [Air](https://github.com/air-verse/air) and a
Vite dev server for the web UI, so Go and frontend changes reload as you edit.
The browser always opens the daemon URL; Vite only serves hot-reload assets. The
first-run account flow is identical to the built binary.

Override runtime state only when an isolated run is intentional, via `DATA_DIR=`,
`AGEN8_DATA_DIR`, or `--data-dir`. `make clean` removes build artifacts and logs
but never the live SQLite database.

## Pre-Push Check

Before publishing or tagging the baseline, run the same gate used for the
current local release checks:

```sh
git status --short
go test ./... -count=1
npm --prefix web run lint
npm --prefix web run test -- --run
npm --prefix web run build
npm --prefix web audit --audit-level=moderate
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
git status --short
```

The final status should be clean. If the web build regenerates
`internal/web/dist`, commit that generated output with the matching source
change. Publish `.mcp.example.json`, not a real local `.mcp.json`.

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
