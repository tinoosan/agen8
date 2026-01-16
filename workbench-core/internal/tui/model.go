package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
	"github.com/tinoosan/workbench-core/internal/events"
)

// TurnRunner executes one user turn and returns the agent final response.
//
// The host (internal/app) owns the actual agent loop, memory commit policy,
// and persistence. The TUI calls this interface and renders events as they stream.
type TurnRunner interface {
	RunTurn(ctx context.Context, userMsg string) (final string, err error)
}

type eventMsg events.Event

type turnDoneMsg struct {
	final string
	err   error
}

type Model struct {
	ctx context.Context

	runner TurnRunner
	events <-chan events.Event

	transcript      viewport.Model
	inspectorList   viewport.Model
	inspectorDetail viewport.Model

	transcriptItems []transcriptItem

	inspectorEntries []inspectorEntry
	inspectorSel     int

	single    textarea.Model
	multiline textarea.Model
	isMulti   bool

	width  int
	height int

	showDetails bool

	turnInFlight bool
	turnStarted  time.Time
	turnTitle    string
	turnN        int

	pendingActionIdx       int
	pendingActionText      string
	waitingForAction       bool
	pendingActionIsToolRun bool

	sessionTitle  string
	workflowTitle string

	lastTurnTokensIn  int
	lastTurnTokensOut int
	lastTurnTokens    int
	totalTokens       int
	lastTurnCostUSD   string
	lastTurnDuration  string
	lastTurnSteps     string
	totalCostUSD      float64

	styleHeaderBar lipgloss.Style
	styleHeaderApp lipgloss.Style
	styleHeaderMid lipgloss.Style
	styleHeaderRHS lipgloss.Style

	styleDim             lipgloss.Style
	styleInspectorHeader lipgloss.Style
	styleInspectorRow    lipgloss.Style
	styleInspectorSel    lipgloss.Style

	styleUserBox   lipgloss.Style
	styleUserLabel lipgloss.Style
	styleAgentBox  lipgloss.Style
	styleAgent     lipgloss.Style
	styleAction    lipgloss.Style
	styleTelemetry lipgloss.Style
	styleOutcome   lipgloss.Style
	styleError     lipgloss.Style

	styleInputBox lipgloss.Style
	styleHint     lipgloss.Style
}

type transcriptItemKind int

const (
	transcriptSpacer transcriptItemKind = iota
	transcriptUser
	transcriptAgent
	transcriptAction
	transcriptError
)

type transcriptItem struct {
	kind transcriptItemKind

	// For user/agent/error content (raw, unwrapped).
	text string

	// For action lines.
	actionText        string
	actionCompletion  string
	actionIsToolRun   bool
	actionIsCompleted bool
}

type inspectorEntry struct {
	at      time.Time
	evType  string
	message string
	data    map[string]string
	summary string
	detail  string
}

func New(ctx context.Context, runner TurnRunner, evCh <-chan events.Event) Model {
	main := viewport.New(0, 0)
	main.Style = lipgloss.NewStyle().Padding(0, 2)

	insList := viewport.New(0, 0)
	insList.Style = lipgloss.NewStyle().Padding(0, 1)
	insDetail := viewport.New(0, 0)
	insDetail.Style = lipgloss.NewStyle().Padding(0, 1)

	// Textarea focus styling:
	//
	// The default bubbles/textarea focused style uses a visible cursor-line highlight.
	// For Workbench, we want focus to affect behavior (cursor + key handling) but not
	// introduce a distinct "selected" visual treatment in the input box.
	//
	// So: use identical styles for focused + blurred, and avoid background/reverse
	// effects on the cursor line.
	plainTextAreaStyle := textarea.Style{
		Base:        lipgloss.NewStyle(),
		CursorLine:  lipgloss.NewStyle(),
		EndOfBuffer: lipgloss.NewStyle().Foreground(lipgloss.Color("#404040")),
		LineNumber:  lipgloss.NewStyle().Foreground(lipgloss.Color("#707070")),
		Placeholder: lipgloss.NewStyle().Foreground(lipgloss.Color("#707070")),
		Prompt: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9ad0ff")).
			Bold(true),
		Text: lipgloss.NewStyle().Foreground(lipgloss.Color("#eaeaea")),
	}

	single := textarea.New()
	single.Placeholder = "Type a message…"
	single.Focus()
	single.Prompt = "you> "
	single.ShowLineNumbers = false
	single.SetHeight(1)
	single.CharLimit = 0
	single.KeyMap.InsertNewline.SetEnabled(false) // Enter should submit in single-line mode.
	single.FocusedStyle = plainTextAreaStyle
	single.BlurredStyle = plainTextAreaStyle

	multi := textarea.New()
	multi.Placeholder = "Multiline message (Ctrl+S to send)…"
	multi.Prompt = "…> "
	multi.ShowLineNumbers = false
	multi.CharLimit = 0
	multi.SetHeight(6)
	// Keep prompt dimmer for multiline mode, but still avoid focus highlighting.
	multiStyle := plainTextAreaStyle
	multiStyle.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("#707070"))
	multi.FocusedStyle = multiStyle
	multi.BlurredStyle = multiStyle

	m := Model{
		ctx:             ctx,
		runner:          runner,
		events:          evCh,
		transcript:      main,
		inspectorList:   insList,
		inspectorDetail: insDetail,
		single:          single,
		multiline:       multi,

		styleHeaderBar: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#c0c0c0")),
		styleHeaderApp: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#eaeaea")).
			Bold(true),
		styleHeaderMid: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#c0c0c0")),
		styleHeaderRHS: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9ad0ff")),

		styleDim: lipgloss.NewStyle().Foreground(lipgloss.Color("#707070")),
		styleInspectorHeader: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#707070")).
			Bold(true),
		styleInspectorRow: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#b0b0b0")),
		styleInspectorSel: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#eaeaea")).
			Background(lipgloss.Color("#303030")),

		styleUserLabel: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9ad0ff")).
			Bold(true),
		styleUserBox: lipgloss.NewStyle().
			Padding(0, 1).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#404040")),
		styleAgentBox: lipgloss.NewStyle().
			Padding(0, 1),
		styleAgent: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#eaeaea")),
		styleAction: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#c0c0c0")),
		styleTelemetry: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6b6b6b")),
		styleOutcome: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8a8a8a")),
		styleError: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ff5f5f")).
			Bold(true),

		styleInputBox: lipgloss.NewStyle().
			Padding(0, 1).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#404040")),
		styleHint: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#707070")),
	}

	m.pendingActionIdx = -1
	m.inspectorSel = -1
	return m
}

func (m Model) Init() tea.Cmd {
	return m.waitEvent()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		return m, nil

	case tea.KeyMsg:
		// Global quit.
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}

		// Toggle details panel.
		if msg.Type == tea.KeyTab {
			m.showDetails = !m.showDetails
			m.layout()
			return m, nil
		}
		if msg.Type == tea.KeyCtrlD {
			m.showDetails = !m.showDetails
			if m.showDetails {
				m.ensureInspectorSelection()
				m.refreshInspectorViews()
			}
			m.layout()
			return m, nil
		}

		// Toggle multiline.
		//
		// Note: Ctrl+J is ASCII LF and is often indistinguishable from Enter in many
		// terminal setups. Use Ctrl+G as the reliable toggle.
		if msg.Type == tea.KeyCtrlG {
			m.toggleMultiline()
			return m, nil
		}

		// Inspector navigation.
		if m.showDetails {
			switch msg.Type {
			case tea.KeyUp:
				m.moveInspectorSelection(-1)
				return m, nil
			case tea.KeyDown:
				m.moveInspectorSelection(1)
				return m, nil
			case tea.KeyPgUp, tea.KeyCtrlU:
				var cmd tea.Cmd
				m.inspectorDetail, cmd = m.inspectorDetail.Update(msg)
				return m, cmd
			case tea.KeyPgDown, tea.KeyCtrlF:
				var cmd tea.Cmd
				m.inspectorDetail, cmd = m.inspectorDetail.Update(msg)
				return m, cmd
			}
		}

		if m.turnInFlight {
			// While a turn is running, we allow scrolling but prevent submitting.
			var cmd tea.Cmd
			m.transcript, cmd = m.transcript.Update(msg)
			return m, cmd
		}

		if m.isMulti {
			// In multiline mode, Enter inserts newline.
			//
			// Note: many terminals do not distinguish Ctrl+Enter from Enter unless an
			// "extended keys" protocol is enabled. We support:
			//   - Ctrl+Enter when it is exposed by the terminal/driver
			//   - Ctrl+S as a reliable fallback "send" key
			//   - Alt+Enter when it is exposed
			if msg.Type == tea.KeyCtrlS ||
				strings.EqualFold(msg.String(), "ctrl+enter") ||
				strings.EqualFold(msg.String(), "ctrl+m") ||
				strings.EqualFold(msg.String(), "alt+enter") {
				return m, m.submitMultiline()
			}
			if msg.Type == tea.KeyEnter {
				// Let textarea handle newline.
				var cmd tea.Cmd
				m.multiline, cmd = m.multiline.Update(msg)
				return m, cmd
			}
			var cmd tea.Cmd
			m.multiline, cmd = m.multiline.Update(msg)
			return m, cmd
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
		return m, cmd

	case eventMsg:
		ev := events.Event(msg)
		m.onEvent(ev)
		return m, m.waitEvent()

	case turnDoneMsg:
		m.turnInFlight = false
		if msg.err != nil {
			m.addTranscriptItem(transcriptItem{kind: transcriptError, text: "agent error: " + msg.err.Error()})
			m.addTranscriptItem(transcriptItem{kind: transcriptSpacer})
			m.turnTitle = ""
			return m, nil
		}
		m.addTranscriptItem(transcriptItem{kind: transcriptAgent, text: strings.TrimSpace(msg.final)})
		m.addTranscriptItem(transcriptItem{kind: transcriptSpacer})
		m.turnTitle = ""
		return m, nil
	}

	// Default fallthrough: keep viewport responsive.
	var cmd tea.Cmd
	m.transcript, cmd = m.transcript.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	header := m.renderHeader()
	body := m.renderBody()
	input := m.renderInput()
	return header + "\n" + body + "\n" + input
}

func (m *Model) layout() {
	headerH := 1
	inputH := 5
	if m.isMulti {
		inputH = 10
	}
	bodyH := m.height - headerH - inputH
	if bodyH < 5 {
		bodyH = 5
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
	m.inspectorList.Width = max(32, detailW)
	m.inspectorDetail.Width = max(32, detailW)
	// Split inspector vertically when visible.
	if detailW != 0 {
		listH := max(5, bodyH/2)
		m.inspectorList.Height = listH
		m.inspectorDetail.Height = max(5, bodyH-listH-1)
	}

	m.rebuildTranscript()
	m.transcript.GotoBottom()
	m.refreshInspectorViews()

	m.single.SetWidth(max(20, m.width-6))
	m.multiline.SetWidth(max(20, m.width-6))
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

	header := m.styleInspectorHeader.Render("Inspector") + m.styleDim.Render(" (Tab/Ctrl+D to hide)")
	divider := m.styleDim.Render(strings.Repeat("─", max(1, m.inspectorList.Width)))

	listBox := header + "\n" + divider + "\n" + m.inspectorList.View()
	detailHeader := m.styleDim.Render("Details (PgUp/PgDn scroll)")
	detailBox := detailHeader + "\n" + divider + "\n" + m.inspectorDetail.View()

	detailsBody := lipgloss.JoinVertical(lipgloss.Top, listBox, m.styleDim.Render(strings.Repeat("─", max(1, m.inspectorList.Width))), detailBox)
	detailsBox := lipgloss.NewStyle().
		Width(m.inspectorList.Width + 2).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("#303030")).
		Render(detailsBody)

	return lipgloss.JoinHorizontal(lipgloss.Top, m.transcript.View(), detailsBox)
}

func (m Model) renderInput() string {
	var input string
	if m.isMulti {
		input = m.multiline.View()
	} else {
		input = m.single.View()
	}

	box := m.styleInputBox.Render(input)
	status := m.renderStatusLine()
	hintText := "ctrl+d inspector  ctrl+g multiline  enter send  ctrl+s send (multiline)  ctrl+c quit"
	if m.showDetails {
		hintText = "ctrl+d hide inspector  ↑/↓ select  pgup/pgdn scroll details  ctrl+g multiline  ctrl+s send (multiline)"
	}
	hint := m.styleHint.Render(hintText)
	if status != "" {
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
	return m.styleHint.Render("last: " + strings.Join(parts, " • "))
}

func (m *Model) toggleMultiline() {
	m.isMulti = !m.isMulti
	if m.isMulti {
		m.multiline.SetValue(strings.TrimSpace(m.single.Value()))
		m.single.SetValue("")
		m.multiline.Focus()
	} else {
		m.single.SetValue(strings.TrimSpace(m.multiline.Value()))
		m.multiline.SetValue("")
		m.single.Focus()
	}
	m.layout()
}

func (m *Model) submitSingle() tea.Cmd {
	txt := strings.TrimSpace(m.single.Value())
	m.single.SetValue("")
	if txt == "" {
		return nil
	}
	return m.submit(txt)
}

func (m *Model) submitMultiline() tea.Cmd {
	txt := strings.TrimSpace(m.multiline.Value())
	m.multiline.SetValue("")
	if txt == "" {
		return nil
	}
	return m.submit(txt)
}

func (m *Model) submit(userMsg string) tea.Cmd {
	m.turnInFlight = true
	m.turnStarted = time.Now()
	m.turnTitle = userMsg
	m.turnN++
	m.pendingActionIdx = -1
	m.pendingActionText = ""
	m.waitingForAction = false

	if m.workflowTitle == "" {
		m.workflowTitle = firstLine(userMsg)
	}

	m.addTranscriptItem(transcriptItem{kind: transcriptUser, text: userMsg})

	return func() tea.Msg {
		final, err := m.runner.RunTurn(m.ctx, userMsg)
		_ = final
		return turnDoneMsg{final: final, err: err}
	}
}

func (m *Model) appendDetails(line string) {
	_ = line
}

func (m *Model) addTranscriptItem(it transcriptItem) {
	m.transcriptItems = append(m.transcriptItems, it)
	m.rebuildTranscript()
	m.transcript.GotoBottom()
}

func (m *Model) rebuildTranscript() {
	w := max(40, m.transcript.Width)
	contentW := max(20, w-8)

	lines := make([]string, 0, len(m.transcriptItems))
	for _, it := range m.transcriptItems {
		switch it.kind {
		case transcriptSpacer:
			lines = append(lines, m.styleDim.Render(""))
		case transcriptUser:
			body := m.styleUserLabel.Render("you> ") + wrapText(it.text, contentW)
			lines = append(lines, m.styleUserBox.Render(body))
		case transcriptAgent:
			body := m.styleAgent.Render(wrapText("agent> "+strings.TrimSpace(it.text), contentW))
			lines = append(lines, m.styleAgentBox.Render(body))
		case transcriptError:
			lines = append(lines, m.styleError.Render(wrapText(it.text, contentW)))
		case transcriptAction:
			prefix := "• "
			if it.actionIsToolRun && !it.actionIsCompleted {
				prefix = "• Run "
			}
			if it.actionIsToolRun && it.actionIsCompleted {
				prefix = "• Ran "
			}
			line := m.styleAction.Render(prefix + wrapText(it.actionText, max(20, w-12)))
			if it.actionIsCompleted && strings.TrimSpace(it.actionCompletion) != "" {
				line += "  " + m.styleDim.Render(strings.TrimSpace(it.actionCompletion))
			}
			lines = append(lines, line)
		}
	}

	m.transcript.SetContent(strings.Join(lines, "\n"))
}

func (m *Model) onEvent(ev events.Event) {
	rr := classifyEvent(ev)
	m.inspectorEntries = append(m.inspectorEntries, inspectorEntry{
		at:      time.Now(),
		evType:  ev.Type,
		message: ev.Message,
		data:    ev.Data,
		summary: inspectorSummary(ev),
		detail:  inspectorDetail(ev),
	})
	m.ensureInspectorSelection()
	if m.showDetails {
		m.refreshInspectorViews()
	}

	// Session title can come from run.started event.
	if ev.Type == "run.started" {
		if v := strings.TrimSpace(ev.Data["sessionTitle"]); v != "" {
			m.sessionTitle = v
		}
	}

	// Chrome metrics only (never rendered as transcript lines).
	switch ev.Type {
	case "llm.usage.total":
		m.lastTurnTokensIn = parseInt(ev.Data["input"])
		m.lastTurnTokensOut = parseInt(ev.Data["output"])
		m.lastTurnTokens = parseInt(ev.Data["total"])
		m.totalTokens += m.lastTurnTokens
	case "llm.cost.total":
		m.lastTurnCostUSD = strings.TrimSpace(ev.Data["costUsd"])
		if v := strings.TrimSpace(ev.Data["costUsd"]); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				m.totalCostUSD += f
			}
		}
	case "agent.turn.complete":
		m.lastTurnDuration = strings.TrimSpace(ev.Data["duration"])
		m.lastTurnSteps = strings.TrimSpace(ev.Data["steps"])
	}

	// Chat transcript: only compact action summaries, paired request+response.
	if rr.Class != RenderAction {
		return
	}

	switch ev.Type {
	case "agent.op.request":
		txt := strings.TrimSpace(rr.Text)
		if txt == "" {
			return
		}
		m.pendingActionText = txt
		m.pendingActionIsToolRun = strings.TrimSpace(ev.Data["op"]) == "tool.run"
		m.waitingForAction = true
		m.pendingActionIdx = len(m.transcriptItems)
		m.addTranscriptItem(transcriptItem{
			kind:            transcriptAction,
			actionText:      txt,
			actionIsToolRun: m.pendingActionIsToolRun,
		})
	case "agent.op.response":
		if !m.waitingForAction || m.pendingActionIdx < 0 {
			return
		}
		comp := strings.TrimSpace(rr.Text)
		if m.pendingActionIdx < len(m.transcriptItems) {
			it := m.transcriptItems[m.pendingActionIdx]
			if it.kind == transcriptAction && !it.actionIsCompleted {
				it.actionCompletion = comp
				it.actionIsCompleted = true
				m.transcriptItems[m.pendingActionIdx] = it
			}
		}
		m.pendingActionText = ""
		m.waitingForAction = false
		m.pendingActionIdx = -1
		m.pendingActionIsToolRun = false
		m.rebuildTranscript()
	}
}

func (m Model) waitEvent() tea.Cmd {
	if m.events == nil {
		return nil
	}
	return func() tea.Msg {
		ev, ok := <-m.events
		if !ok {
			return nil
		}
		return eventMsg(ev)
	}
}

func (m *Model) ensureInspectorSelection() {
	if m.inspectorSel >= 0 && m.inspectorSel < len(m.inspectorEntries) {
		return
	}
	if len(m.inspectorEntries) == 0 {
		m.inspectorSel = -1
		return
	}
	m.inspectorSel = len(m.inspectorEntries) - 1
}

func (m *Model) moveInspectorSelection(delta int) {
	if !m.showDetails || len(m.inspectorEntries) == 0 {
		return
	}
	if m.inspectorSel < 0 {
		m.inspectorSel = 0
	} else {
		m.inspectorSel += delta
		if m.inspectorSel < 0 {
			m.inspectorSel = 0
		}
		if m.inspectorSel >= len(m.inspectorEntries) {
			m.inspectorSel = len(m.inspectorEntries) - 1
		}
	}
	m.refreshInspectorViews()
}

func (m *Model) refreshInspectorViews() {
	// List content.
	if m.inspectorList.Width <= 0 {
		return
	}
	var b strings.Builder
	for i, e := range m.inspectorEntries {
		prefix := "  "
		if i == m.inspectorSel {
			prefix = "› "
		}
		line := fmt.Sprintf("%s%s %s", prefix, e.at.Format("15:04:05"), e.summary)
		if i == m.inspectorSel {
			b.WriteString(m.styleInspectorSel.Render(truncateRight(line, max(1, m.inspectorList.Width-2))))
		} else {
			b.WriteString(m.styleInspectorRow.Render(truncateRight(line, max(1, m.inspectorList.Width-2))))
		}
		if i != len(m.inspectorEntries)-1 {
			b.WriteByte('\n')
		}
	}
	m.inspectorList.SetContent(b.String())

	// Detail content for selected.
	if m.inspectorSel >= 0 && m.inspectorSel < len(m.inspectorEntries) {
		w := max(24, m.inspectorDetail.Width-2)
		m.inspectorDetail.SetContent(lipgloss.NewStyle().Width(w).Render(m.inspectorEntries[m.inspectorSel].detail))
	} else {
		m.inspectorDetail.SetContent("")
	}
}

func truncateRight(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if maxLen <= 0 || len(s) <= maxLen {
		return s
	}
	if maxLen < 2 {
		return s[:maxLen]
	}
	return s[:maxLen-1] + "…"
}

func inspectorSummary(ev events.Event) string {
	switch ev.Type {
	case "agent.op.request":
		op := ev.Data["op"]
		path := ev.Data["path"]
		tool := ev.Data["toolId"]
		act := ev.Data["actionId"]
		switch op {
		case "tool.run":
			// Reuse the transcript-friendly renderer, but keep it single-line and short.
			return fmt.Sprintf("%s %s", ev.Type, truncateRight(renderToolRunInspector(tool, act, strings.TrimSpace(ev.Data["input"])), 80))
		default:
			if path != "" {
				return fmt.Sprintf("%s %s %s", ev.Type, op, path)
			}
			return fmt.Sprintf("%s %s", ev.Type, op)
		}
	case "agent.op.response":
		op := ev.Data["op"]
		ok := ev.Data["ok"]
		if call := ev.Data["callId"]; call != "" {
			if len(call) > 8 {
				call = call[:8]
			}
			return fmt.Sprintf("%s %s ok=%s call=%s", ev.Type, op, ok, call)
		}
		return fmt.Sprintf("%s %s ok=%s", ev.Type, op, ok)
	default:
		if strings.TrimSpace(ev.Message) != "" {
			return fmt.Sprintf("%s %s", ev.Type, ev.Message)
		}
		return ev.Type
	}
}

func inspectorDetail(ev events.Event) string {
	var b strings.Builder
	b.WriteString("type: ")
	b.WriteString(ev.Type)
	b.WriteByte('\n')
	b.WriteString("message: ")
	b.WriteString(ev.Message)
	b.WriteByte('\n')
	if len(ev.Data) != 0 {
		b.WriteString("\n")
		b.WriteString("data:\n")
		keys := make([]string, 0, len(ev.Data))
		for k := range ev.Data {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.WriteString("  ")
			b.WriteString(k)
			b.WriteString(": ")
			b.WriteString(ev.Data[k])
			b.WriteByte('\n')
		}
	}
	// Also include pretty JSON for copy/paste in debug mode, but formatted and wrapped.
	raw := struct {
		Type    string            `json:"type"`
		Message string            `json:"message"`
		Data    map[string]string `json:"data,omitempty"`
	}{
		Type:    ev.Type,
		Message: ev.Message,
		Data:    ev.Data,
	}
	if jb, err := json.MarshalIndent(raw, "", "  "); err == nil {
		b.WriteString("\njson:\n")
		b.WriteString(string(jb))
		b.WriteByte('\n')
	}
	return b.String()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func truncateMiddle(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if maxLen <= 0 || len(s) <= maxLen {
		return s
	}
	if maxLen < 8 {
		return s[:maxLen]
	}
	keep := (maxLen - 1) / 2
	return s[:keep] + "…" + s[len(s)-keep:]
}

func firstLine(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return truncateMiddle(s, 48)
}

func wrapText(s string, width int) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.TrimRight(s, "\n")
	if width <= 0 {
		return s
	}
	// Use a dedicated wrapping lib so reflow behaves consistently across width changes.
	return wordwrap.String(s, width)
}

// Run starts the Workbench Bubble Tea program.
func Run(ctx context.Context, runner TurnRunner, evCh <-chan events.Event) error {
	if runner == nil {
		return fmt.Errorf("tui runner is required")
	}
	m := New(ctx, runner, evCh)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
