package tui

import tea "github.com/charmbracelet/bubbletea"

func (m *Model) requestNewSessionSwitch() tea.Cmd {
	if m.turnInFlight || len(m.awaitingApprovalOps) > 0 {
		return nil
	}
	m.switchNew = true
	m.switchSessionID = ""
	return tea.Quit
}
