package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	implstore "github.com/tinoosan/agen8/internal/store"
	"github.com/tinoosan/agen8/pkg/agent/state"
	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/fsutil"
	"github.com/tinoosan/agen8/pkg/protocol"
	pkgagent "github.com/tinoosan/agen8/pkg/services/agent"
	pkgsession "github.com/tinoosan/agen8/pkg/services/session"
	pkgtask "github.com/tinoosan/agen8/pkg/services/task"
	"github.com/tinoosan/agen8/pkg/store"
	"github.com/tinoosan/agen8/pkg/types"
)

// noopSessionSupervisor is a test double for RuntimeSupervisor (StopRun/ResumeRun no-op).
type noopSessionSupervisor struct{}

func (noopSessionSupervisor) StopRun(runID string) error                        { return nil }
func (noopSessionSupervisor) ResumeRun(ctx context.Context, runID string) error { return nil }

// newTestSessionService returns a Session service (Manager) backed by the given store for tests.
func newTestSessionService(cfg config.Config, st pkgsession.Store) pkgsession.Service {
	return pkgsession.NewManager(cfg, st, noopSessionSupervisor{})
}

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
	if err := sessStore.SaveRun(context.Background(), run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}

	ts, err := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}

	srv := NewRPCServer(RPCServerConfig{
		Cfg:         cfg,
		Run:         run,
		TaskService: pkgtask.NewManager(ts, nil),
		Session:     newTestSessionService(cfg, sessStore),
		NotifyCh:    nil,
		Index:       protocol.NewIndex(0, 0),
		Wake:        nil,
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
	if err := sessStore.SaveRun(context.Background(), run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}

	ts, err := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}

	woke := make(chan struct{}, 1)
	srv := NewRPCServer(RPCServerConfig{
		Cfg:         cfg,
		Run:         run,
		TaskService: pkgtask.NewManager(ts, nil),
		Session:     newTestSessionService(cfg, sessStore),
		NotifyCh:    nil,
		Index:       protocol.NewIndex(0, 0),
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
	if err := sessStore.SaveRun(context.Background(), run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}

	ts, err := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	notifyCh := make(chan protocol.Message, 8)

	idx := protocol.NewIndex(0, 0)
	srv := NewRPCServer(RPCServerConfig{
		Cfg:         cfg,
		Run:         run,
		TaskService: pkgtask.NewManager(ts, nil),
		Session:     newTestSessionService(cfg, sessStore),
		NotifyCh:    notifyCh,
		Index:       idx,
		Wake:        nil,
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
	_ = sessStore.SaveRun(context.Background(), run)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))

	now := time.Now().UTC()
	task := types.Task{
		TaskID:       "task-run-1",
		SessionID:    run.SessionID,
		RunID:        run.RunID,
		Goal:         "run artifact",
		Status:       types.TaskStatusPending,
		CreatedAt:    &now,
		Inputs:       map[string]any{},
		Metadata:     map[string]any{},
		CreatedBy:    "user",
		TaskKind:     "task",
		TeamID:       "",
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
		Artifacts:   []string{"/workspace/run-report.md"},
	}); err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}

	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: run, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
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
		if n.Kind == "file" && strings.EqualFold(strings.TrimSpace(n.VPath), "/workspace/run-report.md") {
			foundFile = true
			break
		}
	}
	if !foundFile {
		t.Fatalf("expected run-scoped file node, got %+v", res.Nodes)
	}
}

func TestRPCServer_ArtifactList_StandaloneIncludesSummaryNode(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess := types.NewSession("goal")
	run := types.NewRun("goal", 8*1024, sess.SessionID)
	sess.CurrentRunID = run.RunID
	sess.Runs = []string{run.RunID}
	sessStore := store.NewMemorySessionStore()
	_ = sessStore.SaveSession(context.Background(), sess)
	_ = sessStore.SaveRun(context.Background(), run)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))

	now := time.Now().UTC()
	taskID := "task-summary-1"
	task := types.Task{
		TaskID:    taskID,
		SessionID: run.SessionID,
		RunID:     run.RunID,
		Goal:      "summary only artifact",
		Status:    types.TaskStatusPending,
		CreatedAt: &now,
		Inputs:    map[string]any{},
		Metadata:  map[string]any{},
		CreatedBy: "user",
		TaskKind:  "task",
	}
	if err := ts.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	done := now.Add(1 * time.Second)
	summaryVPath := "/tasks/2026-02-08/" + taskID + "/SUMMARY.md"
	if err := ts.CompleteTask(context.Background(), task.TaskID, types.TaskResult{
		TaskID:      task.TaskID,
		Status:      types.TaskStatusSucceeded,
		Summary:     "ok",
		CompletedAt: &done,
		Artifacts:   []string{summaryVPath},
	}); err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}

	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: run, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
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
	foundSummary := false
	for _, n := range res.Nodes {
		if n.Kind == "file" && strings.EqualFold(strings.TrimSpace(n.VPath), summaryVPath) && n.IsSummary {
			foundSummary = true
			break
		}
	}
	if !foundSummary {
		t.Fatalf("expected summary file node in standalone artifact list, got %+v", res.Nodes)
	}
}

func TestRPCServer_ArtifactList_StandaloneSessionIncludesAllRuns(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess := types.NewSession("goal")
	runA := types.NewRun("goal", 8*1024, sess.SessionID)
	runB := types.NewRun("goal", 8*1024, sess.SessionID)
	sess.CurrentRunID = runA.RunID
	sess.Runs = []string{runA.RunID, runB.RunID}
	sessStore := store.NewMemorySessionStore()
	_ = sessStore.SaveSession(context.Background(), sess)
	_ = sessStore.SaveRun(context.Background(), runA)
	_ = sessStore.SaveRun(context.Background(), runB)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))

	now := time.Now().UTC()
	tasks := []types.Task{
		{
			TaskID:    "task-run-a",
			SessionID: sess.SessionID,
			RunID:     runA.RunID,
			Goal:      "run a artifact",
			Status:    types.TaskStatusPending,
			CreatedAt: &now,
			Inputs:    map[string]any{},
			Metadata:  map[string]any{},
			CreatedBy: "user",
			TaskKind:  "task",
		},
		{
			TaskID:    "task-run-b",
			SessionID: sess.SessionID,
			RunID:     runB.RunID,
			Goal:      "run b artifact",
			Status:    types.TaskStatusPending,
			CreatedAt: &now,
			Inputs:    map[string]any{},
			Metadata:  map[string]any{},
			CreatedBy: "user",
			TaskKind:  "task",
		},
	}
	for _, task := range tasks {
		if err := ts.CreateTask(context.Background(), task); err != nil {
			t.Fatalf("CreateTask(%s): %v", task.TaskID, err)
		}
		done := now.Add(1 * time.Second)
		if err := ts.CompleteTask(context.Background(), task.TaskID, types.TaskResult{
			TaskID:      task.TaskID,
			Status:      types.TaskStatusSucceeded,
			Summary:     "ok",
			CompletedAt: &done,
			Artifacts:   []string{"/workspace/" + task.TaskID + ".md"},
		}); err != nil {
			t.Fatalf("CompleteTask(%s): %v", task.TaskID, err)
		}
	}

	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: runA, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
	})
	req, _ := protocol.NewRequest("1", protocol.MethodArtifactList, protocol.ArtifactListParams{
		ThreadID: protocol.ThreadID(sess.SessionID),
	})
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	var res protocol.ArtifactListResult
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	foundRunB := false
	for _, n := range res.Nodes {
		if n.Kind == "file" && strings.EqualFold(strings.TrimSpace(n.VPath), "/workspace/task-run-b.md") {
			foundRunB = true
			break
		}
	}
	if !foundRunB {
		t.Fatalf("expected artifact from secondary run, got %+v", res.Nodes)
	}
}

func TestRPCServer_ArtifactGet_StandaloneUnreportedByVPath(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess := types.NewSession("goal")
	run := types.NewRun("goal", 8*1024, sess.SessionID)
	sess.CurrentRunID = run.RunID
	sess.Runs = []string{run.RunID}
	sessStore := store.NewMemorySessionStore()
	_ = sessStore.SaveSession(context.Background(), sess)
	_ = sessStore.SaveRun(context.Background(), run)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))

	p := filepath.Join(fsutil.GetWorkspaceDir(cfg.DataDir, run.RunID), "sample_report.md")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte("hello workspace"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: run, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
	})
	req, _ := protocol.NewRequest("1", protocol.MethodArtifactGet, protocol.ArtifactGetParams{
		ThreadID: protocol.ThreadID(sess.SessionID),
		VPath:    "/workspace/sample_report.md",
	})
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error != nil {
		t.Fatalf("artifact.get error: %+v", resp.Error)
	}
	var out protocol.ArtifactGetResult
	if err := json.Unmarshal(resp.Result, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if strings.TrimSpace(out.Content) != "hello workspace" {
		t.Fatalf("unexpected content: %q", out.Content)
	}
}

func TestRPCServer_ArtifactGet_StandaloneUnreportedAcrossSessionRuns(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess := types.NewSession("goal")
	runA := types.NewRun("goal", 8*1024, sess.SessionID)
	runB := types.NewRun("goal", 8*1024, sess.SessionID)
	sess.CurrentRunID = runA.RunID
	sess.Runs = []string{runA.RunID, runB.RunID}
	sessStore := store.NewMemorySessionStore()
	_ = sessStore.SaveSession(context.Background(), sess)
	_ = sessStore.SaveRun(context.Background(), runA)
	_ = sessStore.SaveRun(context.Background(), runB)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))

	p := filepath.Join(fsutil.GetWorkspaceDir(cfg.DataDir, runB.RunID), "sample_report.md")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte("hello from run-b"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: runA, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
	})
	req, _ := protocol.NewRequest("1", protocol.MethodArtifactGet, protocol.ArtifactGetParams{
		ThreadID: protocol.ThreadID(sess.SessionID),
		VPath:    "/workspace/sample_report.md",
	})
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error != nil {
		t.Fatalf("artifact.get error: %+v", resp.Error)
	}
	var out protocol.ArtifactGetResult
	if err := json.Unmarshal(resp.Result, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if strings.TrimSpace(out.Content) != "hello from run-b" {
		t.Fatalf("unexpected content: %q", out.Content)
	}
}

func TestRPCServer_ArtifactGet_StandaloneCanonicalSubagentSummaryPath(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess := types.NewSession("goal")
	parent := types.NewRun("goal", 8*1024, sess.SessionID)
	sess.CurrentRunID = parent.RunID
	sess.Runs = []string{parent.RunID}
	sessStore := store.NewMemorySessionStore()
	_ = sessStore.SaveSession(context.Background(), sess)
	_ = sessStore.SaveRun(context.Background(), parent)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))

	summaryVPath := "/tasks/subagent-1/2026-02-17/task-child-1/SUMMARY.md"
	summaryDisk := filepath.Join(fsutil.GetTasksDir(cfg.DataDir, parent.RunID), "subagent-1", "2026-02-17", "task-child-1", "SUMMARY.md")
	if err := os.MkdirAll(filepath.Dir(summaryDisk), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(summaryDisk, []byte("child summary content"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: parent, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
	})
	req, _ := protocol.NewRequest("1", protocol.MethodArtifactGet, protocol.ArtifactGetParams{
		ThreadID: protocol.ThreadID(sess.SessionID),
		VPath:    summaryVPath,
	})
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error != nil {
		t.Fatalf("artifact.get error: %+v", resp.Error)
	}
	var out protocol.ArtifactGetResult
	if err := json.Unmarshal(resp.Result, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if strings.TrimSpace(out.Content) != "child summary content" {
		t.Fatalf("unexpected content: %q", out.Content)
	}
}

func TestRPCServer_ArtifactSearch_ThreadMismatchAndQueryValidation(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess := types.NewSession("goal")
	run := types.NewRun("goal", 8*1024, sess.SessionID)
	sessStore := store.NewMemorySessionStore()
	_ = sessStore.SaveSession(context.Background(), sess)
	_ = sessStore.SaveRun(context.Background(), run)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: run, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
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
	_ = sessStore.SaveRun(context.Background(), run)
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
			Artifacts:   []string{"/tasks/" + tk.AssignedRole + "/2026-02-08/" + tk.TaskID + "/SUMMARY.md"},
		}); err != nil {
			t.Fatalf("CompleteTask(%s): %v", tk.TaskID, err)
		}
	}

	// Materialize file for artifact.get under shared team tasks root.
	p := filepath.Join(fsutil.GetTeamTasksDir(cfg.DataDir, "team-1"), t1.AssignedRole, "2026-02-08", t1.TaskID, "SUMMARY.md")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte("1234567890"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: run, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
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
		VPath:    "/tasks/" + t1.AssignedRole + "/2026-02-08/" + t1.TaskID + "/SUMMARY.md",
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
	_ = sessStore.SaveRun(context.Background(), run)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: run, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
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
	inboxAfterClaimReq, _ := protocol.NewRequest("3b", protocol.MethodTaskList, protocol.TaskListParams{
		ThreadID: protocol.ThreadID(run.SessionID),
		View:     "inbox",
	})
	inboxAfterClaimResp := rpcRoundTrip(t, srv, inboxAfterClaimReq)
	if inboxAfterClaimResp.Error != nil {
		t.Fatalf("task.list inbox after claim error: %+v", inboxAfterClaimResp.Error)
	}
	var inboxAfterClaimRes protocol.TaskListResult
	_ = json.Unmarshal(inboxAfterClaimResp.Result, &inboxAfterClaimRes)
	foundActive := false
	for _, tk := range inboxAfterClaimRes.Tasks {
		if tk.ID == createRes.Task.ID && strings.EqualFold(strings.TrimSpace(tk.Status), string(types.TaskStatusActive)) {
			foundActive = true
			break
		}
	}
	if !foundActive {
		t.Fatalf("expected claimed active task to remain visible in inbox view")
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

func TestRPCServer_NonBootstrapThreadScopeDefaults(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	bootstrapSess, bootstrapRun, err := implstore.CreateSession(cfg, "bootstrap", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession bootstrap: %v", err)
	}
	targetSess, targetRun, err := implstore.CreateSession(cfg, "target", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession target: %v", err)
	}
	targetSess.InputTokens = 101
	targetSess.OutputTokens = 17
	targetSess.TotalTokens = 118
	targetSess.CostUSD = 0.42

	sessStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore: %v", err)
	}
	if err := sessStore.SaveSession(context.Background(), bootstrapSess); err != nil {
		t.Fatalf("SaveSession bootstrap: %v", err)
	}
	if err := sessStore.SaveSession(context.Background(), targetSess); err != nil {
		t.Fatalf("SaveSession target: %v", err)
	}
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: bootstrapRun, AllowAnyThread: true, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
	})

	createReq, _ := protocol.NewRequest("1", protocol.MethodTaskCreate, protocol.TaskCreateParams{
		ThreadID: protocol.ThreadID(targetRun.SessionID),
		Goal:     "thread scoped task",
	})
	createResp := rpcRoundTrip(t, srv, createReq)
	if createResp.Error != nil {
		t.Fatalf("task.create error: %+v", createResp.Error)
	}
	var createRes protocol.TaskCreateResult
	_ = json.Unmarshal(createResp.Result, &createRes)
	if got := strings.TrimSpace(string(createRes.Task.ThreadID)); got != targetRun.SessionID {
		t.Fatalf("task thread=%q want %q", got, targetRun.SessionID)
	}
	if got := strings.TrimSpace(string(createRes.Task.RunID)); got != targetRun.RunID {
		t.Fatalf("task run=%q want %q", got, targetRun.RunID)
	}

	claimReq, _ := protocol.NewRequest("2", protocol.MethodTaskClaim, protocol.TaskClaimParams{
		ThreadID: protocol.ThreadID(targetRun.SessionID),
		TaskID:   createRes.Task.ID,
	})
	claimResp := rpcRoundTrip(t, srv, claimReq)
	if claimResp.Error != nil {
		t.Fatalf("task.claim error: %+v", claimResp.Error)
	}
	var claimRes protocol.TaskClaimResult
	_ = json.Unmarshal(claimResp.Result, &claimRes)
	if got := strings.TrimSpace(claimRes.Task.ClaimedByAgentID); got != targetRun.RunID {
		t.Fatalf("claimedBy=%q want %q", got, targetRun.RunID)
	}

	totalsReq, _ := protocol.NewRequest("3", protocol.MethodSessionGetTotals, protocol.SessionGetTotalsParams{
		ThreadID: protocol.ThreadID(targetRun.SessionID),
	})
	totalsResp := rpcRoundTrip(t, srv, totalsReq)
	if totalsResp.Error != nil {
		t.Fatalf("session.getTotals error: %+v", totalsResp.Error)
	}
	var totals protocol.SessionGetTotalsResult
	_ = json.Unmarshal(totalsResp.Result, &totals)
	if totals.TotalTokensIn != 101 || totals.TotalTokensOut != 17 || totals.TotalTokens != 118 {
		t.Fatalf("unexpected totals tokens: %+v", totals)
	}
	if totals.TotalCostUSD != 0.42 {
		t.Fatalf("unexpected totals cost: %+v", totals)
	}
}

func TestRPCServer_SessionRename_DefaultSessionFromThread_NonBootstrap(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	bootstrapSess, bootstrapRun, err := implstore.CreateSession(cfg, "bootstrap", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession bootstrap: %v", err)
	}
	targetSess, targetRun, err := implstore.CreateSession(cfg, "target", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession target: %v", err)
	}
	sessStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore: %v", err)
	}
	_ = sessStore.SaveSession(context.Background(), bootstrapSess)
	_ = sessStore.SaveSession(context.Background(), targetSess)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: bootstrapRun, AllowAnyThread: true, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
	})

	req, _ := protocol.NewRequest("1", protocol.MethodSessionRename, protocol.SessionRenameParams{
		ThreadID: protocol.ThreadID(targetRun.SessionID),
		Title:    "renamed target session",
	})
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error != nil {
		t.Fatalf("session.rename error: %+v", resp.Error)
	}
	loadedTarget, err := sessStore.LoadSession(context.Background(), targetRun.SessionID)
	if err != nil {
		t.Fatalf("LoadSession target: %v", err)
	}
	if got := strings.TrimSpace(loadedTarget.Title); got != "renamed target session" {
		t.Fatalf("target title=%q want renamed target session", got)
	}
	loadedBootstrap, err := sessStore.LoadSession(context.Background(), bootstrapRun.SessionID)
	if err != nil {
		t.Fatalf("LoadSession bootstrap: %v", err)
	}
	if strings.TrimSpace(loadedBootstrap.Title) == "renamed target session" {
		t.Fatalf("bootstrap session title was incorrectly renamed")
	}
}

func TestRPCServer_AgentList_DefaultSessionFromThread_NonBootstrap(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	bootstrapSess, bootstrapRun, err := implstore.CreateSession(cfg, "bootstrap", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession bootstrap: %v", err)
	}
	targetSess, targetRun, err := implstore.CreateSession(cfg, "target", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession target: %v", err)
	}
	targetExtraRun := types.NewRun("target-extra", 8*1024, targetRun.SessionID)
	if err := implstore.SaveRun(cfg, targetExtraRun); err != nil {
		t.Fatalf("SaveRun target extra: %v", err)
	}
	targetSess.Runs = append(targetSess.Runs, targetExtraRun.RunID)
	sessStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore: %v", err)
	}
	_ = sessStore.SaveSession(context.Background(), bootstrapSess)
	_ = sessStore.SaveSession(context.Background(), targetSess)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	sessionSvc := newTestSessionService(cfg, sessStore)
	taskMgr := pkgtask.NewManager(ts, sessionSvc)
	agentMgr := pkgagent.NewManager(sessionSvc, taskMgr, taskMgr)
	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: bootstrapRun, AllowAnyThread: true, TaskService: taskMgr, Session: sessionSvc, AgentService: agentMgr, Index: protocol.NewIndex(0, 0),
	})

	req, _ := protocol.NewRequest("1", protocol.MethodAgentList, protocol.AgentListParams{
		ThreadID: protocol.ThreadID(targetRun.SessionID),
	})
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error != nil {
		t.Fatalf("agent.list error: %+v", resp.Error)
	}
	var listed protocol.AgentListResult
	_ = json.Unmarshal(resp.Result, &listed)
	if len(listed.Agents) == 0 {
		t.Fatalf("expected agents for target session")
	}
	for _, a := range listed.Agents {
		if strings.TrimSpace(a.SessionID) != targetRun.SessionID {
			t.Fatalf("agent.list returned non-target session run: %+v", a)
		}
	}
}

func TestRPCServer_AgentStart_DefaultSessionFromThread_NonBootstrap(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	bootstrapSess, bootstrapRun, err := implstore.CreateSession(cfg, "bootstrap", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession bootstrap: %v", err)
	}
	targetSess, targetRun, err := implstore.CreateSession(cfg, "target", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession target: %v", err)
	}
	sessStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore: %v", err)
	}
	_ = sessStore.SaveSession(context.Background(), bootstrapSess)
	_ = sessStore.SaveSession(context.Background(), targetSess)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	sessionSvc := newTestSessionService(cfg, sessStore)
	taskMgr := pkgtask.NewManager(ts, sessionSvc)
	agentMgr := pkgagent.NewManager(sessionSvc, taskMgr, taskMgr)
	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: bootstrapRun, AllowAnyThread: true, TaskService: taskMgr, Session: sessionSvc, AgentService: agentMgr, Index: protocol.NewIndex(0, 0),
	})

	req, _ := protocol.NewRequest("1", protocol.MethodAgentStart, protocol.AgentStartParams{
		ThreadID: protocol.ThreadID(targetRun.SessionID),
		Goal:     "new target run",
	})
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error != nil {
		t.Fatalf("agent.start error: %+v", resp.Error)
	}
	var out protocol.AgentStartResult
	_ = json.Unmarshal(resp.Result, &out)
	if got := strings.TrimSpace(out.SessionID); got != targetRun.SessionID {
		t.Fatalf("agent.start session=%q want %q", got, targetRun.SessionID)
	}
	createdRun, err := implstore.LoadRun(cfg, strings.TrimSpace(out.RunID))
	if err != nil {
		t.Fatalf("LoadRun created: %v", err)
	}
	if strings.TrimSpace(createdRun.SessionID) != targetRun.SessionID {
		t.Fatalf("created run session=%q want %q", createdRun.SessionID, targetRun.SessionID)
	}
}

func TestRPCServer_SessionPauseResume_DefaultSessionFromThread_NonBootstrap(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	bootstrapSess, bootstrapRun, err := implstore.CreateSession(cfg, "bootstrap", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession bootstrap: %v", err)
	}
	targetSess, targetRun, err := implstore.CreateSession(cfg, "target", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession target: %v", err)
	}
	targetRun2 := types.NewRun("target-2", 8*1024, targetRun.SessionID)
	if err := implstore.SaveRun(cfg, targetRun2); err != nil {
		t.Fatalf("SaveRun target2: %v", err)
	}
	targetSess.Runs = append(targetSess.Runs, targetRun2.RunID)
	sessStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore: %v", err)
	}
	_ = sessStore.SaveSession(context.Background(), bootstrapSess)
	_ = sessStore.SaveSession(context.Background(), targetSess)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	sessionSvc := newTestSessionService(cfg, sessStore)
	taskMgr := pkgtask.NewManager(ts, sessionSvc)
	agentMgr := pkgagent.NewManager(sessionSvc, taskMgr, taskMgr)
	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: bootstrapRun, AllowAnyThread: true, TaskService: taskMgr, Session: sessionSvc, AgentService: agentMgr, Index: protocol.NewIndex(0, 0),
	})

	pauseReq, _ := protocol.NewRequest("1", protocol.MethodSessionPause, protocol.SessionPauseParams{
		ThreadID: protocol.ThreadID(targetRun.SessionID),
	})
	pauseResp := rpcRoundTrip(t, srv, pauseReq)
	if pauseResp.Error != nil {
		t.Fatalf("session.pause error: %+v", pauseResp.Error)
	}
	var pauseOut protocol.SessionPauseResult
	_ = json.Unmarshal(pauseResp.Result, &pauseOut)
	if got := strings.TrimSpace(pauseOut.SessionID); got != targetRun.SessionID {
		t.Fatalf("session.pause sessionId=%q want %q", got, targetRun.SessionID)
	}
	for _, runID := range []string{targetRun.RunID, targetRun2.RunID} {
		run, err := implstore.LoadRun(cfg, runID)
		if err != nil {
			t.Fatalf("LoadRun(%s): %v", runID, err)
		}
		if run.Status != types.RunStatusPaused {
			t.Fatalf("run %s status=%q want paused", runID, run.Status)
		}
	}
	bootstrapLoaded, err := implstore.LoadRun(cfg, bootstrapRun.RunID)
	if err != nil {
		t.Fatalf("LoadRun bootstrap: %v", err)
	}
	if bootstrapLoaded.Status == types.RunStatusPaused {
		t.Fatalf("bootstrap run should not be paused")
	}

	resumeReq, _ := protocol.NewRequest("2", protocol.MethodSessionResume, protocol.SessionResumeParams{
		ThreadID: protocol.ThreadID(targetRun.SessionID),
	})
	resumeResp := rpcRoundTrip(t, srv, resumeReq)
	if resumeResp.Error != nil {
		t.Fatalf("session.resume error: %+v", resumeResp.Error)
	}
	for _, runID := range []string{targetRun.RunID, targetRun2.RunID} {
		run, err := implstore.LoadRun(cfg, runID)
		if err != nil {
			t.Fatalf("LoadRun(%s): %v", runID, err)
		}
		if run.Status != types.RunStatusRunning {
			t.Fatalf("run %s status=%q want running", runID, run.Status)
		}
	}
}

func TestRPCServer_ThreadGet_NonBootstrapThread(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	bootstrapSess, bootstrapRun, err := implstore.CreateSession(cfg, "bootstrap", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession bootstrap: %v", err)
	}
	targetSess, targetRun, err := implstore.CreateSession(cfg, "target", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession target: %v", err)
	}
	targetSess.CurrentRunID = targetRun.RunID
	sessStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore: %v", err)
	}
	_ = sessStore.SaveSession(context.Background(), bootstrapSess)
	_ = sessStore.SaveSession(context.Background(), targetSess)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: bootstrapRun, AllowAnyThread: true, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
	})

	req, _ := protocol.NewRequest("1", protocol.MethodThreadGet, protocol.ThreadGetParams{
		ThreadID: protocol.ThreadID(targetRun.SessionID),
	})
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error != nil {
		t.Fatalf("thread.get error: %+v", resp.Error)
	}
	var out protocol.ThreadGetResult
	_ = json.Unmarshal(resp.Result, &out)
	if got := strings.TrimSpace(string(out.Thread.ID)); got != targetRun.SessionID {
		t.Fatalf("thread id=%q want %q", got, targetRun.SessionID)
	}
	if got := strings.TrimSpace(string(out.Thread.ActiveRunID)); got != targetRun.RunID {
		t.Fatalf("active run=%q want %q", got, targetRun.RunID)
	}
}

func TestRPCServer_ThreadCreate_NonBootstrapThread(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	bootstrapSess, bootstrapRun, err := implstore.CreateSession(cfg, "bootstrap", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession bootstrap: %v", err)
	}
	targetSess, targetRun, err := implstore.CreateSession(cfg, "target", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession target: %v", err)
	}
	sessStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore: %v", err)
	}
	_ = sessStore.SaveSession(context.Background(), bootstrapSess)
	_ = sessStore.SaveSession(context.Background(), targetSess)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: bootstrapRun, AllowAnyThread: true, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
	})

	req, _ := protocol.NewRequest("1", protocol.MethodThreadCreate, protocol.ThreadCreateParams{
		ThreadID:    protocol.ThreadID(targetRun.SessionID),
		Title:       "target updated",
		ActiveModel: "openai/gpt-5",
	})
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error != nil {
		t.Fatalf("thread.create error: %+v", resp.Error)
	}
	var out protocol.ThreadCreateResult
	_ = json.Unmarshal(resp.Result, &out)
	if got := strings.TrimSpace(string(out.Thread.ID)); got != targetRun.SessionID {
		t.Fatalf("thread id=%q want %q", got, targetRun.SessionID)
	}
	if got := strings.TrimSpace(out.Thread.Title); got != "target updated" {
		t.Fatalf("thread title=%q want %q", got, "target updated")
	}
	if got := strings.TrimSpace(out.Thread.ActiveModel); got != "openai/gpt-5" {
		t.Fatalf("thread active model=%q want %q", got, "openai/gpt-5")
	}

	loadedTarget, err := sessStore.LoadSession(context.Background(), targetRun.SessionID)
	if err != nil {
		t.Fatalf("LoadSession target: %v", err)
	}
	if strings.TrimSpace(loadedTarget.Title) != "target updated" {
		t.Fatalf("target title not updated")
	}
	loadedBootstrap, err := sessStore.LoadSession(context.Background(), bootstrapRun.SessionID)
	if err != nil {
		t.Fatalf("LoadSession bootstrap: %v", err)
	}
	if strings.TrimSpace(loadedBootstrap.Title) == "target updated" {
		t.Fatalf("bootstrap session should not be updated")
	}
}

func TestRPCServer_ThreadCreate_AllowAnyThreadRequiresThreadID(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess, run, err := implstore.CreateSession(cfg, "bootstrap", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	sessStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore: %v", err)
	}
	_ = sessStore.SaveSession(context.Background(), sess)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: run, AllowAnyThread: true, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
	})

	req, _ := protocol.NewRequest("1", protocol.MethodThreadCreate, protocol.ThreadCreateParams{
		Title: "missing thread id",
	})
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error == nil {
		t.Fatalf("expected invalid params error")
	}
	if resp.Error.Code != protocol.CodeInvalidParams {
		t.Fatalf("error code=%d want %d", resp.Error.Code, protocol.CodeInvalidParams)
	}
}

func TestRPCServer_TurnCreate_NonBootstrapThreadScope(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	bootstrapSess, bootstrapRun, err := implstore.CreateSession(cfg, "bootstrap", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession bootstrap: %v", err)
	}
	targetSess, targetRun, err := implstore.CreateSession(cfg, "target", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession target: %v", err)
	}
	sessStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore: %v", err)
	}
	_ = sessStore.SaveSession(context.Background(), bootstrapSess)
	_ = sessStore.SaveSession(context.Background(), targetSess)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: bootstrapRun, AllowAnyThread: true, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
	})

	req, _ := protocol.NewRequest("1", protocol.MethodTurnCreate, protocol.TurnCreateParams{
		ThreadID: protocol.ThreadID(targetRun.SessionID),
		Input:    &protocol.UserMessageContent{Text: "hello from target"},
	})
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error != nil {
		t.Fatalf("turn.create error: %+v", resp.Error)
	}
	var out protocol.TurnCreateResult
	_ = json.Unmarshal(resp.Result, &out)
	if got := strings.TrimSpace(string(out.Turn.ThreadID)); got != targetRun.SessionID {
		t.Fatalf("turn thread=%q want %q", got, targetRun.SessionID)
	}
	if got := strings.TrimSpace(string(out.Turn.RunID)); got != targetRun.RunID {
		t.Fatalf("turn run=%q want %q", got, targetRun.RunID)
	}
	task, err := ts.GetTask(context.Background(), string(out.Turn.ID))
	if err != nil {
		t.Fatalf("GetTask turn task: %v", err)
	}
	if strings.TrimSpace(task.SessionID) != targetRun.SessionID || strings.TrimSpace(task.RunID) != targetRun.RunID {
		t.Fatalf("task scope mismatch session=%q run=%q", task.SessionID, task.RunID)
	}
}

func TestRPCServer_TurnCancel_NonBootstrapTask(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	bootstrapSess, bootstrapRun, err := implstore.CreateSession(cfg, "bootstrap", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession bootstrap: %v", err)
	}
	targetSess, targetRun, err := implstore.CreateSession(cfg, "target", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession target: %v", err)
	}
	sessStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore: %v", err)
	}
	_ = sessStore.SaveSession(context.Background(), bootstrapSess)
	_ = sessStore.SaveSession(context.Background(), targetSess)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	now := time.Now().UTC()
	task := types.Task{
		TaskID:    "task-turn-cancel-target",
		SessionID: targetRun.SessionID,
		RunID:     targetRun.RunID,
		Goal:      "cancel me",
		Status:    types.TaskStatusPending,
		CreatedAt: &now,
	}
	if err := ts.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: bootstrapRun, AllowAnyThread: true, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
	})

	req, _ := protocol.NewRequest("1", protocol.MethodTurnCancel, protocol.TurnCancelParams{
		TurnID: protocol.TurnID(task.TaskID),
	})
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error != nil {
		t.Fatalf("turn.cancel error: %+v", resp.Error)
	}
	var out protocol.TurnCancelResult
	_ = json.Unmarshal(resp.Result, &out)
	if out.Turn.Status != protocol.TurnStatusCanceled {
		t.Fatalf("turn status=%q want canceled", out.Turn.Status)
	}
	updated, err := ts.GetTask(context.Background(), task.TaskID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if updated.Status != types.TaskStatusCanceled {
		t.Fatalf("task status=%q want canceled", updated.Status)
	}
}

func TestRPCServer_ResolveScope_RunOverride_PreservesThreadSession(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	bootstrapSess, bootstrapRun, err := implstore.CreateSession(cfg, "bootstrap", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession bootstrap: %v", err)
	}
	targetSess, targetRun, err := implstore.CreateSession(cfg, "target", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession target: %v", err)
	}
	sessStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore: %v", err)
	}
	_ = sessStore.SaveSession(context.Background(), bootstrapSess)
	_ = sessStore.SaveSession(context.Background(), targetSess)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: bootstrapRun, AllowAnyThread: true, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
	})

	scope, err := srv.resolveTeamOrRunScope(context.Background(), protocol.ThreadID(targetRun.SessionID), "", targetRun.RunID)
	if err != nil {
		t.Fatalf("resolveTeamOrRunScope: %v", err)
	}
	if got := strings.TrimSpace(scope.sessionID); got != targetRun.SessionID {
		t.Fatalf("scope.sessionID=%q want %q", got, targetRun.SessionID)
	}
	if got := strings.TrimSpace(scope.runID); got != targetRun.RunID {
		t.Fatalf("scope.runID=%q want %q", got, targetRun.RunID)
	}
}

func TestDaemonRPC_ControlSetProfile_Disabled(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	bootstrapSess, bootstrapRun, err := implstore.CreateSession(cfg, "bootstrap", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession bootstrap: %v", err)
	}
	sessStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore: %v", err)
	}
	_ = sessStore.SaveSession(context.Background(), bootstrapSess)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: bootstrapRun, AllowAnyThread: true, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
		ControlSetProfile: func(_ context.Context, _, _, _ string) ([]string, error) {
			return nil, &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "control.setProfile is disabled; use /new"}
		},
	})

	req, _ := protocol.NewRequest("1", protocol.MethodControlSetProfile, protocol.ControlSetProfileParams{
		ThreadID: protocol.ThreadID(bootstrapRun.SessionID),
		Profile:  "general",
	})
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error == nil {
		t.Fatalf("expected error for disabled control.setProfile")
	}
	if resp.Error.Code != protocol.CodeInvalidState {
		t.Fatalf("error code=%d want %d", resp.Error.Code, protocol.CodeInvalidState)
	}
}

func TestRPCServer_TaskListAndCreate_RunScopedOverrides(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess := types.NewSession("goal")
	runA := types.NewRun("goal", 8*1024, sess.SessionID)
	runB := types.NewRun("goal", 8*1024, sess.SessionID)
	sess.CurrentRunID = runA.RunID
	sess.Runs = []string{runA.RunID, runB.RunID}
	sessStore := store.NewMemorySessionStore()
	_ = sessStore.SaveSession(context.Background(), sess)
	_ = sessStore.SaveRun(context.Background(), runA)
	_ = sessStore.SaveRun(context.Background(), runB)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: runA, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
	})

	createReq, _ := protocol.NewRequest("1", protocol.MethodTaskCreate, protocol.TaskCreateParams{
		ThreadID: protocol.ThreadID(runA.SessionID),
		RunID:    runB.RunID,
		Goal:     "scoped to run b",
	})
	createResp := rpcRoundTrip(t, srv, createReq)
	if createResp.Error != nil {
		t.Fatalf("task.create error: %+v", createResp.Error)
	}
	var createRes protocol.TaskCreateResult
	_ = json.Unmarshal(createResp.Result, &createRes)
	if got := strings.TrimSpace(string(createRes.Task.RunID)); got != runB.RunID {
		t.Fatalf("created task runId=%q, want %q", got, runB.RunID)
	}

	inboxRunAReq, _ := protocol.NewRequest("2", protocol.MethodTaskList, protocol.TaskListParams{
		ThreadID: protocol.ThreadID(runA.SessionID),
		View:     "inbox",
		RunID:    runA.RunID,
	})
	inboxRunAResp := rpcRoundTrip(t, srv, inboxRunAReq)
	if inboxRunAResp.Error != nil {
		t.Fatalf("task.list runA error: %+v", inboxRunAResp.Error)
	}
	var inboxRunA protocol.TaskListResult
	_ = json.Unmarshal(inboxRunAResp.Result, &inboxRunA)
	for _, tk := range inboxRunA.Tasks {
		if strings.TrimSpace(tk.ID) == strings.TrimSpace(createRes.Task.ID) {
			t.Fatalf("task for runB unexpectedly returned in runA scope")
		}
	}

	inboxRunBReq, _ := protocol.NewRequest("3", protocol.MethodTaskList, protocol.TaskListParams{
		ThreadID: protocol.ThreadID(runA.SessionID),
		View:     "inbox",
		RunID:    runB.RunID,
	})
	inboxRunBResp := rpcRoundTrip(t, srv, inboxRunBReq)
	if inboxRunBResp.Error != nil {
		t.Fatalf("task.list runB error: %+v", inboxRunBResp.Error)
	}
	var inboxRunB protocol.TaskListResult
	_ = json.Unmarshal(inboxRunBResp.Result, &inboxRunB)
	found := false
	for _, tk := range inboxRunB.Tasks {
		if strings.TrimSpace(tk.ID) == strings.TrimSpace(createRes.Task.ID) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected scoped task in runB inbox")
	}
}

func TestRPCServer_ControlSetModel(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess := types.NewSession("goal")
	run := types.NewRun("goal", 8*1024, sess.SessionID)
	sessStore := store.NewMemorySessionStore()
	_ = sessStore.SaveSession(context.Background(), sess)
	_ = sessStore.SaveRun(context.Background(), run)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	seen := ""
	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: run, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
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

func TestRPCServer_ControlSetReasoning(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess := types.NewSession("goal")
	run := types.NewRun("goal", 8*1024, sess.SessionID)
	sessStore := store.NewMemorySessionStore()
	_ = sessStore.SaveSession(context.Background(), sess)
	_ = sessStore.SaveRun(context.Background(), run)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	seen := ""
	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: run, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
		ControlSetReasoning: func(_ context.Context, threadID, target, effort, summary string) ([]string, error) {
			seen = threadID + "|" + target + "|" + effort + "|" + summary
			return []string{"run-1"}, nil
		},
	})

	req, _ := protocol.NewRequest("1", protocol.MethodControlSetReasoning, protocol.ControlSetReasoningParams{
		ThreadID: protocol.ThreadID(run.SessionID),
		Effort:   "medium",
		Summary:  "none",
		Target:   "run-1",
	})
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error != nil {
		t.Fatalf("control.setReasoning error: %+v", resp.Error)
	}
	if seen != run.SessionID+"|run-1|medium|off" {
		t.Fatalf("unexpected callback payload: %q", seen)
	}
	var res protocol.ControlSetReasoningResult
	_ = json.Unmarshal(resp.Result, &res)
	if !res.Accepted || len(res.AppliedTo) != 1 || res.AppliedTo[0] != "run-1" || res.Effort != "medium" || res.Summary != "off" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestRPCServer_ControlSetReasoning_InvalidParams(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess := types.NewSession("goal")
	run := types.NewRun("goal", 8*1024, sess.SessionID)
	sessStore := store.NewMemorySessionStore()
	_ = sessStore.SaveSession(context.Background(), sess)
	_ = sessStore.SaveRun(context.Background(), run)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: run, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
		ControlSetReasoning: func(_ context.Context, _, _, _, _ string) ([]string, error) {
			t.Fatalf("callback should not be called")
			return nil, nil
		},
	})

	reqMissing, _ := protocol.NewRequest("1", protocol.MethodControlSetReasoning, protocol.ControlSetReasoningParams{
		ThreadID: protocol.ThreadID(run.SessionID),
	})
	respMissing := rpcRoundTrip(t, srv, reqMissing)
	if respMissing.Error == nil || respMissing.Error.Code != protocol.CodeInvalidParams {
		t.Fatalf("expected invalid params for missing reasoning args, got %+v", respMissing.Error)
	}

	reqInvalid, _ := protocol.NewRequest("2", protocol.MethodControlSetReasoning, protocol.ControlSetReasoningParams{
		ThreadID: protocol.ThreadID(run.SessionID),
		Summary:  "verbose",
	})
	respInvalid := rpcRoundTrip(t, srv, reqInvalid)
	if respInvalid.Error == nil || respInvalid.Error.Code != protocol.CodeInvalidParams {
		t.Fatalf("expected invalid params for bad summary, got %+v", respInvalid.Error)
	}
}

func TestRPCServer_ControlSetProfile_ThreadMismatch(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess := types.NewSession("goal")
	run := types.NewRun("goal", 8*1024, sess.SessionID)
	sessStore := store.NewMemorySessionStore()
	_ = sessStore.SaveSession(context.Background(), sess)
	_ = sessStore.SaveRun(context.Background(), run)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: run, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
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

func TestRPCServer_ControlSetProfile_PreservesSessionContext(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess := types.NewSession("goal")
	run := types.NewRun("goal", 8*1024, sess.SessionID)
	sessStore := store.NewMemorySessionStore()
	_ = sessStore.SaveSession(context.Background(), sess)
	_ = sessStore.SaveRun(context.Background(), run)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))

	seen := ""
	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: run, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
		ControlSetProfile: func(_ context.Context, threadID, target, profile string) ([]string, error) {
			seen = threadID + "|" + target + "|" + profile
			return []string{run.RunID}, nil
		},
	})

	req, _ := protocol.NewRequest("1", protocol.MethodControlSetProfile, protocol.ControlSetProfileParams{
		ThreadID: protocol.ThreadID(run.SessionID),
		Profile:  "software_dev",
	})
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error != nil {
		t.Fatalf("control.setProfile error: %+v", resp.Error)
	}
	if seen != run.SessionID+"||software_dev" {
		t.Fatalf("unexpected callback payload: %q", seen)
	}
	var res protocol.ControlSetProfileResult
	_ = json.Unmarshal(resp.Result, &res)
	if !res.Accepted || !res.PreservesSessionContext || len(res.AppliedTo) != 1 || res.AppliedTo[0] != run.RunID {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestRPCServer_SessionGetTotals_AndActivityList(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess := types.NewSession("goal")
	sess.InputTokens = 10
	sess.OutputTokens = 20
	sess.TotalTokens = 999 // intentionally inconsistent; RPC should normalize to in+out
	sess.CostUSD = 1.5
	run := types.NewRun("goal", 8*1024, sess.SessionID)
	sessStore := store.NewMemorySessionStore()
	_ = sessStore.SaveSession(context.Background(), sess)
	_ = sessStore.SaveRun(context.Background(), run)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))

	now := time.Now().UTC()
	_ = ts.CreateTask(context.Background(), types.Task{
		TaskID: "task-a", SessionID: run.SessionID, RunID: run.RunID, Goal: "A",
		Status: types.TaskStatusSucceeded, CreatedAt: &now, CompletedAt: &now,
	})

	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: run, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
	})

	reqTotals, _ := protocol.NewRequest("1", protocol.MethodSessionGetTotals, protocol.SessionGetTotalsParams{
		ThreadID: protocol.ThreadID(run.SessionID),
		RunID:    run.RunID,
	})
	respTotals := rpcRoundTrip(t, srv, reqTotals)
	if respTotals.Error != nil {
		t.Fatalf("session.getTotals error: %+v", respTotals.Error)
	}
	var totals protocol.SessionGetTotalsResult
	_ = json.Unmarshal(respTotals.Result, &totals)
	if totals.TotalTokensIn != 10 || totals.TotalTokensOut != 20 || totals.TotalTokens != 30 || totals.TotalCostUSD != 1.5 {
		t.Fatalf("unexpected totals: %+v", totals)
	}

	reqActs, _ := protocol.NewRequest("2", protocol.MethodActivityList, protocol.ActivityListParams{
		ThreadID: protocol.ThreadID(run.SessionID),
		RunID:    run.RunID,
		Limit:    10,
	})
	respActs := rpcRoundTrip(t, srv, reqActs)
	if respActs.Error != nil {
		t.Fatalf("activity.list error: %+v", respActs.Error)
	}
}

func TestRPCServer_SessionGetTotals_TeamScopeIncludesTokenBreakdown(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	bootstrapSess, bootstrapRun, err := implstore.CreateSession(cfg, "bootstrap", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession bootstrap: %v", err)
	}
	sessA, runA, err := implstore.CreateSession(cfg, "team-a", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession team-a: %v", err)
	}
	sessB, runB, err := implstore.CreateSession(cfg, "team-b", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession team-b: %v", err)
	}
	sessA.InputTokens, sessA.OutputTokens, sessA.TotalTokens, sessA.CostUSD = 11, 7, 18, 0.03
	sessB.InputTokens, sessB.OutputTokens, sessB.TotalTokens, sessB.CostUSD = 13, 5, 18, 0.02

	sessStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore: %v", err)
	}
	if err := sessStore.SaveSession(context.Background(), bootstrapSess); err != nil {
		t.Fatalf("SaveSession bootstrap: %v", err)
	}
	if err := sessStore.SaveSession(context.Background(), sessA); err != nil {
		t.Fatalf("SaveSession A: %v", err)
	}
	if err := sessStore.SaveSession(context.Background(), sessB); err != nil {
		t.Fatalf("SaveSession B: %v", err)
	}

	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	now := time.Now().UTC()
	teamID := "team-1"
	_ = ts.CreateTask(context.Background(), types.Task{
		TaskID: "task-a", SessionID: runA.SessionID, RunID: runA.RunID, TeamID: teamID, AssignedRole: "role-a",
		Goal: "A", Status: types.TaskStatusPending, CreatedAt: &now,
	})
	_ = ts.CreateTask(context.Background(), types.Task{
		TaskID: "task-b", SessionID: runB.SessionID, RunID: runB.RunID, TeamID: teamID, AssignedRole: "role-b",
		Goal: "B", Status: types.TaskStatusPending, CreatedAt: &now,
	})
	_ = ts.CompleteTask(context.Background(), "task-a", types.TaskResult{
		TaskID:      "task-a",
		Status:      types.TaskStatusSucceeded,
		TotalTokens: 18,
		CostUSD:     0.03,
		CompletedAt: &now,
	})
	_ = ts.CompleteTask(context.Background(), "task-b", types.TaskResult{
		TaskID:      "task-b",
		Status:      types.TaskStatusSucceeded,
		TotalTokens: 18,
		CostUSD:     0.02,
		CompletedAt: &now,
	})

	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: bootstrapRun, AllowAnyThread: true, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
	})
	req, _ := protocol.NewRequest("1", protocol.MethodSessionGetTotals, protocol.SessionGetTotalsParams{
		ThreadID: protocol.ThreadID(bootstrapRun.SessionID),
		TeamID:   teamID,
	})
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error != nil {
		t.Fatalf("session.getTotals(team) error: %+v", resp.Error)
	}
	var out protocol.SessionGetTotalsResult
	_ = json.Unmarshal(resp.Result, &out)
	if out.TotalTokensIn != 24 || out.TotalTokensOut != 12 || out.TotalTokens != 36 {
		t.Fatalf("unexpected team totals token breakdown: %+v", out)
	}
	if math.Abs(out.TotalCostUSD-0.05) > 1e-9 {
		t.Fatalf("unexpected team totals cost: %+v", out)
	}
}

func TestRPCServer_SessionStart_Standalone(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess := types.NewSession("goal")
	run := types.NewRun("goal", 8*1024, sess.SessionID)
	sessStore := store.NewMemorySessionStore()
	_ = sessStore.SaveSession(context.Background(), sess)
	_ = sessStore.SaveRun(context.Background(), run)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))

	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: run, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
	})

	req, _ := protocol.NewRequest("1", protocol.MethodSessionStart, protocol.SessionStartParams{
		ThreadID: protocol.ThreadID(run.SessionID),
		Mode:     "standalone",
		Goal:     "fresh start",
		Profile:  "general",
	})
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error != nil {
		t.Fatalf("session.start error: %+v", resp.Error)
	}
	var out protocol.SessionStartResult
	_ = json.Unmarshal(resp.Result, &out)
	if strings.TrimSpace(out.SessionID) == "" || strings.TrimSpace(out.PrimaryRunID) == "" {
		t.Fatalf("missing ids: %+v", out)
	}
	if out.Mode != "standalone" {
		t.Fatalf("mode = %q, want standalone", out.Mode)
	}
	if strings.TrimSpace(out.Model) != "" {
		t.Fatalf("model = %q, want empty when no explicit model provided", out.Model)
	}
	if got, err := sessStore.LoadSession(context.Background(), out.SessionID); err != nil {
		t.Fatalf("load created session: %v", err)
	} else if strings.TrimSpace(got.ActiveModel) != "" {
		t.Fatalf("created session active model = %q, want empty", got.ActiveModel)
	}
}

func TestRPCServer_SessionStart_InvalidMode(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess := types.NewSession("goal")
	run := types.NewRun("goal", 8*1024, sess.SessionID)
	sessStore := store.NewMemorySessionStore()
	_ = sessStore.SaveSession(context.Background(), sess)
	_ = sessStore.SaveRun(context.Background(), run)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))

	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: run, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
	})
	req, _ := protocol.NewRequest("1", protocol.MethodSessionStart, protocol.SessionStartParams{
		ThreadID: protocol.ThreadID(run.SessionID),
		Mode:     "invalid",
	})
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error == nil {
		t.Fatalf("expected invalid mode error")
	}
	if resp.Error.Code != protocol.CodeInvalidParams {
		t.Fatalf("code = %d want %d", resp.Error.Code, protocol.CodeInvalidParams)
	}
}

func TestRPCServer_SessionStart_Team(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	if err := os.MkdirAll(filepath.Join(cfg.DataDir, "profiles", "startup_team"), 0o755); err != nil {
		t.Fatalf("mkdir profiles: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.DataDir, "profiles", "startup_team", "profile.yaml"), []byte(
		"id: startup_team\ndescription: Team\nteam:\n  model: openai/gpt-5-mini\n  roles:\n    - name: ceo\n      coordinator: true\n      description: Lead\n      prompts:\n        system_prompt: lead\n    - name: cto\n      description: Build\n      prompts:\n        system_prompt: build\n",
	), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}

	sess := types.NewSession("goal")
	run := types.NewRun("goal", 8*1024, sess.SessionID)
	sessStore := store.NewMemorySessionStore()
	_ = sessStore.SaveSession(context.Background(), sess)
	_ = sessStore.SaveRun(context.Background(), run)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: run, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
	})

	req, _ := protocol.NewRequest("1", protocol.MethodSessionStart, protocol.SessionStartParams{
		ThreadID: protocol.ThreadID(run.SessionID),
		Mode:     "team",
		Profile:  "startup_team",
	})
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error != nil {
		t.Fatalf("session.start(team) error: %+v", resp.Error)
	}
	var out protocol.SessionStartResult
	_ = json.Unmarshal(resp.Result, &out)
	if out.Mode != "team" {
		t.Fatalf("mode=%q want team", out.Mode)
	}
	if strings.TrimSpace(out.TeamID) == "" {
		t.Fatalf("expected team id, got %+v", out)
	}
	if len(out.RunIDs) != 3 {
		t.Fatalf("expected 3 role runs (including injected reviewer), got %d", len(out.RunIDs))
	}
	if _, err := os.Stat(filepath.Join(fsutil.GetTeamDir(cfg.DataDir, out.TeamID), "team.json")); err != nil {
		t.Fatalf("expected team manifest: %v", err)
	}
	createdSess, err := sessStore.LoadSession(context.Background(), out.SessionID)
	if err != nil {
		t.Fatalf("LoadSession(team): %v", err)
	}
	if got := strings.TrimSpace(createdSess.Mode); got != "team" {
		t.Fatalf("session mode=%q want team", got)
	}
	if got := strings.TrimSpace(createdSess.TeamID); got != strings.TrimSpace(out.TeamID) {
		t.Fatalf("session teamID=%q want %q", got, out.TeamID)
	}
	if got := strings.TrimSpace(createdSess.Profile); got != "startup_team" {
		t.Fatalf("session profile=%q want startup_team", got)
	}
	firstRun, err := sessStore.LoadRun(context.Background(), out.RunIDs[0])
	if err != nil {
		t.Fatalf("LoadRun(team role): %v", err)
	}
	if firstRun.Runtime == nil {
		t.Fatalf("expected runtime metadata for team run")
	}
	if got := strings.TrimSpace(firstRun.Runtime.TeamID); got != strings.TrimSpace(out.TeamID) {
		t.Fatalf("run runtime teamID=%q want %q", got, out.TeamID)
	}
	if strings.TrimSpace(firstRun.Runtime.Role) == "" {
		t.Fatalf("expected run runtime role to be set")
	}
}

func TestRPCServer_SessionStart_StandaloneRejectsTeamProfile(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	if err := os.MkdirAll(filepath.Join(cfg.DataDir, "profiles", "startup_team"), 0o755); err != nil {
		t.Fatalf("mkdir profiles: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.DataDir, "profiles", "startup_team", "profile.yaml"), []byte(
		"id: startup_team\ndescription: Team\nteam:\n  model: openai/gpt-5-mini\n  roles:\n    - name: ceo\n      coordinator: true\n      description: Lead\n      prompts:\n        system_prompt: lead\n",
	), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	sess := types.NewSession("goal")
	run := types.NewRun("goal", 8*1024, sess.SessionID)
	sessStore := store.NewMemorySessionStore()
	_ = sessStore.SaveSession(context.Background(), sess)
	_ = sessStore.SaveRun(context.Background(), run)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: run, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
	})

	req, _ := protocol.NewRequest("1", protocol.MethodSessionStart, protocol.SessionStartParams{
		ThreadID: protocol.ThreadID(run.SessionID),
		Mode:     "standalone",
		Profile:  "startup_team",
	})
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error == nil {
		t.Fatalf("expected standalone rejection for team profile")
	}
	if resp.Error.Code != protocol.CodeInvalidParams {
		t.Fatalf("code = %d want %d", resp.Error.Code, protocol.CodeInvalidParams)
	}
	if !strings.Contains(strings.ToLower(resp.Error.Message), "non-team profile") {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
}

func TestRPCServer_SessionList_And_AgentList(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sessA, runA, err := implstore.CreateSession(cfg, "alpha goal", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession A: %v", err)
	}
	sessB, runB, err := implstore.CreateSession(cfg, "beta goal", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession B: %v", err)
	}
	runBLoaded, err := implstore.LoadRun(cfg, runB.RunID)
	if err != nil {
		t.Fatalf("LoadRun B: %v", err)
	}
	if runBLoaded.Runtime == nil {
		runBLoaded.Runtime = &types.RunRuntimeConfig{}
	}
	runBLoaded.Runtime.Profile = "market_researcher"
	runBLoaded.Runtime.TeamID = "team-1"
	runBLoaded.Runtime.Role = "cto"
	if err := implstore.SaveRun(cfg, runBLoaded); err != nil {
		t.Fatalf("SaveRun B: %v", err)
	}
	sessStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore: %v", err)
	}
	bootstrap, _, err := implstore.CreateSession(cfg, "autonomous agent", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession bootstrap: %v", err)
	}
	bootstrap.System = true
	if err := sessStore.SaveSession(context.Background(), bootstrap); err != nil {
		t.Fatalf("SaveSession bootstrap: %v", err)
	}
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	now := time.Now().UTC()
	if err := ts.CreateTask(context.Background(), types.Task{
		TaskID:       "task-role-inference",
		SessionID:    sessB.SessionID,
		RunID:        runB.RunID,
		TeamID:       "team-1",
		AssignedRole: "cto",
		RoleSnapshot: "cto",
		Goal:         "role task",
		Status:       types.TaskStatusPending,
		CreatedAt:    &now,
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	sessionSvc := newTestSessionService(cfg, sessStore)
	taskMgr := pkgtask.NewManager(ts, sessionSvc)
	agentMgr := pkgagent.NewManager(sessionSvc, taskMgr, taskMgr)
	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: runA, TaskService: taskMgr, Session: sessionSvc, AgentService: agentMgr, Index: protocol.NewIndex(0, 0),
	})

	reqList, _ := protocol.NewRequest("1", protocol.MethodSessionList, protocol.SessionListParams{
		ThreadID:      protocol.ThreadID(runA.SessionID),
		TitleContains: "goal",
		Limit:         10,
	})
	respList := rpcRoundTrip(t, srv, reqList)
	if respList.Error != nil {
		t.Fatalf("session.list error: %+v", respList.Error)
	}
	var listed protocol.SessionListResult
	_ = json.Unmarshal(respList.Result, &listed)
	if listed.TotalCount < 2 {
		t.Fatalf("expected at least 2 sessions, got %+v", listed)
	}
	for _, s := range listed.Sessions {
		if strings.TrimSpace(s.SessionID) == strings.TrimSpace(bootstrap.SessionID) {
			t.Fatalf("system/bootstrap session should be excluded from session.list")
		}
	}
	if len(listed.Sessions) == 0 {
		t.Fatalf("expected non-empty sessions page when totalCount=%d", listed.TotalCount)
	}
	for _, s := range listed.Sessions {
		if strings.TrimSpace(s.SessionID) == "" {
			t.Fatalf("expected sessionId on session.list item, got %+v", s)
		}
	}
	foundSessB := false
	for _, s := range listed.Sessions {
		if strings.TrimSpace(s.SessionID) != strings.TrimSpace(sessB.SessionID) {
			continue
		}
		foundSessB = true
		if got := strings.TrimSpace(s.Mode); got != "team" {
			t.Fatalf("session %s mode=%q want team", s.SessionID, got)
		}
		if got := strings.TrimSpace(s.TeamID); got != "team-1" {
			t.Fatalf("session %s teamID=%q want team-1", s.SessionID, got)
		}
		if got := strings.TrimSpace(s.Profile); got != "market_researcher" {
			t.Fatalf("session %s profile=%q want market_researcher", s.SessionID, got)
		}
	}
	if !foundSessB {
		t.Fatalf("expected session %s in session.list", sessB.SessionID)
	}

	reqAgents, _ := protocol.NewRequest("2", protocol.MethodAgentList, protocol.AgentListParams{
		ThreadID:  protocol.ThreadID(runA.SessionID),
		SessionID: sessB.SessionID,
	})
	respAgents := rpcRoundTrip(t, srv, reqAgents)
	if respAgents.Error != nil {
		t.Fatalf("agent.list error: %+v", respAgents.Error)
	}
	var agents protocol.AgentListResult
	_ = json.Unmarshal(respAgents.Result, &agents)
	found := false
	for _, a := range agents.Agents {
		if strings.TrimSpace(a.RunID) == strings.TrimSpace(runB.RunID) && strings.TrimSpace(a.SessionID) == strings.TrimSpace(sessB.SessionID) {
			if got := strings.TrimSpace(a.Role); got != "cto" {
				t.Fatalf("expected role inference %q, got %q", "cto", got)
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected run %s in agent.list, got %+v", runB.RunID, agents.Agents)
	}

	_ = sessA // keep lint happy for created baseline session
}

func TestRPCServer_AgentPauseResume(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess, run, err := implstore.CreateSession(cfg, "pauseable", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	sessStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore: %v", err)
	}
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	now := time.Now().UTC()
	if err := ts.CreateTask(context.Background(), types.Task{
		TaskID:         "task-active",
		SessionID:      run.SessionID,
		RunID:          run.RunID,
		AssignedToType: "agent",
		AssignedTo:     run.RunID,
		Goal:           "active work",
		Status:         types.TaskStatusActive,
		CreatedAt:      &now,
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	sessionSvc := newTestSessionService(cfg, sessStore)
	taskMgr := pkgtask.NewManager(ts, sessionSvc)
	agentMgr := pkgagent.NewManager(sessionSvc, taskMgr, taskMgr)
	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: run, TaskService: taskMgr, Session: sessionSvc, AgentService: agentMgr, Index: protocol.NewIndex(0, 0),
	})

	reqPause, _ := protocol.NewRequest("1", protocol.MethodAgentPause, protocol.AgentPauseParams{
		ThreadID: protocol.ThreadID(run.SessionID),
		RunID:    run.RunID,
	})
	respPause := rpcRoundTrip(t, srv, reqPause)
	if respPause.Error != nil {
		t.Fatalf("agent.pause error: %+v", respPause.Error)
	}
	loaded, err := implstore.LoadRun(cfg, run.RunID)
	if err != nil {
		t.Fatalf("LoadRun pause: %v", err)
	}
	if loaded.Status != types.RunStatusPaused {
		t.Fatalf("status after pause=%q want %q", loaded.Status, types.RunStatusPaused)
	}
	pausedTask, err := ts.GetTask(context.Background(), "task-active")
	if err != nil {
		t.Fatalf("GetTask pause: %v", err)
	}
	if pausedTask.Status != types.TaskStatusCanceled {
		t.Fatalf("task status after pause=%q want %q", pausedTask.Status, types.TaskStatusCanceled)
	}

	reqResume, _ := protocol.NewRequest("2", protocol.MethodAgentResume, protocol.AgentResumeParams{
		ThreadID: protocol.ThreadID(run.SessionID),
		RunID:    run.RunID,
	})
	respResume := rpcRoundTrip(t, srv, reqResume)
	if respResume.Error != nil {
		t.Fatalf("agent.resume error: %+v", respResume.Error)
	}
	loaded, err = implstore.LoadRun(cfg, run.RunID)
	if err != nil {
		t.Fatalf("LoadRun resume: %v", err)
	}
	if loaded.Status != types.RunStatusRunning {
		t.Fatalf("status after resume=%q want %q", loaded.Status, types.RunStatusRunning)
	}
	_ = sess
}

func TestRPCServer_SessionPauseResume_AffectsAllRuns(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess, runA, err := implstore.CreateSession(cfg, "session pause", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	runB := types.NewRun("secondary", 8*1024, sess.SessionID)
	if err := implstore.SaveRun(cfg, runB); err != nil {
		t.Fatalf("SaveRun runB: %v", err)
	}
	sess.Runs = append(sess.Runs, runB.RunID)
	sess.CurrentRunID = runA.RunID
	sessStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore: %v", err)
	}
	if err := sessStore.SaveSession(context.Background(), sess); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	sessionSvc := newTestSessionService(cfg, sessStore)
	taskMgr := pkgtask.NewManager(ts, sessionSvc)
	agentMgr := pkgagent.NewManager(sessionSvc, taskMgr, taskMgr)
	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: runA, TaskService: taskMgr, Session: sessionSvc, AgentService: agentMgr, Index: protocol.NewIndex(0, 0),
	})

	reqPause, _ := protocol.NewRequest("1", protocol.MethodSessionPause, protocol.SessionPauseParams{
		ThreadID:  protocol.ThreadID(runA.SessionID),
		SessionID: sess.SessionID,
	})
	respPause := rpcRoundTrip(t, srv, reqPause)
	if respPause.Error != nil {
		t.Fatalf("session.pause error: %+v", respPause.Error)
	}
	var pauseRes protocol.SessionPauseResult
	if err := json.Unmarshal(respPause.Result, &pauseRes); err != nil {
		t.Fatalf("unmarshal pause result: %v", err)
	}
	if len(pauseRes.AffectedRunIDs) < 2 {
		t.Fatalf("expected >=2 affected runs, got %v", pauseRes.AffectedRunIDs)
	}

	for _, runID := range []string{runA.RunID, runB.RunID} {
		loaded, err := implstore.LoadRun(cfg, runID)
		if err != nil {
			t.Fatalf("LoadRun pause (%s): %v", runID, err)
		}
		if loaded.Status != types.RunStatusPaused {
			t.Fatalf("status after session pause for %s = %q want %q", runID, loaded.Status, types.RunStatusPaused)
		}
	}

	reqResume, _ := protocol.NewRequest("2", protocol.MethodSessionResume, protocol.SessionResumeParams{
		ThreadID:  protocol.ThreadID(runA.SessionID),
		SessionID: sess.SessionID,
	})
	respResume := rpcRoundTrip(t, srv, reqResume)
	if respResume.Error != nil {
		t.Fatalf("session.resume error: %+v", respResume.Error)
	}
	for _, runID := range []string{runA.RunID, runB.RunID} {
		loaded, err := implstore.LoadRun(cfg, runID)
		if err != nil {
			t.Fatalf("LoadRun resume (%s): %v", runID, err)
		}
		if loaded.Status != types.RunStatusRunning {
			t.Fatalf("status after session resume for %s = %q want %q", runID, loaded.Status, types.RunStatusRunning)
		}
	}
}

func TestRPCServer_SessionStop_DefaultSessionFromThread(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess, runA, err := implstore.CreateSession(cfg, "session stop", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	runB := types.NewRun("secondary", 8*1024, sess.SessionID)
	if err := implstore.SaveRun(cfg, runB); err != nil {
		t.Fatalf("SaveRun runB: %v", err)
	}
	sess.Runs = append(sess.Runs, runB.RunID)
	sess.CurrentRunID = runA.RunID
	sessStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore: %v", err)
	}
	if err := sessStore.SaveSession(context.Background(), sess); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	sessionSvc := newTestSessionService(cfg, sessStore)
	taskMgr := pkgtask.NewManager(ts, sessionSvc)
	agentMgr := pkgagent.NewManager(sessionSvc, taskMgr, taskMgr)
	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: runA, TaskService: taskMgr, Session: sessionSvc, AgentService: agentMgr, Index: protocol.NewIndex(0, 0),
	})

	reqStop, _ := protocol.NewRequest("1", protocol.MethodSessionStop, protocol.SessionStopParams{
		ThreadID: protocol.ThreadID(runA.SessionID),
	})
	respStop := rpcRoundTrip(t, srv, reqStop)
	if respStop.Error != nil {
		t.Fatalf("session.stop error: %+v", respStop.Error)
	}
	var stopRes protocol.SessionStopResult
	if err := json.Unmarshal(respStop.Result, &stopRes); err != nil {
		t.Fatalf("unmarshal stop result: %v", err)
	}
	if got := strings.TrimSpace(stopRes.SessionID); got != runA.SessionID {
		t.Fatalf("session.stop sessionId=%q want %q", got, runA.SessionID)
	}
	for _, runID := range []string{runA.RunID, runB.RunID} {
		loaded, err := implstore.LoadRun(cfg, runID)
		if err != nil {
			t.Fatalf("LoadRun stop (%s): %v", runID, err)
		}
		if loaded.Status != types.RunStatusCanceled {
			t.Fatalf("status after session stop for %s = %q want %q", runID, loaded.Status, types.RunStatusCanceled)
		}
	}
}

func TestRPCServer_SessionStop_ThreadMismatch(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	_, runA, err := implstore.CreateSession(cfg, "session stop mismatch", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	otherSess, _, err := implstore.CreateSession(cfg, "other", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession other: %v", err)
	}
	sessStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore: %v", err)
	}
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	sessionSvc := newTestSessionService(cfg, sessStore)
	taskMgr := pkgtask.NewManager(ts, sessionSvc)
	agentMgr := pkgagent.NewManager(sessionSvc, taskMgr, taskMgr)
	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: runA, TaskService: taskMgr, Session: sessionSvc, AgentService: agentMgr, Index: protocol.NewIndex(0, 0),
	})

	reqStop, _ := protocol.NewRequest("1", protocol.MethodSessionStop, protocol.SessionStopParams{
		ThreadID:  protocol.ThreadID(runA.SessionID),
		SessionID: otherSess.SessionID,
	})
	respStop := rpcRoundTrip(t, srv, reqStop)
	if respStop.Error == nil {
		t.Fatalf("expected error for thread/session mismatch")
	}
	if respStop.Error.Code != protocol.CodeThreadNotFound {
		t.Fatalf("error code=%d want %d", respStop.Error.Code, protocol.CodeThreadNotFound)
	}
}

func TestRPCServer_SessionClearHistory_StandaloneSession(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess, runA, err := implstore.CreateSession(cfg, "session clear", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	runB := types.NewRun("secondary", 8*1024, sess.SessionID)
	if err := implstore.SaveRun(cfg, runB); err != nil {
		t.Fatalf("SaveRun runB: %v", err)
	}
	if _, err := implstore.AddRunToSession(cfg, sess.SessionID, runB.RunID); err != nil {
		t.Fatalf("AddRunToSession runB: %v", err)
	}
	historyStore, err := implstore.NewSQLiteHistoryStore(cfg, sess.SessionID)
	if err != nil {
		t.Fatalf("NewSQLiteHistoryStore: %v", err)
	}
	for _, runID := range []string{runA.RunID, runB.RunID} {
		if err := implstore.AppendEvent(context.Background(), cfg, types.EventRecord{
			RunID:   runID,
			Type:    "agent.op.request",
			Message: "msg",
			Data:    map[string]string{"id": "x"},
		}); err != nil {
			t.Fatalf("append events: %v", err)
		}
		line := fmt.Sprintf(`{"id":"hist-%s","ts":"%s","runId":"%s","origin":"agent","kind":"assistant","message":"hello"}`,
			runID, time.Now().UTC().Format(time.RFC3339Nano), runID)
		if err := historyStore.AppendLine(context.Background(), []byte(line)); err != nil {
			t.Fatalf("append history: %v", err)
		}
	}
	sessStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore: %v", err)
	}
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	sessionSvc := newTestSessionService(cfg, sessStore)
	taskMgr := pkgtask.NewManager(ts, sessionSvc)
	agentMgr := pkgagent.NewManager(sessionSvc, taskMgr, taskMgr)
	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: runA, TaskService: taskMgr, Session: sessionSvc, AgentService: agentMgr, Index: protocol.NewIndex(0, 0),
	})

	req, _ := protocol.NewRequest("1", protocol.MethodSessionClearHistory, protocol.SessionClearHistoryParams{
		ThreadID:  protocol.ThreadID(runA.SessionID),
		SessionID: sess.SessionID,
	})
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error != nil {
		t.Fatalf("session.clearHistory error: %+v", resp.Error)
	}
	var out protocol.SessionClearHistoryResult
	if err := json.Unmarshal(resp.Result, &out); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if got := strings.TrimSpace(out.SessionID); got != sess.SessionID {
		t.Fatalf("sessionId=%q want %q", got, sess.SessionID)
	}
	if len(out.SourceRuns) != 2 {
		t.Fatalf("expected 2 source runs, got %d", len(out.SourceRuns))
	}
	if out.EventsDeleted == 0 || out.HistoryDeleted == 0 {
		t.Fatalf("expected deleted counts, got %+v", out)
	}
}

func TestRPCServer_SessionClearHistory_TeamUsesManifestRuns(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	coordinatorSession, coordinatorRun, err := implstore.CreateSession(cfg, "team coordinator", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession coordinator: %v", err)
	}
	roleSession, roleRun, err := implstore.CreateSession(cfg, "team role", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession role: %v", err)
	}
	teamID := "team-test"
	coordinatorSession.Mode = "team"
	coordinatorSession.TeamID = teamID
	coordinatorSession.Profile = "team-profile"
	coordinatorSession.Runs = []string{coordinatorRun.RunID}
	sessStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore: %v", err)
	}
	if err := sessStore.SaveSession(context.Background(), coordinatorSession); err != nil {
		t.Fatalf("SaveSession coordinator: %v", err)
	}
	roleSessLoaded, err := sessStore.LoadSession(context.Background(), roleSession.SessionID)
	if err != nil {
		t.Fatalf("LoadSession role: %v", err)
	}
	roleSessLoaded.Mode = "team"
	roleSessLoaded.TeamID = teamID
	if err := sessStore.SaveSession(context.Background(), roleSessLoaded); err != nil {
		t.Fatalf("SaveSession role: %v", err)
	}
	teamDir := fsutil.GetTeamDir(cfg.DataDir, teamID)
	if err := os.MkdirAll(teamDir, 0o755); err != nil {
		t.Fatalf("mkdir team dir: %v", err)
	}
	manifest := fmt.Sprintf(`{"teamId":"%s","profileId":"p1","coordinatorRole":"ceo","coordinatorRunId":"%s","roles":[{"roleName":"ceo","runId":"%s","sessionId":"%s"},{"roleName":"cto","runId":"%s","sessionId":"%s"}],"createdAt":"2026-01-01T00:00:00Z"}`,
		teamID, coordinatorRun.RunID, coordinatorRun.RunID, coordinatorSession.SessionID, roleRun.RunID, roleSession.SessionID)
	if err := os.WriteFile(filepath.Join(teamDir, "team.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write team manifest: %v", err)
	}
	historyBySession := map[string]*implstore.SQLiteHistoryStore{}
	for _, runID := range []string{coordinatorRun.RunID, roleRun.RunID} {
		sessionID := coordinatorSession.SessionID
		if runID == roleRun.RunID {
			sessionID = roleSession.SessionID
		}
		if err := implstore.AppendEvent(context.Background(), cfg, types.EventRecord{
			RunID:   runID,
			Type:    "agent.op.request",
			Message: "msg",
			Data:    map[string]string{"id": "x"},
		}); err != nil {
			t.Fatalf("append events: %v", err)
		}
		hs := historyBySession[sessionID]
		if hs == nil {
			created, herr := implstore.NewSQLiteHistoryStore(cfg, sessionID)
			if herr != nil {
				t.Fatalf("NewSQLiteHistoryStore: %v", herr)
			}
			hs = created
			historyBySession[sessionID] = hs
		}
		line := fmt.Sprintf(`{"id":"hist-%s","ts":"%s","runId":"%s","origin":"agent","kind":"assistant","message":"hello"}`,
			runID, time.Now().UTC().Format(time.RFC3339Nano), runID)
		if err := hs.AppendLine(context.Background(), []byte(line)); err != nil {
			t.Fatalf("append history: %v", err)
		}
	}
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	sessionSvc := newTestSessionService(cfg, sessStore)
	taskMgr := pkgtask.NewManager(ts, sessionSvc)
	agentMgr := pkgagent.NewManager(sessionSvc, taskMgr, taskMgr)
	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: coordinatorRun, TaskService: taskMgr, Session: sessionSvc, AgentService: agentMgr, Index: protocol.NewIndex(0, 0),
	})
	req, _ := protocol.NewRequest("1", protocol.MethodSessionClearHistory, protocol.SessionClearHistoryParams{
		ThreadID: protocol.ThreadID(coordinatorSession.SessionID),
		TeamID:   teamID,
	})
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error != nil {
		t.Fatalf("session.clearHistory(team) error: %+v", resp.Error)
	}
	var out protocol.SessionClearHistoryResult
	if err := json.Unmarshal(resp.Result, &out); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if got := strings.TrimSpace(out.TeamID); got != teamID {
		t.Fatalf("teamId=%q want %q", got, teamID)
	}
	if len(out.SourceRuns) != 2 {
		t.Fatalf("expected 2 source runs, got %d", len(out.SourceRuns))
	}
	if out.EventsDeleted == 0 || out.HistoryDeleted == 0 {
		t.Fatalf("expected deleted counts, got %+v", out)
	}
}

func TestRPCServer_SessionList_IncludesPausedCounts(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess, runA, err := implstore.CreateSession(cfg, "counts", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	runB := types.NewRun("paused", 8*1024, sess.SessionID)
	runB.Status = types.RunStatusPaused
	if err := implstore.SaveRun(cfg, runB); err != nil {
		t.Fatalf("SaveRun runB: %v", err)
	}
	runC := types.NewRun("done", 8*1024, sess.SessionID)
	runC.Status = types.RunStatusSucceeded
	if err := implstore.SaveRun(cfg, runC); err != nil {
		t.Fatalf("SaveRun runC: %v", err)
	}
	sess.Runs = append(sess.Runs, runB.RunID, runC.RunID)
	sess.CurrentRunID = runA.RunID

	sessStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore: %v", err)
	}
	if err := sessStore.SaveSession(context.Background(), sess); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: runA, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
	})

	req, _ := protocol.NewRequest("1", protocol.MethodSessionList, protocol.SessionListParams{
		ThreadID: protocol.ThreadID(runA.SessionID),
		Limit:    20,
	})
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error != nil {
		t.Fatalf("session.list error: %+v", resp.Error)
	}
	var listed protocol.SessionListResult
	if err := json.Unmarshal(resp.Result, &listed); err != nil {
		t.Fatalf("unmarshal session.list: %v", err)
	}
	var got *protocol.SessionListItem
	for i := range listed.Sessions {
		if strings.TrimSpace(listed.Sessions[i].SessionID) == strings.TrimSpace(sess.SessionID) {
			got = &listed.Sessions[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("expected session %s in response", sess.SessionID)
	}
	if got.TotalAgents != 3 || got.RunningAgents != 1 || got.PausedAgents != 1 {
		t.Fatalf("unexpected counts: running=%d paused=%d total=%d", got.RunningAgents, got.PausedAgents, got.TotalAgents)
	}
}

func TestRPCServer_LogsRequestAndResponseSummary(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	_, run, err := implstore.CreateSession(cfg, "logging", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	sessStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore: %v", err)
	}
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: run, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
	})

	var logs bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(prev)

	req, _ := protocol.NewRequest("1", protocol.MethodThreadGet, protocol.ThreadGetParams{ThreadID: protocol.ThreadID(run.SessionID)})
	_ = rpcRoundTrip(t, srv, req)

	out := logs.String()
	if !strings.Contains(out, "rpc.request") {
		t.Fatalf("expected rpc.request log, got %q", out)
	}
	if !strings.Contains(out, "rpc.response") {
		t.Fatalf("expected rpc.response log, got %q", out)
	}
}

func TestRPCServer_SessionRename(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess, run, err := implstore.CreateSession(cfg, "old title", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	sessStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore: %v", err)
	}
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: run, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
	})

	req, _ := protocol.NewRequest("1", protocol.MethodSessionRename, protocol.SessionRenameParams{
		ThreadID:  protocol.ThreadID(run.SessionID),
		SessionID: sess.SessionID,
		Title:     "new title",
	})
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error != nil {
		t.Fatalf("session.rename error: %+v", resp.Error)
	}
	got, err := sessStore.LoadSession(context.Background(), sess.SessionID)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if strings.TrimSpace(got.Title) != "new title" {
		t.Fatalf("title=%q want %q", got.Title, "new title")
	}
}

func TestRPCServer_AgentStart(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess, run, err := implstore.CreateSession(cfg, "alpha goal", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	sess.ActiveModel = "openai/gpt-5-nano"
	sessStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore: %v", err)
	}
	if err := sessStore.SaveSession(context.Background(), sess); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	sessionSvc := newTestSessionService(cfg, sessStore)
	taskMgr := pkgtask.NewManager(ts, sessionSvc)
	agentMgr := pkgagent.NewManager(sessionSvc, taskMgr, taskMgr)
	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: run, TaskService: taskMgr, Session: sessionSvc, AgentService: agentMgr, Index: protocol.NewIndex(0, 0),
	})

	req, _ := protocol.NewRequest("1", protocol.MethodAgentStart, protocol.AgentStartParams{
		ThreadID:  protocol.ThreadID(run.SessionID),
		SessionID: sess.SessionID,
		Profile:   "general",
		Goal:      "next run",
	})
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error != nil {
		t.Fatalf("agent.start error: %+v", resp.Error)
	}
	var out protocol.AgentStartResult
	_ = json.Unmarshal(resp.Result, &out)
	if strings.TrimSpace(out.RunID) == "" || strings.TrimSpace(out.SessionID) == "" {
		t.Fatalf("missing ids: %+v", out)
	}
	if out.SessionID != sess.SessionID {
		t.Fatalf("session mismatch: got %s want %s", out.SessionID, sess.SessionID)
	}
	if out.Model != "openai/gpt-5-nano" {
		t.Fatalf("model = %q want inherited openai/gpt-5-nano", out.Model)
	}
	loadedSess, err := sessStore.LoadSession(context.Background(), sess.SessionID)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	found := false
	for _, id := range loadedSess.Runs {
		if strings.TrimSpace(id) == strings.TrimSpace(out.RunID) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("new run %s not attached to session %s", out.RunID, sess.SessionID)
	}
}

func TestRPCServer_TeamManifestPlanModelEndpoints(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess := types.NewSession("goal")
	run := types.NewRun("goal", 8*1024, sess.SessionID)
	sessStore := store.NewMemorySessionStore()
	_ = sessStore.SaveSession(context.Background(), sess)
	_ = sessStore.SaveRun(context.Background(), run)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))

	teamID := "team-1"
	teamDir := fsutil.GetTeamDir(cfg.DataDir, teamID)
	if err := os.MkdirAll(teamDir, 0o755); err != nil {
		t.Fatalf("mkdir team dir: %v", err)
	}
	manifest := `{"teamId":"team-1","profileId":"startup","teamModel":"openai/gpt-5-mini","coordinatorRole":"ceo","coordinatorRunId":"run-1","roles":[{"roleName":"ceo","runId":"run-1","sessionId":"sess-1"}],"createdAt":"2026-01-01T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(teamDir, "team.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	runDir := fsutil.GetAgentDir(cfg.DataDir, run.RunID)
	planDir := filepath.Join(runDir, "plan")
	_ = os.MkdirAll(planDir, 0o755)
	_ = os.WriteFile(filepath.Join(planDir, "HEAD.md"), []byte("details"), 0o644)
	_ = os.WriteFile(filepath.Join(planDir, "CHECKLIST.md"), []byte("checklist"), 0o644)

	now := time.Now().UTC()
	_ = ts.CreateTask(context.Background(), types.Task{
		TaskID: "task-team", SessionID: run.SessionID, RunID: run.RunID, TeamID: teamID, AssignedRole: "ceo",
		Goal: "G", Status: types.TaskStatusPending, CreatedAt: &now,
	})

	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: run, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
	})

	reqStatus, _ := protocol.NewRequest("1", protocol.MethodTeamGetStatus, protocol.TeamGetStatusParams{
		ThreadID: protocol.ThreadID(run.SessionID), TeamID: teamID,
	})
	respStatus := rpcRoundTrip(t, srv, reqStatus)
	if respStatus.Error != nil {
		t.Fatalf("team.getStatus error: %+v", respStatus.Error)
	}

	reqManifest, _ := protocol.NewRequest("2", protocol.MethodTeamGetManifest, protocol.TeamGetManifestParams{
		ThreadID: protocol.ThreadID(run.SessionID), TeamID: teamID,
	})
	respManifest := rpcRoundTrip(t, srv, reqManifest)
	if respManifest.Error != nil {
		t.Fatalf("team.getManifest error: %+v", respManifest.Error)
	}

	reqPlan, _ := protocol.NewRequest("3", protocol.MethodPlanGet, protocol.PlanGetParams{
		ThreadID: protocol.ThreadID(run.SessionID), RunID: run.RunID,
	})
	respPlan := rpcRoundTrip(t, srv, reqPlan)
	if respPlan.Error != nil {
		t.Fatalf("plan.get error: %+v", respPlan.Error)
	}
	var planRes protocol.PlanGetResult
	_ = json.Unmarshal(respPlan.Result, &planRes)
	if !strings.Contains(planRes.Checklist, "checklist") {
		t.Fatalf("unexpected plan result: %+v", planRes)
	}

	reqModels, _ := protocol.NewRequest("4", protocol.MethodModelList, protocol.ModelListParams{
		ThreadID: protocol.ThreadID(run.SessionID), Provider: "openai",
	})
	respModels := rpcRoundTrip(t, srv, reqModels)
	if respModels.Error != nil {
		t.Fatalf("model.list error: %+v", respModels.Error)
	}
	var models protocol.ModelListResult
	_ = json.Unmarshal(respModels.Result, &models)
	if len(models.Models) == 0 {
		t.Fatalf("expected model.list results")
	}
}

func TestRPCServer_ActivityList_TeamRunFilter(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess := types.NewSession("goal")
	run := types.NewRun("goal", 8*1024, sess.SessionID)
	_, runA, err := implstore.CreateSession(cfg, "goal-a", 8*1024)
	if err != nil {
		t.Fatalf("create run-a session: %v", err)
	}
	_, runB, err := implstore.CreateSession(cfg, "goal-b", 8*1024)
	if err != nil {
		t.Fatalf("create run-b session: %v", err)
	}
	sessStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore: %v", err)
	}
	_ = sessStore.SaveSession(context.Background(), sess)
	_ = sessStore.SaveRun(context.Background(), run)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))

	teamID := "team-1"
	now := time.Now().UTC()
	_ = ts.CreateTask(context.Background(), types.Task{
		TaskID: "task-a", SessionID: run.SessionID, RunID: runA.RunID, TeamID: teamID, AssignedRole: "researcher",
		Goal: "A", Status: types.TaskStatusPending, CreatedAt: &now,
	})
	_ = ts.CreateTask(context.Background(), types.Task{
		TaskID: "task-b", SessionID: run.SessionID, RunID: runB.RunID, TeamID: teamID, AssignedRole: "writer",
		Goal: "B", Status: types.TaskStatusPending, CreatedAt: &now,
	})
	if err := implstore.AppendEvent(context.Background(), cfg, types.EventRecord{
		RunID:     runA.RunID,
		Timestamp: now,
		Type:      "agent.op.request",
		Message:   "run-a op",
		Data: map[string]string{
			"opId": "a1",
			"op":   "fs_read",
			"path": "/workspace/a.txt",
		},
	}); err != nil {
		t.Fatalf("append event run-a: %v", err)
	}
	if err := implstore.AppendEvent(context.Background(), cfg, types.EventRecord{
		RunID:     runB.RunID,
		Timestamp: now.Add(1 * time.Second),
		Type:      "agent.op.request",
		Message:   "run-b op",
		Data: map[string]string{
			"opId": "b1",
			"op":   "fs_read",
			"path": "/workspace/b.txt",
		},
	}); err != nil {
		t.Fatalf("append event run-b: %v", err)
	}

	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: run, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
	})
	req, _ := protocol.NewRequest("1", protocol.MethodActivityList, protocol.ActivityListParams{
		ThreadID: protocol.ThreadID(run.SessionID),
		TeamID:   teamID,
		RunID:    runA.RunID,
		Limit:    50,
	})
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error != nil {
		t.Fatalf("activity.list error: %+v", resp.Error)
	}
	var out protocol.ActivityListResult
	_ = json.Unmarshal(resp.Result, &out)
	if len(out.Activities) != 1 {
		t.Fatalf("activities=%d want 1", len(out.Activities))
	}
	if !strings.HasPrefix(strings.TrimSpace(out.Activities[0].ID), runA.RunID+":") {
		t.Fatalf("expected run-a activity only, got id=%q", out.Activities[0].ID)
	}
}

func TestRPCServer_PlanGet_AggregateTeamUsesManifestRuns(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess := types.NewSession("goal")
	run := types.NewRun("goal", 8*1024, sess.SessionID)
	sessStore := store.NewMemorySessionStore()
	_ = sessStore.SaveSession(context.Background(), sess)
	_ = sessStore.SaveRun(context.Background(), run)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))

	teamID := "team-1"
	teamDir := fsutil.GetTeamDir(cfg.DataDir, teamID)
	if err := os.MkdirAll(teamDir, 0o755); err != nil {
		t.Fatalf("mkdir team dir: %v", err)
	}
	manifest := `{"teamId":"team-1","profileId":"startup","teamModel":"openai/gpt-5-mini","coordinatorRole":"ceo","coordinatorRunId":"run-1","roles":[{"roleName":"ceo","runId":"run-1","sessionId":"sess-1"},{"roleName":"writer","runId":"run-2","sessionId":"sess-2"}],"createdAt":"2026-01-01T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(teamDir, "team.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	for _, tc := range []struct {
		runID     string
		head      string
		checklist string
	}{
		{runID: "run-1", head: "head-1", checklist: "check-1"},
		{runID: "run-2", head: "head-2", checklist: "check-2"},
	} {
		planDir := filepath.Join(fsutil.GetAgentDir(cfg.DataDir, tc.runID), "plan")
		if err := os.MkdirAll(planDir, 0o755); err != nil {
			t.Fatalf("mkdir plan dir: %v", err)
		}
		_ = os.WriteFile(filepath.Join(planDir, "HEAD.md"), []byte(tc.head), 0o644)
		_ = os.WriteFile(filepath.Join(planDir, "CHECKLIST.md"), []byte(tc.checklist), 0o644)
	}

	now := time.Now().UTC()
	_ = ts.CreateTask(context.Background(), types.Task{
		TaskID: "task-1", SessionID: run.SessionID, RunID: "run-1", TeamID: teamID, AssignedRole: "ceo",
		Goal: "G", Status: types.TaskStatusPending, CreatedAt: &now,
	})
	_ = ts.CreateTask(context.Background(), types.Task{
		TaskID: "task-legacy", SessionID: run.SessionID, RunID: "run-legacy", TeamID: teamID, AssignedRole: "legacy",
		Goal: "legacy", Status: types.TaskStatusPending, CreatedAt: &now,
	})

	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: run, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
	})
	req, _ := protocol.NewRequest("1", protocol.MethodPlanGet, protocol.PlanGetParams{
		ThreadID:      protocol.ThreadID(run.SessionID),
		TeamID:        teamID,
		AggregateTeam: true,
	})
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error != nil {
		t.Fatalf("plan.get error: %+v", resp.Error)
	}
	var out protocol.PlanGetResult
	_ = json.Unmarshal(resp.Result, &out)
	if !strings.Contains(out.Checklist, "check-2") {
		t.Fatalf("expected aggregate checklist to include manifest-only run, got: %q", out.Checklist)
	}
	if len(out.SourceRuns) < 2 || strings.TrimSpace(out.SourceRuns[1]) != "run-2" {
		t.Fatalf("expected source runs to include manifest run-2 in order, got %+v", out.SourceRuns)
	}
	for _, runID := range out.SourceRuns {
		if strings.TrimSpace(runID) == "run-legacy" {
			t.Fatalf("stale task-history run should not be included in aggregate source runs: %+v", out.SourceRuns)
		}
	}
}

func TestRPCServer_TaskList_TeamScopeNoRunIDReturnsAllRoleRuns(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess := types.NewSession("goal")
	teamID := "team-1"
	sess.TeamID = teamID
	runA := types.NewRun("goal-a", 8*1024, sess.SessionID)
	runB := types.NewRun("goal-b", 8*1024, sess.SessionID)
	sess.CurrentRunID = runA.RunID
	sess.Runs = []string{runA.RunID, runB.RunID}
	sessStore := store.NewMemorySessionStore()
	_ = sessStore.SaveSession(context.Background(), sess)
	_ = sessStore.SaveRun(context.Background(), runA)
	_ = sessStore.SaveRun(context.Background(), runB)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	now := time.Now().UTC()
	_ = ts.CreateTask(context.Background(), types.Task{
		TaskID: "task-a", SessionID: sess.SessionID, RunID: runA.RunID, TeamID: teamID, AssignedRole: "pm",
		Goal: "task a", Status: types.TaskStatusPending, CreatedAt: &now,
	})
	_ = ts.CreateTask(context.Background(), types.Task{
		TaskID: "task-b", SessionID: sess.SessionID, RunID: runB.RunID, TeamID: teamID, AssignedRole: "ux",
		Goal: "task b", Status: types.TaskStatusPending, CreatedAt: &now,
	})

	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: runA, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
	})
	req, _ := protocol.NewRequest("1", protocol.MethodTaskList, protocol.TaskListParams{
		ThreadID: protocol.ThreadID(sess.SessionID),
		TeamID:   teamID,
		View:     "inbox",
	})
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error != nil {
		t.Fatalf("task.list team error: %+v", resp.Error)
	}
	var out protocol.TaskListResult
	_ = json.Unmarshal(resp.Result, &out)
	if len(out.Tasks) != 2 {
		t.Fatalf("expected 2 team inbox tasks across role runs, got %d", len(out.Tasks))
	}
	seen := map[string]bool{}
	for _, tk := range out.Tasks {
		seen[strings.TrimSpace(tk.ID)] = true
	}
	if !seen["task-a"] || !seen["task-b"] {
		t.Fatalf("expected team-wide tasks from run-a and run-b, got %+v", out.Tasks)
	}
}

func TestRPCServer_TeamGetStatus_ManifestRosterIgnoresStaleTaskRuns(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess := types.NewSession("goal")
	teamID := "team-1"
	sess.TeamID = teamID
	run := types.NewRun("goal", 8*1024, sess.SessionID)
	sessStore := store.NewMemorySessionStore()
	_ = sessStore.SaveSession(context.Background(), sess)
	_ = sessStore.SaveRun(context.Background(), run)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))

	teamDir := fsutil.GetTeamDir(cfg.DataDir, teamID)
	if err := os.MkdirAll(teamDir, 0o755); err != nil {
		t.Fatalf("mkdir team dir: %v", err)
	}
	manifest := `{"teamId":"team-1","profileId":"startup","teamModel":"openai/gpt-5-mini","coordinatorRole":"product-manager","coordinatorRunId":"run-pm","roles":[{"roleName":"product-manager","runId":"run-pm","sessionId":"sess-pm"},{"roleName":"ux-researcher","runId":"run-ux","sessionId":"sess-ux"}],"createdAt":"2026-01-01T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(teamDir, "team.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	now := time.Now().UTC()
	_ = ts.CreateTask(context.Background(), types.Task{
		TaskID: "task-pm", SessionID: sess.SessionID, RunID: "run-pm", TeamID: teamID, AssignedRole: "product-manager",
		Goal: "pm", Status: types.TaskStatusPending, CreatedAt: &now,
	})
	_ = ts.CreateTask(context.Background(), types.Task{
		TaskID: "task-stale", SessionID: sess.SessionID, RunID: "run-legacy", TeamID: teamID, AssignedRole: "legacy",
		Goal: "stale", Status: types.TaskStatusPending, CreatedAt: &now,
	})

	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: run, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
	})
	req, _ := protocol.NewRequest("1", protocol.MethodTeamGetStatus, protocol.TeamGetStatusParams{
		ThreadID: protocol.ThreadID(sess.SessionID),
		TeamID:   teamID,
	})
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error != nil {
		t.Fatalf("team.getStatus error: %+v", resp.Error)
	}
	var out protocol.TeamGetStatusResult
	_ = json.Unmarshal(resp.Result, &out)
	if len(out.RunIDs) != 2 || strings.TrimSpace(out.RunIDs[0]) != "run-pm" || strings.TrimSpace(out.RunIDs[1]) != "run-ux" {
		t.Fatalf("run roster must match manifest and exclude stale history, got %+v", out.RunIDs)
	}
	if got := strings.TrimSpace(out.RoleByRunID["run-pm"]); got != "product-manager" {
		t.Fatalf("roleByRunID[run-pm]=%q want product-manager", got)
	}
	if got := strings.TrimSpace(out.RoleByRunID["run-ux"]); got != "ux-researcher" {
		t.Fatalf("roleByRunID[run-ux]=%q want ux-researcher", got)
	}
	if _, ok := out.RoleByRunID["run-legacy"]; ok {
		t.Fatalf("stale run should not appear in role map: %+v", out.RoleByRunID)
	}
}

func TestRPCServer_TeamGetStatus_IncludesTokenBreakdown(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	bootstrapSess, bootstrapRun, err := implstore.CreateSession(cfg, "bootstrap", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession bootstrap: %v", err)
	}
	sessA, runA, err := implstore.CreateSession(cfg, "team-a", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession team-a: %v", err)
	}
	sessB, runB, err := implstore.CreateSession(cfg, "team-b", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession team-b: %v", err)
	}
	sessA.InputTokens, sessA.OutputTokens, sessA.TotalTokens, sessA.CostUSD = 100, 40, 140, 0.20
	sessB.InputTokens, sessB.OutputTokens, sessB.TotalTokens, sessB.CostUSD = 80, 20, 100, 0.15

	sessStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore: %v", err)
	}
	if err := sessStore.SaveSession(context.Background(), bootstrapSess); err != nil {
		t.Fatalf("SaveSession bootstrap: %v", err)
	}
	if err := sessStore.SaveSession(context.Background(), sessA); err != nil {
		t.Fatalf("SaveSession A: %v", err)
	}
	if err := sessStore.SaveSession(context.Background(), sessB); err != nil {
		t.Fatalf("SaveSession B: %v", err)
	}

	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	now := time.Now().UTC()
	teamID := "team-1"
	_ = ts.CreateTask(context.Background(), types.Task{
		TaskID: "task-a", SessionID: runA.SessionID, RunID: runA.RunID, TeamID: teamID, AssignedRole: "research",
		Goal: "A", Status: types.TaskStatusPending, CreatedAt: &now,
	})
	_ = ts.CreateTask(context.Background(), types.Task{
		TaskID: "task-b", SessionID: runB.SessionID, RunID: runB.RunID, TeamID: teamID, AssignedRole: "ops",
		Goal: "B", Status: types.TaskStatusPending, CreatedAt: &now,
	})
	_ = ts.CompleteTask(context.Background(), "task-a", types.TaskResult{
		TaskID:      "task-a",
		Status:      types.TaskStatusSucceeded,
		TotalTokens: 140,
		CostUSD:     0.20,
		CompletedAt: &now,
	})
	_ = ts.CompleteTask(context.Background(), "task-b", types.TaskResult{
		TaskID:      "task-b",
		Status:      types.TaskStatusSucceeded,
		TotalTokens: 100,
		CostUSD:     0.15,
		CompletedAt: &now,
	})

	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: bootstrapRun, AllowAnyThread: true, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
	})
	req, _ := protocol.NewRequest("1", protocol.MethodTeamGetStatus, protocol.TeamGetStatusParams{
		ThreadID: protocol.ThreadID(bootstrapRun.SessionID),
		TeamID:   teamID,
	})
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error != nil {
		t.Fatalf("team.getStatus error: %+v", resp.Error)
	}
	var out protocol.TeamGetStatusResult
	_ = json.Unmarshal(resp.Result, &out)
	if out.TotalTokensIn != 180 || out.TotalTokensOut != 60 || out.TotalTokens != 240 {
		t.Fatalf("unexpected team status token breakdown: %+v", out)
	}
	if math.Abs(out.TotalCostUSD-0.35) > 1e-9 {
		t.Fatalf("unexpected team status cost: %+v", out)
	}
}

func TestRPCServer_SessionGetTotals_TeamScope_UsesManifestRuns(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	bootstrapSess, bootstrapRun, err := implstore.CreateSession(cfg, "bootstrap", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession bootstrap: %v", err)
	}
	sessManifest, runManifest, err := implstore.CreateSession(cfg, "manifest", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession manifest: %v", err)
	}
	sessStale, runStale, err := implstore.CreateSession(cfg, "stale", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession stale: %v", err)
	}
	sessManifest.InputTokens, sessManifest.OutputTokens, sessManifest.TotalTokens = 50, 25, 75
	sessStale.InputTokens, sessStale.OutputTokens, sessStale.TotalTokens = 9999, 9999, 19998

	sessStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore: %v", err)
	}
	if err := sessStore.SaveSession(context.Background(), bootstrapSess); err != nil {
		t.Fatalf("SaveSession bootstrap: %v", err)
	}
	if err := sessStore.SaveSession(context.Background(), sessManifest); err != nil {
		t.Fatalf("SaveSession manifest: %v", err)
	}
	if err := sessStore.SaveSession(context.Background(), sessStale); err != nil {
		t.Fatalf("SaveSession stale: %v", err)
	}

	teamID := "team-1"
	teamDir := fsutil.GetTeamDir(cfg.DataDir, teamID)
	if err := os.MkdirAll(teamDir, 0o755); err != nil {
		t.Fatalf("mkdir team dir: %v", err)
	}
	manifest := fmt.Sprintf(`{"teamId":"%s","profileId":"startup","teamModel":"openai/gpt-5-mini","coordinatorRole":"pm","coordinatorRunId":"%s","roles":[{"roleName":"pm","runId":"%s","sessionId":"%s"}],"createdAt":"2026-01-01T00:00:00Z"}`, teamID, runManifest.RunID, runManifest.RunID, runManifest.SessionID)
	if err := os.WriteFile(filepath.Join(teamDir, "team.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	now := time.Now().UTC()
	_ = ts.CreateTask(context.Background(), types.Task{
		TaskID: "task-manifest", SessionID: runManifest.SessionID, RunID: runManifest.RunID, TeamID: teamID, AssignedRole: "pm",
		Goal: "manifest", Status: types.TaskStatusPending, CreatedAt: &now,
	})
	_ = ts.CompleteTask(context.Background(), "task-manifest", types.TaskResult{
		TaskID:      "task-manifest",
		Status:      types.TaskStatusSucceeded,
		TotalTokens: 75,
		CompletedAt: &now,
	})
	_ = ts.CreateTask(context.Background(), types.Task{
		TaskID: "task-stale", SessionID: runStale.SessionID, RunID: runStale.RunID, TeamID: teamID, AssignedRole: "legacy",
		Goal: "stale", Status: types.TaskStatusPending, CreatedAt: &now,
	})
	_ = ts.CompleteTask(context.Background(), "task-stale", types.TaskResult{
		TaskID:      "task-stale",
		Status:      types.TaskStatusSucceeded,
		TotalTokens: 19998,
		CompletedAt: &now,
	})

	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: bootstrapRun, AllowAnyThread: true, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
	})
	req, _ := protocol.NewRequest("1", protocol.MethodSessionGetTotals, protocol.SessionGetTotalsParams{
		ThreadID: protocol.ThreadID(bootstrapRun.SessionID),
		TeamID:   teamID,
	})
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error != nil {
		t.Fatalf("session.getTotals(team) error: %+v", resp.Error)
	}
	var out protocol.SessionGetTotalsResult
	_ = json.Unmarshal(resp.Result, &out)
	if out.TotalTokensIn != 50 || out.TotalTokensOut != 25 || out.TotalTokens != 75 {
		t.Fatalf("expected manifest-only totals, got %+v", out)
	}
}

func TestRPCServer_TeamGetStatus_TotalTokensMatchesInOutWhenTheyDifferFromRunStats(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	bootstrapSess, bootstrapRun, err := implstore.CreateSession(cfg, "bootstrap", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession bootstrap: %v", err)
	}
	sessA, runA, err := implstore.CreateSession(cfg, "team-a", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession team-a: %v", err)
	}
	sessA.InputTokens, sessA.OutputTokens = 1000, 250
	sessA.TotalTokens = 1250

	sessStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore: %v", err)
	}
	if err := sessStore.SaveSession(context.Background(), bootstrapSess); err != nil {
		t.Fatalf("SaveSession bootstrap: %v", err)
	}
	if err := sessStore.SaveSession(context.Background(), sessA); err != nil {
		t.Fatalf("SaveSession team-a: %v", err)
	}

	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	now := time.Now().UTC()
	teamID := "team-1"
	_ = ts.CreateTask(context.Background(), types.Task{
		TaskID: "task-a", SessionID: runA.SessionID, RunID: runA.RunID, TeamID: teamID, AssignedRole: "research",
		Goal: "A", Status: types.TaskStatusPending, CreatedAt: &now,
	})
	// Deliberately low run stats to ensure total token display uses in/out totals.
	_ = ts.CompleteTask(context.Background(), "task-a", types.TaskResult{
		TaskID:      "task-a",
		Status:      types.TaskStatusSucceeded,
		TotalTokens: 10,
		CompletedAt: &now,
	})

	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: bootstrapRun, AllowAnyThread: true, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
	})
	req, _ := protocol.NewRequest("1", protocol.MethodTeamGetStatus, protocol.TeamGetStatusParams{
		ThreadID: protocol.ThreadID(bootstrapRun.SessionID),
		TeamID:   teamID,
	})
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error != nil {
		t.Fatalf("team.getStatus error: %+v", resp.Error)
	}
	var out protocol.TeamGetStatusResult
	_ = json.Unmarshal(resp.Result, &out)
	if out.TotalTokensIn != 1000 || out.TotalTokensOut != 250 {
		t.Fatalf("unexpected token in/out: %+v", out)
	}
	if out.TotalTokens != 1250 {
		t.Fatalf("expected total tokens to match in+out, got %+v", out)
	}
}

func TestRPCServer_ActivityList_TeamUsesManifestRunsOnlyByDefault(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess := types.NewSession("goal")
	teamID := "team-1"
	sess.TeamID = teamID
	rootRun := types.NewRun("goal", 8*1024, sess.SessionID)
	manifestRun := types.NewRun("manifest-run", 8*1024, sess.SessionID)
	manifestRun.RunID = "run-manifest"
	staleRun := types.NewRun("stale-run", 8*1024, sess.SessionID)
	staleRun.RunID = "run-legacy"
	sess.CurrentRunID = rootRun.RunID
	sess.Runs = []string{rootRun.RunID, manifestRun.RunID, staleRun.RunID}
	sessStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore: %v", err)
	}
	_ = sessStore.SaveSession(context.Background(), sess)
	_ = sessStore.SaveRun(context.Background(), rootRun)
	_ = sessStore.SaveRun(context.Background(), manifestRun)
	_ = sessStore.SaveRun(context.Background(), staleRun)
	ts, _ := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))

	teamDir := fsutil.GetTeamDir(cfg.DataDir, teamID)
	if err := os.MkdirAll(teamDir, 0o755); err != nil {
		t.Fatalf("mkdir team dir: %v", err)
	}
	manifest := `{"teamId":"team-1","profileId":"startup","teamModel":"openai/gpt-5-mini","coordinatorRole":"product-manager","coordinatorRunId":"run-manifest","roles":[{"roleName":"product-manager","runId":"run-manifest","sessionId":"sess-pm"}],"createdAt":"2026-01-01T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(teamDir, "team.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	now := time.Now().UTC()
	_ = ts.CreateTask(context.Background(), types.Task{
		TaskID: "task-manifest", SessionID: sess.SessionID, RunID: manifestRun.RunID, TeamID: teamID, AssignedRole: "product-manager",
		Goal: "manifest", Status: types.TaskStatusPending, CreatedAt: &now,
	})
	_ = ts.CreateTask(context.Background(), types.Task{
		TaskID: "task-stale", SessionID: sess.SessionID, RunID: staleRun.RunID, TeamID: teamID, AssignedRole: "legacy",
		Goal: "stale", Status: types.TaskStatusPending, CreatedAt: &now,
	})
	if err := implstore.AppendEvent(context.Background(), cfg, types.EventRecord{
		RunID:     manifestRun.RunID,
		Timestamp: now,
		Type:      "agent.op.request",
		Message:   "manifest op",
		Data:      map[string]string{"opId": "m1", "op": "fs_read", "path": "/workspace/m.txt"},
	}); err != nil {
		t.Fatalf("append event manifest run: %v", err)
	}
	if err := implstore.AppendEvent(context.Background(), cfg, types.EventRecord{
		RunID:     staleRun.RunID,
		Timestamp: now.Add(time.Second),
		Type:      "agent.op.request",
		Message:   "stale op",
		Data:      map[string]string{"opId": "s1", "op": "fs_read", "path": "/workspace/s.txt"},
	}); err != nil {
		t.Fatalf("append event stale run: %v", err)
	}

	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg, Run: rootRun, TaskService: pkgtask.NewManager(ts, nil), Session: newTestSessionService(cfg, sessStore), Index: protocol.NewIndex(0, 0),
	})
	req, _ := protocol.NewRequest("1", protocol.MethodActivityList, protocol.ActivityListParams{
		ThreadID: protocol.ThreadID(sess.SessionID),
		TeamID:   teamID,
		Limit:    50,
	})
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error != nil {
		t.Fatalf("activity.list error: %+v", resp.Error)
	}
	var out protocol.ActivityListResult
	_ = json.Unmarshal(resp.Result, &out)
	if len(out.Activities) != 1 {
		t.Fatalf("expected only manifest-run activity, got %d entries: %+v", len(out.Activities), out.Activities)
	}
	if !strings.HasPrefix(strings.TrimSpace(out.Activities[0].ID), manifestRun.RunID+":") {
		t.Fatalf("expected manifest run activity only, got id=%q", out.Activities[0].ID)
	}
}
