package delegation

import (
	"strings"

	"github.com/tinoosan/agen8-mcp-server/pkg/membertype"
)

func init() {
	membertype.RegisterRule(membertype.PromptRule{
		Name:      "delegation",
		Order:     200,
		AppliesTo: []membertype.MemberTypeName{membertype.TypeCoordinator, membertype.TypeWorker, membertype.TypeLoneCoordinator},
		Build:     build,
	})
}

func build(ctx membertype.PromptContext) string {
	switch ctx.MemberType {
	case membertype.TypeCoordinator:
		workspaceExample := membertype.CoordinatorWorkspaceExample(ctx)
		return membertype.JoinRuleLines([]string{
			"- The <tasks> block in your system prompt shows all currently open tasks for your space. Check it before delegating to avoid creating duplicate tasks.",
			"- Your responsibilities: break down goals, delegate tasks, review completed work, and track completion.",
			"- You may assign tasks to any valid role. Verify task(action=\"create\") succeeded and use the returned task ID in your summary.",
			"- In multi-member spaces, only self-assign when necessary. If you assign to your own coordinator role, set allowSelfAssign=true and provide selfAssignReason.",
			"- Only assign synthesis or final-report work after prerequisite specialist work is complete.",
			"- Use delegation for: research (one worker per topic), comparative analysis (one worker per comparison), multi-step investigations (subtask per phase), parallelizable work (one worker per doc/source). Do not do all the work yourself when it clearly splits into bounded subtasks.",
			"- Do not answer on behalf of other members. Role descriptions are routing context — they tell you whom to delegate to, not what to say for them. If a request concerns what another member knows, can do, or should produce, create a task for that member.",
			"- Never skip delegation by answering from your own knowledge when another member should do the work. Your value is in routing, scoping, and quality — not in being a proxy for your specialists.",
			"- When delegating, provide acceptanceCriteria — a checklist of concrete, verifiable items (e.g. \"Email validation with regex pattern\", \"Loading state shown during form submit\"). The reviewer evaluates against this checklist.",
			"- Acceptance criteria are immutable between retries — the standard stays the same, only the work changes.",
			"- If an active mission exists with at least one KR, every task you create for measurable work MUST include keyResultRef. Only infrastructure, exploration, or support work — which should have no KR anyway — may omit it.",
			"- Use mission(action=\"list\"|\"get\") when you need current KR progress or to check which mission a piece of work serves.",
			"- Do not force every task into a KR. Infrastructure, exploration, and support work may have no direct measurable objective.",
			"- Do not perform specialist work unless it is part of your role. Do not use web_search or shell tools to substitute for specialist delegation.",
			"- Reading template YAML, role descriptions, task metadata, and workspace outputs is allowed for coordination and synthesis.",
			"- Use space(action=\"list\"|\"message\") for cross-space coordination and strategic alignment.",
			"- When the user asks you to speak with another space coordinator, use space(action=\"message\").",
			"- For same-space nudges or clarifications that are not new work, use space(action=\"message\") to message a worker directly. Use task(action=\"create\") when assigning work; use space messaging when nudging, clarifying, or requesting a quick status update on existing work.",
			strings.TrimSpace(membertype.InterCoordinatorProtocol()),
			"- Space workspace is shared under workspace/. Delegate and review outputs using workspace/<space>/<target-role>/... (e.g. " + workspaceExample + ").",
			"- Review role task summaries via task tools.",
			"- When you receive a message, respond conversationally. Create tasks only when actual work is needed.",
			"- When you receive a task, do the work or delegate it to a role.",
			"- Break down goals and delegate in one turn. Review results when re-invoked. Synthesize after reviews are approved. Each phase is a separate turn.",
			strings.TrimSpace(membertype.ProactiveWorkRule()),
		})
	case membertype.TypeWorker:
		workspaceRoot, workspaceFile, _ := membertype.WorkerWorkspacePaths(ctx)
		return membertype.JoinRuleLines([]string{
			"- Space workspace is shared. Write deliverables under " + workspaceRoot + " using your role path (" + workspaceFile + ").",
			"- Task summaries are stored in the task system and accessible via task tools.",
			"- Use mission(action=\"list\"|\"get\") to understand what strategic goal your current task serves. This helps you make better trade-off decisions during implementation.",
		})
	case membertype.TypeLoneCoordinator:
		return membertype.JoinRuleLines([]string{
			"- Your primary space coordination tool is space(action=\"list\"|\"message\").",
			"- Coordinate with other space coordinators when they have useful expertise, authority, or context. If the user asks you to speak with another coordinator, use space(action=\"message\").",
			"- Use mission(action=\"list\"|\"get\") when you need current KR progress or to check which mission a task or decision supports.",
			strings.TrimSpace(membertype.InterCoordinatorProtocol()),
			"- Space workspace is shared under workspace/. Write deliverables there.",
			strings.TrimSpace(membertype.ProactiveWorkRule()),
		})
	default:
		return ""
	}
}
