package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

	view viewport.Model
	log  string

	single    textarea.Model
	multiline textarea.Model
	isMulti   bool

	width  int
	height int

	turnInFlight bool
	turnStarted  time.Time
	turnTitle    string

	statusLine string

	styleHeader lipgloss.Style
	styleEvent  lipgloss.Style
	styleUser   lipgloss.Style
	styleAgent  lipgloss.Style
	styleError  lipgloss.Style
}

func New(ctx context.Context, runner TurnRunner, evCh <-chan events.Event) Model {
	vp := viewport.New(0, 0)
	vp.Style = lipgloss.NewStyle().Padding(0, 1)

	single := textarea.New()
	single.Placeholder = "Type a message…"
	single.Focus()
	single.Prompt = "you> "
	single.ShowLineNumbers = false
	single.SetHeight(1)
	single.CharLimit = 0
	single.KeyMap.InsertNewline.SetEnabled(false) // Enter should submit in single-line mode.

	multi := textarea.New()
	multi.Placeholder = "Multiline message (Ctrl+Enter to send)…"
	multi.Prompt = "…> "
	multi.ShowLineNumbers = false
	multi.CharLimit = 0
	multi.SetHeight(6)

	m := Model{
		ctx:       ctx,
		runner:    runner,
		events:    evCh,
		view:      vp,
		single:    single,
		multiline: multi,
		styleHeader: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#c0c0c0")),
		styleEvent: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#707070")),
		styleUser: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9ad0ff")).
			Bold(true),
		styleAgent: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#eaeaea")),
		styleError: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ff5f5f")).
			Bold(true),
	}

	m.appendLine(m.styleHeader.Render("== Chat session started (Ctrl+C to quit) =="))
	m.appendLine(m.styleHeader.Render("Tip: Ctrl+J toggles multiline; in multiline, Enter inserts newline and Ctrl+Enter sends."))
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

		// Toggle multiline (Ctrl+J is reliable across terminals).
		if msg.Type == tea.KeyCtrlJ {
			m.toggleMultiline()
			return m, nil
		}

		if m.turnInFlight {
			// While a turn is running, we allow scrolling but prevent submitting.
			var cmd tea.Cmd
			m.view, cmd = m.view.Update(msg)
			return m, cmd
		}

		if m.isMulti {
			// In multiline mode, Enter inserts newline. We treat Ctrl+Enter as send when
			// the terminal exposes it, but also support "ctrl+m" (some terminals).
			if strings.EqualFold(msg.String(), "ctrl+enter") || strings.EqualFold(msg.String(), "ctrl+m") {
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
		// The final answer is rendered as the agent message, not as an event row.
		if ev.Type == "agent.final" {
			return m, m.waitEvent()
		}
		line := formatEventLine(ev)
		if strings.TrimSpace(line) != "" {
			m.appendLine(m.styleEvent.Render(line))
		}
		return m, m.waitEvent()

	case turnDoneMsg:
		m.turnInFlight = false
		if msg.err != nil {
			m.appendLine(m.styleError.Render("agent error: " + msg.err.Error()))
			m.appendLine(m.styleHeader.Render(""))
			m.turnTitle = ""
			return m, nil
		}
		m.appendLine(m.styleAgent.Render("agent> " + strings.TrimSpace(msg.final)))
		m.appendLine(m.styleHeader.Render(""))
		m.turnTitle = ""
		return m, nil
	}

	// Default fallthrough: keep viewport responsive.
	var cmd tea.Cmd
	m.view, cmd = m.view.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	header := m.renderHeader()
	body := m.view.View()
	input := m.renderInput()
	return header + "\n" + body + "\n" + input
}

func (m *Model) layout() {
	// Layout:
	// - 1 line header
	// - 1 line separator
	// - viewport fills remaining minus input height
	// - input at bottom
	headerH := 2
	inputH := 3
	if m.isMulti {
		inputH = 8
	}
	viewH := m.height - headerH - inputH
	if viewH < 3 {
		viewH = 3
	}
	m.view.Width = m.width
	m.view.Height = viewH

	m.single.SetWidth(max(20, m.width-2))
	m.multiline.SetWidth(max(20, m.width-2))
	m.view.SetContent(m.log)
	m.view.GotoBottom()
}

func (m Model) renderHeader() string {
	chips := []string{}
	if m.statusLine != "" {
		chips = append(chips, m.statusLine)
	}
	if m.turnInFlight {
		chips = append(chips, "running…")
	}
	line := strings.Join(chips, "  ")
	if line == "" {
		line = "workbench"
	}
	return m.styleHeader.Render(line) + "\n" + m.styleHeader.Render(strings.Repeat("─", max(1, m.width)))
}

func (m Model) renderInput() string {
	if m.isMulti {
		return m.multiline.View()
	}
	return m.single.View()
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
	m.appendLine(m.styleUser.Render("you> " + userMsg))

	return func() tea.Msg {
		final, err := m.runner.RunTurn(m.ctx, userMsg)
		_ = final
		return turnDoneMsg{final: final, err: err}
	}
}

func (m *Model) appendLine(line string) {
	if strings.TrimSpace(m.log) == "" {
		m.log = line
	} else {
		m.log += "\n" + line
	}
	m.view.SetContent(m.log)
	m.view.GotoBottom()
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
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
