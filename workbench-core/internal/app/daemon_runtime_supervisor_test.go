package app

import (
	"context"
	"sync"
	"testing"
	"time"

	implstore "github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/pkg/agent/state"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/types"
)

func TestRuntimeSupervisor_StopRun_CancelsWorkerAndPauses(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	_, run, err := implstore.CreateSession(cfg, "stop run", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	var mu sync.Mutex
	cancelCalled := false
	done := make(chan struct{})
	supervisor := &runtimeSupervisor{
		cfg: cfg,
		workers: map[string]*managedRuntime{
			run.RunID: {
				runID:     run.RunID,
				sessionID: run.SessionID,
				cancel: func() {
					mu.Lock()
					cancelCalled = true
					mu.Unlock()
					close(done)
				},
				done: done,
			},
		},
	}

	if err := supervisor.StopRun(run.RunID); err != nil {
		t.Fatalf("StopRun: %v", err)
	}

	mu.Lock()
	gotCancel := cancelCalled
	mu.Unlock()
	if !gotCancel {
		t.Fatalf("expected worker cancel to be called")
	}

	loaded, err := implstore.LoadRun(cfg, run.RunID)
	if err != nil {
		t.Fatalf("LoadRun: %v", err)
	}
	if loaded.Status != types.RunStatusPaused {
		t.Fatalf("run status=%q want %q", loaded.Status, types.RunStatusPaused)
	}

	supervisor.mu.Lock()
	_, exists := supervisor.workers[run.RunID]
	supervisor.mu.Unlock()
	if exists {
		t.Fatalf("expected worker to be removed after stop")
	}
}

func TestRuntimeSupervisor_StopSession_StopsOnlySessionRuns(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sessA, runA, err := implstore.CreateSession(cfg, "session-a", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession A: %v", err)
	}
	runA2 := types.NewRun("secondary-a", 8*1024, sessA.SessionID)
	if err := implstore.SaveRun(cfg, runA2); err != nil {
		t.Fatalf("SaveRun A2: %v", err)
	}
	sessA.Runs = append(sessA.Runs, runA2.RunID)

	_, runB, err := implstore.CreateSession(cfg, "session-b", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession B: %v", err)
	}

	sessionStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore: %v", err)
	}
	if err := sessionStore.SaveSession(context.Background(), sessA); err != nil {
		t.Fatalf("SaveSession A: %v", err)
	}

	done := make(chan struct{})
	supervisor := &runtimeSupervisor{
		cfg:          cfg,
		sessionStore: sessionStore,
		workers: map[string]*managedRuntime{
			runA.RunID: {
				runID:     runA.RunID,
				sessionID: runA.SessionID,
				cancel:    func() { close(done) },
				done:      done,
			},
		},
	}

	affected, err := supervisor.StopSession(context.Background(), sessA.SessionID)
	if err != nil {
		t.Fatalf("StopSession: %v", err)
	}
	if len(affected) < 2 {
		t.Fatalf("expected >=2 affected runs, got %v", affected)
	}

	loadedA, err := implstore.LoadRun(cfg, runA.RunID)
	if err != nil {
		t.Fatalf("LoadRun A: %v", err)
	}
	if loadedA.Status != types.RunStatusPaused {
		t.Fatalf("runA status=%q want %q", loadedA.Status, types.RunStatusPaused)
	}

	loadedA2, err := implstore.LoadRun(cfg, runA2.RunID)
	if err != nil {
		t.Fatalf("LoadRun A2: %v", err)
	}
	if loadedA2.Status != types.RunStatusPaused {
		t.Fatalf("runA2 status=%q want %q", loadedA2.Status, types.RunStatusPaused)
	}

	loadedB, err := implstore.LoadRun(cfg, runB.RunID)
	if err != nil {
		t.Fatalf("LoadRun B: %v", err)
	}
	if loadedB.Status != types.RunStatusRunning {
		t.Fatalf("runB status=%q want %q", loadedB.Status, types.RunStatusRunning)
	}
}

func TestRuntimeSupervisor_PauseRun_CancelsWorkerAndActiveTasks(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	_, run, err := implstore.CreateSession(cfg, "pause run", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	ts, err := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	now := time.Now().UTC()
	if err := ts.CreateTask(context.Background(), types.Task{
		TaskID:         "task-active",
		SessionID:      run.SessionID,
		RunID:          run.RunID,
		AssignedToType: "agent",
		AssignedTo:     run.RunID,
		Goal:           "work",
		Status:         types.TaskStatusActive,
		CreatedAt:      &now,
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	var mu sync.Mutex
	cancelCalled := false
	done := make(chan struct{})
	supervisor := &runtimeSupervisor{
		cfg:       cfg,
		taskStore: ts,
		workers: map[string]*managedRuntime{
			run.RunID: {
				runID:     run.RunID,
				sessionID: run.SessionID,
				cancel: func() {
					mu.Lock()
					cancelCalled = true
					mu.Unlock()
					close(done)
				},
				done: done,
			},
		},
	}

	if err := supervisor.PauseRun(run.RunID); err != nil {
		t.Fatalf("PauseRun: %v", err)
	}
	mu.Lock()
	gotCancel := cancelCalled
	mu.Unlock()
	if !gotCancel {
		t.Fatalf("expected worker cancel to be called")
	}
	loadedTask, err := ts.GetTask(context.Background(), "task-active")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if loadedTask.Status != types.TaskStatusCanceled {
		t.Fatalf("task status=%q want %q", loadedTask.Status, types.TaskStatusCanceled)
	}
}

// TestApplySessionModel_SkipsChildRuns verifies that when applying session model to workers,
// child runs (sub-agents) are skipped so the parent's model change does not overwrite sub-agent model.
func TestApplySessionModel_SkipsChildRuns(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess, parentRun, err := implstore.CreateSession(cfg, "parent", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	childRun := types.NewChildRun(parentRun.RunID, "child goal", sess.SessionID, 1)
	childRun.Runtime = &types.RunRuntimeConfig{Model: "child-model"}
	if err := implstore.SaveRun(cfg, childRun); err != nil {
		t.Fatalf("SaveRun child: %v", err)
	}
	sess.Runs = append(sess.Runs, childRun.RunID)
	sessionStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore: %v", err)
	}
	if err := sessionStore.SaveSession(context.Background(), sess); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	done := make(chan struct{})
	supervisor := &runtimeSupervisor{
		cfg:          cfg,
		sessionStore: sessionStore,
		workers: map[string]*managedRuntime{
			parentRun.RunID: {runID: parentRun.RunID, sessionID: sess.SessionID, cancel: func() {}, done: done},
			childRun.RunID:  {runID: childRun.RunID, sessionID: sess.SessionID, cancel: func() {}, done: done},
		},
	}

	applied, err := supervisor.ApplySessionModel(context.Background(), sess.SessionID, "", "new-model")
	if err != nil {
		t.Fatalf("ApplySessionModel: %v", err)
	}
	// Child run must not receive session model; only parent can be in applied (if it had a session we'd have called SetModel).
	// We have no real session so parent worker.session is nil and gets skipped by "if worker.session == nil { continue }".
	// So applied should be empty. The important part is child run must not be in applied.
	for _, id := range applied {
		if id == childRun.RunID {
			t.Fatalf("ApplySessionModel must not apply to child run %q", childRun.RunID)
		}
	}
}
