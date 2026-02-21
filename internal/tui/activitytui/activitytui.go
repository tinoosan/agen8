// Package activitytui provides a standalone, full-screen Bubble Tea TUI for
// browsing agent activities. It is designed for tmux pane composition alongside
// the monitor and mail TUIs.
package activitytui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tinoosan/agen8/pkg/protocol"
	"github.com/tinoosan/agen8/pkg/types"
)

type Options struct {
	ProjectRoot        string
	FollowProjectState bool
}

// Model is the Bubble Tea model for the full-screen activity TUI.
type Model struct {
	endpoint  string
	sessionID string
	width     int
	height    int
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
}

// Run launches the full-screen activity TUI.
func Run(endpoint, sessionID string, opts Options) error {
	if endpoint == "" {
		endpoint = protocol.DefaultRPCEndpoint
	}
	m := &Model{
		endpoint:          endpoint,
		sessionID:         sessionID,
		projectRoot:       opts.ProjectRoot,
		followProjectState: opts.FollowProjectState,
		connected:         true,
		liveFollow:        true,
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
