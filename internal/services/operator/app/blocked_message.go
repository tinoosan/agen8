// Package app — blocked_message.go provides the system message formatter
// for operator action blocked notifications. When an OA is blocked by the
// operator, a structured system message is sent to the creating agent's
// conversation so the agent knows work has paused.
package app

import (
	"fmt"
	"strings"

	"github.com/tinoosan/agen8-mcp-server/internal/services/operator/domain"
)

// FormatBlockedMessage formats the structured system message body for a
// blocked operator action.
func FormatBlockedMessage(oa domain.OperatorAction, reason string) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Operator Action blocked: %q\n", oa.Title))
	b.WriteString(fmt.Sprintf("Operator action ID: %s\n", oa.ID))
	if strings.TrimSpace(oa.TaskRef) != "" {
		b.WriteString(fmt.Sprintf("Blocked task: %s\n", oa.TaskRef))
	}
	if strings.TrimSpace(reason) != "" {
		b.WriteString(fmt.Sprintf("Reason: %s\n", reason))
	}
	b.WriteString("\nThe operator has paused work on this request. No action required — you will be notified when the operator resumes or completes the action.")

	return strings.TrimRight(b.String(), "\n")
}
