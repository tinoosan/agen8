---
name: agen8-coordination
description: Coordinate multi-agent work through Agen8 missions, key results, tasks, evidence, decisions, and reviews. Use when acting as an orchestrator, delegating to workers/reviewers, managing parallel agents, designing durable coordination workflows, recovering stalled work, or closing coordinated work without relying on chat memory.
---

# Agen8 Coordination

Use this skill with the `agen8` and `agen8-graph` skills. This skill governs coordination; Agen8 remains the durable state layer.

## Core Rule

Coordinate through Agen8 state, not chat memory. The coordinator creates structure, workers produce evidence, reviewers judge evidence, and decisions capture why the work moved.

## Roles

- **Coordinator:** owns mission/KR/task structure, assigns work, monitors liveness, resolves blockers, integrates approved outputs, updates KRs, and closes cleanly.
- **Worker:** owns one task/goal at a time, registers to Agen8, claims the task, works to acceptance criteria, submits evidence, and stops at defined blockers.
- **Reviewer:** independently checks submitted evidence against criteria, records review notes, requests changes or approves, and does not silently take over the worker's implementation.

One agent may play multiple roles only when the user asks for a simple workflow or no separate agents are available. For delegated work, prefer distinct worker and reviewer agents.

## Coordinator Loop

1. **Register and inspect.** Register to the existing Agen8 project. Read active missions, relevant KRs/tasks, decisions, and graph context before creating work.
2. **Shape outcomes.** Create or reuse a mission. Write KRs as observable outcomes, not chores.
3. **Create bounded tasks.** Each task needs an owner, acceptance criteria, required evidence, stop conditions, and review path.
4. **Delegate one goal per worker.** Give each worker exactly one task/goal. Tell the worker to register to Agen8 and claim the task before starting.
5. **Require heartbeat.** For long work, require periodic Agen8 updates or check-ins. Treat stale claimed tasks as coordination problems.
6. **Route evidence to review.** Workers submit artifacts, verification output, notes, and residual risks. Reviewers inspect that evidence and record approval or requested changes.
7. **Send changes back to the owner.** If review finds required changes, pass the task back to the original worker unless reassignment is explicit.
8. **Close deliberately.** Update KRs, log consequential decisions, clean temporary artifacts, close agents/threads if applicable, and record residual risks.

## Task Definition

Before delegation, write acceptance criteria that make review possible without rerunning the whole task. Include:

- scope and non-goals
- owned files, systems, or artifacts when applicable
- required checks or validation method
- evidence required on submit
- stop conditions and escalation triggers
- review owner or review route

Avoid vague tasks such as "improve X." Prefer tasks that can be judged: "produce a source-backed memo with citations," "reduce direct imports to zero," "verify the dashboard renders with screenshot evidence."

## Worker Instructions

Every worker prompt should include:

- the Agen8 project root or project id to register against
- the mission, KR, and task ids
- the worker's role/name
- the exact task goal and acceptance criteria
- required evidence and validation commands/checks
- stop conditions
- instruction not to create stray missions/KRs/tasks
- instruction not to claim more than one task at a time

Workers must submit before ending. A claimed task with no evidence reads as stuck.

## Reviewer Instructions

Reviewer prompts should include:

- the task id and acceptance criteria
- where to find the worker's evidence/artifacts
- what "approve," "retry," and "fail" mean for this task
- instruction to record review notes in Agen8
- instruction to send required changes back to the worker rather than quietly implementing them

Reviewers check criteria, evidence quality, scope control, and residual risk. They should state what the work proves and what it does not prove.

## Evidence

Evidence must match the task. Examples:

- source-backed research: cited sources, notes, contradiction handling, dated caveats
- data work: query, snapshot, metric definitions, validation checks, report/dashboard
- planning: requirements, acceptance criteria, tradeoff notes, open questions
- documentation: changed docs, rendered output if relevant, stale-doc scan notes
- code work: changed files, test output, review artifact, CI/PR status when the delivery profile requires it

Use Agen8 artifacts for project files and task attachments for external/generated evidence.

## Heartbeats and Stale Work

For work expected to take a while, create a heartbeat expectation in the worker prompt. The coordinator should periodically check task state, worker messages, and evidence. If the same blocker repeats, record it and either unblock, reassign, or pause the work.

Do not leave active claimed tasks unattended at the end of coordination.

## Temporary Work Areas

If a workflow uses temporary folders, name them predictably, keep them inside the project unless policy says otherwise, and clean them after integration or cancellation. If a temporary folder must remain, record why in Agen8.

For this generic skill, do not assume a specific folder name, branch strategy, PR system, or review platform. Those belong in delivery profiles or project-local instructions.

## Decisions

Log decisions when coordination policy changes, scope changes, review overturns earlier direction, blockers change the plan, or a tradeoff affects future work. Link decisions to the mission/KR/task they affect when possible.

## Closeout Checklist

Before ending coordinated work:

- all claimed tasks are submitted, reviewed, released, blocked, or explicitly handed off
- KRs reflect completed evidence
- decisions and graph links capture important relationships
- temporary work areas are cleaned or documented
- reviewers' residual risks are visible
- the final answer reports mission/task status and remaining risks
