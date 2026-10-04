# Agen8 Dev staging setup

The separate private Site is `agen8-dev.tinoosan.chatgpt.site`. The last inspected published version is v9; this branch has not been deployed. `.openai/hosting.json` names only that dev Site. Preserve its owner-only audience and production's separate Site, database, plugin and credentials. Do not import legacy records.

The canonical repository and nested staging source contain concurrent user edits. This feature was implemented in an isolated `feat/mcp-events` checkout, based on `codex/agen8-site-plugin-extensions`. The selected uncommitted staging workspace/sidebar/CSS were copied read-only and refined here; no canonical file was edited. Root Go coordination and legacy web code remain untouched.

Staging publication, schema migrations, plugin installation/connection, persistent credentials, owner grants and subscriptions require the owner's separate action-time approval. This implementation authorization covers local synthetic testing and draft PR updates only. See [the concrete runtime proposal](live-activation.md).

After approved publication and plugin rescan, test through the actual exposed Agen8 Dev plugin:

1. List projects with `project`; create a project prefixed `[STAGING TEST]` with a timestamp.
2. Read that exact project. Create clearly synthetic records with `work`, log reasoning with `decision`, and connect them through `graph_query`.
3. Read the work/decision and `get_context`; confirm IDs, versions, content and connections match the writes.
4. Block, complete, stop and reopen work with `expected_version`; verify required reasons, retained results and expandable history. Resolve stale writes by rereading.
5. Open the same project in the browser and verify shelves, graph, selection, connections and mobile detail views. Archive the synthetic project afterward.
6. Only after separately approved Events activation, run the signed-delivery, retry and disconnection checks in [the runbook](live-activation.md).

The five tools are `project`, `work`, `decision`, `graph_query` and `get_context`. No registration, claims, assignment, review gate or legacy import is part of this workflow. Browser/MCP HTTP tests complement this check; they do not establish ChatGPT plugin usability.
