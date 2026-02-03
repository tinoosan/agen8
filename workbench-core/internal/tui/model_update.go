package tui

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// Update implements the Bubble Tea update loop.
//
// The implementation lives in `update` to keep this file as the routing surface
// while we split the model into feature-focused files.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return m.update(msg)
}

func (m Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if mm, cmd, ok := m.keyGlobalQuit(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.keyStopTurn(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.keyReasoningSummaryPicker(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.keyReasoningEffortPicker(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.keyHelpModal(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.keyOpenHelpModal(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.keyComposePrefill(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.keySessionPickerModal(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.keyModelPickerModal(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.keyEditorMode(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.keyFilePickerModal(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.keyTranscriptScrollKeys(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.keyToggleActivityPanel(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.keyTogglePlanView(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.keyEscClosesPanels(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.keyTabCyclesFocus(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.keyTelemetryToggle(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.keyThinkingToggle(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.keyToggleMultiline(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.keyActivityPaneFocused(msg); ok {
		return mm, cmd
	}

	// Only forward key events into the input when input is focused.
	if m.focus != focusInput {
		return m, nil
	}
	if mm, cmd, ok := m.keyCommandPaletteNav(msg); ok {
		return mm, cmd
	}
	return m.keyInput(msg)
}

func (m Model) keyReasoningEffortPicker(msg tea.KeyMsg) (Model, tea.Cmd, bool) {
	if !m.reasoningEffortPickerOpen {
		return m, nil, false
	}

	// Capture all keys while open; user can Esc to cancel.
	switch msg.Type {
	case tea.KeyEsc:
		m.closeReasoningEffortPicker()
		return m, nil, true
	case tea.KeyEnter:
		return m, m.selectReasoningEffortFromPicker(), true
	case tea.KeyUp:
		m.reasoningEffortPickerSelected--
		if m.reasoningEffortPickerSelected < 0 {
			m.reasoningEffortPickerSelected = len(reasoningEffortOptions) - 1
		}
		return m, nil, true
	case tea.KeyDown:
		m.reasoningEffortPickerSelected++
		if m.reasoningEffortPickerSelected >= len(reasoningEffortOptions) {
			m.reasoningEffortPickerSelected = 0
		}
		return m, nil, true
	}
	switch msg.String() {
	case "j":
		m.reasoningEffortPickerSelected++
		if m.reasoningEffortPickerSelected >= len(reasoningEffortOptions) {
			m.reasoningEffortPickerSelected = 0
		}
		return m, nil, true
	case "k":
		m.reasoningEffortPickerSelected--
		if m.reasoningEffortPickerSelected < 0 {
			m.reasoningEffortPickerSelected = len(reasoningEffortOptions) - 1
		}
		return m, nil, true
	}
	return m, nil, true
}

func (m Model) keyReasoningSummaryPicker(msg tea.KeyMsg) (Model, tea.Cmd, bool) {
	if !m.reasoningSummaryPickerOpen {
		return m, nil, false
	}

	switch msg.Type {
	case tea.KeyEsc:
		m.closeReasoningSummaryPicker()
		return m, nil, true
	case tea.KeyEnter:
		return m, m.selectReasoningSummaryFromPicker(), true
	case tea.KeyUp:
		m.reasoningSummaryPickerSelected--
		if m.reasoningSummaryPickerSelected < 0 {
			m.reasoningSummaryPickerSelected = len(reasoningSummaryOptions) - 1
		}
		return m, nil, true
	case tea.KeyDown:
		m.reasoningSummaryPickerSelected++
		if m.reasoningSummaryPickerSelected >= len(reasoningSummaryOptions) {
			m.reasoningSummaryPickerSelected = 0
		}
		return m, nil, true
	}
	switch msg.String() {
	case "j":
		m.reasoningSummaryPickerSelected++
		if m.reasoningSummaryPickerSelected >= len(reasoningSummaryOptions) {
			m.reasoningSummaryPickerSelected = 0
		}
		return m, nil, true
	case "k":
		m.reasoningSummaryPickerSelected--
		if m.reasoningSummaryPickerSelected < 0 {
			m.reasoningSummaryPickerSelected = len(reasoningSummaryOptions) - 1
		}
		return m, nil, true
	}
	return m, nil, true
}

func (m Model) keyGlobalQuit(msg tea.KeyMsg) (Model, tea.Cmd, bool) {
	if msg.Type == tea.KeyCtrlC {
		m.quitByCtrlC = true
		return m, tea.Quit, true
	}
	return m, nil, false
}

func (m Model) keyStopTurn(msg tea.KeyMsg) (Model, tea.Cmd, bool) {
	// Ctrl+X stops the current turn (model streaming + host ops) without exiting Workbench.
	if msg.Type != tea.KeyCtrlX && !strings.EqualFold(msg.String(), "ctrl+x") {
		return m, nil, false
	}
	if !m.turnInFlight {
		// Treat as handled to avoid forwarding to the input/editor.
		return m, nil, true
	}
	if m.turnCancelRequested {
		return m, nil, true
	}
	m.turnCancelRequested = true
	if m.turnCancel != nil {
		m.turnCancel()
	}
	return m, nil, true
}

func (m Model) keyHelpModal(msg tea.KeyMsg) (Model, tea.Cmd, bool) {
	if !m.helpModalOpen {
		return m, nil, false
	}
	if msg.Type == tea.KeyEsc {
		m.closeHelpModal()
		return m, nil, true
	}
	// Vim keys.
	switch msg.String() {
	case "j":
		msg = tea.KeyMsg{Type: tea.KeyDown}
	case "k":
		msg = tea.KeyMsg{Type: tea.KeyUp}
	}
	var cmd tea.Cmd
	m.helpViewport, cmd = m.helpViewport.Update(msg)
	m.clampHelpViewport()
	return m, cmd, true
}

func (m Model) keyOpenHelpModal(msg tea.KeyMsg) (Model, tea.Cmd, bool) {
	// Ctrl+P opens help modal (only when input is focused).
	if m.focus == focusInput && (msg.Type == tea.KeyCtrlP || strings.EqualFold(msg.String(), "ctrl+p")) {
		m.openHelpModal()
		return m, nil, true
	}
	return m, nil, false
}

func (m Model) keyComposePrefill(msg tea.KeyMsg) (Model, tea.Cmd, bool) {
	// Ctrl+E opens external editor for composing a message, prefilled from current input.
	// Only active when the input is focused (so we don't override Activity panel shortcuts).
	if m.focus == focusInput && strings.EqualFold(msg.String(), "ctrl+e") {
		return m, m.openComposeEditorPrefill(), true
	}
	return m, nil, false
}

func (m Model) keySessionPickerModal(msg tea.KeyMsg) (Model, tea.Cmd, bool) {
	// Modal: session picker (must capture keys even when input is focused).
	if !m.sessionPickerOpen {
		return m, nil, false
	}

	switch msg.Type {
	case tea.KeyEsc:
		m.closeSessionPicker()
		m.layout()
		return m, nil, true
	case tea.KeyEnter:
		return m, m.selectSessionFromPicker(), true
	case tea.KeyUp:
		m.sessionPickerList.CursorUp()
		return m, nil, true
	case tea.KeyDown:
		m.sessionPickerList.CursorDown()
		return m, nil, true
	case tea.KeyPgUp, tea.KeyCtrlU:
		m.sessionPickerList.CursorUp()
		return m, nil, true
	case tea.KeyPgDown, tea.KeyCtrlF:
		m.sessionPickerList.CursorDown()
		return m, nil, true
	default:
		var cmd tea.Cmd
		m.sessionPickerList, cmd = m.sessionPickerList.Update(msg)
		if m.sessionPickerList.FilteringEnabled() && m.sessionPickerList.FilterState() == list.Filtering {
			m.sessionPickerList.SetFilterText(m.sessionPickerList.FilterValue())
			m.sessionPickerList.SetFilterState(list.Filtering)
			return m, nil, true
		}
		return m, cmd, true
	}
}

func (m Model) keyModelPickerModal(msg tea.KeyMsg) (Model, tea.Cmd, bool) {
	// Modal: model picker (must capture keys even when input is focused).
	if !m.modelPickerOpen {
		return m, nil, false
	}

	switch msg.Type {
	case tea.KeyEsc:
		m.closeModelPicker()
		m.layout()
		return m, nil, true
	case tea.KeyEnter:
		return m, m.selectModelFromPicker(), true
	case tea.KeyUp:
		m.modelPickerList.CursorUp()
		return m, nil, true
	case tea.KeyDown:
		m.modelPickerList.CursorDown()
		return m, nil, true
	case tea.KeyPgUp, tea.KeyCtrlU:
		m.modelPickerList.CursorUp()
		return m, nil, true
	case tea.KeyPgDown, tea.KeyCtrlF:
		m.modelPickerList.CursorDown()
		return m, nil, true
	default:
		var cmd tea.Cmd
		m.modelPickerList, cmd = m.modelPickerList.Update(msg)
		// Make filtering deterministic: bubbles/list filtering returns a command
		// (often via tea.Batch). In unit tests those commands are typically not
		// executed, which means VisibleItems won't reflect the new filter text.
		//
		// Re-apply the filter synchronously from the current filter value, then
		// keep the list in Filtering mode for continued typing.
		if m.modelPickerList.FilteringEnabled() && m.modelPickerList.FilterState() == list.Filtering {
			m.modelPickerList.SetFilterText(m.modelPickerList.FilterValue())
			m.modelPickerList.SetFilterState(list.Filtering)
			return m, nil, true
		}
		return m, cmd, true
	}
}

func (m Model) keyEditorMode(msg tea.KeyMsg) (Model, tea.Cmd, bool) {
	// In-TUI editor mode: capture keys and render a full-screen editor.
	if !m.editorOpen {
		return m, nil, false
	}
	switch msg.Type {
	case tea.KeyEsc:
		if m.editorComposeOnClose {
			txt := m.editorBuf.Value()
			// Best-effort: persist compose buffer to disk (absolute compose path),
			// then load it into the multiline input.
			if strings.TrimSpace(m.editorVPath) != "" && !isVFSMountPath(m.editorVPath) {
				_ = os.MkdirAll(filepath.Dir(m.editorVPath), 0755)
				_ = os.WriteFile(m.editorVPath, []byte(txt), 0644)
			}
			m.single.SetValue("")
			m.isMulti = true
			m.multiline.SetValue(txt)
		}
		m.editorOpen = false
		m.editorVPath = ""
		m.editorDirty = false
		m.editorErr = ""
		m.editorNotice = ""
		m.editorComposeOnClose = false
		m.focus = focusInput
		if m.isMulti {
			m.multiline.Focus()
		} else {
			m.single.Focus()
		}
		m.layout()
		return m, nil, true
	}
	if msg.String() == "ctrl+o" {
		return m, m.saveEditor(), true
	}

	before := m.editorBuf.Value()
	var cmd tea.Cmd
	m.editorBuf, cmd = m.editorBuf.Update(msg)
	if m.editorBuf.Value() != before {
		m.editorDirty = true
		m.editorNotice = ""
	}
	return m, cmd, true
}

func (m Model) keyFilePickerModal(msg tea.KeyMsg) (Model, tea.Cmd, bool) {
	// Modal-ish: file picker (captures navigation/confirm/cancel keys, but lets
	// normal typing continue to flow into the input).
	if !(m.filePickerOpen && m.focus == focusInput) {
		return m, nil, false
	}
	switch msg.Type {
	case tea.KeyEsc:
		m.closeFilePicker()
		m.layout()
		return m, nil, true
	case tea.KeyEnter:
		return m, m.selectFileFromPicker(), true
	case tea.KeyUp:
		m.filePickerList.CursorUp()
		return m, nil, true
	case tea.KeyDown:
		m.filePickerList.CursorDown()
		return m, nil, true
	case tea.KeyPgUp, tea.KeyCtrlU:
		m.filePickerList.CursorUp()
		return m, nil, true
	case tea.KeyPgDown, tea.KeyCtrlF:
		m.filePickerList.CursorDown()
		return m, nil, true
	default:
		return m, nil, false
	}
}

func (m Model) keyTranscriptScrollKeys(msg tea.KeyMsg) (Model, tea.Cmd, bool) {
	// Transcript scrolling (mouse capture is off by default for native selection).
	//
	// These keys scroll the main chat transcript regardless of input focus:
	//   - PgUp/PgDn
	//   - Ctrl+U/Ctrl+F (half-page up/down)
	//
	// When the Activity panel is focused, PgUp/PgDn are routed to the details panel
	// instead (see below).
	if msg.Type != tea.KeyPgUp && msg.Type != tea.KeyPgDown && msg.Type != tea.KeyCtrlU && msg.Type != tea.KeyCtrlF {
		return m, nil, false
	}
	if m.showDetails && m.planTabActive {
		var cmd tea.Cmd
		m.planViewport, cmd = m.planViewport.Update(msg)
		return m, cmd, true
	}
	// When Activity panel is focused, scroll the activity detail panel instead of transcript.
	if m.showDetails && m.focus == focusActivityList {
		var cmd tea.Cmd
		m.activityDetail, cmd = m.activityDetail.Update(msg)
		return m, cmd, true
	}
	var cmd tea.Cmd
	m.transcript, cmd = m.transcript.Update(msg)
	return m, cmd, true
}

func (m Model) keyToggleActivityPanel(msg tea.KeyMsg) (Model, tea.Cmd, bool) {
	// Toggle activity panel open/closed.
	if msg.Type != tea.KeyCtrlA {
		return m, nil, false
	}
	m.showDetails = !m.showDetails
	if m.showDetails {
		m.focus = focusActivityList
		m.single.Blur()
		m.multiline.Blur()
		m.refreshActivityDetail()
	} else {
		m.focus = focusInput
		if m.isMulti {
			m.multiline.Focus()
		} else {
			m.single.Focus()
		}
	}
	m.layout()
	return m, nil, true
}

func (m Model) keyTogglePlanView(msg tea.KeyMsg) (Model, tea.Cmd, bool) {
	// Ctrl+] toggles between Activity and Plan tabs in the right pane (when open).
	if !m.showDetails {
		return m, nil, false
	}
	if strings.EqualFold(msg.String(), "ctrl+]") {
		m.planTabActive = !m.planTabActive
		m.layout()
		return m, nil, true
	}
	return m, nil, false
}

func (m Model) keyEscClosesPanels(msg tea.KeyMsg) (Model, tea.Cmd, bool) {
	// Esc closes command palette first (if open), then file preview, then activity panel.
	if msg.Type != tea.KeyEsc {
		return m, nil, false
	}
	if m.commandPaletteOpen {
		m.commandPaletteOpen = false
		m.commandPaletteMatches = nil
		m.commandPaletteSelected = 0
		m.layout()
		return m, nil, true
	}
	if m.showDetails && m.fileViewOpen {
		m.fileViewOpen = false
		m.fileViewPath = ""
		m.fileViewContent = ""
		m.fileViewTruncated = false
		m.fileViewErr = ""
		m.refreshActivityDetail()
		return m, nil, true
	}
	if m.showDetails {
		m.showDetails = false
		m.focus = focusInput
		if m.isMulti {
			m.multiline.Focus()
		} else {
			m.single.Focus()
		}
		m.layout()
		return m, nil, true
	}
	return m, nil, false
}

func (m Model) keyTabCyclesFocus(msg tea.KeyMsg) (Model, tea.Cmd, bool) {
	// Tab cycles focus between input and activity list.
	if msg.Type != tea.KeyTab {
		return m, nil, false
	}
	if !m.showDetails {
		m.focus = focusInput
		return m, nil, true
	}
	if m.focus == focusInput {
		m.focus = focusActivityList
		m.single.Blur()
		m.multiline.Blur()
	} else {
		m.focus = focusInput
		if m.isMulti {
			m.multiline.Focus()
		} else {
			m.single.Focus()
		}
	}
	return m, nil, true
}

func (m Model) keyTelemetryToggle(msg tea.KeyMsg) (Model, tea.Cmd, bool) {
	// Telemetry toggle (hidden by default).
	// - When input is focused, only Ctrl+T toggles (so normal typing is never hijacked).
	// - When Activity list is focused, allow Ctrl+T and plain "t".
	if (m.focus == focusInput && msg.Type == tea.KeyCtrlT) ||
		(m.focus == focusActivityList && (msg.Type == tea.KeyCtrlT || strings.EqualFold(msg.String(), "t"))) {
		m.showTelemetry = !m.showTelemetry
		m.refreshActivityDetail()
		return m, nil, true
	}
	return m, nil, false
}

func (m Model) keyThinkingToggle(msg tea.KeyMsg) (Model, tea.Cmd, bool) {
	// Thinking summary toggle (Phase 2).
	// Ctrl+T is already used for telemetry, so we use Ctrl+Y.
	if msg.Type == tea.KeyCtrlY || strings.EqualFold(msg.String(), "ctrl+y") {
		m.thinkingExpanded = !m.thinkingExpanded
		wasAtBottom := m.transcript.AtBottom()
		m.rebuildTranscript()
		if wasAtBottom {
			m.transcript.GotoBottom()
		}
		return m, nil, true
	}
	return m, nil, false
}

func (m Model) keyToggleMultiline(msg tea.KeyMsg) (Model, tea.Cmd, bool) {
	// Toggle multiline.
	//
	// Note: Ctrl+J is ASCII LF and is often indistinguishable from Enter in many
	// terminal setups. Use Ctrl+G as the reliable toggle.
	if m.focus == focusInput && msg.Type == tea.KeyCtrlG {
		m.toggleMultiline()
		return m, nil, true
	}
	return m, nil, false
}

func (m Model) keyActivityPaneFocused(msg tea.KeyMsg) (Model, tea.Cmd, bool) {
	// Activity navigation / details scrolling (when focused).
	if !(m.showDetails && m.focus == focusActivityList) {
		return m, nil, false
	}
	switch msg.Type {
	case tea.KeyUp:
		m.activityList.CursorUp()
		m.refreshActivityDetail()
		return m, nil, true
	case tea.KeyDown:
		m.activityList.CursorDown()
		m.refreshActivityDetail()
		return m, nil, true
	case tea.KeyEnter:
		m.expandOutput = !m.expandOutput
		m.refreshActivityDetail()
		return m, nil, true
	}
	switch msg.String() {
	case "j":
		m.activityList.CursorDown()
		m.refreshActivityDetail()
		return m, nil, true
	case "k":
		m.activityList.CursorUp()
		m.refreshActivityDetail()
		return m, nil, true
	case "e", "ctrl+e":
		m.expandOutput = !m.expandOutput
		m.refreshActivityDetail()
		return m, nil, true
	case "o":
		return m, m.openSelectedActivityFile(), true
	case "ctrl+p":
		m.activityList.CursorUp()
		m.refreshActivityDetail()
		return m, nil, true
	case "ctrl+n":
		m.activityList.CursorDown()
		m.refreshActivityDetail()
		return m, nil, true
	}
	switch msg.Type {
	case tea.KeyPgUp, tea.KeyCtrlU, tea.KeyPgDown, tea.KeyCtrlF:
		var cmd tea.Cmd
		m.activityDetail, cmd = m.activityDetail.Update(msg)
		return m, cmd, true
	}
	// Do not forward keys to the input when Activity is focused.
	return m, nil, true
}

func (m Model) keyCommandPaletteNav(msg tea.KeyMsg) (Model, tea.Cmd, bool) {
	// Command palette navigation (Up/Down/Enter) when palette is open.
	// These keys must be intercepted before they reach the textarea.
	if !m.commandPaletteOpen {
		return m, nil, false
	}
	switch msg.Type {
	case tea.KeyUp:
		if len(m.commandPaletteMatches) > 0 {
			m.commandPaletteSelected--
			if m.commandPaletteSelected < 0 {
				m.commandPaletteSelected = len(m.commandPaletteMatches) - 1
			}
		}
		return m, nil, true
	case tea.KeyDown:
		if len(m.commandPaletteMatches) > 0 {
			m.commandPaletteSelected++
			if m.commandPaletteSelected >= len(m.commandPaletteMatches) {
				m.commandPaletteSelected = 0
			}
		}
		return m, nil, true
	case tea.KeyEnter:
		// Autocomplete the selected command then either open a picker or submit.
		m.autocompleteCommand()
		txt := strings.TrimSpace(m.currentInputValue())
		if txt == "" {
			return m, nil, true
		}
		// Use submitSingle/submitMultiline to ensure picker commands are handled correctly.
		// These functions check for commands like /model, /approval, etc. and open pickers.
		// Note: These functions handle clearing the input themselves, so we don't clear here.
		if m.isMulti {
			return m, m.submitMultiline(), true
		}
		return m, m.submitSingle(), true
	}
	return m, nil, false
}

func (m Model) keyInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.isMulti {
		// In multiline mode, Enter inserts newline.
		//
		// Note: many terminals do not distinguish Ctrl+Enter from Enter unless an
		// "extended keys" protocol is enabled. We support:
		//   - Ctrl+Enter when it is exposed by the terminal/driver
		//   - Ctrl+O as a reliable fallback "send" key (avoids Alt+Enter fullscreen on macOS terminals)
		if msg.Type == tea.KeyCtrlO || strings.EqualFold(msg.String(), "ctrl+o") || strings.EqualFold(msg.String(), "ctrl+enter") {
			return m, m.submitMultiline()
		}
		if msg.Type == tea.KeyEnter {
			// Let textarea handle newline.
			var cmd tea.Cmd
			m.multiline, cmd = m.multiline.Update(msg)
			m.updateCommandPalette()
			cmd2 := m.syncFilePickerFromInput()
			if cmd == nil {
				return m, cmd2
			}
			if cmd2 == nil {
				return m, cmd
			}
			return m, tea.Batch(cmd, cmd2)
		}
		var cmd tea.Cmd
		m.multiline, cmd = m.multiline.Update(msg)
		m.updateCommandPalette()
		cmd2 := m.syncFilePickerFromInput()
		if cmd == nil {
			return m, cmd2
		}
		if cmd2 == nil {
			return m, cmd
		}
		return m, tea.Batch(cmd, cmd2)
	}

	// Single-line mode: Enter submits.
	if msg.Type == tea.KeyEnter {
		return m, m.submitSingle()
	}
	var cmd tea.Cmd
	m.single, cmd = m.single.Update(msg)
	// If pasted content includes newlines, switch to multiline.
	if strings.Contains(m.single.Value(), "\n") {
		m.multiline.SetValue(m.single.Value())
		m.single.SetValue("")
		m.isMulti = true
		m.multiline.Focus()
		m.layout()
	}
	m.updateCommandPalette()
	cmd2 := m.syncFilePickerFromInput()
	if cmd == nil {
		return m, cmd2
	}
	if cmd2 == nil {
		return m, cmd
	}
	return m, tea.Batch(cmd, cmd2)
}
