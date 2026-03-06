package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	implstore "github.com/tinoosan/agen8/internal/store"
	"github.com/tinoosan/agen8/pkg/agent"
	agentsession "github.com/tinoosan/agen8/pkg/agent/session"
	"github.com/tinoosan/agen8/pkg/agent/state"
	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/fsutil"
	llmtypes "github.com/tinoosan/agen8/pkg/llm/types"
	"github.com/tinoosan/agen8/pkg/profile"
	pkgsession "github.com/tinoosan/agen8/pkg/services/session"
	pkgtask "github.com/tinoosan/agen8/pkg/services/task"
	"github.com/tinoosan/agen8/pkg/types"
)

type ctxCheckingSessionService struct {
	pkgsession.Service
	expected context.Context
	loadRun  func(runID string) (types.Run, error)
	saveRun  func(run types.Run) error
}

func (s *ctxCheckingSessionService) LoadRun(ctx context.Context, runID string) (types.Run, error) {
	if s.expected != nil && ctx != s.expected {
		return types.Run{}, fmt.Errorf("LoadRun received unexpected context")
	}
	if s.loadRun != nil {
		return s.loadRun(runID)
	}
	return types.Run{}, nil
}

func (s *ctxCheckingSessionService) SaveRun(ctx context.Context, run types.Run) error {
	if s.expected != nil && ctx != s.expected {
		return fmt.Errorf("SaveRun received unexpected context")
	}
	if s.saveRun != nil {
		return s.saveRun(run)
	}
	return nil
}

type ctxCheckingTaskService struct {
	pkgtask.TaskServiceForSupervisor
	expected context.Context
	cancel   func(runID, reason string) (int, error)
}

func (s *ctxCheckingTaskService) CancelActiveTasksByRun(ctx context.Context, runID, reason string) (int, error) {
	if s.expected != nil && ctx != s.expected {
		return 0, fmt.Errorf("CancelActiveTasksByRun received unexpected context")
	}
	if s.cancel != nil {
		return s.cancel(runID, reason)
	}
	return 0, nil
}

func (s *ctxCheckingTaskService) GetRunStats(_ context.Context, _ string) (state.RunStats, error) {
	return state.RunStats{}, nil
}

// newSupervisorTestSessionService creates a session service backed by SQLite for supervisor tests.
func newSupervisorTestSessionService(t *testing.T, cfg config.Config) pkgsession.Service {
	t.Helper()
	sessionStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore: %v", err)
	}
	return newTestSessionService(cfg, sessionStore)
}

func injectHandle(s *runtimeSupervisor, runID string, rt *managedRuntime, state handleState) {
	if s == nil {
		return
	}
	s.ensureLoopState()
	h := &runHandle{
		runID:     strings.TrimSpace(runID),
		sessionID: strings.TrimSpace(rt.sessionID),
		rt:        rt,
		state:     state,
	}
	s.handles[h.runID] = h
	s.updateSnapshot(h.runID, h, types.RunStatusRunning)
	if rt != nil && rt.done != nil {
		s.startExitWatcher(rt, h.runID, h)
	}
}

func processNextSupervisorCmd(t *testing.T, s *runtimeSupervisor) {
	t.Helper()
	select {
	case cmd := <-s.cmdCh:
		s.processCmd(context.Background(), cmd)
	case <-time.After(1 * time.Second):
		t.Fatalf("timed out waiting for supervisor command")
	}
}

func drainSupervisorCmds(s *runtimeSupervisor) {
	for {
		select {
		case cmd := <-s.cmdCh:
			s.processCmd(context.Background(), cmd)
		default:
			return
		}
	}
}

func startSupervisorLoop(t *testing.T, s *runtimeSupervisor) (context.CancelFunc, <-chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.Run(ctx)
		close(done)
	}()
	return cancel, done
}

func startSupervisorCmdProcessor(t *testing.T, s *runtimeSupervisor) (func(), <-chan struct{}) {
	t.Helper()
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			case cmd := <-s.cmdCh:
				s.processCmd(context.Background(), cmd)
			}
		}
	}()
	return func() { close(stop) }, done
}

func TestRuntimeSupervisor_StopRun_CancelsWorkerAndPauses(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	_, run, err := implstore.CreateSession(cfg, "stop run", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	ts, err := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	taskSvc := pkgtask.NewManager(ts, nil)
	sessionSvc := newSupervisorTestSessionService(t, cfg)

	var mu sync.Mutex
	cancelCalled := false
	done := make(chan struct{})
	supervisor := &runtimeSupervisor{
		cfg:            cfg,
		taskService:    taskSvc,
		sessionService: sessionSvc,
	}
	injectHandle(supervisor, run.RunID, &managedRuntime{
		runID:     run.RunID,
		sessionID: run.SessionID,
		cancel: func() {
			mu.Lock()
			cancelCalled = true
			mu.Unlock()
			close(done)
		},
		done: done,
	}, handleStateRunning)
	stopProcessor, loopDone := startSupervisorCmdProcessor(t, supervisor)
	defer func() {
		stopProcessor()
		<-loopDone
	}()

	if err := supervisor.StopRun(context.Background(), run.RunID); err != nil {
		t.Fatalf("StopRun: %v", err)
	}

	// stopWorker runs asynchronously; wait for the done channel to close (cancel fires inside).
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for worker cancel to be called")
	}

	mu.Lock()
	gotCancel := cancelCalled
	mu.Unlock()
	if !gotCancel {
		t.Fatalf("expected worker cancel to be called")
	}

	loaded, err := sessionSvc.LoadRun(context.Background(), run.RunID)
	if err != nil {
		t.Fatalf("LoadRun: %v", err)
	}
	if loaded.Status != types.RunStatusPaused {
		t.Fatalf("run status=%q want %q", loaded.Status, types.RunStatusPaused)
	}

	// Wait for the worker exit to be reflected in externally visible runtime state.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		st, stateErr := supervisor.GetRunState(context.Background(), run.SessionID, run.RunID)
		if stateErr == nil && !st.WorkerPresent {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	st, err := supervisor.GetRunState(context.Background(), run.SessionID, run.RunID)
	if err != nil {
		t.Fatalf("GetRunState: %v", err)
	}
	if st.WorkerPresent {
		t.Fatalf("expected worker to be absent after stop")
	}
}

func TestRuntimeSupervisor_SubagentAwaitingReviewTimeout_DefaultAndEnv(t *testing.T) {
	t.Setenv("AGEN8_SUBAGENT_AWAITING_REVIEW_TIMEOUT", "")
	s := &runtimeSupervisor{}
	if got := s.subagentAwaitingReviewTimeout(); got != defaultSubagentAwaitingReviewTimeout {
		t.Fatalf("default timeout=%v want %v", got, defaultSubagentAwaitingReviewTimeout)
	}

	t.Setenv("AGEN8_SUBAGENT_AWAITING_REVIEW_TIMEOUT", "45s")
	if got := s.subagentAwaitingReviewTimeout(); got != 45*time.Second {
		t.Fatalf("env timeout=%v want 45s", got)
	}

	t.Setenv("AGEN8_SUBAGENT_AWAITING_REVIEW_TIMEOUT", "invalid")
	if got := s.subagentAwaitingReviewTimeout(); got != defaultSubagentAwaitingReviewTimeout {
		t.Fatalf("invalid env timeout=%v want %v", got, defaultSubagentAwaitingReviewTimeout)
	}

	t.Setenv("AGEN8_SUBAGENT_AWAITING_REVIEW_TIMEOUT", "-1s")
	if got := s.subagentAwaitingReviewTimeout(); got != defaultSubagentAwaitingReviewTimeout {
		t.Fatalf("negative env timeout=%v want %v", got, defaultSubagentAwaitingReviewTimeout)
	}

	// Sanity: environment restoration not required due to t.Setenv, but ensure no panic path.
	_ = os.Getenv("AGEN8_SUBAGENT_AWAITING_REVIEW_TIMEOUT")
}

func TestRuntimeSupervisor_WorkerShutdownTimeout_DefaultAndEnv(t *testing.T) {
	t.Setenv("AGEN8_WORKER_SHUTDOWN_TIMEOUT", "")
	s := &runtimeSupervisor{}
	if got := s.workerShutdownTimeout(); got != defaultWorkerShutdownTimeout {
		t.Fatalf("default timeout=%v want %v", got, defaultWorkerShutdownTimeout)
	}

	t.Setenv("AGEN8_WORKER_SHUTDOWN_TIMEOUT", "3s")
	if got := s.workerShutdownTimeout(); got != 3*time.Second {
		t.Fatalf("env timeout=%v want 3s", got)
	}

	t.Setenv("AGEN8_WORKER_SHUTDOWN_TIMEOUT", "invalid")
	if got := s.workerShutdownTimeout(); got != defaultWorkerShutdownTimeout {
		t.Fatalf("invalid env timeout=%v want %v", got, defaultWorkerShutdownTimeout)
	}
}

func TestRuntimeSupervisor_DeactivateAndArchiveSubagent_DoesNotHangOnStuckWorker(t *testing.T) {
	t.Setenv("AGEN8_WORKER_SHUTDOWN_TIMEOUT", "20ms")
	cfg := config.Config{DataDir: t.TempDir()}
	_, run, err := implstore.CreateSession(cfg, "cleanup run", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	run.Runtime = &types.RunRuntimeConfig{LifecycleState: "active"}
	if err := implstore.SaveRun(cfg, run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}
	sessionSvc := newSupervisorTestSessionService(t, cfg)

	cancelCalled := false
	supervisor := &runtimeSupervisor{
		cfg:            cfg,
		sessionService: sessionSvc,
	}
	injectHandle(supervisor, run.RunID, &managedRuntime{
		runID: run.RunID,
		cancel: func() {
			cancelCalled = true
		},
		done: make(chan struct{}), // never closes: simulates stuck worker
	}, handleStateRunning)

	stopProcessor, loopDone := startSupervisorCmdProcessor(t, supervisor)
	defer func() {
		stopProcessor()
		<-loopDone
	}()
	done := make(chan struct{})
	go func() {
		supervisor.deactivateAndArchiveSubagent(context.Background(), run.RunID)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatalf("deactivateAndArchiveSubagent blocked on stuck worker")
	}
	if !cancelCalled {
		t.Fatalf("expected worker cancel to be called")
	}
	loaded, err := sessionSvc.LoadRun(context.Background(), run.RunID)
	if err != nil {
		t.Fatalf("LoadRun: %v", err)
	}
	if loaded.Status != types.RunStatusSucceeded {
		t.Fatalf("run status=%q want %q", loaded.Status, types.RunStatusSucceeded)
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

	sessionSvc := newSupervisorTestSessionService(t, cfg)
	if err := sessionSvc.SaveSession(context.Background(), sessA); err != nil {
		t.Fatalf("SaveSession A: %v", err)
	}

	ts, err := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	taskSvc := pkgtask.NewManager(ts, nil)

	done := make(chan struct{})
	supervisor := &runtimeSupervisor{
		cfg:            cfg,
		taskService:    taskSvc,
		sessionService: sessionSvc,
	}
	injectHandle(supervisor, runA.RunID, &managedRuntime{
		runID:     runA.RunID,
		sessionID: runA.SessionID,
		cancel:    func() { close(done) },
		done:      done,
	}, handleStateRunning)

	affected, err := supervisor.StopSession(context.Background(), sessA.SessionID)
	if err != nil {
		t.Fatalf("StopSession: %v", err)
	}
	if len(affected) < 2 {
		t.Fatalf("expected >=2 affected runs, got %v", affected)
	}

	loadedA, err := sessionSvc.LoadRun(context.Background(), runA.RunID)
	if err != nil {
		t.Fatalf("LoadRun A: %v", err)
	}
	if loadedA.Status != types.RunStatusPaused {
		t.Fatalf("runA status=%q want %q", loadedA.Status, types.RunStatusPaused)
	}

	loadedA2, err := sessionSvc.LoadRun(context.Background(), runA2.RunID)
	if err != nil {
		t.Fatalf("LoadRun A2: %v", err)
	}
	if loadedA2.Status != types.RunStatusPaused {
		t.Fatalf("runA2 status=%q want %q", loadedA2.Status, types.RunStatusPaused)
	}

	loadedB, err := sessionSvc.LoadRun(context.Background(), runB.RunID)
	if err != nil {
		t.Fatalf("LoadRun B: %v", err)
	}
	if loadedB.Status != types.RunStatusRunning {
		t.Fatalf("runB status=%q want %q", loadedB.Status, types.RunStatusRunning)
	}
}

func TestRuntimeSupervisor_PauseRun_CancelsWorkerButLeavesActiveTasks(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	_, run, err := implstore.CreateSession(cfg, "pause run", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	ts, err := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	taskSvc := pkgtask.NewManager(ts, nil)
	sessionSvc := newSupervisorTestSessionService(t, cfg)
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
		cfg:            cfg,
		taskService:    taskSvc,
		sessionService: sessionSvc,
	}
	injectHandle(supervisor, run.RunID, &managedRuntime{
		runID:     run.RunID,
		sessionID: run.SessionID,
		cancel: func() {
			mu.Lock()
			cancelCalled = true
			mu.Unlock()
			close(done)
		},
		done: done,
	}, handleStateRunning)
	stopProcessor, loopDone := startSupervisorCmdProcessor(t, supervisor)
	defer func() {
		stopProcessor()
		<-loopDone
	}()

	if err := supervisor.PauseRun(context.Background(), run.RunID); err != nil {
		t.Fatalf("PauseRun: %v", err)
	}

	// Pause should not cancel the active task; it only drains the worker.
	loadedTask, err := ts.GetTask(context.Background(), "task-active")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if loadedTask.Status != types.TaskStatusActive {
		t.Fatalf("task status=%q want %q", loadedTask.Status, types.TaskStatusActive)
	}

	// stopWorker runs asynchronously; wait for the done channel to close (cancel fires inside).
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for worker cancel to be called")
	}

	mu.Lock()
	gotCancel := cancelCalled
	mu.Unlock()
	if !gotCancel {
		t.Fatalf("expected worker cancel to be called")
	}
}

func TestRuntimeSupervisor_PauseRun_UsesCallerContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), struct{}{}, "marker")
	var saved types.Run
	sessionSvc := &ctxCheckingSessionService{
		expected: ctx,
		loadRun: func(runID string) (types.Run, error) {
			return types.Run{RunID: runID, SessionID: "sess-1", Status: types.RunStatusRunning}, nil
		},
		saveRun: func(run types.Run) error {
			saved = run
			return nil
		},
	}
	supervisor := &runtimeSupervisor{
		sessionService: sessionSvc,
		taskService:    &ctxCheckingTaskService{expected: ctx},
	}

	if err := supervisor.pauseRun(ctx, "run-ctx-1"); err != nil {
		t.Fatalf("pauseRun: %v", err)
	}
	if saved.Status != types.RunStatusPaused {
		t.Fatalf("saved status=%q want %q", saved.Status, types.RunStatusPaused)
	}
}

func TestRuntimeSupervisor_StopRun_UsesCallerContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), struct{}{}, "marker")
	cancelCalled := false
	var saved types.Run
	sessionSvc := &ctxCheckingSessionService{
		expected: ctx,
		loadRun: func(runID string) (types.Run, error) {
			return types.Run{RunID: runID, Status: types.RunStatusRunning}, nil
		},
		saveRun: func(run types.Run) error {
			saved = run
			return nil
		},
	}
	taskSvc := &ctxCheckingTaskService{
		expected: ctx,
		cancel: func(runID, reason string) (int, error) {
			cancelCalled = true
			return 1, nil
		},
	}
	supervisor := &runtimeSupervisor{
		sessionService: sessionSvc,
		taskService:    taskSvc,
	}

	if err := supervisor.stopRun(ctx, "run-ctx-2"); err != nil {
		t.Fatalf("stopRun: %v", err)
	}
	if !cancelCalled {
		t.Fatalf("expected CancelActiveTasksByRun to be called")
	}
	if saved.Status != types.RunStatusPaused {
		t.Fatalf("saved status=%q want %q", saved.Status, types.RunStatusPaused)
	}
}

func TestRuntimeSupervisor_StopRun_HonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sessionSvc := &ctxCheckingSessionService{
		expected: ctx,
		loadRun: func(runID string) (types.Run, error) {
			return types.Run{}, ctx.Err()
		},
	}
	supervisor := &runtimeSupervisor{
		sessionService: sessionSvc,
		taskService:    &ctxCheckingTaskService{expected: ctx},
	}

	err := supervisor.stopRun(ctx, "run-ctx-3")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
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

	sessionSvc := newSupervisorTestSessionService(t, cfg)
	if err := sessionSvc.SaveSession(context.Background(), sess); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	ts, err := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	taskSvc := pkgtask.NewManager(ts, nil)

	done := make(chan struct{})
	supervisor := &runtimeSupervisor{
		cfg:            cfg,
		taskService:    taskSvc,
		sessionService: sessionSvc,
	}
	injectHandle(supervisor, parentRun.RunID, &managedRuntime{runID: parentRun.RunID, sessionID: sess.SessionID, cancel: func() {}, done: done}, handleStateRunning)
	injectHandle(supervisor, childRun.RunID, &managedRuntime{runID: childRun.RunID, sessionID: sess.SessionID, cancel: func() {}, done: done}, handleStateRunning)

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

func TestBuildRoleRuntimeProfile_CopiesRoleSkillsForSupervisor(t *testing.T) {
	enabled := true
	role := profile.RoleConfig{
		Name:                    "qa",
		Description:             "QA role",
		Skills:                  []string{"automation"},
		CodeExecOnly:            &enabled,
		CodeExecRequiredImports: []string{"requests"},
		AllowedTools:            []string{"task_review"},
	}
	got := buildRoleRuntimeProfile(role)
	if got == nil {
		t.Fatalf("expected profile")
	}
	if got.ID != "qa" {
		t.Fatalf("id=%q", got.ID)
	}
	if len(got.Skills) != 1 || got.Skills[0] != "automation" {
		t.Fatalf("unexpected skills: %v", got.Skills)
	}
	if len(got.AllowedTools) != 1 || got.AllowedTools[0] != "task_review" {
		t.Fatalf("unexpected allowed tools: %v", got.AllowedTools)
	}
	if !got.CodeExecOnly {
		t.Fatalf("expected code_exec_only copied from role override")
	}
	if len(got.CodeExecRequiredImports) != 1 || got.CodeExecRequiredImports[0] != "requests" {
		t.Fatalf("unexpected code_exec required imports: %v", got.CodeExecRequiredImports)
	}
}

type countingAgent struct {
	cfg      agent.AgentConfig
	mu       sync.Mutex
	setModel int
}

func newCountingAgent(model string) *countingAgent {
	return &countingAgent{
		cfg: agent.AgentConfig{
			Model: model,
			Hooks: agent.Hooks{},
		},
	}
}

func (a *countingAgent) Run(_ context.Context, _ string) (agent.RunResult, error) {
	return agent.RunResult{Text: "ok", Status: types.TaskStatusSucceeded}, nil
}

func (a *countingAgent) RunConversation(_ context.Context, _ []llmtypes.LLMMessage) (agent.RunResult, []llmtypes.LLMMessage, int, error) {
	return agent.RunResult{Text: "ok", Status: types.TaskStatusSucceeded}, nil, 0, nil
}

func (a *countingAgent) ExecHostOp(_ context.Context, _ types.HostOpRequest) types.HostOpResponse {
	return types.HostOpResponse{Ok: true}
}

func (a *countingAgent) GetModel() string { return a.cfg.Model }
func (a *countingAgent) SetModel(v string) {
	a.mu.Lock()
	a.cfg.Model = v
	a.setModel++
	a.mu.Unlock()
}
func (a *countingAgent) modelSetCalls() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.setModel
}
func (a *countingAgent) WebSearchEnabled() bool                      { return a.cfg.EnableWebSearch }
func (a *countingAgent) SetEnableWebSearch(v bool)                   { a.cfg.EnableWebSearch = v }
func (a *countingAgent) GetApprovalsMode() string                    { return a.cfg.ApprovalsMode }
func (a *countingAgent) SetApprovalsMode(v string)                   { a.cfg.ApprovalsMode = v }
func (a *countingAgent) GetReasoningEffort() string                  { return a.cfg.ReasoningEffort }
func (a *countingAgent) SetReasoningEffort(v string)                 { a.cfg.ReasoningEffort = v }
func (a *countingAgent) GetReasoningSummary() string                 { return a.cfg.ReasoningSummary }
func (a *countingAgent) SetReasoningSummary(v string)                { a.cfg.ReasoningSummary = v }
func (a *countingAgent) GetSystemPrompt() string                     { return a.cfg.SystemPrompt }
func (a *countingAgent) SetSystemPrompt(v string)                    { a.cfg.SystemPrompt = v }
func (a *countingAgent) GetHooks() *agent.Hooks                      { return &a.cfg.Hooks }
func (a *countingAgent) SetHooks(v agent.Hooks)                      { a.cfg.Hooks = v }
func (a *countingAgent) GetToolRegistry() agent.ToolRegistryProvider { return nil }
func (a *countingAgent) SetToolRegistry(agent.ToolRegistryProvider)  {}
func (a *countingAgent) GetExtraTools() []llmtypes.Tool              { return a.cfg.ExtraTools }
func (a *countingAgent) SetExtraTools(v []llmtypes.Tool)             { a.cfg.ExtraTools = v }
func (a *countingAgent) Clone() agent.Agent                          { return a }
func (a *countingAgent) Config() agent.AgentConfig                   { return a.cfg }
func (a *countingAgent) CloneWithConfig(cfg agent.AgentConfig) (agent.Agent, error) {
	a.cfg = cfg
	return a, nil
}

// TestRuntimeSupervisor_SyncOnce_DoesNotCancelTeamChildRuns ensures we no longer cancel
// team child runs in syncOnce. Spawned subagents in team mode are allowed to run;
// only lifecycle-driven paths (e.g. approval) deactivate them.
func TestRuntimeSupervisor_SyncOnce_DoesNotCancelTeamChildRuns(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess, parentRun, err := implstore.CreateSession(cfg, "team parent", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	sess.TeamID = "team-1"
	parentRun.Status = types.RunStatusSucceeded
	if err := implstore.SaveRun(cfg, parentRun); err != nil {
		t.Fatalf("SaveRun parent: %v", err)
	}
	childRun := types.NewChildRun(parentRun.RunID, "legacy child", sess.SessionID, 1)
	childRun.Status = types.RunStatusRunning
	if childRun.Runtime == nil {
		childRun.Runtime = &types.RunRuntimeConfig{}
	}
	childRun.Runtime.Role = "Subagent-1"
	if err := implstore.SaveRun(cfg, childRun); err != nil {
		t.Fatalf("SaveRun child: %v", err)
	}
	sess.Runs = append(sess.Runs, childRun.RunID)
	sessionSvc := newSupervisorTestSessionService(t, cfg)
	if err := sessionSvc.SaveSession(context.Background(), sess); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	ts, err := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	taskSvc := pkgtask.NewManager(ts, nil)
	now := time.Now().UTC()
	if err := ts.CreateTask(context.Background(), types.Task{
		TaskID:         "task-active",
		SessionID:      sess.SessionID,
		RunID:          childRun.RunID,
		TeamID:         sess.TeamID,
		AssignedToType: "agent",
		AssignedTo:     childRun.RunID,
		Goal:           "legacy child work",
		Status:         types.TaskStatusActive,
		CreatedAt:      &now,
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	supervisor := &runtimeSupervisor{
		cfg:            cfg,
		taskService:    taskSvc,
		sessionService: sessionSvc,
	}
	supervisor.spawnOverride = func(_ context.Context, sess types.Session, runID string) (*managedRuntime, error) {
		done := make(chan struct{})
		return &managedRuntime{
			runID:     runID,
			sessionID: sess.SessionID,
			cancel:    func() { close(done) },
			done:      done,
		}, nil
	}
	if err := supervisor.loadAndSpawnActiveRuns(context.Background()); err != nil {
		t.Fatalf("loadAndSpawnActiveRuns: %v", err)
	}

	loadedChild, err := sessionSvc.LoadRun(context.Background(), childRun.RunID)
	if err != nil {
		t.Fatalf("LoadRun child: %v", err)
	}
	// We no longer cancel team child runs; they are allowed to run (subagent role class).
	if loadedChild.Status == types.RunStatusCanceled {
		t.Fatalf("child run should not be canceled; got status=%q", loadedChild.Status)
	}
	_ = taskSvc // used by supervisor
}

func TestRuntimeSupervisor_MakeSpawnWorkerFunc_AllowsTeamModeWhenAllowed(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess, parentRun, err := implstore.CreateSession(cfg, "team session", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	sess.TeamID = "team-1"
	sessionSvc := newSupervisorTestSessionService(t, cfg)
	if err := sessionSvc.SaveSession(context.Background(), sess); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	supervisor := &runtimeSupervisor{
		cfg:            cfg,
		sessionService: sessionSvc,
	}
	spawn := supervisor.makeSpawnWorkerFunc(parentRun, "openai/gpt-5-mini", nil)
	childRunID, childRole, err := spawn(context.Background(), "do work", sess.SessionID, parentRun.RunID)
	if err != nil {
		t.Fatalf("spawn in team mode: %v", err)
	}
	if childRunID == "" {
		t.Fatalf("expected non-empty child run ID")
	}
	if strings.TrimSpace(childRole) == "" {
		t.Fatalf("expected canonical child role from spawn callback")
	}
	child, err := sessionSvc.LoadRun(context.Background(), childRunID)
	if err != nil {
		t.Fatalf("LoadRun child: %v", err)
	}
	if child.ParentRunID != parentRun.RunID {
		t.Fatalf("child ParentRunID=%q want %q", child.ParentRunID, parentRun.RunID)
	}
}

func TestRuntimeSupervisor_MakeSpawnWorkerFunc_PrefersRoleSubagentModel(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess, parentRun, err := implstore.CreateSession(cfg, "team session", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	sess.TeamID = "team-1"
	sessionSvc := newSupervisorTestSessionService(t, cfg)
	if err := sessionSvc.SaveSession(context.Background(), sess); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	parentRun.Runtime = &types.RunRuntimeConfig{
		Profile: "startup_team",
		Role:    "coordinator",
		TeamID:  "team-1",
		Model:   "moonshotai/kimi-k2.5",
	}
	if err := sessionSvc.SaveRun(context.Background(), parentRun); err != nil {
		t.Fatalf("SaveRun parent: %v", err)
	}

	supervisor := &runtimeSupervisor{
		cfg:            cfg,
		sessionService: sessionSvc,
		defaultProfile: &profile.Profile{
			ID: "startup_team",
			Team: &profile.TeamConfig{
				Roles: []profile.RoleConfig{
					{Name: "coordinator", Coordinator: true, SubagentModel: "openai/gpt-5-nano"},
				},
			},
		},
	}
	spawn := supervisor.makeSpawnWorkerFunc(parentRun, "moonshotai/kimi-k2.5", nil)
	childRunID, _, err := spawn(context.Background(), "do work", sess.SessionID, parentRun.RunID)
	if err != nil {
		t.Fatalf("spawn in team mode: %v", err)
	}
	child, err := sessionSvc.LoadRun(context.Background(), childRunID)
	if err != nil {
		t.Fatalf("LoadRun child: %v", err)
	}
	if child.Runtime == nil {
		t.Fatalf("child runtime should be set")
	}
	if got := strings.TrimSpace(child.Runtime.Model); got != "openai/gpt-5-nano" {
		t.Fatalf("child runtime model=%q want openai/gpt-5-nano", got)
	}
}

func TestApplySessionModel_SkipsWhenAlreadyOnModel(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess, run, err := implstore.CreateSession(cfg, "same model", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	sessionSvc := newSupervisorTestSessionService(t, cfg)
	if err := sessionSvc.SaveSession(context.Background(), sess); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	ts, err := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	agentImpl := newCountingAgent("openai/gpt-5-mini")
	rtSession, err := agentsession.New(agentsession.Config{
		Agent:     agentImpl,
		Profile:   &profile.Profile{ID: "general"},
		TaskStore: ts,
		SessionID: sess.SessionID,
		RunID:     run.RunID,
	})
	if err != nil {
		t.Fatalf("agentsession.New: %v", err)
	}
	supervisor := &runtimeSupervisor{
		cfg:            cfg,
		taskService:    pkgtask.NewManager(ts, nil),
		sessionService: sessionSvc,
	}
	injectHandle(supervisor, run.RunID, &managedRuntime{
		runID:     run.RunID,
		sessionID: sess.SessionID,
		session:   rtSession,
		model:     "openai/gpt-5-mini",
	}, handleStateRunning)

	applied, err := supervisor.ApplySessionModel(context.Background(), sess.SessionID, "", "openai/gpt-5-mini")
	if err != nil {
		t.Fatalf("ApplySessionModel: %v", err)
	}
	if len(applied) != 1 || applied[0] != run.RunID {
		t.Fatalf("expected runtime model persistence application, got %+v", applied)
	}
	if calls := agentImpl.modelSetCalls(); calls != 0 {
		t.Fatalf("expected SetModel not to be called when model unchanged, got %d calls", calls)
	}
	loadedRun, err := sessionSvc.LoadRun(context.Background(), run.RunID)
	if err != nil {
		t.Fatalf("LoadRun: %v", err)
	}
	if loadedRun.Runtime == nil || loadedRun.Runtime.Model != "openai/gpt-5-mini" {
		t.Fatalf("expected runtime model persisted, got %+v", loadedRun.Runtime)
	}
}

func TestResolveRunModel_PrefersRoleModelInTeamMode(t *testing.T) {
	sess := types.Session{SessionID: "sess-1", TeamID: "team-1", ActiveModel: "session-model"}
	run := types.Run{
		RunID: "run-1",
		Runtime: &types.RunRuntimeConfig{
			TeamID: "team-1",
			Model:  "role-model",
		},
	}
	got, source := resolveRunModel(sess, run, "fallback-model")
	if got != "role-model" {
		t.Fatalf("resolveRunModel(team)=%q want %q", got, "role-model")
	}
	if source != "run" {
		t.Fatalf("resolveRunModel(team) source=%q want %q", source, "run")
	}
}

func TestResolveRunModel_PrefersSessionModelInStandalone(t *testing.T) {
	sess := types.Session{SessionID: "sess-1", ActiveModel: "session-model"}
	run := types.Run{
		RunID: "run-1",
		Runtime: &types.RunRuntimeConfig{
			Model: "run-model",
		},
	}
	got, source := resolveRunModel(sess, run, "fallback-model")
	if got != "session-model" {
		t.Fatalf("resolveRunModel(standalone)=%q want %q", got, "session-model")
	}
	if source != "session" {
		t.Fatalf("resolveRunModel(standalone) source=%q want %q", source, "session")
	}
}

func TestShouldSyncModelFromSession_SkipsTeamAndChildRuns(t *testing.T) {
	teamRun := types.Run{RunID: "r1"}
	teamSess := types.Session{SessionID: "s1", TeamID: "team-1"}
	if shouldSyncModelFromSession(teamRun, teamSess) {
		t.Fatalf("expected team run sync to be disabled")
	}
	childRun := types.Run{RunID: "r2", ParentRunID: "parent"}
	standaloneSess := types.Session{SessionID: "s2"}
	if shouldSyncModelFromSession(childRun, standaloneSess) {
		t.Fatalf("expected child run sync to be disabled")
	}
	if !shouldSyncModelFromSession(types.Run{RunID: "r3"}, standaloneSess) {
		t.Fatalf("expected standalone top-level sync to remain enabled")
	}
}

func TestApplySessionModel_PersistsRuntimeModelEvenWhenAgentAlreadyOnTarget(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess, run, err := implstore.CreateSession(cfg, "persist model", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	run.Runtime = &types.RunRuntimeConfig{Model: "openai/gpt-4.1-mini"}
	if err := implstore.SaveRun(cfg, run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}
	sessionSvc := newSupervisorTestSessionService(t, cfg)
	if err := sessionSvc.SaveSession(context.Background(), sess); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	ts, err := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	agentImpl := newCountingAgent("openai/gpt-5-mini")
	rtSession, err := agentsession.New(agentsession.Config{
		Agent:     agentImpl,
		Profile:   &profile.Profile{ID: "general"},
		TaskStore: ts,
		SessionID: sess.SessionID,
		RunID:     run.RunID,
	})
	if err != nil {
		t.Fatalf("agentsession.New: %v", err)
	}
	supervisor := &runtimeSupervisor{
		cfg:            cfg,
		taskService:    pkgtask.NewManager(ts, nil),
		sessionService: sessionSvc,
	}
	injectHandle(supervisor, run.RunID, &managedRuntime{
		runID:     run.RunID,
		sessionID: sess.SessionID,
		session:   rtSession,
		model:     "openai/gpt-5-mini",
	}, handleStateRunning)

	applied, err := supervisor.ApplySessionModel(context.Background(), sess.SessionID, "", "openai/gpt-5-mini")
	if err != nil {
		t.Fatalf("ApplySessionModel: %v", err)
	}
	if len(applied) != 1 || applied[0] != run.RunID {
		t.Fatalf("expected runtime model persistence to be recorded, got %+v", applied)
	}
	if calls := agentImpl.modelSetCalls(); calls != 0 {
		t.Fatalf("expected SetModel not to be called when model unchanged, got %d calls", calls)
	}
	loadedRun, err := sessionSvc.LoadRun(context.Background(), run.RunID)
	if err != nil {
		t.Fatalf("LoadRun: %v", err)
	}
	if loadedRun.Runtime == nil || loadedRun.Runtime.Model != "openai/gpt-5-mini" {
		t.Fatalf("expected runtime model to persist new value, got %+v", loadedRun.Runtime)
	}
}

// TestGetRunState_PausedStatusNotMaskedByWorker verifies that GetRunState returns
// EffectiveStatus="paused" (not "running") when the persisted status is paused,
// even when a worker goroutine is present for that run.
func TestGetRunState_PausedStatusNotMaskedByWorker(t *testing.T) {
	sessionSvc := &ctxCheckingSessionService{
		loadRun: func(runID string) (types.Run, error) {
			return types.Run{RunID: runID, Status: types.RunStatusPaused}, nil
		},
	}
	taskSvc := &ctxCheckingTaskService{}
	supervisor := &runtimeSupervisor{
		sessionService: sessionSvc,
		taskService:    taskSvc,
	}

	// Without a worker: paused should remain paused.
	st, err := supervisor.GetRunState(context.Background(), "sess-1", "run-1")
	if err != nil {
		t.Fatalf("GetRunState: %v", err)
	}
	if st.EffectiveStatus != types.RunStatusPaused {
		t.Errorf("EffectiveStatus without worker: got %q, want %q", st.EffectiveStatus, types.RunStatusPaused)
	}
	if !st.PausedFlag {
		t.Errorf("PausedFlag without worker: got false, want true")
	}

	// With a worker present: paused should still be paused, not masked as running.
	injectHandle(supervisor, "run-1", &managedRuntime{runID: "run-1"}, handleStateRunning)
	st, err = supervisor.GetRunState(context.Background(), "sess-1", "run-1")
	if err != nil {
		t.Fatalf("GetRunState with worker: %v", err)
	}
	if st.EffectiveStatus != types.RunStatusPaused {
		t.Errorf("EffectiveStatus with worker: got %q, want %q (paused must not be masked as running)", st.EffectiveStatus, types.RunStatusPaused)
	}
	if !st.PausedFlag {
		t.Errorf("PausedFlag with worker: got false, want true")
	}
	if !st.WorkerPresent {
		t.Errorf("WorkerPresent: got false, want true")
	}
}
func TestRuntimeSupervisor_Run_UsesTaskWakeToSyncRuns(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess, run1, err := implstore.CreateSession(cfg, "wake sync", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	sessionSvc := newSupervisorTestSessionService(t, cfg)
	ts, err := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	taskSvc := pkgtask.NewManager(ts, nil)

	spawned := make(chan string, 8)
	supervisor := newRuntimeSupervisor(runtimeSupervisorConfig{
		Cfg:            cfg,
		TaskService:    taskSvc,
		SessionService: sessionSvc,
	})
	supervisor.spawnOverride = func(_ context.Context, sess types.Session, runID string) (*managedRuntime, error) {
		done := make(chan struct{})
		spawned <- runID
		return &managedRuntime{
			runID:     runID,
			sessionID: sess.SessionID,
			cancel:    func() { close(done) },
			done:      done,
		}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan struct{})
	go func() {
		supervisor.Run(ctx)
		close(runDone)
	}()

	select {
	case got := <-spawned:
		if got != run1.RunID {
			t.Fatalf("initial spawned runID=%q want %q", got, run1.RunID)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("expected initial sync spawn")
	}

	run2 := types.NewRun("wake sync second", 8*1024, sess.SessionID)
	if err := sessionSvc.SaveRun(context.Background(), run2); err != nil {
		t.Fatalf("SaveRun run2: %v", err)
	}
	if _, err := sessionSvc.AddRunToSession(context.Background(), sess.SessionID, run2.RunID); err != nil {
		t.Fatalf("AddRunToSession run2: %v", err)
	}
	taskSvc.NotifyWake("", run2.RunID)

	select {
	case got := <-spawned:
		if got != run2.RunID {
			t.Fatalf("woken spawned runID=%q want %q", got, run2.RunID)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("expected wake-driven sync spawn for run2")
	}

	cancel()
	select {
	case <-runDone:
	case <-time.After(1 * time.Second):
		t.Fatalf("supervisor run did not stop on context cancel")
	}
}

func TestRuntimeSupervisor_LifecycleOrdering(t *testing.T) {
	t.Run("resume then immediate pause", func(t *testing.T) {
		cfg := config.Config{DataDir: t.TempDir()}
		_, run, err := implstore.CreateSession(cfg, "ordering", 8*1024)
		if err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		run.Status = types.RunStatusPaused
		if err := implstore.SaveRun(cfg, run); err != nil {
			t.Fatalf("SaveRun paused: %v", err)
		}
		sessionSvc := newSupervisorTestSessionService(t, cfg)
		taskStore, err := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
		if err != nil {
			t.Fatalf("NewSQLiteTaskStore: %v", err)
		}
		taskSvc := pkgtask.NewManager(taskStore, nil)

		cancelled := make(chan struct{})
		supervisor := newRuntimeSupervisor(runtimeSupervisorConfig{
			Cfg:            cfg,
			TaskService:    taskSvc,
			SessionService: sessionSvc,
		})
		supervisor.spawnOverride = func(_ context.Context, sess types.Session, runID string) (*managedRuntime, error) {
			done := make(chan struct{})
			return &managedRuntime{
				runID:     runID,
				sessionID: sess.SessionID,
				cancel:    func() { close(done); close(cancelled) },
				done:      done,
			}, nil
		}

		if err := supervisor.ResumeRun(context.Background(), run.RunID); err != nil {
			t.Fatalf("ResumeRun: %v", err)
		}
		if err := supervisor.pauseRun(context.Background(), run.RunID); err != nil {
			t.Fatalf("pauseRun: %v", err)
		}
		drainSupervisorCmds(supervisor)
		select {
		case <-cancelled:
		case <-time.After(1 * time.Second):
			t.Fatalf("expected spawned worker to be cancelled")
		}
		processNextSupervisorCmd(t, supervisor)

		loaded, err := sessionSvc.LoadRun(context.Background(), run.RunID)
		if err != nil {
			t.Fatalf("LoadRun: %v", err)
		}
		if loaded.Status != types.RunStatusPaused {
			t.Fatalf("run status=%q want %q", loaded.Status, types.RunStatusPaused)
		}
		if _, ok := supervisor.getSnapshot(run.RunID); ok {
			t.Fatalf("expected no live snapshot after pause drain")
		}
	})

	t.Run("stop during spawning", func(t *testing.T) {
		cfg := config.Config{DataDir: t.TempDir()}
		sess, run, err := implstore.CreateSession(cfg, "spawning", 8*1024)
		if err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		sessionSvc := newSupervisorTestSessionService(t, cfg)
		taskStore, err := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
		if err != nil {
			t.Fatalf("NewSQLiteTaskStore: %v", err)
		}
		taskSvc := pkgtask.NewManager(taskStore, nil)

		releaseSpawn := make(chan struct{})
		cancelled := make(chan struct{})
		supervisor := newRuntimeSupervisor(runtimeSupervisorConfig{
			Cfg:            cfg,
			TaskService:    taskSvc,
			SessionService: sessionSvc,
		})
		supervisor.spawnOverride = func(_ context.Context, sess types.Session, runID string) (*managedRuntime, error) {
			<-releaseSpawn
			done := make(chan struct{})
			return &managedRuntime{
				runID:     runID,
				sessionID: sess.SessionID,
				cancel:    func() { close(done); close(cancelled) },
				done:      done,
			}, nil
		}

		errCh := make(chan error, 1)
		go func() {
			errCh <- supervisor.handleSpawn(context.Background(), supervisorCmd{
				kind:      cmdSpawn,
				runID:     run.RunID,
				sessionID: sess.SessionID,
				sess:      &sess,
			})
		}()

		deadline := time.Now().Add(1 * time.Second)
		for time.Now().Before(deadline) {
			snap, ok := supervisor.getSnapshot(run.RunID)
			if ok && snap.HandleState == handleStateSpawning {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}

		supervisor.handleStop(supervisorCmd{kind: cmdStop, runID: run.RunID, paused: true})
		run.Status = types.RunStatusPaused
		if err := sessionSvc.SaveRun(context.Background(), run); err != nil {
			t.Fatalf("SaveRun paused: %v", err)
		}
		close(releaseSpawn)

		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("handleSpawn: %v", err)
			}
		case <-time.After(1 * time.Second):
			t.Fatalf("timed out waiting for spawn completion")
		}
		select {
		case <-cancelled:
		case <-time.After(1 * time.Second):
			t.Fatalf("expected cancel after spawn completion")
		}
		deadline = time.Now().Add(1 * time.Second)
		for time.Now().Before(deadline) {
			drainSupervisorCmds(supervisor)
			if _, ok := supervisor.getSnapshot(run.RunID); !ok {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		if _, ok := supervisor.getSnapshot(run.RunID); ok {
			t.Fatalf("expected handle removal after stop during spawning")
		}
	})

	t.Run("stop then resume respawns worker", func(t *testing.T) {
		cfg := config.Config{DataDir: t.TempDir()}
		sess, run, err := implstore.CreateSession(cfg, "stop-resume", 8*1024)
		if err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		sessionSvc := newSupervisorTestSessionService(t, cfg)
		taskStore, err := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
		if err != nil {
			t.Fatalf("NewSQLiteTaskStore: %v", err)
		}
		taskSvc := pkgtask.NewManager(taskStore, nil)

		supervisor := newRuntimeSupervisor(runtimeSupervisorConfig{
			Cfg:            cfg,
			TaskService:    taskSvc,
			SessionService: sessionSvc,
		})

		spawnCount := 0
		cancelled := make(chan struct{}, 1)
		supervisor.spawnOverride = func(_ context.Context, loaded types.Session, runID string) (*managedRuntime, error) {
			spawnCount++
			done := make(chan struct{})
			return &managedRuntime{
				runID:     runID,
				sessionID: loaded.SessionID,
				cancel: func() {
					select {
					case cancelled <- struct{}{}:
					default:
					}
					close(done)
				},
				done: done,
			}, nil
		}

		if err := supervisor.handleSpawn(context.Background(), supervisorCmd{
			kind:      cmdSpawn,
			runID:     run.RunID,
			sessionID: sess.SessionID,
			sess:      &sess,
		}); err != nil {
			t.Fatalf("handleSpawn: %v", err)
		}
		if err := supervisor.StopRun(context.Background(), run.RunID); err != nil {
			t.Fatalf("StopRun: %v", err)
		}
		drainSupervisorCmds(supervisor)
		select {
		case <-cancelled:
		case <-time.After(1 * time.Second):
			t.Fatalf("expected running worker to be cancelled by stop")
		}

		stopped, err := sessionSvc.LoadRun(context.Background(), run.RunID)
		if err != nil {
			t.Fatalf("LoadRun after stop: %v", err)
		}
		if stopped.Status != types.RunStatusPaused {
			t.Fatalf("run status after stop=%q want %q", stopped.Status, types.RunStatusPaused)
		}

		if err := supervisor.ResumeRun(context.Background(), run.RunID); err != nil {
			t.Fatalf("ResumeRun after stop: %v", err)
		}
		drainSupervisorCmds(supervisor)

		resumed, err := sessionSvc.LoadRun(context.Background(), run.RunID)
		if err != nil {
			t.Fatalf("LoadRun after resume: %v", err)
		}
		if resumed.Status != types.RunStatusRunning {
			t.Fatalf("run status after resume=%q want %q", resumed.Status, types.RunStatusRunning)
		}
		if spawnCount != 2 {
			t.Fatalf("spawn count=%d want 2", spawnCount)
		}
	})

	t.Run("double resume spawns one worker", func(t *testing.T) {
		cfg := config.Config{DataDir: t.TempDir()}
		_, run, err := implstore.CreateSession(cfg, "double resume", 8*1024)
		if err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		run.Status = types.RunStatusPaused
		if err := implstore.SaveRun(cfg, run); err != nil {
			t.Fatalf("SaveRun paused: %v", err)
		}
		sessionSvc := newSupervisorTestSessionService(t, cfg)
		taskStore, err := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
		if err != nil {
			t.Fatalf("NewSQLiteTaskStore: %v", err)
		}
		taskSvc := pkgtask.NewManager(taskStore, nil)

		spawnCount := 0
		supervisor := newRuntimeSupervisor(runtimeSupervisorConfig{
			Cfg:            cfg,
			TaskService:    taskSvc,
			SessionService: sessionSvc,
		})
		supervisor.spawnOverride = func(_ context.Context, sess types.Session, runID string) (*managedRuntime, error) {
			spawnCount++
			done := make(chan struct{})
			return &managedRuntime{
				runID:     runID,
				sessionID: sess.SessionID,
				cancel:    func() { close(done) },
				done:      done,
			}, nil
		}

		if err := supervisor.ResumeRun(context.Background(), run.RunID); err != nil {
			t.Fatalf("ResumeRun #1: %v", err)
		}
		if err := supervisor.ResumeRun(context.Background(), run.RunID); err != nil {
			t.Fatalf("ResumeRun #2: %v", err)
		}
		drainSupervisorCmds(supervisor)

		if spawnCount != 1 {
			t.Fatalf("spawnCount=%d want 1", spawnCount)
		}
	})

	t.Run("stale exit watcher discarded", func(t *testing.T) {
		supervisor := newRuntimeSupervisor(runtimeSupervisorConfig{})
		oldDone := make(chan struct{})
		newDone := make(chan struct{})
		oldHandle := &runHandle{
			runID:     "run-1",
			sessionID: "sess-1",
			rt:        &managedRuntime{runID: "run-1", sessionID: "sess-1", done: oldDone},
			state:     handleStateRunning,
		}
		newHandle := &runHandle{
			runID:     "run-1",
			sessionID: "sess-1",
			rt:        &managedRuntime{runID: "run-1", sessionID: "sess-1", done: newDone},
			state:     handleStateRunning,
		}
		supervisor.handles["run-1"] = newHandle
		supervisor.updateSnapshot("run-1", newHandle, types.RunStatusRunning)

		if err := supervisor.handleWorkerExited(context.Background(), supervisorCmd{
			kind:   cmdWorkerExited,
			runID:  "run-1",
			handle: oldHandle,
		}); err != nil {
			t.Fatalf("handleWorkerExited: %v", err)
		}

		snap, ok := supervisor.getSnapshot("run-1")
		if !ok || snap.rt != newHandle.rt {
			t.Fatalf("expected stale exit to leave newer handle intact")
		}
	})
}
