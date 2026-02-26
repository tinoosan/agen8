package app

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tinoosan/agen8/pkg/agent/state"
	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/fsutil"
	"github.com/tinoosan/agen8/pkg/protocol"
	pkgtask "github.com/tinoosan/agen8/pkg/services/task"
	"github.com/tinoosan/agen8/pkg/store"
	"github.com/tinoosan/agen8/pkg/types"
)

func TestRPCServerTaskCreateAndListIncludesHarnessID(t *testing.T) {
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
		Index:       protocol.NewIndex(0, 0),
	})

	createReq, _ := protocol.NewRequest("1", protocol.MethodTaskCreate, protocol.TaskCreateParams{
		ThreadID:   protocol.ThreadID(run.SessionID),
		RunID:      run.RunID,
		Goal:       "Do harness task",
		HarnessID:  "codex-cli",
		AssignedTo: run.RunID,
	})
	createResp := rpcRoundTrip(t, srv, createReq)
	if createResp.Error != nil {
		t.Fatalf("task.create error: %+v", createResp.Error)
	}
	var createRes protocol.TaskCreateResult
	if err := json.Unmarshal(createResp.Result, &createRes); err != nil {
		t.Fatalf("unmarshal task.create: %v", err)
	}
	if got := createRes.Task.HarnessID; got != "codex-cli" {
		t.Fatalf("task.create harnessId = %q want %q", got, "codex-cli")
	}

	stored, err := ts.GetTask(context.Background(), createRes.Task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got := stored.Metadata["harnessId"]; got == nil || got.(string) != "codex-cli" {
		t.Fatalf("stored task harnessId metadata = %#v", got)
	}

	listReq, _ := protocol.NewRequest("2", protocol.MethodTaskList, protocol.TaskListParams{
		ThreadID: protocol.ThreadID(run.SessionID),
		RunID:    run.RunID,
		View:     "inbox",
	})
	listResp := rpcRoundTrip(t, srv, listReq)
	if listResp.Error != nil {
		t.Fatalf("task.list error: %+v", listResp.Error)
	}
	var listRes protocol.TaskListResult
	if err := json.Unmarshal(listResp.Result, &listRes); err != nil {
		t.Fatalf("unmarshal task.list: %v", err)
	}
	if len(listRes.Tasks) == 0 {
		t.Fatalf("task.list returned no tasks")
	}
	if got := listRes.Tasks[0].HarnessID; got != "codex-cli" {
		t.Fatalf("task.list harnessId = %q want %q", got, "codex-cli")
	}
}
