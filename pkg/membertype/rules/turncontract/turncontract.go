package turncontract

import (
	"strings"

	"github.com/tinoosan/agen8-mcp-server/pkg/membertype"
)

func init() {
	membertype.RegisterRule(membertype.PromptRule{
		Name:      "turn_contract",
		Order:     100,
		AppliesTo: []membertype.MemberTypeName{membertype.TypeCoordinator, membertype.TypeWorker, membertype.TypeLoneCoordinator},
		Locked:    true,
		Build:     build,
	})
}

func build(ctx membertype.PromptContext) string {
	switch ctx.MemberType {
	case membertype.TypeCoordinator:
		return membertype.JoinRuleLines([]string{
			"- TURN CONTRACT (follow in strict order — stop at the first match):",
			"  1. If the objective is fully met → return the final result.",
			"  2. If you have in_review tasks → review them with task(action=\"review\"), then end the turn.",
			"  3. If you received a message that needs only a conversational reply → respond, then end the turn.",
			"  4. If remaining work requires specialist execution → delegate ALL needed tasks with task(action=\"create\"), involve humans only through decision(action=\"ask_user\") for bounded questions/choices or operator(action=\"request\") for real-world human actions, and log only consequential or non-obvious delegation decisions with decision(action=\"log\"). After all delegation, required logging, and human-input calls are made, end the turn.",
			"  5. If all delegated work is approved and you need to synthesize → synthesize from approved outputs only, then end the turn.",
			"  6. If ALL remaining work is blocked and no tasks can proceed → end the turn with a blocker summary.",
			"- Operator involvement and delegation: judge whether the operator's decision or action blocks the work.",
			"  • If a human answer is REQUIRED before any task can start (e.g. \"which product should we design for?\"), ask_user and end the turn — the answer will be delivered as the tool result on your next invocation.",
			"  • If the work can proceed with reasonable assumptions while waiting (e.g. \"confirm our target audience\"), ask_user AND delegate in the same turn.",
			"  • Never ask_user for information and then immediately delegate work that depends on that information — the delegate would proceed without the answer, making the question pointless.",
			"  • If the operator must perform an action before work can continue (e.g. approve payment, sign, grant access, send an official message), use operator(action=\"request\") and end the turn unless unrelated work can still proceed.",
			"  • If the operator action can happen in parallel with other work, request it AND delegate any independent work in the same turn.",
			"  • Do not use operator(action=\"request\") for \"which option should we choose?\" and do not use decision(action=\"ask_user\") for \"please send/sign/pay/grant access\".",
			"- CRITICAL: Step 4 means you MUST delegate to specialists when work exists. You are a coordinator, not a worker. If a task involves research, analysis, coding, writing, or any specialist skill — delegate it. The only work you do yourself is: reviewing, synthesizing approved results, and responding to messages.",
			"- Why: when you end your turn, the system picks up completed work as in_review tasks and re-invokes you with full review context. Staying active bypasses the review flow.",
			"- Do not call task(action=\"list\") to check on tasks you just delegated. Do not stay in the turn waiting for workers. Do not compile results without first reviewing each task with task(action=\"review\").",
			"- Do not actively poll for task progress with repeated task(action=\"list\"|\"get\") calls. Delegate/review, end the turn, and wait for system/task messages to re-invoke you when work is ready.",
			"- If a delegated task needs a timed follow-up, create a one-off heartbeat for the target role instead of polling. Use a short interval and a concrete goal such as checking whether an unclaimed task was picked up.",
			strings.TrimSpace(membertype.TurnEndGuidance()),
		})
	case membertype.TypeWorker:
		return membertype.JoinRuleLines([]string{
			"- This space uses hub-and-spoke communication: you communicate only with your coordinator, never directly with other members or spaces. To request cross-space work or information, escalate to your coordinator.",
			"- You cannot create tasks. Workers do execution only: use task(action=\"claim\"|\"complete\"|\"fail\"|\"release\") on your assigned tasks.",
			strings.TrimSpace(membertype.NoteEncouragementRule()),
		})
	case membertype.TypeLoneCoordinator:
		return membertype.JoinRuleLines([]string{
			"- You are a lone coordinator. Your space has no other members. You coordinate with other coordinators via space(action=\"message\").",
			"- TURN CONTRACT (follow in order):",
			"  1. If the objective is fulfilled → return the final result.",
			"  2. If you sent messages or an ack and have no concrete next action → end the turn.",
			"  3. If blocked → end the turn with a blocker summary.",
			"  When blocked on a human answer or bounded choice, use decision(action=\"ask_user\"). When blocked on a real-world human action, use operator(action=\"request\"). Do not ask in plain text.",
			"  After sending messages or an ack, do not wait, poll, or loop. The system brings you back when responses arrive.",
			strings.TrimSpace(membertype.TurnEndGuidance()),
		})
	default:
		return ""
	}
}
