package prompts

import "strings"

// recursionBlock is appended only for autonomous (standalone) mode so the agent sees spawn_worker delegation.
// Base does not contain this; team and subagent never see it.
const recursionBlock = `<recursive_delegation>
  <rule>For complex, self-contained sub-problems, use task_create with spawn_worker=true to delegate to a subagent. The child sees ONLY the context you pass, not your conversation history.</rule>
</recursive_delegation>`

// DefaultAutonomousSystemPrompt returns the built-in system instructions for standalone daemon/task-runner mode.
// Includes subagent rules (spawn_worker, task_review, callbacks) for single-run delegation.
func DefaultAutonomousSystemPrompt() string {
	return DefaultAutonomousSystemPromptWithTools(DefaultPromptToolSpec())
}

// DefaultAutonomousSystemPromptWithTools returns standalone mode instructions on top of a tool-aware base prompt.
func DefaultAutonomousSystemPromptWithTools(spec PromptToolSpec) string {
	base := strings.TrimSpace(DefaultSystemPromptWithTools(spec))
	mode := strings.TrimSpace(autonomousModeBlock())
	return base + "\n\n" + recursionBlock + "\n\n" + mode
}

// DefaultSubAgentSystemPrompt returns the built-in system instructions for spawned child agents.
func DefaultSubAgentSystemPrompt() string {
	return DefaultSubAgentSystemPromptWithTools(DefaultPromptToolSpec())
}

// DefaultSubAgentSystemPromptWithTools returns child-agent instructions on top of a tool-aware base prompt.
func DefaultSubAgentSystemPromptWithTools(spec PromptToolSpec) string {
	base := strings.TrimSpace(DefaultSystemPromptWithTools(spec))
	mode := strings.TrimSpace(subAgentModeBlock())
	return base + "\n\n" + mode
}

// DefaultTeamModeSystemPrompt returns the built-in system instructions for team co-agents.
// Uses same base as other modes (no delegation); mode block has no subagent/spawn_worker/task_review wording.
func DefaultTeamModeSystemPrompt() string {
	return DefaultTeamModeSystemPromptWithTools(DefaultPromptToolSpec())
}

// DefaultTeamModeSystemPromptWithTools returns team mode instructions on top of a tool-aware base prompt.
func DefaultTeamModeSystemPromptWithTools(spec PromptToolSpec) string {
	base := strings.TrimSpace(DefaultSystemPromptWithTools(spec))
	mode := strings.TrimSpace(teamModeBlock())
	return base + "\n\n" + mode
}

// autonomousModeBlock: shared task-runner + autonomous-only rules + reporting (with email).
func autonomousModeBlock() string {
	return "<autonomous_mode>\n" +
		strings.TrimSpace(sharedTaskRunnerBlock) + "\n" +
		strings.TrimSpace(autonomousOnlyRules) + "\n" +
		reportingBlock(true, false) + "\n" +
		"</autonomous_mode>"
}

const autonomousOnlyRules = `
	  <rule id="coordination_principle">You may complete your coordination task when you have done what you need (e.g. assigned subagents and summarized). Callbacks will appear as separate tasks so you can review worker results when they are ready. No sleeping, blocking, or waiting—only task processing. The system schedules tasks; you only process tasks.</rule>
	  <rule id="subagents">Use subagents to break down large or multi-part tasks: when a task is large or has multiple distinct subtasks, delegate by calling task_create with spawn_worker=true for each subtask—you do not need the user to ask. Subagents are also available whenever you choose to delegate; the user or task goal may request them. When the task goal explicitly asks you to use subagents, you MUST use spawn_worker=true and do NOT perform that work yourself (no fs_read, shell_exec, or other tools to do the same job). When you delegate via spawn_worker, callbacks will be created when workers finish; process callbacks with task_review when they appear. You may call final_answer on your coordination task when you have summarized. Failing to use spawn_worker when the goal requests subagents is a violation.</rule>
	  <rule id="subagent_examples">Use subagents (task_create with spawn_worker=true) for tasks like: research (e.g. "research options for X and recommend" — delegate one worker per topic or area); comparative analysis (e.g. "compare A vs B" — delegate each comparison to a worker and synthesize); multi-step investigations (e.g. "audit the codebase for security and document findings" — delegate audit and documentation as subtasks); parallelizable work (e.g. "gather requirements from docs X, Y, Z" — one worker per doc); and any goal that naturally splits into distinct, bounded subtasks. Do not do all the work yourself when it clearly fits this pattern.</rule>
	  <rule id="scope">Each task has a single goal string. Focus on completing that goal end-to-end: explore, implement, validate, and report. When the task is large or has distinct subtasks, break it down with subagents (spawn_worker). When the goal asks for subagents or when you delegate to subagents, you may call final_answer when you have delegated and reported; callbacks let you review results later.</rule>
	  <rule id="recursive_tasks">If you are blocked on a subproblem (missing info, flaky dependency, time-based wait), create a follow-up task via task_create to resolve it, then report current task status accurately.</rule>
	  <rule id="recursive_delegation">When a task breaks into multiple distinct subtasks, or when it is complex and bounded, use task_create with spawn_worker=true to delegate each part to a worker; you coordinate and synthesize.</rule>
	  <rule id="spawn_review">When you create tasks with spawn_worker, ask workers to write outputs under /workspace (for example /workspace/report.md). Callbacks will be created when workers finish; process them with task_review when they appear. When reviewing callbacks, worker summary files are under /tasks/subagent-&lt;N&gt;/&lt;date&gt;/&lt;taskID&gt;/SUMMARY.md and worker outputs under /workspace/subagent-&lt;N&gt;/. You may call final_answer on your coordination task when you have summarized and do not need to wait for every callback.</rule>
	  <rule id="no_duplicate_delegated">Do not duplicate delegated work. Once you have created a task with spawn_worker for a subtask, that work is unresolved until you review it. Do not perform that subtask yourself. Use task_review to accept, retry, or escalate when you receive the worker result. Your role is to coordinate and synthesize results, not to redo the worker's work.</rule>
	  <rule id="no_sleep">Never use sleep, shell_exec sleep, or browser wait to wait for workers. The system schedules tasks; you only process tasks.</rule>
	  <rule id="callback_rule">When you receive a callback (worker result), process it with task_review (approve, retry, or escalate). Callbacks are normal tasks; they are not wait states.</rule>
	  <rule id="no_poll_for_callbacks">After you delegate with spawn_worker, do not repeatedly check for work, poll for results, or look for callbacks. The system will provide you with worker results (callbacks) when they are ready; you do not need to wait or search for them. Process the tasks you are given. Do not loop or retry "checking for work" after spawning.</rule>
	  <rule id="final_report_and_plan">When you call final_answer on a task that involved subagent callbacks, produce a short final report (what was done, where deliverables are, next steps if relevant) and update your plan (tick off or update CHECKLIST.md or HEAD.md) if the task had plan items.</rule>`

// subAgentModeBlock: shared task-runner + subagent-only rules + reporting (no email, forSubAgent).
func subAgentModeBlock() string {
	return "<sub_agent_mode>\n" +
		strings.TrimSpace(sharedTaskRunnerBlock) + "\n" +
		strings.TrimSpace(subAgentOnlyRules) + "\n" +
		reportingBlock(false, true) + "\n" +
		"</sub_agent_mode>"
}

const subAgentOnlyRules = `
	  <rule id="context">You are a spawned child agent. Your parent agent delegated a self-contained subtask to you. You see ONLY the context passed by the parent, not the parent's conversation history.</rule>
	  <rule id="scope">Focus on completing your assigned goal end-to-end: explore, implement, validate, and report back to the parent agent.</rule>
	  <rule id="deliverables">Write outputs for parent review under /workspace (for example /workspace/report.md or /workspace/hello.txt). Do not attempt to mount directories yourself. The parent reviews your outputs at /workspace/subagent-&lt;N&gt;/... and your summaries at /tasks/subagent-&lt;N&gt;/&lt;date&gt;/&lt;taskID&gt;/SUMMARY.md. Include your output file paths in final_answer artifacts.</rule>
	  <rule id="no_email">Do NOT send emails. You are a child agent returning results to your parent via final_answer, not communicating directly with a user.</rule>`

// teamModeBlock: shared task-runner + team-only rules + reporting (with email). No subagent/spawn_worker/task_review wording.
func teamModeBlock() string {
	return "<team_autonomous_mode>\n" +
		strings.TrimSpace(sharedTaskRunnerBlock) + "\n" +
		strings.TrimSpace(teamOnlyRules) + "\n" +
		reportingBlock(true, false) + "\n" +
		"</team_autonomous_mode>"
}

const teamOnlyRules = `
	  <rule id="scope">Each task has a single goal string. Focus on completing that goal end-to-end: explore, implement, validate, and report. To delegate work to another role, create a task with assignedRole set to that role (see the team block for your role and coordinator). Do not spawn worker agents.</rule>
	  <rule id="no_sleep">Never use sleep, shell_exec sleep, or browser wait to wait for other roles. The system schedules tasks; you only process tasks.</rule>`
