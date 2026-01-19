package tui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
)

func (m *Model) layout() {
	wasAtBottom := m.transcript.AtBottom()

	// Width-dependent components (header + footer) can wrap when the terminal is narrow.
	// We compute their real rendered heights so the header is never "pushed off" by a
	// footer that becomes taller due to wrapping.
	// Composer width:
	// - we render the composer as a bordered "card" with an accent bar and outer margin.
	// - to keep layout stable, we must pre-compute the *content* width and size
	//   the textarea(s) so they never exceed the card inner width.
	//
	// Overhead budget (left + right):
	// - outer margin: 2 cols (1 each side)
	// - border: 2 cols
	// - padding: 2 cols
	// - accent + gap: 2 cols
	// => total ~8 cols of overhead
	composerContentW := max(20, m.width-8)
	m.single.SetWidth(composerContentW)
	m.multiline.SetWidth(composerContentW)

	headerH := lipgloss.Height(m.renderHeader())
	if headerH < 1 {
		headerH = 1
	}
	footerH := lipgloss.Height(m.renderInput())
	if footerH < 1 {
		footerH = 1
	}

	bodyH := m.height - headerH - footerH
	if bodyH < 1 {
		bodyH = 1
	}

	m.transcript.Height = bodyH

	mainW := m.width
	detailW := 0
	if m.showDetails {
		// 70/30 split with a minimum transcript width.
		detailW = int(math.Round(float64(m.width) * 0.33))
		if detailW < 32 {
			detailW = 32
		}
		if detailW > m.width-40 {
			detailW = max(32, m.width-40)
		}
		mainW = m.width - detailW
	}

	m.transcript.Width = max(40, mainW)
	if detailW != 0 {
		// The right pane has a border, so its inner content must be sized to
		// fit within (detailW-2)x(bodyH-2). If we let inner components render
		// taller than the pane, the combined view can exceed terminal height and
		// cause the header to appear to "disappear" (clipped off the top).
		innerW := max(24, detailW-2)
		innerH := max(1, bodyH-2)

		m.activityList.SetWidth(max(24, innerW))

		// Split the inner height between list and details.
		listH := max(6, innerH/2)
		if listH > innerH-1 {
			listH = max(1, innerH-1)
		}
		m.activityList.SetHeight(listH)

		m.activityDetail.Width = max(24, innerW)
		// No extra divider line between list + detail; keep the right pane height exact.
		m.activityDetail.Height = max(1, innerH-listH)
	}

	// Model picker sizing: ensure the underlying list model has a real viewport so
	// VisibleItems/filtering behave correctly (not just during rendering).
	if m.modelPickerOpen {
		modalW := 60
		if modalW > m.width-8 {
			modalW = m.width - 8
		}
		if modalW < 40 {
			modalW = 40
		}
		modalH := 20
		if modalH > m.height-8 {
			modalH = m.height - 8
		}
		if modalH < 10 {
			modalH = 10
		}

		innerW := max(20, modalW-4) // modal padding/border budget
		innerH := max(4, modalH-2)
		m.modelPickerList.SetWidth(innerW)
		m.modelPickerList.SetHeight(innerH)
	}

	// File picker sizing: ensure the underlying list model has a real viewport so
	// navigation + selection behave correctly (not just during rendering).
	if m.filePickerOpen {
		modalW := 80
		if modalW > m.width-8 {
			modalW = m.width - 8
		}
		if modalW < 40 {
			modalW = 40
		}
		modalH := 22
		if modalH > m.height-8 {
			modalH = m.height - 8
		}
		if modalH < 10 {
			modalH = 10
		}

		innerW := max(20, modalW-4) // modal padding/border budget
		innerH := max(4, modalH-2)
		m.filePickerList.SetWidth(innerW)
		m.filePickerList.SetHeight(innerH)
	}

	m.rebuildTranscript()
	if wasAtBottom {
		m.transcript.GotoBottom()
	}
	m.refreshActivityDetail()

	// Recompute once after content/layout changes so the footer measurement stays correct
	// for the next resize cycle.
}

func (m Model) renderHeader() string {
	left := m.styleHeaderApp.Render("workbench")

	mid := strings.TrimSpace(m.workflowTitle)
	if mid == "" {
		mid = strings.TrimSpace(m.sessionTitle)
	}
	if mid == "" {
		mid = "interactive"
	}
	if wd := strings.TrimSpace(m.workdir); wd != "" {
		// Keep the workdir visible but bounded.
		wd = truncateMiddle(wd, max(16, m.width/3))
		mid = mid + " · " + wd
	}
	mid = truncateMiddle(mid, max(16, m.width/2))
	mid = m.styleHeaderMid.Render(mid)

	rhsParts := []string{}
	if m.lastTurnTokens != 0 {
		rhsParts = append(rhsParts, fmt.Sprintf("%d tok", m.lastTurnTokens))
	}
	if strings.TrimSpace(m.lastTurnCostUSD) != "" {
		rhsParts = append(rhsParts, "$"+m.lastTurnCostUSD)
	}
	if m.totalCostUSD > 0 {
		rhsParts = append(rhsParts, fmt.Sprintf("Σ$%.4f", m.totalCostUSD))
	}
	if m.turnInFlight {
		rhsParts = append(rhsParts, "running…")
	}
	rhs := m.styleHeaderRHS.Render(strings.Join(rhsParts, "  "))

	// Fit: left | mid | rhs
	avail := max(1, m.width)
	leftW := lipgloss.Width(left)
	rhsW := lipgloss.Width(rhs)
	midW := max(0, avail-leftW-rhsW-2)
	mid = lipgloss.NewStyle().Width(midW).Align(lipgloss.Center).Render(mid)

	return m.styleHeaderBar.Render(lipgloss.JoinHorizontal(lipgloss.Top, left, " ", mid, " ", rhs))
}

func (m Model) renderBody() string {
	if !m.showDetails {
		return m.transcript.View()
	}

	// Note: lipgloss Style Width/Height refer to the content area and do not include
	// border width/height. Since the right pane uses a border, we size it using the
	// inner dimensions so the overall pane stays exactly aligned with the transcript.
	rightW := max(24, m.activityList.Width())
	rightH := max(1, m.transcript.Height-2) // -2 for top/bottom border

	// Important: keep the right pane height exactly equal to the transcript height.
	// If the right pane is taller, Bubble Tea will clip the top of the overall view,
	// which makes the header appear to "disappear" when Activity is toggled.
	rightBody := lipgloss.JoinVertical(lipgloss.Top, m.activityList.View(), m.activityDetail.View())
	rightPane := lipgloss.NewStyle().
		Width(rightW).
		Height(rightH).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("#303030")).
		Render(rightBody)

	return lipgloss.JoinHorizontal(lipgloss.Top, m.transcript.View(), rightPane)
}

func (m Model) renderInput() string {
	var input string
	if m.isMulti {
		input = m.multiline.View()
	} else {
		input = m.single.View()
	}

	isFocused := m.focus == focusInput
	cardStyle := m.styleComposerCardBlurred
	accentStyle := m.styleComposerAccentBlur
	if isFocused {
		cardStyle = m.styleComposerCardFocused
		accentStyle = m.styleComposerAccentFocus
	}

	modelID := strings.TrimSpace(m.modelID)
	if modelID == "" {
		modelID = "unknown"
	}
	modelIDDisplay := modelID

	eff := strings.TrimSpace(m.reasoningEffort)
	effortLabel := ""
	if eff != "" {
		effortLabel = m.styleComposerStatusKey.Render("effort") + " " + m.styleComposerStatusVal.Render(eff)
	}

	modelLabel := m.styleComposerStatusKey.Render("model") + " " + m.styleComposerStatusVal.Render(modelIDDisplay)

	ids := []string{}
	if v := strings.TrimSpace(m.sessionID); v != "" {
		ids = append(ids, "sess:"+truncateMiddle(v, 10))
	}
	if v := strings.TrimSpace(m.runID); v != "" {
		ids = append(ids, "run:"+truncateMiddle(v, 10))
	}
	idsLabel := ""
	if len(ids) != 0 {
		idsLabel = m.styleDim.Render(strings.Join(ids, " "))
	}

	// Status row is rendered inside the composer card.
	// It uses the same width as the editor so it never overflows the viewport.
	statusW := max(20, m.width-8)
	statusRight := idsLabel
	rightW := lipgloss.Width(statusRight)
	leftMax := statusW
	if rightW != 0 {
		leftMax = max(0, statusW-rightW-1)
	}

	// Prefer keeping effort visible (when known); truncate the model ID if needed.
	statusLeft := modelLabel
	if effortLabel != "" {
		statusLeft = modelLabel + "  " + effortLabel
		if leftMax > 0 && lipgloss.Width(statusLeft) > leftMax {
			excess := lipgloss.Width(statusLeft) - leftMax
			allowedIDW := max(8, lipgloss.Width(modelIDDisplay)-excess-1)
			modelIDDisplay = truncateMiddle(modelID, allowedIDW)
			modelLabel = m.styleComposerStatusKey.Render("model") + " " + m.styleComposerStatusVal.Render(modelIDDisplay)
			statusLeft = modelLabel + "  " + effortLabel
		}
	}
	leftW := lipgloss.Width(statusLeft)
	rightW = lipgloss.Width(statusRight)
	midW := max(0, statusW-leftW-rightW-1)
	status := statusLeft
	if midW > 0 {
		status += strings.Repeat(" ", midW)
	}
	if statusRight != "" {
		status += " " + statusRight
	}
	status = lipgloss.NewStyle().Width(statusW).Render(status)

	// Render command palette if open.
	palette := m.renderCommandPalette()
	contentParts := []string{status}
	if palette != "" {
		contentParts = append(contentParts, "", palette)
	}
	contentParts = append(contentParts, "", input)
	content := lipgloss.JoinVertical(lipgloss.Top, contentParts...)
	h := lipgloss.Height(content)
	if h < 1 {
		h = 1
	}
	accentLines := make([]string, 0, h)
	for i := 0; i < h; i++ {
		accentLines = append(accentLines, "│")
	}
	accent := accentStyle.Render(strings.Join(accentLines, "\n"))

	box := cardStyle.Render(lipgloss.JoinHorizontal(lipgloss.Top, accent, " ", content))
	statusRaw := m.renderStatusLine()
	focusName := "input"
	if m.focus == focusActivityList {
		focusName = "activity"
	}

	hintText := "ctrl+a activity  tab focus  ctrl+g multiline  enter send  ctrl+c quit"
	if m.showDetails {
		hintText = "ctrl+a hide activity  tab focus  esc close  j/k↑/↓ select  e/enter expand  o open file  pgup/pgdn scroll  ctrl+t telemetry  ctrl+g multiline  ctrl+o send (multiline)"
	} else {
		hintText = "pgup/pgdn scroll  " + hintText
	}
	footerW := max(20, m.width-2)
	hintRaw := hintText + "  focus: " + focusName
	hint := m.styleHint.Render(wordwrap.String(hintRaw, footerW))
	if statusRaw != "" {
		status := m.styleHint.Render(wordwrap.String(statusRaw, footerW))
		return box + "\n" + status + "\n" + hint
	}
	return box + "\n" + hint
}

func (m Model) renderStatusLine() string {
	parts := []string{}
	if strings.TrimSpace(m.lastTurnDuration) != "" {
		parts = append(parts, m.lastTurnDuration)
	}
	if m.lastTurnTokens != 0 {
		parts = append(parts, fmt.Sprintf("%d tokens", m.lastTurnTokens))
	}
	if strings.TrimSpace(m.lastTurnCostUSD) != "" {
		parts = append(parts, "$"+m.lastTurnCostUSD)
	}
	if m.totalCostUSD > 0 {
		parts = append(parts, fmt.Sprintf("Σ$%.4f", m.totalCostUSD))
	}
	if len(parts) == 0 {
		return ""
	}
	return "last: " + strings.Join(parts, " • ")
}

func (m Model) renderCommandPalette() string {
	if !m.commandPaletteOpen || len(m.commandPaletteMatches) == 0 {
		return ""
	}

	// Limit displayed matches to 6 for readability.
	maxDisplay := 6
	displayMatches := m.commandPaletteMatches
	if len(displayMatches) > maxDisplay {
		displayMatches = displayMatches[:maxDisplay]
	}

	// Style for selected vs unselected items.
	styleSelected := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6bbcff")).
		Bold(true)
	styleUnselected := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#c0c0c0"))

	lines := []string{}
	for i, cmd := range displayMatches {
		if i == m.commandPaletteSelected {
			lines = append(lines, styleSelected.Render("  "+cmd))
		} else {
			lines = append(lines, styleUnselected.Render("  "+cmd))
		}
	}

	// Wrap in a subtle border/background.
	paletteContent := strings.Join(lines, "\n")

	// IMPORTANT: keep the palette's TOTAL rendered width within the composer content width.
	// lipgloss.Style.Width applies to the content box (excluding border + padding).
	// Since we use padding(0,1) and a rounded border, total width is:
	//   contentWidth + (padding L+R=2) + (border L+R=2) = contentWidth + 4
	// The composer content budget is ~ (m.width-8), so we set contentWidth to (budget-4).
	outerW := max(20, m.width-8)
	contentW := max(1, outerW-4)
	paletteStyle := lipgloss.NewStyle().
		Width(contentW).
		Padding(0, 1).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#404040"))

	return paletteStyle.Render(paletteContent)
}
