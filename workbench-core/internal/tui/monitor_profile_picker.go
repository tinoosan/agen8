package tui

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/workbench-core/internal/tui/kit"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/profile"
)

type monitorProfilePickerItem struct {
	ref         string
	id          string
	description string
}

func (p monitorProfilePickerItem) FilterValue() string {
	// Allow filtering by either ID or description (and directory ref).
	return strings.TrimSpace(p.ref + " " + p.id + " " + p.description)
}

func (p monitorProfilePickerItem) Title() string {
	title := strings.TrimSpace(p.id)
	if title == "" {
		title = strings.TrimSpace(p.ref)
	}
	return title
}

func (p monitorProfilePickerItem) Description() string {
	desc := strings.TrimSpace(p.description)
	if desc == "" {
		return ""
	}
	// Include ref when it differs from the profile ID.
	ref := strings.TrimSpace(p.ref)
	id := strings.TrimSpace(p.id)
	if ref != "" && id != "" && !strings.EqualFold(ref, id) {
		return desc + " (" + ref + ")"
	}
	return desc
}

type monitorProfilePickerDelegate struct {
	styleRow lipgloss.Style
	styleSel lipgloss.Style
	styleDim lipgloss.Style
}

func newMonitorProfilePickerDelegate() monitorProfilePickerDelegate {
	return monitorProfilePickerDelegate{
		styleRow: lipgloss.NewStyle().Foreground(lipgloss.Color("#b0b0b0")),
		styleSel: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#eaeaea")).
			Bold(true),
		styleDim: lipgloss.NewStyle().Foreground(lipgloss.Color("#707070")),
	}
}

func (d monitorProfilePickerDelegate) Height() int  { return 1 }
func (d monitorProfilePickerDelegate) Spacing() int { return 0 }
func (d monitorProfilePickerDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}

func (d monitorProfilePickerDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	it, ok := item.(monitorProfilePickerItem)
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

	title := it.Title()
	desc := strings.TrimSpace(it.Description())
	line := title
	if desc != "" {
		line = title + " — " + desc
	}

	maxW := max(1, m.Width()-lipgloss.Width(prefix))
	line = kit.TruncateRight(line, maxW)
	_, _ = fmt.Fprint(w, style.Render(prefix+line))
}

func (m *monitorModel) openProfilePicker() tea.Cmd {
	return m.openProfilePickerFor("switch", false)
}

func (m *monitorModel) openProfilePickerFor(mode string, teamOnly bool) tea.Cmd {
	// Close other modals/pickers; only one should be open at a time.
	m.closeHelpModal()
	m.closeModelPicker()
	m.closeReasoningEffortPicker()
	m.closeReasoningSummaryPicker()
	m.closeFilePicker()

	m.profilePickerOpen = true
	m.profilePickerMode = strings.TrimSpace(mode)
	m.profilePickerTeamOnly = teamOnly

	items, titleSuffix := m.monitorProfilePickerItems(teamOnly)
	l := list.New(items, newMonitorProfilePickerDelegate(), 0, 0)
	l.Title = "Select Profile"
	if teamOnly {
		l.Title = "Select Team Profile"
	}
	if strings.TrimSpace(titleSuffix) != "" {
		l.Title += " " + strings.TrimSpace(titleSuffix)
	}
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(true)
	l.SetFilteringEnabled(true)
	l.SetShowFilter(true)
	// Keep initial state unfiltered so arrow navigation works immediately.
	l.SetFilterText("")
	l.SetFilterState(list.Unfiltered)
	l.Styles.Title = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#707070")).
		Bold(true)

	if len(items) > 0 {
		l.Select(0)
	}
	m.profilePickerList = l
	return nil
}

func (m *monitorModel) monitorProfilePickerItems(teamOnly bool) ([]list.Item, string) {
	profilesDir := fsutil.GetProfilesDir(m.cfg.DataDir)
	ents, err := os.ReadDir(profilesDir)
	if err != nil {
		return nil, "(no profiles)"
	}

	var items []monitorProfilePickerItem
	for _, ent := range ents {
		if !ent.IsDir() {
			continue
		}
		ref := strings.TrimSpace(ent.Name())
		if ref == "" {
			continue
		}

		dir := filepath.Join(profilesDir, ref)
		// Only include profiles that look like profile directories.
		if _, err := os.Stat(filepath.Join(dir, "profile.yaml")); err != nil {
			continue
		}

		p, err := profile.Load(dir)
		if err != nil || p == nil {
			// Skip invalid profiles rather than breaking the picker.
			continue
		}
		if teamOnly && p.Team == nil {
			continue
		}
		items = append(items, monitorProfilePickerItem{
			ref:         ref,
			id:          strings.TrimSpace(p.ID),
			description: strings.TrimSpace(p.Description),
		})
	}

	sort.Slice(items, func(i, j int) bool {
		a := items[i].Title()
		b := items[j].Title()
		return strings.ToLower(a) < strings.ToLower(b)
	})

	out := make([]list.Item, 0, len(items))
	for _, it := range items {
		out = append(out, it)
	}
	if len(out) == 0 {
		return nil, "(no profiles)"
	}
	return out, ""
}

func (m *monitorModel) closeProfilePicker() {
	m.profilePickerOpen = false
	m.profilePickerList = list.Model{}
	m.profilePickerMode = ""
	m.profilePickerTeamOnly = false
}

func (m *monitorModel) updateProfilePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		m.profilePickerList.CursorUp()
		return m, nil
	case tea.KeyDown:
		m.profilePickerList.CursorDown()
		return m, nil
	}

	switch msg.String() {
	case "esc", "escape":
		m.closeProfilePicker()
		return m, nil
	case "enter":
		return m, m.selectProfileFromPicker()
	}

	if msg.Type == tea.KeyRunes || msg.Type == tea.KeyBackspace || msg.Type == tea.KeyDelete {
		if m.profilePickerList.FilteringEnabled() {
			m.profilePickerList.SetFilterState(list.Filtering)
		}
	}

	var cmd tea.Cmd
	m.profilePickerList, cmd = m.profilePickerList.Update(msg)
	if m.profilePickerList.FilteringEnabled() && m.profilePickerList.FilterState() == list.Filtering {
		m.profilePickerList.SetFilterText(m.profilePickerList.FilterInput.Value())
	}
	return m, cmd
}

func (m *monitorModel) selectProfileFromPicker() tea.Cmd {
	if m.profilePickerList.Items() == nil || len(m.profilePickerList.Items()) == 0 {
		return nil
	}
	selectedItem := m.profilePickerList.SelectedItem()
	if selectedItem == nil {
		return nil
	}
	item, ok := selectedItem.(monitorProfilePickerItem)
	if !ok {
		return nil
	}

	ref := strings.TrimSpace(item.ref)
	id := strings.TrimSpace(item.id)
	if ref == "" {
		return nil
	}

	// Optimistically update the profile label.
	if id != "" {
		m.profile = id
	} else {
		m.profile = ref
	}
	mode := strings.TrimSpace(m.profilePickerMode)
	m.closeProfilePicker()
	if strings.EqualFold(mode, "new-team") {
		return m.startNewTeamSession(ref, "")
	}
	if strings.EqualFold(mode, "new-standalone") {
		return m.startNewStandaloneSession(ref, "")
	}
	return func() tea.Msg {
		return commandLinesMsg{lines: []string{"[command] profile switching is disabled; use /new"}}
	}
}

func (m *monitorModel) renderProfilePicker(base string) string {
	maxModalW := max(1, m.width-8)
	modalWidth := min(70, maxModalW)
	minModalW := min(40, maxModalW)
	if modalWidth < minModalW {
		modalWidth = minModalW
	}

	maxModalH := max(1, m.height-8)
	modalHeight := min(20, maxModalH)
	minModalH := min(10, maxModalH)
	if modalHeight < minModalH {
		modalHeight = minModalH
	}

	listHeight := modalHeight - 3 // Account for filter input and borders
	if listHeight < 4 {
		listHeight = 4
	}
	m.profilePickerList.SetWidth(modalWidth - 4)
	m.profilePickerList.SetHeight(listHeight)

	content := m.profilePickerList.View()

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
