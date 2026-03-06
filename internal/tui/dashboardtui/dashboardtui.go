// Package dashboardtui provides a standalone, full-screen Bubble Tea TUI for
// project and team dashboarding. It is designed for tmux pane composition
// alongside the monitor, mail, and activity TUIs.
//
// The default top-level screen is a project overview listing all teams.
// Selecting a team drills into the per-agent workspace for that team's session.
package dashboardtui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tinoosan/agen8/internal/tui/adapter"
	"github.com/tinoosan/agen8/pkg/protocol"
)

type viewMode int

const (
	viewProject viewMode = iota // top-level: list of teams
	viewTeam                    // drill-in: per-agent dashboard for one team
)

// Options configures the dashboard TUI.
type Options struct {
	ProjectRoot        string
	FollowProjectState bool
	RefreshInterval    time.Duration
	SessionID          string // non-empty to scope to a specific session
	SessionExplicit    bool   // true when --session-id was explicitly passed
}

// Model is the Bubble Tea model for the full-screen dashboard TUI.
type Model struct {
	endpoint           string
	sessionID          string
	width              int
	height             int
	projectRoot        string
	followProjectState bool
	refreshInterval    time.Duration

	connected bool
	lastErr   string
	notice    string
	noticeAt  time.Time

	// Team workspace (per-agent) state.
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

	// Project overview state.
	mode            viewMode
	teams           []teamRow
	teamSel         int
	selectedTeam    *teamRow
	projectID       string
	sessionExplicit bool
}

// Run launches the full-screen dashboard TUI.
func Run(endpoint string, opts Options) error {
	if endpoint == "" {
		endpoint = protocol.DefaultRPCEndpoint
	}
	refreshInterval := opts.RefreshInterval
	if refreshInterval <= 0 {
		refreshInterval = 2 * time.Second
	}

	mode := viewProject
	sessionID := strings.TrimSpace(opts.SessionID)
	if opts.SessionExplicit && sessionID != "" {
		mode = viewTeam
	}

	m := &Model{
		endpoint:           endpoint,
		sessionID:          sessionID,
		projectRoot:        opts.ProjectRoot,
		followProjectState: opts.FollowProjectState,
		refreshInterval:    refreshInterval,
		connected:          true,
		sessionMode:        "team",
		mode:               mode,
		sessionExplicit:    opts.SessionExplicit,
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m *Model) Init() tea.Cmd {
	baseCmds := []tea.Cmd{
		tickCmd(m.refreshInterval),
		adapter.StartNotificationListenerCmd(m.endpoint),
	}
	switch m.mode {
	case viewProject:
		return tea.Batch(append(baseCmds, fetchProjectDataCmd(m.endpoint, m.projectRoot))...)
	case viewTeam:
		return tea.Batch(append(baseCmds, fetchDataCmd(m.endpoint, m.sessionID))...)
	}
	return tea.Batch(baseCmds...)
}

func (m *Model) selectedAgent() *agentRow {
	if m.sel < 0 || m.sel >= len(m.agents) {
		return nil
	}
	return &m.agents[m.sel]
}
