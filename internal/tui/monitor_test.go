package tui

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/tinoosan/agen8/internal/app"
	implstore "github.com/tinoosan/agen8/internal/store"
	"github.com/tinoosan/agen8/internal/tui/kit"
	layoutmgr "github.com/tinoosan/agen8/internal/tui/layout"
	agentstate "github.com/tinoosan/agen8/pkg/agent/state"
	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/fsutil"
	"github.com/tinoosan/agen8/pkg/protocol"
	pkgagent "github.com/tinoosan/agen8/pkg/services/agent"
	pkgsession "github.com/tinoosan/agen8/pkg/services/session"
	pkgtask "github.com/tinoosan/agen8/pkg/services/task"
	"github.com/tinoosan/agen8/pkg/types"
)

type noopSessionSupervisor struct{}

func (noopSessionSupervisor) StopRun(string) error                    { return nil }
func (noopSessionSupervisor) ResumeRun(context.Context, string) error { return nil }

func startMonitorTestRPCServer(t *testing.T, cfg config.Config, runID string) string {
	t.Helper()
	taskStore, err := agentstate.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	if err != nil {
		t.Fatalf("task store: %v", err)
	}
	sessionStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("session store: %v", err)
	}
	run := types.NewRun("rpc-test", 8*1024, "sess-rpc-test")
	if rid := strings.TrimSpace(runID); rid != "" {
		run.RunID = rid
	}
	if err := implstore.SaveRun(cfg, run); err != nil {
		t.Fatalf("save run: %v", err)
	}
	sess := types.NewSession("rpc-test")
	sess.SessionID = run.SessionID
	sess.CurrentRunID = run.RunID
	sess.Runs = []string{run.RunID}
	if err := sessionStore.SaveSession(context.Background(), sess); err != nil {
		t.Fatalf("save session: %v", err)
	}
	if err := sessionStore.SaveRun(context.Background(), run); err != nil {
		t.Fatalf("save run: %v", err)
	}
	sessionSvc := pkgsession.NewManager(cfg, sessionStore, noopSessionSupervisor{})
	taskSvc := pkgtask.NewManager(taskStore, sessionSvc)
	agentMgr := pkgagent.NewManager(sessionSvc, taskSvc, taskSvc)
	srv := app.NewRPCServer(app.RPCServerConfig{
		Cfg:            cfg,
		Run:            run,
		AllowAnyThread: true,
		TaskService:    taskSvc,
		Session:        sessionSvc,
		AgentService:   agentMgr,
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_ = srv.Serve(ctx, c, c)
			}(conn)
		}
	}()
	t.Cleanup(func() {
		cancel()
		_ = ln.Close()
	})
	endpoint := ln.Addr().String()
	t.Setenv("AGEN8_RPC_ENDPOINT", endpoint)
	return endpoint
}

func TestIsCompactMode_Breakpoints(t *testing.T) {
	tests := []struct {
		width  int
		height int
		want   bool
	}{
		{120, 35, false},
		{110, 32, false},
		{109, 32, true},
		{110, 31, true},
		{100, 30, true},
	}
	for _, tt := range tests {
		m := &monitorModel{width: tt.width, height: tt.height}
		got := m.isCompactMode()
		if got != tt.want {
			t.Errorf("isCompactMode(%d,%d) = %v, want %v", tt.width, tt.height, got, tt.want)
		}
	}
}

func TestSplitMonitorCommand_WhitespaceTolerant(t *testing.T) {
	cmd, rest := splitMonitorCommand("/model\nhello world\n")
	if cmd != "/model" {
		t.Fatalf("expected cmd %q, got %q", "/model", cmd)
	}
	if rest != "hello world" {
		t.Fatalf("expected rest %q, got %q", "hello world", rest)
	}

	cmd, rest = splitMonitorCommand("   /memory\tsearch \n  query\nline2  ")
	if cmd != "/memory search" {
		t.Fatalf("expected cmd %q, got %q", "/memory search", cmd)
	}
	if rest != "query\nline2" {
		t.Fatalf("expected rest %q, got %q", "query\nline2", rest)
	}
}

func TestMonitorHandleCommand_EnqueuesPlainTextAndRejectsUnknownSlashCommand(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	runID := "test-run-handle-command"
	startMonitorTestRPCServer(t, cfg, runID)

	m, err := newMonitorModel(ctx, cfg, runID, &MonitorResult{})
	if err != nil {
		t.Fatalf("newMonitorModel: %v", err)
	}

	assertQueued := func(input string, wantGoal string) {
		t.Helper()
		cmd := m.handleCommand(input)
		if cmd == nil {
			t.Fatalf("handleCommand(%q) returned nil; want enqueue cmd", input)
		}
		_ = cmd()
		tasks, err := m.taskStore.ListTasks(ctx, agentstate.TaskFilter{
			RunID:    runID,
			Status:   []types.TaskStatus{types.TaskStatusPending},
			SortBy:   "created_at",
			SortDesc: true,
			Limit:    1,
		})
		if err != nil {
			t.Fatalf("ListTasks: %v", err)
		}
		if len(tasks) == 0 {
			t.Fatalf("expected at least one queued task")
		}
		if tasks[0].Goal != wantGoal {
			t.Fatalf("task goal = %q, want %q", tasks[0].Goal, wantGoal)
		}
	}

	// Plain text should enqueue as task.
	assertQueued("hello world", "hello world")

	before, err := m.taskStore.CountTasks(ctx, agentstate.TaskFilter{RunID: runID})
	if err != nil {
		t.Fatalf("CountTasks(before): %v", err)
	}
	cmd := m.handleCommand("/not-a-command hello")
	if cmd == nil {
		t.Fatalf("expected command response for unknown slash command")
	}
	msg := cmd()
	lines, ok := msg.(commandLinesMsg)
	if !ok || len(lines.lines) == 0 || !strings.Contains(lines.lines[0], "unknown command") {
		t.Fatalf("expected unknown command output, got %#v", msg)
	}
	after, err := m.taskStore.CountTasks(ctx, agentstate.TaskFilter{RunID: runID})
	if err != nil {
		t.Fatalf("CountTasks(after): %v", err)
	}
	if after != before {
		t.Fatalf("unexpected task count change: before=%d after=%d", before, after)
	}
}

func TestMonitorCommandRegistry_IncludesCopy(t *testing.T) {
	found := false
	for _, name := range monitorAvailableCommands {
		if name == "/copy" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected /copy in monitor commands")
	}
}

func TestEnqueueTask_AutoResumesPausedStandaloneRun(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	runID := "run-auto-resume"
	startMonitorTestRPCServer(t, cfg, runID)

	run, err := implstore.LoadRun(cfg, runID)
	if err != nil {
		t.Fatalf("LoadRun: %v", err)
	}
	run.Status = types.RunStatusPaused
	if err := implstore.SaveRun(cfg, run); err != nil {
		t.Fatalf("SaveRun(paused): %v", err)
	}

	m, err := newMonitorModel(ctx, cfg, runID, &MonitorResult{})
	if err != nil {
		t.Fatalf("newMonitorModel: %v", err)
	}
	cmd := m.enqueueTask("auto resume test", 5)
	if cmd == nil {
		t.Fatalf("expected enqueue cmd")
	}
	_ = cmd()

	loaded, err := implstore.LoadRun(cfg, runID)
	if err != nil {
		t.Fatalf("LoadRun(after): %v", err)
	}
	if loaded.Status != types.RunStatusRunning {
		t.Fatalf("run status=%q want %q", loaded.Status, types.RunStatusRunning)
	}

	tasks, err := m.taskStore.ListTasks(ctx, agentstate.TaskFilter{
		RunID:          runID,
		AssignedToType: "agent",
		AssignedTo:     runID,
		Status:         []types.TaskStatus{types.TaskStatusPending},
		SortBy:         "created_at",
		SortDesc:       true,
		Limit:          1,
	})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) == 0 {
		t.Fatalf("expected pending queued task")
	}
}

func TestEnqueueTask_TeamDefaultsToCoordinatorRun(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	runID := "run-coordinator"
	startMonitorTestRPCServer(t, cfg, runID)

	m, err := newMonitorModel(ctx, cfg, runID, &MonitorResult{})
	if err != nil {
		t.Fatalf("newMonitorModel: %v", err)
	}
	m.teamID = "team-1"
	m.teamCoordinatorRunID = runID
	m.teamRoleByRunID = map[string]string{runID: "coordinator"}
	cmd := m.enqueueTask("team queue test", 5)
	if cmd == nil {
		t.Fatalf("expected enqueue cmd")
	}
	_ = cmd()

	tasks, err := m.taskStore.ListTasks(ctx, agentstate.TaskFilter{
		TeamID:         "team-1",
		RunID:          runID,
		AssignedToType: "role",
		AssignedTo:     "coordinator",
		Status:         []types.TaskStatus{types.TaskStatusPending},
		SortBy:         "created_at",
		SortDesc:       true,
		Limit:          1,
	})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) == 0 {
		t.Fatalf("expected team task assigned to coordinator run")
	}
	if got := strings.TrimSpace(tasks[0].AssignedRole); got != "coordinator" {
		t.Fatalf("assignedRole=%q want %q", got, "coordinator")
	}
}

func TestEnqueueTask_TeamRoleMissingReturnsError(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	runID := "run-role-missing"
	startMonitorTestRPCServer(t, cfg, runID)

	m, err := newMonitorModel(ctx, cfg, runID, &MonitorResult{})
	if err != nil {
		t.Fatalf("newMonitorModel: %v", err)
	}
	m.teamID = "team-1"
	m.teamCoordinatorRunID = runID
	m.teamRoleByRunID = map[string]string{}
	cmd := m.enqueueTask("should fail", 5)
	if cmd == nil {
		t.Fatalf("expected enqueue cmd")
	}
	msg := cmd()
	lines, ok := msg.(commandLinesMsg)
	if !ok || len(lines.lines) == 0 {
		t.Fatalf("expected commandLinesMsg, got %#v", msg)
	}
	if !strings.Contains(strings.ToLower(lines.lines[0]), "target role unavailable") {
		t.Fatalf("unexpected error line: %q", lines.lines[0])
	}
}

func TestEnqueueTask_TeamFocusedRunUsesFocusedRole(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	runID := "run-focused"
	startMonitorTestRPCServer(t, cfg, runID)

	m, err := newMonitorModel(ctx, cfg, runID, &MonitorResult{})
	if err != nil {
		t.Fatalf("newMonitorModel: %v", err)
	}
	m.teamID = "team-1"
	m.teamCoordinatorRunID = "run-coordinator"
	m.focusedRunID = runID
	m.teamRoleByRunID = map[string]string{
		runID:             "cto",
		"run-coordinator": "ceo",
	}
	cmd := m.enqueueTask("focused team queue test", 5)
	if cmd == nil {
		t.Fatalf("expected enqueue cmd")
	}
	_ = cmd()

	tasks, err := m.taskStore.ListTasks(ctx, agentstate.TaskFilter{
		TeamID:         "team-1",
		RunID:          runID,
		AssignedToType: "role",
		AssignedTo:     "cto",
		Status:         []types.TaskStatus{types.TaskStatusPending},
		SortBy:         "created_at",
		SortDesc:       true,
		Limit:          1,
	})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) == 0 {
		t.Fatalf("expected focused role-assigned team task")
	}
	if got := strings.TrimSpace(tasks[0].AssignedRole); got != "cto" {
		t.Fatalf("assignedRole=%q want %q", got, "cto")
	}
}

func TestMonitorAgentOutputForClipboard(t *testing.T) {
	now := time.Date(2026, 2, 18, 14, 30, 0, 0, time.UTC)
	m := &monitorModel{
		agentOutput: []AgentOutputItem{
			{Timestamp: now, Type: "tool_call", Content: "Run python code"},
			{Timestamp: now.Add(2 * time.Second), Type: "tool_result", Content: "Run python code — ok"},
		},
		showThoughts: true,
	}

	txt := m.agentOutputForClipboard()
	if !strings.Contains(txt, "[14:30:00] Run python code") {
		t.Fatalf("expected first line in clipboard export, got:\n%s", txt)
	}
	if !strings.Contains(txt, "[14:30:02] Run python code — ok") {
		t.Fatalf("expected second line in clipboard export, got:\n%s", txt)
	}
}

func TestMonitorWriteControl_SetReasoning_UsesRPCMethod(t *testing.T) {
	m := &monitorModel{
		runID:       "run-test",
		sessionID:   "sess-test",
		rpcEndpoint: "127.0.0.1:1",
	}
	msg := m.writeControl("set_reasoning", map[string]any{"summary": "auto"})()
	lines, ok := msg.(commandLinesMsg)
	if !ok || len(lines.lines) == 0 {
		t.Fatalf("expected commandLinesMsg, got %#v", msg)
	}
	line := strings.ToLower(strings.TrimSpace(lines.lines[0]))
	if strings.Contains(line, "unsupported command set_reasoning") {
		t.Fatalf("expected set_reasoning RPC path, got %q", lines.lines[0])
	}
	if !strings.Contains(line, "rpc control.setreasoning") {
		t.Fatalf("expected control.setReasoning rpc error, got %q", lines.lines[0])
	}
}

func TestShouldReloadPlanOnEvent(t *testing.T) {
	t.Run("fs_write", func(t *testing.T) {
		ev := types.EventRecord{
			Type: "fs_write",
			Data: map[string]string{"path": "/plan/HEAD.md"},
		}
		if !shouldReloadPlanOnEvent(ev) {
			t.Fatalf("expected plan reload for %v", ev)
		}
	})

	t.Run("agent.op.request fs_write", func(t *testing.T) {
		ev := types.EventRecord{
			Type: "agent.op.request",
			Data: map[string]string{"op": "fs_write", "path": "/plan/CHECKLIST.md"},
		}
		if !shouldReloadPlanOnEvent(ev) {
			t.Fatalf("expected plan reload for %v", ev)
		}
	})

	t.Run("agent.op.response fs_write with path", func(t *testing.T) {
		ev := types.EventRecord{
			Type: "agent.op.response",
			Data: map[string]string{"op": "fs_write", "path": "/plan/HEAD.md"},
		}
		if !shouldReloadPlanOnEvent(ev) {
			t.Fatalf("expected plan reload for %v", ev)
		}
	})

	t.Run("agent.op.response fs_read", func(t *testing.T) {
		ev := types.EventRecord{
			Type: "agent.op.response",
			Data: map[string]string{"op": "fs_read", "path": "/plan/HEAD.md"},
		}
		if shouldReloadPlanOnEvent(ev) {
			t.Fatalf("did not expect plan reload for %v", ev)
		}
	})
}

func TestMonitorObserveEvent_CostUSDKey(t *testing.T) {
	m := &monitorModel{}
	m.observeEvent(types.EventRecord{
		Type: "llm.cost.total",
		Data: map[string]string{
			"known":   "true",
			"costUSD": "1.2345",
		},
	})
	if got := strings.TrimSpace(m.stats.lastTurnCostUSD); got != "1.2345" {
		t.Fatalf("lastTurnCostUSD = %q, want %q", got, "1.2345")
	}
	if !m.stats.pricingKnown {
		t.Fatalf("expected pricingKnown true")
	}
}

func TestMonitorObserveEvent_ThoughtsFallbackFromReasoningTokens(t *testing.T) {
	m := &monitorModel{
		reasoningUsageByStep: map[string]int{},
	}
	m.observeEvent(types.EventRecord{
		Type:  "llm.usage.total",
		RunID: "run-a",
		Data: map[string]string{
			"step":      "2",
			"input":     "11",
			"output":    "7",
			"total":     "18",
			"reasoning": "5",
		},
	})
	m.observeEvent(types.EventRecord{
		Type:  "agent.step",
		RunID: "run-a",
		Data: map[string]string{
			"step": "2",
		},
	})
	if len(m.thinkingEntries) != 1 {
		t.Fatalf("expected one thinking entry, got %d", len(m.thinkingEntries))
	}
	if !strings.Contains(strings.ToLower(m.thinkingEntries[0].Summary), "reasoning used (5 tokens)") {
		t.Fatalf("unexpected fallback summary: %q", m.thinkingEntries[0].Summary)
	}
}

func TestMonitorObserveEvent_EffectiveModelPreferred(t *testing.T) {
	m := &monitorModel{}
	m.observeEvent(types.EventRecord{
		Type: "agent.step",
		Data: map[string]string{
			"model":          "requested-model",
			"effectiveModel": "provider-model",
			"step":           "1",
		},
	})
	if got := strings.TrimSpace(m.model); got != "provider-model" {
		t.Fatalf("model = %q, want provider-model", got)
	}
}

func TestMonitorObserveEvent_TeamIgnoresEffectiveModel(t *testing.T) {
	m := &monitorModel{
		teamID: "team-1",
		model:  "openai/gpt-5",
	}
	m.observeEvent(types.EventRecord{
		Type: "agent.step",
		Data: map[string]string{
			"model":          "requested-model",
			"effectiveModel": "provider-model",
			"step":           "1",
		},
	})
	if got := strings.TrimSpace(m.model); got != "openai/gpt-5" {
		t.Fatalf("model = %q, want openai/gpt-5", got)
	}
}

func TestMonitorWorkspaceDir_TeamAndRun(t *testing.T) {
	dataDir := t.TempDir()

	teamMonitor := &monitorModel{
		cfg:    config.Config{DataDir: dataDir},
		teamID: "team-1",
		runID:  "team:team-1",
	}
	if got, want := teamMonitor.workspaceDir(), fsutil.GetTeamWorkspaceDir(dataDir, "team-1"); got != want {
		t.Fatalf("team workspaceDir = %q, want %q", got, want)
	}

	runMonitor := &monitorModel{
		cfg:   config.Config{DataDir: dataDir},
		runID: "run-1",
	}
	if got, want := runMonitor.workspaceDir(), fsutil.GetWorkspaceDir(dataDir, "run-1"); got != want {
		t.Fatalf("run workspaceDir = %q, want %q", got, want)
	}
}

func TestMonitorBottomBar_ShowsAndClearsLLMError(t *testing.T) {
	m := &monitorModel{styles: defaultMonitorStyles()}
	m.observeEvent(types.EventRecord{
		Type: "llm.error",
		Data: map[string]string{
			"class":     "quota",
			"retryable": "false",
		},
	})
	line := m.renderBottomBar(220)
	if !strings.Contains(line, "LLM error: quota (no-retry)") {
		t.Fatalf("expected llm error indicator, got %q", line)
	}

	m.observeEvent(types.EventRecord{
		Type: "agent.step",
		Data: map[string]string{
			"step": "1",
		},
	})
	line = m.renderBottomBar(220)
	if strings.Contains(line, "LLM error:") {
		t.Fatalf("expected llm error indicator cleared, got %q", line)
	}
}

func TestMonitorBottomBar_PrioritizesLLMAlertBeforeHints(t *testing.T) {
	m := &monitorModel{
		styles:         defaultMonitorStyles(),
		width:          120,
		height:         40,
		rpcHealthKnown: true,
		rpcReachable:   false,
	}
	m.stats.lastLLMErrorSet = true
	m.stats.lastLLMErrorClass = "quota"
	m.stats.lastLLMErrorRetryable = false

	line := m.renderBottomBar(220)
	llmIdx := strings.Index(line, "LLM error: quota (no-retry)")
	verboseIdx := strings.Index(line, "Ctrl+]/Ctrl+[")
	if llmIdx < 0 || verboseIdx < 0 {
		t.Fatalf("expected llm and verbose segments; got %q", line)
	}
	if strings.Contains(line, "daemon disconnected") {
		t.Fatalf("expected daemon disconnected text removed from bottom bar, got %q", line)
	}
	if llmIdx > verboseIdx {
		t.Fatalf("llm alert should appear before verbose hints; got %q", line)
	}
}

func TestTruncateText_DisplayWidthSafe(t *testing.T) {
	got := truncateText("  hello world  ", 8)
	if got != "hello..." {
		t.Fatalf("ASCII truncate mismatch: got %q", got)
	}

	emoji := truncateText("😀😀😀😀", 5)
	if !utf8.ValidString(emoji) {
		t.Fatalf("emoji truncate produced invalid utf8: %q", emoji)
	}
	if runewidth.StringWidth(emoji) > 5 {
		t.Fatalf("emoji truncate exceeds width: %q", emoji)
	}

	cjk := truncateText("你好世界", 7)
	if !utf8.ValidString(cjk) {
		t.Fatalf("CJK truncate produced invalid utf8: %q", cjk)
	}
	if runewidth.StringWidth(cjk) > 7 {
		t.Fatalf("CJK truncate exceeds width: %q", cjk)
	}

	short := truncateText("abcdef", 3)
	if strings.Contains(short, "...") {
		t.Fatalf("max<=3 should not append suffix: got %q", short)
	}
}

func TestMonitorModelPicker_ProviderAndScopedFilteringWorks(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	runID := "test-run-model-filter"
	startMonitorTestRPCServer(t, cfg, runID)
	m, err := newMonitorModel(ctx, cfg, runID, &MonitorResult{})
	if err != nil {
		t.Fatalf("newMonitorModel: %v", err)
	}
	m.width = 120
	m.height = 40

	_ = m.openModelPicker()
	if !m.modelPickerOpen {
		t.Fatalf("expected modelPickerOpen true")
	}
	if !m.modelPickerProviderView {
		t.Fatalf("expected provider view initially")
	}
	if len(m.modelPickerList.Items()) == 0 {
		t.Fatalf("expected model picker items")
	}

	for _, r := range []rune("openai") {
		_, _ = m.updateModelPicker(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	if got := m.modelPickerQuery; got != "openai" {
		t.Fatalf("expected picker query %q, got %q", "openai", got)
	}
	items := m.modelPickerList.Items()
	if len(items) == 0 {
		t.Fatalf("expected provider items after filtering")
	}
	for _, it := range items {
		mi, ok := it.(monitorModelPickerItem)
		if !ok {
			t.Fatalf("expected monitorModelPickerItem, got %T", it)
		}
		if !mi.isProvider {
			t.Fatalf("expected provider item, got model item: %+v", mi)
		}
		if !strings.Contains(strings.ToLower(mi.provider), "openai") {
			t.Fatalf("expected provider to match query, got %+v", mi)
		}
	}

	_, _ = m.updateModelPicker(tea.KeyMsg{Type: tea.KeyEnter})
	if m.modelPickerProviderView {
		t.Fatalf("expected model view after selecting provider")
	}
	if got := strings.TrimSpace(m.modelPickerProvider); got == "" {
		t.Fatalf("expected selected provider in model view")
	}
	if got := m.modelPickerQuery; got != "" {
		t.Fatalf("expected query reset on provider select, got %q", got)
	}

	for _, r := range []rune("gpt-5") {
		_, _ = m.updateModelPicker(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if got := m.modelPickerQuery; got != "gpt-5" {
		t.Fatalf("expected scoped query %q, got %q", "gpt-5", got)
	}
	modelItems := m.modelPickerList.Items()
	if len(modelItems) == 0 {
		t.Fatalf("expected model items after scoped filtering")
	}
	for _, it := range modelItems {
		mi, ok := it.(monitorModelPickerItem)
		if !ok {
			t.Fatalf("expected monitorModelPickerItem, got %T", it)
		}
		if mi.isProvider {
			t.Fatalf("expected model item, got provider item: %+v", mi)
		}
		if !strings.Contains(strings.ToLower(mi.id), "gpt-5") {
			t.Fatalf("expected scoped model to contain %q, got %q", "gpt-5", mi.id)
		}
	}

	_, _ = m.updateModelPicker(tea.KeyMsg{Type: tea.KeyEsc})
	if !m.modelPickerProviderView {
		t.Fatalf("expected esc in model view to return to provider view")
	}
	if got := strings.TrimSpace(m.modelPickerProvider); got != "" {
		t.Fatalf("expected provider cleared after back, got %q", got)
	}
}

func TestMonitorProfilePicker_FilterAndSelectStartsNewStandaloneSession(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	startMonitorTestRPCServer(t, cfg, "profile-picker-run")

	// Seed a few profiles.
	profilesDir := fsutil.GetProfilesDir(cfg.DataDir)
	if err := os.MkdirAll(filepath.Join(profilesDir, "general"), 0o755); err != nil {
		t.Fatalf("mkdir profiles/general: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(profilesDir, "software_dev"), 0o755); err != nil {
		t.Fatalf("mkdir profiles/software_dev: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(profilesDir, "stock_analyst"), 0o755); err != nil {
		t.Fatalf("mkdir profiles/stock_analyst: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(profilesDir, "startup_team"), 0o755); err != nil {
		t.Fatalf("mkdir profiles/startup_team: %v", err)
	}
	writeProfile := func(dir, id, desc string) {
		t.Helper()
		raw := "id: " + id + "\ndescription: " + desc + "\nmodel: openai/gpt-5-mini\nprompts:\n  system_prompt: hello\n"
		if err := os.WriteFile(filepath.Join(dir, "profile.yaml"), []byte(raw), 0o644); err != nil {
			t.Fatalf("write profile.yaml: %v", err)
		}
	}
	writeProfile(filepath.Join(profilesDir, "general"), "general", "General profile")
	writeProfile(filepath.Join(profilesDir, "software_dev"), "software_dev", "Software development")
	writeProfile(filepath.Join(profilesDir, "stock_analyst"), "stock_analyst", "Stocks and markets")
	if err := os.WriteFile(
		filepath.Join(profilesDir, "startup_team", "profile.yaml"),
		[]byte("id: startup_team\ndescription: Team\nteam:\n  model: test\n  roles:\n    - name: ceo\n      coordinator: true\n      description: Lead\n      prompts:\n        system_prompt: lead\n"),
		0o644,
	); err != nil {
		t.Fatalf("write team profile.yaml: %v", err)
	}

	_, run, err := implstore.CreateSession(cfg, "profile filter test", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	runID := run.RunID
	m, err := newMonitorModel(ctx, cfg, runID, &MonitorResult{})
	if err != nil {
		t.Fatalf("newMonitorModel: %v", err)
	}
	m.width = 120
	m.height = 40

	_ = m.openProfilePickerFor("new-standalone", false)
	if !m.profilePickerOpen {
		t.Fatalf("expected profilePickerOpen true")
	}
	if len(m.profilePickerList.Items()) == 0 {
		t.Fatalf("expected profile picker items")
	}
	for _, it := range m.profilePickerList.Items() {
		pi, ok := it.(monitorProfilePickerItem)
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(pi.id), "startup_team") {
			t.Fatalf("standalone picker should not include team profiles")
		}
	}

	// Filter for "software".
	for _, r := range []rune("software") {
		_, _ = m.updateProfilePicker(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if got := m.profilePickerList.FilterInput.Value(); got != "software" {
		t.Fatalf("expected filter input %q, got %q", "software", got)
	}
	visible := m.profilePickerList.VisibleItems()
	if len(visible) == 0 {
		t.Fatalf("expected visible items after filtering")
	}
	first, ok := visible[0].(monitorProfilePickerItem)
	if !ok {
		t.Fatalf("expected monitorProfilePickerItem, got %T", visible[0])
	}
	if !strings.Contains(strings.ToLower(first.FilterValue()), "software") {
		t.Fatalf("expected filtered item to match %q, got %q", "software", first.FilterValue())
	}

	// Select the current item.
	_, cmd := m.updateProfilePicker(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("expected cmd from selection")
	}
	if m.profilePickerOpen {
		t.Fatalf("expected profilePickerOpen false after selection")
	}
	msg := cmd()
	sw, ok := msg.(monitorSwitchRunMsg)
	if !ok {
		t.Fatalf("expected monitorSwitchRunMsg, got %T", msg)
	}
	if strings.TrimSpace(sw.RunID) == "" {
		t.Fatalf("expected non-empty switched run id")
	}
	if m.profile != "software_dev" {
		t.Fatalf("expected monitor profile %q, got %q", "software_dev", m.profile)
	}
}

func TestMonitorProfilePicker_DefaultMode_DisallowsProfileSwitch(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()

	profilesDir := fsutil.GetProfilesDir(cfg.DataDir)
	if err := os.MkdirAll(filepath.Join(profilesDir, "general"), 0o755); err != nil {
		t.Fatalf("mkdir profiles/general: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profilesDir, "general", "profile.yaml"), []byte(
		"id: general\ndescription: General\nprompts:\n  system_prompt: hi\n"), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}

	_, run, err := implstore.CreateSession(cfg, "profile default mode", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	m, err := newMonitorModel(ctx, cfg, run.RunID, &MonitorResult{})
	if err != nil {
		t.Fatalf("newMonitorModel: %v", err)
	}
	_ = m.openProfilePicker()
	_, cmd := m.updateProfilePicker(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("expected command")
	}
	msg := cmd()
	lines, ok := msg.(commandLinesMsg)
	if !ok || len(lines.lines) == 0 {
		t.Fatalf("expected commandLinesMsg, got %#v", msg)
	}
	if !strings.Contains(strings.ToLower(lines.lines[0]), "disabled") {
		t.Fatalf("unexpected response: %q", lines.lines[0])
	}
}

func TestMonitorProfilePicker_ArrowKeysMoveSelection(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()

	profilesDir := fsutil.GetProfilesDir(cfg.DataDir)
	if err := os.MkdirAll(filepath.Join(profilesDir, "general"), 0o755); err != nil {
		t.Fatalf("mkdir profiles/general: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(profilesDir, "software_dev"), 0o755); err != nil {
		t.Fatalf("mkdir profiles/software_dev: %v", err)
	}
	writeProfile := func(dir, id, desc string) {
		t.Helper()
		raw := "id: " + id + "\ndescription: " + desc + "\nmodel: openai/gpt-5-mini\nprompts:\n  system_prompt: hello\n"
		if err := os.WriteFile(filepath.Join(dir, "profile.yaml"), []byte(raw), 0o644); err != nil {
			t.Fatalf("write profile.yaml: %v", err)
		}
	}
	writeProfile(filepath.Join(profilesDir, "general"), "general", "General profile")
	writeProfile(filepath.Join(profilesDir, "software_dev"), "software_dev", "Software development")

	_, run, err := implstore.CreateSession(cfg, "profile arrows", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	m, err := newMonitorModel(ctx, cfg, run.RunID, &MonitorResult{})
	if err != nil {
		t.Fatalf("newMonitorModel: %v", err)
	}
	m.width = 120
	m.height = 40

	_ = m.openProfilePicker()
	if len(m.profilePickerList.Items()) < 2 {
		t.Fatalf("expected at least two profiles")
	}
	start := m.profilePickerList.Index()
	_, _ = m.updateProfilePicker(tea.KeyMsg{Type: tea.KeyDown})
	if got := m.profilePickerList.Index(); got == start {
		t.Fatalf("expected down arrow to move selection, still at %d", got)
	}
	_, _ = m.updateProfilePicker(tea.KeyMsg{Type: tea.KeyUp})
	if got := m.profilePickerList.Index(); got != start {
		t.Fatalf("expected up arrow to return selection to %d, got %d", start, got)
	}
}

func TestAgentsToPickerItems_PrefersRoleLabel(t *testing.T) {
	items := agentsToPickerItems([]protocol.AgentListItem{
		{
			RunID:     "run-12345678-1234-1234-1234-1234567890ab",
			Role:      "cto",
			Profile:   "market_researcher",
			Status:    "running",
			Goal:      "Implement feature",
			TeamID:    "team-1",
			SessionID: "sess-1",
		},
	})
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	it, ok := items[0].(agentPickerItem)
	if !ok {
		t.Fatalf("expected agentPickerItem, got %T", items[0])
	}
	if got := strings.TrimSpace(it.Title()); !strings.HasPrefix(got, "cto · market_researcher · ") {
		t.Fatalf("expected role-prefixed title, got %q", got)
	}
}

func TestAgentsToPickerItems_UsesProfileWhenNoRole(t *testing.T) {
	items := agentsToPickerItems([]protocol.AgentListItem{
		{
			RunID:     "run-12345678-1234-1234-1234-1234567890ab",
			Profile:   "general",
			Status:    "running",
			Goal:      "Implement feature",
			SessionID: "sess-1",
		},
	})
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	it, ok := items[0].(agentPickerItem)
	if !ok {
		t.Fatalf("expected agentPickerItem, got %T", items[0])
	}
	if got := strings.TrimSpace(it.Title()); !strings.HasPrefix(got, "general · ") {
		t.Fatalf("expected profile-prefixed title, got %q", got)
	}
}

func TestParseNewSessionRequest(t *testing.T) {
	cases := []struct {
		in          string
		defaultProf string
		wantMode    string
		wantProfile string
		wantGoal    string
	}{
		{"", "general", "standalone", "general", ""},
		{"ship feature", "general", "standalone", "general", "ship feature"},
		{"standalone software_dev implement parser", "general", "standalone", "software_dev", "implement parser"},
		{"team startup_team launch", "general", "team", "startup_team", "launch"},
		{"team", "general", "team", "", ""},
	}
	for _, tc := range cases {
		got := parseNewSessionRequest(tc.in, tc.defaultProf)
		if got.Mode != tc.wantMode || got.Profile != tc.wantProfile || got.Goal != tc.wantGoal {
			t.Fatalf("parseNewSessionRequest(%q) => mode=%q profile=%q goal=%q, want mode=%q profile=%q goal=%q",
				tc.in, got.Mode, got.Profile, got.Goal, tc.wantMode, tc.wantProfile, tc.wantGoal)
		}
	}
}

func TestMonitorHandleCommand_NewTeamOpensTeamProfileWizard(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()

	profilesDir := fsutil.GetProfilesDir(cfg.DataDir)
	if err := os.MkdirAll(filepath.Join(profilesDir, "general"), 0o755); err != nil {
		t.Fatalf("mkdir general: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(profilesDir, "startup_team"), 0o755); err != nil {
		t.Fatalf("mkdir startup_team: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profilesDir, "general", "profile.yaml"), []byte(
		"id: general\ndescription: General\nprompts:\n  system_prompt: hi\n",
	), 0o644); err != nil {
		t.Fatalf("write general profile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profilesDir, "startup_team", "profile.yaml"), []byte(
		"id: startup_team\ndescription: Team\nteam:\n  roles:\n    - name: ceo\n      coordinator: true\n      description: Lead\n      prompts:\n        system_prompt: lead\n",
	), 0o644); err != nil {
		t.Fatalf("write team profile: %v", err)
	}

	_, run, err := implstore.CreateSession(cfg, "wizard", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	m, err := newMonitorModel(ctx, cfg, run.RunID, &MonitorResult{})
	if err != nil {
		t.Fatalf("newMonitorModel: %v", err)
	}
	if cmd := m.handleCommand("/new team"); cmd != nil {
		_ = cmd()
	}
	if !m.profilePickerOpen {
		t.Fatalf("expected profile picker open")
	}
	if !m.profilePickerTeamOnly {
		t.Fatalf("expected team-only profile picker")
	}
	if got := strings.TrimSpace(m.profilePickerMode); got != "new-team" {
		t.Fatalf("profilePickerMode=%q want new-team", got)
	}
	if len(m.profilePickerList.Items()) != 1 {
		t.Fatalf("expected only team profiles in picker, got %d", len(m.profilePickerList.Items()))
	}
}

func TestMonitorHandleCommand_RenameSession(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	startMonitorTestRPCServer(t, cfg, "rename-session-run")
	_, run, err := implstore.CreateSession(cfg, "before", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	m, err := newMonitorModel(ctx, cfg, run.RunID, &MonitorResult{})
	if err != nil {
		t.Fatalf("newMonitorModel: %v", err)
	}
	cmd := m.handleCommand("/rename after rename")
	if cmd == nil {
		t.Fatalf("expected command")
	}
	_ = cmd()
	sess, err := m.session.LoadSession(ctx, m.sessionID)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if got := strings.TrimSpace(sess.Title); got != "after rename" {
		t.Fatalf("title=%q want %q", got, "after rename")
	}
}

func TestMonitorSessionPicker_ShowsVisibleItemsWhenSessionsExist(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	startMonitorTestRPCServer(t, cfg, "session-picker-run")

	_, run, err := implstore.CreateSession(cfg, "session picker visibility", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if _, _, err := implstore.CreateSession(cfg, "second session", 8*1024); err != nil {
		t.Fatalf("CreateSession second: %v", err)
	}

	m, err := newMonitorModel(ctx, cfg, run.RunID, &MonitorResult{})
	if err != nil {
		t.Fatalf("newMonitorModel: %v", err)
	}
	m.width = 120
	m.height = 40

	cmd := m.openSessionPicker()
	if cmd == nil {
		t.Fatalf("expected fetch command from openSessionPicker")
	}
	msg := cmd()
	updatedModel, _ := m.Update(msg)
	updated, ok := updatedModel.(*monitorModel)
	if !ok {
		t.Fatalf("expected *monitorModel, got %T", updatedModel)
	}
	if updated.sessionPickerTotal < 2 {
		t.Fatalf("expected sessionPickerTotal >= 2, got %d", updated.sessionPickerTotal)
	}
	if len(updated.sessionPickerList.Items()) == 0 {
		t.Fatalf("expected picker items, got 0")
	}
	if len(updated.sessionPickerList.VisibleItems()) == 0 {
		t.Fatalf("expected visible picker items, got 0")
	}
}

func TestMonitorDetached_SessionPickerLoadsSessions(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	startMonitorTestRPCServer(t, cfg, "detached-picker-run")

	if _, _, err := implstore.CreateSession(cfg, "detached session 1", 8*1024); err != nil {
		t.Fatalf("CreateSession 1: %v", err)
	}
	if _, _, err := implstore.CreateSession(cfg, "detached session 2", 8*1024); err != nil {
		t.Fatalf("CreateSession 2: %v", err)
	}

	m, err := newDetachedMonitorModel(ctx, cfg, &MonitorResult{})
	if err != nil {
		t.Fatalf("newDetachedMonitorModel: %v", err)
	}
	if !m.isDetached() {
		t.Fatalf("expected detached model")
	}
	m.width = 120
	m.height = 40

	cmd := m.openSessionPicker()
	if cmd == nil {
		t.Fatalf("expected fetch command from openSessionPicker")
	}
	msg := cmd()
	updatedModel, _ := m.Update(msg)
	updated, ok := updatedModel.(*monitorModel)
	if !ok {
		t.Fatalf("expected *monitorModel, got %T", updatedModel)
	}
	if updated.sessionPickerTotal < 2 {
		t.Fatalf("expected sessionPickerTotal >= 2, got %d", updated.sessionPickerTotal)
	}
	if len(updated.sessionPickerList.Items()) == 0 {
		t.Fatalf("expected picker items, got 0")
	}
	if len(updated.sessionPickerList.VisibleItems()) == 0 {
		t.Fatalf("expected visible picker items, got 0")
	}
}

func TestMonitorDetached_SessionSwitch_QueuesTasksToSelectedRun(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	bootstrapRunID := "bootstrap-switch-run"
	startMonitorTestRPCServer(t, cfg, bootstrapRunID)

	_, targetRun, err := implstore.CreateSession(cfg, "target switch session", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession target: %v", err)
	}

	m, err := newDetachedMonitorModel(ctx, cfg, &MonitorResult{})
	if err != nil {
		t.Fatalf("newDetachedMonitorModel: %v", err)
	}
	m.width = 120
	m.height = 40

	cmd := m.openSessionPicker()
	if cmd == nil {
		t.Fatalf("expected fetch command from openSessionPicker")
	}
	msg := cmd()
	updatedModel, _ := m.Update(msg)
	updated, ok := updatedModel.(*monitorModel)
	if !ok {
		t.Fatalf("expected *monitorModel, got %T", updatedModel)
	}

	items := updated.sessionPickerList.Items()
	targetIdx := -1
	for i, it := range items {
		sp, ok := it.(sessionPickerItem)
		if !ok {
			continue
		}
		if strings.TrimSpace(sp.id) == strings.TrimSpace(targetRun.SessionID) {
			targetIdx = i
			break
		}
	}
	if targetIdx < 0 {
		t.Fatalf("target session %q not present in picker", targetRun.SessionID)
	}
	updated.sessionPickerList.Select(targetIdx)

	switchCmd := updated.selectSessionFromPicker()
	if switchCmd == nil {
		t.Fatalf("expected switch command")
	}
	switchMsg := switchCmd()
	switchRunMsg, ok := switchMsg.(monitorSwitchRunMsg)
	if !ok {
		t.Fatalf("expected monitorSwitchRunMsg, got %T", switchMsg)
	}
	if got := strings.TrimSpace(switchRunMsg.RunID); got != strings.TrimSpace(targetRun.RunID) {
		t.Fatalf("switch run id=%q want %q", got, targetRun.RunID)
	}

	switchedModel, reloadCmd := updated.Update(switchMsg)
	switched, ok := switchedModel.(*monitorModel)
	if !ok {
		t.Fatalf("expected *monitorModel after switch update, got %T", switchedModel)
	}
	if reloadCmd == nil {
		t.Fatalf("expected reload command")
	}
	reloadMsg := reloadCmd()
	reloadedModel, _ := switched.Update(reloadMsg)
	reloaded, ok := reloadedModel.(*monitorModel)
	if !ok {
		t.Fatalf("expected *monitorModel after reload, got %T", reloadedModel)
	}

	if got := strings.TrimSpace(reloaded.runID); got != strings.TrimSpace(targetRun.RunID) {
		t.Fatalf("active run=%q want %q", got, targetRun.RunID)
	}
	if got := strings.TrimSpace(reloaded.sessionID); got != strings.TrimSpace(targetRun.SessionID) {
		t.Fatalf("active session=%q want %q", got, targetRun.SessionID)
	}

	queueCmd := reloaded.handleCommand("task routed to switched run")
	if queueCmd == nil {
		t.Fatalf("expected enqueue command")
	}
	_ = queueCmd()

	targetTasks, err := reloaded.taskStore.ListTasks(ctx, agentstate.TaskFilter{
		RunID:    strings.TrimSpace(targetRun.RunID),
		SortBy:   "created_at",
		SortDesc: true,
		Limit:    20,
	})
	if err != nil {
		t.Fatalf("ListTasks target: %v", err)
	}
	foundOnTarget := false
	for _, tk := range targetTasks {
		if strings.TrimSpace(tk.Goal) == "task routed to switched run" {
			foundOnTarget = true
			break
		}
	}
	if !foundOnTarget {
		t.Fatalf("expected queued task on target run")
	}

	bootstrapTasks, err := reloaded.taskStore.ListTasks(ctx, agentstate.TaskFilter{
		RunID:    strings.TrimSpace(bootstrapRunID),
		SortBy:   "created_at",
		SortDesc: true,
		Limit:    20,
	})
	if err != nil {
		t.Fatalf("ListTasks bootstrap: %v", err)
	}
	for _, tk := range bootstrapTasks {
		if strings.TrimSpace(tk.Goal) == "task routed to switched run" {
			t.Fatalf("task was incorrectly queued on bootstrap run")
		}
	}
}

func TestMonitorDetached_SessionPickerRequiresDaemonRPC(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()

	if _, _, err := implstore.CreateSession(cfg, "offline session 1", 8*1024); err != nil {
		t.Fatalf("CreateSession 1: %v", err)
	}
	if _, _, err := implstore.CreateSession(cfg, "offline session 2", 8*1024); err != nil {
		t.Fatalf("CreateSession 2: %v", err)
	}
	// Intentionally point at an unused endpoint.
	t.Setenv("AGEN8_RPC_ENDPOINT", "127.0.0.1:1")

	m, err := newDetachedMonitorModel(ctx, cfg, &MonitorResult{})
	if err != nil {
		t.Fatalf("newDetachedMonitorModel: %v", err)
	}

	cmd := m.openSessionPicker()
	if cmd == nil {
		t.Fatalf("expected fetch command from openSessionPicker")
	}
	msg := cmd()
	updatedModel, _ := m.Update(msg)
	updated, ok := updatedModel.(*monitorModel)
	if !ok {
		t.Fatalf("expected *monitorModel, got %T", updatedModel)
	}
	if strings.TrimSpace(updated.sessionPickerErr) == "" {
		t.Fatalf("expected rpc error in session picker")
	}
	if updated.sessionPickerTotal != 0 {
		t.Fatalf("expected sessionPickerTotal=0 when rpc unavailable, got %d", updated.sessionPickerTotal)
	}
	if len(updated.sessionPickerList.Items()) != 0 {
		t.Fatalf("expected no session picker items when rpc unavailable, got %d", len(updated.sessionPickerList.Items()))
	}
}

func TestMonitorDetached_ViewShowsDisconnectedBanner(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()

	m, err := newDetachedMonitorModel(ctx, cfg, &MonitorResult{})
	if err != nil {
		t.Fatalf("newDetachedMonitorModel: %v", err)
	}
	m.width = 120
	m.height = 40
	m.rpcHealthKnown = true
	m.rpcReachable = false
	view := m.View()
	if !strings.Contains(view, "Daemon disconnected") {
		t.Fatalf("expected disconnected banner in view, got: %q", view)
	}
}

func TestMonitorHandleCommand_ReconnectUpdatesHealthState(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	t.Setenv("AGEN8_RPC_ENDPOINT", "127.0.0.1:1")

	m, err := newDetachedMonitorModel(ctx, cfg, &MonitorResult{})
	if err != nil {
		t.Fatalf("newDetachedMonitorModel: %v", err)
	}
	cmd := m.handleCommand("/reconnect")
	if cmd == nil {
		t.Fatalf("expected reconnect command")
	}
	msg := cmd()
	health, ok := msg.(rpcHealthMsg)
	if !ok {
		t.Fatalf("expected rpcHealthMsg, got %T", msg)
	}
	if health.reachable {
		t.Fatalf("expected unreachable for invalid endpoint")
	}
	if !health.manual {
		t.Fatalf("expected manual reconnect msg")
	}
	updatedModel, _ := m.Update(msg)
	updated, ok := updatedModel.(*monitorModel)
	if !ok {
		t.Fatalf("expected *monitorModel, got %T", updatedModel)
	}
	if !updated.rpcHealthKnown || updated.rpcReachable {
		t.Fatalf("expected disconnected health state, known=%v reachable=%v", updated.rpcHealthKnown, updated.rpcReachable)
	}
	if len(updated.agentOutput) == 0 {
		t.Fatalf("expected reconnect feedback in agent output")
	}
	joined := ""
	for _, it := range updated.agentOutput {
		if joined != "" {
			joined += "\n"
		}
		joined += it.Content
	}
	if !strings.Contains(joined, "Daemon RPC disconnected") {
		t.Fatalf("expected reconnect feedback in agent output: %q", joined)
	}
}

func TestMonitorSessionPicker_SelectTeamSessionSwitchesTeam(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()

	m, err := newDetachedMonitorModel(ctx, cfg, &MonitorResult{})
	if err != nil {
		t.Fatalf("newDetachedMonitorModel: %v", err)
	}

	l := list.New(nil, newSessionPickerDelegate(), 0, 0)
	l.SetItems([]list.Item{
		sessionPickerItem{
			id:      "sess-team-1",
			title:   "Team Session",
			mode:    "team",
			teamID:  "team-123",
			profile: "startup_team",
		},
	})
	l.Select(0)
	m.sessionPickerList = l
	m.sessionPickerOpen = true

	cmd := m.selectSessionFromPicker()
	if cmd == nil {
		t.Fatalf("expected switch command")
	}
	msg := cmd()
	sw, ok := msg.(monitorSwitchTeamMsg)
	if !ok {
		t.Fatalf("expected monitorSwitchTeamMsg, got %T", msg)
	}
	if got := strings.TrimSpace(sw.TeamID); got != "team-123" {
		t.Fatalf("TeamID=%q want %q", got, "team-123")
	}
}

func TestMonitorAgentPicker_TeamModeSetsFocusWithoutRunSwitch(t *testing.T) {
	m := &monitorModel{
		teamID:          "team-123",
		teamRoleByRunID: map[string]string{"run-1": "ceo"},
	}
	l := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	l.SetItems([]list.Item{
		agentPickerItem{
			runID:  "run-1",
			label:  "ceo · general · run-1",
			status: "running",
			role:   "ceo",
			teamID: "team-123",
		},
	})
	l.Select(0)
	m.agentPickerList = l
	m.agentPickerOpen = true

	cmd := m.selectAgentFromPicker()
	if cmd == nil {
		t.Fatalf("expected command")
	}
	if got := strings.TrimSpace(m.focusedRunID); got != "run-1" {
		t.Fatalf("focusedRunID=%q want run-1", got)
	}
	if got := strings.TrimSpace(m.focusedRunRole); got != "ceo" {
		t.Fatalf("focusedRunRole=%q want ceo", got)
	}
	msg := cmd()
	if _, ok := msg.(monitorSwitchRunMsg); ok {
		t.Fatalf("team-mode agent picker must not switch out to run monitor")
	}
}

func TestMonitorDetached_EnqueueTaskRequiresContext(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	m, err := newDetachedMonitorModel(ctx, cfg, &MonitorResult{})
	if err != nil {
		t.Fatalf("newDetachedMonitorModel: %v", err)
	}
	cmd := m.handleCommand("do the thing")
	if cmd == nil {
		t.Fatalf("expected command")
	}
	msg := cmd()
	lines, ok := msg.(commandLinesMsg)
	if !ok || len(lines.lines) == 0 {
		t.Fatalf("expected commandLinesMsg, got %#v", msg)
	}
	if !strings.Contains(strings.ToLower(lines.lines[0]), "no active context") {
		t.Fatalf("unexpected message: %q", lines.lines[0])
	}
}

func TestMonitorCommandPalette_ProfileRemoved(t *testing.T) {
	foundStop := false
	foundClear := false
	for _, cmd := range monitorAvailableCommands {
		if cmd == "/profile" {
			t.Fatalf("/profile should not be present in command palette")
		}
		if cmd == "/stop" {
			foundStop = true
		}
		if cmd == "/clear" {
			foundClear = true
		}
	}
	if !foundStop {
		t.Fatalf("/stop should be present in command palette")
	}
	if !foundClear {
		t.Fatalf("/clear should be present in command palette")
	}
	if monitorCommandInvokesWithoutArgs("/profile") {
		t.Fatalf("/profile should not be invokable")
	}
	if !monitorCommandInvokesWithoutArgs("/stop") {
		t.Fatalf("/stop should be invokable without args")
	}
	if !monitorCommandInvokesWithoutArgs("/clear") {
		t.Fatalf("/clear should be invokable without args")
	}
}

func TestHandleCommand_ClearOpensConfirmModal(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	startMonitorTestRPCServer(t, cfg, "run-clear-open-modal")
	_, run, err := implstore.CreateSession(cfg, "clear confirm", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	m, err := newMonitorModel(ctx, cfg, run.RunID, &MonitorResult{})
	if err != nil {
		t.Fatalf("newMonitorModel: %v", err)
	}
	if cmd := m.handleCommand("/clear"); cmd != nil {
		_ = cmd()
	}
	if !m.confirmModalOpen {
		t.Fatalf("expected confirm modal to open")
	}
	if m.confirmAction != confirmActionClearHistory {
		t.Fatalf("expected clear history confirm action, got %q", m.confirmAction)
	}
}

func TestSessionPicker_KeyDOpensDeleteConfirm(t *testing.T) {
	m := &monitorModel{}
	l := list.New([]list.Item{
		sessionPickerItem{id: "sess-1", title: "one"},
	}, kit.NewPickerDelegate(kit.DefaultPickerDelegateStyles(), renderSessionPickerLine), 0, 0)
	m.sessionPickerOpen = true
	m.sessionPickerList = l
	model, _ := m.updateSessionPicker(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	updated, ok := model.(*monitorModel)
	if !ok {
		t.Fatalf("expected *monitorModel, got %T", model)
	}
	if !updated.confirmModalOpen {
		t.Fatalf("expected confirm modal to open")
	}
	if updated.confirmAction != confirmActionDeleteSession {
		t.Fatalf("expected delete session action, got %q", updated.confirmAction)
	}
	if strings.TrimSpace(updated.confirmSessionID) != "sess-1" {
		t.Fatalf("confirmSessionID=%q want sess-1", updated.confirmSessionID)
	}
}

func TestMonitorView_NoClipping_DashboardMode(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	runID := "test-run-no-clip-dashboard"
	m, err := newMonitorModel(ctx, cfg, runID, &MonitorResult{})
	if err != nil {
		t.Fatalf("newMonitorModel: %v", err)
	}
	m.width = 120
	m.height = 45
	m.runStatus = types.RunStatusRunning
	m.layout()
	m.refreshViewports()

	if m.isCompactMode() {
		t.Fatalf("expected dashboard mode at 120x45")
	}
	view := m.View()
	gotH := lipgloss.Height(view)
	if gotH > 45 {
		t.Fatalf("View() height %d exceeds terminal height 45", gotH)
	}
	for i, line := range strings.Split(view, "\n") {
		if w := lipgloss.Width(line); w > 120 {
			t.Fatalf("line %d width %d exceeds terminal width 120", i+1, w)
		}
	}
}

func TestMonitorView_NoClipping_100x30_Compact(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	runID := "test-run-no-clip-100x30"
	m, err := newMonitorModel(ctx, cfg, runID, &MonitorResult{})
	if err != nil {
		t.Fatalf("newMonitorModel: %v", err)
	}
	m.width = 100
	m.height = 30

	if !m.isCompactMode() {
		t.Fatalf("expected compact mode at 100x30")
	}
	m.layout()
	m.refreshViewports()

	view := m.View()
	gotH := lipgloss.Height(view)
	if gotH > 30 {
		t.Fatalf("View() height %d exceeds terminal height 30 (compact mode)", gotH)
	}
	for i, line := range strings.Split(view, "\n") {
		if w := lipgloss.Width(line); w > 100 {
			t.Fatalf("line %d width %d exceeds terminal width 100", i+1, w)
		}
	}
}

func TestMonitorHandleCommand_TeamCommandOnlyInTeamMode(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()

	m, err := newMonitorModel(ctx, cfg, "run-non-team", &MonitorResult{})
	if err != nil {
		t.Fatalf("newMonitorModel: %v", err)
	}
	cmd := m.handleCommand("/team")
	if cmd == nil {
		t.Fatalf("expected command output for /team in non-team mode")
	}
	msg := cmd()
	linesMsg, ok := msg.(commandLinesMsg)
	if !ok || len(linesMsg.lines) == 0 {
		t.Fatalf("expected commandLinesMsg with error text, got %#v", msg)
	}
	if !strings.Contains(linesMsg.lines[0], "only available in team monitor") {
		t.Fatalf("unexpected command response: %q", linesMsg.lines[0])
	}

	tm, err := newTeamMonitorModel(ctx, cfg, "team-a", &MonitorResult{})
	if err != nil {
		t.Fatalf("newTeamMonitorModel: %v", err)
	}
	if cmd := tm.handleCommand("/team extra"); cmd == nil {
		t.Fatalf("expected usage output for /team extra")
	}
	_ = tm.handleCommand("/team")
	if !tm.teamPickerOpen {
		t.Fatalf("expected team picker to open for /team in team monitor")
	}
}

func TestScopedTaskFilter_FocusedRunUsesRunID(t *testing.T) {
	m := &monitorModel{
		teamID:       "team-a",
		focusedRunID: "run-a",
	}
	filter := m.scopedTaskFilter(agentstate.TaskFilter{
		AssignedRole: "researcher",
		RunID:        "ignored",
	})
	if filter.TeamID != "team-a" {
		t.Fatalf("TeamID=%q, want %q", filter.TeamID, "team-a")
	}
	if filter.RunID != "run-a" {
		t.Fatalf("RunID=%q, want %q", filter.RunID, "run-a")
	}
	if filter.AssignedRole != "" {
		t.Fatalf("AssignedRole=%q, want empty", filter.AssignedRole)
	}
}

func TestScopedTaskFilter_NonTeamUsesRunID(t *testing.T) {
	m := &monitorModel{
		runID: "run-single",
	}
	filter := m.scopedTaskFilter(agentstate.TaskFilter{
		TeamID: "team-should-clear",
		RunID:  "run-should-replace",
	})
	if filter.TeamID != "" {
		t.Fatalf("TeamID=%q, want empty", filter.TeamID)
	}
	if filter.RunID != "run-single" {
		t.Fatalf("RunID=%q, want %q", filter.RunID, "run-single")
	}
}

func TestUpdateTeamManifestLoadedMsg_PrefersPendingRequestedModel(t *testing.T) {
	m := &monitorModel{
		teamID: "team-a",
		model:  "openai/gpt-5",
	}
	_, _ = m.Update(teamManifestLoadedMsg{
		manifest: &teamManifestFile{
			TeamID:    "team-a",
			TeamModel: "openai/gpt-5",
			ModelChange: &teamModelChangeFile{
				RequestedModel: "anthropic/claude-3.7-sonnet",
				Status:         "pending",
			},
		},
	})
	if got := strings.TrimSpace(m.model); got != "anthropic/claude-3.7-sonnet" {
		t.Fatalf("model=%q, want pending requested model", got)
	}
}

func TestResolveTeamControlSessionID_PrefersCoordinatorSession(t *testing.T) {
	manifest := &teamManifestFile{
		CoordinatorRun: "run-coordinator",
		Roles: []teamManifestRole{
			{RoleName: "researcher", RunID: "run-research", SessionID: "sess-research"},
			{RoleName: "coordinator", RunID: "run-coordinator", SessionID: "sess-coordinator"},
		},
	}
	got := resolveTeamControlSessionID(manifest, "")
	if got != "sess-coordinator" {
		t.Fatalf("control session=%q want %q", got, "sess-coordinator")
	}
}

func TestUpdateTeamManifestLoadedMsg_UpdatesSessionToCoordinator(t *testing.T) {
	m := &monitorModel{
		teamID:    "team-a",
		sessionID: "sess-wrong",
	}
	_, _ = m.Update(teamManifestLoadedMsg{
		manifest: &teamManifestFile{
			TeamID:         "team-a",
			CoordinatorRun: "run-coordinator",
			Roles: []teamManifestRole{
				{RoleName: "worker", RunID: "run-worker", SessionID: "sess-worker"},
				{RoleName: "coordinator", RunID: "run-coordinator", SessionID: "sess-coordinator"},
			},
		},
	})
	if got := strings.TrimSpace(m.sessionID); got != "sess-coordinator" {
		t.Fatalf("sessionID=%q, want %q", got, "sess-coordinator")
	}
	if got := strings.TrimSpace(m.rpcRun().SessionID); got != "sess-coordinator" {
		t.Fatalf("rpc sessionID=%q, want %q", got, "sess-coordinator")
	}
}

func TestLoadPlanFilesCmd_TeamFocusedLoadsSingleRunPlan(t *testing.T) {
	dataDir := t.TempDir()
	cfg := config.Config{DataDir: dataDir}
	endpoint := startMonitorTestRPCServer(t, cfg, "run-a")
	writePlan := func(runID, head, checklist string) {
		t.Helper()
		planDir := filepath.Join(fsutil.GetAgentDir(dataDir, runID), "plan")
		if err := os.MkdirAll(planDir, 0o755); err != nil {
			t.Fatalf("mkdir plan dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(planDir, "HEAD.md"), []byte(head), 0o644); err != nil {
			t.Fatalf("write head: %v", err)
		}
		if err := os.WriteFile(filepath.Join(planDir, "CHECKLIST.md"), []byte(checklist), 0o644); err != nil {
			t.Fatalf("write checklist: %v", err)
		}
	}
	writePlan("run-a", "head-a", "check-a")
	writePlan("run-b", "head-b", "check-b")

	m := &monitorModel{
		cfg:         cfg,
		sessionID:   "sess-rpc-test",
		runID:       "run-a",
		rpcEndpoint: endpoint,
	}

	cmd := m.loadPlanFilesCmd()
	if cmd == nil {
		t.Fatalf("expected non-nil plan load cmd")
	}
	msg, ok := cmd().(planFilesLoadedMsg)
	if !ok {
		t.Fatalf("unexpected msg type from loadPlanFilesCmd")
	}
	if !strings.Contains(msg.details, "head-a") || strings.Contains(msg.details, "head-b") {
		t.Fatalf("expected focused run details only, got %q", msg.details)
	}
	if !strings.Contains(msg.checklist, "check-a") || strings.Contains(msg.checklist, "check-b") {
		t.Fatalf("expected focused run checklist only, got %q", msg.checklist)
	}
}

func TestHandleCommand_StopSession(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	endpoint := startMonitorTestRPCServer(t, cfg, "run-stop")
	m := &monitorModel{
		cfg:         cfg,
		runID:       "run-stop",
		sessionID:   "sess-rpc-test",
		rpcEndpoint: endpoint,
	}
	cmd := m.handleCommand("/stop")
	if cmd == nil {
		t.Fatalf("expected command")
	}
	raw := cmd()
	msg, ok := raw.(commandLinesMsg)
	if !ok || len(msg.lines) == 0 {
		t.Fatalf("unexpected message: %#v", raw)
	}
	if !strings.Contains(msg.lines[0], "[stop] session stopped") {
		t.Fatalf("unexpected stop output: %q", msg.lines[0])
	}
}

func TestRefreshThinkingViewport_FiltersByFocusedRunID(t *testing.T) {
	m := &monitorModel{
		focusedRunID: "run-a",
		thinkingEntries: []thinkingEntry{
			{RunID: "run-a", Role: "researcher", Summary: "focus-summary"},
			{RunID: "run-b", Role: "writer", Summary: "other-summary"},
		},
		thinkingVP: viewport.New(0, 0),
	}
	m.thinkingVP.Width = 80
	m.thinkingVP.Height = 20
	m.refreshThinkingViewport()
	view := m.thinkingVP.View()
	if !strings.Contains(view, "focus-summary") {
		t.Fatalf("expected focused summary in thinking viewport: %q", view)
	}
	if strings.Contains(view, "other-summary") {
		t.Fatalf("did not expect non-focused summary in thinking viewport: %q", view)
	}
}

func TestRefreshThinkingViewport_FocusedRunFallsBackToRoleWhenRunIDMissing(t *testing.T) {
	m := &monitorModel{
		focusedRunID:   "run-a",
		focusedRunRole: "ceo",
		thinkingEntries: []thinkingEntry{
			{RunID: "", Role: "ceo", Summary: "legacy-no-runid"},
			{RunID: "", Role: "writer", Summary: "other-role"},
		},
		thinkingVP: viewport.New(0, 0),
	}
	m.thinkingVP.Width = 80
	m.thinkingVP.Height = 20
	m.refreshThinkingViewport()
	view := m.thinkingVP.View()
	if !strings.Contains(view, "legacy-no-runid") {
		t.Fatalf("expected role-fallback summary in thinking viewport: %q", view)
	}
	if strings.Contains(view, "other-role") {
		t.Fatalf("did not expect non-matching role summary in thinking viewport: %q", view)
	}
}

func TestRefreshThinkingViewport_UsesAlignedTimelineGutter(t *testing.T) {
	m := &monitorModel{
		thinkingEntries: []thinkingEntry{
			{RunID: "run-a", Role: "researcher", Summary: "first line\nsecond line"},
			{RunID: "run-b", Role: "writer", Summary: "last line\ntail"},
		},
		thinkingVP: viewport.New(0, 0),
	}
	m.thinkingVP.Width = 80
	m.thinkingVP.Height = 20
	m.refreshThinkingViewport()
	view := m.thinkingVP.View()
	if !strings.Contains(view, "●") {
		t.Fatalf("expected timeline node marker in view: %q", view)
	}
	if !strings.Contains(view, "│") {
		t.Fatalf("expected timeline spine marker in view: %q", view)
	}
	if !strings.Contains(view, "second line") || !strings.Contains(view, "tail") {
		t.Fatalf("expected multiline summaries to render in timeline view: %q", view)
	}
}

func TestAgentOutputFocusFilter_ShowsGlobalAndFocusedRunOnly(t *testing.T) {
	m := &monitorModel{
		teamID:           "team-a",
		focusedRunID:     "run-a",
		agentOutput:      []AgentOutputItem{{Content: "unscoped line"}, {Content: "focused line", RunID: "run-a"}, {Content: "other line", RunID: "run-b"}},
		agentOutputRunID: []string{"", "run-a", "run-b"},
		agentOutputVP:    viewport.New(0, 0),
	}
	m.agentOutputVP.Width = 80
	m.agentOutputVP.Height = 20
	m.refreshAgentOutputViewport()
	view := m.agentOutputVP.View()
	if !strings.Contains(view, "focused line") {
		t.Fatalf("expected focused line in output: %q", view)
	}
	if !strings.Contains(view, "unscoped line") {
		t.Fatalf("expected unscoped/global line in focused output: %q", view)
	}
	if strings.Contains(view, "other line") {
		t.Fatalf("unexpected other-run line in focused output: %q", view)
	}
}

func TestFormatTaskEventLines_UsesSummaryMarker(t *testing.T) {
	ev := types.EventRecord{
		Type:      "task.done",
		Timestamp: time.Now(),
		Data: map[string]string{
			"taskId":  "task-1",
			"status":  "succeeded",
			"summary": "## Result\n- done",
		},
	}
	lines := formatTaskEventLines(ev)
	found := false
	for _, line := range lines {
		if strings.HasPrefix(line, agentOutputSummaryMarker) {
			found = true
		}
		if strings.Contains(line, "summary: ") {
			t.Fatalf("did not expect legacy summary label in line %q", line)
		}
	}
	if !found {
		t.Fatalf("expected summary marker line, got %#v", lines)
	}
}

func TestFormatTaskEventLines_BatchEventsIncludeWaveID(t *testing.T) {
	now := time.Now()
	progress := formatTaskEventLines(types.EventRecord{
		Type:      "callback.batch.progress",
		Timestamp: now,
		Data: map[string]string{
			"parentTaskId":        "task-parent-1",
			"batchWaveId":         "wave-parent-1",
			"batchCompletedCount": "3",
			"batchExpectedCount":  "5",
		},
	})
	if len(progress) == 0 || !strings.Contains(progress[0], "wave=") {
		t.Fatalf("expected wave marker in progress line: %#v", progress)
	}

	queued := formatTaskEventLines(types.EventRecord{
		Type:      "callback.batch.queued",
		Timestamp: now,
		Data: map[string]string{
			"parentTaskId":     "task-parent-1",
			"batchWaveId":      "wave-parent-1",
			"items":            "5",
			"batchFlushReason": "all_complete",
		},
	})
	if len(queued) == 0 || !strings.Contains(queued[0], "wave=") {
		t.Fatalf("expected wave marker in queued line: %#v", queued)
	}
}

func TestFormatTaskEventLines_InvalidRepeated(t *testing.T) {
	now := time.Now()
	invalid := formatTaskEventLines(types.EventRecord{
		Type:      "task.tool.invalid_repeated",
		Timestamp: now,
		Data: map[string]string{
			"taskId":             "task-coord-1",
			"tool":               "task_create",
			"elapsedSeconds":     "5",
			"consecutiveInvalid": "6",
			"reason":             "task_create.assignedRole is required for coordinators in team mode",
		},
	})
	if len(invalid) == 0 || !strings.Contains(invalid[0], "task.tool.invalid_repeated") || !strings.Contains(invalid[0], "consecutiveInvalid=6") {
		t.Fatalf("expected task.tool.invalid_repeated line with count, got %#v", invalid)
	}
}

func TestRefreshAgentOutputViewport_RendersSummaryMarkdownWithoutSummaryLabel(t *testing.T) {
	m := &monitorModel{
		renderer: newContentRenderer(),
		agentOutput: []AgentOutputItem{
			{Content: "[12:00:00] task.done: task-1 succeeded"},
			{Content: agentOutputSummaryMarker + "## Result\n- first\n- second"},
		},
		agentOutputRunID: []string{"", ""},
		agentOutputVP:    viewport.New(0, 0),
	}
	m.agentOutputVP.Width = 80
	m.agentOutputVP.Height = 20
	m.refreshAgentOutputViewport()
	view := m.agentOutputVP.View()
	if strings.Contains(view, "summary:") {
		t.Fatalf("unexpected summary label in rendered output: %q", view)
	}
	if !strings.Contains(view, "Result") || !strings.Contains(view, "first") {
		t.Fatalf("expected markdown summary content rendered, got: %q", view)
	}
}

func TestHandleTailAndStreamMessages_FiltersLegacyTaskDoneCommandLinesInTeamMode(t *testing.T) {
	m := &monitorModel{
		teamID: "team-1",
	}
	_, _ = m.handleTailAndStreamMessages(commandLinesMsg{
		lines: []string{
			"[10:49:45] [marketing-strategist] task.done task-202 succeeded - As the marketing strategist...",
			"[10:49:45] Write /tasks/marketing-strategist/2026-02-18/task-202/SUMMARY",
			"As the marketing strategist, I develop data-driven plans...",
			"[10:49:45] [marketing-strategist] task.done: task-202 succeeded goal=\"Provide a concise summary\"",
		},
	})
	if len(m.agentOutput) != 1 {
		t.Fatalf("expected only structured task.done line, got %d items: %#v", len(m.agentOutput), m.agentOutput)
	}
	if got := m.agentOutput[0].Content; !strings.Contains(got, "task.done: task-202 succeeded") {
		t.Fatalf("unexpected preserved line: %q", got)
	}
}

func TestTrimAgentOutputBuffer_KeepsRunIDSliceInSync(t *testing.T) {
	size := agentOutputMaxLines + 15
	m := &monitorModel{
		agentOutput:              make([]AgentOutputItem, size),
		agentOutputRunID:         make([]string, size),
		agentOutputFilteredCache: []AgentOutputItem{{Content: "cached"}},
	}
	for i := 0; i < size; i++ {
		m.agentOutput[i] = AgentOutputItem{Content: "line", RunID: "run-a"}
		m.agentOutputRunID[i] = "run-a"
	}
	m.trimAgentOutputBuffer()
	if len(m.agentOutput) != len(m.agentOutputRunID) {
		t.Fatalf("agent output/runID length mismatch: %d vs %d", len(m.agentOutput), len(m.agentOutputRunID))
	}
	if m.agentOutputFilteredCache != nil {
		t.Fatalf("expected filtered cache reset after trim")
	}
}

func TestTeamPickerSelect_ClearAndRunSelection(t *testing.T) {
	m := &monitorModel{
		teamID:     "team-a",
		teamRunIDs: []string{"run-a", "run-b"},
		teamRoleByRunID: map[string]string{
			"run-a": "researcher",
			"run-b": "writer",
		},
	}
	_ = m.openTeamPicker()
	if !m.teamPickerOpen {
		t.Fatalf("expected team picker open")
	}
	items := m.teamPickerList.Items()
	idxRunA := -1
	for i, raw := range items {
		item, ok := raw.(teamPickerItem)
		if ok && item.runID == "run-a" {
			idxRunA = i
			break
		}
	}
	if idxRunA < 0 {
		t.Fatalf("expected run-a item in team picker")
	}
	m.teamPickerList.Select(idxRunA)
	cmd := m.selectFromTeamPicker()
	if cmd != nil {
		_ = cmd()
	}
	if m.focusedRunID != "run-a" {
		t.Fatalf("focusedRunID=%q, want %q", m.focusedRunID, "run-a")
	}

	_ = m.openTeamPicker()
	m.teamPickerList.Select(0)
	cmd = m.selectFromTeamPicker()
	if cmd != nil {
		_ = cmd()
	}
	if m.focusedRunID != "" {
		t.Fatalf("expected focus to clear, got %q", m.focusedRunID)
	}
}

func TestFocusedRunAutoClearsWhenRunRemoved(t *testing.T) {
	m := &monitorModel{
		teamID:         "team-a",
		focusedRunID:   "run-a",
		focusedRunRole: "researcher",
		teamRunIDs:     []string{"run-b"},
		teamRoleByRunID: map[string]string{
			"run-b": "writer",
		},
	}
	cmd := m.ensureFocusedRunStillValid()
	if cmd == nil {
		t.Fatalf("expected lens refresh cmd when focused run disappears")
	}
	if m.focusedRunID != "" || m.focusedRunRole != "" {
		t.Fatalf("expected focused run to auto-clear, got run=%q role=%q", m.focusedRunID, m.focusedRunRole)
	}
}

func TestActivityItemTitle_FallbacksWhenTitleMissing(t *testing.T) {
	tests := []struct {
		name string
		act  Activity
		want string
	}{
		{
			name: "kind+path",
			act:  Activity{Kind: "fs_read", Path: "/tmp/a.txt"},
			want: "fs_read /tmp/a.txt",
		},
		{
			name: "from-to",
			act:  Activity{Kind: "copy", From: "/a", To: "/b"},
			want: "copy /a -> /b",
		},
		{
			name: "kind-only",
			act:  Activity{Kind: "shell_exec"},
			want: "shell_exec",
		},
		{
			name: "default",
			act:  Activity{},
			want: "op",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := activityItem{act: tt.act}.Title()
			if got != tt.want {
				t.Fatalf("title=%q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderComposer_ShowsReasoningTagsForReasoningModel(t *testing.T) {
	m := &monitorModel{
		model:            "openai/gpt-5-nano",
		profile:          "general",
		reasoningEffort:  "medium",
		reasoningSummary: "auto",
		styles:           defaultMonitorStyles(),
		input:            textarea.New(),
		focusedPanel:     panelComposer,
	}
	m.input.Prompt = ""
	spec := layoutmgr.PanelSpec{Width: 100, Height: 8, ContentWidth: 96, ContentHeight: 6}
	out := m.renderComposer(spec)
	if !strings.Contains(out, "reasoning-effort") || !strings.Contains(out, "medium") {
		t.Fatalf("expected effort tag in composer output: %q", out)
	}
	if !strings.Contains(out, "reasoning-summary") || !strings.Contains(out, "auto") {
		t.Fatalf("expected summary tag in composer output: %q", out)
	}
}

func TestRenderComposer_HidesReasoningTagsForNonReasoningModel(t *testing.T) {
	m := &monitorModel{
		model:            "moonshotai/kimi-k2.5",
		profile:          "general",
		reasoningEffort:  "high",
		reasoningSummary: "detailed",
		styles:           defaultMonitorStyles(),
		input:            textarea.New(),
		focusedPanel:     panelComposer,
	}
	spec := layoutmgr.PanelSpec{Width: 100, Height: 8, ContentWidth: 96, ContentHeight: 6}
	out := m.renderComposer(spec)
	if strings.Contains(out, "reasoning-effort") || strings.Contains(out, "reasoning-summary") {
		t.Fatalf("did not expect reasoning tags in composer output: %q", out)
	}
}

func TestRenderComposer_DoesNotShowPromptGlyph(t *testing.T) {
	m := &monitorModel{
		model:            "openai/gpt-5-nano",
		profile:          "general",
		reasoningEffort:  "medium",
		reasoningSummary: "auto",
		styles:           defaultMonitorStyles(),
		input:            textarea.New(),
		focusedPanel:     panelComposer,
	}
	m.input.SetValue("hello")
	m.input.Prompt = ""
	spec := layoutmgr.PanelSpec{Width: 80, Height: 8, ContentWidth: 76, ContentHeight: 6}
	out := m.renderComposer(spec)
	if strings.Contains(out, "\n>") || strings.Contains(out, "> ") {
		t.Fatalf("composer should not render prompt glyph: %q", out)
	}
}

func TestRenderComposer_PrioritizesReasoningSummaryInNarrowWidth(t *testing.T) {
	m := &monitorModel{
		model:            "openai/gpt-5-nano",
		profile:          "very_long_profile_name_that_will_not_fit",
		reasoningEffort:  "medium",
		reasoningSummary: "auto",
		styles:           defaultMonitorStyles(),
		input:            textarea.New(),
		focusedPanel:     panelComposer,
	}
	spec := layoutmgr.PanelSpec{Width: 52, Height: 8, ContentWidth: 48, ContentHeight: 6}
	out := m.renderComposer(spec)
	if !strings.Contains(out, "reasoning-summary") || !strings.Contains(out, "auto") {
		t.Fatalf("expected summary tag in narrow composer output: %q", out)
	}
}

func TestNormalizeThinkingSummary_SplitsGluedReasoningSections(t *testing.T) {
	in := "I should mention constraints in planning.Listing capabilities and process. I can do tasks.Understanding task creation"
	got := normalizeThinkingSummary(in)
	if !strings.Contains(got, "planning.\n\nListing capabilities") {
		t.Fatalf("expected first split, got: %q", got)
	}
	if !strings.Contains(got, "tasks.\n\nUnderstanding task creation") {
		t.Fatalf("expected second split, got: %q", got)
	}
}

func TestNormalizeThinkingSummary_SplitsMarkdownHeadingBoundary(t *testing.T) {
	in := "I can generate reports.**Detailing interaction methods** and workflows."
	got := normalizeThinkingSummary(in)
	if !strings.Contains(got, "reports.\n\n**Detailing interaction methods**") {
		t.Fatalf("expected markdown heading split, got: %q", got)
	}
}

func TestAppendThinkingEntry_NormalizesBeforeStore(t *testing.T) {
	m := &monitorModel{}
	m.appendThinkingEntry("run-1", "", "A.Beta section")
	if len(m.thinkingEntries) != 1 {
		t.Fatalf("entries = %d, want 1", len(m.thinkingEntries))
	}
	if !strings.Contains(m.thinkingEntries[0].Summary, "A.\n\nBeta section") {
		t.Fatalf("normalized summary = %q", m.thinkingEntries[0].Summary)
	}
}

func TestObserveEvent_DoesNotOverwriteReasoningSummaryFromTaskSummary(t *testing.T) {
	m := &monitorModel{
		reasoningSummary: "auto",
	}
	m.observeEvent(types.EventRecord{
		Type: "task.done",
		Data: map[string]string{
			"summary": "this is a task result summary, not reasoning settings",
		},
	})
	if got := strings.TrimSpace(m.reasoningSummary); got != "auto" {
		t.Fatalf("reasoningSummary overwritten = %q, want %q", got, "auto")
	}
	m.observeEvent(types.EventRecord{
		Type: "control.success",
		Data: map[string]string{
			"command": "set_reasoning",
			"summary": "detailed",
		},
	})
	if got := strings.TrimSpace(m.reasoningSummary); got != "detailed" {
		t.Fatalf("reasoningSummary = %q, want %q", got, "detailed")
	}
}

func TestRenderComposer_WrapsStatusTagsWithoutHardTruncation(t *testing.T) {
	m := &monitorModel{
		model:            "openai/gpt-5-nano",
		profile:          "market_researcher",
		reasoningEffort:  "medium",
		reasoningSummary: "auto",
		styles:           defaultMonitorStyles(),
		input:            textarea.New(),
		focusedPanel:     panelComposer,
	}
	spec := layoutmgr.PanelSpec{Width: 84, Height: 8, ContentWidth: 80, ContentHeight: 6}
	out := m.renderComposer(spec)
	if !strings.Contains(out, "profile") {
		t.Fatalf("expected profile tag in output: %q", out)
	}
	if strings.Contains(out, "pr…") {
		t.Fatalf("unexpected hard-truncated profile tag: %q", out)
	}
}

func TestRenderBottomBar_SingleLineAndHasCriticalInfo(t *testing.T) {
	m := &monitorModel{
		styles:         defaultMonitorStyles(),
		rpcHealthKnown: true,
		rpcReachable:   false,
	}
	m.stats.started = time.Now().Add(-2 * time.Minute)
	m.stats.tasksDone = 3
	m.stats.totalTokens = 42
	m.stats.totalTokensIn = 20
	m.stats.totalTokensOut = 22
	m.stats.totalCostUSD = 0.1234
	line := m.renderBottomBar(260)
	if lipgloss.Height(line) != 1 {
		t.Fatalf("bottom bar should be single line, got height=%d line=%q", lipgloss.Height(line), line)
	}
	if !strings.Contains(line, "tasks: 3") || !strings.Contains(line, "Tab: focus") {
		t.Fatalf("expected key metrics/controls in bottom bar: %q", line)
	}
	if strings.Contains(line, "daemon disconnected") {
		t.Fatalf("expected daemon disconnected alert to render in warning banner, not bottom bar: %q", line)
	}
}

func TestRenderOutboxLines_DoesNotRenderSummaryBody(t *testing.T) {
	out := renderOutboxLines([]outboxEntry{
		{
			TaskID:      "task-1",
			Goal:        "ship monitor UX improvements",
			Status:      "succeeded",
			Summary:     "Long markdown summary body that should not be shown in outbox anymore.",
			SummaryPath: "/workspace/deliverables/2026-02-12/task-1/SUMMARY.md",
		},
	}, newContentRenderer(), 80)
	if strings.Contains(out, "Long markdown summary body") {
		t.Fatalf("outbox should not include summary body, got: %q", out)
	}
	if !strings.Contains(out, "deliverables:") {
		t.Fatalf("outbox should retain deliverables metadata, got: %q", out)
	}
}

func TestRenderRightFooterPanel_ShowsSessionStats(t *testing.T) {
	m := &monitorModel{styles: defaultMonitorStyles()}
	m.stats.lastTurnTokens = 200
	m.stats.lastTurnTokensIn = 120
	m.stats.lastTurnTokensOut = 80
	m.stats.totalTokens = 900
	m.stats.totalTokensIn = 500
	m.stats.totalTokensOut = 400
	m.stats.lastTurnCostUSD = "0.0012"
	m.stats.totalCostUSD = 0.0123
	m.stats.pricingKnown = true
	panel := m.renderRightFooterPanel(48, 9)
	if !strings.Contains(panel, "Session Stats") || !strings.Contains(panel, "Last tokens: 200 (120 in + 80 out)") {
		t.Fatalf("expected session stats panel content, got: %q", panel)
	}
	if !strings.Contains(panel, "Total tokens: 900 (500 in + 400 out)") || !strings.Contains(panel, "Total cost: $0.0123") {
		t.Fatalf("expected total stats in panel, got: %q", panel)
	}
}

func TestHandleDataLoadMessages_TeamStatusUpdatesTokenBreakdown(t *testing.T) {
	m := &monitorModel{}
	model, _ := m.dispatchUpdate(teamStatusLoadedMsg{
		totalTokensIn:  321,
		totalTokensOut: 123,
		totalTokens:    444,
		totalCostUSD:   0.09,
		pricingKnown:   true,
	})
	updated, ok := model.(*monitorModel)
	if !ok {
		t.Fatalf("expected *monitorModel, got %T", model)
	}
	if updated.stats.totalTokensIn != 321 || updated.stats.totalTokensOut != 123 || updated.stats.totalTokens != 444 {
		t.Fatalf("unexpected team token stats: %+v", updated.stats)
	}
	if updated.stats.totalCostUSD != 0.09 || !updated.stats.pricingKnown {
		t.Fatalf("unexpected team cost/pricing stats: %+v", updated.stats)
	}
}

func TestComposerStatusText_NarrowWidthSingleLine(t *testing.T) {
	m := &monitorModel{
		model:            "openai/gpt-5-nano",
		profile:          "very_long_profile_name_that_will_not_fit",
		reasoningEffort:  "medium",
		reasoningSummary: "auto",
		styles:           defaultMonitorStyles(),
	}
	status, lines := m.composerStatusText(50)
	if lines != 1 {
		t.Fatalf("expected single status line for narrow width, got %d (%q)", lines, status)
	}
	if lipgloss.Width(status) > 50 {
		t.Fatalf("status line exceeds width: %q", status)
	}
}

func TestRenderMainBodyDashboard_AgentOutputBottomBorderVisible(t *testing.T) {
	m := &monitorModel{
		styles:           defaultMonitorStyles(),
		planViewport:     viewport.New(0, 0),
		agentOutputVP:    viewport.New(0, 0),
		dashboardSideTab: 1, // Plan tab keeps dependencies minimal for this render test.
	}
	m.agentOutputVP.SetContent("line 1\nline 2\n")
	manager := layoutmgr.NewManager(m.styles.panel, true)
	grid := manager.CalculateDashboard(120, 35, 6, 0, 1, false)
	main := m.renderMainBodyDashboard(grid)
	if !strings.Contains(main, "╰") || !strings.Contains(main, "╯") {
		t.Fatalf("expected rounded bottom border to be visible in main dashboard output: %q", main)
	}
}

func TestMonitorDispatch_KeyPrecedence_ModalConsumesGlobal(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	m, err := newDetachedMonitorModel(ctx, cfg, &MonitorResult{})
	if err != nil {
		t.Fatalf("newDetachedMonitorModel: %v", err)
	}
	m.helpModalOpen = true

	model, cmd := m.dispatchUpdate(tea.KeyMsg{Type: tea.KeyCtrlC})
	updated, ok := model.(*monitorModel)
	if !ok {
		t.Fatalf("expected *monitorModel, got %T", model)
	}
	if cmd != nil {
		t.Fatalf("expected help modal to consume ctrl+c before global quit")
	}
	if !updated.helpModalOpen {
		t.Fatalf("expected help modal to remain open")
	}
}

func TestMonitorDispatch_KeyPrecedence_ArtifactBeforeModal(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	m, err := newDetachedMonitorModel(ctx, cfg, &MonitorResult{})
	if err != nil {
		t.Fatalf("newDetachedMonitorModel: %v", err)
	}
	m.artifactViewerOpen = true
	m.helpModalOpen = true

	model, _ := m.dispatchUpdate(tea.KeyMsg{Type: tea.KeyEsc})
	updated, ok := model.(*monitorModel)
	if !ok {
		t.Fatalf("expected *monitorModel, got %T", model)
	}
	if updated.artifactViewerOpen {
		t.Fatalf("expected artifact viewer to close on esc")
	}
	if !updated.helpModalOpen {
		t.Fatalf("expected help modal untouched when artifact viewer handles key first")
	}
}

func TestMonitorDispatch_PaginationGatedByFocusedPanel(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	m, err := newDetachedMonitorModel(ctx, cfg, &MonitorResult{})
	if err != nil {
		t.Fatalf("newDetachedMonitorModel: %v", err)
	}
	m.width = 120
	m.height = 45
	m.focusedPanel = panelComposer
	m.updateFocus()
	beforeInboxPage := m.inboxPage
	beforeOutboxPage := m.outboxPage
	beforeActivityPage := m.activityPage

	model, _ := m.dispatchUpdate(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	updated := model.(*monitorModel)

	if updated.input.Value() != "n" {
		t.Fatalf("expected key to route to composer input, got %q", updated.input.Value())
	}
	if updated.inboxPage != beforeInboxPage || updated.outboxPage != beforeOutboxPage || updated.activityPage != beforeActivityPage {
		t.Fatalf("expected pagination state unchanged when non-paginated panel is focused")
	}
}

func TestMonitorDispatch_CompactTabNavigationParity(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	m, err := newMonitorModel(ctx, cfg, "compact-nav", &MonitorResult{})
	if err != nil {
		t.Fatalf("newMonitorModel: %v", err)
	}
	m.width = 100
	m.height = 30
	if !m.isCompactMode() {
		t.Fatalf("expected compact mode")
	}
	m.compactTab = 0
	m.focusedPanel = panelComposer

	model, _ := m.dispatchUpdate(tea.KeyMsg{Type: tea.KeyCtrlCloseBracket})
	updated := model.(*monitorModel)
	if updated.compactTab != 1 {
		t.Fatalf("compactTab=%d want 1", updated.compactTab)
	}
	if updated.focusedPanel != panelComposer {
		t.Fatalf("focusedPanel=%v want composer in compact mode when composer is focused", updated.focusedPanel)
	}
}

func TestMonitorDispatch_DashboardTabNavigationParity(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	m, err := newMonitorModel(ctx, cfg, "dashboard-nav", &MonitorResult{})
	if err != nil {
		t.Fatalf("newMonitorModel: %v", err)
	}
	m.width = 120
	m.height = 45
	if m.isCompactMode() {
		t.Fatalf("expected dashboard mode")
	}
	m.dashboardSideTab = 0

	model, _ := m.dispatchUpdate(tea.KeyMsg{Type: tea.KeyCtrlCloseBracket})
	updated := model.(*monitorModel)
	if updated.dashboardSideTab != 1 {
		t.Fatalf("dashboardSideTab=%d want 1", updated.dashboardSideTab)
	}
	if updated.focusedPanel != updated.dashboardSideTabToPanel() {
		t.Fatalf("focusedPanel=%v want %v", updated.focusedPanel, updated.dashboardSideTabToPanel())
	}
}

func TestHandleCommand_InstantEchoBeforeEnqueue(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	runID := "test-run-echo"
	startMonitorTestRPCServer(t, cfg, runID)

	m, err := newMonitorModel(ctx, cfg, runID, &MonitorResult{})
	if err != nil {
		t.Fatalf("newMonitorModel: %v", err)
	}

	// Before submitting, agent output buffer should be empty.
	if len(m.agentOutput) != 0 {
		t.Fatalf("agentOutput len=%d, want 0", len(m.agentOutput))
	}

	// handleCommand returns a tea.Cmd (async RPC); the echo should be in the
	// buffer *before* executing the returned command.
	cmd := m.handleCommand("fix the login bug")
	if cmd == nil {
		t.Fatalf("expected non-nil enqueue cmd")
	}

	// The instant echo must already be in the buffer, synchronously.
	found := false
	for _, line := range m.agentOutput {
		if strings.Contains(line.Content, "▸") && strings.Contains(line.Content, "fix the login bug") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected instant echo line in agentOutput, got %v", m.agentOutput)
	}

	// Slash commands should NOT produce an echo.
	before := len(m.agentOutput)
	_ = m.handleCommand("/help")
	after := len(m.agentOutput)
	for _, line := range m.agentOutput[before:after] {
		if strings.Contains(line.Content, "▸") {
			t.Fatalf("slash command should not produce echo: %q", line.Content)
		}
	}
}

func TestObserveEvent_AgentStatusLine(t *testing.T) {
	t.Parallel()

	m := &monitorModel{
		inbox: map[string]taskState{},
	}

	// task.start → "Thinking…" (agent immediately calls LLM)
	m.observeTaskEvent(types.EventRecord{
		Type:      "task.start",
		Timestamp: time.Now(),
		Data:      map[string]string{"taskId": "task-1", "goal": "test"},
	})
	if m.agentStatusLine != "Thinking…" {
		t.Fatalf("after task.start: status=%q, want %q", m.agentStatusLine, "Thinking…")
	}

	// agent.step → "Processing…" (thinking just finished, about to run tools)
	m.observeEvent(types.EventRecord{
		Type:      "agent.step",
		Timestamp: time.Now(),
		Data:      map[string]string{"step": "1"},
	})
	if m.agentStatusLine != "Processing…" {
		t.Fatalf("after agent.step: status=%q, want %q", m.agentStatusLine, "Processing…")
	}

	// agent.op.request → shows tool name
	m.observeEvent(types.EventRecord{
		Type:      "agent.op.request",
		Timestamp: time.Now(),
		Data:      map[string]string{"op": "shell_exec", "path": "/tmp/foo"},
	})
	if !strings.Contains(m.agentStatusLine, "shell_exec") {
		t.Fatalf("after agent.op.request: status=%q, want to contain %q", m.agentStatusLine, "shell_exec")
	}

	// agent.op.response → "Thinking…" (agent will call LLM again)
	m.observeEvent(types.EventRecord{
		Type:      "agent.op.response",
		Timestamp: time.Now(),
		Data:      map[string]string{"op": "shell_exec", "ok": "true"},
	})
	if m.agentStatusLine != "Thinking…" {
		t.Fatalf("after agent.op.response: status=%q, want %q", m.agentStatusLine, "Thinking…")
	}

	// task.done → "✓ Done"
	m.observeTaskEvent(types.EventRecord{
		Type:      "task.done",
		Timestamp: time.Now(),
		Data:      map[string]string{"taskId": "task-1", "status": "succeeded"},
	})
	if m.agentStatusLine != "✓ Done" {
		t.Fatalf("after task.done: status=%q, want %q", m.agentStatusLine, "✓ Done")
	}

	// agent.turn.complete → "Idle"
	m.observeEvent(types.EventRecord{
		Type:      "agent.turn.complete",
		Timestamp: time.Now(),
		Data:      map[string]string{},
	})
	if m.agentStatusLine != "Idle" {
		t.Fatalf("after agent.turn.complete: status=%q, want %q", m.agentStatusLine, "Idle")
	}

	// llm.retry → "Retrying…"
	m.observeEvent(types.EventRecord{
		Type:      "llm.retry",
		Timestamp: time.Now(),
		Data:      map[string]string{},
	})
	if m.agentStatusLine != "Retrying…" {
		t.Fatalf("after llm.retry: status=%q, want %q", m.agentStatusLine, "Retrying…")
	}

	// daemon.stop → "Stopped"
	m.observeEvent(types.EventRecord{
		Type:      "daemon.stop",
		Timestamp: time.Now(),
		Data:      map[string]string{},
	})
	if m.agentStatusLine != "Stopped" {
		t.Fatalf("after daemon.stop: status=%q, want %q", m.agentStatusLine, "Stopped")
	}

	// daemon.start → "Starting…"
	m.observeEvent(types.EventRecord{
		Type:      "daemon.start",
		Timestamp: time.Now(),
		Data:      map[string]string{},
	})
	if m.agentStatusLine != "Starting…" {
		t.Fatalf("after daemon.start: status=%q, want %q", m.agentStatusLine, "Starting…")
	}

	// daemon.error → "⚠ Daemon Error"
	m.observeEvent(types.EventRecord{
		Type:      "daemon.error",
		Timestamp: time.Now(),
		Data:      map[string]string{},
	})
	if m.agentStatusLine != "⚠ Daemon Error" {
		t.Fatalf("after daemon.error: status=%q, want %q", m.agentStatusLine, "⚠ Daemon Error")
	}

	// agent.error → "⚠ Error"
	m.observeEvent(types.EventRecord{
		Type:      "agent.error",
		Timestamp: time.Now(),
		Data:      map[string]string{},
	})
	if m.agentStatusLine != "⚠ Error" {
		t.Fatalf("after agent.error: status=%q, want %q", m.agentStatusLine, "⚠ Error")
	}
}

func TestMonitorRefreshViewports_ReservesPaginationFooterRow(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	m, err := newDetachedMonitorModel(ctx, cfg, &MonitorResult{})
	if err != nil {
		t.Fatalf("newDetachedMonitorModel: %v", err)
	}
	m.width = 120
	m.height = 45
	m.refreshViewports()
	baseOutboxH := m.outboxVP.Height
	baseInboxH := m.inboxVP.Height
	baseActivityH := m.activityList.Height()

	m.outboxPageSize = 5
	m.outboxTotalCount = 12
	m.inboxPageSize = 5
	m.inboxTotalCount = 11
	m.activityPageSize = 10
	m.activityTotalCount = 50
	m.refreshViewports()

	if m.outboxVP.Height != baseOutboxH-1 {
		t.Fatalf("outbox viewport height=%d want %d with footer row reserved", m.outboxVP.Height, baseOutboxH-1)
	}
	if m.inboxVP.Height != baseInboxH-1 {
		t.Fatalf("inbox viewport height=%d want %d with footer row reserved", m.inboxVP.Height, baseInboxH-1)
	}
	if m.activityList.Height() != baseActivityH-1 {
		t.Fatalf("activity viewport height=%d want %d with footer row reserved", m.activityList.Height(), baseActivityH-1)
	}
}

func TestAgentOutputScroll_GAndShiftG(t *testing.T) {
	m := &monitorModel{
		agentOutputVP:             viewport.New(80, 5),
		agentOutputTotalLines:     30,
		agentOutputLogicalYOffset: 10,
	}

	m.applyAgentOutputScroll(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if m.agentOutputLogicalYOffset != 0 {
		t.Fatalf("expected g to jump to top, got yOffset=%d", m.agentOutputLogicalYOffset)
	}
	if !isScrollKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}}) {
		t.Fatalf("expected g to be recognized as scroll key")
	}

	m.applyAgentOutputScroll(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	if m.agentOutputLogicalYOffset != 25 {
		t.Fatalf("expected G to jump to bottom yOffset=25, got %d", m.agentOutputLogicalYOffset)
	}
	if !isScrollKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}}) {
		t.Fatalf("expected G to be recognized as scroll key")
	}
}
