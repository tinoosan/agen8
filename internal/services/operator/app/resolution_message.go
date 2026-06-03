package app

import (
	"fmt"
	"strings"

	"github.com/tinoosan/agen8-mcp-server/internal/services/operator/domain"
)

// FormatEscalationResolutionMessage builds the inbox body for an operator resolution
// on an escalation (mirrors operator-action completion copy style). The agent
// reads the resolution as a binary verdict (approve | reject) plus the freeform
// note for any "use approach X" / "ask Bob" / "do it later" context.
func FormatEscalationResolutionMessage(esc domain.Escalation, params ResolveEscalationParams) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Your escalation %s was resolved by the operator.\n\n", esc.ID))
	if t := strings.TrimSpace(esc.Title); t != "" {
		b.WriteString(fmt.Sprintf("Title: %s\n", t))
	}
	b.WriteString(fmt.Sprintf("Resolution: %s\n", params.Resolution))
	if note := strings.TrimSpace(params.ResolutionNote); note != "" {
		b.WriteString(fmt.Sprintf("Operator note: %s\n", note))
	}
	return strings.TrimSpace(b.String())
}
