---
name: agen8
description: Use when operating through Agen8 as the durable work-context layer — registering a session, anchoring work to missions/key results/tasks, logging decisions, reviewing, or resuming after context compaction. For reading or linking the work graph, see the agen8-graph skill.
---

# Agen8

Agen8 is the durable work-context layer behind the harness. The harness (Claude Code, Codex, …) is where you do the work; Agen8 records the structure — projects, members, missions, key results, tasks, decisions — so the work survives compaction, handoff, and time. Treat Agen8 records as the source of truth over compressed chat memory.

Call the Agen8 MCP tools directly: `project`, `mission`, `task`, `decision`, `graph_query`, `http`. Their input schemas already document every action and field — this skill is the operating doctrine, not a field reference.

## The Loop

1. **Register.** Call `project.register` early — prefer `project_root` for a local project. Set `display_name` to `Name (Role)` (e.g. `Atlas (Backend Engineer)`) when a role is given or inferable, otherwise a bare name. Keep the returned `memberId`/`projectId`; you act as that member.
2. **Inspect before choosing work.** Read the current state — active missions, their key results, open tasks, recent decisions, the roster — before creating or claiming anything. Use the **agen8-graph** skill to read the work graph. This is mandatory after compaction, handoff, or a "continue".
3. **Anchor.** Every task serves a key result (or, lacking one, a mission). Reuse the active mission/KR if it fits; create a mission, then a KR, when the goal is new durable work. KRs are observable outcomes, not chores or working methods.
4. **One task at a time.** Create or claim the task, then claim it before doing the work. Work from its acceptance criteria. Keep tasks small enough to submit with clear evidence; don't duplicate a task an active one already covers.
5. **Decide out loud.** Log consequential choices with `decision.log` — why a direction changed, why a tradeoff was accepted — linked to the task and mission/KR. (`decision` only logs; there is no question or escalation verb.)
6. **Submit.** Concise summary, artifacts, verification evidence, decision ids, remaining risks. Use `task` `attach` to upload an artifact file to the task.
7. **Review** when you own closing the task. Check every acceptance criterion. Make it a reflection, not a rubber stamp: state what the work proves, what it does *not*, and the residual risk. Record durable findings as a decision.

Update KR progress when a task materially moves the outcome.

## Hard Rules

- **Finish what you claim.** If you claim a task you must submit and review it before ending the turn. The user cannot tell from the Agen8 UI whether your session is still alive, so a claimed-but-abandoned task reads as stuck. Leave follow-ups as *pending* tasks, never as half-done claimed ones.
- **No stray tasks.** A task with no mission/KR anchor is acceptable only when no anchor is discoverable — and say so.
- **Don't invent** ids, project/member/task state, or decision history. If a read returns nothing, that is the answer.
- When a tool rejects a field, adapt to the schema and note the mismatch as workflow feedback.

## Notes Are For People

Decisions, summaries, review notes, and titles get read later by a human who has to act on them. Lead with the point, then the why. Plain words, short sentences, one idea each. Cut filler ("leverage", "utilize", "robust", "holistic", "delve"). Keep a technical term only when it carries real meaning. Test it: could a teammate skim the note and know what happened and what to do next?

## Operating Notes

- **Responses are lean.** Mutating actions (create / claim / submit / review, `kr_progress`, …) return an acknowledgement — ids and status, not the whole object. When you need the full description, acceptance criteria, or a KR's detail, fetch it with `get` / `kr_get`. Read mission/KR state with `mission` `progress`, `history`, `kr_history`.
- **Member-as-actor failures.** If a verb like `task` `claim` fails with "registered member_id is required", the session did not resolve to a member (for example, a token shared across sessions). Re-run `project.register` with an explicit `session_id` to bind this session, then retry.
- **Keep it legible.** Prefer member display names and clear titles over raw ids in anything a human reads.
