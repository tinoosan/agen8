# Message Bus Cutover (Task-Optional Message Envelope)

## Summary

The runtime now supports a message-bus-first execution path where agents claim work
from `messages` and `Task` is optional message payload.

External RPC method names and response shapes are unchanged.

## What Changed

- Added `types.AgentMessage` with causal identity fields:
  - `intentId`
  - `correlationId`
  - `causationId`
  - `producer`
- Added `MessageStore` contracts in `pkg/agent/state`.
- Added SQLite `messages` persistence with:
  - idempotency key `UNIQUE(thread_id, intent_id)`
  - queue indexes for claim performance.
- Added SQLite operations:
  - publish/get/list/count
  - claim/ack/nack
  - expired-claim requeue
- Added `pkg/services/message` manager with wake subscriptions.
- `pkg/services/task.Manager` now publishes a corresponding message when creating
  a task (projection bridge).
- `pkg/agent/session.Session` inbox drain is now message-authoritative:
  - claim messages
  - resolve task payload
  - execute existing task pipeline
  - ack/nack message
- no legacy task polling fallback in runtime worker sessions.
- `turn.create` now marks created task metadata so publish path emits
  `kind=user_input` messages.
- `task.claim` / `task.complete` require a backing inbox message envelope.
  Projection-only task rows without envelopes return explicit invalid-state errors.

## Causal Identity Rules

- `intentId` deduplicates producer retries per thread.
- `correlationId` ties a logical flow together end-to-end.
- `causationId` points to the message that triggered this message.
- Producer retries with same `(threadId, intentId)` return existing message.

## Delivery Semantics

- At-least-once processing.
- Claim is lease-based; duplicate processing remains possible.
- Handlers must stay idempotent for side effects.

## Runtime Notes

- Wake-driven loops continue to be used.
- Immediate nack requeue is currently used to avoid wake-stall in retry paths.
- Legacy pending task rows are not backfilled into `messages` in this phase.
- `tasks` remains a read/projection surface; execution authority is `messages`.

## Verification Commands

```bash
go test ./...
go vet ./...
go test ./pkg/agent/state -run Message
go test -bench MessageStore ./pkg/agent/state
```
