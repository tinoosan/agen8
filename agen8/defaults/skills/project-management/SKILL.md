---
name: project-management
description: Orchestrate multi-role teams by decomposing goals, delegating work, tracking progress, and resolving blockers.
---

# Instructions

Use this skill when you are a coordinator responsible for driving outcomes through other roles. Unlike planning (which is for breaking down your own technical work), project management is about orchestrating *other people's* work to deliver a collective result.

## When to use

- You are a coordinator with specialist roles available to delegate to.
- The goal requires contributions from multiple roles working in parallel or sequence.
- You need to track progress across several workstreams and synthesize results.
- Blockers need to be identified and resolved to keep the team moving.
- Stakeholders need status updates on multi-role efforts.

## Workflow

1. **Decompose the goal into delegatable tasks.** Break the objective into discrete work items that map to specific roles. Each task should be self-contained enough for a specialist to execute independently.

2. **Allocate roles.** Assign each task to the role best suited for it. Consider role expertise, current workload, and dependencies between tasks.

3. **Define acceptance criteria.** For every delegated task, specify:
   - What the deliverable looks like (format, content, level of detail)
   - How you will evaluate completeness and quality
   - Any constraints (time, scope, data sources)

4. **Set execution order.** Identify which tasks can run in parallel and which have dependencies. Prefer parallel execution when possible to maximize throughput.

5. **Delegate with context.** When creating tasks, include:
   - The specific goal and why it matters
   - Acceptance criteria
   - Relevant background information the role needs
   - Expected artifacts or outputs

6. **Track progress.** Monitor task completions and callbacks. Maintain a mental model of:
   - What is done, what is in progress, what is blocked
   - Whether outputs meet acceptance criteria
   - Whether the overall goal is still on track

7. **Resolve blockers.** When a role is stuck or produces insufficient output:
   - Diagnose whether the issue is unclear requirements, missing data, or wrong role assignment
   - Create targeted follow-up tasks with additional context
   - Reassign work to a different role if needed

8. **Manage milestones.** Group related tasks into milestones that represent meaningful progress checkpoints. Use milestones to:
   - Communicate progress to stakeholders
   - Make go/no-go decisions before investing in the next phase
   - Identify when the overall goal is met

9. **Communicate status.** Provide clear, concise updates that cover:
   - Progress against the goal
   - Completed deliverables
   - Active blockers and mitigation plans
   - Next steps and expected timeline

## Decision rules

- Never do specialist work yourself. Your value is in coordination, not execution.
- Prefer parallel delegation when tasks are independent — don't serialize unnecessarily.
- If a callback result is incomplete, create a focused follow-up task rather than redoing work.
- When multiple roles could handle a task, assign it to the most specialized one.
- Escalate to the user only when you lack authority or information to unblock the team.
- Keep task descriptions specific — vague delegation produces vague results.
- Match delegation granularity to complexity — don't micro-manage simple tasks or under-specify complex ones.

## Quality checks

- [ ] Every delegated task has clear acceptance criteria.
- [ ] No task requires knowledge that the assigned role doesn't have access to.
- [ ] Dependencies between tasks are identified and sequenced correctly.
- [ ] Parallel work is maximized where dependencies allow.
- [ ] Progress is tracked — you know the status of every outstanding task.
- [ ] Blockers are identified and addressed promptly.
- [ ] Final output synthesizes all role contributions into a coherent deliverable.

## Templates

### Delegation Brief
```markdown
## Task: <Clear, specific title>

**Assigned to**: <role-name>
**Priority**: High/Medium/Low
**Depends on**: <other task or "none">

### Goal
<What needs to be accomplished and why it matters to the overall objective>

### Acceptance Criteria
- [ ] <Specific, testable criterion>
- [ ] <Specific, testable criterion>
- [ ] <Specific, testable criterion>

### Context
<Background info the role needs to do this well>

### Constraints
- <Time, scope, format, or source constraints>

### Expected Output
<What the deliverable should look like>
```

### Milestone Tracker
```markdown
# Project: <Name>

## Milestone 1: <Name>
**Status**: Complete / In Progress / Not Started
**Target**: <Date or condition>

| Task | Owner | Status | Notes |
|------|-------|--------|-------|
| <task> | <role> | Done/WIP/Blocked | <context> |

## Milestone 2: <Name>
...

## Blockers
| Blocker | Impact | Owner | Resolution |
|---------|--------|-------|------------|
| <what> | <which tasks affected> | <who can fix> | <plan> |
```

### Status Update
```markdown
# Status: <Project Name> — <Date>

## Progress
- **Completed**: <what's done since last update>
- **In Progress**: <what's being worked on>
- **Blocked**: <what's stuck and why>

## Key Decisions
- <Decision made and rationale>

## Next Steps
- <What happens next, assigned to whom>

## Risk Watch
- <Emerging risks or concerns>
```

## Example

**Goal**: "Research and recommend a pricing strategy for our new product"

**Decomposition**:
```
Milestone 1: Research (parallel)
├── Task 1: Competitive pricing analysis → market-researcher
├── Task 2: Cost structure breakdown → data-analyst
└── Task 3: Customer willingness-to-pay research → market-researcher

Milestone 2: Analysis (after M1)
├── Task 4: Model 3 pricing scenarios → strategy-analyst
└── Task 5: Sensitivity analysis on key variables → data-analyst

Milestone 3: Deliverable (after M2)
└── Task 6: Pricing recommendation memo → deliverable-writer
```

**Delegation**: Tasks 1-3 run in parallel. Tasks 4-5 depend on M1 completion. Task 6 depends on M2. Total: 3 phases, 6 tasks, 3 roles.

## Advanced Techniques

### Dependency management
Map task dependencies to find the critical path:
```
Task A (2 days) → Task C (1 day) → Task E (2 days) = 5 days
Task B (3 days) → Task D (1 day) ─────────────────→ = 4 days
Critical path: A → C → E (5 days minimum)
Task B and D can run in parallel without affecting timeline.
```

### Progressive delegation
For uncertain work, delegate in waves:
1. **Wave 1**: Research and scoping tasks (low commitment)
2. **Review**: Assess findings, adjust plan if needed
3. **Wave 2**: Execution tasks based on Wave 1 results
4. **Review**: Quality check against acceptance criteria
5. **Wave 3**: Polish and synthesis

This prevents wasted work when early findings change the direction.

### Callback quality assessment
When reviewing specialist callbacks, check:
- **Completeness**: Does it address all acceptance criteria?
- **Evidence**: Are claims supported with data or sources?
- **Actionability**: Can the output be used directly or does it need refinement?
- **Consistency**: Does it align with outputs from other roles?

If quality is insufficient, create a focused follow-up specifying exactly what's missing — not a vague "do better."

### Resource balancing
When one role is overloaded:
- Resequence: Move non-urgent tasks to later waves
- Redistribute: Assign tasks to roles with capacity (if capable)
- Decompose: Break large tasks into smaller pieces that multiple roles can work on

## Anti-patterns to avoid

- **Doing the work yourself** — Coordinators who execute specialist tasks create bottlenecks and underuse their team.
- **Over-decomposition** — Don't split work so finely that coordination overhead exceeds execution time.
- **Fire and forget** — Delegating without tracking leads to missed deliverables and quality gaps.
- **Vague delegation** — "Look into X" produces inconsistent results. Be specific about what you need.
- **Serial execution bias** — Running everything sequentially when tasks could be parallelized wastes time.
- **Ignoring quality** — Accepting the first callback without reviewing against acceptance criteria leads to weak final output.
- **Scope creep via delegation** — Adding "while you're at it" tasks to every delegation slowly expands scope beyond the original goal.

## When NOT to use this skill

- You are working alone with no roles to delegate to (use planning instead).
- The task is simple enough for a single role to handle without coordination.
- You are a specialist role executing assigned work (use planning for your own task breakdown).
