package session

import (
	"context"
	"testing"
	"time"

	"github.com/tinoosan/agen8/pkg/agent"
	llmtypes "github.com/tinoosan/agen8/pkg/llm/types"
	"github.com/tinoosan/agen8/pkg/types"
)

func TestTasksBase_WritesToRunLevelTasks(t *testing.T) {
	when := time.Date(2026, 2, 12, 12, 0, 0, 0, time.UTC)
	got := tasksBase("", "", when, "task-1")
	want := "/tasks/2026-02-12/task-1"
	if got != want {
		t.Fatalf("tasksBase() = %q, want %q", got, want)
	}
}

func TestTasksBase_WritesToTeamRoleTasks(t *testing.T) {
	when := time.Date(2026, 2, 12, 12, 0, 0, 0, time.UTC)
	got := tasksBase("team-1", "backend-engineer", when, "task-1")
	want := "/tasks/backend-engineer/2026-02-12/task-1"
	if got != want {
		t.Fatalf("tasksBase() = %q, want %q", got, want)
	}
}

// mockRunner implements agent.Runner for artifact filtering tests.
type mockRunner struct {
	existingPaths map[string]bool
}

func (m *mockRunner) Run(context.Context, string) (agent.RunResult, error) {
	return agent.RunResult{}, nil
}

func (m *mockRunner) RunConversation(context.Context, []llmtypes.LLMMessage) (agent.RunResult, []llmtypes.LLMMessage, int, error) {
	return agent.RunResult{}, nil, 0, nil
}

func (m *mockRunner) ExecHostOp(_ context.Context, req types.HostOpRequest) types.HostOpResponse {
	if req.Op == types.HostOpFSStat {
		if m.existingPaths[req.Path] {
			return types.HostOpResponse{Ok: true}
		}
		return types.HostOpResponse{Ok: false, Error: "not found"}
	}
	return types.HostOpResponse{Ok: true}
}

func TestFilterExistingArtifacts_DropsNonExistent(t *testing.T) {
	runner := &mockRunner{existingPaths: map[string]bool{
		"/workspace/report.md":     true,
		"/tasks/task-1/SUMMARY.md": true,
	}}
	in := []string{
		"/workspace/report.md",
		"/workspace/phantom.md",
		"/tasks/task-1/SUMMARY.md",
	}
	got := filterExistingArtifacts(context.Background(), runner, in, t.Logf)
	if len(got) != 2 {
		t.Fatalf("len(filterExistingArtifacts)=%d, want 2 (%+v)", len(got), got)
	}
	if got[0] != "/workspace/report.md" {
		t.Fatalf("unexpected first artifact %q", got[0])
	}
	if got[1] != "/tasks/task-1/SUMMARY.md" {
		t.Fatalf("unexpected second artifact %q", got[1])
	}
}

func TestFilterExistingArtifacts_NilRunner(t *testing.T) {
	in := []string{"/workspace/report.md"}
	got := filterExistingArtifacts(context.Background(), nil, in, nil)
	if len(got) != 1 || got[0] != "/workspace/report.md" {
		t.Fatalf("expected passthrough with nil runner, got %+v", got)
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
