---
name: agen8
description: Use when working with Agen8, using Agen8 MCP tools, coordinating work through Agen8 projects/missions/key results/tasks/decisions/graph context, recovering after Codex context compaction, or operating as an agent with Agen8 as the durable work-context layer.
---

# Agen8

Use Agen8 as the durable work-context layer behind the harness. Codex, Claude Code, and other harnesses remain the work surfaces. Agen8 records projects, members, missions, key results, tasks, decisions, files, activity, credentials, HTTP actions, and graph context.

## Start Or Resume

1. Call `project.register` early for the current project.
   - Prefer `project_root` when working from a local project directory.
   - Set `display_name` as `Name (Role)` — a self-chosen name plus the working role — when the human gives a role or one is inferable from the request, for example `Atlas (Backend Engineer)` or `Iris (Frontend Reviewer)`. Use a bare `Name` only when the role is too ambiguous to infer. The name keeps the member recognizable in the graph; the role keeps the roster scannable.
   - Use the returned `projectId`, `memberId`, `channelId`, `url`, and `token` for later calls. Treat `memberType` as compatibility metadata, not as a permission model.
   - Do not invent a thread id. If the harness exposes native session metadata through MCP, Agen8 can bind it. If not, use explicit user-provided ids only.
2. After context compaction, thread resume, handoff, or a user says to continue, inspect Agen8 before continuing.
   - Prefer `graph_query` for project memory, then check active missions, KRs, open tasks, recent decisions, and the member roster.
   - Treat Agen8 records as the durable source of truth over compressed chat memory.
3. Identify the mission and key result before selecting or creating task work.
   - Reuse the active mission/KR if it fits the goal.
   - Create a mission first, then a KR, when the goal is new durable work and the MCP surface supports it.
   - Do not create standalone tasks for normal Agen8 work. A task without `mission_ref` or `key_result_ref` is only acceptable when no mission/KR tool or graph anchor is available, and the limitation must be stated.
4. Work one focused task at a time.
   - Create or claim the task, then claim before doing the work.
   - Work from its acceptance criteria.
   - Submit with concise artifacts and verification results.
   - Review when the task is ready to close and you are responsible for checking the submitted work.

## Tool Use

- `project`: register the harness session, inspect the current project, and manage members.
- `mission`: create or inspect missions and key results when exposed by the current Agen8 MCP surface.
- `task`: create, claim, submit, review, cancel, or reassign work.
- `decision`: log consequential choices and ask the user structured questions.
- `graph_query`: inspect nodes, search memory, and link decisions/tasks/KRs/missions so the work is understandable later.
- `http`: perform real-world HTTP actions through Agen8, including credential-backed calls when configured.

Call Agen8 MCP tools directly. If a wrapper or parallel call mangles the namespace, retry with a direct tool call and record the tool-surface issue if it affects work.

## Work Loop

1. Register into Agen8 and identify the current member id and display name.
2. Inspect graph/context, active missions, open KRs, open tasks, recent decisions, and relevant members before choosing work.
3. Anchor work to a mission and key result before creating tasks.
   - Reuse an active mission/KR when it matches the current goal.
   - Create a mission/KR when the current goal is a new durable objective and the tool surface exposes mission management.
   - If mission/KR tools are unavailable, attach tasks to the best available `mission_ref` or `key_result_ref` from graph/context; only create a standalone task when no graph anchor is discoverable.
4. Create or claim the Agen8 task for the slice being worked, linking `mission_ref` and `key_result_ref` whenever the schema allows it.
   - Keep tasks small enough to submit and review with clear evidence.
   - Avoid creating duplicate tasks when an active or in-review task already covers the slice.
5. Do the work using the harness and tools available for the current domain.
6. Verify the result in the most direct available way for the work being done.
7. Record important choices with `decision.log`, linked to the current task and mission/KR.
8. Use `graph_query` links when the relationship matters: task serves KR, decision made during task, decision informed by another node, task blocked by another node.
9. Submit task results with artifacts, verification evidence, decision ids, graph links, and remaining risks.
10. Review against every acceptance criterion when you are responsible for closing the task. Treat the review as a reflection point, not a rubber stamp: before approving, articulate what the work proves, what it does *not* prove, and any residual risk. Record durable findings with `decision.log` so the reflection becomes work memory rather than a transient pass/fail.

## Mission Discipline

- Every meaningful Agen8 workstream should have a mission with one or more KRs before task execution starts.
- KRs describe observable outcomes, not implementation chores or working methods.
- Tasks are execution slices that move a KR; they should inherit `mission_ref` and `key_result_ref`.
- Decisions capture why the direction changed, why an integration path was chosen, or why a tradeoff was accepted.
- Graph/context inspection is not optional after compaction or when joining ongoing work; it prevents stale chat memory from overriding durable project state.
- Keep the graph readable for the human. Prefer member display names and clear task/KR titles over raw ids in user-facing surfaces.

## Write Notes Plainly

Notes are for people. Decisions, task summaries, review notes, and KR/task titles get read later by a human who has to act on them. Write so the point lands on the first read.

- Lead with the point. Say what you did or decided, then why.
- Use plain words. If a shorter, common word works, use it. No one should need a thesaurus.
- Cut filler. Drop words like "leverage", "utilize", "robust", "holistic", and "delve" when a simpler word fits.
- Keep a technical term only when it carries real meaning, and define it once if it is not obvious.
- Short sentences. One idea each.
- Test it: could a teammate skim the note and know what happened and what to do next? If not, rewrite it.

Plain notes keep the work manageable. The human can review and steer without decoding the language.

## Workflow Defaults

- For a new objective, create or choose one mission and one KR first.
- For execution, create one small task, claim it, complete it, submit it, then review it before starting the next task.
- For exploratory work, create a task with acceptance criteria that include the decision or recommendation expected at the end.
- For user-facing work, verify the result from the user's point of view when that is possible.
- For tool-facing work, test through the same tool surface the agent or user is expected to use when that is possible.
- Update KR progress when a task materially moves the outcome.

## Boundaries

- Agen8 is the durable work-context layer. The active harness remains the work surface.
- Do not invent missing ids, project state, member state, task state, or decision history.
- When a tool contract rejects fields, adapt to the schema and note the mismatch as Agen8 workflow feedback.
- Do not create stray tasks. If no mission/KR exists, create those first or state why the tool surface made that impossible.

## Compaction Recovery Checklist

- Re-register with `project.register` if the current member/session is unclear.
- Inspect graph/work context when exposed by MCP.
- Identify the current mission and KR before selecting work.
- List current tasks and find any assigned or active task for this member under that mission/KR.
- Read recent decisions linked to the current task, mission/KR, or project.
- Continue only after reconciling Agen8 state with the summarized chat context.
