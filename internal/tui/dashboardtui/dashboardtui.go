// Package dashboardtui provides a standalone, full-screen Bubble Tea TUI for
// per-agent session dashboarding. It is designed for tmux pane composition
// alongside the monitor, mail, and activity TUIs.
package dashboardtui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tinoosan/agen8/internal/tui/adapter"
	"github.com/tinoosan/agen8/pkg/protocol"
)

type Options struct {
	ProjectRoot        string
	FollowProjectState bool
}

// Model is the Bubble Tea model for the full-screen dashboard TUI.
type Model struct {
	endpoint           string
	sessionID          string
	width              int
	height             int
	projectRoot        string
	followProjectState bool

	connected bool
	lastErr   string
	notice    string
	noticeAt  time.Time

	agents       []agentRow
	stats        sessionStats
	sessionMode  string
	teamID       string
	runID        string
	reviewerRole string

	sel          int
	detailOpen   bool
	detailScroll int
	spinFrame    int
}

// Run launches the full-screen dashboard TUI.
func Run(endpoint, sessionID string, opts Options) error {
	if endpoint == "" {
		endpoint = protocol.DefaultRPCEndpoint
	}
	m := &Model{
		endpoint:           endpoint,
		sessionID:          sessionID,
		projectRoot:        opts.ProjectRoot,
		followProjectState: opts.FollowProjectState,
		connected:          true,
		sessionMode:        "standalone",
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		fetchDataCmd(m.endpoint, m.sessionID),
		tickCmd(),
		adapter.StartNotificationListenerCmd(m.endpoint),
	)
}

func (m *Model) selectedAgent() *agentRow {
	if m.sel < 0 || m.sel >= len(m.agents) {
		return nil
	}
	return &m.agents[m.sel]
}
