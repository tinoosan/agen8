package app

import (
	"fmt"
	"strings"

	"github.com/tinoosan/agen8-mcp-server/internal/services/operator/domain"
)

// FormatCommentMessage formats the structured system message body for an
// operator-authored comment on an active operator action.
func FormatCommentMessage(oa domain.OperatorAction, comment domain.Comment) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Operator comment on Operator Action %q\n", oa.Title))
	b.WriteString(fmt.Sprintf("Status: %s\n", oa.Status))
	if oa.Description != "" {
		b.WriteString(fmt.Sprintf("Description: %s\n", oa.Description))
	}
	b.WriteString(fmt.Sprintf("Action ID: %s\n", oa.ID))
	b.WriteString(fmt.Sprintf("Comment: %s\n", comment.Text))
	b.WriteString("\nAction-space replies are disabled in this build. If follow-up operator work is needed, create a new operator(action=\"request\") linked to the same task/key result/mission.")

	return strings.TrimRight(b.String(), "\n")
}
