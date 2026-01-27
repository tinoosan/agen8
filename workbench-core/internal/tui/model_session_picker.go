package tui

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/workbench-core/internal/tui/kit"
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
	m.sessionPickerOpen = true
	m.sessionPickerErr = ""

	l := list.New(nil, newSessionPickerDelegate(), 0, 0)
	l.Title = "Select Session"
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(true)
	l.SetFilteringEnabled(true)
	l.SetShowFilter(true)
	l.SetFilterText("")
	l.SetFilterState(list.Filtering)
	l.Styles.Title = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#707070")).
		Bold(true)

	m.sessionPickerList = l
	m.layout()

	return m.fetchSessionsList()
}

func (m *Model) closeSessionPicker() {
	m.sessionPickerOpen = false
	m.sessionPickerList = list.Model{}
	m.sessionPickerErr = ""
}

func (m *Model) fetchSessionsList() tea.Cmd {
	return func() tea.Msg {
		sessions, err := m.runner.ListSessions(m.ctx)
		return sessionsListMsg{sessions: sessions, err: err}
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
	modalWidth := 80
	if modalWidth > m.width-8 {
		modalWidth = m.width - 8
	}
	if modalWidth < 48 {
		modalWidth = 48
	}
	modalHeight := 22
	if modalHeight > m.height-8 {
		modalHeight = m.height - 8
	}
	if modalHeight < 12 {
		modalHeight = 12
	}

	listHeight := modalHeight - 3
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

func sessionsToPickerItems(sessions []types.Session) []list.Item {
	out := make([]list.Item, 0, len(sessions))
	sorted := make([]types.Session, 0, len(sessions))
	sorted = append(sorted, sessions...)
	sort.Slice(sorted, func(i, j int) bool {
		return sessionSortTime(sorted[i]).After(sessionSortTime(sorted[j]))
	})
	for _, s := range sorted {
		out = append(out, sessionPickerItem{
			id:        strings.TrimSpace(s.SessionID),
			title:     strings.TrimSpace(s.Title),
			goal:      strings.TrimSpace(s.CurrentGoal),
			updatedAt: formatSessionTime(s),
		})
	}
	return out
}

func sessionSortTime(s types.Session) time.Time {
	if s.UpdatedAt != nil {
		return *s.UpdatedAt
	}
	if s.CreatedAt != nil {
		return *s.CreatedAt
	}
	return time.Time{}
}

func formatSessionTime(s types.Session) string {
	t := sessionSortTime(s)
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04")
}
