# Subagent feature verification

How to check that subagent **review gate**, **cleanup**, and **artifact paths** work as expected. This aligns with the [Execution model (PRD)](execution-model.md): sub-agent work is not complete until the parent approves; cleanup happens after approval (or final failure / escalation resolution) and must preserve audit history and indexed artifacts. The execution model uses state-based coordination: delegation creates dependencies, callbacks resolve them, completion requires zero unresolved dependencies; the system schedules tasks and agents never block.

## What to verify

1. **Review gate** – When a subagent completes a task, the parent gets a callback task to review the result (approve / retry / escalate). The callback refers to the subagent’s task and includes artifact paths the parent can read.
2. **Cleanup** – After the subagent finishes its task and after the parent **approves** the review (completes the callback successfully), the subagent run is stopped and removed from the active workers list; run status is set to Succeeded. Run records and artifacts are **preserved** (only execution context is removed).
3. **Artifact paths** – The callback task includes `subagentArtifactsDir` (`/workspace/subagents/<runId>`) and an `artifacts` list with paths under that dir so the main agent can open the subagent’s outputs (shared workspace & attribution per PRD §9). The parent locates deliverables using the **callback task goal** (which explicitly includes the deliverable directory and artifact paths to review); the same paths are available in the task's `Inputs` for tooling.
4. **Subagents list** – The Subagents tab shows only **active** (running) subagents; completed ones are summarized as “No active subagents. (N completed.)” and are not deleted from the store (audit history kept per PRD §7).

## Manual verification steps

1. **Start the daemon and TUI**
   - Run the workbench daemon and connect the TUI (e.g. session picker → select a run, or use the run that has spawn_worker available).

2. **Spawn a subagent**
   - In the main agent run, trigger a task that uses `spawn_worker` (or the host tool that creates a child run) so a subagent run is created and appears in the Subagents tab.

3. **Let the subagent complete**
   - Wait until the subagent finishes its single task. In logs you should see `subagent.finished` and the run status updated to Succeeded. The Subagents tab should then show “No active subagents. (1 completed.)” (or similar) instead of the run in the list.

4. **Review callback**
   - In the parent run, a new task should appear (callback for the subagent’s task). Open that task and confirm:
     - Goal/text refers to the subagent’s work (e.g. “Review … result from spawned worker for task …”).
     - Inputs include `subagentArtifactsDir` (e.g. `/workspace/subagents/<childRunId>`) and `artifacts` with paths under that dir (e.g. `/workspace/subagents/<id>/tasks/.../SUMMARY.md`).

5. **Complete the review**
   - Let the main agent complete the callback task successfully (e.g. mark the result as accepted). After that:
     - The subagent run should remain in a terminal state (Succeeded); if it was still in the supervisor’s map, it will have been stopped and removed.
     - The Subagents tab should still show only active subagents (or “No active subagents. (N completed.)” if none are running).

6. **Artifacts on disk**
   - Under the parent run’s workspace dir, check `workspace/subagents/<childRunId>/` and confirm the subagent’s task outputs (e.g. `tasks/.../SUMMARY.md`) are there and match the paths in the callback’s `artifacts`.

## Run records: keep vs delete

Per the [Execution model](execution-model.md) §7 (Cleanup behaviour): cleanup must **remove execution context** but **preserve indexed artifacts** and **preserve audit history**. So:

- **Subagent run records are not deleted** when cleanup runs. They are marked Succeeded (or Failed/Canceled) and stay in the store. Only the **worker** (goroutine/session) is stopped and removed from the supervisor.
- The **Subagents tab** hides completed runs from the list and shows “N completed” so the list is not cluttered; the data is still available via store/API if needed.

## Quick automated checks

- Build and tests:
  - `go build ./...`
  - `go test ./internal/tui/... -run TestRenderDashboardSubagentsTab` (covers “active only” list and “N completed” message).
- The cleanup notifier is exercised whenever a parent run completes a task whose `Metadata["source"]` is `subagent.callback` and result status is Succeeded; integration tests that drive that flow would cover it end-to-end.
