package store

import (
	"context"
	"testing"
	"time"

	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/types"
)

func TestActivities_UpsertFromAgentOpEvents(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{DataDir: tmpDir}

	_, run, err := CreateSession(cfg, "Activity Test Run", 1024)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	start := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	if err := AppendEvent(context.Background(), cfg, types.EventRecord{
		RunID:     run.RunID,
		Timestamp: start,
		Type:      "agent.op.request",
		Message:   "Agent requested host op",
		Data: map[string]string{
			"opId":       "op-1",
			"op":         "fs_read",
			"path":       "/project/main.go",
			"maxBytes":   "1234",
			"textIsJSON": "false",
		},
	}); err != nil {
		t.Fatalf("AppendEvent request: %v", err)
	}

	finish := start.Add(150 * time.Millisecond)
	if err := AppendEvent(context.Background(), cfg, types.EventRecord{
		RunID:     run.RunID,
		Timestamp: finish,
		Type:      "agent.op.response",
		Message:   "Host op completed",
		Data: map[string]string{
			"opId":     "op-1",
			"op":       "fs_read",
			"ok":       "true",
			"bytesLen": "12",
		},
	}); err != nil {
		t.Fatalf("AppendEvent response: %v", err)
	}

	total, err := CountActivities(context.Background(), cfg, run.RunID)
	if err != nil {
		t.Fatalf("CountActivities: %v", err)
	}
	if total != 1 {
		t.Fatalf("total=%d want 1", total)
	}

	acts, err := ListActivities(context.Background(), cfg, run.RunID, 10, 0)
	if err != nil {
		t.Fatalf("ListActivities: %v", err)
	}
	if len(acts) != 1 {
		t.Fatalf("len(acts)=%d want 1", len(acts))
	}
	a := acts[0]
	if a.ID != "op-1" {
		t.Fatalf("id=%q want %q", a.ID, "op-1")
	}
	if a.Kind != "fs_read" {
		t.Fatalf("kind=%q want %q", a.Kind, "fs_read")
	}
	if a.Status != types.ActivityOK {
		t.Fatalf("status=%q want %q", a.Status, types.ActivityOK)
	}
	if a.Path != "/project/main.go" {
		t.Fatalf("path=%q want %q", a.Path, "/project/main.go")
	}
	if a.FinishedAt == nil || a.FinishedAt.IsZero() {
		t.Fatalf("expected finishedAt to be set")
	}
	if a.Duration <= 0 {
		t.Fatalf("expected duration > 0, got %v", a.Duration)
	}
}
