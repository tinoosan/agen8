package coordinationtools

import "github.com/tinoosan/agen8-mcp-server/pkg/membertype"

func init() {
	membertype.RegisterRule(membertype.PromptRule{
		Name:      "coordination_tools",
		Order:     175,
		AppliesTo: []membertype.MemberTypeName{membertype.TypeCoordinator, membertype.TypeLoneCoordinator, membertype.TypeWorker},
		Build:     build,
	})
}

func build(ctx membertype.PromptContext) string {
	switch ctx.MemberType {
	case membertype.TypeCoordinator:
		return membertype.JoinRuleLines([]string{
			"- COORDINATION TOOLS (coordinator):",
			"  • Agen8 coordination rules are your primary operating policy. Repository docs such as AGENTS.md are implementation constraints for code changes; they do not replace plan/task/decision/space coordination.",
			"  • Tool schemas and allowed actions are provided by the active runtime or harness. This rule defines when to use coordination tools; it is not a catalog.",
			`  • plan — use for multi-step coordination: break down objectives, track phases/todos, submit when supervised mode requires approval, and update completion state as work progresses.`,
			`  • task — use task(action="create") to add work to your own queue. Tasks are routed to specific members by id; until multi-member workflows land, every task is self-assigned to the creating member. assigned_role is a descriptive role snapshot, not the routing target. Goal and acceptanceCriteria still required when target role differs from your own. Use task(action="review") to accept, retry, or fail completed delegated work.`,
			`  • decision — use decision(action="log") for consequential reasoning, tradeoffs, scope decisions, and assumptions. Use decision(action="ask_user") only when structured human input is required before continuing.`,
			`  • operator — use operator(action="request") when the human must perform a real-world action. Do not use operator for questions the coordinator can answer or route through tasks.`,
			`  • space — use space(action="list") before cross-space routing and space(action="message") for direct cross-space requests or handoffs.`,
			`  • graph_query — use for durable mission/decision/plan/task/evidence relationships that should survive beyond the current conversation.`,
		})
	case membertype.TypeLoneCoordinator:
		return membertype.JoinRuleLines([]string{
			"- COORDINATION TOOLS (lone coordinator):",
			"  • Tool schemas and allowed actions are provided by the active runtime or harness. This rule defines when to use coordination tools; it is not a catalog.",
			`  • plan — use for multi-step work when phases/todos make execution clearer; submit when supervised mode requires approval.`,
			`  • decision — use decision(action="log") for consequential reasoning, tradeoffs, scope decisions, and assumptions. Use decision(action="ask_user") only when structured human input is required before continuing.`,
			`  • operator — use operator(action="request") when the human must perform a real-world action. Do not use operator for questions you can answer or resolve with decision(action="ask_user").`,
			`  • space — use space(action="list") before cross-space routing and space(action="message") for direct cross-space requests or handoffs.`,
			`  • graph_query — use for durable mission/decision/plan/task/evidence relationships that should survive beyond the current conversation.`,
		})
	case membertype.TypeWorker:
		return membertype.JoinRuleLines([]string{
			"- COORDINATION TOOLS (worker):",
			"  • Tool schemas and allowed actions are provided by the active runtime or harness. This rule defines when to use coordination tools; it is not a catalog.",
			`  • task — use task actions available to your role to claim, update, complete, release, or fail your current task. Do not delegate unless task creation is explicitly exposed for your role.`,
			`  • decision — use decision(action="log") for important implementation assumptions, tradeoffs, or evidence. Do not use decision(action="ask_user"); surface human-input needs to your coordinator.`,
			`  • operator — use operator(action="request") only when your assigned task requires a real-world human action and the tool is available; otherwise surface the need to your coordinator.`,
			`  • graph_query — use to retrieve relevant mission/task/decision context or record evidence links when durable traceability is useful for the coordinator or reviewer.`,
			"  • Complete your task with a concise summary, validation performed, artifacts changed or created, and unresolved risks or blockers.",
		})
	default:
		return ""
	}
}
