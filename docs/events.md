## Event Type Naming

Use a consistent naming convention for event types to make filtering and debugging easier.

### Format

- `<domain>.<noun>.<verb>` for actions
- `<domain>.<noun>` for stable state markers

### Tense

- Past tense for completed actions: `queued`, `started`, `completed`
- Present tense for ongoing states: `running`, `idle`

### Examples

- `agent.op.request`
- `agent.op.response`
- `task.queued`
- `task.started`
- `daemon.start`

## Coordinator Rendering Contract

- Preserve semantic op identity in `agent.op.request`/`agent.op.response`.
- For bridged `code_exec` calls, use `action=code_exec_bridge` as the provenance marker.
- Do not downgrade known semantic tagged tool results (`task_create`, `task_review`, `obsidian`, `soul_update`) to generic `tool_result` in coordinator rendering.
- Coordinator feed ingestion must be replay-idempotent: replaying the same event slice must not increase rendered entry count.
