package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/tinoosan/workbench-core/pkg/agent/state"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/protocol"
	"github.com/tinoosan/workbench-core/pkg/store"
	"github.com/tinoosan/workbench-core/pkg/types"
)

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
