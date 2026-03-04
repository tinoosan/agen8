package hosttools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/agen8/pkg/agent/state"
	"github.com/tinoosan/agen8/pkg/types"
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

func TestTaskCreateTool_CoordinatorCannotAssignDedicatedReviewer(t *testing.T) {
	store := newFakeTaskStore()
	tool := &TaskCreateTool{
		Store:           store,
		SessionID:       "session-1",
		RunID:           "run-1",
		TeamID:          "team-1",
		RoleName:        "ceo",
		IsCoordinator:   true,
		CoordinatorRole: "ceo",
		ReviewerRole:    "reviewer",
		ReviewerOnly:    true,
		ValidRoles:      []string{"ceo", "cto", "reviewer"},
	}
	raw, _ := json.Marshal(map[string]any{
		"goal":         "delegate build work",
		"taskId":       "task-coord-reviewer-blocked",
		"assignedRole": "reviewer",
	})
	if _, err := tool.Execute(context.Background(), raw); err == nil {
		t.Fatalf("expected reviewer-only assignment error")
	}
}

func TestTaskCreateTool_CoordinatorMustSpecifyAssignedRole(t *testing.T) {
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
		"goal":   "Research semiconductors",
		"taskId": "task-coord-no-role",
	}
	raw, _ := json.Marshal(args)
	if _, err := tool.Execute(context.Background(), raw); err == nil {
		t.Fatalf("expected error when coordinator omits assignedRole")
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

func TestTaskCreateTool_Definition_CoordinatorRequiresAssignedRoleInDescription(t *testing.T) {
	tool := &TaskCreateTool{
		Store:           newFakeTaskStore(),
		SessionID:       "s",
		RunID:           "r",
		TeamID:          "team-1",
		RoleName:        "ceo",
		IsCoordinator:   true,
		CoordinatorRole: "ceo",
		ValidRoles:      []string{"ceo", "cto"},
	}
	def := tool.Definition()
	if !strings.Contains(def.Function.Description, "must delegate by creating tasks with an explicit assignedRole") {
		t.Fatalf("expected coordinator description to require assignedRole, got: %q", def.Function.Description)
	}
	params, _ := def.Function.Parameters.(map[string]any)
	props, _ := params["properties"].(map[string]any)
	assigned, _ := props["assignedRole"].(map[string]any)
	desc, _ := assigned["description"].(string)
	if !strings.Contains(desc, "Required in team mode for coordinators") {
		t.Fatalf("expected coordinator assignedRole description, got: %q", desc)
	}
}

func TestTaskCreateTool_Definition_WithSpawnWorker_IncludesSpawnWorker(t *testing.T) {
	tool := &TaskCreateTool{
		Store:     newFakeTaskStore(),
		SessionID: "s",
		RunID:     "r",
		SpawnWorker: func(context.Context, string, string, string) (string, string, error) {
			return "child", "Subagent-1", nil
		},
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

func TestTaskCreateTool_ReviewerCannotCreateSelfAssignedNormalTask(t *testing.T) {
	store := newFakeTaskStore()
	tool := &TaskCreateTool{
		Store:           store,
		SessionID:       "session-3",
		RunID:           "run-reviewer",
		TeamID:          "team-1",
		RoleName:        "reviewer",
		IsCoordinator:   false,
		CoordinatorRole: "ceo",
		ReviewerRole:    "reviewer",
		ReviewerOnly:    true,
		ValidRoles:      []string{"ceo", "cto", "reviewer"},
	}
	raw, _ := json.Marshal(map[string]any{
		"goal":   "do implementation work",
		"taskId": "task-reviewer-self",
	})
	if _, err := tool.Execute(context.Background(), raw); err == nil {
		t.Fatalf("expected reviewer-only assignment error")
	}
}

func TestTaskCreateTool_TagsBatchMetadataFromContext(t *testing.T) {
	store := newFakeTaskStore()
	tool := &TaskCreateTool{
		Store:           store,
		SessionID:       "session-3",
		RunID:           "run-3",
		TeamID:          "team-1",
		RoleName:        "head-analyst",
		IsCoordinator:   true,
		CoordinatorRole: "head-analyst",
		ValidRoles:      []string{"head-analyst", "researcher"},
	}
	args := map[string]any{
		"goal":         "research task",
		"taskId":       "task-batch-1",
		"assignedRole": "researcher",
	}
	raw, _ := json.Marshal(args)
	ctx := WithBatchWaveState(WithParentTaskID(context.Background(), "task-parent-1"))
	_, err := tool.Execute(ctx, raw)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	task := store.tasks["task-batch-1"]
	if task.Metadata["batchMode"] != true {
		t.Fatalf("expected batchMode=true, got %v", task.Metadata["batchMode"])
	}
	if strings.TrimSpace(task.Metadata["batchParentTaskId"].(string)) != "task-parent-1" {
		t.Fatalf("unexpected batchParentTaskId: %v", task.Metadata["batchParentTaskId"])
	}
	if strings.TrimSpace(task.Metadata["batchWaveId"].(string)) == "" {
		t.Fatalf("expected batchWaveId to be set")
	}
	if strings.TrimSpace(fmt.Sprint(task.Metadata["intentId"])) == "" {
		t.Fatalf("expected deterministic intentId to be set for coordinator delegation")
	}
}

func TestTaskCreateTool_ReusesBatchWaveIDWithinContext(t *testing.T) {
	store := newFakeTaskStore()
	tool := &TaskCreateTool{
		Store:           store,
		SessionID:       "session-3",
		RunID:           "run-3",
		TeamID:          "team-1",
		RoleName:        "head-analyst",
		IsCoordinator:   true,
		CoordinatorRole: "head-analyst",
		ValidRoles:      []string{"head-analyst", "researcher"},
	}
	ctx := WithBatchWaveState(WithParentTaskID(context.Background(), "task-parent-1"))
	for _, id := range []string{"task-batch-a", "task-batch-b"} {
		raw, _ := json.Marshal(map[string]any{
			"goal":         "research task",
			"taskId":       id,
			"assignedRole": "researcher",
		})
		if _, err := tool.Execute(ctx, raw); err != nil {
			t.Fatalf("Execute(%s): %v", id, err)
		}
	}
	waveA := strings.TrimSpace(store.tasks["task-batch-a"].Metadata["batchWaveId"].(string))
	waveB := strings.TrimSpace(store.tasks["task-batch-b"].Metadata["batchWaveId"].(string))
	if waveA == "" || waveB == "" {
		t.Fatalf("expected wave IDs, got a=%q b=%q", waveA, waveB)
	}
	if waveA != waveB {
		t.Fatalf("expected stable wave ID in shared context, got %q vs %q", waveA, waveB)
	}
	intentA := strings.TrimSpace(fmt.Sprint(store.tasks["task-batch-a"].Metadata["intentId"]))
	intentB := strings.TrimSpace(fmt.Sprint(store.tasks["task-batch-b"].Metadata["intentId"]))
	if intentA != intentB {
		t.Fatalf("expected stable intentId for identical coordinator delegation, got %q vs %q", intentA, intentB)
	}
}

func TestTaskCreateTool_SpawnWorkerMessage_UsesCanonicalSubagentPaths(t *testing.T) {
	store := newFakeTaskStore()
	tool := &TaskCreateTool{
		Store:     store,
		SessionID: "s",
		RunID:     "parent-run",
		SpawnWorker: func(context.Context, string, string, string) (string, string, error) {
			return "child-run", "Subagent-1", nil
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

func TestTaskCreateTool_AliasSpawnWorkerAndTaskID_SpawnsWorker(t *testing.T) {
	store := newFakeTaskStore()
	tool := &TaskCreateTool{
		Store:     store,
		SessionID: "s",
		RunID:     "parent-run",
		SpawnWorker: func(context.Context, string, string, string) (string, string, error) {
			return "child-run", "Subagent-1", nil
		},
	}
	raw := json.RawMessage(`{"goal":"create hello","task_id":"task-alias-1","spawn_worker":true}`)
	if _, err := tool.Execute(context.Background(), raw); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	task, ok := store.tasks["task-alias-1"]
	if !ok {
		t.Fatalf("expected task created with task_id alias")
	}
	if strings.TrimSpace(task.RunID) != "child-run" {
		t.Fatalf("expected spawned child run assignment, got %q", task.RunID)
	}
	if strings.TrimSpace(task.AssignedRole) != "Subagent-1" {
		t.Fatalf("expected assignedRole Subagent-1 for spawned worker task, got %q", task.AssignedRole)
	}
	if strings.TrimSpace(metadataString(task.Metadata, "source")) != "spawn_worker" {
		t.Fatalf("expected source=spawn_worker, got %v", task.Metadata["source"])
	}
}

func TestTaskCreateTool_AliasAssignedRole_TeamMode(t *testing.T) {
	store := newFakeTaskStore()
	tool := &TaskCreateTool{
		Store:           store,
		SessionID:       "session-1",
		RunID:           "run-1",
		TeamID:          "team-1",
		RoleName:        "head-analyst",
		IsCoordinator:   true,
		CoordinatorRole: "head-analyst",
		ValidRoles:      []string{"head-analyst", "researcher"},
	}
	raw := json.RawMessage(`{"goal":"delegate task","task_id":"task-role-alias","assigned_role":"researcher"}`)
	if _, err := tool.Execute(context.Background(), raw); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	task, ok := store.tasks["task-role-alias"]
	if !ok {
		t.Fatalf("expected task created with task_id alias")
	}
	if task.AssignedRole != "researcher" {
		t.Fatalf("expected assignedRole researcher, got %q", task.AssignedRole)
	}
}

func TestTaskCreateTool_ConflictingAliasValues_ReturnsError(t *testing.T) {
	store := newFakeTaskStore()
	tool := &TaskCreateTool{
		Store:     store,
		SessionID: "s",
		RunID:     "r",
		SpawnWorker: func(context.Context, string, string, string) (string, string, error) {
			return "child-run", "Subagent-1", nil
		},
	}
	raw := json.RawMessage(`{"goal":"x","spawnWorker":true,"spawn_worker":false}`)
	if _, err := tool.Execute(context.Background(), raw); err == nil {
		t.Fatalf("expected conflict error")
	} else if !strings.Contains(err.Error(), `conflicting values for "spawnWorker"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTaskCreateTool_UnknownField_ReturnsError(t *testing.T) {
	store := newFakeTaskStore()
	tool := &TaskCreateTool{
		Store:     store,
		SessionID: "s",
		RunID:     "r",
	}
	raw := json.RawMessage(`{"goal":"x","nope":123}`)
	if _, err := tool.Execute(context.Background(), raw); err == nil {
		t.Fatalf("expected unknown-field error")
	} else {
		if !strings.Contains(err.Error(), "unknown field(s): nope") {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(err.Error(), "supported fields:") {
			t.Fatalf("expected supported-fields guidance, got: %v", err)
		}
	}
}
