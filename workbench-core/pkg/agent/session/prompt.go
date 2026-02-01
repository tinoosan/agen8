package session

import (
	"strings"

	"github.com/tinoosan/workbench-core/pkg/agent"
	"github.com/tinoosan/workbench-core/pkg/profile"
)

func buildSystemPrompt(base string, p profile.Profile, profilePrompt string, memories []agent.MemorySnippet) string {
	base = strings.TrimSpace(base)
	var b strings.Builder
	if base != "" {
		b.WriteString(base)
	}

	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
	b.WriteString("<profile>\n")
	b.WriteString("ID: ")
	b.WriteString(strings.TrimSpace(p.ID))
	b.WriteString("\n")
	if strings.TrimSpace(p.Description) != "" {
		b.WriteString("Description: ")
		b.WriteString(strings.TrimSpace(p.Description))
		b.WriteString("\n")
	}
	if len(p.Skills) != 0 {
		b.WriteString("Skills:\n")
		for _, s := range p.Skills {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			b.WriteString("- ")
			b.WriteString(s)
			b.WriteString("\n")
		}
	}
	if strings.TrimSpace(profilePrompt) != "" {
		b.WriteString("Prompt:\n")
		b.WriteString(strings.TrimSpace(profilePrompt))
		if !strings.HasSuffix(profilePrompt, "\n") {
			b.WriteString("\n")
		}
	}
	b.WriteString("</profile>")

	if len(memories) != 0 {
		b.WriteString("\n\n<memories>\n")
		for i, m := range memories {
			if i >= 6 {
				break
			}
			title := strings.TrimSpace(m.Title)
			content := strings.TrimSpace(m.Content)
			if title == "" && content == "" {
				continue
			}
			if title != "" {
				b.WriteString("Title: ")
				b.WriteString(title)
				b.WriteString("\n")
			}
			if content != "" {
				if len(content) > 1200 {
					content = content[:1200] + "…"
				}
				b.WriteString(content)
				b.WriteString("\n")
			}
			b.WriteString("---\n")
		}
		b.WriteString("</memories>")
	}

	return strings.TrimSpace(b.String())
}

