package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	implstore "github.com/tinoosan/workbench-core/internal/store"
	layoutmgr "github.com/tinoosan/workbench-core/internal/tui/layout"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/types"
)

func TestRenderDashboardSubagentsTab(t *testing.T) {
	now := time.Now()
	later := now.Add(5 * time.Minute)

	tests := []struct {
		name      string
		childRuns []types.Run
		wantLines []string
		wantEmpty bool
	}{
		{
			name:      "Empty",
			childRuns: []types.Run{},
			wantLines: []string{"No subagents spawned yet."},
			wantEmpty: false, // The panel title is always rendered
		},
		{
			name: "Single Run Running",
			childRuns: []types.Run{
				{
					RunID:      "run-1",
					Status:     types.RunStatusRunning,
					Goal:       "Research agent memory",
					StartedAt:  &now,
					SpawnIndex: 1,
				},
			},
			wantLines: []string{
				"Sub-agent 1",
				"Research agent memory",
			},
		},
		{
			name: "Multiple Runs Mixed Status - only active shown",
			childRuns: []types.Run{
				{
					RunID:      "run-1",
					Status:     types.RunStatusSucceeded,
					Goal:       "Fetch data",
					StartedAt:  &now,
					FinishedAt: &later,
					SpawnIndex: 1,
					CostUSD:    0.05,
				},
				{
					RunID:      "run-2",
					Status:     types.RunStatusFailed,
					Goal:       "Process data with strict failure handling which is long...",
					StartedAt:  &now,
					FinishedAt: &later,
					SpawnIndex: 2,
				},
			},
			// Tab shows only running subagents; completed ones are summarized.
			wantLines: []string{"No active subagents.", "2 completed"},
		},
		{
			name:      "Viewing child run shows Back to parent",
			childRuns: []types.Run{},
			wantLines: []string{"Back to parent"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Config{DataDir: t.TempDir()}
			runID := ""
			if tt.name == "Viewing child run shows Back to parent" {
				_, parentRun, err := implstore.CreateSession(cfg, "parent", 8*1024)
				if err != nil {
					t.Fatalf("CreateSession: %v", err)
				}
				childRun := types.NewChildRun(parentRun.RunID, "child goal", parentRun.SessionID, 1)
				if err := implstore.SaveRun(cfg, childRun); err != nil {
					t.Fatalf("SaveRun child: %v", err)
				}
				runID = childRun.RunID
			}
			m := &monitorModel{
				ctx:           context.Background(),
				cfg:           cfg,
				runID:         runID,
				childRuns:     tt.childRuns,
				subagentsVP:   viewport.New(0, 0),
				subagentsList: newSubagentsList(),
				styles:        defaultMonitorStyles(),
			}
			grid := layoutmgr.GridLayout{
				Plan: layoutmgr.PanelSpec{
					Width:  80,
					Height: 20,
				},
			}

			// Render
			got := renderDashboardSubagentsTab(m, grid)

			// Simple verification: check if rendered output contains key substrings
			// Since lipgloss adds ANSI codes, exact string match is hard without stripping.
			// We'll check for content presence.

			for _, want := range tt.wantLines {
				if !strings.Contains(got, want) {
					t.Errorf("Output missing expected substring %q", want)
				}
			}

			// Better: check the viewport content which is set inside the function
			// Wait, the function modifies m.subagentsVP content!
			// BUT m.subagentsVP.SetContent(content) sets the model content.
			// We can verify m.subagentsVP.View() or the content string passed to it.

			// We can inspect m.subagentsVP model directly? No, it's opaque.
			// However, renderDashboardSubagentsTab returns the final rendered string.

			// Let's just verifying it runs without panic and returns non-empty string.
			if got == "" {
				t.Errorf("renderDashboardSubagentsTab() returned empty string")
			}

			// Check that goal text is present for runs that are shown (active only)
			for _, run := range tt.childRuns {
				if strings.EqualFold(strings.TrimSpace(run.Status), types.RunStatusRunning) {
					if !strings.Contains(got, run.Goal) && len(run.Goal) < 60 {
						t.Errorf("Output missing goal for active run: %q", run.Goal)
					}
				}
			}
		})
	}
}

func TestRenderOpRequest_TaskCreate(t *testing.T) {
	tests := []struct {
		name string
		data map[string]string
		want string
	}{
		{
			name: "Normal Task Create",
			data: map[string]string{
				"op":  "noop",
				"tag": "task_create",
			},
			want: "Create task",
		},
		{
			name: "Other Tag",
			data: map[string]string{
				"op":  "noop",
				"tag": "other",
			},
			want: "op=noop tag=other", // fallback behavior
		},
		{
			name: "No Tag",
			data: map[string]string{
				"op": "noop",
			},
			want: "op=noop", // fallback behavior
		},
		{
			name: "Reclassified op=task_create",
			data: map[string]string{
				"op": "task_create",
			},
			want: "Create task",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderOpRequest(tt.data)
			if got != tt.want {
				t.Errorf("renderOpRequest() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderOpResponse_TaskCreate(t *testing.T) {
	tests := []struct {
		name string
		data map[string]string
		want string
	}{
		{
			name: "Success",
			data: map[string]string{
				"op":   "noop",
				"tag":  "task_create",
				"ok":   "true",
				"text": "Task created successfully",
			},
			want: "✓ Task created successfully",
		},
		{
			name: "Failure with Error",
			data: map[string]string{
				"op":  "noop",
				"tag": "task_create",
				"ok":  "false",
				"err": "failed to create",
			},
			want: "✗ failed to create",
		},
		{
			name: "Failure Generic",
			data: map[string]string{
				"op":  "noop",
				"tag": "task_create",
				"ok":  "false",
			},
			want: "✗ task creation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderOpResponse(tt.data)
			if got != tt.want {
				t.Errorf("renderOpResponse() = %q, want %q", got, tt.want)
			}
		})
	}
}
