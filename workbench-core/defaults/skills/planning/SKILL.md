---
name: Planning
description: Break down complex tasks into actionable steps with clear milestones and dependencies.
---

# Instructions

Use this skill when facing complex, multi-phase work that requires careful sequencing, risk analysis, or stakeholder alignment before execution. This skill helps you create clear, executable plans that prevent costly rework.

## When to use

- The task involves multiple components, teams, or integration points.
- Changes have architectural implications or breaking change risk.
- The user needs a proposal before committing resources.
- The scope is unclear and needs decomposition.
- There are multiple valid approaches that need evaluation.

## Workflow

1. **Understand the objective.** Clarify the business goal, user need, or problem being solved. Identify constraints (time, budget, compatibility, performance).

2. **Gather context.** Survey the existing codebase, dependencies, and related systems. Identify what works today and what's missing.

3. **Identify alternatives.** List 2-3 viable approaches with trade-offs. Consider refactor vs. rewrite, buy vs. build, incremental vs. big-bang.

4. **Decompose into phases.** Break work into logical milestones that deliver value independently. Each phase should be testable and reversible.

5. **Map dependencies.** Identify what must happen first (data migration before new API, tests before refactor). Note external blockers (API keys, access, third-party approvals).

6. **Estimate effort & risk.** Use T-shirt sizes (S/M/L) or time ranges. Flag high-risk areas (legacy code, third-party integrations, performance unknowns).

7. **Define success criteria.** For each phase, specify what "done" looks like: passing tests, performance benchmarks, user acceptance, or deployment to production.

8. **Document the plan.** Write a clear, scannable document with:
   - **Goal** - One sentence describing what success looks like
   - **Context** - Why this matters, what exists today
   - **Approach** - Chosen solution with rationale
   - **Phases** - Numbered milestones with deliverables
   - **Risks** - Known unknowns and mitigation strategies
   - **Non-goals** - What's explicitly out of scope

9. **Request review.** Share the plan with stakeholders before implementation. Incorporate feedback and adjust.

## Decision rules

- Prefer incremental delivery over Big Bang launches. Each phase should add value.
- If a phase exceeds 1 week of effort, break it down further.
- Always have a rollback plan for risky changes (feature flags, database migrations, API versioning).
- Document assumptions explicitly—they often become the source of surprises.
- When multiple approaches are equally valid, choose the **simplest** one that meets requirements.

## Quality checks

- [ ] The plan has a clear, measurable goal.
- [ ] Each phase delivers standalone value.
- [ ] Dependencies are explicitly stated (blockers, prerequisites).
- [ ] Risks are identified with mitigation strategies.
- [ ] Success criteria are testable/verifiable.
- [ ] Out-of-scope items are defined to prevent scope creep.

## Templates

### Option Comparison

| Approach | Pros | Cons | Effort | Risk |
|----------|------|------|--------|------|
| Refactor existing | Preserves data, low risk | Slower, tech debt remains | M | Low |
| Rewrite from scratch | Clean slate, modern stack | High risk, data migration | L | High |
| Hybrid (phase migration) | Incremental, testable | Complex coordination | M-L | Medium |

### Phase Breakdown

**Phase 1: Foundation**
- Goal: Set up infrastructure
- Deliverables: Database schema, API scaffolding, CI/CD pipeline
- Success: Tests pass, deployment works
- Effort: 3-5 days
- Risks: Cloud permissions, quota limits

**Phase 2: Core Logic**
- Goal: Implement business rules
- Deliverables: Core API endpoints, validation, error handling
- Success: 80% test coverage, manual QA pass
- Effort: 1-2 weeks
- Risks: Edge cases, performance with large datasets

**Phase 3: Integration**
- Goal: Connect to existing systems
- Deliverables: Webhook handlers, data sync, auth flow
- Success: End-to-end flow works in staging
- Effort: 1 week
- Risks: Third-party API downtime, rate limits

**Phase 4: Launch**
- Goal: Production deployment
- Deliverables: Rollout plan, monitoring, docs
- Success: Live in prod, metrics green
- Effort: 2-3 days
- Risks: Traffic spike, edge cases

## Example

**User request**: "Migrate our authentication from OAuth1 to OAuth2"

**Plan**:
1. **Context**: OAuth1 is deprecated, blocking new integrations. 5,000 active users.
2. **Approach**: Dual-mode migration (support both OAuth1 and OAuth2 during transition)
3. **Phases**:
   - Phase 1: Add OAuth2 support alongside OAuth1 (1 week)
   - Phase 2: Migrate 10% of users, monitor errors (3 days)
   - Phase 3: Full migration, deprecate OAuth1 (1 week)
4. **Risks**: Session invalidation (give 7-day notice), token expiry edge cases (add retry logic)
5. **Success**: All users on OAuth2, OAuth1 code removed, zero downtime

## Anti-patterns to avoid

- **No plan = plan to fail** - Jumping into implementation without clear milestones leads to scope creep and missed edge cases.
- **Over-planning** - Don't spend 3 days planning a 1-day task. Match planning depth to complexity.
- **Ignoring stakeholders** - Plans created in isolation often miss critical constraints or business requirements.
- **Optimistic estimates** - Always buffer for unknowns. If you think it's 3 days, call it 5.
- **Treating the plan as gospel** - Plans change. Be ready to adapt when new information emerges.

## Advanced techniques

### Risk matrix

Plot risks on Impact (low/high) vs. Likelihood (low/high):

- **High Impact + High Likelihood** → Mitigate immediately (add feature flags, extra testing)
- **High Impact + Low Likelihood** → Have a contingency plan (data backup, rollback script)
- **Low Impact + High Likelihood** → Accept and monitor (minor UI glitches)
- **Low Impact + Low Likelihood** → Ignore

### Critical path analysis

Identify the longest sequence of dependent tasks. This is your minimum timeline. Parallelize non-dependent work to shorten delivery.

Example:
```
Database schema (2 days) → API backend (5 days) → Frontend (3 days) = 10 days minimum
Testing can happen in parallel with frontend = saves 1-2 days
```

### Spike planning

For high-uncertainty work, dedicate time-boxed "spikes" to reduce risk before committing:
- "Spend 1 day prototyping the new auth flow to validate feasibility"
- "2-hour investigation: Can we use library X or do we need a custom solution?"

Spikes should answer a specific question and have a hard time limit.

## When NOT to use this skill

- Simple, well-understood tasks (bug fixes, copy changes, routine updates) - just do them
- Urgent hotfixes - plan quickly, execute faster
- Exploratory work where the goal is to learn, not deliver
- Tasks with rigid, predefined plans (just follow the existing spec)
