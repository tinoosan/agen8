package coordinator

import (
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tinoosan/agen8/internal/tui/rpcscope"
)

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.Width = maxInt(12, m.width-30)
		return m, nil

	case tickMsg:
		m.spinFrame = (m.spinFrame + 1) % len(spinnerFrames)
		if m.feedback != "" && time.Since(m.feedbackAt) > 3*time.Second {
			m.feedback = ""
		}
		return m, tea.Batch(
			fetchSessionCmd(m.endpoint, m.sessionID),
			fetchActivityCmd(m.endpoint, m.sessionID),
			tickCmd(),
		)

	case sessionLoadedMsg:
		if msg.err != nil {
			m.connected = false
			m.lastErr = msg.err.Error()
			m.setFeedback(msg.err.Error(), feedbackErr)
			return m, nil
		}
		m.connected = true
		m.lastErr = ""
		m.sessionMode = msg.sessionMode
		m.teamID = msg.teamID
		m.runID = msg.runID
		m.threadID = msg.threadID
		m.coordinatorRole = msg.coordinatorRole
		return m, nil

	case activityLoadedMsg:
		if msg.err != nil {
			m.connected = false
			m.lastErr = msg.err.Error()
			m.setFeedback(msg.err.Error(), feedbackErr)
			return m, nil
		}
		m.connected = true
		m.lastErr = ""
		m.mergeActivityEntries(msg.entries)
		return m, nil

	case goalSubmittedMsg:
		if msg.err != nil {
			m.setFeedback(msg.err.Error(), feedbackErr)
			return m, nil
		}
		m.applyRecoveredScope(msg.scope)
		m.feed = append(m.feed, feedEntry{
			kind:      feedUser,
			timestamp: time.Now(),
			text:      msg.goal,
		})
		m.appendReconnectNotice(msg.recovered)
		m.setFeedback("queued", feedbackOK)
		m.pinFeedToBottom()
		return m, nil

	case sessionActionMsg:
		if msg.err != nil {
			m.setFeedback(msg.err.Error(), feedbackErr)
			return m, nil
		}
		m.applyRecoveredScope(msg.scope)
		text := "Session " + msg.action + "d"
		if msg.action == "stop" {
			text = "Session stopped"
		}
		m.feed = append(m.feed, feedEntry{
			kind:      feedSystem,
			timestamp: time.Now(),
			text:      text,
		})
		m.appendReconnectNotice(msg.recovered)
		m.setFeedback(text, feedbackOK)
		m.pinFeedToBottom()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.input.SetValue("")
			return m, nil
		case "pgup":
			m.liveFollow = false
			m.feedScroll -= maxInt(1, m.feedHeight()/2)
			if m.feedScroll < 0 {
				m.feedScroll = 0
			}
			return m, nil
		case "pgdown":
			m.feedScroll += maxInt(1, m.feedHeight()/2)
			maxScroll := maxInt(0, m.totalFeedLines()-m.feedHeight())
			if m.feedScroll >= maxScroll {
				m.liveFollow = true
				m.feedScroll = maxScroll
			}
			return m, nil
		case "home", "g":
			m.liveFollow = false
			m.feedScroll = 0
			return m, nil
		case "end", "G":
			m.liveFollow = true
			m.pinFeedToBottom()
			return m, nil
		case "enter":
			line := strings.TrimSpace(m.input.Value())
			if line == "" {
				return m, nil
			}
			m.input.SetValue("")
			if strings.HasPrefix(line, "/") {
				return m, m.handleSlash(line)
			}
			return m, submitGoalCmd(m.endpoint, m.sessionID, m.teamID, m.runID, m.coordinatorRole, line)
		}
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *Model) handleSlash(line string) tea.Cmd {
	cmd := strings.ToLower(strings.TrimSpace(line))
	switch cmd {
	case "/pause":
		return sessionActionCmd(m.endpoint, m.sessionID, m.teamID, "pause")
	case "/resume":
		return sessionActionCmd(m.endpoint, m.sessionID, m.teamID, "resume")
	case "/stop":
		return sessionActionCmd(m.endpoint, m.sessionID, m.teamID, "stop")
	case "/help":
		m.setFeedback("commands: /pause /resume /stop /help /quit", feedbackInfo)
		return nil
	case "/quit":
		return tea.Quit
	default:
		m.setFeedback("unknown command: "+line, feedbackErr)
		return nil
	}
}

func (m *Model) setFeedback(msg string, kind int) {
	m.feedback = strings.TrimSpace(msg)
	m.feedbackKind = kind
	m.feedbackAt = time.Now()
}

func (m *Model) mergeActivityEntries(entries []feedEntry) {
	if len(entries) == 0 {
		return
	}
	others := make([]feedEntry, 0, len(m.feed))
	for _, e := range m.feed {
		if e.kind != feedAgent {
			others = append(others, e)
		}
	}
	merged := append(others, entries...)
	sort.SliceStable(merged, func(i, j int) bool {
		return merged[i].timestamp.Before(merged[j].timestamp)
	})
	m.feed = merged
	if m.liveFollow {
		m.pinFeedToBottom()
	}
}

func (m *Model) pinFeedToBottom() {
	m.liveFollow = true
	m.feedScroll = maxInt(0, m.totalFeedLines()-m.feedHeight())
}

func (m *Model) applyRecoveredScope(scope rpcscope.ScopeState) {
	if strings.TrimSpace(scope.SessionID) != "" {
		m.sessionID = strings.TrimSpace(scope.SessionID)
	}
	if strings.TrimSpace(scope.TeamID) != "" {
		m.teamID = strings.TrimSpace(scope.TeamID)
	}
	if strings.TrimSpace(scope.RunID) != "" {
		m.runID = strings.TrimSpace(scope.RunID)
	}
	if strings.TrimSpace(scope.ThreadID) != "" {
		m.threadID = strings.TrimSpace(scope.ThreadID)
	}
	if strings.TrimSpace(scope.CoordinatorRole) != "" {
		m.coordinatorRole = strings.TrimSpace(scope.CoordinatorRole)
	}
}

func (m *Model) appendReconnectNotice(recovered bool) {
	if !recovered {
		return
	}
	if time.Since(m.lastReconnectAt) < 5*time.Second {
		return
	}
	m.lastReconnectAt = time.Now()
	m.feed = append(m.feed, feedEntry{
		kind:      feedSystem,
		timestamp: m.lastReconnectAt,
		text:      "reconnected context",
	})
}

func (m *Model) feedHeight() int {
	h := m.height - 5 // header + separator + input(2 with border) + footer
	if h < 1 {
		return 1
	}
	return h
}
