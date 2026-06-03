package reviewhandling

import (
	"strings"

	"github.com/tinoosan/agen8-mcp-server/pkg/membertype"
)

func init() {
	membertype.RegisterRule(membertype.PromptRule{
		Name:      "review_handling",
		Order:     500,
		AppliesTo: []membertype.MemberTypeName{membertype.TypeCoordinator, membertype.TypeReviewer},
		Locked:    true,
		Build:     build,
	})
}

func build(ctx membertype.PromptContext) string {
	switch ctx.MemberType {
	case membertype.TypeCoordinator:
		return membertype.JoinRuleLines([]string{
			"- When a delegated task returns in in_review, review it with task(action=\"review\") — a prose-only reply does not count.",
			"- Do not delegate review lifecycle actions to workers. Workers execute (claim/submit/block/release); review decisions (approve/retry/fail) are coordinator/reviewer responsibilities.",
			"- Review against the acceptanceCriteria checklist. Reference specific criteria that passed or failed. Do not approve work that fails any criterion.",
			"- MUST: include the criteria_checked parameter in task(action=\"review\") — a boolean array with one entry per criterion (true=met, false=not met). This updates the board's checklist.",
			"- When you retry a task, the original task is reopened with full attempt history — a new task is not created.",
			"- If you complete a self-assigned task yourself, do not create a separate review task.",
			"- When synthesizing results, use worker outputs — not your own knowledge. The delegation exists to produce grounded, specialist work.",
		})
	case membertype.TypeReviewer:
		return membertype.JoinRuleLines([]string{
			"- You are the reviewer. Review completed work on the original task when it is in in_review. Inspect the work and use task(action=\"review\") to approve, retry, or fail. A prose-only reply is not a review decision.",
			"- Review against the task's acceptanceCriteria checklist (in task metadata). Evaluate each criterion individually. In your feedback, reference specific criteria that passed or failed. Do not approve work that fails any criterion.",
			"- IMPORTANT: When calling task(action=\"review\"), you MUST include criteria_checked — a boolean array with one entry per acceptance criterion indicating whether it was met (true) or not (false). This updates the task's checklist so the board shows which criteria passed and which failed.",
			"- When retrying, provide actionable feedback citing which criteria were not met and what specifically needs to change. The worker receives your feedback as context for the next attempt.",
			strings.TrimSpace(membertype.NoteEncouragementRule()),
		})
	default:
		return ""
	}
}
