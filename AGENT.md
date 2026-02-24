# Agent guidance

The role of this file is to describe common mistakes and confusion points that agents might encounter as they work this project. If you ever encounter something in the project that surprises you, please alert the developer working with you and indicate that this is the case in the AgentMD file to prevent future agents from having the same issue.

We are developing; there is no need for backward compatibility. We just delete the database and start afresh.

## Session learnings (team-only reviewer pipeline)

1. Reviewer -> coordinator handoff is mandatory.
   - When a `team.batch.callback` review completes successfully, the system must create exactly one coordinator-facing handoff task (`review-handoff-<batchTaskId>`).
   - Do not rely on a single happy-path tool flow. Keep an idempotent fallback so handoff is still created if the reviewer completed the batch task through normal completion flow.

2. Never create callback-on-callback loops.
   - `review.handoff` is terminal for callback generation.
   - Completing a `review.handoff` task must NOT generate a new `team.callback`/`subagent.callback`.
   - Treat `review.handoff` as a callback source in callback guards.

3. Reviewer-facing inbox visibility must stay batch-only.
   - `team.callback` and `subagent.callback` are internal staging records only.
   - Reviewer/user inbox surfaces must only show synthetic batch callbacks (`team.batch.callback`, `subagent.batch.callback`).
   - Apply this in centralized listing/filter paths (not just one UI path).

4. Handoff routing must target coordinator runtime visibility.
   - For team handoff tasks, resolve to coordinator role and coordinator run context so coordinator run-scoped views can see the task.
   - Include review artifact pointers in handoff metadata/inputs (`reviewSummaryPath` and artifact paths) so coordinator can act immediately.

5. Dedicated reviewer lane is review-only.
   - In teams with dedicated reviewer enabled, normal work delegation to reviewer should be blocked.
   - Reviewer should receive review batch tasks/handoff workflow, not specialist execution tasks.
