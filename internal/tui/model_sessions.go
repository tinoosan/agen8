package tui

import tea "github.com/charmbracelet/bubbletea"

func (m *Model) requestNewSessionSwitch() tea.Cmd {
	if m.turnInFlight {
		return nil
	}
	m.switchNew = true
	m.switchSessionID = ""
	return tea.Quit
}
