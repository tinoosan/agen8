You are starting a new feature development workflow. Follow this process exactly. The full workflow reference is at `docs/feature-development-workflow.md`.

## Phase 1: Discussion (Chat Only — NO CODE)

You are now in discussion mode. Do NOT create any files or write any code until the user explicitly says to move to Phase 2.

1. Ask the user to describe the feature idea
2. Use AskUserQuestion to explore decisions systematically:
   - User stories: agent perspective AND operator perspective
   - Data model: types, fields, storage
   - UI/UX: where does this show up, what surfaces (transcript cards, board, dedicated page)
   - Orchestration: how does this interact with agent types (coordinator/worker/reviewer), review pipeline, hub-and-spoke
   - Composition: how does this work with existing primitives (tasks, messages, memory, plans, events, blocking, notifications)
   - Edge cases: replicas, error handling, circuit breakers, what happens when things fail
   - Observability: events, notifications (with throttling), operator visibility
3. Challenge assumptions — validate against the existing orchestration model before accepting a design
4. Keep iterating until the user is satisfied. Do NOT rush to Phase 2.

## Phase 2: PRD

When the user says to write the PRD:

1. Write the PRD to `docs/prd/{feature-name}.md` directly on `dev` — PRDs are documentation, they don't need a feature branch or CI process
2. Commit and push to `dev` directly
3. Iterate in commits — the user will push back on gaps
4. Validate against the checklist:
   - Agent type boundaries (coordinator vs worker tools, hub-and-spoke)
   - Review pipeline integration
   - OCP/DDD patterns for extensibility
   - Notification integration (if applicable)
   - Operator UX (ongoing visibility, not just configuration)
   - Cross-surface navigation (contextual info + deep links)

## Phase 3: Agent Planning

When the PRD is complete:

1. Analyze file ownership conflicts across implementation phases
2. Split into agents (typically 3): backend, frontend, bounded context
3. NEVER split backend between two agents — merge conflicts kill velocity
4. Add "Agent Work Assignments" section to the PRD with: branching model (agents branch from feature branch, PR back into it), merge order, per-agent ownership, acceptance criteria, do-not-touch lists
5. Agents work in git worktrees under .git

## Phase 4: GitHub Issues

1. Create an epic issue linking all sub-issues
2. Create PR-level issues (one per agent PR)
3. Create feature-level issues (one per F-number) — each with: agent name, files, acceptance criteria, dependencies
4. Label with `agent:{name}`, `phase-N`, feature label

## Phase 5: Execution

Provide agent initialization prompts:

```
You are {AgentName}, the {role} agent for the {feature} feature. Your work is defined
in the PRD at docs/prd/{feature}.md — read Section 13 "Agent Work Assignments" →
"Agent: {AgentName}".

Work in a git worktree under .git: create your worktree from {feature-branch} with
branch {agent-branch}. Your issues are: {issue numbers}. Work through them in order.
Push your branch and submit PRs targeting {feature-branch}. All code must be
self-documenting with clear comments. {Do-not-touch constraints}. Epic: #{epic}.
```

Begin Phase 1 now. Ask the user what feature they want to build.
