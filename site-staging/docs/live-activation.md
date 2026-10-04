# Agen8 Dev activation proposal and test runbook

This is a proposal, not an installed service or approved deployment. The implementation and draft PR can be reviewed now. Security audits and real-host disconnection verification remain release blockers. No host, network rule, credential, owner grant or live subscription has been created by this work.

## Smallest runtime proposal

Reuse an existing approved Linux host if it supports patched Node.js 24+, systemd, public HTTPS/DNS egress and a controlled TLS hostname. Otherwise propose one DigitalOcean Basic Droplet: 1 vCPU, 1 GiB RAM, 25 GiB SSD and 1,000 GiB transfer, advertised at **$6/month** on 2026-10-04 ([official pricing](https://www.digitalocean.com/pricing/droplets)). Taxes, excess transfer and any new domain registration are additional. Region and hostname need the owner's choice; London/Ubuntu 24.04 is a candidate, not a provisioned resource. Build/install dependencies before running on the small VM and measure capacity.

The host runs only the companion relay plus the supervised dispatcher timer; the graph and durable queue stay in the existing dev Site/D1. Ingress is HTTPS 443 to a TLS reverse proxy, then loopback `127.0.0.1:8788`; permit ACME HTTP 80 only if that certificate method is approved. Restrict SSH to the owner's approved key/source addresses. Egress permits public DNS and HTTPS. Do not open the relay loopback port publicly. Two unprivileged service users and mode-0600 environment files isolate relay and dispatcher credentials. Unit templates are in `event-companion/systemd`; no unit has been installed or validated on a real host.

## Approval scope and credentials

Approve the host/provider/cost, owned HTTPS hostname, DNS/TLS and network changes, service installation, dev-only schema migration/deployment, and the following separately scoped secret configuration. Preserve owner-private Site policy and production unchanged. No D1 credential or connected-app/OAuth grant is required for this companion.

| Setting | Kept where | Purpose |
| --- | --- | --- |
| Fresh 32-byte relay token | Site and relay only | Authenticated fixed relay access |
| Fresh 32-byte dispatch key | Site and dispatcher only | Separate background dispatch capability and owner setup authorization |
| Stable 32-byte AES-GCM key | Site only | Encrypt signing secrets in D1 |
| Existing dev Sites service bearer | Dispatcher only, exact dev origin | Platform service access; supplies no visitor identity |
| Finite D1 owner grant ID | Site only | Actual Site-authenticated principal, one minute–24 hours |
| Temporary signing secret and genuine test sessions | Approved receiver / local ignored account files | Synthetic end-to-end verification; delete after testing |

Never put values in source, command arguments, prompts, PR text or logs. The companion never receives the AES key or webhook signing keys. It receives signed minimal event bytes. Existing service credentials must be configured only after approval; do not mint replacement OAuth grants or assume account metadata IDs are Site principals.

Site runtime settings are `AGEN8_EVENTS_ENABLED` (default off), `AGEN8_EVENT_RELAY_URL`, `AGEN8_EVENT_RELAY_TOKEN`, `AGEN8_EVENT_ENCRYPTION_KEY`, `AGEN8_EVENT_GRANT_ID`, `AGEN8_EVENT_DISPATCH_KEY`. Host settings are documented in [the companion README](../../event-companion/README.md). Use the exact dev `/api/events/dispatch` URL and a common protected heartbeat path.

Apply schema-only migrations through `0005` to dev D1 after approval. Configure the dispatch key while Events remain off. An owner-signed-in request plus that key can POST `{ "action": "grant", "ttlMs": 86400000 }` to `/api/events/access`; it captures the genuine Site principal and returns the finite grant ID. Configure that ID in the Site runtime. Service access alone cannot create/refresh the grant. Grant replacement clears subscriptions and retries, so unattended monitoring stops within 24 hours unless the owner deliberately renews and resubscribes. This limitation is explicit; subscription refresh never expands the grant.

## Approved synthetic verification

1. Confirm the real relay's TLS certificate, auth rejection, redirect defense, DNS/socket pinning and timer readiness. Validate supplied systemd units on the actual OS. Stop/restart services and confirm D1 pending rows/leases resume while the browser is closed. A recent heartbeat proves a scheduler invocation, not delivery success.
2. Obtain genuine dedicated account sessions through approved Sites sign-in. Run hosted E2E with the exact dev URL, owner account, an outside-audience account and explicit write consent. The second account must remain denied by private Site policy. Do not fabricate authentication or change sharing to make the test pass. Archive synthetic projects afterward.
3. For retry transport verification only, approve a temporary TLS path `/test/callback` to loopback `127.0.0.1:8790` on the same controlled host. `test-receiver.mjs` requires an explicitly labelled synthetic `project_id` and approved signing secret, validates exact Standard Webhooks bytes, echoes only valid signed challenges, fails the first two attempts and accepts subsequent stable-ID duplicates once. It shuts down after ten minutes. Receipt files contain event IDs/attempt counts, not graph summaries/bodies/URLs; keep them protected and delete them after review. Do not use real graph content. This receiver is a transport test, not a ChatGPT subscription.
4. Enable Events only in approved dev configuration for the test window after verifying the access/disconnection policy. Rescan the actual Agen8 Dev plugin. Ask ChatGPT for the intended synthetic monitoring; ChatGPT supplies its real callback and signing key through `events/subscribe`. Never substitute a guessed callback or manually mint a live automation during implementation.
5. Exercise project/node/status filters, decision creation, refresh, finite expiration, explicit unsubscribe and grant revocation. Capture stable event IDs and versions while rereading authoritative records. Verify retry IDs remain stable and signatures/timestamps refresh; newer notifications may arrive before older retries.
6. Disconnect the actual plugin/account and change only the synthetic graph through an independently authorized path. Confirm no further callback is accepted/sent according to the observed unsubscribe/revocation mechanism. Inspect secrets cleared and queued attempts cancelled. **TTL alone is not proof of immediate disconnection revocation.** If the platform supplies no supported immediate mechanism, disable Events and report that blocker; do not invent introspection or token refresh.
7. Revoke the test grant, unsubscribe, disable Events, remove the temporary receiver route/secrets/session files and archive test records. A request already sent cannot be recalled. Keep production unchanged.

The current implementation has no native Sites cron/queue assumption: the Worker exports only `fetch`. D1 supplies durable state; an approved independent timer supplies dispatch. Until real hosting and disconnection checks succeed, report the feature as implemented for review with live Events disabled, not as operational monitoring.
