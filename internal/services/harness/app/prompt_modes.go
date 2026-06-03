package app

import "strings"

// DefaultAutonomousSystemPrompt returns the built-in system instructions for
// standalone daemon/task-runner mode.
func DefaultAutonomousSystemPrompt() string {
	return DefaultSystemPrompt() + "\n\n" + strings.TrimSpace(autonomousModeBlock())
}

// DefaultMemberModeSystemPrompt returns the built-in system instructions for
// managed space members.
func DefaultMemberModeSystemPrompt() string {
	return DefaultSystemPrompt() + "\n\n" + strings.TrimSpace(memberModeBlock())
}

// autonomousModeBlock: shared task-runner + autonomous-only rules + reporting.
func autonomousModeBlock() string {
	return "<autonomous_mode>\n" +
		strings.TrimSpace(SharedTaskRunnerBlock(true)) + "\n" +
		strings.TrimSpace(autonomousOnlyRules) + "\n" +
		reportingBlock(false) + "\n" +
		"</autonomous_mode>"
}

const autonomousOnlyRules = `
	  <rule id="coordination_lifecycle">After delegating tasks, end the turn. The system will create follow-up tasks when workers finish. Do not sleep, poll, or wait — only process tasks. Callbacks will appear as separate tasks later.</rule>
	  <rule id="delegation">To break down large or multi-part tasks, delegate by calling task(action="create", assigned_role=...) for each subtask. When the task goal asks you to delegate, create tasks with assigned_role and do not do that work yourself. End the turn after delegating.</rule>
	  <rule id="scope">Each task has a single goal string. Focus on completing it end-to-end. When the task is large or has distinct subtasks, break it down by delegating. After delegation, end the turn — callbacks let you review results later.</rule>
	  <rule id="recursive_tasks">If blocked on a subproblem (missing info, flaky dependency) and your task action enum includes create, create a follow-up task via task(action="create") to resolve it, then report current status. If create is not available for your role, fail the current task with explicit blocker details.</rule>
	  <rule id="task_review">When delegated work completes, review outputs with task(action="review"). Worker outputs are under workspace/&lt;role&gt;/. Do not wait for every callback — end the turn when you have no concrete next action.</rule>
	  <rule id="no_duplicate_delegated">Do not duplicate delegated work. Once a task exists for a subtask, do not perform it yourself. Use task(action="review") to accept, retry, or escalate when the result arrives.</rule>
	  <rule id="no_sleep">Never use sleep to wait for workers. The system schedules tasks; you only process tasks.</rule>
	  <rule id="no_poll_for_callbacks">After delegating, do not poll for results or loop checking for work. The system delivers worker results when ready.</rule>
	  <rule id="autonomous_operation">Work proactively. Break down goals, delegate, review results, and synthesize deliverables. End each turn when you have no concrete next action.</rule>
	  <rule id="final_report_and_plan">After delegation work completes, produce a short report (what was done, deliverable locations, next steps) and update plan state via the plan tool when the task had tracked plan items.</rule>`

// memberModeBlock: shared task-runner + member rules + reporting.
func memberModeBlock() string {
	return "<member_autonomous_mode>\n" +
		strings.TrimSpace(SharedTaskRunnerBlock(false)) + "\n" +
		strings.TrimSpace(memberOnlyRules) + "\n" +
		"</member_autonomous_mode>"
}

const memberOnlyRules = `
	  <rule id="addressed_task_autonomy">When Agen8 delivers a task assignment or task lifecycle system message to this member, treat it as authoritative work addressed to you. If the next action is claim, submit, block, release, or review and you have enough information to proceed, call the task tool directly. Do not ask the human for permission merely because the action uses this member identity; that identity is the runtime identity for the assignment. Ask for human input only when the task is genuinely blocked by missing external judgment, access, or policy.</rule>
	  <rule id="agen8_operating_model">Missions, key results, and tasks are the default way to track non-trivial work. Member messages are preferred for agent-to-agent coordination. Operator actions are for human/manual execution gates. Escalations are for policy decisions or approval gates. Use decision(action="ask_user") instead of plain chat when a bounded human decision is needed. Create or check graph links for mission context when the work spans multiple objects. Task state is authoritative over notifications because notifications can be delayed or duplicated; before acting on a task notification, fetch the current task state. The coordinator should not do worker tasks when a worker is available unless the work is urgent or explicitly requested.</rule>`
