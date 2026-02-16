package session

import (
	"testing"
	"time"
)

func TestTasksBase_WritesToRunLevelTasks(t *testing.T) {
	when := time.Date(2026, 2, 12, 12, 0, 0, 0, time.UTC)
	got := tasksBase(when, "task-1")
	want := "/tasks/2026-02-12/task-1"
	if got != want {
		t.Fatalf("tasksBase() = %q, want %q", got, want)
	}
}

func TestSanitizeArtifactPaths_ExcludesPlanFiles(t *testing.T) {
	in := []string{
		"/plan/HEAD.md",
		"/workspace/plan/CHECKLIST.md",
		"/tasks/2026-02-12/task-1/SUMMARY.md",
		"/workspace/researcher/report.md",
	}
	got := sanitizeArtifactPaths(in)
	if len(got) != 2 {
		t.Fatalf("len(sanitizeArtifactPaths)=%d, want 2 (%+v)", len(got), got)
	}
	if got[0] != "/tasks/2026-02-12/task-1/SUMMARY.md" {
		t.Fatalf("unexpected first artifact %q", got[0])
	}
	if got[1] != "/workspace/researcher/report.md" {
		t.Fatalf("unexpected second artifact %q", got[1])
	}
}
