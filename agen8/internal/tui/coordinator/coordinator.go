// Package coordinator provides a standalone, full-screen Bubble Tea chat-style
// TUI for interacting with the session coordinator.
package coordinator

import (
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/tinoosan/agen8/pkg/protocol"
)

const (
	feedbackInfo = iota
	feedbackOK
	feedbackErr
)

// turnKind classifies a conversation turn.
type turnKind int

const (
	turnUser turnKind = iota
	turnAgent
	turnSystem
)

// conversationTurn groups consecutive feed entries into a visual block.
type conversationTurn struct {
	kind      turnKind
	role      string      // label for the block ("You", agent role, "system")
	timestamp time.Time   // most recent timestamp in the group
	text      string      // for user/system turns
	entries   []feedEntry // for agent turns — grouped ops
}

// Model is the Bubble Tea model for the coordinator chat UI.
type Model struct {
	endpoint  string
	sessionID string
	width     int
	height    int

	connected bool
	lastErr   string

	feed       []feedEntry
	feedScroll int
	liveFollow bool

	sessionMode     string
	teamID          string
	runID           string
	coordinatorRole string

	input         textinput.Model
	spinFrame     int
	scrollPercent float64

	feedback     string
	feedbackKind int
	feedbackAt   time.Time
}

// Run launches the full-screen coordinator TUI.
func Run(endpoint, sessionID string) error {
	if endpoint == "" {
		endpoint = protocol.DefaultRPCEndpoint
	}
	in := textinput.New()
	in.Prompt = "> "
	in.Placeholder = "type a goal or /command..."
	in.Focus()
	in.CharLimit = 0

	m := &Model{
		endpoint:    endpoint,
		sessionID:   sessionID,
		connected:   true,
		sessionMode: "standalone",
		liveFollow:  true,
		input:       in,
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		fetchSessionCmd(m.endpoint, m.sessionID),
		fetchActivityCmd(m.endpoint, m.sessionID),
		tickCmd(),
		textinput.Blink,
	)
}
