package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/types"
)

type tailedEventMsg struct {
	ev store.TailedEvent
}

type tailErrMsg struct {
	err error
}

type commandLinesMsg struct {
	lines []string
}

type monitorModel struct {
	ctx   context.Context
	cfg   config.Config
	runID string

	offset int64

	vp    viewport.Model
	input textinput.Model

	lines []string

	tailCh <-chan store.TailedEvent
	errCh  <-chan error

	cancel context.CancelFunc
}

func RunMonitor(ctx context.Context, cfg config.Config, runID string) error {
	m, err := newMonitorModel(ctx, cfg, runID)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func newMonitorModel(ctx context.Context, cfg config.Config, runID string) (*monitorModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, fmt.Errorf("runID is required")
	}

	evs, off, _ := store.ListEvents(cfg, runID)
	lines := make([]string, 0, 200)
	for _, e := range evs {
		lines = append(lines, formatEventLine(e))
	}
	if len(lines) > 200 {
		lines = lines[len(lines)-200:]
	}

	vp := viewport.New(0, 0)
	vp.SetContent(strings.Join(lines, "\n"))
	vp.GotoBottom()

	in := textinput.New()
	in.Placeholder = "/task <goal> | /memory search <query> | /role <name> | /model <id> | /quit"
	in.Focus()

	tctx, cancel := context.WithCancel(ctx)
	tailCh, errCh := store.TailEvents(cfg, tctx, runID, off)

	return &monitorModel{
		ctx:    ctx,
		cfg:    cfg,
		runID:  runID,
		offset: off,
		vp:     vp,
		input:  in,
		lines:  lines,
		tailCh: tailCh,
		errCh:  errCh,
		cancel: cancel,
	}, nil
}

func (m *monitorModel) Init() tea.Cmd {
	return tea.Batch(m.listenEvent(), m.listenErr())
}

func (m *monitorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.vp.Width = msg.Width
		m.vp.Height = imax(3, msg.Height-2)
		m.vp.SetContent(strings.Join(m.lines, "\n"))
		m.vp.GotoBottom()
		return m, nil

	case tailedEventMsg:
		m.offset = msg.ev.NextOffset
		m.lines = append(m.lines, formatEventLine(msg.ev.Event))
		if len(m.lines) > 400 {
			m.lines = m.lines[len(m.lines)-400:]
		}
		m.vp.SetContent(strings.Join(m.lines, "\n"))
		m.vp.GotoBottom()
		return m, m.listenEvent()

	case tailErrMsg:
		if msg.err != nil {
			m.lines = append(m.lines, "[error] "+msg.err.Error())
			if len(m.lines) > 400 {
				m.lines = m.lines[len(m.lines)-400:]
			}
			m.vp.SetContent(strings.Join(m.lines, "\n"))
			m.vp.GotoBottom()
		}
		return m, m.listenErr()

	case commandLinesMsg:
		if len(msg.lines) != 0 {
			m.lines = append(m.lines, msg.lines...)
			if len(m.lines) > 400 {
				m.lines = m.lines[len(m.lines)-400:]
			}
			m.vp.SetContent(strings.Join(m.lines, "\n"))
			m.vp.GotoBottom()
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		case "enter":
			cmd := strings.TrimSpace(m.input.Value())
			m.input.SetValue("")
			if cmd == "" {
				return m, nil
			}
			return m, m.handleCommand(cmd)
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *monitorModel) View() string {
	header := fmt.Sprintf("Workbench Monitor | run=%s | offset=%d", m.runID, m.offset)
	return header + "\n" + m.vp.View() + "\n" + m.input.View()
}

func (m *monitorModel) listenEvent() tea.Cmd {
	return func() tea.Msg {
		if m.tailCh == nil {
			time.Sleep(250 * time.Millisecond)
			return tailedEventMsg{}
		}
		ev, ok := <-m.tailCh
		if !ok {
			time.Sleep(250 * time.Millisecond)
			return tailedEventMsg{}
		}
		return tailedEventMsg{ev: ev}
	}
}

func (m *monitorModel) listenErr() tea.Cmd {
	return func() tea.Msg {
		if m.errCh == nil {
			time.Sleep(250 * time.Millisecond)
			return tailErrMsg{}
		}
		err, ok := <-m.errCh
		if !ok {
			time.Sleep(250 * time.Millisecond)
			return tailErrMsg{}
		}
		return tailErrMsg{err: err}
	}
}

func (m *monitorModel) handleCommand(raw string) tea.Cmd {
	raw = strings.TrimSpace(raw)
	if raw == "/quit" {
		if m.cancel != nil {
			m.cancel()
		}
		return tea.Quit
	}

	if strings.HasPrefix(raw, "/task ") {
		goal := strings.TrimSpace(strings.TrimPrefix(raw, "/task "))
		goal = strings.Trim(goal, "\"")
		return m.enqueueTask(goal, 0)
	}
	if strings.HasPrefix(raw, "/role ") {
		roleName := strings.TrimSpace(strings.TrimPrefix(raw, "/role "))
		return m.writeControl(map[string]any{"role": roleName})
	}
	if strings.HasPrefix(raw, "/model ") {
		model := strings.TrimSpace(strings.TrimPrefix(raw, "/model "))
		return m.writeControl(map[string]any{"model": model})
	}
	if strings.HasPrefix(raw, "/memory search ") {
		query := strings.TrimSpace(strings.TrimPrefix(raw, "/memory search "))
		query = strings.Trim(query, "\"")
		return m.searchMemory(query)
	}

	// Unknown commands are logged to the view.
	return func() tea.Msg { return commandLinesMsg{lines: []string{"[command] " + raw}} }
}

func (m *monitorModel) enqueueTask(goal string, priority int) tea.Cmd {
	return func() tea.Msg {
		goal = strings.TrimSpace(goal)
		if goal == "" {
			return commandLinesMsg{lines: []string{"[queued] error: goal is empty"}}
		}
		runDir := fsutil.GetRunDir(m.cfg.DataDir, m.runID)
		inboxDir := filepath.Join(runDir, "inbox")
		_ = os.MkdirAll(inboxDir, 0755)
		now := time.Now()
		id := "task-" + uuid.NewString()
		task := types.Task{
			TaskID:    id,
			Goal:      goal,
			Priority:  priority,
			Status:    "pending",
			CreatedAt: &now,
		}
		b, _ := json.MarshalIndent(task, "", "  ")
		if err := os.WriteFile(filepath.Join(inboxDir, id+".json"), b, 0644); err != nil {
			return commandLinesMsg{lines: []string{"[queued] error: " + err.Error()}}
		}
		return commandLinesMsg{lines: []string{"[queued] " + id + " " + goal}}
	}
}

func (m *monitorModel) writeControl(payload map[string]any) tea.Cmd {
	return func() tea.Msg {
		runDir := fsutil.GetRunDir(m.cfg.DataDir, m.runID)
		inboxDir := filepath.Join(runDir, "inbox")
		_ = os.MkdirAll(inboxDir, 0755)
		payload["processed"] = false
		b, _ := json.MarshalIndent(payload, "", "  ")
		if err := os.WriteFile(filepath.Join(inboxDir, "control.json"), b, 0644); err != nil {
			return commandLinesMsg{lines: []string{"[control] error: " + err.Error()}}
		}
		return commandLinesMsg{lines: []string{"[control] updated"}}
	}
}

func (m *monitorModel) searchMemory(query string) tea.Cmd {
	return func() tea.Msg {
		query = strings.TrimSpace(query)
		if query == "" {
			return commandLinesMsg{lines: []string{"[memory] error: query is empty"}}
		}
		vm, err := store.NewVectorMemoryStore(m.cfg)
		if err != nil {
			return commandLinesMsg{lines: []string{"[memory] error: " + err.Error()}}
		}
		results, err := vm.Search(m.ctx, query, 5)
		if err != nil {
			return commandLinesMsg{lines: []string{"[memory] error: " + err.Error()}}
		}
		lines := []string{"[memory] search: " + query}
		for _, r := range results {
			lines = append(lines, fmt.Sprintf("  - %.3f %s (%s)", r.Score, r.Title, r.Filename))
		}
		return commandLinesMsg{lines: lines}
	}
}

func formatEventLine(e types.Event) string {
	ts := e.Timestamp.Local().Format("15:04:05")
	line := fmt.Sprintf("[%s] %s: %s", ts, e.Type, e.Message)
	if len(e.Data) > 0 {
		// Keep it compact.
		if v, ok := e.Data["taskId"]; ok && v != "" {
			line += " taskId=" + v
		}
		if v, ok := e.Data["role"]; ok && v != "" {
			line += " role=" + v
		}
		if v, ok := e.Data["model"]; ok && v != "" {
			line += " model=" + v
		}
	}
	return line
}

func imax(a, b int) int {
	if a > b {
		return a
	}
	return b
}
