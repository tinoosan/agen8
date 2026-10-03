# Agen8 Dev MCP Events

This implementation prepares `work.status_changed` and `decision.created` for Agen8 Dev's shared graph. Live subscriptions are blocked on hosting support. Nothing in this change deploys a Site, provisions callback credentials, subscribes a Work chat or changes production.

## Committed events

`Work` writes a minimal outbox record in the same D1 batch as the node and its history. The history ID is the stable event ID. Optimistic updates that lose their version check emit no event; outbox failure rolls back the node and history. Decision creation emits once; revising a decision and changing a relationship do not emit either event. Relationships remain deferred because each link change writes history for both endpoints.

`work.status_changed` covers transitions to blocked, done or stopped and reopening done/stopped work into planned, working or blocked. The optional `status` filter selects the transition, including `reopened`. A done-to-blocked change has transition `reopened` and resulting status `blocked`. Initial work creation emits no status event.

Both events require an owned `project_id` and optionally filter by `node_id`. Only work accepts `status`. Each delivery has `eventId`, `name`, occurrence `timestamp`, `cursor: null` and `data`. Data contains project/node IDs, version, current status at occurrence, a summary of at most 360 characters and the existing plugin graph deep link. Work events also carry previous status and transition. Bodies, blockers, outcome details and artifacts stay in authoritative records.

Consumers should deduplicate by event ID and reread with `work`, `decision` or `graph_query` before acting. Versions identify stale/out-of-order notifications. Existing `expected_version` checks reject stale writes. At-least-once delivery cannot guarantee exactly-once execution of a consumer's actions. This implementation does not add replay or promise idempotent creation of arbitrary follow-up records.

## Subscription lifecycle

The methods use the same Sites-authenticated MCP endpoint and require MCP 2.0, version `2026-07-28`. Event name header mirrors use the existing protocol validation. The authenticated catalog is private and contains no project data.

Subscription identity hashes the principal, callback URL, event name and canonical filter JSON. Repeating the same identity refreshes one durable subscription. Filters are checked against account-owned records before callback verification. Signing keys must use `whsec_` and canonical base64 decoding to 24–64 bytes.

A verification POST contains only a fresh challenge, with a unique webhook ID and Standard Webhooks HMAC-SHA256 signature. Activation requires a successful response and constant-time challenge comparison within ten seconds. Response reads are bounded to 4 KiB. Verification caches for five minutes per owner, callback and signing key; a replacement key must verify again. The host must enforce the abort signal on the complete exchange.

Signing secrets use AES-GCM encryption with the owner and subscription ID as authenticated associated data. They never appear in logs or MCP responses. The deployment encryption key must survive restarts, remain outside source, and be loaded by a future hosting adapter. Replacements sign with both keys for five minutes. Expiration/unsubscribe clears stored key material; dispatch clears expired rotation keys. A subscription defaults to 24 hours, caps all requests including `ttlMs: null` at 24 hours, and applies a one-minute minimum. There are no indefinite subscriptions.

Subscription creation starts after the current outbox sequence. Refresh of an active subscription preserves that sequence and pending retries. Refresh after expiration or unsubscribe resets the sequence, so old missed events are excluded. Responses return a finite `refreshBefore`, `cursor: null`, `truncated: false`; there is no protocol replay. Unsubscribe is owner-scoped, idempotent and cancels pending retries, including when project access has been lost. A request already sent cannot be recalled.

## Delivery and retry storage

`dispatchEvents` fans matching committed events into uniquely keyed subscription/event delivery rows. Each dispatch run is restricted to one explicit authenticated owner. Every send rechecks active subscription, expiration, project ownership, node existence and the hosting adapter's current owner access. A claim and one-minute lease prevent overlapping dispatchers from taking the same attempt. A process crash recovers after lease expiration with the same event ID. An accepted callback whose local acknowledgment was lost can receive a duplicate.

The dispatcher serializes one event per request and signs those exact UTF-8 bytes. It requires HTTPS and rejects redirects. Bodies are limited to 256 KiB. Network/transport errors, HTTP 408, 429 and 5xx use exponential backoff with six attempts. The base delay is one second. Each retry has a fresh signing timestamp/signature. HTTP 410 disables the subscription; 413 and other permanent HTTP errors stop that delivery. This code does not send a callback from a graph mutation or rely on request traffic to revisit failed deliveries.

D1 stores the outbox, subscriptions and delivery attempts across process restarts. A host dispatcher must run independently of requests, revisit due rows and expired leases, and clean up expired secrets. Terminal rows are retained for deduplication. Before release, establish bounded retention/cleanup appropriate to event volume; do not delete pending events or reused delivery IDs. No Worker timer or unsupported queue/cron binding is added. The new owner-scoped dispatch route returns 503 until its integration is explicitly wired. No long-lived credential is provisioned.

## Exact host blockers

The [official MCP Events contract](https://developers.openai.com/plugins/build/mcp-events) requires public destination address validation at connection time, connecting to the validated address with the original TLS hostname, redirect rejection, durable subscriptions and bounded delivery retries.

The inspected Sites guidance supports a stateless Worker MCP endpoint, D1/R2, request identity and linked cloud tasks for recurring updates. It does not document a queue/cron binding or a callback transport that pins validated public addresses. The current `build/sites-worker.ts` exports only `fetch`; `.openai/hosting.json` declares D1 and `mcp`. The native Sites metadata read on 2026-10-03 exposed linked automation metadata but no queue, cron or outbound transport settings. That read did not establish either required mechanism. A linked cloud task is not evidence of reliable delivery retries. Ordinary Worker `fetch`, URL validation alone and a separate DNS lookup cannot meet connection-time address pinning.

A tested [Node companion and relay adapter](../../event-companion/README.md) now implement socket-level DNS validation/pinning and supervised scheduling code. That code has not been deployed, and Sites background identity/current-access integration remains unverified. `EventHost` is therefore not wired into production. The route calls `mcpResponse(request)` without a host: `server/discover` omits `events`, authenticated `events/list` returns an empty catalog and `events/subscribe` returns `-32015` with `data.reason: host_unavailable`. There is no external callback or stored subscription in that mode. The outbox still records committed events for implementation verification.

Before enabling the adapter, validate:

1. The supplied companion timer on an approved external runtime, or a documented Sites-supported independent durable dispatcher. Verify supervision/restart recovery and readiness before subscription activation.
2. The supplied Node relay on the actual runtime. Its connection-time lookup checks public addresses and supplies checked numeric answers to the TLS socket. Real ingress/TLS/DNS validation and abort behavior must be verified before use.
3. A current account/connection access check for background deliveries, including revocation/disconnection. Existing D1 project ownership alone cannot establish this.
4. A stable server-only AES-GCM key and an approved operational retention policy.

Obtain documented platform support or choose an explicitly approved alternate host before wiring this adapter. Tests inject local mocks; an adapter with a boolean readiness assertion alone is not proof of host support. No undocumented hosting manifest fields should be added.

## Validation and release

`tests/events.test.ts` uses in-memory SQLite/D1 and synthetic local callback functions. No real graph content leaves the machine. It checks filtering, isolation, rollback, independent HMAC signatures, challenge failures, URL/secret validation, cache/refresh/rotation, expiration, unsubscribe, retry backoff and terminal errors, leases/crash recovery, duplicate IDs, out-of-order versions, revocation and unchanged tools. The companion tests connection-time DNS pinning, mixed addresses, rebinding, TLS options, redirects and timeout/size defenses with mocks. Real-host dispatch and SSRF validation remain unverified.

Run `npm run typecheck`, `npm run lint -- --max-warnings 0`, `npm test`, `npm run build` and `npm audit --audit-level=low`. The added `site-staging` CI job runs these checks; `event-companion` runs its tests and audit separately. Existing Go, web and container security gates remain mandatory. Existing npm/Go dependencies and lockfiles remain unchanged. The separate companion package pins one new dependency, `ipaddr.js@2.5.0`, with a clean audit. Pre-existing Go x/crypto/toolchain, npm and container OpenSSL findings still block release. Full staging source lint also fails on the unchanged `components/workspace.tsx:96` set-state-in-effect error and two warnings; changed files pass lint.

After hosting support and security gates pass, the owner must authorize and perform the staging deployment and migration, plugin rescan, and a real Work/Cloud subscription test. User-requested monitoring supplies the callback and signing key through `events/subscribe`; developers do not hard-code a receiver or provision a live subscription during implementation. Verify callback challenge, filtered delivery, refresh, revocation and unsubscribe on the real host before describing events as available. Production merge/deployment requires separate authorization.
