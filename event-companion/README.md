# Agen8 event companion

A small Node service supplies the callback transport with connection-time DNS pinning. A systemd timer calls an owner-scoped Site dispatcher while the browser is closed. This directory contains runnable service/client code, transport tests, a real HTTP receiver integration test and deployment unit templates. It has not been installed or deployed. Site runtime wiring is prepared and defaults to disabled pending approved deployment and live runtime/disconnection verification.

## What runs where

D1 retains subscriptions, encrypted signing keys, minimal outbox records, delivery IDs, retry counts and leases. The Site signs the exact webhook bytes, applies tenant/project/node/transition filters and rechecks the stored application grant and resource ownership. It sends those signed bytes to the configured companion over HTTPS. The companion sees the callback URL and minimal event payload but does not receive webhook signing keys, encryption keys, D1 credentials or graph mutation access.

The relay accepts only authenticated `/relay` POSTs with event bodies and the five Standard Webhooks headers. It cannot forward arbitrary methods or authorization headers. A separate `/health` endpoint requires the same relay credential and reports whether a dispatch process started within the last 90 seconds. It proves recent scheduler invocation, not successful delivery or perpetual scheduler health. Operational setup must verify the systemd timer and monitor failed runs; a file written once by hand is insufficient.

`transport.mjs` uses Node HTTPS with a new direct TLS agent per request. The custom socket lookup resolves all destination addresses, rejects non-public addresses, and returns the validated numeric answer to the connection. There is no second DNS lookup. Original hostname/SNI and certificate verification remain enabled. It rejects literal IP callback URLs, reserved/private/mapped/translation/tunnel addresses, non-HTTPS, credentials, fragments, nonstandard ports and redirects. It ignores environment proxy settings. Exchange timeout is ten seconds, callback response bytes are capped at 4 KiB, outgoing event bodies at 256 KiB and concurrent relays at eight. DNS is revalidated on every new connection.

`dispatch.mjs` calls only the configured Site's fixed `/api/events/dispatch` route, after atomically writing a scheduler heartbeat. It sends `OAI-Sites-Authorization` for platform service access and a separate dispatch key. Sites consumes the service credential without adding visitor identity. The route derives the owner from its configured, active D1 grant; it ignores caller identities and bodies. Neither service credential is forwarded to the callback relay.

The supplied timer runs after boot and thirty seconds after each dispatch service finishes. The single-event dispatch limit keeps each invocation within the twenty-second client and twenty-five-second service deadlines. This gives about two event attempts per minute and up to roughly thirty seconds of timer delay. D1 backoff controls the earliest next attempt; the timer cadence can delay it further. A restart resumes durable pending rows and expired leases. The companion itself stores no delivery queue. This modest throughput is suitable for an initial personal graph; higher volume requires measured dispatch concurrency/batch tuning and renewed validation.

## Supported Sites authorization and remaining verification

Current Sites authentication guidance permits shared updates through service access on a confirmed owner-private Site. Agen8 Dev was confirmed owner-private on 2026-10-04, with only its owner allowed and no connected-app requirement. This supplies the platform access boundary; the separate dispatch capability and revocable D1 owner grant narrow background execution to authenticated subscriptions for one Site principal. The previous requirement for an identity-bearing background OAuth token was unnecessary.

Use `grantOwnerAccess(db, grantId)` in `createRelayHost`, and the same grant ID in `eventDispatchResponse`. `createDispatchGrant` captures the real Site principal from an authenticated request during separately approved privileged setup. Do not assume account metadata IDs equal Site principals. Grants last at most 24 hours and require deliberate renewal; creating/replacing them revokes old subscriptions. `revokeDispatchGrant` atomically disables the grant, clears encrypted secrets and cancels queued attempts. Subscribing cannot reauthorize a revoked grant. The privileged helper is exposed only through the disabled-by-default, separately keyed operational `/api/events/access` route; it is absent from MCP tools and graph UI. Service credentials alone cannot renew a grant.

Immediate plugin-disconnection behavior is still unverified. Grant revocation and expiry fail closed, but their tests do not prove automatic revocation on ChatGPT disconnect. Before live activation, verify the platform's disconnect/unsubscribe behavior or an approved revocation integration. No invented token-introspection/refresh API is required. The independently recurring timer and secure relay must also be installed and tested on an approved host; no such service is running from this PR.

## Minimal deployment requirements, pending approval

1. An approved Linux host with a supported, patched Node.js 24+ runtime, systemd and outbound public HTTPS/DNS. Reuse an existing host if available; otherwise approve its provider and cost. Install the pinned package with `npm ci --ignore-scripts` after its audit passes.
2. A controlled HTTPS relay hostname and TLS ingress forwarding to `127.0.0.1:8788`. Keep the service loopback-only; preserve default certificate validation for outbound requests. Limit ingress requests to 2 MiB and do not log authorization headers, URLs with callback identifiers, webhook headers or request/response bodies. Provisioning DNS/TLS/network rules requires approval.
3. Separate unprivileged users `agen8-relay` and `agen8-dispatch`, with group `agen8-events`. Precreate `/var/lib/agen8-events` as dispatcher-owned, group-readable, mode 0750. Heartbeat files use mode 0640. The relay reads this state; only the dispatcher writes it. The unit files use read-only application files, private temporary storage and process visibility restrictions.
4. Two separately generated 32-byte canonical-base64 credentials: relay token and dispatch key. The relay token is configured only in the Site and relay process. The dispatch key is configured only in the Site and dispatcher. Keep them in root-owned mode-0600 environment files at `/etc/agen8-events/relay.env` and `dispatch.env`, loaded by systemd. Do not put values in commands, source or logs.
5. A stable 32-byte Site-only AES-GCM encryption key, fixed relay origin, configured Site-specific owner grant ID, `grantOwnerAccess` and the existing Sites service credential for the exact Site origin. Grant creation/renewal and persistent credential configuration require action-time approval.
6. Wire one verified host into both MCP handling and the dispatch route, apply staging's schema-only migration, validate readiness, timer restart recovery, TLS/DNS defenses, revocation and owner isolation using synthetic events, then authorize staging publication and a real subscription separately. Verify the implemented owner-scoped 30-day retention and AUTOINCREMENT cursor migration on staging.

The code expects these environment names on the external host:

| Process | Names |
| --- | --- |
| Relay | `AGEN8_RELAY_TOKEN`, `AGEN8_HEARTBEAT_PATH` |
| Dispatcher | `AGEN8_DISPATCH_URL`, `AGEN8_DISPATCH_KEY`, `AGEN8_SITES_SERVICE_BEARER`, `AGEN8_HEARTBEAT_PATH` |

`AGEN8_HEARTBEAT_PATH` should be `/var/lib/agen8-events/dispatch.json` in both files. The Node binary and checkout paths in unit templates are deployment choices, not assumptions about a provisioned machine. No systemd installation commands were run. Real ingress/TLS, systemd unit validation, Site service authorization, disconnection/revocation and multi-host recovery require verification on the approved runtime.

## Local validation

`npm test` runs synthetic socket/DNS/HTTP mocks and local temporary heartbeat files. It opens no external callback. Tests cover public/non-public IPv4/IPv6, mixed DNS answers, connection-time pinning and rebinding, original TLS hostname and certificate-check flags, redirect/TLS/abort/size failures, relay authorization and capacity, readiness expiration and atomic heartbeat/dispatch behavior. `npm audit --audit-level=low` checks the one pinned dependency, `ipaddr.js@2.5.0`. The fresh audit found zero vulnerabilities on 2026-10-04. Mock TLS tests do not establish real-host network behavior.

Primary contracts: [Node HTTPS](https://nodejs.org/api/https.html), [Node socket lookup](https://nodejs.org/api/net.html#socketconnectoptions-connectlistener), [systemd timer definitions](https://github.com/systemd/systemd/blob/main/man/systemd.timer.xml), [MCP Events](https://developers.openai.com/plugins/build/mcp-events). The temporary `test-receiver.mjs` accepts only signed synthetic-project notifications, returns two initial 503 responses per stable event ID, deduplicates accepted IDs and expires after ten minutes. Its real loopback HTTP integration test needs no public receiver, real content or live secret. Deployment and real disconnection steps are in the [activation runbook](../site-staging/docs/live-activation.md).

The runtime integration and core lifecycle are described in [Agen8 Dev MCP Events](../site-staging/docs/mcp-events.md).
