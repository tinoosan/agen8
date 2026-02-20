package app

import (
	"path/filepath"
	"testing"
)

func TestResolveArtifactDiskPath_StandaloneSubagentCanonicalPaths(t *testing.T) {
	dataDir := t.TempDir()
	runID := "run-parent-1"

	gotWorkspace := resolveArtifactDiskPath(dataDir, "", runID, "/workspace/subagent-2/hello.txt")
	wantWorkspace := filepath.Join(dataDir, "agents", runID, "workspace", "subagent-2", "hello.txt")
	if gotWorkspace != wantWorkspace {
		t.Fatalf("workspace path = %q, want %q", gotWorkspace, wantWorkspace)
	}

	gotTasks := resolveArtifactDiskPath(dataDir, "", runID, "/tasks/subagent-2/2026-02-17/task-1/SUMMARY.md")
	wantTasks := filepath.Join(dataDir, "agents", runID, "tasks", "subagent-2", "2026-02-17", "task-1", "SUMMARY.md")
	if gotTasks != wantTasks {
		t.Fatalf("tasks path = %q, want %q", gotTasks, wantTasks)
	}
}

func TestResolveArtifactDiskPath_TeamTasksStillResolveFromTeamRoot(t *testing.T) {
	dataDir := t.TempDir()
	got := resolveArtifactDiskPath(dataDir, "team-1", "run-ceo", "/tasks/ceo/2026-02-17/task-1/SUMMARY.md")
	want := filepath.Join(dataDir, "teams", "team-1", "tasks", "ceo", "2026-02-17", "task-1", "SUMMARY.md")
	if got != want {
		t.Fatalf("team tasks path = %q, want %q", got, want)
	}
}
