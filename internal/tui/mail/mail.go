package mail

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tinoosan/agen8/internal/tui/adapter"
	"github.com/tinoosan/agen8/pkg/protocol"
)

type panel int

const (
	panelInbox panel = iota
	panelOutbox
)

type Options struct {
	ProjectRoot        string
	FollowProjectState bool
}

// Model is the Bubble Tea model for the full-screen mail TUI.
type Model struct {
	endpoint           string
	sessionID          string
	width              int
	height             int
	projectRoot        string
	followProjectState bool

	connected   bool
	lastErr     string
	notice      string
	noticeAt    time.Time
	currentTask *taskEntry
	inbox       []taskEntry
	outbox      []taskEntry

	focus     panel
	inboxSel  int
	outboxSel int

	detailOpen bool
}

// Run launches the full-screen mail TUI.
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
		focus:              panelInbox,
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

func (m *Model) selectedTask() *taskEntry {
	switch m.focus {
	case panelInbox:
		if m.inboxSel >= 0 && m.inboxSel < len(m.inbox) {
			return &m.inbox[m.inboxSel]
		}
	case panelOutbox:
		if m.outboxSel >= 0 && m.outboxSel < len(m.outbox) {
			return &m.outbox[m.outboxSel]
		}
	}
	return nil
}

// outboxScrollOffset returns the line offset for the outbox viewport.
// Each outbox entry can take 1-3 lines, so we estimate.
func (m *Model) outboxScrollOffset() int {
	if len(m.outbox) == 0 {
		return 0
	}
	offset := 0
	for i := 0; i < m.outboxSel && i < len(m.outbox); i++ {
		offset++ // header line
		r := m.outbox[i]
		if r.Error != "" && (r.Status == "failed" || r.Status == "canceled") {
			offset++
		}
		if r.TotalTokens > 0 {
			offset++
		}
	}
	return offset
}
