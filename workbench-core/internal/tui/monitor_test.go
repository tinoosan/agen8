package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	implstore "github.com/tinoosan/workbench-core/internal/store"
	agentstate "github.com/tinoosan/workbench-core/pkg/agent/state"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/protocol"
	"github.com/tinoosan/workbench-core/pkg/types"
)

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

func TestMonitorModelPicker_ProviderAndScopedFilteringWorks(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	runID := "test-run-model-filter"
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
	writeProfile := func(dir, id, desc string) {
		t.Helper()
		raw := "id: " + id + "\ndescription: " + desc + "\nprompts:\n  system_prompt: hello\n"
		if err := os.WriteFile(filepath.Join(dir, "profile.yaml"), []byte(raw), 0o644); err != nil {
			t.Fatalf("write profile.yaml: %v", err)
		}
	}
	writeProfile(filepath.Join(profilesDir, "general"), "general", "General profile")
	writeProfile(filepath.Join(profilesDir, "software_dev"), "software_dev", "Software development")
	writeProfile(filepath.Join(profilesDir, "stock_analyst"), "stock_analyst", "Stocks and markets")

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
		raw := "id: " + id + "\ndescription: " + desc + "\nprompts:\n  system_prompt: hello\n"
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
	_, run, err := implstore.CreateSession(cfg, "before", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	m, err := newMonitorModel(ctx, cfg, run.RunID, &MonitorResult{})
	if err != nil {
		t.Fatalf("newMonitorModel: %v", err)
	}
	cmd := m.handleCommand("/rename-session after rename")
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
	for _, cmd := range monitorAvailableCommands {
		if cmd == "/profile" {
			t.Fatalf("/profile should not be present in command palette")
		}
	}
	if monitorCommandInvokesWithoutArgs("/profile") {
		t.Fatalf("/profile should not be invokable")
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

func TestLoadPlanFilesCmd_TeamFocusedLoadsSingleRunPlan(t *testing.T) {
	dataDir := t.TempDir()
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
		cfg:          config.Config{DataDir: dataDir},
		teamID:       "team-a",
		focusedRunID: "run-a",
		teamRunIDs:   []string{"run-a", "run-b"},
		teamRoleByRunID: map[string]string{
			"run-a": "researcher",
			"run-b": "writer",
		},
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

func TestAgentOutputFocusFilter_ShowsGlobalAndFocusedRunOnly(t *testing.T) {
	m := &monitorModel{
		teamID:           "team-a",
		focusedRunID:     "run-a",
		agentOutput:      []string{"unscoped line", "focused line", "other line"},
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

func TestTrimAgentOutputBuffer_KeepsRunIDSliceInSync(t *testing.T) {
	size := agentOutputMaxLines + 15
	m := &monitorModel{
		agentOutput:              make([]string, size),
		agentOutputRunID:         make([]string, size),
		agentOutputFilteredCache: []string{"cached"},
	}
	for i := 0; i < size; i++ {
		m.agentOutput[i] = "line"
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
