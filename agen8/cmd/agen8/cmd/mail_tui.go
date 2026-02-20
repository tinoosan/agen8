package cmd

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/tinoosan/agen8/pkg/protocol"
)

type mailViewLoadedMsg struct {
	view      string
	tasks     []protocol.Task
	total     int
	err       error
	connected bool
}

type mailTickMsg struct{}

type mailModel struct {
	sessionID string
	view      string
	tasks     []protocol.Task
	total     int
	selected  int
	connected bool
	lastErr   string
}

func runMailTUI(cmd *cobra.Command) error {
	sessionID := strings.TrimSpace(mailWatchSessionID)
	if sessionID == "" {
		projectCtx, err := loadProjectContext()
		if err == nil && projectCtx.Exists {
			sessionID = strings.TrimSpace(projectCtx.State.ActiveSessionID)
		}
	}
	if sessionID == "" {
		return fmt.Errorf("session id is required (use --session-id or initialize project and attach a session)")
	}
	m := mailModel{
		sessionID: sessionID,
		view:      "inbox",
		connected: true,
	}
	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
}

func (m mailModel) Init() tea.Cmd {
	return tea.Batch(fetchMailViewCmd(m.sessionID, m.view), mailTickCmd())
}

func (m mailModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case mailTickMsg:
		return m, tea.Batch(fetchMailViewCmd(m.sessionID, m.view), mailTickCmd())
	case mailViewLoadedMsg:
		if msg.err != nil {
			m.connected = false
			m.lastErr = msg.err.Error()
			return m, nil
		}
		m.connected = msg.connected
		m.lastErr = ""
		m.tasks = msg.tasks
		m.total = msg.total
		if m.selected >= len(m.tasks) {
			m.selected = max(0, len(m.tasks)-1)
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			if m.view == "inbox" {
				m.view = "outbox"
			} else {
				m.view = "inbox"
			}
			m.selected = 0
			return m, fetchMailViewCmd(m.sessionID, m.view)
		case "j", "down":
			if m.selected < len(m.tasks)-1 {
				m.selected++
			}
		case "k", "up":
			if m.selected > 0 {
				m.selected--
			}
		case "r":
			return m, fetchMailViewCmd(m.sessionID, m.view)
		}
	}
	return m, nil
}

func (m mailModel) View() string {
	var b strings.Builder
	status := "connected"
	if !m.connected {
		status = "reconnecting"
	}
	fmt.Fprintf(&b, "Mail [%s]  session=%s  total=%d  %s\n", strings.ToUpper(m.view), m.sessionID, m.total, status)
	if m.lastErr != "" {
		fmt.Fprintf(&b, "error: %s\n", m.lastErr)
	}
	b.WriteString("TAB switch inbox/outbox | j/k move | r refresh | q quit\n\n")
	if len(m.tasks) == 0 {
		b.WriteString("(no tasks)\n")
		return b.String()
	}
	for i, task := range m.tasks {
		prefix := "  "
		if i == m.selected {
			prefix = "> "
		}
		assignee := strings.TrimSpace(task.AssignedRole)
		if assignee == "" {
			assignee = strings.TrimSpace(task.AssignedTo)
		}
		fmt.Fprintf(&b, "%s%s  %s  %s\n", prefix, task.ID, strings.TrimSpace(task.Status), fallback(assignee, "-"))
	}
	b.WriteString("\n")
	t := m.tasks[m.selected]
	fmt.Fprintf(&b, "Task %s\n", t.ID)
	fmt.Fprintf(&b, "Run: %s  Status: %s\n", t.RunID, t.Status)
	if a := strings.TrimSpace(t.AssignedRole); a != "" {
		fmt.Fprintf(&b, "Role: %s\n", a)
	}
	if s := strings.TrimSpace(t.Summary); s != "" {
		fmt.Fprintf(&b, "Summary: %s\n", s)
	}
	if e := strings.TrimSpace(t.Error); e != "" {
		fmt.Fprintf(&b, "Error: %s\n", e)
	}
	return b.String()
}

func fetchMailViewCmd(sessionID, view string) tea.Cmd {
	return func() tea.Msg {
		var out protocol.TaskListResult
		err := rpcCall(nil, protocol.MethodTaskList, protocol.TaskListParams{
			ThreadID: protocol.ThreadID(strings.TrimSpace(sessionID)),
			View:     strings.TrimSpace(view),
			Limit:    200,
			Offset:   0,
		}, &out)
		if err != nil {
			return mailViewLoadedMsg{view: view, err: err, connected: false}
		}
		return mailViewLoadedMsg{
			view:      view,
			tasks:     out.Tasks,
			total:     out.TotalCount,
			connected: true,
		}
	}
}

func mailTickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return mailTickMsg{}
	})
}
