package prompts

import "strings"

// sharedTaskRunnerBlock contains the four rules shared by autonomous, subagent, and team modes.
// Use when building mode prompts; do not wrap in an outer tag (the mode block provides that).
const sharedTaskRunnerBlock = `
	  <rule id="not_chat">You are running as an autonomous task runner. You are NOT in a chat. Do not ask the user follow-up questions unless you are truly blocked; make reasonable assumptions and proceed.</rule>
	  <rule id="honest_reporting">Honest reporting is mandatory. If the goal is not met, call final_answer with status="failed" and a concrete error; do NOT claim success.</rule>
	  <rule id="state_persistence">Persist critical context and intermediate results to /workspace files so progress survives context compaction and restarts.</rule>
	  <rule id="initiative">Be proactive and creative when needed: inspect the repo, run targeted tests, add small helper scripts, and iterate until the task is complete. Prefer simple, reliable solutions.</rule>`

// completionReportBullets is the Step 1 content shared by all reporting variants.
const completionReportBullets = `	    Step 1: Prepare a completion report (plain text)
	    - what you did (high level summary)
	    - where to look (key file paths, URLs, deliverables)
	    - next steps (tests/commands) if relevant`

// finalAnswerRequirement is the single source for final_answer params; used in all reporting variants.
const finalAnswerRequirement = `- IMPORTANT: final_answer parameters MUST include "status", "error", and "artifacts" (use empty string/empty array when not applicable). Never include /plan files in artifacts.`

// reportingBlock returns the <rule id="reporting">...</rule> content for task-runner modes.
// includeEmail: if true, include Step 2 (send email) and Step 3 (final_answer); if false, only Step 1 + Step 2 (call final_answer).
// forSubAgent: if true, Step 2 says "This returns your result to the parent agent"; otherwise "this ends the task".
func reportingBlock(includeEmail bool, forSubAgent bool) string {
	var b strings.Builder
	b.WriteString(`	  <rule id="reporting">
	    CRITICAL REQUIREMENT: You MUST complete these steps`)
	if includeEmail {
		b.WriteString(" IN ORDER")
	}
	b.WriteString(" before ending")
	if forSubAgent {
		b.WriteString(":")
	} else {
		b.WriteString(" the task:")
	}
	b.WriteString("\n\n")
	b.WriteString(completionReportBullets)
	b.WriteString("\n\n")

	if includeEmail {
		b.WriteString(`	    Step 2: Send the completion email (MANDATORY - DO NOT SKIP)
	    - To: The email address from GMAIL_USER environment variable
	    - Subject: "[Agen8] Task Complete: <task_goal>"
	    - Body: Include the completion report from Step 1
	    - Use the email tool: email(to, subject, body)
	    ⚠️  THE TASK IS NOT COMPLETE UNTIL THE EMAIL IS SENT ⚠️
	    Only skip the email if the email tool returns an error indicating it is not configured.

	    Step 3: Call final_answer with the completion report (this ends the task)
	    `)
		b.WriteString(finalAnswerRequirement)
	} else {
		b.WriteString(`	    Step 2: Call final_answer with the completion report
	    - This returns your result to the parent agent
	    `)
		b.WriteString(finalAnswerRequirement)
	}
	b.WriteString("\n	  </rule>")
	return b.String()
}
