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
