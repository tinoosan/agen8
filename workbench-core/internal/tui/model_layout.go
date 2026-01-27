package tui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
	"github.com/tinoosan/workbench-core/internal/tui/kit"
	"github.com/tinoosan/workbench-core/pkg/cost"
	"github.com/tinoosan/workbench-core/pkg/types"
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

	// View() inserts explicit newline separators:
	//   header + "\n" + body + "\n" + input
	// Account for those 2 rows here so the rendered view never exceeds terminal bounds.
	bodyH := m.height - headerH - footerH - 2
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
		tabBarH := lipgloss.Height(m.renderRightPaneTabs())
		if tabBarH < 1 {
			tabBarH = 1
		}
		contentH := innerH - tabBarH
		if contentH < 1 {
			contentH = 1
		}

		m.activityList.SetWidth(max(24, innerW))
		m.planViewport.Width = max(24, innerW)
		m.swarmViewport.Width = max(24, innerW)
		m.planViewport.Height = max(1, contentH)
		m.swarmViewport.Height = max(1, contentH)

		// Split the inner height between list and details.
		//
		// Prefer showing more activity rows, but keep Details usable on small terminals
		// via an adaptive split + minimum heights.
		const (
			minListH    = 6
			minDetailH  = 6
			largeDetail = 0.50
			smallDetail = 0.65
		)
		detailFrac := largeDetail
		if contentH < 18 {
			detailFrac = smallDetail
		}
		detailH := int(math.Round(float64(contentH) * detailFrac))
		if detailH < minDetailH {
			detailH = minDetailH
		}
		// Ensure list keeps a minimum when possible.
		if contentH-detailH < minListH {
			detailH = max(1, contentH-minListH)
		}
		listH := contentH - detailH
		if listH < minListH {
			listH = max(1, contentH-minDetailH)
			detailH = max(1, contentH-listH)
		}
		// Final clamp: ensure Details has at least 1 row.
		if listH > contentH-1 {
			listH = max(1, contentH-1)
			detailH = max(1, contentH-listH)
		}

		// bubbles/list renders a small chrome header (title + spacer) above the items.
		// `SetHeight` controls the list viewport, but the rendered View() includes that chrome.
		// Account for it so list+details never exceed the pane height (critical for small terminals).
		listChromeH := 0
		if strings.TrimSpace(m.activityList.Title) != "" {
			listChromeH = 2
		}
		m.activityList.SetHeight(max(1, listH-listChromeH))

		m.activityDetail.Width = max(24, innerW)
		// No extra divider line between list + detail; keep the right pane height exact.
		m.activityDetail.Height = max(1, detailH)

		// Defensive clamp: bubbles/list (and viewport) can render slightly taller than the
		// nominal heights due to internal chrome/newlines. Ensure the combined right pane
		// content never exceeds contentH, otherwise the overall View can exceed terminal
		// bounds (see TestLayout_WithCommandPalette_ViewNeverExceedsTerminalBounds).
		for i := 0; i < 4; i++ {
			total := lipgloss.Height(m.activityList.View()) + lipgloss.Height(m.activityDetail.View())
			if total <= contentH {
				break
			}
			over := total - contentH
			m.activityList.SetHeight(max(1, m.activityList.Height()-over))
		}
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
	m.refreshPlanView()
	m.refreshSwarmView()

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
		wd = kit.TruncateMiddle(wd, max(16, m.width/3))
		mid = mid + " · " + wd
	}
	mid = kit.TruncateMiddle(mid, max(16, m.width/2))
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
		if m.turnCancelRequested {
			rhsParts = append(rhsParts, "stopping…")
		} else {
			rhsParts = append(rhsParts, "running…")
		}
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
	tabBar := m.renderRightPaneTabs()
	var rightContent string
	if m.swarmTabActive {
		rightContent = m.swarmViewport.View()
	} else if m.planTabActive {
		rightContent = m.planViewport.View()
	} else {
		rightContent = lipgloss.JoinVertical(lipgloss.Top, m.activityList.View(), m.activityDetail.View())
	}
	rightBody := lipgloss.JoinVertical(lipgloss.Top, tabBar, rightContent)
	rightPaneStyle := lipgloss.NewStyle().
		Width(rightW).
		Height(rightH)
	// On very small terminals, the border itself can make the right pane taller than the
	// transcript (since the transcript can shrink below the 2-line border budget). In that
	// case, render borderless to guarantee the overall View never exceeds terminal bounds.
	if m.transcript.Height >= 4 {
		rightPaneStyle = rightPaneStyle.
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#303030"))
	} else {
		// Borderless: use the full outer width/height budget.
		rightPaneStyle = lipgloss.NewStyle().
			Width(rightW + 2).
			Height(max(1, m.transcript.Height))
	}
	rightPane := rightPaneStyle.Render(rightBody)

	return lipgloss.JoinHorizontal(lipgloss.Top, m.transcript.View(), rightPane)
}

func (m Model) renderRightPaneTabs() string {
	activity := "Activity"
	plan := "Plan"
	swarm := "Swarm"
	var activityTab, planTab, swarmTab string
	if m.swarmTabActive {
		activityTab = m.styleRightTabInactive.Render(activity)
		planTab = m.styleRightTabInactive.Render(plan)
		swarmTab = m.styleRightTabActive.Render(swarm)
	} else if m.planTabActive {
		activityTab = m.styleRightTabInactive.Render(activity)
		planTab = m.styleRightTabActive.Render(plan)
		swarmTab = m.styleRightTabInactive.Render(swarm)
	} else {
		activityTab = m.styleRightTabActive.Render(activity)
		planTab = m.styleRightTabInactive.Render(plan)
		swarmTab = m.styleRightTabInactive.Render(swarm)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, activityTab, "  ", planTab, "  ", swarmTab)
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

	tagKeyStyle := kit.CloneStyle(kit.StyleStatusKey)
	tagValueStyle := kit.CloneStyle(kit.StyleStatusValue)

	eff := strings.TrimSpace(m.reasoningEffort)
	effortLabel := ""
	if eff != "" && cost.SupportsReasoningEffort(modelID) {
		effortLabel = kit.RenderTag(kit.TagOptions{
			Key:   "effort",
			Value: eff,
			Styles: kit.TagStyles{
				KeyStyle:   tagKeyStyle,
				ValueStyle: tagValueStyle,
			},
		})
	}

	webState := "off"
	if m.webSearchEnabled {
		webState = "on"
	}
	webLabel := kit.RenderTag(kit.TagOptions{
		Key:   "web",
		Value: webState,
		Styles: kit.TagStyles{
			KeyStyle:   tagKeyStyle,
			ValueStyle: tagValueStyle,
		},
	})

	approvalLabel := kit.RenderTag(kit.TagOptions{
		Key:   "approval",
		Value: defaultIfEmpty(strings.TrimSpace(m.approvalsMode), "enabled"),
		Styles: kit.TagStyles{
			KeyStyle:   tagKeyStyle,
			ValueStyle: tagValueStyle,
		},
	})

	modelLabel := kit.RenderTag(kit.TagOptions{
		Key:   "model",
		Value: modelIDDisplay,
		Styles: kit.TagStyles{
			KeyStyle:   tagKeyStyle,
			ValueStyle: tagValueStyle,
		},
	})

	ids := []string{}
	if v := strings.TrimSpace(m.sessionID); v != "" {
		ids = append(ids, "sess:"+kit.TruncateMiddle(v, 10))
	}
	if v := strings.TrimSpace(m.runID); v != "" {
		ids = append(ids, "run:"+kit.TruncateMiddle(v, 10))
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

	// Prefer keeping web/effort visible; truncate the model ID if needed.
	parts := []string{modelLabel, webLabel, approvalLabel}
	if m.swarmModeActive {
		modeLabel := kit.RenderTag(kit.TagOptions{
			Key:   "mode",
			Value: "swarm",
			Styles: kit.TagStyles{
				KeyStyle:   kit.StylePtr(lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5f5f")).Bold(true)),
				ValueStyle: kit.StylePtr(lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5f5f"))),
			},
		})
		parts = append(parts, modeLabel)
	}
	if effortLabel != "" {
		parts = append(parts, effortLabel)
	}
	statusLeft := strings.Join(parts, "  ")
	if leftMax > 0 && lipgloss.Width(statusLeft) > leftMax {
		excess := lipgloss.Width(statusLeft) - leftMax
		allowedIDW := max(8, lipgloss.Width(modelIDDisplay)-excess-1)
		modelIDDisplay = kit.TruncateMiddle(modelID, allowedIDW)
		modelLabel = m.styleComposerStatusKey.Render("model") + " " + m.styleComposerStatusVal.Render(modelIDDisplay)
		parts[0] = modelLabel
		statusLeft = strings.Join(parts, "  ")
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
	// Render reasoning-effort picker if open (in-composer).
	if p := m.renderReasoningEffortPicker(); p != "" {
		contentParts = append(contentParts, "", p)
	}
	if p := m.renderReasoningSummaryPicker(); p != "" {
		contentParts = append(contentParts, "", p)
	}
	if p := m.renderApprovalPicker(); p != "" {
		contentParts = append(contentParts, "", p)
	}
	if p := m.renderApprovalPrompt(); p != "" {
		contentParts = append(contentParts, "", p)
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

	hintText := "shift+tab swarm  ctrl+a activity  tab focus  ctrl+g multiline enter send  ctrl+c quit"
	if m.showDetails {
		hintText = "ctrl+] tabs  shift+tab swarm  ctrl+a hide activity  tab focus  esc close  j/k↑/↓ select  e/enter expand  o open file  pgup/pgdn scroll  ctrl+t telemetry"
	} else {
		hintText = "pgup/pgdn scroll  " + hintText
	}
	if m.turnInFlight {
		if m.turnCancelRequested {
			hintText = "ctrl+x stopping…  " + hintText
		} else {
			hintText = "ctrl+x stop  " + hintText
		}
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

	maxDisplay := 6

	outerW := max(20, m.width-8)
	contentW := max(1, outerW-4)

	items := make([]kit.Item, len(m.commandPaletteMatches))
	for i, cmd := range m.commandPaletteMatches {
		items[i] = commandPaletteItem(cmd)
	}

	selected := m.commandPaletteSelected
	if selected < 0 {
		selected = 0
	}
	if selected >= len(items) {
		selected = len(items) - 1
	}

	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6bbcff")).
		Bold(true)
	unselectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#c0c0c0"))

	opts := kit.SelectorOptions{
		Width:         contentW,
		MaxHeight:     maxDisplay,
		SelectedIndex: selected,
		ShowPrefix:    true,
		Styles: kit.SelectorStyles{
			SelectedTitle:   kit.CloneStyle(selectedStyle),
			UnselectedTitle: kit.CloneStyle(unselectedStyle),
		},
	}

	paletteContent := kit.RenderSelector(items, opts)

	// IMPORTANT: keep the palette's TOTAL rendered width within the composer content width.
	// lipgloss.Style.Width applies to the content box (excluding border + padding).
	// Since we use padding(0,1) and a rounded border, total width is:
	//   contentWidth + (padding L+R=2) + (border L+R=2) = contentWidth + 4
	// The composer content budget is ~ (m.width-8), so we set contentWidth to (budget-4).
	paletteStyle := lipgloss.NewStyle().
		Width(contentW).
		Padding(0, 1).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#404040"))

	return paletteStyle.Render(paletteContent)
}

func (m Model) renderReasoningEffortPicker() string {
	if !m.reasoningEffortPickerOpen {
		return ""
	}

	// Layout within the composer width budget (same constraints as command palette).
	outerW := max(20, m.width-8)
	contentW := max(1, outerW-4) // padding(0,1) + border => +4 total

	items := make([]kit.Item, len(reasoningEffortOptions))
	for i, opt := range reasoningEffortOptions {
		items[i] = reasoningEffortItem(opt)
	}

	selected := m.reasoningEffortPickerSelected
	if selected < 0 {
		selected = 0
	}
	if selected >= len(items) {
		selected = len(items) - 1
	}

	maxHeight := 6
	opts := kit.SelectorOptions{
		Width:         contentW,
		MaxHeight:     maxHeight,
		SelectedIndex: selected,
		ShowPrefix:    true,
	}

	rendered := kit.RenderSelector(items, opts)

	pickerStyle := lipgloss.NewStyle().
		Width(contentW).
		Padding(0, 1).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#6bbcff")).
		Foreground(lipgloss.Color("#eaeaea"))

	return pickerStyle.Render(rendered)
}

func (m Model) renderReasoningSummaryPicker() string {
	if !m.reasoningSummaryPickerOpen {
		return ""
	}

	outerW := max(20, m.width-8)
	contentW := max(1, outerW-4)

	items := make([]kit.Item, len(reasoningSummaryOptions))
	for i, opt := range reasoningSummaryOptions {
		items[i] = reasoningSummaryItem(opt)
	}

	selected := m.reasoningSummaryPickerSelected
	if selected < 0 {
		selected = 0
	}
	if selected >= len(items) {
		selected = len(items) - 1
	}

	opts := kit.SelectorOptions{
		Width:         contentW,
		MaxHeight:     len(items),
		SelectedIndex: selected,
		ShowPrefix:    true,
	}

	rendered := kit.RenderSelector(items, opts)

	style := lipgloss.NewStyle().
		Width(contentW).
		Padding(0, 1).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#6bbcff")).
		Foreground(lipgloss.Color("#eaeaea"))

	return style.Render(rendered)
}

func (m Model) renderApprovalPicker() string {
	if !m.approvalPickerOpen {
		return ""
	}

	outerW := max(20, m.width-8)
	contentW := max(1, outerW-4)

	items := make([]kit.Item, len(approvalPickerOptions))
	for i, opt := range approvalPickerOptions {
		items[i] = opt
	}

	selected := m.approvalPickerSelected
	if selected < 0 {
		selected = 0
	}
	if selected >= len(items) {
		selected = len(items) - 1
	}

	maxHeight := 6
	opts := kit.SelectorOptions{
		Width:         contentW,
		MaxHeight:     maxHeight,
		SelectedIndex: selected,
		ShowPrefix:    true,
		Spacing:       1,
		Styles: kit.SelectorStyles{
			SelectedTitle:   kit.CloneStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("#6bbcff")).Bold(true)),
			SelectedDesc:    kit.CloneStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("#9ad0ff"))),
			UnselectedTitle: kit.CloneStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("#eaeaea")).Bold(true)),
			UnselectedDesc:  kit.CloneStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("#b0b0b0"))),
		},
	}

	style := lipgloss.NewStyle().
		Width(contentW).
		Padding(0, 1).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#6bbcff")).
		Foreground(lipgloss.Color("#eaeaea"))

	return style.Render(kit.RenderSelector(items, opts))
}

func (m Model) renderApprovalPrompt() string {
	if len(m.awaitingApprovalOps) == 0 {
		return ""
	}
	op := m.awaitingApprovalOps[0]
	title, desc := approvalPromptText(op.Req)

	outerW := max(20, m.width-8)
	contentW := max(1, outerW-4)

	lines := []string{
		lipgloss.NewStyle().Foreground(lipgloss.Color("#ffb347")).Bold(true).Render("Approval required"),
		m.styleBold.Render(kit.TruncateRight(title, contentW)),
		m.styleDim.Render(kit.TruncateRight(desc, contentW)),
		m.styleComposerStatusKey.Copy().Render("press") + " " + m.styleComposerStatusVal.Render("A/Y approve • D/N deny"),
	}

	style := lipgloss.NewStyle().
		Width(contentW).
		Padding(0, 1).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#ffb347")).
		Foreground(lipgloss.Color("#eaeaea"))

	return style.Render(strings.Join(lines, "\n"))
}

func approvalPromptText(req types.HostOpRequest) (string, string) {
	op := strings.ToLower(strings.TrimSpace(req.Op))
	switch op {
	case types.HostOpFSWrite:
		return "Write file", "Path: " + req.Path
	case types.HostOpFSAppend:
		return "Append to file", "Path: " + req.Path
	case types.HostOpFSEdit:
		return "Edit file", "Path: " + req.Path
	case types.HostOpFSPatch:
		return "Patch file", "Path: " + req.Path
	case types.HostOpShellExec:
		cmd := strings.Join(req.Argv, " ")
		if cmd == "" {
			cmd = "<shell command>"
		}
		return "Shell command", "Command: " + cmd
	case types.HostOpHTTPFetch:
		method := strings.ToUpper(strings.TrimSpace(req.Method))
		if method == "" {
			method = "GET"
		}
		return "HTTP request", method + " " + req.URL
	case types.HostOpToolRun:
		title := "Tool run"
		if strings.TrimSpace(req.ToolID.String()) != "" {
			title = "Tool run: " + req.ToolID.String()
		}
		desc := "Action: " + req.ActionID
		return title, desc
	default:
		desc := "Op: " + req.Op
		if strings.TrimSpace(req.Path) != "" {
			desc += " " + req.Path
		}
		return "Host operation", desc
	}
}

func defaultIfEmpty(v, fallback string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return fallback
	}
	return v
}
