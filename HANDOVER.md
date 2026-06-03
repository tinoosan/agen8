# Agen8 MCP Server Handover

Last updated: 2026-06-03

## Current Direction

Agen8 MCP Server is an MCP-first work-context layer for AI harnesses. Codex, Claude Code, and other harnesses remain the main work surfaces. Agen8 provides durable structured records behind those harnesses: projects, spaces, members, tasks, decisions, messages, requests, files, activity, and graph context.

Do not rebuild Agen8 as a replacement chat UI. The UI should stay focused on setup, login, inspection, overview, board, decisions, members, locations, and operational control.

## Current Project State

- Project id: `agen8-mcp-server-3333e627`
- Main space id: `space-32322b1e9f1f`
- Current Codex member observed during dogfood: `member-8b4bbf2059a5`
- Current Codex native thread ref observed: `019e8d57-2197-7230-9d6d-b85e79418ee9`
- MCP token used locally: `agen8-local`

The daemon is not intentionally left running from the latest verification. Start it with the normal dev command before testing MCP again.

## What Changed Recently

- The MCP token model was hard-cut over toward one user-scoped setup token, with harness/thread identity coming from MCP request metadata where available.
- `space.register` now binds a harness session/member from the native harness metadata instead of requiring the user to manually provide model, harness, or thread fields.
- Local Codex runtime host discovery now prefers the managed Codex remote-control Unix socket:
  - `unix:///Users/santinoonyeme/.codex/app-server-control/app-server-control.sock`
- The Codex app-server client can dial WebSocket over Unix sockets.
- System-message delivery now tries to steer into an active Codex turn only when a registered active turn exists.
- A preflight checks `thread/loaded/list` before `turn/steer` on the remote-control socket.
- If the reachable Codex app-server does not own the loaded thread, Agen8 keeps the message queued instead of spawning or consuming through a background run.

## Important Finding

After updating Codex CLI to `0.136.0`, `codex remote-control start --json` works and exposes the managed control socket.

The socket is reachable and accepts JSON-RPC. It can read this thread from disk with `thread/read`, but `thread/loaded/list` is empty for the visible Codex Desktop turn. `turn/steer` fails because the current desktop thread is not loaded by that reachable remote-control server.

Practical outcome:

- Agen8 can identify this Codex thread from MCP metadata.
- Agen8 can reach the managed app-server socket.
- Agen8 cannot yet push unsolicited system messages into this visible desktop turn through that socket.
- The correct current behavior is queue-or-steer: steer only when the reachable app-server owns the loaded turn; otherwise leave the message queued.

Decision logged in Agen8:

- `dec-aa841977-f293-4cc6-aacf-79c2bc5adf0c`
- Title: `Codex remote-control socket is reachable but does not own current desktop turn`

## Live Smoke Tests

Created self-assigned tasks through the real Agen8 MCP `task` tool to trigger task-assigned system messages.

Latest final smoke task:

- `task-729774de48c50107`
- Message id: `msg-e60a9439-2a96-446a-ad10-be5a05cb5b85`
- Expected: queued if the current thread is not loaded by the reachable remote-control app-server.

Daemon log confirmed:

- MCP registration captured the native thread and turn metadata.
- Delivery attempted the Unix remote-control socket.
- Preflight reported the thread was not loaded by the reachable remote-control server.
- Message stayed queued and was not consumed by a background run.

## Files Touched In The Messaging/Runtime Slice

Main files:

- `internal/daemon/daemon.go`
- `internal/daemon/harness_events.go`
- `internal/daemon/http.go`
- `internal/daemon/runtime_host.go`
- `internal/services/harness/infra/codex/appserver.go`
- `internal/daemon/daemon_test.go`

Supporting files from earlier work:

- `internal/logging/config.go`
- `internal/logging/logger.go`
- `Makefile`

There are many unrelated dirty files from earlier cleanup/UI/build work. Do not revert them blindly.

## Verification Run

Focused tests passing:

```sh
go test ./internal/daemon ./internal/mcp ./internal/services/message/app ./internal/services/harness/infra/codex -count=1
```

Useful daemon/log checks:

```sh
lsof -nP -iTCP:7777 -sTCP:LISTEN || true
tail -n 120 tmp/daemon.log
ps -axo pid=,start=,command= | rg 'agen8-mcp-server daemon start|codex app-server --listen|codex app-server --remote-control|standalone/current/codex'
```

Useful Codex remote-control checks:

```sh
codex --version
codex remote-control start --json
codex app-server generate-json-schema --out tmp/codex-appserver-schema-current --experimental
```

## Next Engineering Work

1. Preserve queue-or-steer semantics.
   - Never consume an Agen8 system message unless delivery actually reaches the intended harness session.
   - Never spawn a background Codex run as a substitute for the current visible turn.

2. Find the real live Codex Desktop owner route.
   - The managed remote-control daemon can see persisted thread metadata but not the loaded turn.
   - Investigate whether Codex Desktop exposes a live app-server/control socket for the current visible thread, or whether the app needs to connect/enroll differently.

3. Add a clean queued inbox/tool surface if native push remains unavailable.
   - This should be explicit MCP behavior, not a workaround that hides failed push delivery.
   - The Agen8 skill should inspect queued messages after compaction/resume.

4. Continue simplifying the MCP-first surface.
   - Keep overview and board as primary UI surfaces.
   - Inspector should show MCP tool calls and member activity when rebuilt.
   - Keep login/setup and user-scoped MCP token generation.

5. Clean stale task/message smoke data when appropriate.
   - Several smoke tasks remain pending/queued from delivery testing.
   - Do not mark them complete unless the test they represent is actually satisfied.

## Product Rules To Keep

- Agen8 owns durable work context, not the harness UI.
- Harnesses register themselves through MCP.
- A member represents a harness session/thread identity inside a space.
- First registering member in a space can be coordinator; later session members can be workers depending on product rules.
- One user-scoped MCP token is acceptable for local/self-hosted setup, with per-session/member identity resolved from MCP headers/body metadata.
- Claude Code and Codex may expose different metadata and callback capabilities; design the tool surface around capability differences instead of forcing one behavior.
