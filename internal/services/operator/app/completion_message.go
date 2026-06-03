// Package app — completion_message.go provides the system message formatter
// for operator action completions (F25). When an OA reaches completed status,
// a structured system message is sent to the agent's conversation so the agent
// receives full context about the real-world work that was done.
package app

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tinoosan/agen8-mcp-server/internal/services/operator/domain"
)

// FormatCompletionMessage formats the structured system message body for a
// completed operator action. The message includes outcome status, summary,
// structured key-value pairs, attachment references, and progress history.
func FormatCompletionMessage(oa domain.OperatorAction) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Operator Action completed: %q\n", oa.Title))
	if strings.TrimSpace(oa.TaskRef) != "" {
		b.WriteString(fmt.Sprintf("Blocked task: %s\n", oa.TaskRef))
	}
	b.WriteString(fmt.Sprintf("Operator action ID: %s\n", oa.ID))
	b.WriteString(fmt.Sprintf("Outcome: %s\n", oa.OutcomeStatus))
	b.WriteString(fmt.Sprintf("Summary: %s\n", oa.OutcomeSummary))
	if strings.TrimSpace(oa.TaskRef) != "" {
		b.WriteString(fmt.Sprintf("Next step: the task has been unblocked and is ready to resume. Use task(action=\"claim\", task_id=%q) to pick it up and continue.\n", oa.TaskRef))
	}

	if len(oa.OutcomePairs) > 0 {
		b.WriteString("\nData:\n")
		// Sort keys for deterministic output.
		keys := make([]string, 0, len(oa.OutcomePairs))
		for k := range oa.OutcomePairs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.WriteString(fmt.Sprintf("- %s: %s\n", k, oa.OutcomePairs[k]))
		}
	}

	if len(oa.Attachments) > 0 {
		b.WriteString("\nAttachments:\n")
		for _, att := range oa.Attachments {
			switch att.Kind {
			case "file":
				label := att.Label
				if label == "" {
					label = att.Filename
				}
				b.WriteString(fmt.Sprintf("- [file] %s (%s, %d bytes) — %s\n", label, att.Filename, att.SizeBytes, att.ID))
			case "url":
				label := att.Label
				if label == "" {
					label = att.URL
				}
				b.WriteString(fmt.Sprintf("- [url] %s — %s\n", label, att.URL))
			}
		}
		b.WriteString("\nAttachment IDs are included for traceability. Direct attachment retrieval tools are disabled in this build.\n")
	}

	if len(oa.ProgressNotes) > 0 {
		b.WriteString("\nProgress notes:\n")
		for i, note := range oa.ProgressNotes {
			b.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, note.CreatedAt.UTC().Format("2006-01-02 15:04"), note.Text))
		}
	}

	if len(oa.Comments) > 0 {
		b.WriteString("\nComments:\n")
		for i, comment := range oa.Comments {
			b.WriteString(fmt.Sprintf("%d. [%s] %s: %s\n", i+1, comment.CreatedAt.UTC().Format("2006-01-02 15:04"), comment.Author, comment.Text))
		}
	}

	return strings.TrimRight(b.String(), "\n")
}
