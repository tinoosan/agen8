package tui

import (
	"math"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/agen8/internal/tui/kit"
	layoutmgr "github.com/tinoosan/agen8/internal/tui/layout"
	"github.com/tinoosan/agen8/pkg/types"
)

func (m *monitorModel) handleNextPage() (tea.Model, tea.Cmd) {
	switch m.focusedPanel {
	case panelActivity:
		maxPage := kit.MaxPage(m.activityTotalCount, m.activityPageSize)
		if m.activityPage < maxPage {
			m.activityPage++
			m.activityFollowingTail = m.activityPage == maxPage
			return m, m.loadActivityPage()
		}
	case panelInbox:
		maxPage := kit.MaxPage(m.inboxTotalCount, m.inboxPageSize)
		if m.inboxPage < maxPage {
			m.inboxPage++
			return m, m.loadInboxPage()
		}
	case panelOutbox:
		maxPage := kit.MaxPage(m.outboxTotalCount, m.outboxPageSize)
		if m.outboxPage < maxPage {
			m.outboxPage++
			return m, m.loadOutboxPage()
		}
	}
	return m, nil
}

func (m *monitorModel) handlePrevPage() (tea.Model, tea.Cmd) {
	switch m.focusedPanel {
	case panelActivity:
		if m.activityPage > 0 {
			m.activityPage--
			m.activityFollowingTail = false
			return m, m.loadActivityPage()
		}
	case panelInbox:
		if m.inboxPage > 0 {
			m.inboxPage--
			return m, m.loadInboxPage()
		}
	case panelOutbox:
		if m.outboxPage > 0 {
			m.outboxPage--
			return m, m.loadOutboxPage()
		}
	}
	return m, nil
}

func (m *monitorModel) handleFirstPage() (tea.Model, tea.Cmd) {
	switch m.focusedPanel {
	case panelActivity:
		if m.activityPage != 0 {
			m.activityPage = 0
			m.activityFollowingTail = false
			return m, m.loadActivityPage()
		}
	case panelInbox:
		if m.inboxPage != 0 {
			m.inboxPage = 0
			return m, m.loadInboxPage()
		}
	case panelOutbox:
		if m.outboxPage != 0 {
			m.outboxPage = 0
			return m, m.loadOutboxPage()
		}
	}
	return m, nil
}

func (m *monitorModel) handleLastPage() (tea.Model, tea.Cmd) {
	switch m.focusedPanel {
	case panelActivity:
		maxPage := kit.MaxPage(m.activityTotalCount, m.activityPageSize)
		if m.activityPage != maxPage {
			m.activityPage = maxPage
			m.activityFollowingTail = true
			return m, m.loadActivityPage()
		}
	case panelInbox:
		maxPage := kit.MaxPage(m.inboxTotalCount, m.inboxPageSize)
		if m.inboxPage != maxPage {
			m.inboxPage = maxPage
			return m, m.loadInboxPage()
		}
	case panelOutbox:
		maxPage := kit.MaxPage(m.outboxTotalCount, m.outboxPageSize)
		if m.outboxPage != maxPage {
			m.outboxPage = maxPage
			return m, m.loadOutboxPage()
		}
	}
	return m, nil
}

func (m *monitorModel) refreshActivityList() {
	prevIdx := m.activityList.Index()
	prevID := ""
	wasFollowingTail := m.activityFollowingTail
	if prevIdx >= 0 && prevIdx < len(m.activityPageItems) {
		prevID = m.activityPageItems[prevIdx].ID
	}

	items := make([]list.Item, 0, len(m.activityPageItems))
	for _, a := range m.activityPageItems {
		items = append(items, activityItem{act: a})
	}
	m.activityList.SetItems(items)
	if len(items) == 0 {
		return
	}

	selectIdx := min(max(prevIdx, 0), len(items)-1)
	if wasFollowingTail {
		selectIdx = len(items) - 1
	} else if strings.TrimSpace(prevID) != "" {
		for i := range m.activityPageItems {
			if m.activityPageItems[i].ID == prevID {
				selectIdx = i
				break
			}
		}
	}
	m.activityList.Select(selectIdx)
}

func (m *monitorModel) refreshActivityDetail(forceTop bool) {
	if m.renderer == nil {
		return
	}
	if len(m.activityPageItems) == 0 || m.activityList.Index() < 0 || m.activityList.Index() >= len(m.activityPageItems) {
		m.activityDetail.SetContent("")
		m.activityDetailAct = ""
		m.activityDetail.GotoTop()
		return
	}
	prevYOffset := m.activityDetail.YOffset
	w := imax(24, m.activityDetail.Width-4)
	header := "### Details\n\n"
	help := "_PgUp/PgDn scroll · use Activity to change selection_\n\n"
	act := m.activityPageItems[m.activityList.Index()]
	md := renderActivityDetailMarkdown(act, false, false)
	rendered := strings.TrimRight(m.renderer.RenderMarkdown(header+help+md, w), "\n")
	rendered = wrapViewportText(rendered, imax(10, m.activityDetail.Width))
	m.activityDetail.SetContent(rendered)
	if forceTop || m.activityDetailAct != act.ID {
		m.activityDetail.GotoTop()
	} else if prevYOffset > 0 {
		m.activityDetail.YOffset = prevYOffset
	}
	m.activityDetailAct = act.ID
}

func (m *monitorModel) layout() layoutmgr.GridLayout {
	manager := layoutmgr.NewManager(m.styles.panel, true)
	frameW, frameH := m.styles.commandBar.GetFrameSize()
	titleH := 1
	composerContentW := max(10, m.width-frameW)
	if !m.isCompactMode() {
		composerContentW = max(10, m.dashboardLeftColumnWidth(m.width)-frameW)
	}
	composerHeight := m.composerContentLineCount(composerContentW) + frameH + titleH
	if m.isCompactMode() {
		return manager.CalculateCompact(m.width, m.height, composerHeight)
	}
	statsHeight := 0
	statusBarH := lipgloss.Height(m.renderBottomBar(m.width))
	showWarning := m.runStatus != types.RunStatusRunning
	return manager.CalculateDashboard(m.width, m.height, composerHeight, statsHeight, statusBarH, showWarning)
}

func (m *monitorModel) dashboardLeftColumnWidth(width int) int {
	const (
		minLeftWidth  = 60
		minRightWidth = 32
		gapCols       = 1
	)
	if width < 0 {
		width = 0
	}
	minTotalWidth := minLeftWidth + minRightWidth + gapCols
	if width < minTotalWidth {
		available := width - gapCols
		if available < 0 {
			available = 0
		}
		leftW := int(math.Round(float64(available) * 0.66))
		rightW := available - leftW
		if leftW < 1 && available >= 2 {
			leftW = 1
			rightW = available - 1
		} else if rightW < 1 && available >= 2 {
			rightW = 1
			leftW = available - 1
		}
		return leftW
	}
	leftW := int(math.Round(float64(width) * 0.66))
	if leftW < minLeftWidth {
		leftW = minLeftWidth
	}
	if leftW > width-minRightWidth-gapCols {
		leftW = max(0, width-minRightWidth-gapCols)
	}
	return leftW
}

func (m *monitorModel) calculatePanelHeight(contentRows int, isEmpty bool, isFocused bool) int {
	const maxPanelHeight = 6

	if isEmpty && !isFocused {
		return 0
	}

	desired := contentRows + 3 // border(2) + title(1)
	if desired > maxPanelHeight {
		return maxPanelHeight
	}
	if desired < 0 {
		return 0
	}
	return desired
}
