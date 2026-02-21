package tui

import (
	"fmt"
	"strings"
)

// agentOutputForClipboard exports the current Agent Output stream as plain text.
func (m *monitorModel) agentOutputForClipboard() string {
	if m == nil {
		return ""
	}
	items := m.currentAgentOutputItems()
	if len(items) == 0 {
		return ""
	}

	var b strings.Builder
	for _, item := range items {
		content := normalizeNewlinesForClipboard(strings.TrimSpace(item.Content))
		if content == "" {
			continue
		}
		lines := strings.Split(content, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if isTimestampedLogLine(line) {
				b.WriteString(line)
				b.WriteString("\n")
				continue
			}
			b.WriteString(fmt.Sprintf("[%s] %s\n", item.Timestamp.Local().Format("15:04:05"), line))
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return ""
	}
	return out + "\n"
}
