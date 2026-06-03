# Agen8 MCP Server

Agen8 MCP Server is an MCP-first work-context layer for AI harnesses.

Codex, Claude Code, and other harnesses remain the primary work surfaces. Agen8
provides the durable structured record behind them: spaces, members, missions,
tasks, decisions, requests, files, activity, and the context map.

This repository is being reshaped from selected Agen8 code. It intentionally
does not continue the old product as a replacement chat UI or as a default
harness session manager.

## Direction

- MCP is the primary integration surface.
- Harness sessions register or join through MCP instead of being owned by
  Agen8 by default.
- SQLite is the first durable store.
- The existing Agen8 MCP tool naming style is preserved where it still fits.
- The inspection UI is focused on spaces, members, context map, decisions,
  tasks, requests, files, and activity.
- Architecture docs live as HTML under `docs/`.

## Initial Repository Shape

```text
cmd/agen8-mcp-server/        # stdio and streamable HTTP MCP server
internal/domain/             # durable work-context domain
internal/mcp/                # tool definitions and transport adapters
internal/storage/            # SQLite-first persistence
web/                         # focused inspection UI, adapted later
docs/                        # target architecture HTML docs
```

See `HANDOFF.md` for the full project handoff.
