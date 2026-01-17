package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
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

	transcript     viewport.Model
	activityList   list.Model
	activityDetail viewport.Model

	transcriptItems         []transcriptItem
	transcriptItemStartLine []int
	lastTurnUserItemIdx     int

	activities        []Activity
	activityIndexByID map[string]int
	pendingActivityID string
	activitySeq       int
	expandOutput      bool

	single    textarea.Model
	multiline textarea.Model
	isMulti   bool

	width  int
	height int

	showDetails   bool
	showTelemetry bool

	focus focusTarget

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

	styleDim lipgloss.Style

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

	md *markdownRenderer
}

type focusTarget int

const (
	focusInput focusTarget = iota
	focusActivityList
)

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

func New(ctx context.Context, runner TurnRunner, evCh <-chan events.Event) Model {
	main := viewport.New(0, 0)
	main.Style = lipgloss.NewStyle().Padding(0, 2)
	main.MouseWheelEnabled = true

	details := viewport.New(0, 0)
	details.Style = lipgloss.NewStyle().Padding(0, 1)
	details.MouseWheelEnabled = true

	activity := list.New([]list.Item{}, newActivityDelegate(), 0, 0)
	activity.Title = "Activity"
	activity.SetShowHelp(false)
	activity.SetShowStatusBar(false)
	activity.SetShowPagination(false)
	activity.SetFilteringEnabled(false)
	activity.SetShowFilter(false)
	activity.Styles.Title = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#707070")).
		Bold(true)

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
	multi.Placeholder = "Multiline message (Ctrl+O to send)…"
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
		ctx:               ctx,
		runner:            runner,
		events:            evCh,
		transcript:        main,
		activityList:      activity,
		activityDetail:    details,
		showDetails:       true,
		activityIndexByID: map[string]int{},
		single:            single,
		multiline:         multi,

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

		md:    newMarkdownRenderer(),
		focus: focusInput,
	}

	m.pendingActionIdx = -1
	m.lastTurnUserItemIdx = -1
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

		// Toggle activity panel open/closed.
		if msg.Type == tea.KeyCtrlA {
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
			return m, nil
		}

		// Esc closes the activity panel and returns focus to input.
		if msg.Type == tea.KeyEsc && m.showDetails {
			m.showDetails = false
			m.focus = focusInput
			if m.isMulti {
				m.multiline.Focus()
			} else {
				m.single.Focus()
			}
			m.layout()
			return m, nil
		}

		// Tab cycles focus between input and activity list.
		if msg.Type == tea.KeyTab {
			if !m.showDetails {
				m.focus = focusInput
				return m, nil
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
			return m, nil
		}

		// Telemetry toggle (hidden by default).
		// Use Ctrl+T as the primary toggle so we don't hijack normal typing.
		// For convenience, allow plain "t" when the input is empty.
		if msg.Type == tea.KeyCtrlT || (strings.EqualFold(msg.String(), "t") && m.single.Value() == "" && m.multiline.Value() == "") {
			m.showTelemetry = !m.showTelemetry
			m.refreshActivityDetail()
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

		// Activity navigation / details scrolling (when focused).
		if m.showDetails && m.focus == focusActivityList {
			switch msg.Type {
			case tea.KeyUp:
				m.activityList.CursorUp()
				m.refreshActivityDetail()
				return m, nil
			case tea.KeyDown:
				m.activityList.CursorDown()
				m.refreshActivityDetail()
				return m, nil
			case tea.KeyEnter:
				m.expandOutput = !m.expandOutput
				m.refreshActivityDetail()
				return m, nil
			}
			switch msg.String() {
			case "j":
				m.activityList.CursorDown()
				m.refreshActivityDetail()
				return m, nil
			case "k":
				m.activityList.CursorUp()
				m.refreshActivityDetail()
				return m, nil
			case "e", "ctrl+e":
				m.expandOutput = !m.expandOutput
				m.refreshActivityDetail()
				return m, nil
			case "ctrl+p":
				m.activityList.CursorUp()
				m.refreshActivityDetail()
				return m, nil
			case "ctrl+n":
				m.activityList.CursorDown()
				m.refreshActivityDetail()
				return m, nil
			}
			switch msg.Type {
			case tea.KeyPgUp, tea.KeyCtrlU, tea.KeyPgDown, tea.KeyCtrlF:
				var cmd tea.Cmd
				m.activityDetail, cmd = m.activityDetail.Update(msg)
				return m, cmd
			}
			// Do not forward keys to the input when Activity is focused.
			return m, nil
		}

		if m.turnInFlight {
			// While a turn is running, we allow scrolling but prevent submitting.
			// Mouse scroll handling is done in the global MouseMsg handler below.
		}

		// Only forward key events into the input when input is focused.
		if m.focus != focusInput {
			return m, nil
		}

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
		m.scrollToCurrentTurnStart()
		m.turnTitle = ""
		return m, nil
	}

	// Mouse wheel scrolling:
	// - Always allow scrolling the transcript.
	// - When the Activity pane is open, scroll the Details panel if the cursor is over it,
	//   otherwise scroll the transcript.
	switch msg := msg.(type) {
	case tea.MouseMsg:
		// If details are visible and the mouse is within the right pane, scroll details.
		if m.showDetails && m.activityList.Width() > 0 {
			leftW := m.transcript.Width
			if msg.X >= leftW {
				var cmd tea.Cmd
				m.activityDetail, cmd = m.activityDetail.Update(msg)
				return m, cmd
			}
		}
		var cmd tea.Cmd
		m.transcript, cmd = m.transcript.Update(msg)
		return m, cmd
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
	wasAtBottom := m.transcript.AtBottom()

	// Width-dependent components (header + footer) can wrap when the terminal is narrow.
	// We compute their real rendered heights so the header is never "pushed off" by a
	// footer that becomes taller due to wrapping.
	m.single.SetWidth(max(20, m.width-6))
	m.multiline.SetWidth(max(20, m.width-6))

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

	box := m.styleInputBox.Render(input)
	statusRaw := m.renderStatusLine()
	focusName := "input"
	if m.focus == focusActivityList {
		focusName = "activity"
	}

	hintText := "ctrl+a activity  tab focus  ctrl+g multiline  enter send  ctrl+c quit"
	if m.showDetails {
		hintText = "ctrl+a hide activity  tab focus  esc close  j/k↑/↓ select  e/enter expand  pgup/pgdn scroll details  ctrl+t telemetry  ctrl+g multiline  ctrl+o send (multiline)"
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

	m.lastTurnUserItemIdx = len(m.transcriptItems)
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
	wasAtBottom := m.transcript.AtBottom()
	wasEmpty := len(m.transcriptItems) == 0

	m.transcriptItems = append(m.transcriptItems, it)
	m.rebuildTranscript()
	// If the user was at the bottom, keep them there (chat behavior). Otherwise,
	// preserve their scroll position.
	if wasEmpty {
		// For the first item, keep the top visible (avoid "first message is cut off").
		m.transcript.SetYOffset(0)
	} else if wasAtBottom {
		m.transcript.GotoBottom()
	}
}

func (m *Model) rebuildTranscript() {
	w := max(40, m.transcript.Width)
	contentW := max(20, w-8)

	lines := make([]string, 0, len(m.transcriptItems))
	startLines := make([]int, 0, len(m.transcriptItems))
	lineNo := 0
	for _, it := range m.transcriptItems {
		startLines = append(startLines, lineNo)
		switch it.kind {
		case transcriptSpacer:
			lines = append(lines, m.styleDim.Render(""))
			lineNo++
		case transcriptUser:
			// Render user text as markdown so pasted tasks and lists are readable.
			body := m.styleUserLabel.Render("you> ") + strings.TrimRight(m.md.render(it.text, contentW), "\n")
			lines = append(lines, m.styleUserBox.Render(body))
			lineNo += 1 + strings.Count(lines[len(lines)-1], "\n")
		case transcriptAgent:
			// Render agent answers as markdown (code blocks, bullets, tables).
			//
			// Important: do not prefix "agent>" inside the markdown source, otherwise
			// fenced blocks (```json) stop being recognized by the markdown parser.
			rendered := strings.TrimRight(m.md.render(strings.TrimSpace(it.text), contentW), "\n")
			rendered = prefixFirstLine(rendered, "agent> ")
			body := m.styleAgent.Render(rendered)
			lines = append(lines, m.styleAgentBox.Render(body))
			lineNo += 1 + strings.Count(lines[len(lines)-1], "\n")
		case transcriptError:
			lines = append(lines, m.styleError.Render(wrapText(it.text, contentW)))
			lineNo += 1 + strings.Count(lines[len(lines)-1], "\n")
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
			lineNo += 1 + strings.Count(lines[len(lines)-1], "\n")
		}
	}

	m.transcriptItemStartLine = startLines
	m.transcript.SetContent(strings.Join(lines, "\n"))
	// Clamp scroll when content shrinks or re-wrap changes line count.
	m.transcript.SetYOffset(m.transcript.YOffset)
}

func prefixFirstLine(s string, prefix string) string {
	if s == "" {
		return prefix
	}
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return prefix + s[:idx] + "\n" + s[idx+1:]
	}
	return prefix + s
}

func (m *Model) scrollToCurrentTurnStart() {
	// Keep the current turn "anchored" so the user message + action lines remain visible
	// even when the agent answer is long.
	idx := m.lastTurnUserItemIdx
	if idx < 0 || idx >= len(m.transcriptItemStartLine) {
		return
	}
	m.transcript.SetYOffset(m.transcriptItemStartLine[idx])
}

func (m *Model) onEvent(ev events.Event) {
	rr := classifyEvent(ev)
	m.observeActivityEvent(ev)

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

func (m *Model) observeActivityEvent(ev events.Event) {
	switch ev.Type {
	case "agent.op.request":
		op := strings.TrimSpace(ev.Data["op"])
		if op == "" {
			return
		}
		m.activitySeq++
		id := fmt.Sprintf("act-%d", m.activitySeq)
		now := time.Now()

		act := Activity{
			ID:            id,
			Kind:          op,
			Status:        ActivityPending,
			StartedAt:     now,
			Path:          strings.TrimSpace(ev.Data["path"]),
			MaxBytes:      strings.TrimSpace(ev.Data["maxBytes"]),
			ToolID:        strings.TrimSpace(ev.Data["toolId"]),
			ActionID:      strings.TrimSpace(ev.Data["actionId"]),
			InputJSON:     strings.TrimSpace(ev.Data["input"]),
			TextPreview:   strings.TrimSpace(ev.Data["textPreview"]),
			TextTruncated: strings.TrimSpace(ev.Data["textTruncated"]) == "true",
			TextRedacted:  strings.TrimSpace(ev.Data["textRedacted"]) == "true",
			TextIsJSON:    strings.TrimSpace(ev.Data["textIsJSON"]) == "true",
			TextBytes:     strings.TrimSpace(ev.Data["textBytes"]),
		}
		if op == "tool.run" {
			act.Command = strings.TrimSpace(renderToolRunTranscript(act.ToolID, act.ActionID, act.InputJSON))
			if act.Command != "" {
				act.Title = "Run " + act.Command
			} else {
				act.Title = "Run tool"
			}
		} else {
			act.Title = renderOpRequest(ev.Data)
		}

		m.pendingActivityID = id
		m.activities = append(m.activities, act)
		m.activityIndexByID[id] = len(m.activities) - 1
		m.refreshActivityList()
		m.activityList.Select(len(m.activities) - 1)
		m.refreshActivityDetail()

	case "agent.op.response":
		if strings.TrimSpace(ev.Data["op"]) == "" {
			return
		}
		idx, ok := m.activityIndexByID[m.pendingActivityID]
		if !ok || idx < 0 || idx >= len(m.activities) {
			return
		}
		act := m.activities[idx]
		now := time.Now()

		act.Ok = strings.TrimSpace(ev.Data["ok"])
		act.Error = strings.TrimSpace(ev.Data["err"])
		act.CallID = strings.TrimSpace(ev.Data["callId"])
		act.OutputPreview = strings.TrimSpace(ev.Data["outputPreview"])

		fin := now
		act.FinishedAt = &fin
		act.Duration = fin.Sub(act.StartedAt)
		if act.Ok == "true" {
			act.Status = ActivityOK
		} else {
			act.Status = ActivityError
		}

		m.activities[idx] = act
		m.pendingActivityID = ""
		m.refreshActivityList()
		m.refreshActivityDetail()
	}
}

func (m *Model) refreshActivityList() {
	items := make([]list.Item, 0, len(m.activities))
	for _, a := range m.activities {
		items = append(items, activityItem{act: a})
	}
	cur := m.activityList.Index()
	m.activityList.SetItems(items)
	if cur >= 0 && cur < len(items) {
		m.activityList.Select(cur)
	}
}

func (m *Model) refreshActivityDetail() {
	if !m.showDetails {
		return
	}
	if len(m.activities) == 0 || m.activityList.Index() < 0 || m.activityList.Index() >= len(m.activities) {
		m.activityDetail.SetContent("")
		return
	}
	act := m.activities[m.activityList.Index()]
	w := max(24, m.activityDetail.Width-4)
	md := renderActivityDetailMarkdown(act, m.showTelemetry, m.expandOutput)
	telemetryBadge := ""
	if m.showTelemetry {
		telemetryBadge = " _(telemetry on)_"
	}
	header := "### Details" + telemetryBadge + "\n\n"
	help := "_PgUp/PgDn scroll · Ctrl+E expand · Ctrl+T telemetry_\n\n"
	m.activityDetail.SetContent(strings.TrimRight(m.md.render(header+help+md, w), "\n"))
}

func renderActivityDetailMarkdown(a Activity, telemetry bool, expanded bool) string {
	var b strings.Builder
	b.WriteString("## ")
	b.WriteString(a.Kind)
	b.WriteString(" ")
	b.WriteString(a.ShortStatus())
	if a.Duration > 0 {
		b.WriteString(" · ")
		b.WriteString(a.Duration.String())
	}
	b.WriteString("\n\n")

	b.WriteString("**Summary**\n\n")
	b.WriteString("- ")
	b.WriteString(a.Title)
	b.WriteString("\n\n")

	b.WriteString("**Inputs**\n\n")
	if a.Kind == "tool.run" {
		if strings.TrimSpace(a.ToolID) != "" || strings.TrimSpace(a.ActionID) != "" {
			b.WriteString("- tool: ")
			b.WriteString(fmt.Sprintf("%s/%s", a.ToolID, a.ActionID))
			b.WriteString("\n")
		}
		if strings.TrimSpace(a.Command) != "" {
			b.WriteString("- cmd:\n\n```sh\n")
			b.WriteString(a.Command)
			b.WriteString("\n```\n")
		}
		if strings.TrimSpace(a.InputJSON) != "" {
			b.WriteString("\n- args:\n\n```json\n")
			b.WriteString(prettyJSONOneLine(a.InputJSON))
			b.WriteString("\n```\n")
		}
	} else {
		if strings.TrimSpace(a.Path) != "" {
			b.WriteString("- path: `")
			b.WriteString(a.Path)
			b.WriteString("`\n")
		}
		if telemetry && strings.TrimSpace(a.MaxBytes) != "" {
			b.WriteString("- maxBytes: ")
			b.WriteString(a.MaxBytes)
			b.WriteString("\n")
		}
		if telemetry && strings.TrimSpace(a.TextBytes) != "" && (a.Kind == "fs.write" || a.Kind == "fs.append") {
			b.WriteString("- textBytes: ")
			b.WriteString(a.TextBytes)
			b.WriteString("\n")
		}

		if a.Kind == "fs.write" || a.Kind == "fs.append" {
			if a.TextRedacted {
				b.WriteString("\n- content: _(redacted)_\n")
			} else if strings.TrimSpace(a.TextPreview) != "" {
				lang := guessCodeFenceLang(a.Path, a.TextIsJSON)
				if a.TextTruncated {
					b.WriteString("\n- content preview: _(truncated)_\n\n```" + lang + "\n")
				} else {
					b.WriteString("\n- content preview:\n\n```" + lang + "\n")
				}
				b.WriteString(a.TextPreview)
				if !strings.HasSuffix(a.TextPreview, "\n") {
					b.WriteString("\n")
				}
				b.WriteString("```\n")
			}
		}
	}
	b.WriteString("\n**Outputs**\n\n")
	if strings.TrimSpace(a.CallID) != "" {
		b.WriteString("- callId: `")
		b.WriteString(a.CallID)
		b.WriteString("`\n")
	}
	if strings.TrimSpace(a.Error) != "" {
		b.WriteString("- error: ")
		b.WriteString(a.Error)
		b.WriteString("\n")
	}
	if strings.TrimSpace(a.OutputPreview) != "" {
		txt := a.OutputPreview
		if !expanded && len(txt) > 600 {
			txt = txt[:599] + "…"
		}
		b.WriteString("\n**Preview** _(press `e` to expand)_\n\n```text\n")
		b.WriteString(txt)
		b.WriteString("\n```\n")
	}

	return b.String()
}

func guessCodeFenceLang(path string, isJSON bool) string {
	if isJSON {
		return "json"
	}
	low := strings.ToLower(strings.TrimSpace(path))
	switch {
	case strings.HasSuffix(low, ".md"):
		return "md"
	case strings.HasSuffix(low, ".go"):
		return "go"
	case strings.HasSuffix(low, ".sh"):
		return "sh"
	case strings.HasSuffix(low, ".js"):
		return "js"
	case strings.HasSuffix(low, ".ts"):
		return "ts"
	case strings.HasSuffix(low, ".html"), strings.HasSuffix(low, ".htm"):
		return "html"
	default:
		return "text"
	}
}

func prettyJSONOneLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return s
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return s
	}
	return string(b)
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
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseAllMotion())
	_, err := p.Run()
	return err
}
