# Webhooks

Workbench supports two webhook paths:

1. Incoming webhooks: queue tasks via HTTP.
2. Outgoing webhooks: notify external systems when tasks complete.

## Incoming webhook (task queue)

Enable the server with:

```sh
workbench --webhook-addr ":8080"
# or
export WORKBENCH_WEBHOOK_ADDR=":8080"
```

### Endpoints

- `POST /task` queues a task for the autonomous agent.
- `GET /healthz` returns `{"status":"ok"}`.

### Payload

```json
{
  "taskId": "string (optional, auto-generated if omitted)",
  "goal": "string (required)",
  "priority": 0,
  "inputs": { "key": "value" },
  "metadata": { "key": "value" }
}
```

### Response

```json
{
  "taskId": "task-<uuid>",
  "status": "queued"
}
```

### Example

```sh
curl -X POST "http://localhost:8080/task" \
  -H "Content-Type: application/json" \
  -d '{"goal":"Summarize the last run in /outbox"}'
```

## Outgoing webhook (result notifier)

Enable result notifications with:

```sh
workbench --result-webhook-url "https://example.com/workbench/result"
# or
export WORKBENCH_RESULT_WEBHOOK_URL="https://example.com/workbench/result"
```

### Payload

The notifier sends a `POST` with this JSON:

```json
{
  "task": {
    "taskId": "string",
    "goal": "string",
    "status": "succeeded|failed",
    "inputs": {},
    "metadata": {},
    "createdAt": "timestamp",
    "startedAt": "timestamp",
    "completedAt": "timestamp",
    "error": "string (if failed)"
  },
  "result": {
    "taskId": "string",
    "status": "succeeded|failed",
    "summary": "string",
    "artifacts": ["string array"],
    "error": "string (if failed)",
    "completedAt": "timestamp"
  }
}
```

The notifier treats any 2xx response as success.

## Events emitted

- `webhook.task.queued` when a task is accepted
- `webhook.error` when the incoming server fails
- `task.notify.error` when the result webhook fails

## Related files

- `internal/app/daemon.go` (incoming server)
- `internal/app/notifier.go` (outgoing notifier)
