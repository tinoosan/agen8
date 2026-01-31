package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/tui/kit"
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

	input textinput.Model

	activity    []string
	maxActivity int

	taskStatus map[string]string
	stats      monitorStats
	memResults []string
	model      string
	role       string

	activityVP viewport.Model
	queueVP    viewport.Model
	memoryVP   viewport.Model

	width  int
	height int
	styles *monitorStyles

	tailCh <-chan store.TailedEvent
	errCh  <-chan error

	cancel context.CancelFunc
}

type monitorStats struct {
	started   time.Time
	tasksDone int
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
	act := make([]string, 0, 200)
	taskStatus := map[string]string{}
	stats := monitorStats{started: time.Now()}
	for _, e := range evs {
		act = append(act, formatEventLine(e))
		updateStateFromEvent(e, taskStatus, &stats, nil, nil)
	}
	if len(act) > 200 {
		act = act[len(act)-200:]
	}

	in := textinput.New()
	in.Placeholder = "/task <goal> | /memory search <query> | /role <name> | /model <id> | /quit"
	in.PlaceholderStyle = kit.StyleDim
	in.TextStyle = kit.StyleStatusValue
	in.Prompt = "> "
	in.PromptStyle = kit.StyleStatusKey
	in.Focus()

	tctx, cancel := context.WithCancel(ctx)
	tailCh, errCh := store.TailEvents(cfg, tctx, runID, off)

	activityVP := viewport.New(0, 0)
	queueVP := viewport.New(0, 0)
	memoryVP := viewport.New(0, 0)

	return &monitorModel{
		ctx:         ctx,
		cfg:         cfg,
		runID:       runID,
		offset:      off,
		input:       in,
		activity:    act,
		maxActivity: 400,
		taskStatus:  taskStatus,
		stats:       stats,
		activityVP:  activityVP,
		queueVP:     queueVP,
		memoryVP:    memoryVP,
		styles:      defaultMonitorStyles(),
		tailCh:      tailCh,
		errCh:       errCh,
		cancel:      cancel,
	}, nil
}

func (m *monitorModel) Init() tea.Cmd {
	return tea.Batch(m.listenEvent(), m.listenErr())
}

func (m *monitorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.refreshViewports()
		return m, nil

	case tailedEventMsg:
		m.offset = msg.ev.NextOffset
		line := formatEventLine(msg.ev.Event)
		m.activity = append(m.activity, line)
		if len(m.activity) > m.maxActivity {
			m.activity = m.activity[len(m.activity)-m.maxActivity:]
		}
		updateStateFromEvent(msg.ev.Event, m.taskStatus, &m.stats, &m.model, &m.role)
		m.refreshViewports()
		return m, m.listenEvent()

	case tailErrMsg:
		if msg.err != nil {
			m.activity = append(m.activity, "[error] "+msg.err.Error())
			if len(m.activity) > m.maxActivity {
				m.activity = m.activity[len(m.activity)-m.maxActivity:]
			}
		}
		m.refreshViewports()
		return m, m.listenErr()

	case commandLinesMsg:
		if len(msg.lines) != 0 {
			m.activity = append(m.activity, msg.lines...)
			if len(m.activity) > m.maxActivity {
				m.activity = m.activity[len(m.activity)-m.maxActivity:]
			}
			if strings.HasPrefix(strings.TrimSpace(msg.lines[0]), "[memory] search:") {
				m.memResults = msg.lines[1:]
			}
		}
		m.refreshViewports()
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
	left := m.styles.panel.Render(m.styles.sectionTitle.Render("Activity Stream") + "\n" + m.activityVP.View())
	rightQueue := m.styles.panel.Render(m.styles.sectionTitle.Render("Task Queue") + "\n" + m.queueVP.View())
	rightStats := m.styles.panel.Render(m.styles.sectionTitle.Render("Stats") + "\n" + renderStats(m.stats))
	right := lipgloss.JoinVertical(lipgloss.Left, rightQueue, rightStats)
	memory := m.styles.panel.Render(m.styles.sectionTitle.Render("Memory (semantic search)") + "\n" + m.memoryVP.View())
	cmdBar := m.styles.commandBar.Render("Commands: /task \"goal\"   /role <name>   /model <id>   /memory search \"query\"   /quit\n" + m.input.View())

	headerLine := m.styles.header.Render(
		lipgloss.JoinHorizontal(lipgloss.Left,
			m.styles.headerTitle.Render("Workbench - Always On"),
			kit.RenderTag(kit.TagOptions{Key: "Model", Value: fallback(m.model, "(unknown)")}),
			kit.RenderTag(kit.TagOptions{Key: "Role", Value: fallback(m.role, "(none)")}),
			kit.RenderTag(kit.TagOptions{Key: "Run", Value: m.runID}),
		))

	body := lipgloss.JoinVertical(lipgloss.Left,
		headerLine,
		lipgloss.JoinHorizontal(lipgloss.Top, left, right),
		memory,
		cmdBar,
	)

	return body
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

func updateStateFromEvent(e types.Event, tasks map[string]string, stats *monitorStats, model *string, role *string) {
	if tasks == nil {
		return
	}
	taskID := strings.TrimSpace(e.Data["taskId"])
	switch e.Type {
	case "task.queued", "task.generated":
		if taskID != "" {
			tasks[taskID] = "pending"
		}
	case "task.start":
		if taskID != "" {
			tasks[taskID] = "active"
		}
	case "task.done":
		if taskID != "" {
			tasks[taskID] = strings.TrimSpace(e.Data["status"])
		}
		if stats != nil {
			stats.tasksDone++
			if stats.started.IsZero() {
				stats.started = time.Now()
			}
		}
	}
	if model != nil {
		if v := strings.TrimSpace(e.Data["model"]); v != "" {
			*model = v
		}
	}
	if role != nil {
		if v := strings.TrimSpace(e.Data["role"]); v != "" {
			*role = v
		}
	}
}

func renderQueue(tasks map[string]string) string {
	active := []string{}
	pending := []string{}
	done := []string{}
	for id, st := range tasks {
		switch strings.ToLower(st) {
		case "active", "in_progress":
			active = append(active, id)
		case "pending", "":
			pending = append(pending, id)
		default:
			done = append(done, fmt.Sprintf("%s (%s)", id, st))
		}
	}
	sort.Strings(active)
	sort.Strings(pending)
	sort.Strings(done)
	var b strings.Builder
	if len(active) > 0 {
		b.WriteString("[ACTIVE] ")
		b.WriteString(strings.Join(active, ", "))
		b.WriteString("\n")
	}
	if len(pending) > 0 {
		b.WriteString("[PENDING] ")
		b.WriteString(strings.Join(pending, ", "))
		b.WriteString("\n")
	}
	if len(done) > 0 {
		b.WriteString("[DONE] ")
		if len(done) > 3 {
			done = done[len(done)-3:]
			b.WriteString(strings.Join(done, ", "))
			b.WriteString(" (latest 3)")
		} else {
			b.WriteString(strings.Join(done, ", "))
		}
		b.WriteString("\n")
	}
	if b.Len() == 0 {
		return "No tasks yet."
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderStats(s monitorStats) string {
	uptime := ""
	if !s.started.IsZero() {
		uptime = time.Since(s.started).Round(time.Second).String()
	}
	return fmt.Sprintf("Tasks done: %d\nUptime: %s", s.tasksDone, fallback(uptime, "unknown"))
}

func renderMemResults(results []string) string {
	if len(results) == 0 {
		return "No memory results."
	}
	return strings.Join(results, "\n")
}

func renderSection(title, body string) string {
	header := lipgloss.NewStyle().Bold(true).Render(title)
	return header + "\n" + body
}

func fallback(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

type monitorStyles struct {
	header       lipgloss.Style
	headerTitle  lipgloss.Style
	sectionTitle lipgloss.Style
	panel        lipgloss.Style
	commandBar   lipgloss.Style
}

func defaultMonitorStyles() *monitorStyles {
	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(kit.BorderColorDefault).
		Padding(0, 1)

	return &monitorStyles{
		header:       lipgloss.NewStyle().Padding(0, 1),
		headerTitle:  lipgloss.NewStyle().Bold(true),
		sectionTitle: lipgloss.NewStyle().Bold(true),
		panel:        panel,
		commandBar:   lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(kit.BorderColorDefault).Padding(0, 1),
	}
}

func (m *monitorModel) refreshViewports() {
	if m.width == 0 || m.height == 0 {
		return
	}
	headerH := 1
	footerH := 2
	memoryH := 7
	mainH := m.height - headerH - footerH - memoryH
	if mainH < 6 {
		mainH = 6
	}
	leftW := int(float64(m.width) * 0.62)
	if leftW < 40 {
		leftW = 40
	}
	rightW := m.width - leftW
	if rightW < 30 {
		rightW = 30
		leftW = m.width - rightW
	}

	m.activityVP.Width = leftW - 4
	m.activityVP.Height = mainH - 2
	m.activityVP.SetContent(strings.Join(m.activity, "\n"))
	m.activityVP.GotoBottom()

	m.queueVP.Width = rightW - 4
	m.queueVP.Height = mainH/2 - 1
	m.queueVP.SetContent(renderQueue(m.taskStatus))

	m.memoryVP.Width = m.width - 4
	m.memoryVP.Height = memoryH - 2
	m.memoryVP.SetContent(renderMemResults(m.memResults))
}
