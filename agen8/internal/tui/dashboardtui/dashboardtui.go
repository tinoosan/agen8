// Package dashboardtui provides a standalone, full-screen Bubble Tea TUI for
// per-agent session dashboarding. It is designed for tmux pane composition
// alongside the monitor, mail, and activity TUIs.
package dashboardtui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/tinoosan/agen8/pkg/protocol"
)

// Model is the Bubble Tea model for the full-screen dashboard TUI.
type Model struct {
	endpoint  string
	sessionID string
	width     int
	height    int

	connected bool
	lastErr   string

	agents      []agentRow
	stats       sessionStats
	sessionMode string
	teamID      string
	runID       string

	sel          int
	detailOpen   bool
	detailScroll int
	spinFrame    int
}

// Run launches the full-screen dashboard TUI.
func Run(endpoint, sessionID string) error {
	if endpoint == "" {
		endpoint = protocol.DefaultRPCEndpoint
	}
	m := &Model{
		endpoint:    endpoint,
		sessionID:   sessionID,
		connected:   true,
		sessionMode: "standalone",
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		fetchDataCmd(m.endpoint, m.sessionID),
		tickCmd(),
	)
}

func (m *Model) selectedAgent() *agentRow {
	if m.sel < 0 || m.sel >= len(m.agents) {
		return nil
	}
	return &m.agents[m.sel]
}
