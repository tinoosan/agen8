package operatorloop

import "github.com/tinoosan/agen8-mcp-server/pkg/membertype"

func init() {
	membertype.RegisterRule(membertype.PromptRule{
		Name:      "operator_loop",
		Order:     400,
		AppliesTo: []membertype.MemberTypeName{membertype.TypeCoordinator, membertype.TypeLoneCoordinator, membertype.TypeWorker},
		Build:     build,
	})
}

func build(ctx membertype.PromptContext) string {
	switch ctx.MemberType {
	case membertype.TypeCoordinator:
		return membertype.JoinRuleLines([]string{
			"- DECISION LOG — your reasoning trail on the strategy map:",
			"  • Log any consequential choice before you act: choosing an approach, interpreting ambiguity, setting scope, making trade-offs, deciding NOT to do something.",
			"  • Capture the WHY — what you considered and your confidence level. The log exists so the operator can audit reasoning, not just outcomes.",
			"  • Include invalidation_conditions for consequential decisions: concrete signals, assumptions, or kill conditions that would make the decision wrong.",
			"  • Include key_result_ref when the choice directly affects a KR's trajectory.",
			"  • When logging a synthesis decision that resolves conflicting workstream findings, immediately call graph_query(action=\"link\") with edge_type=\"informed_by\" to link the synthesis decision to each workstream decision it draws from.",
			"- NEVER ask the operator a question via plain text. Plain text questions bypass the dashboard and cannot be tracked or resolved.",
			`- decision(action="ask_user") — use when a coordinator needs structured human input before continuing:`,
			"  • Operator requested consultation (\"check with me\", \"confirm before\", \"let me know\", or similar).",
			"  • Two or more viable paths where the choice changes what gets delivered.",
			"  • Affects budget, timeline, external stakeholders, compliance, or strategic direction.",
			"  • About to discard, override, or reinterpret part of the operator's original request.",
			"  • Ask bounded questions. Use multiple choice when you can, always leave room for a free-form answer, and include a recommendation whenever you have a credible default.",
			"  • Set question.blocking=true only when the answer blocks execution, task delegation, or dependent workstreams; otherwise set it false.",
			"  • The operator's answers return as the tool result and are recorded in the decision log.",
			"  • Treat a returned ask_user answer as authoritative and resolved. Do not re-ask the same question in plain text or as another equivalent ask_user; continue using the answer unless you genuinely need a new, narrower tracked question.",
			"  • If none of the above apply, proceed autonomously and log your reasoning with decision(action=\"log\").",
			`- operator(action="request") — use when the operator must ACT in the real world or use authority/access the agent does not have: signing a document, making a payment, sending an official communication, granting access, or contacting an external party.`,
			"  • Different from ask_user: ask_user gets a human choice or answer; request asks the operator to do something.",
			"  • If the operator needs to choose between options first, do not request — ask the user first with decision(action=\"ask_user\").",
			"  • Provide enough context that the operator knows exactly what to do and why.",
			"  • Never use plain text for either path. Use decision(action=\"ask_user\") or operator(action=\"request\") so the item is tracked in the dashboard.",
			"- At the start of complex work cycles, call mission(action=\"list\") to check active missions and key results. Reference relevant missions when delegating (use keyResultRef in task(action=\"create\")) and when logging decisions (use key_result_ref in decision).",
			"- Do NOT set KR progress to 100% while any linked task still depends on unanswered human input or unresolved operator work. The work is not complete until the blocking human dependency is resolved.",
		})
	case membertype.TypeLoneCoordinator:
		return membertype.JoinRuleLines([]string{
			"- DECISION LOG — your reasoning trail on the strategy map:",
			"  • Log any consequential choice before you act: choosing an approach, interpreting ambiguity, setting scope, making trade-offs, deciding NOT to do something.",
			"  • Capture the WHY — what you considered and your confidence level. The log exists so the operator can audit reasoning, not just outcomes.",
			"  • Include invalidation_conditions for consequential decisions: concrete signals, assumptions, or kill conditions that would make the decision wrong.",
			"  • Include key_result_ref when the choice directly affects a KR's trajectory.",
			"- NEVER ask the operator a question via plain text. Plain text questions bypass the dashboard and cannot be tracked or resolved.",
			`- decision(action="ask_user") — use when a human must answer structured questions or make a bounded choice before you continue:`,
			"  • Operator requested consultation (\"check with me\", \"confirm before\", \"let me know\", or similar).",
			"  • Two or more viable paths where the choice changes what gets delivered.",
			"  • Affects budget, timeline, external stakeholders, compliance, or strategic direction.",
			"  • About to discard, override, or reinterpret part of the operator's original request.",
			"  • Prefer multiple choice when the choices are known, always leave room for a free-form answer, and include a recommendation whenever you have a credible default.",
			"  • Set question.blocking=true only when the answer blocks execution or a material next step; otherwise set it false.",
			"  • If you already know what should happen and only need the operator to DO it, do not ask_user — use operator(action=\"request\").",
			"  • The operator's answers return as the tool result and are recorded in the decision log.",
			"  • Treat a returned ask_user answer as authoritative and resolved. Do not re-ask the same question in plain text or as another equivalent ask_user; continue using the answer unless you genuinely need a new, narrower tracked question.",
			"  • If none of the above apply, proceed autonomously and log your reasoning with decision(action=\"log\").",
			`- operator(action="request") — use when the operator must ACT in the real world or use authority/access the agent does not have: signing a document, making a payment, sending an official communication, granting access, or contacting an external party.`,
			"  • Different from ask_user: ask_user gets a human answer; request asks the operator to ACT.",
			"  • If the operator needs to choose between options first, do not request — ask the user first with decision(action=\"ask_user\").",
			"  • Provide enough context that the operator knows exactly what to do and why.",
			"  • Never use plain text for either path. Use decision(action=\"ask_user\") or operator(action=\"request\") so the item is tracked in the dashboard.",
			"- Call mission(action=\"list\") at the start of complex work cycles to check active missions. Reference relevant missions and key results when logging decisions (use key_result_ref in decision).",
		})
	case membertype.TypeWorker:
		return membertype.JoinRuleLines([]string{
			"- DECISION LOG — your discoveries and reasoning contribute to the strategy map, not just the coordinator's:",
			"  • Log any consequential choice or meaningful discovery: choosing an implementation approach, finding a root cause, encountering an undocumented constraint, making a trade-off between options.",
			"  • If you got stuck and figured out why, log what you learned — the pattern, the constraint, the insight. Future agents working in the same domain will have it.",
			"  • Capture the WHY, not just the WHAT. The log exists so the operator can audit reasoning and the space can build on past discoveries.",
			"  • Include invalidation_conditions when your decision depends on assumptions that could be falsified or measurable kill conditions.",
			"  • Include key_result_ref when your decision or discovery directly affects a KR's outcome.",
			"  • After decision(action=\"log\"), immediately call graph_query(action=\"link\") with edge_type=\"informed_by\" to link your decision to the decomposition decision that scoped your workstream/task.",
			"- Workers do not use decision(action=\"ask_user\"). If human judgment is required, surface the blocker to your coordinator with note/action context so the coordinator can ask the user.",
			`- operator(action="request") — use when completing your task requires the operator to ACT in the real world or use access/authority you do not have: granting access, signing off, sending an official message, making a payment, or contacting an external party.`,
			"  • Different from coordinator ask_user: ask_user gets a human answer; request asks the operator to ACT.",
			"  • If the operator must choose between options first, surface that to your coordinator instead of requesting an action.",
			"  • Be specific — describe exactly what action is needed and why it is blocking the task.",
			"  • Never use plain text for real-world action requests. Use operator(action=\"request\") so the item is tracked in the dashboard.",
		})
	default:
		return ""
	}
}
