package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinoosan/agen8/pkg/fsutil"
	"github.com/tinoosan/agen8/pkg/types"
)

func TestDiskPlanReader_ReadPlan(t *testing.T) {
	dataDir := t.TempDir()
	run := types.Run{RunID: "run-1", SessionID: "sess-1"}
	runDir := fsutil.GetRunDir(dataDir, run)
	planDir := filepath.Join(runDir, "plan")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatalf("mkdir plan: %v", err)
	}
	if err := os.WriteFile(filepath.Join(planDir, "HEAD.md"), []byte("# Plan details\n\nGoal: build X"), 0o644); err != nil {
		t.Fatalf("write HEAD.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(planDir, "CHECKLIST.md"), []byte("- [ ] Step 1\n- [ ] Step 2"), 0o644); err != nil {
		t.Fatalf("write CHECKLIST.md: %v", err)
	}

	r := NewDiskPlanReader(dataDir)
	checklist, details, checklistErr, detailsErr := r.ReadPlan(context.Background(), run)
	if checklistErr != "" || detailsErr != "" {
		t.Fatalf("unexpected errors: checklistErr=%q detailsErr=%q", checklistErr, detailsErr)
	}
	if !strings.Contains(details, "Plan details") {
		t.Errorf("details = %q, want to contain 'Plan details'", details)
	}
	if !strings.Contains(checklist, "Step 1") {
		t.Errorf("checklist = %q, want to contain 'Step 1'", checklist)
	}
}

func TestDiskPlanReader_ReadPlan_MissingFiles(t *testing.T) {
	dataDir := t.TempDir()
	run := types.Run{RunID: "run-empty"}
	r := NewDiskPlanReader(dataDir)
	checklist, details, checklistErr, detailsErr := r.ReadPlan(context.Background(), run)
	if checklistErr != "" || detailsErr != "" {
		t.Fatalf("missing files should return empty, not errors: checklistErr=%q detailsErr=%q", checklistErr, detailsErr)
	}
	if checklist != "" || details != "" {
		t.Errorf("missing files: checklist=%q details=%q, want empty", checklist, details)
	}
}
