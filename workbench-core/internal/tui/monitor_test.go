package tui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
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

func TestMonitorHandleCommand_EnqueuesTasksForNonCommands(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	runID := "test-run-handle-command"

	m, err := newMonitorModel(ctx, cfg, runID, &MonitorResult{})
	if err != nil {
		t.Fatalf("newMonitorModel: %v", err)
	}

	runDir := fsutil.GetAgentDir(cfg.DataDir, runID)
	inboxDir := filepath.Join(runDir, "inbox")

	assertQueued := func(input string, wantGoal string) {
		t.Helper()
		cmd := m.handleCommand(input)
		if cmd == nil {
			t.Fatalf("handleCommand(%q) returned nil; want enqueue cmd", input)
		}
		_ = cmd()

		ents, err := os.ReadDir(inboxDir)
		if err != nil {
			t.Fatalf("ReadDir(inbox): %v", err)
		}
		if len(ents) == 0 {
			t.Fatalf("expected at least 1 task file in inbox")
		}
		// Read the most recently created task file (best-effort).
		var newest string
		var newestInfo os.FileInfo
		for _, e := range ents {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			fi, err := e.Info()
			if err != nil {
				continue
			}
			if newestInfo == nil || fi.ModTime().After(newestInfo.ModTime()) {
				newestInfo = fi
				newest = e.Name()
			}
		}
		if newest == "" {
			t.Fatalf("expected a .json task file in inbox")
		}
		b, err := os.ReadFile(filepath.Join(inboxDir, newest))
		if err != nil {
			t.Fatalf("ReadFile(task): %v", err)
		}
		var task types.Task
		if err := json.Unmarshal(b, &task); err != nil {
			t.Fatalf("Unmarshal(task): %v", err)
		}
		if task.Goal != wantGoal {
			t.Fatalf("task goal = %q, want %q", task.Goal, wantGoal)
		}
	}

	// Plain text should enqueue as task.
	assertQueued("hello world", "hello world")
	// Unknown slash commands should also enqueue as task.
	assertQueued("/not-a-command hello", "/not-a-command hello")
}

func TestMonitorModelPicker_FilteringWorks(t *testing.T) {
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
	if len(m.modelPickerList.Items()) == 0 {
		t.Fatalf("expected model picker items")
	}
	before := len(m.modelPickerList.VisibleItems())

	for _, r := range []rune("anthropic") {
		_, _ = m.updateModelPicker(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	if got := m.modelPickerList.FilterValue(); got != "anthropic" {
		t.Fatalf("expected filter value %q, got %q", "anthropic", got)
	}
	after := m.modelPickerList.VisibleItems()
	if len(after) == 0 {
		t.Fatalf("expected visible items after filtering")
	}
	if len(after) > before {
		t.Fatalf("expected visible items <= before, before=%d after=%d", before, len(after))
	}
	for _, it := range after {
		mi, ok := it.(monitorModelPickerItem)
		if !ok {
			t.Fatalf("expected monitorModelPickerItem, got %T", it)
		}
		if !strings.Contains(strings.ToLower(mi.id), "anthropic") {
			t.Fatalf("expected filtered item to contain %q, got %q", "anthropic", mi.id)
		}
	}
}

func TestMonitorProfilePicker_FilterAndSelectWritesControl(t *testing.T) {
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

	runID := "test-run-profile-filter"
	m, err := newMonitorModel(ctx, cfg, runID, &MonitorResult{})
	if err != nil {
		t.Fatalf("newMonitorModel: %v", err)
	}
	m.width = 120
	m.height = 40

	_ = m.openProfilePicker()
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
	if got := m.profilePickerList.FilterValue(); got != "software" {
		t.Fatalf("expected filter value %q, got %q", "software", got)
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
	_ = cmd() // write control file

	inboxDir := filepath.Join(fsutil.GetAgentDir(cfg.DataDir, runID), "inbox")
	matches, err := filepath.Glob(filepath.Join(inboxDir, "control-*.json"))
	if err != nil {
		t.Fatalf("glob inbox: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("expected control file in inbox, got none")
	}
	b, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read control file: %v", err)
	}
	got := string(b)
	if !strings.Contains(got, "\"command\": \"switch_profile\"") {
		t.Fatalf("expected switch_profile command, got %s", got)
	}
	if !strings.Contains(got, "\"profile\": \"software_dev\"") {
		t.Fatalf("expected profile software_dev, got %s", got)
	}
	if m.profile != "software_dev" {
		t.Fatalf("expected monitor profile %q, got %q", "software_dev", m.profile)
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
	m.runStatus = types.StatusRunning
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
