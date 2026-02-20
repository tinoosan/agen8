# Sovereign Agent Ideas (Parked Integration Plan)

This note captures how we can implement the core ideas from:

- **Sovereign Agents paper**: [arXiv 2602.14951](https://arxiv.org/abs/2602.14951)

without moving our platform to Conway infrastructure or rewriting `agen8`.

Status: **Parked** (design-ready; not scheduled).

---

## 1. Goal (for our stack)

Build agents that can:

1. Persist identity and intent over time.
2. Run with bounded autonomy under hard policy constraints.
3. Attribute actions to clear control layers (policy/operator/agent/scheduler/tool).
4. Start with an allowance, then **earn and replenish** operating budget.

This is "infrastructural sovereignty on our own infrastructure", not full decentralization.

---

## 2. Key Model: SOUL + Profiles + Teams

We keep our current structure and add one durable identity layer:

- `SOUL.md`: persistent core identity ("who I am", long-term values, economic intent).
- **Profiles**: runtime role overlays ("how I behave in this run/role").
- **Teams**: coordination topology for multi-role execution.

Rule of thumb:

- SOUL is durable and cross-run.
- Profiles are situational and replaceable.
- Teams are orchestration mechanics.

---

## 3. Patterns Worth Reusing from Conway Automaton

Observed in `automaton/src` and directly portable:

1. Survival tiers driven by budget (`normal`, `low_compute`, `critical`, `dead`).
2. Heartbeat daemon that continues checks and minimal actions while the agent is idle.
3. Self-preservation guards in tool execution paths (block destructive/self-harm actions).
4. Layered system prompt assembly: immutable rules + identity + dynamic resource state.
5. Append-only audit logs for self-mod and high-risk operations.
6. Payment abstraction (`x402`/USDC in their case) behind tool or client boundaries.

---

## 4. Proposed Architecture in `agen8`

### 4.1 Identity + Prompt Layer

- Add optional SOUL path in agent/runtime config.
- Inject SOUL content into system prompt each turn.
- Keep constitution/policy text as immutable preamble (non-overridable by profile prompt text).

Likely touch points:

- `pkg/prompts/base.go`
- `pkg/agent/session/prompt.go`
- `pkg/agent/config.go`
- `pkg/runtime/runtime.go`

### 4.2 Treasury Layer (new)

Add `pkg/services/treasury` with:

- Per-agent/team ledger.
- Budget state and tier calculator.
- Balance operations: `seed_allowance`, `reserve_spend`, `record_cost`, `record_revenue`.
- Allocation policy: revenue split into `operating`, `reserve`, optional `owner_withdrawal`.

Core idea:

- Agent starts funded by operator.
- Agent executes work.
- Verified revenue events top up budget.
- Tiering throttles capability before hard-stop.

### 4.3 Payment Provider Interface

Define a pluggable payment rail:

- `PaymentProvider` interface (`authorize`, `capture`, `refund`, `balance`, `transfer`).
- Start with internal/custodial implementation.
- Add optional crypto/x402 provider later without changing agent loop.

### 4.4 Policy Layer (hard gate)

Policy checks run before:

- Tool execution.
- Spend or transfer.
- Payout settlement.
- Self-modifying operations.

Policy examples:

- Per-action spend caps.
- Counterparty allowlist.
- Restricted tool families by tier.
- Emergency stop.

### 4.5 Accountability Layer

Emit append-only audit events tagged with `actor_layer`:

- `policy`
- `operator`
- `agent`
- `heartbeat`
- `payment_provider`

Events to guarantee:

- policy allow/deny
- budget transitions
- spend/revenue postings
- overrides and pause/stop actions
- identity/SOUL updates

---

## 5. Self-Funding Loop (Allowance -> Earn -> Refill)

1. Operator seeds initial allowance.
2. Agent performs tasks.
3. Agent creates `PayoutIntent` for monetizable outcomes.
4. Settlement worker validates evidence and books revenue.
5. Treasury reallocates revenue to operating budget.
6. Budget tier updates runtime behavior automatically.

Tier behavior:

- `normal`: full model/tools.
- `degraded`: lower-cost model + reduced nonessential work.
- `restricted`: essential tasks only (health, revenue recovery, critical inbox).
- `suspended`: no paid inference; heartbeat + recovery paths only.

---

## 6. Mapping to Existing Capabilities

Already present in our codebase:

- Heartbeat jobs (`profile.Heartbeat`, session scheduling).
- Quota/credit error classification in LLM retry handling.
- Team/task services and event surfaces.
- Prompt composition pipeline.

What we add:

- Treasury ledger/service.
- SOUL storage + prompt injection.
- Strict policy middleware in runtime executor chain.
- Settlement workflow for revenue ingestion.

---

## 7. Implementation Phases (when we unpark)

### Phase 1 (MVP foundation)

- SOUL support + immutable policy preamble.
- Treasury ledger with tier evaluation.
- Budget-aware tool gating.

### Phase 2 (autonomous economics)

- `PayoutIntent` + settlement worker.
- Revenue allocation rules.
- Tier-based model/tool downgrade behavior.

### Phase 3 (governance + hardening)

- Full append-only audit coverage.
- Explicit override protocol with reason codes.
- Bounded self-mod paths with protected core files.

### Phase 4 (optional crypto expansion)

- x402/USDC provider plugin.
- Optional wallet-backed identity and settlement rails.

---

## 8. Non-goals (for now)

- Migrating runtime to Conway Cloud.
- Requiring on-chain identity for base operation.
- Implementing full decentralized non-overrideability.
- Replacing profiles/teams with SOUL (SOUL complements them).

---

## 9. Open Decisions to Revisit Later

1. Revenue validation strictness (manual review threshold vs full automation).
2. Default revenue split percentages.
3. Allowed autonomous spending categories.
4. Whether team budget is shared or per-role.
5. Crypto rollout order (if/when enabled).

---

## 10. References

- Sovereign Agents paper: [https://arxiv.org/abs/2602.14951](https://arxiv.org/abs/2602.14951)
