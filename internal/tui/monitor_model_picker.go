package tui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/tinoosan/agen8/internal/tui/kit"
	"github.com/tinoosan/agen8/internal/tui/modelpicker"
)

// modelPickerItem implements list.Item for the model picker.
// It is kept for test compatibility; list contents are mirrored from the shared controller.
type monitorModelPickerItem struct {
	id         string
	provider   string
	isProvider bool
	count      int
	inputPerM  float64
	outputPerM float64
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
			return strings.TrimSpace(m.provider) + " (" + strconv.Itoa(m.count) + ")"
		}
		return strings.TrimSpace(m.provider)
	}
	return modelpicker.FormatModelTitle(m.id, m.inputPerM, m.outputPerM)
}

func (m monitorModelPickerItem) Description() string { return "" }

// openModelPicker initializes and opens the provider-first model picker modal.
func (m *monitorModel) openModelPicker() tea.Cmd {
	m.helpModalOpen = false
	m.closeAllPickers()
	cmd := m.modelPickerCtrl.Open(m.rpcEndpointOrDefault(), strings.TrimSpace(m.rpcRun().SessionID))
	if cmd != nil {
		msg := cmd()
		_, _ = m.modelPickerCtrl.Update(msg)
	}
	m.syncModelPickerLegacy()
	return nil
}

// closeModelPicker closes the model picker modal.
func (m *monitorModel) closeModelPicker() {
	m.modelPickerCtrl.Close()
	m.syncModelPickerLegacy()
}

// refreshModelPickerItems keeps backward-compatible behavior for tests that call this directly.
func (m *monitorModel) refreshModelPickerItems() {
	m.syncModelPickerLegacy()
}

// updateModelPicker delegates to the shared controller. dispatchUpdate also handles picker messages globally.
func (m *monitorModel) updateModelPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	cmd, event := m.modelPickerCtrl.Update(msg)
	if cmd != nil {
		nextMsg := cmd()
		_, _ = m.modelPickerCtrl.Update(nextMsg)
		cmd = nil
	}
	m.syncModelPickerLegacy()
	switch event.Type {
	case modelpicker.EventModelSelected:
		m.model = event.ModelID
		if m.teamID != "" {
			return m, m.writeTeamControl("set_team_model", event.ModelID)
		}
		return m, m.writeControl("set_model", map[string]any{"model": event.ModelID})
	case modelpicker.EventError:
		if event.Err != nil {
			m.appendAgentOutput("[model picker] error: " + event.Err.Error())
		}
	}
	return m, cmd
}

func (m *monitorModel) renderModelPicker(base string) string {
	dims := m.modelPickerCtrl.SetSize(m.width, m.height)
	content := m.modelPickerCtrl.View()
	opts := kit.DefaultPickerModalOpts(content, m.width, m.height, dims.ModalWidth, dims.ModalHeight)
	_ = base
	return kit.RenderOverlay(opts)
}

func (m *monitorModel) syncModelPickerLegacy() {
	m.modelPickerOpen = m.modelPickerCtrl.IsOpen()
	view := m.modelPickerCtrl.State()
	m.modelPickerProviderView = view.ProviderView
	m.modelPickerProvider = view.Provider
	m.modelPickerQuery = view.Query
	l := view.List
	items := make([]list.Item, 0, len(view.Items))
	for _, it := range view.Items {
		items = append(items, monitorModelPickerItem{
			id:         strings.TrimSpace(it.ID),
			provider:   strings.TrimSpace(it.Provider),
			isProvider: it.IsProvider,
			count:      it.Count,
			inputPerM:  it.InputPerM,
			outputPerM: it.OutputPerM,
		})
	}
	l.SetItems(items)
	if len(items) > 0 && l.Index() >= len(items) {
		l.Select(len(items) - 1)
	}
	m.modelPickerList = l
}

func (m *monitorModel) rpcEndpointOrDefault() string {
	endpoint := strings.TrimSpace(m.rpcEndpoint)
	if endpoint == "" {
		endpoint = monitorRPCEndpoint()
	}
	return endpoint
}
