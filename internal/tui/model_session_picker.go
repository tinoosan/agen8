package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	pkgstore "github.com/tinoosan/agen8/pkg/store"
	"github.com/tinoosan/agen8/internal/tui/kit"
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
	mode := strings.ToLower(strings.TrimSpace(it.mode))
	switch mode {
	case "team":
		line += " · team"
	case "standalone":
		line += " · standalone"
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
	m.sessionPickerOpen = true
	m.sessionPickerErr = ""
	if m.sessionPickerPageSize == 0 {
		m.sessionPickerPageSize = 50
	}
	m.sessionPickerPage = 0
	m.sessionPickerTotal = 0
	m.sessionPickerFilter = ""

	l := list.New(nil, kit.NewPickerDelegate(kit.DefaultPickerDelegateStyles(), renderSessionPickerLine), 0, 0)
	l.Title = "Select Session"
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	l.SetFilteringEnabled(true)
	l.SetShowFilter(true)
	l.SetFilterText("")
	l.SetFilterState(list.Unfiltered)
	l.Styles.Title = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#707070")).
		Bold(true)

	m.sessionPickerList = l
	m.layout()

	return m.fetchSessionsPage()
}

func (m *Model) closeSessionPicker() {
	m.sessionPickerOpen = false
	m.sessionPickerList = list.Model{}
	m.sessionPickerErr = ""
	m.sessionPickerPage = 0
	m.sessionPickerTotal = 0
	m.sessionPickerFilter = ""
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
	if m.sessionPickerList.Items() == nil || len(m.sessionPickerList.Items()) == 0 {
		return nil
	}
	selectedItem := m.sessionPickerList.SelectedItem()
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
	dims := kit.ComputeModalDims(m.width, m.height, 80, 22, 48, 12, 8, 4)
	m.sessionPickerList.SetWidth(dims.ModalWidth - 4)
	m.sessionPickerList.SetHeight(dims.ListHeight)

	content := m.sessionPickerList.View()
	if strings.TrimSpace(m.sessionPickerErr) != "" {
		errLine := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff8080")).Render("Error: " + m.sessionPickerErr)
		content = errLine + "\n\n" + content
	}
	content += "\n" + m.renderSessionPickerFooter()

	opts := kit.DefaultPickerModalOpts(content, m.width, m.height, dims.ModalWidth, dims.ModalHeight)

	_ = base
	return kit.RenderOverlay(opts)
}

func (m *Model) renderSessionPickerFooter() string {
	if m.sessionPickerTotal == 0 {
		if strings.TrimSpace(m.sessionPickerErr) != "" {
			return m.styleDim.Render("Ctrl+N/P: page")
		}
		return m.styleDim.Render("No sessions")
	}

	pageSize := m.sessionPickerPageSize
	if pageSize <= 0 {
		pageSize = 50
	}
	maxPage := (m.sessionPickerTotal + pageSize - 1) / pageSize
	currentPage := m.sessionPickerPage + 1

	pageInfo := fmt.Sprintf("Page %d of %d (%d sessions)", currentPage, maxPage, m.sessionPickerTotal)
	return m.styleDim.Render(pageInfo + " • Ctrl+N/P: page")
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
