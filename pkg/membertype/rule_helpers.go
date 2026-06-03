package membertype

import (
	"path"
	"strings"
)

func JoinRuleLines(lines []string) string {
	var b strings.Builder
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		b.WriteString(line)
		if !strings.HasSuffix(line, "\n") {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func CoordinatorWorkspaceExample(ctx PromptContext) string {
	spaceName := strings.TrimSpace(ctx.SpaceName)
	memberLabel := strings.TrimSpace(ctx.MemberLabel)
	workspaceExample := "workspace/<space>/<target-member>/report.pdf"
	if spaceName == "" {
		return workspaceExample
	}
	targetMember := "<target-member>"
	if memberLabel != "" {
		targetMember = memberLabel
	}
	workspaceExample = path.Join("workspace", sanitizeWorkspaceSegment(spaceName), sanitizeWorkspaceSegment(targetMember))
	return strings.TrimSuffix(workspaceExample, targetMember) + "<target-member>/report.pdf"
}

func WorkerWorkspacePaths(ctx PromptContext) (string, string, string) {
	memberLabel := strings.TrimSpace(ctx.MemberLabel)
	spaceName := strings.TrimSpace(ctx.SpaceName)
	workspaceRoot := "workspace/<space>/"
	workspaceFile := "workspace/<space>/<member>/report.md"
	workspaceBin := "workspace/<space>/<member>/artifact.bin"
	if spaceName != "" && memberLabel != "" {
		workspaceRoot = path.Join("workspace", sanitizeWorkspaceSegment(spaceName)) + "/"
		workspaceFile = path.Join("workspace", sanitizeWorkspaceSegment(spaceName), sanitizeWorkspaceSegment(memberLabel), "report.md")
		workspaceBin = path.Join("workspace", sanitizeWorkspaceSegment(spaceName), sanitizeWorkspaceSegment(memberLabel), "artifact.bin")
	}
	return workspaceRoot, workspaceFile, workspaceBin
}

// AIIdentityRule returns the shared "you are an AI agent, not a human" rule.
// compact=true gives a one-liner for workers; compact=false gives the full version for coordinators.
func AIIdentityRule(compact bool) string {
	if compact {
		return "- You are an AI agent. You have no calendar, availability, or need for meetings. Exchange information directly — never propose scheduling, meetings, or other human coordination rituals.\n"
	}
	return "- You and all other space coordinators are AI agents, not humans. You operate instantly — no calendars, availability windows, time zones, meetings, or scheduling. When you need something from another space, use space(action=\"message\") with the request or data. Never propose meeting times, availability slots, calendar coordination, agendas, sync-ups, standups, or check-ins. If another agent proposes scheduling, redirect: exchange the information directly.\n"
}

// InterCoordinatorProtocol returns the shared messaging protocol rules for coordinators.
func InterCoordinatorProtocol() string {
	var b strings.Builder
	b.WriteString("- You receive MESSAGES (communication) and TASKS (work). Messages come from the user or other coordinators as conversation turns. Tasks come from delegation or continuations (pending → active → in_review → done).\n")
	b.WriteString("- User messages are plain text from the human user. Messages from other coordinators are labeled with sender name and source space — do not confuse them with user messages. System messages are labeled [System: ...].\n")
	b.WriteString("- For a new outbound space(action=\"message\") (inform or query), omit correlation_id — the runtime generates it. Use the inbound correlation_id when sending ack or response.\n")
	b.WriteString("- Call space(action=\"list\") to get the live space catalog before routing with space(action=\"message\"). It returns stable space identifiers, member labels, and member types.\n")
	b.WriteString("- When you receive a query you can answer now, reply with space(action=\"message\", kind=\"response\", correlation_id=<inbound correlation_id>).\n")
	b.WriteString("- When you receive a query you cannot answer yet, send space(action=\"message\", kind=\"ack\", correlation_id=<inbound correlation_id>), then respond later when the result is ready.\n")
	b.WriteString("- MUST: When you receive an ack (kind=ack), do NOT reply to it. An ack is a receipt confirmation — replying creates infinite loops.\n")
	b.WriteString("- MUST: When replying to a query, always use kind=\"response\" with the inbound correlation_id. Never use kind=\"inform\" to answer a query — the runtime will reject it. Use inform only for unsolicited updates.\n")
	b.WriteString("- space(action=\"message\") is asynchronous. The tool confirms delivery only; it does not return the other space's reply. A reply arrives as a later message with the same correlation_id.\n")
	b.WriteString("- After sending a space(action=\"message\") query, continue useful work if you have any. Otherwise end your turn and wait to be re-invoked with the response.\n")
	return b.String()
}

// TurnEndGuidance returns the shared coordinator turn-end instruction.
func TurnEndGuidance() string {
	return "- Call note(text=\"...\") to share your thinking with the user mid-turn without ending the turn. Use it to narrate what you're investigating, why you're changing approach, or what surprised you.\n" +
		"- After receiving tool results, deliver your complete response directly as assistant text. Do not split it across a partial reply plus a follow-up control tool call.\n"
}

// NoteEncouragementRule returns guidance to use note() liberally.
func NoteEncouragementRule() string {
	return "- Use note(text=\"...\") liberally to keep the user informed as you work. Narrate what you're doing, what you're trying next, what you learned from the last tool call. Write naturally — this is how the user watches your process unfold. Do not use it for terse status pings like 'working on it'; instead explain *why* you're doing what you're doing.\n"
}

// ProactiveWorkRule returns the shared "work proactively, stay scoped" guidance.
func ProactiveWorkRule() string {
	return "- Work proactively. Do not wait for user input when you can make reasonable progress. Keep follow-up work inside the active objective — do not silently broaden a narrow request without explicit user instruction.\n"
}
