# Agen8 event companion

A small Node service supplies the callback transport that the current Sites Worker cannot implement. A systemd timer calls an owner-scoped Site dispatcher while the browser is closed. This directory contains runnable service/client code, mock tests and deployment unit templates. It has not been installed or deployed. The Site routes remain disabled until background identity and current access checks are verified.

## What runs where

D1 retains subscriptions, encrypted signing keys, minimal outbox records, delivery IDs, retry counts and leases. The Site signs the exact webhook bytes, applies tenant/project/node/transition filters and rechecks account access. It sends those signed bytes to the configured companion over HTTPS. The companion sees the callback URL and minimal event payload but does not receive webhook signing keys, encryption keys, D1 credentials or graph mutation access.

The relay accepts only authenticated `/relay` POSTs with event bodies and the five Standard Webhooks headers. It cannot forward arbitrary methods or authorization headers. A separate `/health` endpoint requires the same relay credential and reports whether a dispatch process started within the last 90 seconds. It proves recent scheduler invocation, not successful delivery or perpetual scheduler health. Operational setup must verify the systemd timer and monitor failed runs; a file written once by hand is insufficient.

`transport.mjs` uses Node HTTPS with a new direct TLS agent per request. The custom socket lookup resolves all destination addresses, rejects non-public addresses, and returns the validated numeric answer to the connection. There is no second DNS lookup. Original hostname/SNI and certificate verification remain enabled. It rejects literal IP callback URLs, reserved/private/mapped/translation/tunnel addresses, non-HTTPS, credentials, fragments, nonstandard ports and redirects. It ignores environment proxy settings. Exchange timeout is ten seconds, callback response bytes are capped at 4 KiB, outgoing event bodies at 256 KiB and concurrent relays at eight. DNS is revalidated on every new connection.

`dispatch.mjs` makes one authenticated request to the fixed `/api/events/dispatch` route, after atomically writing a scheduler heartbeat. Its dispatch key is separate from the relay key. It sends no trusted user-ID header; only Sites may attach that identity after authenticating the caller. The dispatch route requires both the trusted identity for the configured owner and the dispatch key, plus the runtime current-access check. A Sites service-bypass token alone does not meet this requirement.

The supplied timer runs after boot and thirty seconds after each dispatch service finishes. The single-event dispatch limit keeps each invocation within the twenty-second client and twenty-five-second service deadlines. This gives about two event attempts per minute and up to roughly thirty seconds of timer delay. D1 backoff controls the earliest next attempt; the timer cadence can delay it further. A restart resumes durable pending rows and expired leases. The companion itself stores no delivery queue. This modest throughput is suitable for an initial personal graph; higher volume requires measured dispatch concurrency/batch tuning and renewed validation.

## Remaining Sites integration blocker

A generic Sites service token grants Site access without a visitor identity. The inspected Sites documentation provides neither a background principal/connection authorization check nor a verified identity-bearing authorization path for this new API route. The companion cannot invent trusted identity headers or treat D1 project ownership as proof that the user's connection remains authorized.

`createRelayHost` takes a current-owner-access function with no default. `eventDispatchResponse` takes an explicit owner, dispatch key, host and D1 handle. Neither is wired into the deployed routes. `AGEN8_SITE_IDENTITY_BEARER` is a contract for a separately approved, verified identity-bearing authorization mechanism, not a claim that a Sites bypass token or an existing plugin OAuth token works on this API route. Token acquisition/refresh and access introspection remain unimplemented until the platform's supported contract is established. An expired or invalid token must fail closed.

If Sites cannot provide that contract, the owner must approve moving event dispatch and its authorization boundary to a host that can, or a platform-supported alternative. Do not expand access, export D1 credentials or relax identity checks just to make the timer succeed. This PR does not claim an end-to-end deployable subscription system on the current Sites host.

## Minimal deployment requirements, pending approval

1. An approved Linux host with a supported, patched Node.js 24+ runtime, systemd and outbound public HTTPS/DNS. Reuse an existing host if available; otherwise approve its provider and cost. Install the pinned package with `npm ci --ignore-scripts` after its audit passes.
2. A controlled HTTPS relay hostname and TLS ingress forwarding to `127.0.0.1:8788`. Keep the service loopback-only; preserve default certificate validation for outbound requests. Limit ingress requests to 2 MiB and do not log authorization headers, URLs with callback identifiers, webhook headers or request/response bodies. Provisioning DNS/TLS/network rules requires approval.
3. Separate unprivileged users `agen8-relay` and `agen8-dispatch`, with group `agen8-events`. Precreate `/var/lib/agen8-events` as dispatcher-owned, group-readable, mode 0750. Heartbeat files use mode 0640. The relay reads this state; only the dispatcher writes it. The unit files use read-only application files, private temporary storage and process visibility restrictions.
4. Two separately generated 32-byte canonical-base64 credentials: relay token and dispatch key. The relay token is configured only in the Site and relay process. The dispatch key is configured only in the Site and dispatcher. Keep them in root-owned mode-0600 environment files at `/etc/agen8-events/relay.env` and `dispatch.env`, loaded by systemd. Do not put values in commands, source or logs.
5. A stable 32-byte Site-only AES-GCM encryption key, fixed relay origin, configured account owner, a verified current-access callback and approved identity-bearing dispatcher authorization with supported expiry/refresh. Any OAuth grant or other persistent credential requires action-time approval.
6. Wire one verified host into both MCP handling and the dispatch route, apply staging's schema-only migration, validate readiness, timer restart recovery, TLS/DNS defenses, revocation and owner isolation using synthetic events, then authorize staging publication and a real subscription separately. Choose and implement bounded outbox/delivery retention before release.

The code expects these environment names on the external host:

| Process | Names |
| --- | --- |
| Relay | `AGEN8_RELAY_TOKEN`, `AGEN8_HEARTBEAT_PATH` |
| Dispatcher | `AGEN8_DISPATCH_URL`, `AGEN8_DISPATCH_KEY`, `AGEN8_SITE_IDENTITY_BEARER`, `AGEN8_HEARTBEAT_PATH` |

`AGEN8_HEARTBEAT_PATH` should be `/var/lib/agen8-events/dispatch.json` in both files. The Node binary and checkout paths in unit templates are deployment choices, not assumptions about a provisioned machine. No systemd installation commands were run. Real ingress/TLS, systemd unit validation, identity authorization, revocation and multi-host recovery require verification on the approved runtime.

## Local validation

`npm test` runs synthetic socket/DNS/HTTP mocks and local temporary heartbeat files. It opens no external callback. Tests cover public/non-public IPv4/IPv6, mixed DNS answers, connection-time pinning and rebinding, original TLS hostname and certificate-check flags, redirect/TLS/abort/size failures, relay authorization and capacity, readiness expiration and atomic heartbeat/dispatch behavior. `npm audit --audit-level=low` checks the one pinned dependency, `ipaddr.js@2.5.0`. The recorded audit found zero vulnerabilities on 2026-10-03. Mock TLS tests do not establish real-host network behavior.

Primary contracts: [Node HTTPS](https://nodejs.org/api/https.html), [Node socket lookup](https://nodejs.org/api/net.html#socketconnectoptions-connectlistener), [systemd timer definitions](https://github.com/systemd/systemd/blob/main/man/systemd.timer.xml), [MCP Events](https://developers.openai.com/plugins/build/mcp-events). The runtime integration and core lifecycle are described in [Agen8 Dev MCP Events](../site-staging/docs/mcp-events.md).
