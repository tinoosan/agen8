package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/tinoosan/agen8/internal/tui/kit"
	pkgstore "github.com/tinoosan/agen8/pkg/store"
	"github.com/tinoosan/agen8/pkg/timeutil"
	"github.com/tinoosan/agen8/pkg/types"
)

type sessionPickerItem struct {
	id            string
	title         string
	goal          string
	updatedAt     string
	mode          string
	teamID        string
	profile       string
	model         string
	current       string
	runningAgents int
	pausedAgents  int
	totalAgents   int
}

func (s sessionPickerItem) FilterValue() string {
	return strings.TrimSpace(strings.Join([]string{
		s.id, s.title, s.goal, s.mode, s.teamID, s.profile, s.model, s.current,
		fmt.Sprintf("%d", s.runningAgents), fmt.Sprintf("%d", s.pausedAgents), fmt.Sprintf("%d", s.totalAgents),
	}, " "))
}
func (s sessionPickerItem) Title() string       { return s.id }
func (s sessionPickerItem) Description() string { return "" }
func renderSessionPickerLine(item list.Item, maxWidth int) string {
	it, ok := item.(sessionPickerItem)
	if !ok {
		line := strings.TrimSpace(item.FilterValue())
		if line == "" {
			line = "(unknown session)"
		}
		return kit.TruncateRight(line, maxWidth)
	}

	title := strings.TrimSpace(it.title)
	if title == "" {
		title = it.id
	}
	if strings.TrimSpace(title) == "" {
		title = "(unknown session)"
	}
	meta := strings.TrimSpace(it.updatedAt)
	line := title
	if strings.TrimSpace(it.teamID) != "" {
		line += " · team"
	}
	if p := strings.TrimSpace(it.profile); p != "" {
		line += " · " + p
	}
	if m := strings.TrimSpace(it.model); m != "" {
		line += " · " + kit.TruncateMiddle(m, 24)
	}
	if it.totalAgents > 0 {
		line += fmt.Sprintf(" · %d running · %d paused · %d total", it.runningAgents, it.pausedAgents, it.totalAgents)
	}
	if t := strings.TrimSpace(it.teamID); t != "" {
		line += " · " + shortID(t)
	}
	if it.id != "" && !strings.EqualFold(strings.TrimSpace(title), strings.TrimSpace(it.id)) {
		line += " · " + shortID(it.id)
	}
	if meta != "" {
		line += " • " + meta
	}

	return kit.TruncateRight(line, maxWidth)
}

func newSessionPickerDelegate() list.ItemDelegate {
	return kit.NewPickerDelegate(kit.DefaultPickerDelegateStyles(), renderSessionPickerLine)
}

func (m *Model) openSessionPicker() tea.Cmd {
	if m.runtimeChangeLocked("switching sessions") {
		return nil
	}
	m.sessionPickerErr = ""
	m.sessionPickerCtrl.Open(kit.PickerConfig{
		Title:            "Select Session",
		FilteringEnabled: true,
		ShowFilter:       true,
		PageKeyNav:       true,
		PageSize:         50,
		Delegate:         newSessionPickerDelegate(),
		ModalWidth:       80,
		ModalHeight:      22,
		ModalMinWidth:    48,
		ModalMinHeight:   12,
		ModalMarginX:     8,
		ModalMarginY:     4,
	})
	m.syncSessionPickerState()
	m.layout()

	return m.fetchSessionsPage()
}

func (m *Model) closeSessionPicker() {
	m.sessionPickerCtrl.Close()
	m.syncSessionPickerState()
	m.sessionPickerErr = ""
}

func (m *Model) fetchSessionsPage() tea.Cmd {
	return func() tea.Msg {
		filter := pkgstore.SessionFilter{
			TitleContains: m.sessionPickerFilter,
			Limit:         m.sessionPickerPageSize,
			Offset:        m.sessionPickerPage * m.sessionPickerPageSize,
			SortBy:        "updated_at",
			SortDesc:      true,
		}

		total, err := m.runner.CountSessions(m.ctx, filter)
		if err != nil {
			return sessionsListMsg{err: err}
		}
		sessions, err := m.runner.ListSessionsPaginated(m.ctx, filter)
		if err != nil {
			return sessionsListMsg{err: err}
		}
		return sessionsListMsg{sessions: sessions, total: total, page: m.sessionPickerPage, err: nil}
	}
}

func (m *Model) selectSessionFromPicker() tea.Cmd {
	m.ensureSessionPickerCtrl()
	selectedItem := m.sessionPickerCtrl.SelectedItem()
	if selectedItem == nil {
		return nil
	}
	item, ok := selectedItem.(sessionPickerItem)
	if !ok {
		return nil
	}
	if strings.TrimSpace(item.id) == "" {
		return nil
	}

	m.closeSessionPicker()
	m.switchSessionID = item.id
	m.switchNew = false
	return tea.Quit
}

func (m Model) renderSessionPicker(base string) string {
	m.ensureSessionPickerCtrl()
	_ = base
	return m.sessionPickerCtrl.Render(m.width, m.height, m.renderSessionPickerFooter(), m.sessionPickerErr)
}

func (m *Model) renderSessionPickerFooter() string {
	return pickerFooter(m.sessionPickerTotal, m.sessionPickerPage, m.sessionPickerPageSize, strings.TrimSpace(m.sessionPickerErr), "No sessions", m.styleDim)
}

func sessionsToPickerItems(sessions []types.Session) []list.Item {
	out := make([]list.Item, 0, len(sessions))
	for _, s := range sessions {
		id := strings.TrimSpace(s.SessionID)
		title := strings.TrimSpace(s.Title)
		if title == "" {
			title = strings.TrimSpace(s.CurrentGoal)
		}
		if id == "" && title == "" {
			title = "(unknown session)"
		}
		out = append(out, sessionPickerItem{
			id:            id,
			title:         title,
			goal:          strings.TrimSpace(s.CurrentGoal),
			updatedAt:     formatSessionTime(s),
			mode:          strings.TrimSpace(s.Mode),
			teamID:        strings.TrimSpace(s.TeamID),
			profile:       strings.TrimSpace(s.Profile),
			model:         strings.TrimSpace(s.ActiveModel),
			current:       strings.TrimSpace(s.CurrentRunID),
			totalAgents:   len(s.Runs),
			runningAgents: 0,
			pausedAgents:  0,
		})
	}
	return out
}

func formatSessionTime(s types.Session) string {
	t := timeutil.FirstNonZero(s.UpdatedAt, s.CreatedAt)
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04")
}
