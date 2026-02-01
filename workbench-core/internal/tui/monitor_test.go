package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/workbench-core/pkg/config"
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

func TestMonitorView_NoClipping_DashboardMode(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	runID := "test-run-no-clip-dashboard"
	m, err := newMonitorModel(ctx, cfg, runID)
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
	m, err := newMonitorModel(ctx, cfg, runID)
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
