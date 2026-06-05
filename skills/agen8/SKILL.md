---
name: agen8
description: Use when working with Agen8, using Agen8 MCP tools, coordinating work through Agen8 projects/missions/key results/tasks/decisions/graph context, recovering after Codex context compaction, or operating as an agent with Agen8 as the durable work-context layer.
---

# Agen8

Use Agen8 as the durable work-context layer behind the harness. Codex, Claude Code, and other harnesses remain the work surfaces. Agen8 records projects, members, missions, key results, tasks, decisions, files, activity, credentials, HTTP actions, and graph context.

## Start Or Resume

1. Call `project.register` early for the current project.
   - Prefer `project_root` when working from a local repo.
   - Set `display_name` when the human gives a role or when a clearer graph label helps, for example `Codex backend engineer`, `Claude frontend reviewer`, or `Release coordinator`.
   - Use the returned `projectId`, `memberId`, `memberType`, `channelId`, `url`, and `token` for later calls.
   - Do not invent a thread id. If the harness exposes native session metadata through MCP, Agen8 can bind it. If not, use explicit user-provided ids only.
2. After context compaction, thread resume, handoff, or a user says to continue, inspect Agen8 before continuing.
   - Prefer `graph_query` for project memory, then check active missions, KRs, open tasks, recent decisions, and the member roster.
   - Treat Agen8 records as the durable source of truth over compressed chat memory.
3. Identify the mission and key result before selecting or creating task work.
   - Reuse the active mission/KR if it fits the goal.
   - Create a mission first, then a KR, when the goal is new durable work and the MCP surface supports it.
   - Do not create standalone tasks for normal Agen8 work. A task without `mission_ref` or `key_result_ref` is only acceptable when no mission/KR tool or graph anchor is available, and the limitation must be stated.
4. Work one focused task at a time.
   - Create or claim the task, then claim before touching the repo.
   - Work from its acceptance criteria.
   - Submit with concise artifacts and test/UAT results.
   - Review only when acting as the coordinator and the task is ready to close.

## Tool Use

- `project`: register the harness session, inspect the current project, and manage members.
- `mission`: create or inspect missions and key results when exposed by the current Agen8 MCP surface.
- `task`: create, claim, submit, review, cancel, or reassign work.
- `decision`: log consequential choices and ask the user structured questions.
- `graph_query`: inspect nodes, search memory, and link decisions/tasks/KRs/missions so the work is understandable later.
- `http`: perform real-world HTTP actions through Agen8, including credential-backed calls when configured.

Call Agen8 MCP tools directly. If a wrapper or parallel call mangles the namespace, retry with a direct tool call and record the tool-surface issue if it affects work.

## Engineering Loop

1. Register into Agen8 and identify the current member role.
2. Inspect graph/context, active missions, open KRs, open tasks, recent decisions, and relevant members before choosing work.
3. Anchor work to a mission and key result before creating tasks.
   - Reuse an active mission/KR when it matches the current goal.
   - Create a mission/KR when the current goal is a new durable objective and the tool surface exposes mission management.
   - If mission/KR tools are unavailable, attach tasks to the best available `mission_ref` or `key_result_ref` from graph/context; only create a standalone task when no graph anchor is discoverable.
4. Create or claim the Agen8 task for the slice being worked, linking `mission_ref` and `key_result_ref` whenever the schema allows it.
   - Keep tasks small enough to submit and review with clear evidence.
   - Avoid creating duplicate tasks when an active or in-review task already covers the slice.
5. Implement in the repo with normal engineering discipline: scoped diffs, focused tests, and no unrelated cleanup.
6. Use the in-app browser for UI verification when the change affects pages, navigation, local app behavior, or inspection views.
7. Prefer real MCP/UAT verification for MCP tools and user-visible flows. Unit tests are useful but do not replace proving that the actual agent-facing surface works.
8. Record important architecture, product, UX, routing, security, or workflow choices with `decision.log`, linked to the current task and mission/KR.
9. Use `graph_query` links when the relationship matters: task serves KR, decision made during task, decision informed by another node, task blocked by another node.
10. Submit task results with artifacts, verification run, decision ids, graph links, and remaining risks.
11. If acting as coordinator, review against every acceptance criterion and close or retry the task.

## Mission Discipline

- Every meaningful Agen8 workstream should have a mission with one or more KRs before task execution starts.
- KRs describe observable outcomes, not implementation chores. Dogfooding is not a KR; the result of dogfooding is.
- Tasks are execution slices that move a KR; they should inherit `mission_ref` and `key_result_ref`.
- Decisions capture why the direction changed, why an integration path was chosen, or why a tradeoff was accepted.
- Graph/context inspection is not optional after compaction or when joining ongoing work; it prevents stale chat memory from overriding durable project state.
- Keep the graph readable for the human. Prefer member display names and clear task/KR titles over raw ids in user-facing surfaces.

## Workflow Defaults

- For a new objective, create or choose one mission and one KR first.
- For execution, create one small task, claim it, complete it, submit it, then review it before starting the next task.
- For exploratory work, create a task with acceptance criteria that include the decision or recommendation expected at the end.
- For UI/product refinement, verify in the browser and think like a user: raw ids, unclear labels, broken graph relationships, and empty states are product bugs.
- For MCP/server changes, test through the actual MCP tools whenever possible. Do not substitute curl or private service calls for agent-facing UAT unless the MCP layer is unavailable; if unavailable, say so.
- When building Agen8 itself, dogfooding is the verification method: use Agen8 while improving it, but keep the durable records focused on product outcomes.
- Update KR progress when a task materially moves the outcome.

## Boundaries

- Do not rebuild Agen8 as a replacement chat UI.
- Do not recreate Codex or Claude session/thread management.
- Treat native harness thread delivery as optional integration. Agen8 owns durable context records; harnesses own their own work surfaces.
- Keep MCP tool names and capability surfaces stable unless a task explicitly asks to change them.
- When a tool contract rejects fields, adapt to the schema and note the mismatch as Agen8 workflow feedback instead of forcing broad changes.
- Do not revive removed surfaces such as message/operator/plan/channel code unless a new mission explicitly reinstates them.
- Do not create stray tasks. If no mission/KR exists, create those first or state why the tool surface made that impossible.

## Compaction Recovery Checklist

- Re-register with `project.register` if the current member/session is unclear.
- Inspect graph/work context when exposed by MCP.
- Identify the current mission and KR before selecting work.
- List current tasks and find any assigned or active task for this member under that mission/KR.
- Read recent decisions linked to the current task, mission/KR, or project.
- Continue only after reconciling Agen8 state with the summarized chat context.
