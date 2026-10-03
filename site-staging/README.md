# Agen8 Dev

A shared work graph across chats. Agents maintain meaningful work, decisions, and optional goals through MCP. Humans see the current explanation, relationships, blockers, results, evidence, and expandable history in a read-only graph.

| Tool | Actions |
| --- | --- |
| `project` | List, create, get, update projects |
| `work` | Create, get, list, update, history for work and optional goals |
| `decision` | Log, get, list, update, history for decisions and reasoning |
| `graph_query` | Query, link, unlink |
| `get_context` | Concise current work, blockers, decisions, recent changes |

Read context when starting or resuming. Reuse shared records across chats; connect decisions and dependencies. Record meaningful changes and an accurate final state. Updates require `expected_version`: reread and reconcile conflicts. Work can be planned, working, blocked, done, or stopped. Blocked work requires a blocker; stopped work requires a reason. Completion is recorded directly with results and checks. Reopening retains earlier results in history.

Agen8 does not register or track agents, assign work, enforce claims, run execution, or require review. Recording is deliberate tool use. Account access remains scoped to the connected user. Node revisions and relationship history commit atomically. Browser routes only read data.

## Development

Requires Node.js 22.13 or newer. Run `npm run install:ci` and `npm run dev`. Portable development provides mock sign-in at `/signin-with-chatgpt?return_to=/`; hosted access uses Sites authentication.

Generate schema changes with `npm run db:generate`. Apply `drizzle/*.sql` in order to the local D1 binding `DB` (`site-creator-d1`). Wrangler and Vite share `.wrangler/state`. Run `npm run typecheck`, `npm run lint`, `npm test`, and `npm run build`.

Migration preserves IDs, text, evidence, relationships, and original timestamps. Earlier workflow states are preserved in node history before mapping to the shared graph states. Existing key results and notes remain readable. Old agent storage is retained but unused. Import is unavailable.

## Staging hosting

`.openai/hosting.json` identifies Agen8 Dev and declares D1 plus native MCP hosting. Use the Sites source workflow to check, build, push, package, and publish this checkout. Production and the legacy application remain separate. The open graph refreshes every ten seconds; content refreshes preserve layout by topology. No automatic conversation capture or runtime monitoring is implemented.

## MCP Events implementation

Work-status transitions and decision creation commit a minimal event outbox alongside history. [MCP Events](docs/mcp-events.md) describes the subscription and signed-delivery implementation, tests, and hosting requirements. Live event discovery and subscription acceptance remain disabled until the host supplies a verified durable dispatcher, secure callback transport, encryption key and account-access check. The existing MCP tools and graph interface continue to work.
