# MCP Events and workspace validation

Base: `772bc02c72e1e5b035982b4d4c21d041640fb970`, `codex/agen8-site-plugin-extensions`. Draft [PR37](https://github.com/tinoosan/agen8/pull/37) uses the isolated `feat/mcp-events` checkout. Canonical user repositories were inspected read-only and remain untouched, including concurrent work. No hosted deployment, real subscription, credential provisioning, permission change or real-content callback occurred.

| Check on the current local feature | Result |
| --- | --- |
| Staging typecheck | Pass |
| Full staging lint, zero warnings | Pass; previous workspace effect error repaired |
| Staging tests | 47 passed, including four real D1/workerd integration tests |
| Production build | Pass; MCP, work, access and dispatch routes bundled |
| Native local preview browser QA | 12 checks passed at 1440/768/390; real local D1, synthetic graph, no page errors |
| Companion tests | 10 passed, including real HTTP signed receiver/retry/receipt integration |
| Companion syntax / audit | Pass / zero findings |
| Hosted E2E preflight | Correctly fails without two approved genuine account sessions; live suite not run |
| Whitespace check | Pass |
| Staging security audit | 12 findings: eight high, four moderate; down from 28 including one critical |
| Canonical test-policy checker against this isolated tree | 320 existing naming/double violations across legacy suites; new integration/E2E additions have none |
| Actual HTTPS/systemd/Sites service dispatch | Not run; approved host and credential configuration required |
| Actual plugin disconnect / automatic revocation | Unverified; live Events remain disabled |

The selected uncommitted staging workspace/sidebar/CSS were recovered from the canonical nested staging checkout, then refined here. Browser QA caught and repaired hidden mobile work text and competing graph fit/selection behavior. Evidence is local and ignored under `site-staging/output/playwright`; it is not a hosted authentication or live subscription claim. The live suite uses the exact private dev Site with owner and outside-audience sessions, real MCP/storage and no request interception.

The four D1 integration tests execute actual Worker routes and ordered migrations, verify committed IDs/version rollback, restart persistence, tenant scope, keyed Site-owner grant creation/revocation/replacement, and owner-scoped retention. Migration `0005` preserves previous rowid cursor values while preventing reuse after complete pruning/restart. Pending/unmaterialized and another owner's events remain protected. Existing local synthetic callback tests cover signatures, filtering, retries, leases, expiration/refresh/rotation, unsubscribe, stable IDs/out-of-order versions and unchanged tools. The temporary receiver test adds real loopback HTTP and protected receipt-file persistence. No external callback received graph content.

Compatible staging patches include Next/eslint-config-next 16.3.8, React/RSC 19.3.0, current Vite/vinext/Cloudflare runtime dependencies, esbuild 0.28.2, undici 7.30.0 within Miniflare's major version, and fflate 0.7.5. No `--force`, audit suppression, major downgrade or gate bypass was used. Remaining findings derive from `braces@3.0.3` (no newer published patch on inspection) and old `esbuild@0.18.20` under Drizzle's inherited loader. Audit proposes disruptive toolchain downgrades rather than a supported compatible fix. These findings still block the staging security gate.

Root tooling, legacy web, Go manifests and container files are unchanged. Prior exact-head CI at `d4127dcf8581a919d17e5f952aa8c716e95701f0` completed: [push run](https://github.com/tinoosan/agen8/actions/runs/37190500417) and [PR run](https://github.com/tinoosan/agen8/actions/runs/37190501742). Companion checks passed. Go formatting/vet/staticcheck/revive, unit/race tests passed before seven reachable x/crypto/toolchain security findings. Root tooling audit reported 19 findings (13 high); container contract/build passed before two OpenSSL and eight Go-binary high findings. The [base branch CI](https://github.com/tinoosan/agen8/actions/runs/37153570865) already failed those security gates. The PR description records the final pushed head's CI results; prior runs do not establish the new head's status.

Local validation uses Node 25.8.2; repository CI uses Node 24. No legacy test conversion, Go security upgrade or container rebuild fix is included. The copied canonical policy checker is informational here and was not installed as a weakened CI gate. The original implementation request explicitly allowed local test-only mocks; existing suites remain, while all new boundary tests use real D1, HTTP or browser dependencies.

Review [the lifecycle notes](mcp-events.md), [UI evidence](ui-polish.md) and [concrete activation proposal](live-activation.md). Staging deployment/migrations, host provisioning, persistent secrets/owner grants, live subscription testing and production merge/deployment require separate approval. TTL limits unattended access; it does not prove immediate plugin-disconnection revocation.
