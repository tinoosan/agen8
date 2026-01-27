package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/workbench-core/internal/tui/kit"
)

type skillPickerItem struct {
	name        string
	description string
	value       string
	isNone      bool
}

func (s skillPickerItem) Title() string       { return s.name }
func (s skillPickerItem) Description() string { return s.description }
func (s skillPickerItem) FilterValue() string {
	base := strings.TrimSpace(s.name + " " + s.description)
	if s.isNone {
		base = strings.TrimSpace(base + " none off disable")
	}
	return base
}

type skillPickerDelegate struct {
	styleRowTitle lipgloss.Style
	styleRowDesc  lipgloss.Style
	styleSelTitle lipgloss.Style
	styleSelDesc  lipgloss.Style
}

func newSkillPickerDelegate() skillPickerDelegate {
	return skillPickerDelegate{
		styleRowTitle: lipgloss.NewStyle().Foreground(lipgloss.Color("#b0b0b0")).Bold(true),
		styleRowDesc:  lipgloss.NewStyle().Foreground(lipgloss.Color("#707070")),
		styleSelTitle: lipgloss.NewStyle().Foreground(lipgloss.Color("#eaeaea")).Bold(true),
		styleSelDesc:  lipgloss.NewStyle().Foreground(lipgloss.Color("#9ad0ff")),
	}
}

func (d skillPickerDelegate) Height() int  { return 2 }
func (d skillPickerDelegate) Spacing() int { return 0 }
func (d skillPickerDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}

func (d skillPickerDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	it, ok := item.(skillPickerItem)
	if !ok {
		return
	}

	isSel := index == m.Index()
	prefix := "  "
	titleStyle := d.styleRowTitle
	descStyle := d.styleRowDesc
	if isSel {
		prefix = "› "
		titleStyle = d.styleSelTitle
		descStyle = d.styleSelDesc
	}

	maxW := max(1, m.Width()-lipgloss.Width(prefix))
	title := kit.TruncateRight(it.name, maxW)
	desc := kit.TruncateRight(it.description, maxW)

	_, _ = fmt.Fprint(w, titleStyle.Render(prefix+title))
	if strings.TrimSpace(desc) != "" {
		_, _ = fmt.Fprint(w, "\n"+descStyle.Render(prefix+desc))
	} else {
		_, _ = fmt.Fprint(w, "\n"+descStyle.Render(prefix+""))
	}
}

func (m *Model) openSkillPicker() {
	m.skillPickerOpen = true
	m.commandPaletteOpen = false
	m.commandPaletteMatches = nil
	m.commandPaletteSelected = 0

	items, sel, titleSuffix := m.loadSkillPickerItems()

	l := list.New(items, newSkillPickerDelegate(), 0, 0)
	l.Title = "Select Skill"
	if titleSuffix != "" {
		l.Title += " " + titleSuffix
	}
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

	if len(items) > 0 {
		if sel < 0 {
			sel = 0
		}
		if sel >= len(items) {
			sel = len(items) - 1
		}
		l.Select(sel)
	}

	m.skillPickerList = l
	m.layout()
}

func (m *Model) closeSkillPicker() {
	m.skillPickerOpen = false
	m.skillPickerList = list.Model{}
	m.layout()
}

func (m *Model) selectSkillFromPicker() tea.Cmd {
	if !m.skillPickerOpen || len(m.skillPickerList.Items()) == 0 {
		return nil
	}
	selected := m.skillPickerList.SelectedItem()
	item, ok := selected.(skillPickerItem)
	if !ok {
		return nil
	}

	val := strings.TrimSpace(item.value)
	if item.isNone || val == "" {
		val = "none"
		m.selectedSkill = ""
	} else {
		m.selectedSkill = val
	}

	m.closeSkillPicker()

	return func() tea.Msg {
		final, err := m.runner.RunTurn(m.ctx, "/skill "+val)
		return turnDoneMsg{final: final, err: err, preserveScroll: true}
	}
}

func (m *Model) loadSkillPickerItems() ([]list.Item, int, string) {
	items := make([]list.Item, 0, 16)
	items = append(items, skillPickerItem{
		name:        "None",
		description: "Deactivate active skill",
		value:       "none",
		isNone:      true,
	})

	lister, ok := m.runner.(vfsLister)
	if !ok {
		return items, 0, "(skills unavailable)"
	}

	entries, err := lister.ListVFS(m.ctx, "/skills")
	if err != nil {
		return items, 0, "(skills unavailable)"
	}

	for _, entry := range entries {
		if !entry.IsDir {
			continue
		}
		dir := strings.TrimSpace(strings.TrimPrefix(entry.Path, "/skills/"))
		if dir == "" || dir == "/" {
			continue
		}
		name, desc := m.readSkillMeta(dir)
		if name == "" {
			name = dir
		}
		items = append(items, skillPickerItem{
			name:        name,
			description: desc,
			value:       dir,
		})
	}

	sel := 0
	cur := strings.TrimSpace(m.selectedSkill)
	if cur != "" {
		for i, it := range items {
			if s, ok := it.(skillPickerItem); ok && s.value == cur {
				sel = i
				break
			}
		}
	}

	return items, sel, ""
}

func (m *Model) readSkillMeta(dir string) (string, string) {
	acc, ok := m.runner.(vfsAccessor)
	if !ok {
		return "", ""
	}
	path := "/skills/" + strings.TrimPrefix(dir, "/") + "/SKILL.md"
	txt, _, _, err := acc.ReadVFS(m.ctx, path, 64*1024)
	if err != nil {
		return "", ""
	}
	return parseSkillFrontMatter(txt)
}

func parseSkillFrontMatter(content string) (string, string) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", ""
	}
	name := ""
	desc := ""
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "---" {
			break
		}
		if strings.HasPrefix(line, "name:") {
			name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
		}
		if strings.HasPrefix(line, "description:") {
			desc = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
		}
	}
	return name, desc
}
