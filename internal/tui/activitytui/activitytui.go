// Package activitytui provides a standalone, full-screen Bubble Tea TUI for
// browsing agent activities. It is designed for tmux pane composition alongside
// the monitor and mail TUIs.
package activitytui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tinoosan/agen8/internal/tui/adapter"
	"github.com/tinoosan/agen8/pkg/protocol"
	"github.com/tinoosan/agen8/pkg/types"
)

type viewMode int

const (
	viewProject viewMode = iota // top-level: list of teams
	viewTeam                    // drill-in: activity for one team
)

// Options configures the activity TUI.
type Options struct {
	ProjectRoot        string
	FollowProjectState bool
	SessionID          string // non-empty to scope to a specific session
	SessionExplicit    bool   // true when --session-id was explicitly passed
}

// Model is the Bubble Tea model for the full-screen activity TUI.
type Model struct {
	endpoint           string
	sessionID          string
	width              int
	height             int
	projectRoot        string
	followProjectState bool

	connected  bool
	lastErr    string
	notice     string
	noticeAt   time.Time
	activities []types.Activity
	totalCount int

	sel          int
	detailOpen   bool
	detailScroll int

	// spinFrame is the index into the braille spinner sequence for pending items.
	spinFrame int

	// liveFollow means auto-scroll to the bottom when new items arrive.
	// Disabled when user manually scrolls up.
	liveFollow bool

	// showTimestamps toggles timestamp display in the list view.
	showTimestamps bool

	// Project overview state.
	mode            viewMode
	teams           []teamRow
	teamSel         int
	selectedTeam    *teamRow
	projectID       string
	sessionExplicit bool
}

// Run launches the full-screen activity TUI.
func Run(endpoint string, opts Options) error {
	if endpoint == "" {
		endpoint = protocol.DefaultRPCEndpoint
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
		connected:          true,
		liveFollow:         true,
		mode:               mode,
		sessionExplicit:    opts.SessionExplicit,
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m *Model) Init() tea.Cmd {
	baseCmds := []tea.Cmd{
		tickCmd(),
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
