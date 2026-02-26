---
name: product-management
description: Turn high-level ideas into prioritized roadmaps, clear requirements, and acceptance criteria. Provide templates, decision rules, and quality checks to enable fast, aligned execution by the engineering, design, and growth teams.
owner: ceo
---

# Product Management Skill

This skill captures a practical, repeatable product-management workflow tailored for a small cross-functional startup team. The goal is to help the CEO/coordinator convert strategy into delegatable work with clear acceptance criteria and measurable success metrics.

## When to use

- You have a business or product idea that needs to be validated, scoped, and delivered by specialists (cto, designer, growth-lead).
- You need a single-source-of-truth Product Requirements Document (PRD) and roadmap for a milestone or MVP.
- You want to standardize how features are specified, prioritized, and accepted.

## Outcomes / Deliverables

- PRD (Product Requirements Document) for the feature/initiative
- Prioritized roadmap or milestone plan (1-3 month scope)
- User stories and acceptance criteria suitable for implementation
- Success metrics and instrumentation checklist
- Release checklist and rollout plan
- Post-release evaluation template

## Workflow (step-by-step)

1. Context & Objective (CEO)
   - State the business objective, target user, and desired outcome in 1-2 sentences.
   - Provide relevant constraints (timeline, budget, regulatory needs).

2. Discovery & Validation Plan (CEO + growth-lead)
   - List assumptions and the riskiest parts of the idea.
   - Propose validation experiments (interviews, landing pages, prototypes) with success/failure criteria.

3. Define Success Metrics (CEO)
   - Primary metric (north star) for the initiative.
   - Supporting metrics (activation, retention, conversion, revenue) and targets.

4. Scope & Prioritization (CEO + CTO + Designer)
   - Split work into MVP scope vs future enhancements.
   - Use a prioritization framework (RICE or MoSCoW) and produce a ranked backlog for the milestone.

5. Write the PRD (CEO / Product)
   - Include: Objective, Background, Target user, User problems, Proposed solution, Scope (MVP/in scope/out of scope), Success metrics, Risks, Dependencies, Open questions.
   - Add links to research artifacts (user interviews, analytics, benchmarks).

6. Break PRD into User Stories & Acceptance Criteria (CTO/Engineer + Designer)
   - For each story include: title, short description, acceptance criteria (Given/When/Then), test data, UI mocks or link to Figma.

7. Create Implementation Checklist (CTO)
   - Technical tasks, API changes, data model modifications, migrations, feature flags, performance considerations.
   - Instrumentation checklist (events, properties, dashboard queries).

8. Design Hand-off (Designer)
   - Deliver high-fidelity screens and redlines, interactive prototype, component spec, accessibility notes.

9. Pre-release QA & Launch Plan (CTO + QA + Ops)
   - Test plan, staging verification steps, rollout plan (canary %, feature-flag strategy), rollback procedure, monitoring/alerts.

10. Release & Post-release Review (All)
    - Measure against success metrics after a defined evaluation window (e.g., 7/14/30 days).
    - Capture learnings, decide to iterate, scale, or kill.

## Templates

### PRD Template (brief)
- Title:
- Author & Date:
- Objective (1-2 sentences):
- Background & Problem:
- Target User / Personas:
- Proposed Solution (high level):
- In-scope (MVP):
- Out-of-scope:
- Acceptance Criteria / Success Metrics:
- Dependencies:
- Risks & Mitigations:
- Open Questions:

### User Story + Acceptance Criteria (format)
- Story: As a <user>, I want <action> so that <benefit>.
- Acceptance Criteria (Given/When/Then):
  - Given <context>
  - When <event>
  - Then <expected outcome>

### Release Checklist (example)
- [ ] Code reviewed and merged
- [ ] Feature flag implemented
- [ ] End-to-end tests passing
- [ ] Instrumentation events sent to analytics
- [ ] Monitoring dashboards created
- [ ] Rollout plan and rollback steps documented
- [ ] Stakeholders notified (list)

## Decision Rules

- Always write acceptance criteria before engineering starts.
- Prioritize experiments that reduce the biggest unknowns first.
- If an output from a role is incomplete, create a focused follow-up task for that role—do not re-implement their work yourself.
- Prefer incremental launches (feature flags/canary) to large-batch launches.
- Keep the scope of an MVP as small as possible to test the key hypothesis.

## Quality Checks

- PRD contains objective, success metric, and at least one acceptance criterion per major story.
- Every task assigned has a clear owner and an expected deliverable format (e.g., Figma link, PR link, dashboard link).
- No ambiguous terms—define key terms and acronyms on first use.
- Timeboxed experiments have explicit evaluation windows and pass/fail criteria.

## Handoff & Communication

- Store the PRD and related artifacts in /project/docs/product/<initiative-slug>/
- Use the task system to assign implementation tasks: include PRD link, acceptance criteria, and required assets in the task description.
- Weekly milestone updates: progress, blockers, metrics.

## Examples

- Small feature: landing page sign-up flow
  - Objective: validate interest from X segment
  - MVP: 1 landing page, signup form, thank-you page, GA funnel + signup event
  - Success: 5% conversion in first 1k visitors

- Larger feature: subscription billing
  - Objective: enable paid plans and measure conversion
  - MVP: pricing page, checkout flow, billing engine integration behind feature flag
  - Success: first 10 paying customers in 30 days

## Compatibility & Notes

- This skill is optimized for small cross-functional teams where the CEO/coordinator defines the what and why, and specialists (cto, designer, growth-lead, finance-ops) implement the how.
- Keep PRDs concise—prefer single-page summaries with links to supporting detail.

