package hosttools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/workbench-core/pkg/agent/state"
	"github.com/tinoosan/workbench-core/pkg/types"
)

type fakeTaskStore struct {
	tasks map[string]types.Task
}

func newFakeTaskStore() *fakeTaskStore {
	return &fakeTaskStore{tasks: map[string]types.Task{}}
}

func (f *fakeTaskStore) GetTask(_ context.Context, taskID string) (types.Task, error) {
	if t, ok := f.tasks[taskID]; ok {
		return t, nil
	}
	return types.Task{}, state.ErrTaskNotFound
}

func (f *fakeTaskStore) GetRunStats(context.Context, string) (state.RunStats, error) {
	return state.RunStats{}, nil
}

func (f *fakeTaskStore) ListTasks(context.Context, state.TaskFilter) ([]types.Task, error) {
	return nil, nil
}

func (f *fakeTaskStore) CountTasks(context.Context, state.TaskFilter) (int, error) {
	return 0, nil
}

func (f *fakeTaskStore) CreateTask(_ context.Context, task types.Task) error {
	f.tasks[task.TaskID] = task
	return nil
}

func (f *fakeTaskStore) DeleteTask(_ context.Context, taskID string) error {
	delete(f.tasks, taskID)
	return nil
}

func (f *fakeTaskStore) UpdateTask(context.Context, types.Task) error {
	return nil
}

func (f *fakeTaskStore) CompleteTask(context.Context, string, types.TaskResult) error {
	return nil
}

func (f *fakeTaskStore) ClaimTask(context.Context, string, time.Duration) error {
	return nil
}

func (f *fakeTaskStore) ExtendLease(context.Context, string, time.Duration) error {
	return nil
}

func (f *fakeTaskStore) ReleaseLease(context.Context, string) error {
	return nil
}

func (f *fakeTaskStore) DelegateTask(context.Context, string) error {
	return nil
}

func (f *fakeTaskStore) ResumeTask(context.Context, string) error {
	return nil
}

func (f *fakeTaskStore) RecoverExpiredLeases(context.Context) error {
	return nil
}

func TestTaskCreateTool_CoordinatorCanAssignAnyRole(t *testing.T) {
	store := newFakeTaskStore()
	tool := &TaskCreateTool{
		Store:           store,
		SessionID:       "session-1",
		RunID:           "run-1",
		TeamID:          "team-1",
		RoleName:        "head-analyst",
		IsCoordinator:   true,
		CoordinatorRole: "head-analyst",
		ValidRoles:      []string{"head-analyst", "researcher", "report-writer"},
	}
	args := map[string]any{
		"goal":         "Research semiconductors",
		"taskId":       "task-coord-1",
		"assignedRole": "researcher",
	}
	raw, _ := json.Marshal(args)
	req, err := tool.Execute(context.Background(), raw)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if req.Op == types.HostOpFSWrite {
		t.Fatalf("task_create should not write inbox files")
	}
	if req.Op != types.HostOpToolResult {
		t.Fatalf("req.Op=%q, want %q", req.Op, types.HostOpToolResult)
	}
	task := store.tasks["task-coord-1"]
	if task.AssignedRole != "researcher" {
		t.Fatalf("expected assignedRole researcher, got %q", task.AssignedRole)
	}
	if task.CreatedBy != "head-analyst" {
		t.Fatalf("expected createdBy head-analyst, got %q", task.CreatedBy)
	}
}

func TestTaskCreateTool_WorkerCannotAssignOtherWorker(t *testing.T) {
	store := newFakeTaskStore()
	tool := &TaskCreateTool{
		Store:           store,
		SessionID:       "session-2",
		RunID:           "run-2",
		TeamID:          "team-1",
		RoleName:        "researcher",
		IsCoordinator:   false,
		CoordinatorRole: "head-analyst",
		ValidRoles:      []string{"head-analyst", "researcher", "report-writer"},
	}
	args := map[string]any{
		"goal":         "Write final report",
		"taskId":       "task-worker-1",
		"assignedRole": "report-writer",
	}
	raw, _ := json.Marshal(args)
	if _, err := tool.Execute(context.Background(), raw); err == nil {
		t.Fatalf("expected permission error")
	}
}

func TestTaskCreateTool_Definition_TeamOnly_NoSpawnWorker(t *testing.T) {
	tool := &TaskCreateTool{
		Store:     newFakeTaskStore(),
		SessionID: "s",
		RunID:     "r",
		// SpawnWorker is nil (team mode)
	}
	def := tool.Definition()
	if def.Type != "function" || def.Function.Name != "task_create" {
		t.Fatalf("unexpected tool: type=%q name=%q", def.Type, def.Function.Name)
	}
	params, _ := def.Function.Parameters.(map[string]any)
	if params == nil {
		t.Fatal("parameters is nil")
	}
	props, _ := params["properties"].(map[string]any)
	if props == nil {
		t.Fatal("properties is nil")
	}
	if _, has := props["spawnWorker"]; has {
		t.Error("Definition() when SpawnWorker is nil must not include spawnWorker in parameters")
	}
	// Team-only description should mention assignedRole, not spawn_worker/task_review
	desc := def.Function.Description
	if len(desc) == 0 {
		t.Error("description should be non-empty")
	}
	if strings.Contains(desc, "spawn_worker") || strings.Contains(desc, "task_review") || strings.Contains(desc, "subagent") {
		t.Errorf("team-only description must not mention spawn_worker/task_review/subagent: %q", desc)
	}
}

func TestTaskCreateTool_Definition_WithSpawnWorker_IncludesSpawnWorker(t *testing.T) {
	tool := &TaskCreateTool{
		Store:       newFakeTaskStore(),
		SessionID:   "s",
		RunID:       "r",
		SpawnWorker: func(context.Context, string, string, string) (string, error) { return "child", nil },
	}
	def := tool.Definition()
	params, _ := def.Function.Parameters.(map[string]any)
	if params == nil {
		t.Fatal("parameters is nil")
	}
	props, _ := params["properties"].(map[string]any)
	if props == nil {
		t.Fatal("properties is nil")
	}
	if _, has := props["spawnWorker"]; !has {
		t.Error("Definition() when SpawnWorker is set must include spawnWorker in parameters")
	}
	rawSpawn, ok := props["spawnWorker"].(map[string]any)
	if !ok {
		t.Fatalf("spawnWorker schema should be an object")
	}
	desc, _ := rawSpawn["description"].(string)
	if strings.Contains(desc, "/deliverables") {
		t.Fatalf("spawnWorker description must not reference /deliverables: %q", desc)
	}
	if !strings.Contains(desc, "/workspace") {
		t.Fatalf("spawnWorker description should reference /workspace: %q", desc)
	}
}

func TestTaskCreateTool_WorkerCanEscalateToCoordinator(t *testing.T) {
	store := newFakeTaskStore()
	tool := &TaskCreateTool{
		Store:           store,
		SessionID:       "session-3",
		RunID:           "run-3",
		TeamID:          "team-1",
		RoleName:        "researcher",
		IsCoordinator:   false,
		CoordinatorRole: "head-analyst",
		ValidRoles:      []string{"head-analyst", "researcher", "report-writer"},
	}
	args := map[string]any{
		"goal":         "Need clarification on region scope",
		"taskId":       "task-worker-2",
		"assignedRole": "head-analyst",
	}
	raw, _ := json.Marshal(args)
	req, err := tool.Execute(context.Background(), raw)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if req.Op == types.HostOpFSWrite {
		t.Fatalf("task_create should not write inbox files")
	}
	if req.Op != types.HostOpToolResult {
		t.Fatalf("req.Op=%q, want %q", req.Op, types.HostOpToolResult)
	}
	task := store.tasks["task-worker-2"]
	if task.AssignedRole != "head-analyst" {
		t.Fatalf("expected assignedRole head-analyst, got %q", task.AssignedRole)
	}
}

func TestTaskCreateTool_SpawnWorkerMessage_UsesCanonicalSubagentPaths(t *testing.T) {
	store := newFakeTaskStore()
	tool := &TaskCreateTool{
		Store:     store,
		SessionID: "s",
		RunID:     "parent-run",
		SpawnWorker: func(context.Context, string, string, string) (string, error) {
			return "child-run", nil
		},
	}
	raw, _ := json.Marshal(map[string]any{
		"goal":        "create a hello file",
		"taskId":      "task-1",
		"spawnWorker": true,
	})
	req, err := tool.Execute(context.Background(), raw)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if strings.Contains(req.Text, "/deliverables") {
		t.Fatalf("spawn_worker result message should not reference /deliverables: %q", req.Text)
	}
	if !strings.Contains(req.Text, "/workspace/subagent-<N>/") {
		t.Fatalf("spawn_worker result message should reference /workspace/subagent-<N>/: %q", req.Text)
	}
	if !strings.Contains(req.Text, "/tasks/subagent-<N>/<date>/<taskID>/SUMMARY.md") {
		t.Fatalf("spawn_worker result message should reference canonical task summary path: %q", req.Text)
	}
}
