# MCP Events and workspace validation

Base: `772bc02c72e1e5b035982b4d4c21d041640fb970`, `codex/agen8-site-plugin-extensions`. Draft [PR37](https://github.com/tinoosan/agen8/pull/37) uses the isolated `feat/mcp-events` checkout. Canonical user repositories were inspected read-only and remain untouched, including concurrent work. No hosted deployment, real subscription, credential provisioning, permission change or real-content callback occurred.

| Check on the current local feature | Result |
| --- | --- |
| Staging typecheck | Pass |
| Full staging lint, zero warnings | Pass; previous workspace effect error repaired |
| Staging tests | 47 passed, including four real D1/workerd integration tests |
| Production build | Pass; MCP, work, access and dispatch routes bundled |
| Native local preview browser QA | 12 checks passed at 1440/768/390; real local D1, synthetic graph, no page errors |
| Companion tests | 11 passed, including real HTTP signed receiver/retry/receipt and dispatcher process/file integration |
| Companion syntax / audit | Pass / zero findings |
| Hosted E2E preflight | Correctly fails without two approved genuine account sessions; live suite not run |
| Whitespace check | Pass |
| Staging security audit | Eight high findings, all the unpatched braces dependency chain |
| Root tooling audit | Seven high findings, all the unpatched braces dependency chain |
| Legacy web audit / lint / tests / build | Zero findings / pass / 669 tests passed / pass |
| Go 1.26.8 with x/crypto 0.56 tests / vet / govulncheck | Pass / pass / zero reachable vulnerabilities |
| Docker Compose configuration / workflow lint | Pass / pass; actual image build, runtime tests and scans run in CI |
| Canonical test-policy checker against this isolated tree | 320 existing naming/double violations across legacy suites; new integration/E2E additions have none |
| Actual HTTPS/Docker/Sites service dispatch | Not run; approved host and credential configuration required |
| Actual plugin disconnect / automatic revocation | Unverified; live Events remain disabled |

The selected uncommitted staging workspace/sidebar/CSS were recovered from the canonical nested staging checkout, then refined here. Browser QA caught and repaired hidden mobile work text and competing graph fit/selection behavior. Evidence is local and ignored under `site-staging/output/playwright`; it is not a hosted authentication or live subscription claim. The live suite uses the exact private dev Site with owner and outside-audience sessions, real MCP/storage and no request interception.

The four D1 integration tests execute actual Worker routes and ordered migrations, verify committed IDs/version rollback, restart persistence, tenant scope, keyed Site-owner grant creation/revocation/replacement, and owner-scoped retention. Migration `0005` preserves previous rowid cursor values while preventing reuse after complete pruning/restart. Pending/unmaterialized and another owner's events remain protected. Existing local synthetic callback tests cover signatures, filtering, retries, leases, expiration/refresh/rotation, unsubscribe, stable IDs/out-of-order versions and unchanged tools. The temporary receiver test adds real loopback HTTP and protected receipt-file persistence. No external callback received graph content.

Compatible staging patches include Next/eslint-config-next 16.3.8, React/RSC 19.3.0, current Vite/vinext/Cloudflare runtime dependencies, esbuild 0.28.2, undici 7.30.0 within Miniflare's major version, and fflate 0.7.5. Drizzle's inherited loader now resolves esbuild 0.25.12; actual TypeScript-config schema generation succeeds without migration changes. Compatible root/legacy web lock updates remove patched advisories. No `--force`, audit suppression or gate bypass was used. Remaining root/staging findings derive solely from `braces@3.0.3`, with no newer published patch on inspection. Audit proposes disruptive CLI/framework downgrades; these remain release blockers rather than an authorized broad replacement.

The required SSH denial-of-service fixes in x/crypto 0.56 require Go 1.26. Go 1.26.8, x/crypto 0.56 and their required x/term/x/sys versions pass application tests, vet and a fresh reachable-vulnerability scan. The release Dockerfile uses that exact Go version and targets OpenSSL runtime package updates. The new companion image copies only Node 24 and the application dependency into Alpine, with a targeted OpenSSL update; npm/Yarn remain in the build stage. Go 1.26 deprecates the development proxy Director API, now replaced with Rewrite while preserving loopback routing, host/path/query and credential stripping. A real two-server HTTP integration test verifies the boundary and rejects forged forwarding headers. CI builds and scans both images and runs all companion tests inside the isolated image without external networking. The local Docker daemon is unavailable; no local daemon or homelab workload was started for verification.

Historical exact-head CI at `961973d303b6a2af74353f93953b59ad413290a8`: [push run](https://github.com/tinoosan/agen8/actions/runs/37194803715) and [PR run](https://github.com/tinoosan/agen8/actions/runs/37194806593). Both failed existing security gates: staging 12 findings, root tooling 19, seven reachable Go findings and two OpenSSL plus eight Go-binary container findings. Functional staging, companion, Go tests/race/lints and container contract checks passed. Those results predate the security repairs described above. The [base branch CI](https://github.com/tinoosan/agen8/actions/runs/37153570865) already failed security gates. The PR description records the latest pushed head's CI results; historical runs do not establish its status.

Local validation uses Node 25.8.2; repository CI and the companion image use Node 24. The canonical policy checker is informational here and was not installed as a weakened CI gate. The original implementation request explicitly allowed local test-only mocks; existing suites remain, while all new boundary tests use real D1, HTTP, filesystem, process or browser dependencies. Moving the still-mandatory root audit after web checks lets CI validate the repaired web app while retaining the failing security gate.

Review [the lifecycle notes](mcp-events.md), [UI evidence](ui-polish.md) and [concrete activation proposal](live-activation.md). Staging deployment/migrations, host provisioning, persistent secrets/owner grants, live subscription testing and production merge/deployment require separate approval. TTL limits unattended access; it does not prove immediate plugin-disconnection revocation.
