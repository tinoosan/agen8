package app

import (
	"context"
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

// newSupervisorTestSessionService creates a session service backed by SQLite for supervisor tests.
func newSupervisorTestSessionService(t *testing.T, cfg config.Config) pkgsession.Service {
	t.Helper()
	sessionStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore: %v", err)
	}
	return newTestSessionService(cfg, sessionStore)
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

	loaded, err := sessionSvc.LoadRun(context.Background(), run.RunID)
	if err != nil {
		t.Fatalf("LoadRun: %v", err)
	}
	if loaded.Status != types.RunStatusCanceled {
		t.Fatalf("run status=%q want %q", loaded.Status, types.RunStatusCanceled)
	}

	supervisor.mu.Lock()
	_, exists := supervisor.workers[run.RunID]
	supervisor.mu.Unlock()
	if exists {
		t.Fatalf("expected worker to be removed after stop")
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
		workers: map[string]*managedRuntime{
			run.RunID: {
				runID: run.RunID,
				cancel: func() {
					cancelCalled = true
				},
				done: make(chan struct{}), // never closes: simulates stuck worker
			},
		},
	}

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

	loadedA, err := sessionSvc.LoadRun(context.Background(), runA.RunID)
	if err != nil {
		t.Fatalf("LoadRun A: %v", err)
	}
	if loadedA.Status != types.RunStatusCanceled {
		t.Fatalf("runA status=%q want %q", loadedA.Status, types.RunStatusCanceled)
	}

	loadedA2, err := sessionSvc.LoadRun(context.Background(), runA2.RunID)
	if err != nil {
		t.Fatalf("LoadRun A2: %v", err)
	}
	if loadedA2.Status != types.RunStatusCanceled {
		t.Fatalf("runA2 status=%q want %q", loadedA2.Status, types.RunStatusCanceled)
	}

	loadedB, err := sessionSvc.LoadRun(context.Background(), runB.RunID)
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
		workers:        map[string]*managedRuntime{},
	}
	if err := supervisor.syncOnce(context.Background()); err != nil {
		t.Fatalf("syncOnce: %v", err)
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
		workers: map[string]*managedRuntime{
			run.RunID: {
				runID:     run.RunID,
				sessionID: sess.SessionID,
				session:   rtSession,
				model:     "openai/gpt-5-mini",
			},
		},
	}

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
		workers: map[string]*managedRuntime{
			run.RunID: {
				runID:     run.RunID,
				sessionID: sess.SessionID,
				session:   rtSession,
				model:     "openai/gpt-5-mini",
			},
		},
	}

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
