package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/workbench-core/internal/tui/kit"
	"github.com/tinoosan/workbench-core/pkg/protocol"
)

// modelPickerItem implements list.Item for the model picker.
type monitorModelPickerItem struct {
	id         string
	provider   string
	isProvider bool
	count      int
}

func (m monitorModelPickerItem) FilterValue() string {
	if m.isProvider {
		return m.provider
	}
	return m.id
}

func (m monitorModelPickerItem) Title() string {
	if m.isProvider {
		if m.count > 0 {
			return fmt.Sprintf("%s (%d)", m.provider, m.count)
		}
		return m.provider
	}
	return m.id
}

func (m monitorModelPickerItem) Description() string { return "" }

type monitorModelPickerDelegate struct {
	styleRow lipgloss.Style
	styleSel lipgloss.Style
}

func newMonitorModelPickerDelegate() monitorModelPickerDelegate {
	return monitorModelPickerDelegate{
		styleRow: lipgloss.NewStyle().Foreground(lipgloss.Color("#b0b0b0")),
		styleSel: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#eaeaea")).
			Bold(true),
	}
}

func (d monitorModelPickerDelegate) Height() int  { return 1 }
func (d monitorModelPickerDelegate) Spacing() int { return 0 }
func (d monitorModelPickerDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}

func (d monitorModelPickerDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	it, ok := item.(monitorModelPickerItem)
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

	line := it.Title()
	maxW := max(1, m.Width()-lipgloss.Width(prefix))
	line = kit.TruncateRight(line, maxW)
	_, _ = fmt.Fprint(w, style.Render(prefix+line))
}

// openModelPicker initializes and opens the provider-first model picker modal.
func (m *monitorModel) openModelPicker() tea.Cmd {
	m.modelPickerOpen = true
	m.modelPickerProvider = ""
	m.modelPickerQuery = ""
	m.modelPickerProviderView = true

	l := list.New([]list.Item{}, newMonitorModelPickerDelegate(), 0, 0)
	l.Title = "Select Provider"
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(true)
	l.SetFilteringEnabled(false)
	l.SetShowFilter(false)
	l.Styles.Title = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#707070")).
		Bold(true)

	m.modelPickerList = l
	m.refreshModelPickerItems()
	return nil
}

// closeModelPicker closes the model picker modal.
func (m *monitorModel) closeModelPicker() {
	m.modelPickerOpen = false
	m.modelPickerList = list.Model{}
	m.modelPickerProvider = ""
	m.modelPickerQuery = ""
	m.modelPickerProviderView = false
}

func (m *monitorModel) refreshModelPickerItems() {
	if !m.modelPickerOpen {
		return
	}
	q := strings.ToLower(strings.TrimSpace(m.modelPickerQuery))
	items := make([]list.Item, 0, 64)

	if m.modelPickerProviderView {
		var res protocol.ModelListResult
		_ = m.rpcRoundTrip(protocol.MethodModelList, protocol.ModelListParams{
			ThreadID: protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
			Query:    strings.TrimSpace(m.modelPickerQuery),
		}, &res)
		for _, p := range res.Providers {
			candidate := strings.ToLower(strings.TrimSpace(p.Name))
			if q != "" && !strings.Contains(candidate, q) {
				continue
			}
			items = append(items, monitorModelPickerItem{provider: strings.TrimSpace(p.Name), isProvider: true, count: p.Count})
		}
		m.modelPickerList.Title = "Select Provider"
	} else {
		provider := strings.TrimSpace(m.modelPickerProvider)
		var res protocol.ModelListResult
		_ = m.rpcRoundTrip(protocol.MethodModelList, protocol.ModelListParams{
			ThreadID: protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
			Provider: provider,
			Query:    strings.TrimSpace(m.modelPickerQuery),
		}, &res)
		for _, info := range res.Models {
			if !strings.EqualFold(strings.TrimSpace(info.Provider), provider) {
				continue
			}
			candidate := strings.ToLower(strings.TrimSpace(info.ID))
			if q != "" && !strings.Contains(candidate, q) {
				continue
			}
			items = append(items, monitorModelPickerItem{id: strings.TrimSpace(info.ID), provider: provider})
		}
		m.modelPickerList.Title = "Select Model (" + provider + ")"
	}

	m.modelPickerList.SetItems(items)
	if len(items) > 0 {
		m.modelPickerList.Select(0)
	}
}

// updateModelPicker handles keyboard input when the model picker is open.
func (m *monitorModel) updateModelPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := strings.ToLower(msg.String())
	switch s {
	case "esc", "escape":
		if !m.modelPickerProviderView {
			m.modelPickerProviderView = true
			m.modelPickerProvider = ""
			m.modelPickerQuery = ""
			m.refreshModelPickerItems()
			return m, nil
		}
		m.closeModelPicker()
		return m, nil
	case "backspace":
		if m.modelPickerQuery != "" {
			r := []rune(m.modelPickerQuery)
			m.modelPickerQuery = string(r[:len(r)-1])
			m.refreshModelPickerItems()
			return m, nil
		}
	case "enter":
		return m, m.selectModelFromPicker()
	}

	if len(msg.Runes) > 0 {
		for _, r := range msg.Runes {
			if r >= 32 && r != 127 {
				m.modelPickerQuery += string(r)
			}
		}
		m.refreshModelPickerItems()
		return m, nil
	}

	var cmd tea.Cmd
	m.modelPickerList, cmd = m.modelPickerList.Update(msg)
	return m, cmd
}

// selectModelFromPicker selects the currently highlighted provider/model.
func (m *monitorModel) selectModelFromPicker() tea.Cmd {
	if m.modelPickerList.Items() == nil || len(m.modelPickerList.Items()) == 0 {
		return nil
	}
	selectedItem := m.modelPickerList.SelectedItem()
	if selectedItem == nil {
		return nil
	}
	item, ok := selectedItem.(monitorModelPickerItem)
	if !ok {
		return nil
	}

	if item.isProvider {
		m.modelPickerProviderView = false
		m.modelPickerProvider = strings.TrimSpace(item.provider)
		m.modelPickerQuery = ""
		m.refreshModelPickerItems()
		return nil
	}

	selectedID := item.id
	m.model = selectedID
	m.closeModelPicker()
	if m.teamID != "" {
		return m.writeTeamControl("set_team_model", selectedID)
	}
	return m.writeControl("set_model", map[string]any{"model": selectedID})
}

func (m *monitorModel) renderModelPicker(base string) string {
	maxModalW := max(1, m.width-8)
	modalWidth := min(70, maxModalW)
	minModalW := min(46, maxModalW)
	if modalWidth < minModalW {
		modalWidth = minModalW
	}

	maxModalH := max(1, m.height-8)
	modalHeight := min(22, maxModalH)
	minModalH := min(10, maxModalH)
	if modalHeight < minModalH {
		modalHeight = minModalH
	}

	listHeight := modalHeight - 6
	if listHeight < 4 {
		listHeight = 4
	}
	m.modelPickerList.SetWidth(modalWidth - 4)
	m.modelPickerList.SetHeight(listHeight)

	scope := "Global"
	if !m.modelPickerProviderView {
		scope = "Provider: " + strings.TrimSpace(m.modelPickerProvider)
	}
	searchLine := kit.StyleDim.Render("Search: ") + kit.StyleStatusValue.Render(fallback(m.modelPickerQuery, ""))
	helpLine := kit.StyleDim.Render("Enter select · Esc back/close · type to search")
	content := scope + "\n" + searchLine + "\n\n" + m.modelPickerList.View() + "\n" + helpLine

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
