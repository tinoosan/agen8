---
name: Project Management
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

## Quality checks

- [ ] Every delegated task has clear acceptance criteria.
- [ ] No task requires knowledge that the assigned role doesn't have access to.
- [ ] Dependencies between tasks are identified and sequenced correctly.
- [ ] Parallel work is maximized where dependencies allow.
- [ ] Progress is tracked — you know the status of every outstanding task.
- [ ] Blockers are identified and addressed promptly.
- [ ] Final output synthesizes all role contributions into a coherent deliverable.

## Anti-patterns to avoid

- **Doing the work yourself** — Coordinators who execute specialist tasks create bottlenecks and underuse their team.
- **Over-decomposition** — Don't split work so finely that coordination overhead exceeds execution time.
- **Fire and forget** — Delegating without tracking leads to missed deliverables and quality gaps.
- **Vague delegation** — "Look into X" produces inconsistent results. Be specific about what you need.
- **Serial execution bias** — Running everything sequentially when tasks could be parallelized wastes time.
- **Ignoring quality** — Accepting the first callback without reviewing against acceptance criteria leads to weak final output.

## When NOT to use this skill

- You are working alone with no roles to delegate to (use planning instead).
- The task is simple enough for a single role to handle without coordination.
- You are a specialist role executing assigned work (use planning for your own task breakdown).
