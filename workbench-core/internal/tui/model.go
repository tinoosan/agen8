package tui

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/workbench-core/internal/events"
	"github.com/tinoosan/workbench-core/internal/vfsutil"
)

// TurnRunner executes one user turn and returns the agent final response.
//
// The host (internal/app) owns the actual agent loop, memory commit policy,
// and persistence. The TUI calls this interface and renders events as they stream.
type TurnRunner interface {
	RunTurn(ctx context.Context, userMsg string) (final string, err error)
}

// vfsAccessor is an optional extension interface implemented by the app TurnRunner.
//
// The chat transcript is driven by RunTurn(), but some UI workflows (like the in-TUI
// editor and artifact preview) need direct access to mounted VFS paths.
//
// This keeps the agent loop contract unchanged: the editor is a host UX feature.
type vfsAccessor interface {
	ReadVFS(ctx context.Context, path string, maxBytes int) (text string, bytesLen int, truncated bool, err error)
	WriteVFS(ctx context.Context, path string, data []byte) error
}

type eventMsg events.Event

type turnDoneMsg struct {
	final string
	err   error
}

type fileViewMsg struct {
	path      string
	content   string
	truncated bool
	err       error
}

type editorLoadMsg struct {
	vpath   string
	content string
	notice  string
	err     error
}

type editorSaveMsg struct {
	vpath string
	err   error
}

type editorExternalDoneMsg struct {
	vpath string
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

	fileViewOpen      bool
	fileViewPath      string
	fileViewContent   string
	fileViewTruncated bool
	fileViewErr       string

	editorOpen   bool
	editorVPath  string
	editorBuf    textarea.Model
	editorDirty  bool
	editorErr    string
	editorNotice string

	sessionTitle  string
	workflowTitle string
	workdir       string
	modelID       string
	sessionID     string
	runID         string

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

	styleInputBox            lipgloss.Style
	styleComposerCardFocused lipgloss.Style
	styleComposerCardBlurred lipgloss.Style
	styleComposerAccentFocus lipgloss.Style
	styleComposerAccentBlur  lipgloss.Style
	styleComposerStatusKey   lipgloss.Style
	styleComposerStatusVal   lipgloss.Style
	styleHint                lipgloss.Style

	renderer *ContentRenderer

	// Command palette state
	commandPaletteOpen     bool
	commandPaletteMatches  []string
	commandPaletteSelected int
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

// Hardcoded list of available slash commands for the command palette.
var availableCommands = []string{
	"/model",
	"/open",
	"/editor",
	"/cd",
	"/pwd",
	"/workdir",
}

func isExactCommand(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	for _, cmd := range availableCommands {
		if s == cmd {
			return true
		}
	}
	return false
}

// updateCommandPalette updates the command palette state based on the current input value.
// It detects if the input starts with "/" and filters commands accordingly.
func (m *Model) updateCommandPalette() {
	var inputValue string
	if m.isMulti {
		inputValue = m.multiline.Value()
	} else {
		inputValue = m.single.Value()
	}

	// Extract the first token (command part) from the input.
	fields := strings.Fields(inputValue)
	var firstToken string
	if len(fields) > 0 {
		firstToken = fields[0]
	} else {
		// Empty input or only whitespace - use the raw value.
		firstToken = strings.TrimSpace(inputValue)
	}

	// Check if we're in command mode (starts with "/").
	if strings.HasPrefix(firstToken, "/") {
		// If the user has already completed a valid command token and is now typing
		// arguments (i.e. there is whitespace after the first token), keep the palette closed.
		if isExactCommand(firstToken) && strings.ContainsAny(inputValue, " \t\n") {
			m.commandPaletteOpen = false
			m.commandPaletteMatches = nil
			m.commandPaletteSelected = 0
			return
		}

		// Filter commands that match the typed prefix.
		matches := []string{}
		for _, cmd := range availableCommands {
			if strings.HasPrefix(cmd, firstToken) {
				matches = append(matches, cmd)
			}
		}

		if len(matches) > 0 {
			m.commandPaletteOpen = true
			m.commandPaletteMatches = matches
			// Ensure selected index is valid.
			if m.commandPaletteSelected >= len(matches) {
				m.commandPaletteSelected = 0
			}
			if m.commandPaletteSelected < 0 {
				m.commandPaletteSelected = 0
			}
		} else {
			// No matches, close palette.
			m.commandPaletteOpen = false
			m.commandPaletteMatches = nil
			m.commandPaletteSelected = 0
		}
	} else {
		// Not in command mode, close palette.
		m.commandPaletteOpen = false
		m.commandPaletteMatches = nil
		m.commandPaletteSelected = 0
	}
}

// autocompleteCommand replaces the first token in the input with the selected command,
// preserving any trailing arguments.
func (m *Model) autocompleteCommand() {
	if !m.commandPaletteOpen || len(m.commandPaletteMatches) == 0 {
		return
	}
	if m.commandPaletteSelected < 0 || m.commandPaletteSelected >= len(m.commandPaletteMatches) {
		return
	}

	selectedCmd := m.commandPaletteMatches[m.commandPaletteSelected]

	var inputValue string
	if m.isMulti {
		inputValue = m.multiline.Value()
	} else {
		inputValue = m.single.Value()
	}

	// Extract the first token and any trailing args.
	fields := strings.Fields(inputValue)
	if len(fields) == 0 {
		// Empty input, just set the command.
		if m.isMulti {
			m.multiline.SetValue(selectedCmd)
		} else {
			m.single.SetValue(selectedCmd)
		}
	} else {
		// Replace first token with selected command, preserve rest.
		rest := strings.TrimSpace(strings.TrimPrefix(inputValue, fields[0]))
		newValue := selectedCmd
		if rest != "" {
			newValue = selectedCmd + " " + rest
		}
		if m.isMulti {
			m.multiline.SetValue(newValue)
		} else {
			m.single.SetValue(newValue)
		}
	}

	// Close palette after autocomplete.
	m.commandPaletteOpen = false
	m.commandPaletteMatches = nil
	m.commandPaletteSelected = 0
}

func New(ctx context.Context, runner TurnRunner, evCh <-chan events.Event) Model {
	main := viewport.New(0, 0)
	// Important: avoid horizontal padding on viewports.
	//
	// If viewport content becomes wider than the terminal (due to padding + borders
	// in transcript elements), the terminal will soft-wrap long lines. That increases
	// the effective number of screen lines and can make the header appear to
	// "disappear" (scrolled off the top) on resize or when toggling the sidebar.
	main.Style = lipgloss.NewStyle()
	main.MouseWheelEnabled = true

	details := viewport.New(0, 0)
	details.Style = lipgloss.NewStyle()
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

	editor := textarea.New()
	editor.Placeholder = ""
	editor.Prompt = ""
	editor.ShowLineNumbers = true
	editor.CharLimit = 0
	editor.SetHeight(1)
	editorStyle := plainTextAreaStyle
	editorStyle.Prompt = lipgloss.NewStyle()
	editor.FocusedStyle = editorStyle
	editor.BlurredStyle = editorStyle

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
		editorBuf:         editor,

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
		styleComposerCardFocused: lipgloss.NewStyle().
			Margin(0, 1).
			Padding(0, 1).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#6bbcff")),
		styleComposerCardBlurred: lipgloss.NewStyle().
			Margin(0, 1).
			Padding(0, 1).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#404040")),
		styleComposerAccentFocus: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6bbcff")).
			Bold(true),
		styleComposerAccentBlur: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#404040")),
		styleComposerStatusKey: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9ad0ff")).
			Bold(true),
		styleComposerStatusVal: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#eaeaea")),
		styleHint: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#707070")),

		renderer: newContentRenderer(),
		focus:    focusInput,
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

		// In-TUI editor mode: capture keys and render a full-screen editor.
		if m.editorOpen {
			switch msg.Type {
			case tea.KeyEsc:
				m.editorOpen = false
				m.editorVPath = ""
				m.editorDirty = false
				m.editorErr = ""
				m.editorNotice = ""
				m.focus = focusInput
				if m.isMulti {
					m.multiline.Focus()
				} else {
					m.single.Focus()
				}
				m.layout()
				return m, nil
			}
			if msg.String() == "ctrl+o" {
				return m, m.saveEditor()
			}

			before := m.editorBuf.Value()
			var cmd tea.Cmd
			m.editorBuf, cmd = m.editorBuf.Update(msg)
			if m.editorBuf.Value() != before {
				m.editorDirty = true
				m.editorNotice = ""
			}
			return m, cmd
		}

		// Transcript scrolling (mouse capture is off by default for native selection).
		//
		// These keys scroll the main chat transcript regardless of input focus:
		//   - PgUp/PgDn
		//   - Ctrl+U/Ctrl+F (half-page up/down)
		//
		// When the Activity panel is focused, PgUp/PgDn are routed to the details panel
		// instead (see below).
		if msg.Type == tea.KeyPgUp || msg.Type == tea.KeyPgDown || msg.Type == tea.KeyCtrlU || msg.Type == tea.KeyCtrlF {
			// If Activity is focused, let the details panel consume these keys.
			if m.showDetails && m.focus == focusActivityList {
				break
			}
			var cmd tea.Cmd
			m.transcript, cmd = m.transcript.Update(msg)
			return m, cmd
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

		// Esc closes command palette first (if open), then file preview, then activity panel.
		if msg.Type == tea.KeyEsc {
			if m.commandPaletteOpen {
				m.commandPaletteOpen = false
				m.commandPaletteMatches = nil
				m.commandPaletteSelected = 0
				return m, nil
			}
			if m.showDetails && m.fileViewOpen {
				m.fileViewOpen = false
				m.fileViewPath = ""
				m.fileViewContent = ""
				m.fileViewTruncated = false
				m.fileViewErr = ""
				m.refreshActivityDetail()
				return m, nil
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
				return m, nil
			}
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
			case "o":
				return m, m.openSelectedActivityFile()
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

		// Command palette navigation (Up/Down/Enter) when palette is open.
		// These keys must be intercepted before they reach the textarea.
		if m.commandPaletteOpen {
			switch msg.Type {
			case tea.KeyUp:
				if len(m.commandPaletteMatches) > 0 {
					m.commandPaletteSelected--
					if m.commandPaletteSelected < 0 {
						m.commandPaletteSelected = len(m.commandPaletteMatches) - 1
					}
				}
				return m, nil
			case tea.KeyDown:
				if len(m.commandPaletteMatches) > 0 {
					m.commandPaletteSelected++
					if m.commandPaletteSelected >= len(m.commandPaletteMatches) {
						m.commandPaletteSelected = 0
					}
				}
				return m, nil
			case tea.KeyEnter:
				// Autocomplete the selected command (do not submit).
				m.autocompleteCommand()
				return m, nil
			}
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
				m.updateCommandPalette()
				return m, cmd
			}
			var cmd tea.Cmd
			m.multiline, cmd = m.multiline.Update(msg)
			m.updateCommandPalette()
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
		m.updateCommandPalette()
		return m, cmd

	case eventMsg:
		ev := events.Event(msg)
		m.onEvent(ev)
		cmds := []tea.Cmd{m.waitEvent()}
		if ev.Type == "ui.editor.open" {
			if v := strings.TrimSpace(ev.Data["vpath"]); v != "" {
				cmds = append(cmds, m.openEditor(v))
			}
		}
		return m, tea.Batch(cmds...)

	case editorLoadMsg:
		if strings.TrimSpace(msg.vpath) == "" || msg.vpath != m.editorVPath {
			return m, nil
		}
		if msg.err != nil {
			m.editorErr = msg.err.Error()
			m.editorBuf.SetValue("")
			m.editorDirty = false
			m.editorNotice = ""
		} else {
			m.editorErr = ""
			m.editorBuf.SetValue(msg.content)
			m.editorDirty = false
			m.editorNotice = strings.TrimSpace(msg.notice)
		}
		m.layout()
		return m, nil

	case editorSaveMsg:
		if strings.TrimSpace(msg.vpath) == "" || msg.vpath != m.editorVPath {
			return m, nil
		}
		if msg.err != nil {
			m.editorErr = msg.err.Error()
			return m, nil
		}
		m.editorErr = ""
		m.editorNotice = "saved"
		m.editorDirty = false
		return m, nil

	case editorExternalDoneMsg:
		if strings.TrimSpace(msg.vpath) == "" {
			return m, nil
		}
		if msg.err != nil {
			m.addTranscriptItem(transcriptItem{kind: transcriptError, text: "editor error: " + msg.err.Error()})
			m.addTranscriptItem(transcriptItem{kind: transcriptSpacer})
			m.rebuildTranscript()
		}
		m.focus = focusInput
		if m.isMulti {
			m.multiline.Focus()
		} else {
			m.single.Focus()
		}
		return m, nil

	case fileViewMsg:
		// Ignore stale responses.
		if strings.TrimSpace(msg.path) != "" && msg.path != m.fileViewPath {
			return m, nil
		}
		if msg.err != nil {
			m.fileViewErr = msg.err.Error()
			m.fileViewContent = ""
			m.fileViewTruncated = false
		} else {
			m.fileViewErr = ""
			m.fileViewContent = msg.content
			m.fileViewTruncated = msg.truncated
		}
		m.refreshActivityDetail()
		return m, nil

	case turnDoneMsg:
		m.turnInFlight = false
		if msg.err != nil {
			m.addTranscriptItem(transcriptItem{kind: transcriptError, text: "agent error: " + msg.err.Error()})
			m.addTranscriptItem(transcriptItem{kind: transcriptSpacer})
			m.turnTitle = ""
			return m, nil
		}
		if strings.TrimSpace(msg.final) != "" {
			m.addTranscriptItem(transcriptItem{kind: transcriptAgent, text: strings.TrimSpace(msg.final)})
			m.addTranscriptItem(transcriptItem{kind: transcriptSpacer})
		}
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
	if m.editorOpen {
		return m.renderEditorView()
	}
	header := m.renderHeader()
	body := m.renderBody()
	input := m.renderInput()
	return header + "\n" + body + "\n" + input
}

func (m Model) renderEditorView() string {
	header := m.renderHeader()

	title := m.editorTitle()
	w := max(1, m.width-2)
	bar := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#c0c0c0")).
		Padding(0, 1).
		Render(truncateMiddle(title, w))

	bodyH := max(1, m.height-lipgloss.Height(header)-lipgloss.Height(bar)-2)
	m.editorBuf.SetHeight(bodyH)
	m.editorBuf.SetWidth(max(1, m.width-2))
	editor := lipgloss.NewStyle().Padding(0, 1).Render(m.editorBuf.View())

	footer := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#707070")).
		Padding(0, 1).
		Render("ctrl+o save  esc cancel")

	return header + "\n" + bar + "\n" + editor + "\n" + footer
}

func (m Model) editorTitle() string {
	vp := strings.TrimSpace(m.editorVPath)
	name := vp
	switch {
	case strings.HasPrefix(vp, "/workdir/"):
		name = strings.TrimPrefix(vp, "/workdir/")
	case strings.HasPrefix(vp, "/workspace/"):
		name = strings.TrimPrefix(vp, "/workspace/")
	}
	title := "Editing: " + name
	if m.editorDirty {
		title += " *"
	}
	if strings.TrimSpace(m.editorNotice) != "" {
		title += " · " + strings.TrimSpace(m.editorNotice)
	}
	if strings.TrimSpace(m.editorErr) != "" {
		title += " · error: " + strings.TrimSpace(m.editorErr)
	}
	return title
}

func (m *Model) openEditor(vpath string) tea.Cmd {
	// Prefer the user's external editor when configured.
	//
	// /editor is a host UX convenience command; it should open $VISUAL/$EDITOR
	// rather than forcing users into a bespoke in-TUI editor.
	editor := strings.TrimSpace(os.Getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if editor != "" {
		if cmd, err := m.editorExecCmd(editor, vpath); err == nil && cmd != nil {
			return tea.ExecProcess(cmd, func(err error) tea.Msg {
				return editorExternalDoneMsg{vpath: vpath, err: err}
			})
		}
	}
	return m.openInternalEditor(vpath)
}

func (m *Model) openInternalEditor(vpath string) tea.Cmd {
	m.editorOpen = true
	m.editorVPath = strings.TrimSpace(vpath)
	m.editorDirty = false
	m.editorErr = ""
	m.editorNotice = ""
	m.editorBuf.SetValue("")
	m.editorBuf.Focus()
	m.layout()

	return func() tea.Msg {
		acc, ok := m.runner.(vfsAccessor)
		if !ok {
			return editorLoadMsg{vpath: vpath, err: fmt.Errorf("vfs access not available")}
		}
		txt, _, truncated, err := acc.ReadVFS(m.ctx, vpath, 512*1024)
		if err != nil {
			// Missing file is a valid workflow: open a new file and allow saving it.
			if errors.Is(err, fs.ErrNotExist) || errors.Is(err, os.ErrNotExist) {
				return editorLoadMsg{vpath: vpath, content: "", notice: "new file"}
			}
			return editorLoadMsg{vpath: vpath, err: err}
		}
		if truncated {
			return editorLoadMsg{vpath: vpath, err: fmt.Errorf("file too large to edit in TUI (truncated)")}
		}
		return editorLoadMsg{vpath: vpath, content: txt}
	}
}

func (m *Model) editorExecCmd(editor string, vpath string) (*exec.Cmd, error) {
	editor = strings.TrimSpace(editor)
	vpath = strings.TrimSpace(vpath)
	if editor == "" || vpath == "" {
		return nil, fmt.Errorf("editor and vpath are required")
	}
	if !strings.HasPrefix(vpath, "/workdir/") {
		return nil, fmt.Errorf("external editor only supports /workdir paths")
	}
	workdir := strings.TrimSpace(m.workdir)
	if workdir == "" {
		return nil, fmt.Errorf("workdir is unknown")
	}

	sub := strings.TrimPrefix(vpath, "/workdir/")
	clean, _, err := vfsutil.NormalizeResourceSubpath(sub)
	if err != nil || clean == "" || clean == "." {
		return nil, fmt.Errorf("invalid workdir path: %s", vpath)
	}
	abs := filepath.Join(workdir, filepath.FromSlash(clean))

	fields := strings.Fields(editor)
	if len(fields) == 0 {
		return nil, fmt.Errorf("invalid editor")
	}
	cmd := exec.CommandContext(m.ctx, fields[0], append(fields[1:], abs)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd, nil
}

func (m *Model) saveEditor() tea.Cmd {
	vpath := strings.TrimSpace(m.editorVPath)
	if vpath == "" {
		return nil
	}
	data := []byte(m.editorBuf.Value())
	return func() tea.Msg {
		acc, ok := m.runner.(vfsAccessor)
		if !ok {
			return editorSaveMsg{vpath: vpath, err: fmt.Errorf("vfs access not available")}
		}
		if err := acc.WriteVFS(m.ctx, vpath, data); err != nil {
			return editorSaveMsg{vpath: vpath, err: err}
		}
		return editorSaveMsg{vpath: vpath}
	}
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

func (m *Model) onEvent(ev events.Event) {
	rr := classifyEvent(ev)
	m.observeActivityEvent(ev)

	// Session title can come from run.started event.
	if ev.Type == "run.started" {
		if v := strings.TrimSpace(ev.Data["sessionTitle"]); v != "" {
			m.sessionTitle = v
		}
		if v := strings.TrimSpace(ev.Data["sessionId"]); v != "" {
			m.sessionID = v
		}
		if v := strings.TrimSpace(ev.Data["runId"]); v != "" {
			m.runID = v
		}
	}
	// Model identifier comes from agent.loop.start (host source of truth).
	if ev.Type == "agent.loop.start" {
		if v := strings.TrimSpace(ev.Data["model"]); v != "" {
			m.modelID = v
		}
	}
	// Model identifier can change at runtime via the host /model command.
	if ev.Type == "model.changed" {
		if v := strings.TrimSpace(ev.Data["to"]); v != "" {
			m.modelID = v
		}
	}
	// Workdir is discovered via host.mounted and updated via /cd at runtime.
	if ev.Type == "host.mounted" {
		if wd := strings.TrimSpace(ev.Data["/workdir"]); wd != "" {
			m.workdir = wd
		}
	}
	if ev.Type == "workdir.changed" {
		if wd := strings.TrimSpace(ev.Data["to"]); wd != "" {
			m.workdir = wd
		}
	}
	if ev.Type == "workdir.pwd" {
		if wd := strings.TrimSpace(ev.Data["workdir"]); wd != "" {
			m.workdir = wd
		}
	}
	if ev.Type == "ui.editor.open" {
		// Pre-run /editor includes the absolute workdir so the TUI can resolve
		// and open $VISUAL/$EDITOR without creating a run.
		if wd := strings.TrimSpace(ev.Data["workdir"]); wd != "" {
			m.workdir = wd
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
		known := parseBool(ev.Data["known"])
		m.lastTurnCostUSD = strings.TrimSpace(ev.Data["costUsd"])
		if !known && m.lastTurnCostUSD == "" {
			m.lastTurnCostUSD = "?"
		}
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
	default:
		txt := strings.TrimSpace(rr.Text)
		if txt == "" {
			return
		}
		// Host-side command errors should appear as errors in the transcript.
		if ev.Type == "workdir.error" {
			m.addTranscriptItem(transcriptItem{kind: transcriptError, text: txt})
			m.addTranscriptItem(transcriptItem{kind: transcriptSpacer})
			return
		}
		comp := "✓ ok"
		if ev.Type == "refs.ambiguous" || ev.Type == "refs.unresolved" {
			comp = "✗"
		}
		m.addTranscriptItem(transcriptItem{
			kind:              transcriptAction,
			actionText:        txt,
			actionCompletion:  comp,
			actionIsCompleted: true,
		})
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

// Run starts the Workbench Bubble Tea program.
func Run(ctx context.Context, runner TurnRunner, evCh <-chan events.Event) error {
	if runner == nil {
		return fmt.Errorf("tui runner is required")
	}
	m := New(ctx, runner, evCh)

	// Mouse capture enables mouse wheel / trackpad scrolling in the transcript.
	//
	// Note: enabling xterm mouse tracking often disables native terminal click+drag
	// selection unless your terminal supports shift-drag selection. Workbench defaults
	// to mouse scrolling; set WORKBENCH_MOUSE=false (or --mouse=false) to restore
	// native selection behavior.
	//
	// Mouse mode is opt-in via:
	//   - env: WORKBENCH_MOUSE=true/false
	//   - flag: --mouse (wired in cmd/workbench)
	enableMouse := strings.TrimSpace(os.Getenv("WORKBENCH_MOUSE"))
	mouseOn := true
	if enableMouse != "" {
		mouseOn = enableMouse == "1" || strings.EqualFold(enableMouse, "true") || strings.EqualFold(enableMouse, "yes")
	}

	opts := []tea.ProgramOption{tea.WithAltScreen()}
	if mouseOn {
		// Cell motion is enough for wheel/trackpad scrolling and reduces event spam
		// compared to "all motion".
		opts = append(opts, tea.WithMouseCellMotion())
	}

	p := tea.NewProgram(m, opts...)
	_, err := p.Run()
	return err
}
