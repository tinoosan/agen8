package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/tui/kit"
	"github.com/tinoosan/workbench-core/pkg/timeutil"
	"github.com/tinoosan/workbench-core/pkg/types"
)

type sessionPickerItem struct {
	id        string
	title     string
	goal      string
	updatedAt string
}

func (s sessionPickerItem) FilterValue() string {
	return strings.TrimSpace(strings.Join([]string{s.id, s.title, s.goal}, " "))
}
func (s sessionPickerItem) Title() string       { return s.id }
func (s sessionPickerItem) Description() string { return "" }

type sessionPickerDelegate struct {
	styleRow     lipgloss.Style
	styleSel     lipgloss.Style
	styleMetaDim lipgloss.Style
}

func newSessionPickerDelegate() sessionPickerDelegate {
	return sessionPickerDelegate{
		styleRow: lipgloss.NewStyle().Foreground(lipgloss.Color("#b0b0b0")),
		styleSel: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#eaeaea")).
			Bold(true),
		styleMetaDim: lipgloss.NewStyle().Foreground(lipgloss.Color("#7a7a7a")),
	}
}

func (d sessionPickerDelegate) Height() int  { return 1 }
func (d sessionPickerDelegate) Spacing() int { return 0 }
func (d sessionPickerDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}

func (d sessionPickerDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	it, ok := item.(sessionPickerItem)
	if !ok {
		return
	}

	isSel := index == m.Index()
	prefix := "  "
	style := d.styleRow
	if isSel {
		prefix = "› "
		style = d.styleSel
	}

	title := strings.TrimSpace(it.title)
	if title == "" {
		title = it.id
	}
	meta := it.id
	if strings.TrimSpace(it.updatedAt) != "" {
		meta = it.updatedAt
	}
	line := title
	if it.id != "" && title != it.id {
		line += " (" + it.id + ")"
	}
	if meta != "" {
		line += " • " + meta
	}

	maxW := max(1, m.Width()-lipgloss.Width(prefix))
	line = kit.TruncateRight(line, maxW)

	if isSel {
		_, _ = fmt.Fprint(w, style.Render(prefix+line))
		return
	}

	// Dim the metadata suffix when not selected.
	metaIdx := strings.LastIndex(line, " • ")
	if metaIdx <= 0 {
		_, _ = fmt.Fprint(w, style.Render(prefix+line))
		return
	}
	main := line[:metaIdx]
	metaPart := line[metaIdx:]
	_, _ = fmt.Fprint(w, style.Render(prefix+main)+d.styleMetaDim.Render(metaPart))
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

	l := list.New(nil, newSessionPickerDelegate(), 0, 0)
	l.Title = "Select Session"
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	l.SetFilteringEnabled(true)
	l.SetShowFilter(true)
	l.SetFilterText("")
	l.SetFilterState(list.Filtering)
	// Disable client-side filtering; we use FilterInput as a query for server-side search.
	l.Filter = func(_ string, targets []string) []list.Rank {
		ranks := make([]list.Rank, len(targets))
		for i := range targets {
			ranks[i] = list.Rank{Index: i}
		}
		return ranks
	}
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
		filter := store.SessionFilter{
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
	maxModalW := max(1, m.width-8)
	modalWidth := min(80, maxModalW)
	minModalW := min(48, maxModalW)
	if modalWidth < minModalW {
		modalWidth = minModalW
	}

	maxModalH := max(1, m.height-8)
	modalHeight := min(22, maxModalH)
	minModalH := min(12, maxModalH)
	if modalHeight < minModalH {
		modalHeight = minModalH
	}

	listHeight := modalHeight - 4
	if listHeight < 4 {
		listHeight = 4
	}
	m.sessionPickerList.SetWidth(modalWidth - 4)
	m.sessionPickerList.SetHeight(listHeight)

	content := m.sessionPickerList.View()
	if strings.TrimSpace(m.sessionPickerErr) != "" {
		errLine := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff8080")).Render("Error: " + m.sessionPickerErr)
		content = errLine + "\n\n" + content
	}
	content += "\n" + m.renderSessionPickerFooter()

	opts := kit.ModalOptions{
		Content:      content,
		ScreenWidth:  m.width,
		ScreenHeight: m.height,
		Width:        modalWidth,
		Height:       modalHeight,
		Padding:      [2]int{1, 2},
		BorderStyle:  lipgloss.RoundedBorder(),
		BorderColor:  lipgloss.Color("#6bbcff"),
		Foreground:   lipgloss.Color("#eaeaea"),
	}

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
		out = append(out, sessionPickerItem{
			id:        strings.TrimSpace(s.SessionID),
			title:     strings.TrimSpace(s.Title),
			goal:      strings.TrimSpace(s.CurrentGoal),
			updatedAt: formatSessionTime(s),
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
