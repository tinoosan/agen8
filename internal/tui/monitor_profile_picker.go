package tui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/agen8/internal/tui/kit"
	"github.com/tinoosan/agen8/pkg/fsutil"
	"github.com/tinoosan/agen8/pkg/profile"
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

func renderMonitorProfilePickerLine(item list.Item, maxWidth int) string {
	it, ok := item.(monitorProfilePickerItem)
	if !ok {
		return kit.TruncateRight(strings.TrimSpace(item.FilterValue()), maxWidth)
	}
	line := strings.TrimSpace(it.Title())
	if desc := strings.TrimSpace(it.Description()); desc != "" {
		line += " — " + desc
	}
	return kit.TruncateRight(line, maxWidth)
}

func (m *monitorModel) openProfilePicker() tea.Cmd {
	return m.openProfilePickerFor("switch", false)
}

func (m *monitorModel) openProfilePickerFor(mode string, teamOnly bool) tea.Cmd {
	// Close other modals/pickers; only one should be open at a time.
	m.closeHelpModal()
	m.closeAllPickers()

	m.profilePickerOpen = true
	m.profilePickerMode = strings.TrimSpace(mode)
	m.profilePickerTeamOnly = teamOnly

	standaloneOnly := strings.EqualFold(strings.TrimSpace(mode), "new-standalone")
	items, titleSuffix := m.monitorProfilePickerItems(teamOnly, standaloneOnly)
	l := list.New(items, kit.NewPickerDelegate(kit.DefaultPickerDelegateStyles(), renderMonitorProfilePickerLine), 0, 0)
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

func (m *monitorModel) monitorProfilePickerItems(teamOnly, standaloneOnly bool) ([]list.Item, string) {
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
		if standaloneOnly && p.Team != nil {
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
	dims := kit.ComputeModalDims(m.width, m.height, 70, 20, 40, 10, 8, 3)
	m.profilePickerList.SetWidth(dims.ModalWidth - 4)
	m.profilePickerList.SetHeight(dims.ListHeight)

	content := m.profilePickerList.View()

	opts := kit.DefaultPickerModalOpts(content, m.width, m.height, dims.ModalWidth, dims.ModalHeight)

	_ = base
	return kit.RenderOverlay(opts)
}
