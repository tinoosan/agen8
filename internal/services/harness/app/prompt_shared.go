package app

import "strings"

// SharedTaskRunnerBlock returns the rules shared by autonomous and member modes.
func SharedTaskRunnerBlock(_ bool) string {
	var b strings.Builder
	b.WriteString(`
	  <rule id="honest_reporting">Honest reporting is mandatory. Never claim success if you did not do or delegate the work.</rule>
	  <rule id="state_persistence">Persist critical context and intermediate results to workspace/... files so progress survives context compaction and restarts.</rule>
	  <rule id="final_response_contract">End your turn with a direct assistant response. If relevant, mention key file paths or deliverables directly in the response text. Never mention internal planning artifacts as deliverables.</rule>`)
	return b.String()
}

// completionReportBullets is the Step 1 content shared by all reporting variants.
const completionReportBullets = `	    Step 1: Prepare a completion report (plain text)
	    - what you did (high level summary)
	    - where to look (key file paths, URLs, deliverables)
	    - next steps (tests/commands) if relevant`

// finalResponseRequirement is the single source for final response rules; used in all reporting variants.
const finalResponseRequirement = `- End with a direct assistant response containing the full user-visible completion report. If relevant, mention key file paths or deliverables directly. Never mention internal planning artifacts as deliverables.`

// reportingBlock returns the <rule id="reporting">...</rule> content for task-runner modes.
func reportingBlock(returnsToCoordinator bool) string {
	var b strings.Builder
	b.WriteString(`	  <rule id="reporting">
	    Before ending, complete these steps in order:

`)
	b.WriteString(completionReportBullets)
	b.WriteString("\n\n")

	finalContext := "This ends the task."
	if returnsToCoordinator {
		finalContext = "This returns your result to the coordinator."
	}
	b.WriteString("\t    Step 2: Provide the completion report as your direct response\n")
	b.WriteString("\t    - ")
	b.WriteString(finalContext)
	b.WriteString("\n\t    ")
	b.WriteString(finalResponseRequirement)
	b.WriteString("\n	  </rule>")
	return b.String()
}
