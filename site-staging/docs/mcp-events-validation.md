# MCP Events validation evidence

Base: `772bc02c72e1e5b035982b4d4c21d041640fb970`, `codex/agen8-site-plugin-extensions`. Implementation lives on the isolated `feat/mcp-events` checkout. The user's dirty canonical checkout was inspected read-only and left untouched. No live graph data or external callback was used by tests. No deployment, subscription, OAuth grant or credential was provisioned.

| Check | Result |
| --- | --- |
| Staging `npm run typecheck` | Pass |
| Staging `npm test` | 44 passed, including 26 event/relay tests |
| Staging `npm run build` | Pass, including disabled dispatch route |
| ESLint on changed TypeScript source/tests, `--max-warnings 0` | Pass |
| Companion `npm test` | 9 passed |
| Companion `node --check` | Pass for service, dispatcher and transport |
| Companion audit | 0 vulnerabilities |
| `go test ./... -count=1` | Pass, pinned Go 1.25.12 |
| `go vet ./...`, staticcheck, revive | Pass |
| Tracked Go formatting, actionlint, `git diff --check` | Pass |
| Full staging source lint, excluding generated app bundle | Existing `components/workspace.tsx:96` error and two warnings |
| Root tooling audit | Existing 19 findings: 13 high, 4 moderate, 2 low |
| Legacy web audit | Existing 11 findings: 6 high, 5 moderate |
| Staging audit | Existing 28 findings: 1 critical, 16 high, 10 moderate, 1 low |
| `govulncheck ./...` | Existing 7 reachable findings in x/crypto and Go standard library |
| Local container scan | Not run: Docker daemon unavailable |
| Real Linux/systemd/HTTPS deployment | Not performed; requires approval and runtime verification |
| Sites service-access design | Supported for the confirmed owner-private dev Site; no visitor identity required |
| Real plugin disconnection / automatic revocation | Unverified; live Events remain disabled |

The 2026-10-04 service-access correction adds a finite, revocable Site-owner grant and removes the invented background visitor/OAuth requirement. Synthetic tests additionally verify the exact service header, lack of manufactured identity, grant ownership/expiry/replacement, revocation during verification/delivery, secret clearing, cancelled retries and refresh without grant renewal. No live grant was created. Staging types, 44 tests, build and changed-file lint pass; the companion still passes 9 tests and a fresh audit reports zero findings. Fresh staging audit still reports the same 28 findings. Full source lint still fails only in unchanged `workspace.tsx`.

The critical staging finding affects the inherited `next@16.3.4`; the audit reports `16.3.8` as its fix. Go findings include `x/crypto@v0.52.0` and standard library fixes after pinned Go `1.25.12`. Existing dependency manifests and lockfiles are unchanged; the companion adds only a separate pinned `ipaddr.js@2.5.0` package with its own lockfile and audit. No security gate was disabled.

The [base branch CI run](https://github.com/tinoosan/agen8/actions/runs/37153570865) already failed at Go security checks, root npm audit and container scan. The existing OpenSSL container blocker from prior investigation remains unresolved; this environment could not rerun it. Full source lint findings were confirmed in an unchanged file.

Local verification used Node `25.8.2`; CI uses the repository's Node `24` setting. The new staging and companion jobs cover this change. A passing companion job does not resolve unrelated release audits. The implementation is suitable for code review; live events remain disabled until the requirements in [the lifecycle notes](mcp-events.md) and [companion deployment instructions](../../event-companion/README.md) are met.
