package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/agen8/internal/tui/kit"
)

func (m *Model) syncSessionPickerState() {
	state := m.sessionPickerCtrl.State()
	m.sessionPickerOpen = state.Open
	m.sessionPickerList = state.List
	m.sessionPickerFilter = state.Filter
	m.sessionPickerPage = state.Page
	m.sessionPickerPageSize = state.PageSize
	m.sessionPickerTotal = state.Total
}

func (m *Model) ensureSessionPickerCtrl() {
	if !m.sessionPickerOpen || m.sessionPickerCtrl.IsOpen() {
		return
	}
	m.sessionPickerCtrl.Open(kit.PickerConfig{
		Title:            "Select Session",
		FilteringEnabled: true,
		ShowFilter:       true,
		PageKeyNav:       true,
		PageSize:         maxSessionPageSize(m.sessionPickerPageSize),
		Delegate:         newSessionPickerDelegate(),
		ModalWidth:       80,
		ModalHeight:      22,
		ModalMinWidth:    48,
		ModalMinHeight:   12,
		ModalMarginX:     8,
		ModalMarginY:     4,
	})
	adoptPickerLegacyState(&m.sessionPickerCtrl, m.sessionPickerList, m.sessionPickerFilter, m.sessionPickerPage, m.sessionPickerTotal, m.sessionPickerPageSize)
	m.syncSessionPickerState()
}

func (m *Model) syncFilePickerState() {
	state := m.filePickerCtrl.State()
	m.filePickerOpen = state.Open
	m.filePickerList = state.List
	m.filePickerQuery = state.Filter
}

func (m *Model) ensureFilePickerCtrl() {
	if !m.filePickerOpen || m.filePickerCtrl.IsOpen() {
		return
	}
	m.filePickerCtrl.Open(kit.PickerConfig{
		Title:            "Select File",
		ShowPagination:   true,
		CursorPageKeyNav: true,
		Delegate:         kit.NewPickerDelegate(kit.DefaultPickerDelegateStyles(), nil),
		ModalWidth:       80,
		ModalHeight:      22,
		ModalMinWidth:    40,
		ModalMinHeight:   10,
		ModalMarginX:     8,
		ModalMarginY:     2,
	})
	adoptPickerLegacyState(&m.filePickerCtrl, m.filePickerList, m.filePickerQuery, 0, 0, 0)
	m.syncFilePickerState()
}

func (m *monitorModel) syncSessionPickerState() {
	state := m.sessionPickerCtrl.State()
	m.sessionPickerOpen = state.Open
	m.sessionPickerList = state.List
	m.sessionPickerFilter = state.Filter
	m.sessionPickerPage = state.Page
	m.sessionPickerPageSize = state.PageSize
	m.sessionPickerTotal = state.Total
}

func (m *monitorModel) ensureSessionPickerCtrl() {
	if !m.sessionPickerOpen || m.sessionPickerCtrl.IsOpen() {
		return
	}
	m.sessionPickerCtrl.Open(kit.PickerConfig{
		Title:            "Select Session",
		FilteringEnabled: true,
		ShowFilter:       true,
		PageKeyNav:       true,
		PageSize:         maxSessionPageSize(m.sessionPickerPageSize),
		Delegate:         newSessionPickerDelegate(),
		ModalWidth:       80,
		ModalHeight:      22,
		ModalMinWidth:    48,
		ModalMinHeight:   12,
		ModalMarginX:     8,
		ModalMarginY:     4,
	})
	adoptPickerLegacyState(&m.sessionPickerCtrl, m.sessionPickerList, m.sessionPickerFilter, m.sessionPickerPage, m.sessionPickerTotal, m.sessionPickerPageSize)
	m.syncSessionPickerState()
}

func (m *monitorModel) syncFilePickerState() {
	state := m.filePickerCtrl.State()
	m.filePickerOpen = state.Open
	m.filePickerList = state.List
	m.filePickerQuery = state.Filter
}

func (m *monitorModel) ensureFilePickerCtrl() {
	if !m.filePickerOpen || m.filePickerCtrl.IsOpen() {
		return
	}
	m.filePickerCtrl.Open(kit.PickerConfig{
		Title:            "Select File",
		ShowPagination:   true,
		CursorPageKeyNav: true,
		Delegate:         kit.NewPickerDelegate(kit.DefaultPickerDelegateStyles(), nil),
		ModalWidth:       80,
		ModalHeight:      22,
		ModalMinWidth:    40,
		ModalMinHeight:   10,
		ModalMarginX:     8,
		ModalMarginY:     2,
	})
	adoptPickerLegacyState(&m.filePickerCtrl, m.filePickerList, m.filePickerQuery, 0, 0, 0)
	m.syncFilePickerState()
}

func pickerFooter(total, page, pageSize int, errText, emptyText string, style lipgloss.Style) string {
	if total == 0 {
		if errText != "" {
			return style.Render("Ctrl+N/P: page")
		}
		return style.Render(emptyText)
	}
	if pageSize <= 0 {
		pageSize = 50
	}
	maxPage := (total + pageSize - 1) / pageSize
	currentPage := page + 1
	return style.Render(fmt.Sprintf("Page %d of %d (%d sessions) • Ctrl+N/P: page", currentPage, maxPage, total))
}

func adoptPickerLegacyState(ctrl *kit.PickerController, legacyList list.Model, filter string, page, total, pageSize int) {
	if ctrl == nil {
		return
	}
	if len(legacyList.Items()) > 0 {
		ctrl.SetItems(legacyList.Items())
	}
	if idx := legacyList.Index(); idx > 0 {
		ctrl.Select(idx)
	}
	if strings.TrimSpace(legacyList.Title) != "" {
		ctrl.SetTitle(legacyList.Title)
	}
	ctrl.SetFilter(filter)
	ctrl.SetPage(page, total, pageSize)
}

func maxSessionPageSize(pageSize int) int {
	if pageSize > 0 {
		return pageSize
	}
	return 50
}
