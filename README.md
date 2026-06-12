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

- Binary: `agen8`
- Default local data directory: `~/.agen8` (`--data-dir` or
  `AGEN8_DATA_DIR` can point isolated runs somewhere else)
- Development daemon: `make dev remote`
- Build command: `make build` (full binary with embedded UI; `make build-go`
  recompiles only the Go side)
- Version check: `./bin/agen8 version`
- Health check: `GET http://127.0.0.1:7777/healthz`
- MCP endpoint: `http://127.0.0.1:7777/mcp` with
  `Authorization: Bearer <token>`
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
the binary at `./bin/agen8`. (Check the version any time with
`./bin/agen8 version`.)

### 3. Run

```sh
./bin/agen8
```

That starts the daemon (equivalent to `./bin/agen8 daemon start`). By
default it listens on `127.0.0.1:7777` and stores its data in `~/.agen8`.
Override either if you need to:

```sh
./bin/agen8 daemon start --http-addr 127.0.0.1:8080 --data-dir /path/to/data
```

Confirm it is up:

```sh
curl http://127.0.0.1:7777/healthz
```

### Docker

You can also build and run Agen8 as a container. The image builds the web UI
inside Docker, compiles the `agen8` binary with the UI embedded, runs as a
non-root user, stores state in `/data`, and exposes the daemon on port `7777`.

Build the image:

```sh
docker build -t agen8:local .
```

Run it with a named volume so SQLite state survives container replacement:

```sh
docker run --rm \
  --name agen8 \
  -p 7777:7777 \
  -v agen8-data:/data \
  agen8:local
```

Or use Compose:

```sh
docker compose up --build
```

On first run with an empty volume, the container logs include the setup URL:

```text
agen8 setup: http://[::]:7777/setup?token=...
```

Open the equivalent host URL, `http://127.0.0.1:7777/setup?token=...`, create
the first account, then use the returned API key for MCP clients. Confirm the
container is healthy with:

```sh
curl http://127.0.0.1:7777/healthz
docker inspect --format '{{json .State.Health}}' agen8
```

The image default command is equivalent to:

```sh
agen8 daemon start --listener http --http-addr 0.0.0.0:7777 --data-dir /data
```

To run with a bind-mounted data directory instead of a named volume:

```sh
mkdir -p ./.agen8-docker
docker run --rm -p 7777:7777 -v "$PWD/.agen8-docker:/data" agen8:local
```

### 4. Create your account

On the **first run with a fresh data directory**, the daemon prints a setup URL
to the terminal that includes a one-time setup token. Open that URL in a
browser, create the first local account, then create or open a project.

First-run setup also returns an initial **API key** — this is the token your
harness will use to talk to Agen8. Copy it somewhere safe.

### 5. Connect a harness

Add an MCP server entry to your harness pointing at the daemon, using the API
key from the previous step as a bearer token. `.mcp.example.json` at the repo
root is a ready-to-copy template. Copy it to `.mcp.json`, then set
`AGEN8_MCP_TOKEN` to your key before starting the MCP client. Keep the real
`.mcp.json` local — it is gitignored because it is machine-specific.

For Codex or managed environments where you want the server URL to stay stable
while the secret lives separately, use an environment-backed bearer token:

```toml
[mcp_servers.agen8]
url = "http://127.0.0.1:7777/mcp"
bearer_token_env_var = "AGEN8_MCP_TOKEN"
```

Then set `AGEN8_MCP_TOKEN` to your `ak_...` API key before starting Codex. The
setup snippets use the same stable URL pattern. Query-token auth
remains supported for compatibility and project link-token flows, but it is not
the public default.

### 6. Install the Agen8 workflow skill

Install the workflow skill into your harness so it knows how to drive Agen8
(register, plan missions/key results, run tasks, log decisions):

```sh
./bin/agen8 skill install --harness codex
# or, for Claude Code:
./bin/agen8 skill install --harness claude-cli
```

Re-run the same command any time to refresh the installed skill.

Claude Code's Agen8 hook is not a separate binary. It is the same installed
`agen8` binary invoked as `agen8 claude hook`, so local development should only
produce `./bin/agen8` for releases and `./tmp/agen8-dev` while Air is running.

### Optional: Install the Codex plugin

The repo also includes a Codex plugin at `plugins/agen8`. It packages the
Agen8 skill and a stable MCP server entry for `http://127.0.0.1:7777/mcp`.

Add the Agen8 GitHub repo as a plugin marketplace, then install `agen8` from
the Codex app:

```sh
codex plugin marketplace add tinoosan/agen8
```

For local plugin development from this checkout instead, register the repo
folder:

```sh
codex plugin marketplace add <path-to-this-checkout>
```

Set the API key before starting Codex:

```sh
launchctl setenv AGEN8_MCP_TOKEN '<your-agen8-api-key>'
```

The plugin still needs enterprise Codex accounts with an MCP allowlist to allow
the Agen8 URL:

```toml
[mcp_servers.agen8]
identity = { url = "http://127.0.0.1:7777/mcp" }
```

### 7. Start working

From inside your harness, call `project.register` with the canonical project id
or canonical project root and a readable `display_name` such as
`Atlas (Backend Engineer)` or `Iris (Frontend Reviewer)`, so tasks, decisions,
and graph records stay understandable. From there you create missions and key
results, then run tasks against them.

If a worker uses a git worktree, treat that worktree as an execution checkout
only. It is not a new Agen8 project boundary. Create or enter the worktree for
file edits, but register and log against the canonical Agen8 project:

```json
{
  "action": "register",
  "project_id": "<canonical-project-id>",
  "project_root": "<canonical-main-checkout-root>",
  "display_name": "Atlas (Backend Engineer)"
}
```

Do not register with only a temporary worktree path, because that can mint a
throwaway project and hide tasks, submissions, and decisions from the canonical
work graph.

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

The final status should be clean. The generated web bundle in
`internal/web/dist` is **not** committed — only the `.gitkeep` placeholder is
tracked, so `npm run build` regenerating the bundle leaves git status clean.
The binary embeds the freshly built bundle at `make build` time. Publish
`.mcp.example.json`, not a real local `.mcp.json`.

## Initial Repository Shape

```text
cmd/agen8/                   # daemon CLI entrypoint
internal/app/                # retained service graph construction
internal/daemon/             # HTTP daemon, setup, web serving, MCP mount
internal/mcp/                # retained MCP tool definitions and dispatch
internal/services/           # service-owned domains, apps, repos, RPC adapters
internal/store/              # SQLite baseline schema and preserving checks
web/                         # focused local UI
docs/                        # HTML architecture and release docs
skills/agen8/                # installable Agen8 workflow skill template
```
