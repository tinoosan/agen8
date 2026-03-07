# Agen8

Agen8 Core runs a local agent runtime for your project. Teams do the work, tasks are how you ask for work, `monitor` is the current primary operator surface, and focused `view ...` commands expose dashboard, activity, and mail slices when you need them.

## Table of Contents

- [Getting started](#getting-started)
- [Core concepts](#core-concepts)
- [Commands & workflows](#commands--workflows)
- [Configuration](#configuration)
- [Email notifications (Gmail OAuth2)](#email-notifications-gmail-oauth2)
- [Inspecting runtime state](#inspecting-runtime-state)
- [Troubleshooting](#troubleshooting)
- [Key documentation](#key-documentation)
- [Developer resources](#developer-resources)

## Getting started

### Prerequisites

- Go 1.24+ toolchain (used to build the CLI binary in `cmd/agen8`).
- A writable data directory for `$AGEN8_DATA_DIR` (defaults to `~/.agen8`).
- Optional Gmail credentials when you plan to send completion emails or notifications.

### Build and launch

1. Build the CLI: `go build ./cmd/agen8`.
2. Start the runtime: `./agen8 daemon start`
3. Bind the current project: `./agen8 project init`
4. List teams: `./agen8 team list`
5. Start work with a team: `./agen8 team start startup_team`
6. Operate the live system: `./agen8 monitor`

### Rapid reference

- `./agen8 monitor` is the current live operator surface.
- Tail structured logs with `./agen8 logs --follow`.
- Use focused views with `./agen8 view dashboard`, `./agen8 view activity`, and `./agen8 view mail`.
- When you need flag help, run `./agen8 --help` or read [docs/cli-usage.md](docs/cli-usage.md).

## What agen8 is

Agen8 is a **local agent runtime** where teams are configuration, not code. You define who your agents are and what they can do in plain YAML and Markdown, start the daemon, and hand it work — agen8 manages the rest.

- **Declarative identity**: define an agent's role, model, goals, and allowed tools in a YAML profile.
- **Portable capabilities**: define what an agent _can do_ using [`SKILL.md` files](https://agentskills.io/specification) — part of the Agent Skills open standard, compatible with Claude Code, Cursor, and other runtimes.
- **Autonomous lifecycles**: agents can wake up on a schedule via `heartbeat` jobs configured per-role in the profile — useful for background monitoring, memory consolidation, and recurring tasks.
- **Structured teams**: a project can run one or more teams; each team has a coordinator that delegates to co-agents, which can spawn sub-agents for isolated execution work.

The direction is a declarative runtime for autonomous teams — closer to docker-compose today, with Kubernetes-level orchestration (multi-team scheduling, cross-team routing, declarative desired state) as the target.

## Core concepts

### Agent-as-Config

Agen8 treats agents as configuration rather than imperative code. By isolating behavior into profiles and skills, you gain:

- **Portability**: Move an agent profile from your laptop to a server without changing code.
- **Auditability**: Every instruction and capability is version-controllable in plain text.
- **Interoperability**: Skills follow the [Agent Skills open standard](https://agentskills.io/specification) — shareable across agen8, Claude Code, Cursor, and any compatible runtime.

### Agentic File System (AFS)

Agen8 exposes a virtual filesystem inside each run. Key mounts include:

- `/project` – your host workspace (defaults to the current working directory; overridable via `--workdir`).
- `/workspace` – agent-local workspace mapped to `dataDir/agents/<agentId>/workspace`.
- `/log` – run event stream and trace excerpts.
- `/skills` – user-defined skills (`/skills/<skill_name>/SKILL.md`).
- `/plan` – planning workspace (`HEAD.md` + `CHECKLIST.md`).
- `/memory` – shared agent memory (`MEMORY.MD` + daily `YYYY-MM-DD-memory.md` files).

### Teams, tasks, and views

- **Teams** are the configured worker shapes available to a project.
- **Tasks** are the units of work sent to the active team.
- **Views** are the focused operational surfaces for observing the live runtime.
- **Sessions** and **runs** still exist internally, but they are implementation details rather than the primary public model.

## Commands & workflows

Agen8 ships a Cobra CLI that covers the full agent lifecycle. Use the tables below to pick the right command for creating, observing, and inspecting agents.

### Public commands

| Command | Purpose |
| ------- | ------- |
| `agen8 daemon start|status|stop` | Manage the local runtime process. |
| `agen8 project init|status` | Enable Agen8 for the current project and inspect its active state. |
| `agen8 team list|start` | List available profile-backed teams and start work with one. |
| `agen8 task send|list` | Send work to the active team and inspect team tasks. |
| `agen8 monitor` | Open the primary live operator surface. |
| `agen8 view dashboard|activity|mail` | Open focused operational views. |
| `agen8 logs` | Inspect raw runtime events and structured logs. |

### Administrative commands

| Command | Purpose |
| ------- | ------- |
| `agen8 auth ...` | Manage runtime authentication. |
| `agen8 profiles ...` | Validate or inspect profiles administratively. |

## Configuration

Runtime configuration resolves in this order: CLI flags → environment variables → `${AGEN8_DATA_DIR}/config.toml` → built-in defaults. See [docs/config-toml.md](docs/config-toml.md) for onboarding behavior, `code_exec` settings, and allowed path access.

Agen8 supports two auth providers:

- `api_key` (default): `OPENROUTER_API_KEY`/`OPENAI_API_KEY` + keychain fallback
- `chatgpt_account`: browser OAuth login with local token storage (`${AGEN8_DATA_DIR}/auth/chatgpt_oauth.json`)
  - OpenAI models use account auth directly
  - Non-openai models fail fast by default (no silent fallback)
  - Optional explicit fallback via `config.toml`:
    - `[auth] provider = "chatgpt_account"`
    - `[auth] allow_api_key_fallback_for_non_openai = true`

Quick auth commands:

```sh
./agen8 auth login --provider chatgpt_account
./agen8 auth status --provider chatgpt_account
./agen8 auth logout --provider chatgpt_account
```

| Flag | Env | Description |
|------|-----|-------------|
| `--data-dir` | `AGEN8_DATA_DIR` | Base directory for `agen8.db`, sessions, agents, and shared memory. |
| `--workdir` | `AGEN8_WORKDIR` | Host path mounted as `/project`. |
| `--context-bytes` | `AGEN8_CONTEXT_BYTES` | Max bytes of history included in prompts. |
| `--include-history-ops` | `AGEN8_INCLUDE_HISTORY_OPS` | Include host operations from `/history` (default: enabled). |
| `--auth-provider` | `AGEN8_AUTH_PROVIDER` | Auth provider selector: `api_key` or `chatgpt_account`. |

Helpers in `internal/config/effectiveConfig()` resolve the final configuration before each command runs. Additional flag and env var guidance resides in [docs/cli-usage.md](docs/cli-usage.md).

Project desired running state is configured separately in [docs/agen8-yaml.md](docs/agen8-yaml.md). That manifest lives at `.agen8/agen8.yaml` inside each initialized project.

## Email notifications (Gmail OAuth2)

Agen8 can send **plain-text** email notifications through Gmail using OAuth2 (XOAUTH2 over SMTP).

### Setup

Agen8 prefers real environment variables, but falls back to a `.env` file in your session/workdir root if values are missing.

```sh
export GMAIL_USER="you@gmail.com"
export GMAIL_FROM="you@gmail.com"                 # optional; defaults to GMAIL_USER

export GOOGLE_OAUTH_CLIENT_ID="..."
export GOOGLE_OAUTH_CLIENT_SECRET="..."
export GOOGLE_OAUTH_REFRESH_TOKEN="..."

# Optional (debug only): use an access token instead of refreshing.
export GOOGLE_OAUTH_ACCESS_TOKEN="..."
```

Notes:

- You must create an OAuth client in Google Cloud, enable Gmail access, and mint a refresh token for the `https://mail.google.com/` scope.
- Agen8 uses STARTTLS on port 587; implicit TLS on port 465 is not supported.

### Agent usage

The built-in tool name is `email(to, subject, body)`. Ask the agent explicitly or configure autonomous mode to send completion summaries. If SMTP is not configured, email requests fail gracefully and the agent continues normally.

## Inspecting runtime state

The CLI stores persistent state under the configured `dataDir`:

- `dataDir/agen8.db` (sessions, runs, events, history).
- `dataDir/agents/<agentId>/` (workspace, artifacts, log, inbox, outbox).
- `dataDir/memory/` (shared memory: `MEMORY.MD` + daily `YYYY-MM-DD-memory.md`).

Refer to [docs/data-layout.md](docs/data-layout.md) for a guided walkthrough, sample commands, and tips on inspecting artifacts manually.

## Troubleshooting

- Logs live under `dataDir/agents/<agentId>/log` (JSON/trace artifacts). Use `./agen8 logs --follow` to inspect live runtime behavior.
- The [Troubleshooting guide](docs/troubleshooting.md) covers stuck agents, missing artifacts, and configuration problems with quick remediations.

## Key documentation

- **[docs/cli-usage.md](docs/cli-usage.md)** – Step-by-step workflows, flag guidance, and session lifecycle examples for the CLI.
- **[docs/config-toml.md](docs/config-toml.md)** – Onboarding, config hierarchy, `code_exec`, and path access settings.
- **[docs/agen8-yaml.md](docs/agen8-yaml.md)** – Desired-state manifest for project teams, reconciliation behavior, and examples.
- **[docs/chatgpt-account-auth.md](docs/chatgpt-account-auth.md)** – Browser OAuth setup, token refresh behavior, and troubleshooting for `chatgpt_account`.
- **[docs/developer-guide.md](docs/developer-guide.md)** – Internal architecture, session/run lifecycle, and execution hierarchy reference.
- **[docs/troubleshooting.md](docs/troubleshooting.md)** – Quick triage for build issues, stuck connections, and retries.
- **[docs/data-layout.md](docs/data-layout.md)** – Map directories, sub-agent workspaces, and inspect artifacts/logs manually.

## Developer resources

- Inspect `internal/app` for session runtime wiring and `internal/store` for persistence logic.
- The [Developer guide](docs/developer-guide.md) explains how configuration, session lifecycle, and telemetry hooks fit together.
- The [Execution model](docs/execution-model.md) (PRD, Status: Draft) defines the target orchestration behaviour — sub-agents, teams, hierarchy, review gate, retry/escalation, and daemon responsibilities. Implementation is in progress; treat it as the spec we're building toward.
- Inspect `pkg/agent/hosttools` for the built-in host tool surface and `pkg/tools/builtins` for host-side implementations.

Contributions are welcome—submit documentation fixes or enhancements alongside code changes to keep the docs aligned with evolving runtime behavior.
