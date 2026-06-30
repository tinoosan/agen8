# Agen8

Agen8 is an MCP-first work-context layer for AI harnesses.

Codex and Claude Code are the currently supported work surfaces, with support
for other harnesses coming soon. Agen8 provides the durable structured record
behind them: projects, members, missions, key results, tasks, decisions, files,
credentials, HTTP actions, and the context map.

## What Agen8 Provides

- A local MCP server for Codex and Claude Code.
- A browser UI for projects, missions, key results, tasks, decisions, and
  related context.
- Durable local storage for agent work records, separate from your chat
  harness history.
- Optional attention hooks for supported harnesses.
- Docker and self-hosting support for teams that want a shared daemon.

## Getting Started

Agen8 ships as a single self-contained binary: the web UI is compiled into it,
so once it is built there is nothing else to serve. Run it locally, create an
account in the browser, then point your AI harness at it.

Fast path for a local install:

```sh
make build
./bin/agen8 daemon start
```

Open the setup URL printed by the daemon, create the first account, save the
returned `ak_...` API key, then configure your harness to call
`http://127.0.0.1:7777/mcp` with that API key as a bearer token.

### 1. Prerequisites

- **Go 1.25+** — to compile the daemon.
- **Node.js and npm** — to build the web UI assets that get embedded in the
  binary.
- An MCP-capable harness to connect (e.g. Codex or Claude Code). Optional, but
  it is what Agen8 exists to support.
- Optional: **Docker** and Docker Compose for containerized local or hosted
  runs.

### 2. Build

```sh
make build
```

This builds the web UI and compiles the daemon with that UI embedded, producing
the binary at `./bin/agen8`. (Check the version any time with
`./bin/agen8 version`.)

### 3. Run

```sh
./bin/agen8 daemon start
```

That starts the daemon. Bare `./bin/agen8` prints command help instead of
running a daemon, so startup is always explicit. By default it listens on
`127.0.0.1:7777` and stores its data in `~/.agen8`. Override either if you
need to:

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
  -e AGEN8_PUBLIC_URL=http://127.0.0.1:7777 \
  -e AGEN8_DISABLE_LOCAL_HOOK_PROVISIONING=true \
  agen8:local
```

Or use Compose:

```sh
docker compose up --build
```

Published release images are available from GitHub Container Registry:

```sh
docker pull ghcr.io/tinoosan/agen8:v0.0.1
```

Set `AGEN8_PUBLIC_URL` to the URL users will put in their harness MCP config.
For hosted Docker or Kubernetes deployments, keep
`AGEN8_DISABLE_LOCAL_HOOK_PROVISIONING=true`; the daemon can receive remote hook
events, but only the local machine running Codex or Claude Code can install
those hooks.

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

For Kubernetes and remote self-hosting, including the sample manifest that uses
the versioned release image, see [docs/self-hosting.html](docs/self-hosting.html).

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

Minimal local setup:

```sh
cp .mcp.example.json .mcp.json
export AGEN8_MCP_TOKEN='<your-agen8-api-key>'
```

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

For a hosted daemon, replace `http://127.0.0.1:7777` with your public HTTPS
origin, for example `https://agen8.example.com`.

### 6. Install the Agen8 workflow skill

Install the workflow skill into your harness so it knows how to use Agen8:

```sh
./bin/agen8 skill install --harness codex
# or, for Claude Code:
./bin/agen8 skill install --harness claude-cli
```

Re-run the same command any time to refresh the installed skill.

Attention hooks are installed separately from the skill:

```sh
agen8 hooks install --harness codex --url https://agen8.example.com --token ak_...
agen8 hooks install --harness claude --url https://agen8.example.com --token ak_... --project-dir /path/to/local/project
```

Use the hosted daemon URL for `--url`. These commands write local harness
configuration files and must run on the machine where the harness runs, not
inside a Kubernetes pod.

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

From inside your harness, register a project and start recording work. Agen8 is
designed to keep plans, tasks, decisions, files, and related context available
across harness sessions.

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

Common development commands:

```sh
make build-go          # Go-only rebuild
make test-go           # Go test suite
make test-web          # web vitest suite
make lint              # go vet + web lint
make fmt-check         # gofmt guard
make docs              # serve static docs on the LAN
```

## Contributing

Before opening a pull request, run the local checks:

```sh
go test ./... -count=1
npm --prefix web run lint
npm --prefix web run test -- --run
npm --prefix web run build
npm --prefix web audit --audit-level=moderate
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
```

Do not commit local runtime data, API keys, setup tokens, generated binaries, or
machine-specific MCP configuration. Use `.mcp.example.json` as the shareable
template for local MCP setup.

## License

Agen8 is released under the MIT License. See [LICENSE](LICENSE).
