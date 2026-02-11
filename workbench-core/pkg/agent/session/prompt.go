package session

import (
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/pkg/agent"
	"github.com/tinoosan/workbench-core/pkg/profile"
)

func buildSystemPrompt(base string, p profile.Profile, profilePrompt string, memories []agent.MemorySnippet, previousOutcome string, teamID string, roleName string, coordinatorRole string, teamRoles []string, teamRoleDescriptions map[string]string) string {
	base = strings.TrimSpace(base)
	var b strings.Builder
	if base != "" {
		b.WriteString(base)
	}

	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
	b.WriteString("<context>\n")
	b.WriteString("Current date and time: ")
	b.WriteString(time.Now().UTC().Format("2006-01-02 15:04:05 MST"))
	b.WriteString("\n")
	b.WriteString("</context>\n\n")
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

	if prev := sanitizePreviousOutcome(previousOutcome); prev != "" {
		b.WriteString("\n\n<previous_outcome>")
		b.WriteString(prev)
		b.WriteString("</previous_outcome>")
	}

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

	teamBlock := buildTeamBlock(teamID, roleName, coordinatorRole, teamRoles, teamRoleDescriptions)
	if teamBlock != "" {
		b.WriteString("\n\n")
		b.WriteString(teamBlock)
	}

	return strings.TrimSpace(b.String())
}

func sanitizePreviousOutcome(in string) string {
	// Goal: keep the injected context tiny, single-line, and safe-ish for tag embedding.
	in = strings.TrimSpace(in)
	if in == "" {
		return ""
	}
	// Collapse all whitespace (including newlines) to single spaces.
	in = strings.Join(strings.Fields(in), " ")
	// Avoid tag-breaking and keep content plain.
	in = strings.ReplaceAll(in, "<", "")
	in = strings.ReplaceAll(in, ">", "")
	if len(in) > 199 {
		// Strictly cap to < 200 characters.
		in = in[:196] + "..."
	}
	return strings.TrimSpace(in)
}

func buildTeamBlock(teamID string, roleName string, coordinatorRole string, teamRoles []string, teamRoleDescriptions map[string]string) string {
	teamID = strings.TrimSpace(teamID)
	roleName = strings.TrimSpace(roleName)
	if teamID == "" || roleName == "" {
		return ""
	}
	coordinatorRole = strings.TrimSpace(coordinatorRole)
	if coordinatorRole == "" {
		coordinatorRole = roleName
	}
	roles := make([]string, 0, len(teamRoles))
	seen := map[string]struct{}{}
	for _, role := range teamRoles {
		role = strings.TrimSpace(role)
		if role == "" {
			continue
		}
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		roles = append(roles, role)
	}
	if len(roles) == 0 {
		roles = append(roles, roleName)
		if coordinatorRole != roleName {
			roles = append(roles, coordinatorRole)
		}
	}
	allRoles := strings.Join(roles, ", ")

	var b strings.Builder
	b.WriteString("<team>\n")
	b.WriteString("You are part of a team. Team ID: \"")
	b.WriteString(teamID)
	b.WriteString("\".\n")
	b.WriteString("Your role: \"")
	b.WriteString(roleName)
	b.WriteString("\".\n")
	b.WriteString("Coordinator: ")
	b.WriteString(coordinatorRole)
	b.WriteString("\n")
	b.WriteString("All roles: ")
	b.WriteString(allRoles)
	b.WriteString("\n\n")
	if len(teamRoleDescriptions) != 0 {
		b.WriteString("Role descriptions:\n")
		for _, role := range roles {
			desc := strings.TrimSpace(teamRoleDescriptions[role])
			if desc == "" {
				continue
			}
			b.WriteString("- ")
			b.WriteString(role)
			b.WriteString(": ")
			b.WriteString(desc)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("Communication:\n")
	b.WriteString("- You receive tasks assigned to your role \"")
	b.WriteString(roleName)
	b.WriteString("\" via the shared task list.\n")
	b.WriteString("- To report results, complete your current task with a detailed summary and artifacts.\n")
	b.WriteString("- To request help or escalate, create a task with assignedRole=\"")
	b.WriteString(coordinatorRole)
	b.WriteString("\".\n")
	if roleName != coordinatorRole {
		b.WriteString("- You cannot assign tasks to other non-coordinator roles.\n")
	} else {
		b.WriteString("- As coordinator, you may assign tasks to any valid role.\n")
		b.WriteString("- As coordinator, you MUST NOT perform specialist research, analysis, or report writing.\n")
		b.WriteString("- As coordinator, your only responsibilities are: break down goals, delegate tasks, review callbacks, and track completion.\n")
		b.WriteString("- As coordinator, NEVER use web_search, file tools, or shell tools for specialist work.\n")
	}
	b.WriteString("- You share a workspace at /workspace with all team members. Save your deliverable files to /workspace/")
	b.WriteString(roleName)
	b.WriteString("/ (e.g. /workspace/")
	b.WriteString(roleName)
	b.WriteString("/report.pdf) for organization.\n")
	b.WriteString("- Use WriteMemory and AppendMemory tools for memory updates; do not write memory files directly in the workspace.\n")
	b.WriteString("</team>")
	return strings.TrimSpace(b.String())
}
