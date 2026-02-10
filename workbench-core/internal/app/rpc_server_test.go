package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/workbench-core/pkg/agent/state"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/protocol"
	"github.com/tinoosan/workbench-core/pkg/store"
	"github.com/tinoosan/workbench-core/pkg/types"
)

func rpcRoundTrip(t *testing.T, srv *RPCServer, req protocol.Message) protocol.Message {
	t.Helper()
	pr, pw := io.Pipe()
	var out bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx, pr, &out) }()

	if err := json.NewEncoder(pw).Encode(req); err != nil {
		t.Fatalf("encode req: %v", err)
	}
	_ = pw.Close()
	if err := <-done; err != nil {
		t.Fatalf("Serve: %v", err)
	}
	dec := json.NewDecoder(bytes.NewReader(out.Bytes()))
	var resp protocol.Message
	if err := dec.Decode(&resp); err != nil {
		t.Fatalf("decode resp: %v", err)
	}
	return resp
}

func TestRPCServer_ThreadGet_ReturnsActiveRunID(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}

	sess := types.NewSession("goal")
	run := types.NewRun("goal", 8*1024, sess.SessionID)
	sess.CurrentRunID = run.RunID
	sess.Runs = []string{run.RunID}
	sessStore := store.NewMemorySessionStore()
	if err := sessStore.SaveSession(context.Background(), sess); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	ts, err := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}

	srv := NewRPCServer(RPCServerConfig{
		Cfg:       cfg,
		Run:       run,
		TaskStore: ts,
		Session:   sessStore,
		NotifyCh:  nil,
		Index:     protocol.NewIndex(0, 0),
		Wake:      nil,
	})

	pr, pw := io.Pipe()
	var out bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx, pr, &out) }()

	req, err := protocol.NewRequest("1", protocol.MethodThreadGet, protocol.ThreadGetParams{ThreadID: protocol.ThreadID(run.SessionID)})
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if err := json.NewEncoder(pw).Encode(req); err != nil {
		t.Fatalf("encode req: %v", err)
	}
	_ = pw.Close()

	if err := <-done; err != nil {
		t.Fatalf("Serve: %v", err)
	}

	var resp protocol.Message
	if err := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&resp); err != nil {
		t.Fatalf("decode resp: %v", err)
	}
	if resp.ID == nil || *resp.ID != "1" {
		t.Fatalf("resp.ID = %v", resp.ID)
	}
	var result protocol.ThreadGetResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.Thread.ActiveRunID != protocol.RunID(run.RunID) {
		t.Fatalf("activeRunId = %q want %q", result.Thread.ActiveRunID, run.RunID)
	}
}

func TestRPCServer_TurnCreate_CreatesTaskAndWakes(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}

	sess := types.NewSession("goal")
	run := types.NewRun("goal", 8*1024, sess.SessionID)
	sess.CurrentRunID = run.RunID
	sess.Runs = []string{run.RunID}
	sessStore := store.NewMemorySessionStore()
	if err := sessStore.SaveSession(context.Background(), sess); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	ts, err := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}

	woke := make(chan struct{}, 1)
	srv := NewRPCServer(RPCServerConfig{
		Cfg:       cfg,
		Run:       run,
		TaskStore: ts,
		Session:   sessStore,
		NotifyCh:  nil,
		Index:     protocol.NewIndex(0, 0),
		Wake: func() {
			select {
			case woke <- struct{}{}:
			default:
			}
		},
	})

	pr, pw := io.Pipe()
	var out bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx, pr, &out) }()

	req, err := protocol.NewRequest("1", protocol.MethodTurnCreate, protocol.TurnCreateParams{
		ThreadID: protocol.ThreadID(run.SessionID),
		Input:    &protocol.UserMessageContent{Text: "hello"},
	})
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if err := json.NewEncoder(pw).Encode(req); err != nil {
		t.Fatalf("encode req: %v", err)
	}
	_ = pw.Close()

	if err := <-done; err != nil {
		t.Fatalf("Serve: %v", err)
	}

	select {
	case <-woke:
	default:
		t.Fatalf("expected wake to be triggered")
	}

	var resp protocol.Message
	if err := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&resp); err != nil {
		t.Fatalf("decode resp: %v", err)
	}
	var result protocol.TurnCreateResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.Turn.RunID != protocol.RunID(run.RunID) {
		t.Fatalf("turn.runId = %q want %q", result.Turn.RunID, run.RunID)
	}
	task, err := ts.GetTask(context.Background(), string(result.Turn.ID))
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.SessionID != run.SessionID || task.RunID != run.RunID {
		t.Fatalf("task ids: session=%q run=%q", task.SessionID, task.RunID)
	}
}

func TestRPCServer_ForwardsNotifications(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}

	sess := types.NewSession("goal")
	run := types.NewRun("goal", 8*1024, sess.SessionID)
	sess.CurrentRunID = run.RunID
	sess.Runs = []string{run.RunID}
	sessStore := store.NewMemorySessionStore()
	if err := sessStore.SaveSession(context.Background(), sess); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	ts, err := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	notifyCh := make(chan protocol.Message, 8)

	idx := protocol.NewIndex(0, 0)
	srv := NewRPCServer(RPCServerConfig{
		Cfg:       cfg,
		Run:       run,
		TaskStore: ts,
		Session:   sessStore,
		NotifyCh:  notifyCh,
		Index:     idx,
		Wake:      nil,
	})

	pr, pw := io.Pipe()
	var out bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx, pr, &out) }()

	n, _ := protocol.NewNotification(protocol.NotifyTurnStarted, protocol.TurnNotificationParams{
		Turn: protocol.Turn{
			ID:        "task-1",
			ThreadID:  protocol.ThreadID(run.SessionID),
			RunID:     protocol.RunID(run.RunID),
			Status:    protocol.TurnStatusInProgress,
			CreatedAt: time.Now().UTC(),
		},
	})
	notifyCh <- n

	req, _ := protocol.NewRequest("1", protocol.MethodThreadGet, protocol.ThreadGetParams{ThreadID: protocol.ThreadID(run.SessionID)})
	_ = json.NewEncoder(pw).Encode(req)
	_ = pw.Close()

	if err := <-done; err != nil {
		t.Fatalf("Serve: %v", err)
	}

	dec := json.NewDecoder(bytes.NewReader(out.Bytes()))
	var m1, m2 protocol.Message
	if err := dec.Decode(&m1); err != nil {
		t.Fatalf("decode m1: %v", err)
	}
	if err := dec.Decode(&m2); err != nil {
		t.Fatalf("decode m2: %v", err)
	}
	if m1.Method == "" && m2.Method == "" {
		t.Fatalf("expected a notification method in output")
	}
}

func TestRPCServer_ArtifactList_RunScope(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess := types.NewSession("goal")
	run := types.NewRun("goal", 8*1024, sess.SessionID)
	sess.CurrentRunID = run.RunID
	sess.Runs = []string{run.RunID}
	sessStore := store.NewMemorySessionStore()
	_ = sessStore.SaveSession(context.Background(), sess)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))

	now := time.Now().UTC()
	task := types.Task{
		TaskID:     "task-run-1",
		SessionID:  run.SessionID,
		RunID:      run.RunID,
		Goal:       "run artifact",
		Status:     types.TaskStatusPending,
		CreatedAt:  &now,
		Inputs:     map[string]any{},
		Metadata:   map[string]any{},
		CreatedBy:  "user",
		TaskKind:   "task",
		TeamID:     "",
		RoleSnapshot: "",
	}
	if err := ts.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	done := now.Add(1 * time.Second)
	if err := ts.CompleteTask(context.Background(), task.TaskID, types.TaskResult{
		TaskID:      task.TaskID,
		Status:      types.TaskStatusSucceeded,
		Summary:     "ok",
		CompletedAt: &done,
		Artifacts:   []string{"/workspace/deliverables/2026-02-08/task-run-1/SUMMARY.md"},
	}); err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}

	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: run, TaskStore: ts, Session: sessStore, Index: protocol.NewIndex(0, 0),
	})
	req, _ := protocol.NewRequest("1", protocol.MethodArtifactList, protocol.ArtifactListParams{
		ThreadID: protocol.ThreadID(run.SessionID),
	})
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	var res protocol.ArtifactListResult
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(res.Nodes) == 0 {
		t.Fatalf("expected artifact nodes")
	}
	foundFile := false
	for _, n := range res.Nodes {
		if n.Kind == "file" && strings.Contains(n.VPath, "task-run-1") {
			foundFile = true
			break
		}
	}
	if !foundFile {
		t.Fatalf("expected run-scoped file node, got %+v", res.Nodes)
	}
}

func TestRPCServer_ArtifactSearch_ThreadMismatchAndQueryValidation(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess := types.NewSession("goal")
	run := types.NewRun("goal", 8*1024, sess.SessionID)
	sessStore := store.NewMemorySessionStore()
	_ = sessStore.SaveSession(context.Background(), sess)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: run, TaskStore: ts, Session: sessStore, Index: protocol.NewIndex(0, 0),
	})

	req, _ := protocol.NewRequest("1", protocol.MethodArtifactSearch, protocol.ArtifactSearchParams{
		ThreadID: "wrong-thread",
		Query:    "summary",
	})
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error == nil || resp.Error.Code != protocol.CodeThreadNotFound {
		t.Fatalf("expected thread not found, got %+v", resp.Error)
	}

	req2, _ := protocol.NewRequest("2", protocol.MethodArtifactSearch, protocol.ArtifactSearchParams{
		ThreadID: protocol.ThreadID(run.SessionID),
		Query:    "",
	})
	resp2 := rpcRoundTrip(t, srv, req2)
	if resp2.Error == nil || resp2.Error.Code != protocol.CodeInvalidParams {
		t.Fatalf("expected invalid params, got %+v", resp2.Error)
	}
}

func TestRPCServer_ArtifactList_TeamScopeAndGetTruncated(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess := types.NewSession("goal")
	run := types.NewRun("goal", 8*1024, sess.SessionID)
	sess.CurrentRunID = run.RunID
	sessStore := store.NewMemorySessionStore()
	_ = sessStore.SaveSession(context.Background(), sess)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))

	now := time.Now().UTC()
	t1 := types.Task{
		TaskID:       "callback-task-ceo-001",
		SessionID:    run.SessionID,
		RunID:        run.RunID,
		TeamID:       "team-1",
		AssignedRole: "ceo",
		CreatedBy:    "coordinator",
		Goal:         "CEO callback",
		Status:       types.TaskStatusPending,
		CreatedAt:    &now,
		Inputs:       map[string]any{},
		Metadata:     map[string]any{},
	}
	t2 := types.Task{
		TaskID:       "task-cto-001",
		SessionID:    run.SessionID,
		RunID:        "run-cto-2",
		TeamID:       "team-1",
		AssignedRole: "cto",
		CreatedBy:    "coordinator",
		Goal:         "CTO task",
		Status:       types.TaskStatusPending,
		CreatedAt:    &now,
		Inputs:       map[string]any{},
		Metadata:     map[string]any{},
	}
	for _, tk := range []types.Task{t1, t2} {
		if err := ts.CreateTask(context.Background(), tk); err != nil {
			t.Fatalf("CreateTask(%s): %v", tk.TaskID, err)
		}
		done := now.Add(1 * time.Second)
		if err := ts.CompleteTask(context.Background(), tk.TaskID, types.TaskResult{
			TaskID:      tk.TaskID,
			Status:      types.TaskStatusSucceeded,
			Summary:     "ok",
			CompletedAt: &done,
			Artifacts:   []string{"/workspace/deliverables/2026-02-08/" + tk.TaskID + "/SUMMARY.md"},
		}); err != nil {
			t.Fatalf("CompleteTask(%s): %v", tk.TaskID, err)
		}
	}

	// Materialize file for artifact.get under team workspace.
	p := filepath.Join(cfg.DataDir, "teams", "team-1", "workspace", "deliverables", "2026-02-08", t1.TaskID, "SUMMARY.md")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte("1234567890"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: run, TaskStore: ts, Session: sessStore, Index: protocol.NewIndex(0, 0),
	})

	listReq, _ := protocol.NewRequest("1", protocol.MethodArtifactList, protocol.ArtifactListParams{
		ThreadID: protocol.ThreadID(run.SessionID),
	})
	listResp := rpcRoundTrip(t, srv, listReq)
	if listResp.Error != nil {
		t.Fatalf("list error: %+v", listResp.Error)
	}
	var listRes protocol.ArtifactListResult
	_ = json.Unmarshal(listResp.Result, &listRes)
	foundCTO := false
	for _, n := range listRes.Nodes {
		if n.Kind == "role" && n.Label == "cto" {
			foundCTO = true
			break
		}
	}
	if !foundCTO {
		t.Fatalf("expected team-scope listing to include cto role, got %+v", listRes.Nodes)
	}

	getReq, _ := protocol.NewRequest("2", protocol.MethodArtifactGet, protocol.ArtifactGetParams{
		ThreadID: protocol.ThreadID(run.SessionID),
		VPath:    "/workspace/deliverables/2026-02-08/" + t1.TaskID + "/SUMMARY.md",
		MaxBytes: 5,
	})
	getResp := rpcRoundTrip(t, srv, getReq)
	if getResp.Error != nil {
		t.Fatalf("get error: %+v", getResp.Error)
	}
	var getRes protocol.ArtifactGetResult
	_ = json.Unmarshal(getResp.Result, &getRes)
	if !getRes.Truncated || getRes.BytesRead != 5 || getRes.Content != "12345" {
		t.Fatalf("unexpected get result: %+v", getRes)
	}
}

func TestRPCServer_TaskFlow_CreateListClaimComplete(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess := types.NewSession("goal")
	run := types.NewRun("goal", 8*1024, sess.SessionID)
	sess.CurrentRunID = run.RunID
	sessStore := store.NewMemorySessionStore()
	_ = sessStore.SaveSession(context.Background(), sess)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: run, TaskStore: ts, Session: sessStore, Index: protocol.NewIndex(0, 0),
	})

	createReq, _ := protocol.NewRequest("1", protocol.MethodTaskCreate, protocol.TaskCreateParams{
		ThreadID: protocol.ThreadID(run.SessionID),
		Goal:     "Do X",
	})
	createResp := rpcRoundTrip(t, srv, createReq)
	if createResp.Error != nil {
		t.Fatalf("task.create error: %+v", createResp.Error)
	}
	var createRes protocol.TaskCreateResult
	_ = json.Unmarshal(createResp.Result, &createRes)
	if strings.TrimSpace(createRes.Task.ID) == "" {
		t.Fatalf("missing task id")
	}

	inboxReq, _ := protocol.NewRequest("2", protocol.MethodTaskList, protocol.TaskListParams{
		ThreadID: protocol.ThreadID(run.SessionID),
		View:     "inbox",
	})
	inboxResp := rpcRoundTrip(t, srv, inboxReq)
	if inboxResp.Error != nil {
		t.Fatalf("task.list inbox error: %+v", inboxResp.Error)
	}
	var inboxRes protocol.TaskListResult
	_ = json.Unmarshal(inboxResp.Result, &inboxRes)
	if len(inboxRes.Tasks) == 0 {
		t.Fatalf("expected inbox tasks")
	}

	claimReq, _ := protocol.NewRequest("3", protocol.MethodTaskClaim, protocol.TaskClaimParams{
		ThreadID: protocol.ThreadID(run.SessionID),
		TaskID:   createRes.Task.ID,
		AgentID:  "agent-1",
	})
	claimResp := rpcRoundTrip(t, srv, claimReq)
	if claimResp.Error != nil {
		t.Fatalf("task.claim error: %+v", claimResp.Error)
	}
	var claimRes protocol.TaskClaimResult
	_ = json.Unmarshal(claimResp.Result, &claimRes)
	if claimRes.Task.ClaimedByAgentID != "agent-1" {
		t.Fatalf("expected claimedByAgentId agent-1, got %q", claimRes.Task.ClaimedByAgentID)
	}

	completeReq, _ := protocol.NewRequest("4", protocol.MethodTaskComplete, protocol.TaskCompleteParams{
		ThreadID: protocol.ThreadID(run.SessionID),
		TaskID:   createRes.Task.ID,
		Summary:  "done",
		Status:   "succeeded",
	})
	completeResp := rpcRoundTrip(t, srv, completeReq)
	if completeResp.Error != nil {
		t.Fatalf("task.complete error: %+v", completeResp.Error)
	}

	outboxReq, _ := protocol.NewRequest("5", protocol.MethodTaskList, protocol.TaskListParams{
		ThreadID: protocol.ThreadID(run.SessionID),
		View:     "outbox",
	})
	outboxResp := rpcRoundTrip(t, srv, outboxReq)
	if outboxResp.Error != nil {
		t.Fatalf("task.list outbox error: %+v", outboxResp.Error)
	}
	var outboxRes protocol.TaskListResult
	_ = json.Unmarshal(outboxResp.Result, &outboxRes)
	found := false
	for _, tk := range outboxRes.Tasks {
		if tk.ID == createRes.Task.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("completed task not found in outbox")
	}
}

func TestRPCServer_ControlSetModel(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess := types.NewSession("goal")
	run := types.NewRun("goal", 8*1024, sess.SessionID)
	sessStore := store.NewMemorySessionStore()
	_ = sessStore.SaveSession(context.Background(), sess)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	seen := ""
	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: run, TaskStore: ts, Session: sessStore, Index: protocol.NewIndex(0, 0),
		ControlSetModel: func(_ context.Context, threadID, target, model string) ([]string, error) {
			seen = threadID + "|" + target + "|" + model
			return []string{"run-1"}, nil
		},
	})

	req, _ := protocol.NewRequest("1", protocol.MethodControlSetModel, protocol.ControlSetModelParams{
		ThreadID: protocol.ThreadID(run.SessionID),
		Model:    "openai/gpt-5.2",
		Target:   "run-1",
	})
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error != nil {
		t.Fatalf("control.setModel error: %+v", resp.Error)
	}
	if seen != run.SessionID+"|run-1|openai/gpt-5.2" {
		t.Fatalf("unexpected callback payload: %q", seen)
	}
	var res protocol.ControlSetModelResult
	_ = json.Unmarshal(resp.Result, &res)
	if !res.Accepted || len(res.AppliedTo) != 1 || res.AppliedTo[0] != "run-1" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestRPCServer_ControlSetProfile_ThreadMismatch(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess := types.NewSession("goal")
	run := types.NewRun("goal", 8*1024, sess.SessionID)
	sessStore := store.NewMemorySessionStore()
	_ = sessStore.SaveSession(context.Background(), sess)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: run, TaskStore: ts, Session: sessStore, Index: protocol.NewIndex(0, 0),
		ControlSetProfile: func(_ context.Context, _, _, _ string) ([]string, error) {
			t.Fatalf("callback should not be called")
			return nil, nil
		},
	})

	req, _ := protocol.NewRequest("1", protocol.MethodControlSetProfile, protocol.ControlSetProfileParams{
		ThreadID: protocol.ThreadID("sess-other"),
		Profile:  "software_dev",
	})
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error == nil {
		t.Fatalf("expected thread mismatch error")
	}
	if resp.Error.Code != protocol.CodeThreadNotFound {
		t.Fatalf("error code = %d want %d", resp.Error.Code, protocol.CodeThreadNotFound)
	}
}
