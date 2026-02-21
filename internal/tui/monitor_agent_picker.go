package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/agen8/internal/tui/kit"
	"github.com/tinoosan/agen8/pkg/protocol"
)

type agentPickerItem struct {
	runID   string
	label   string
	status  string
	summary string
	role    string
	teamID  string
}

func (a agentPickerItem) FilterValue() string {
	return strings.TrimSpace(a.runID + " " + a.label + " " + a.status + " " + a.summary)
}
func (a agentPickerItem) Title() string {
	if v := strings.TrimSpace(a.label); v != "" {
		return v
	}
	return strings.TrimSpace(a.runID)
}
func (a agentPickerItem) Description() string { return strings.TrimSpace(a.status + " " + a.summary) }

func renderAgentPickerLine(item list.Item, maxWidth int) string {
	it, ok := item.(agentPickerItem)
	if !ok {
		return kit.TruncateRight(strings.TrimSpace(item.FilterValue()), maxWidth)
	}
	line := strings.TrimSpace(it.Title())
	if desc := strings.TrimSpace(it.Description()); desc != "" {
		line += " — " + desc
	}
	return kit.TruncateRight(line, maxWidth)
}

func agentsToPickerItems(agents []protocol.AgentListItem) []list.Item {
	out := make([]list.Item, 0, len(agents))
	for i, a := range agents {
		runID := strings.TrimSpace(a.RunID)
		if runID == "" {
			continue
		}
		status := strings.TrimSpace(a.Status)
		if status == "" {
			status = "unknown"
		}
		label := shortID(runID)
		profile := strings.TrimSpace(a.Profile)
		role := strings.TrimSpace(a.Role)
		parentRunID := strings.TrimSpace(a.ParentRunID)
		if parentRunID != "" {
			n := a.SpawnIndex
			if n <= 0 {
				n = i + 1
			}
			goal := strings.TrimSpace(a.Goal)
			if goal == "" {
				goal = "(no goal)"
			}
			label = fmt.Sprintf("Sub-agent %d · %s", n, truncateText(goal, 50))
		} else if role != "" {
			if profile != "" {
				label = role + " · " + profile + " · " + shortID(runID)
			} else {
				label = role + " · " + shortID(runID)
			}
		} else if profile != "" {
			label = profile + " · " + shortID(runID)
		}
		if strings.EqualFold(status, "paused") {
			label = "paused · " + label
		}
		summary := strings.TrimSpace(a.Goal)
		if summary == "" {
			summary = "(no goal)"
		}
		out = append(out, agentPickerItem{
			runID:   runID,
			label:   label,
			status:  status,
			summary: truncateText(summary, 80),
			role:    role,
			teamID:  strings.TrimSpace(a.TeamID),
		})
	}
	return out
}

func (m *monitorModel) openAgentPicker() tea.Cmd {
	if m.isDetached() {
		return func() tea.Msg {
			return commandLinesMsg{lines: []string{"[command] select or create a session first: /new or /sessions"}}
		}
	}
	m.helpModalOpen = false
	m.closeAllPickers()

	m.agentPickerOpen = true
	m.agentPickerErr = ""
	l := list.New(nil, kit.NewPickerDelegate(kit.DefaultPickerDelegateStyles(), renderAgentPickerLine), 0, 0)
	l.Title = "Select Agent/Run"
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	l.SetFilteringEnabled(true)
	l.SetShowFilter(false)
	l.Styles.Title = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#707070")).
		Bold(true)
	m.agentPickerList = l
	return m.fetchAgentsList()
}

func (m *monitorModel) closeAgentPicker() {
	m.agentPickerOpen = false
	m.agentPickerList = list.Model{}
	m.agentPickerErr = ""
}

func (m *monitorModel) fetchAgentsList() tea.Cmd {
	return func() tea.Msg {
		var res protocol.AgentListResult
		if err := m.rpcRoundTrip(protocol.MethodAgentList, protocol.AgentListParams{
			ThreadID: protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
		}, &res); err != nil {
			return agentsListMsg{err: err}
		}
		return agentsListMsg{agents: res.Agents}
	}
}

func (m *monitorModel) updateAgentPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.closeAgentPicker()
		return m, nil
	case tea.KeyEnter:
		return m, m.selectAgentFromPicker()
	default:
		var cmd tea.Cmd
		m.agentPickerList, cmd = m.agentPickerList.Update(msg)
		return m, cmd
	}
}

func (m *monitorModel) selectAgentFromPicker() tea.Cmd {
	if m.agentPickerList.Items() == nil || len(m.agentPickerList.Items()) == 0 {
		return nil
	}
	selected := m.agentPickerList.SelectedItem()
	item, ok := selected.(agentPickerItem)
	if !ok {
		return nil
	}
	runID := strings.TrimSpace(item.runID)
	if runID == "" {
		return nil
	}
	m.closeAgentPicker()
	if strings.TrimSpace(m.teamID) != "" {
		m.focusedRunID = runID
		role := strings.TrimSpace(item.role)
		if role == "" {
			role = strings.TrimSpace(m.teamRoleByRunID[runID])
		}
		m.focusedRunRole = role
		focusLabel := strings.TrimSpace(role)
		if focusLabel == "" {
			focusLabel = shortID(runID)
		}
		return tea.Batch(
			func() tea.Msg {
				return commandLinesMsg{lines: []string{"[team] focus set to " + focusLabel}}
			},
			m.applyFocusLens(),
		)
	}
	return func() tea.Msg { return monitorSwitchRunMsg{RunID: runID} }
}

func (m *monitorModel) renderAgentPicker(base string) string {
	dims := kit.ComputeModalDims(m.width, m.height, 90, 22, 56, 12, 8, 4)
	m.agentPickerList.SetWidth(dims.ModalWidth - 4)
	m.agentPickerList.SetHeight(dims.ListHeight)

	content := m.agentPickerList.View()
	if strings.TrimSpace(m.agentPickerErr) != "" {
		errLine := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff8080")).Render("Error: " + m.agentPickerErr)
		content = errLine + "\n\n" + content
	}
	content += "\n" + kit.StyleDim.Render("Enter: switch run • Esc: close")

	opts := kit.DefaultPickerModalOpts(content, m.width, m.height, dims.ModalWidth, dims.ModalHeight)

	_ = base
	return kit.RenderOverlay(opts)
}
