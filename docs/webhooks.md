# Webhooks

Agen8 supports two webhook paths:

1. Incoming webhooks: queue tasks via HTTP.
2. Outgoing webhooks: notify external systems when tasks complete.

## Incoming webhook (task queue)

Enable the server with:

```sh
agen8 --webhook-addr ":8080"
# or
export AGEN8_WEBHOOK_ADDR=":8080"
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

If you provide a `taskId` that does not start with `task-` (or `heartbeat-`), Agen8 normalizes it to `task-<id>`.
When normalization occurs, the original value is recorded in `metadata.originalTaskId`.

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
agen8 --result-webhook-url "https://example.com/agen8/result"
# or
export AGEN8_RESULT_WEBHOOK_URL="https://example.com/agen8/result"
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

- `internal/webhook` – Task archive abstraction (`TaskArchiveWriter`), task ingester (`TaskIngester`), and HTTP server (`Server`). The daemon and team daemon wire via `webhook.NewServer` and `webhook.NewWebhookTaskIngester`.
- `internal/app/daemon_builder.go` – Standalone daemon webhook wiring
- `internal/app/team_daemon.go` – Team daemon webhook wiring
- `internal/app/notifier.go` (outgoing notifier)
