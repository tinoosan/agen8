package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	implstore "github.com/tinoosan/agen8/internal/store"
	"github.com/tinoosan/agen8/pkg/agent"
	hosttools "github.com/tinoosan/agen8/pkg/agent/hosttools"
	agentsession "github.com/tinoosan/agen8/pkg/agent/session"
	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/emit"
	"github.com/tinoosan/agen8/pkg/events"
	"github.com/tinoosan/agen8/pkg/fsutil"
	llmtypes "github.com/tinoosan/agen8/pkg/llm/types"
	"github.com/tinoosan/agen8/pkg/profile"
	"github.com/tinoosan/agen8/pkg/prompts"
	"github.com/tinoosan/agen8/pkg/protocol"
	"github.com/tinoosan/agen8/pkg/runtime"
	eventsvc "github.com/tinoosan/agen8/pkg/services/events"
	pkgsession "github.com/tinoosan/agen8/pkg/services/session"
	pkgsoul "github.com/tinoosan/agen8/pkg/services/soul"
	pkgtask "github.com/tinoosan/agen8/pkg/services/task"
	"github.com/tinoosan/agen8/pkg/services/team"
	pkgstore "github.com/tinoosan/agen8/pkg/store"
	"github.com/tinoosan/agen8/pkg/types"
)

type runtimeSupervisorConfig struct {
	Cfg              config.Config
	Resolved         RunChatOptions
	PollInterval     time.Duration
	TaskService      pkgtask.TaskServiceForSupervisor
	SessionService   pkgsession.Service
	EventsStore      events.StoreAppender
	MemoryStore      pkgstore.DailyMemoryStore
	ConstructorStore pkgstore.ConstructorStateStore
	LLMClient        llmtypes.LLMClient
	Notifier         agent.Notifier
	WorkdirAbs       string
	DefaultProfile   *profile.Profile
	SoulService      pkgsoul.Service
}

type runtimeSupervisor struct {
	cfg              config.Config
	resolved         RunChatOptions
	pollInterval     time.Duration
	taskService      pkgtask.TaskServiceForSupervisor
	sessionService   pkgsession.Service
	eventsStore      events.StoreAppender
	memoryStore      pkgstore.DailyMemoryStore
	constructorStore pkgstore.ConstructorStateStore
	llmClient        llmtypes.LLMClient
	notifier         agent.Notifier
	workdirAbs       string
	defaultProfile   *profile.Profile
	soulService      pkgsoul.Service

	cmdCh               chan supervisorCmd
	handles             map[string]*runHandle
	snapshotMu          sync.RWMutex
	snapshots           map[string]runStateSnapshot
	lastRoutingRepairAt time.Time

	spawnOverride func(context.Context, types.Session, string) (*managedRuntime, error)
}

type supervisorCmdKind int

const (
	cmdSpawn supervisorCmdKind = iota
	cmdStop
	cmdWorkerExited
	cmdRepairDrift
)

type supervisorCmd struct {
	kind      supervisorCmdKind
	runID     string
	sessionID string
	sess      *types.Session
	handle    *runHandle
	paused    bool
}

type handleState int

const (
	handleStateSpawning handleState = iota
	handleStateRunning
	handleStatePaused
	handleStateDraining
)

type runHandle struct {
	runID       string
	sessionID   string
	rt          *managedRuntime
	state       handleState
	stopPending bool
	stopPaused  bool
}

type runStateSnapshot struct {
	RunID           string
	SessionID       string
	HandleState     handleState
	PersistedStatus string
	WorkerPresent   bool
	Model           string
	LastHeartbeatAt time.Time
	rt              *managedRuntime
}

const defaultSubagentAwaitingReviewTimeout = 30 * time.Minute
const defaultWorkerShutdownTimeout = 10 * time.Second
const sessionRunActionConcurrency = 10

const (
	taskMetaSource          = "source"
	taskMetaReviewDecision  = "reviewDecision"
	taskMetaSourceRunID     = "sourceRunId"
	taskSourceSubagentCB    = "subagent.callback"
	taskSourceSubagentBatch = "subagent.batch.callback"
	reviewDecisionApprove   = "approve"
	lifecycleDeactivated    = "deactivated"
	lifecycleArchived       = "archived"
	stopReasonArchived      = "archived"
	controlSourceSession    = "session"
)

type taskWakeSubscriber interface {
	SubscribeWake(teamID, runID string) (<-chan struct{}, func())
}

type taskWakeNotifier interface {
	NotifyWake(teamID, runID string)
}

type sessionWakeSubscriber interface {
	SubscribeWake(sessionID, runID string) (<-chan struct{}, func())
}

type routingDriftRepairer interface {
	RepairRoutingDrift(ctx context.Context, limit int) (int, error)
}

type managedRuntime struct {
	runID           string
	sessionID       string
	session         *agentsession.Session
	cancel          context.CancelFunc
	done            <-chan struct{}
	modelMu         sync.Mutex
	model           string
	heartbeatMu     sync.Mutex
	lastHeartbeatAt time.Time
}

func resolveRunModel(sess types.Session, run types.Run, fallbackModel string) (string, string) {
	isChildRun := strings.TrimSpace(run.ParentRunID) != ""
	runTeamID := ""
	if run.Runtime != nil {
		runTeamID = strings.TrimSpace(run.Runtime.TeamID)
	}
	isTeamRun := runTeamID != "" || strings.TrimSpace(sess.TeamID) != ""
	runModel := ""
	if run.Runtime != nil {
		runModel = strings.TrimSpace(run.Runtime.Model)
	}
	sessionModel := strings.TrimSpace(sess.ActiveModel)
	if isChildRun {
		if runModel != "" {
			return runModel, "run"
		}
	}
	if isTeamRun {
		if runModel != "" {
			return runModel, "run"
		}
		if sessionModel != "" {
			return sessionModel, "session"
		}
	} else {
		if sessionModel != "" {
			return sessionModel, "session"
		}
		if runModel != "" {
			return runModel, "run"
		}
	}
	return strings.TrimSpace(fallbackModel), "profile"
}

// isSubagentRoleClass returns true when the run is a child (subagent) with the canonical
// Subagent-N role. Used to branch on role class instead of string matching in spawnManagedRun.
func isSubagentRoleClass(run types.Run) bool {
	if strings.TrimSpace(run.ParentRunID) == "" {
		return false
	}
	if run.Runtime == nil {
		return false
	}
	r := strings.TrimSpace(run.Runtime.Role)
	if r == "" {
		return false
	}
	return strings.HasPrefix(strings.ToLower(r), "subagent-")
}

func shouldSyncModelFromSession(run types.Run, loaded types.Session) bool {
	if strings.TrimSpace(run.ParentRunID) != "" {
		return false
	}
	// Team runs should only change model via explicit control.setModel.
	if strings.TrimSpace(loaded.TeamID) != "" {
		return false
	}
	return true
}

func (m *managedRuntime) CurrentModel() string {
	if m == nil {
		return ""
	}
	m.modelMu.Lock()
	defer m.modelMu.Unlock()
	return strings.TrimSpace(m.model)
}

func (m *managedRuntime) SetCurrentModel(model string) {
	if m == nil {
		return
	}
	m.modelMu.Lock()
	m.model = strings.TrimSpace(model)
	m.modelMu.Unlock()
}

func (m *managedRuntime) TouchHeartbeat() {
	if m == nil {
		return
	}
	m.heartbeatMu.Lock()
	m.lastHeartbeatAt = time.Now().UTC()
	m.heartbeatMu.Unlock()
}

func (m *managedRuntime) LastHeartbeatAt() time.Time {
	if m == nil {
		return time.Time{}
	}
	m.heartbeatMu.Lock()
	defer m.heartbeatMu.Unlock()
	return m.lastHeartbeatAt
}

// subagentCleanupNotifier stops and finalizes the subagent run when the parent
// completes a subagent callback task successfully (ephemeral subagent cleanup).
type subagentCleanupNotifier struct {
	supervisor *runtimeSupervisor
	store      pkgtask.TaskServiceForSupervisor
	next       agent.Notifier
}

func (n *subagentCleanupNotifier) Notify(ctx context.Context, task types.Task, tr types.TaskResult) error {
	if n != nil && n.supervisor != nil {
		source, _ := task.Metadata[taskMetaSource].(string)
		if source == taskSourceSubagentCB && tr.Status == types.TaskStatusSucceeded {
			// Only cleanup on explicit approval or legacy no-review-gate callbacks.
			// "retry" keeps the child alive; "escalate" defers cleanup to escalation resolution.
			reviewDecision, _ := task.Metadata[taskMetaReviewDecision].(string)
			if reviewDecision == "" || reviewDecision == reviewDecisionApprove {
				if runID, ok := task.Metadata[taskMetaSourceRunID].(string); ok && strings.TrimSpace(runID) != "" {
					runID = strings.TrimSpace(runID)
					n.supervisor.deactivateAndArchiveSubagent(ctx, runID)
				}
			}
		}
		if source == taskSourceSubagentBatch && tr.Status == types.TaskStatusSucceeded && n.store != nil {
			decisions, _ := task.Metadata["batchItemDecisions"].(map[string]any)
			for callbackTaskID, rawDecision := range decisions {
				if !strings.EqualFold(strings.TrimSpace(fmt.Sprint(rawDecision)), reviewDecisionApprove) {
					continue
				}
				callbackTask, err := n.store.GetTask(ctx, strings.TrimSpace(callbackTaskID))
				if err != nil {
					continue
				}
				runID, _ := callbackTask.Metadata[taskMetaSourceRunID].(string)
				runID = strings.TrimSpace(runID)
				if runID == "" {
					continue
				}
				n.supervisor.deactivateAndArchiveSubagent(ctx, runID)
			}
		}
	}
	if n != nil && n.next != nil {
		return n.next.Notify(ctx, task, tr)
	}
	return nil
}

func (s *runtimeSupervisor) deactivateAndArchiveSubagent(ctx context.Context, runID string) {
	run, err := s.sessionService.LoadRun(ctx, runID)
	if err == nil && run.Runtime != nil {
		run.Runtime.LifecycleState = lifecycleDeactivated
		if err := s.sessionService.SaveRun(ctx, run); err != nil {
			slog.Error("deactivate subagent: save run", "component", "supervisor", "run_id", runID, "error", err)
		}
	}

	if err := s.trySendCmd(supervisorCmd{
		kind:      cmdStop,
		runID:     strings.TrimSpace(runID),
		sessionID: strings.TrimSpace(run.SessionID),
		paused:    false,
	}); err != nil {
		slog.Error("deactivate subagent: enqueue stop", "component", "supervisor", "run_id", runID, "error", err)
	}

	timeout := time.After(s.workerShutdownTimeout())
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		snap, ok := s.getSnapshot(strings.TrimSpace(runID))
		if !ok || (!snap.WorkerPresent && snap.HandleState != handleStateSpawning && snap.HandleState != handleStateDraining) {
			break
		}
		select {
		case <-timeout:
			slog.Warn("timed out waiting for worker shutdown during subagent cleanup", "component", "supervisor", "run_id", runID, "timeout", s.workerShutdownTimeout())
			goto archive
		case <-ticker.C:
		}
	}

archive:

	if run, err := s.sessionService.LoadRun(ctx, runID); err == nil {
		if run.Runtime != nil {
			run.Runtime.LifecycleState = lifecycleArchived
			if err := s.sessionService.SaveRun(ctx, run); err != nil {
				slog.Error("archive subagent: save run", "component", "supervisor", "run_id", runID, "error", err)
			}
		}
		// stop run result is best-effort
		// Stop result is best-effort; worker may already be stopped
			_, _ = s.sessionService.StopRun(ctx, runID, types.RunStatusSucceeded, stopReasonArchived)
	}
}

func newRuntimeSupervisor(cfg runtimeSupervisorConfig) *runtimeSupervisor {
	poll := cfg.PollInterval
	if poll <= 0 {
		poll = 1 * time.Second // Faster inbox poll so callbacks and new tasks are picked up sooner
	}
	return &runtimeSupervisor{
		cfg:              cfg.Cfg,
		resolved:         cfg.Resolved,
		pollInterval:     poll,
		taskService:      cfg.TaskService,
		sessionService:   cfg.SessionService,
		eventsStore:      cfg.EventsStore,
		memoryStore:      cfg.MemoryStore,
		constructorStore: cfg.ConstructorStore,
		llmClient:        cfg.LLMClient,
		notifier:         cfg.Notifier,
		workdirAbs:       cfg.WorkdirAbs,
		defaultProfile:   cfg.DefaultProfile,
		soulService:      cfg.SoulService,
		cmdCh:            make(chan supervisorCmd, 256),
		handles:          map[string]*runHandle{},
		snapshots:        map[string]runStateSnapshot{},
	}
}

func (s *runtimeSupervisor) subagentAwaitingReviewTimeout() time.Duration {
	if s == nil {
		return defaultSubagentAwaitingReviewTimeout
	}
	raw := strings.TrimSpace(os.Getenv("AGEN8_EPHEMERAL_IDLE_TIMEOUT"))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("AGEN8_SUBAGENT_AWAITING_REVIEW_TIMEOUT"))
	}
	if raw == "" {
		return defaultSubagentAwaitingReviewTimeout
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return defaultSubagentAwaitingReviewTimeout
	}
	return d
}

func (s *runtimeSupervisor) workerShutdownTimeout() time.Duration {
	if s == nil {
		return defaultWorkerShutdownTimeout
	}
	raw := strings.TrimSpace(os.Getenv("AGEN8_WORKER_SHUTDOWN_TIMEOUT"))
	if raw == "" {
		return defaultWorkerShutdownTimeout
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return defaultWorkerShutdownTimeout
	}
	return d
}

func waitWorkerDone(done <-chan struct{}, timeout time.Duration) bool {
	if done == nil {
		return true
	}
	if timeout <= 0 {
		<-done
		return true
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	}
}

func workerDone(done <-chan struct{}) bool {
	if done == nil {
		return true
	}
	select {
	case <-done:
		return true
	default:
		return false
	}
}

func (s *runtimeSupervisor) Run(ctx context.Context) {
	if s == nil {
		return
	}
	s.ensureLoopState()
	if err := s.loadAndSpawnActiveRuns(ctx); err != nil {
		slog.Error("runtime supervisor startup load failed", "component", "supervisor", "error", err)
	}

	var supervisorWakeCh <-chan struct{}
	var wakeCancel func()
	if wakeSub, ok := s.taskService.(taskWakeSubscriber); ok && wakeSub != nil {
		supervisorWakeCh, wakeCancel = wakeSub.SubscribeWake("", "")
	}
	if wakeCancel != nil {
		defer wakeCancel()
	}

	repairTicker := time.NewTicker(10 * time.Second)
	defer repairTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.stopAllHandles()
			return
		case cmd := <-s.cmdCh:
			s.processCmd(ctx, cmd)
		case <-supervisorWakeCh:
			s.handleTaskWake(ctx)
		case <-repairTicker.C:
			s.maybeRepairRoutingDrift(ctx)
		}
	}
}

func (s *runtimeSupervisor) runWithPollingFallback(ctx context.Context) {
	t := time.NewTicker(1 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			s.stopAll()
			return
		case <-t.C:
			if err := s.syncOnce(ctx); err != nil {
				slog.Warn("runtime supervisor sync failed", "component", "supervisor", "error", err)
			}
		}
	}
}

func (s *runtimeSupervisor) stopAll() {
	s.stopAllHandles()
}

func (s *runtimeSupervisor) syncOnce(ctx context.Context) error {
	if s == nil || s.sessionService == nil {
		return nil
	}
	runs, err := s.sessionService.ListRunsByStatus(ctx, []string{types.RunStatusRunning, types.RunStatusPaused})
	if err != nil {
		return err
	}
	teamIDs := map[string]struct{}{}
	for _, run := range runs {
		sess, lerr := s.loadSessionWithTimeout(ctx, strings.TrimSpace(run.SessionID))
		if lerr != nil {
			slog.Warn("load session for run", "component", "supervisor", "run_id", run.RunID, "error", lerr)
			continue
		}
		teamID := strings.TrimSpace(sess.TeamID)
		if teamID != "" {
			teamIDs[teamID] = struct{}{}
		}
		if teamID != "" && run.Runtime != nil && strings.TrimSpace(run.Runtime.Role) == "" {
			if _, roleByRun := loadTeamManifestRunRolesFromStore(ctx, team.NewFileManifestStore(s.cfg), teamID); len(roleByRun) != 0 {
				if role := strings.TrimSpace(roleByRun[strings.TrimSpace(run.RunID)]); role != "" {
					run.Runtime.Role = role
					// Best-effort: save run state; errors are logged but not fatal
		if err := s.sessionService.SaveRun(ctx, run); err != nil {
			slog.Debug("failed to save run on stop", "runID", run.RunID, "error", err)
		}
				}
			}
		}
		if err := s.handleSpawn(ctx, supervisorCmd{
			kind:      cmdSpawn,
			runID:     strings.TrimSpace(run.RunID),
			sessionID: strings.TrimSpace(run.SessionID),
			sess:      &sess,
		}); err != nil {
			slog.Error("managed run start failed", "component", "supervisor", "run_id", run.RunID, "error", err)
		}
	}
	for teamID := range teamIDs {
		if err := s.reconcileTeamRoleReplicas(ctx, teamID); err != nil {
			slog.Warn("role replica reconcile failed", "component", "supervisor", "team_id", teamID, "error", err)
		}
	}
	s.maybeRepairRoutingDrift(ctx)
	return nil
}

func workerClassForRun(run types.Run) string {
	if run.Runtime != nil {
		switch strings.ToLower(strings.TrimSpace(run.Runtime.WorkerClass)) {
		case "persistent":
			return "persistent"
		case "ephemeral":
			return "ephemeral"
		}
	}
	if strings.TrimSpace(run.ParentRunID) != "" {
		return "ephemeral"
	}
	return "persistent"
}

func stableRunLess(a, b types.Run) bool {
	at := time.Time{}
	if a.StartedAt != nil {
		at = a.StartedAt.UTC()
	}
	bt := time.Time{}
	if b.StartedAt != nil {
		bt = b.StartedAt.UTC()
	}
	if !at.Equal(bt) {
		return at.Before(bt)
	}
	return strings.TrimSpace(a.RunID) < strings.TrimSpace(b.RunID)
}

func (s *runtimeSupervisor) reconcileTeamRoleReplicas(ctx context.Context, teamID string) error {
	if s == nil || s.sessionService == nil {
		return nil
	}
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		return nil
	}
	store := team.NewFileManifestStore(s.cfg)
	manifest, err := store.Load(ctx, teamID)
	if err != nil || manifest == nil {
		return err
	}
	desired := cloneDesiredReplicas(manifest.DesiredReplicasByRole)
	if len(desired) == 0 {
		return nil
	}
	runs, err := s.sessionService.ListRunsByStatus(ctx, []string{types.RunStatusRunning, types.RunStatusPaused})
	if err != nil {
		return err
	}
	roleRuns := map[string][]types.Run{}
	for _, run := range runs {
		if strings.TrimSpace(run.ParentRunID) != "" {
			continue
		}
		if run.Runtime == nil || strings.TrimSpace(run.Runtime.TeamID) != teamID {
			continue
		}
		if workerClassForRun(run) != "persistent" {
			continue
		}
		role := strings.TrimSpace(run.Runtime.Role)
		if role == "" {
			continue
		}
		roleRuns[role] = append(roleRuns[role], run)
	}

	manifestChanged := false
	for role, target := range desired {
		role = strings.TrimSpace(role)
		if role == "" || target <= 0 {
			continue
		}
		current := roleRuns[role]
		sort.SliceStable(current, func(i, j int) bool { return stableRunLess(current[i], current[j]) })
		if len(current) < target {
			needed := target - len(current)
			for i := 0; i < needed; i++ {
				newRun, newSessionID, spawnErr := s.spawnPersistentReplica(ctx, teamID, role, manifest)
				if spawnErr != nil {
					return spawnErr
				}
				manifest.Roles = append(manifest.Roles, team.RoleRecord{
					RoleName:  role,
					RunID:     strings.TrimSpace(newRun.RunID),
					SessionID: strings.TrimSpace(newSessionID),
				})
				manifestChanged = true
				current = append(current, newRun)
				roleRuns[role] = current
				if sess, lerr := s.loadSessionWithTimeout(ctx, newSessionID); lerr == nil {
					if err := s.handleSpawn(ctx, supervisorCmd{
						kind:      cmdSpawn,
						runID:     strings.TrimSpace(newRun.RunID),
						sessionID: strings.TrimSpace(newSessionID),
						sess:      &sess,
					}); err != nil {
						slog.Warn("start scaled replica", "component", "supervisor", "run_id", newRun.RunID, "error", err)
					}
				}
			}
			continue
		}
		if len(current) <= target {
			continue
		}
		extras := current[target:]
		for i := len(extras) - 1; i >= 0; i-- {
			runID := strings.TrimSpace(extras[i].RunID)
			if runID == "" {
				continue
			}
			if err := s.stopRun(ctx, runID); err != nil {
				return err
			}
			manifest.Roles = removeManifestRoleRun(manifest.Roles, runID)
			if strings.EqualFold(strings.TrimSpace(manifest.CoordinatorRole), role) && strings.EqualFold(strings.TrimSpace(manifest.CoordinatorRun), runID) {
				manifest.CoordinatorRun = ""
			}
			manifestChanged = true
		}
		kept := current[:target]
		if strings.EqualFold(strings.TrimSpace(manifest.CoordinatorRole), role) && strings.TrimSpace(manifest.CoordinatorRun) == "" && len(kept) > 0 {
			sort.SliceStable(kept, func(i, j int) bool { return stableRunLess(kept[i], kept[j]) })
			manifest.CoordinatorRun = strings.TrimSpace(kept[0].RunID)
		}
	}
	if !manifestChanged {
		return nil
	}
	return store.Save(ctx, *manifest)
}

func removeManifestRoleRun(roles []team.RoleRecord, runID string) []team.RoleRecord {
	runID = strings.TrimSpace(runID)
	if runID == "" || len(roles) == 0 {
		return roles
	}
	out := make([]team.RoleRecord, 0, len(roles))
	for _, role := range roles {
		if strings.EqualFold(strings.TrimSpace(role.RunID), runID) {
			continue
		}
		out = append(out, role)
	}
	return out
}

func (s *runtimeSupervisor) spawnPersistentReplica(ctx context.Context, teamID, roleName string, manifest *team.Manifest) (types.Run, string, error) {
	teamID = strings.TrimSpace(teamID)
	roleName = strings.TrimSpace(roleName)
	if teamID == "" || roleName == "" {
		return types.Run{}, "", fmt.Errorf("teamID and roleName are required")
	}
	var templateRun types.Run
	templateSessionID := ""
	if manifest != nil {
		for _, role := range manifest.Roles {
			if !strings.EqualFold(strings.TrimSpace(role.RoleName), roleName) {
				continue
			}
			runID := strings.TrimSpace(role.RunID)
			if runID == "" {
				continue
			}
			run, err := s.sessionService.LoadRun(ctx, runID)
			if err != nil {
				continue
			}
			templateRun = run
			templateSessionID = strings.TrimSpace(role.SessionID)
			break
		}
	}
	if strings.TrimSpace(templateRun.RunID) == "" {
		return types.Run{}, "", fmt.Errorf("no template run found for team=%s role=%s", teamID, roleName)
	}
	if templateSessionID == "" {
		templateSessionID = strings.TrimSpace(templateRun.SessionID)
	}
	if templateSessionID == "" {
		return types.Run{}, "", fmt.Errorf("template session is missing for role %s", roleName)
	}
	goal := strings.TrimSpace(templateRun.Goal)
	if goal == "" {
		goal = roleName + " worker"
	}
	maxContext := templateRun.MaxBytesForContext
	if maxContext <= 0 {
		maxContext = 8 * 1024
	}
	run := types.NewRun(goal, maxContext, templateSessionID)
	if templateRun.Runtime != nil {
		cfg := *templateRun.Runtime
		run.Runtime = &cfg
	} else {
		run.Runtime = &types.RunRuntimeConfig{}
	}
	run.Runtime.TeamID = teamID
	run.Runtime.Role = roleName
	run.Runtime.WorkerClass = "persistent"
	if err := s.sessionService.SaveRun(ctx, run); err != nil {
		return types.Run{}, "", err
	}
	if _, err := s.sessionService.AddRunToSession(ctx, templateSessionID, run.RunID); err != nil {
		return types.Run{}, "", err
	}
	return run, templateSessionID, nil
}

func (s *runtimeSupervisor) maybeRepairRoutingDrift(ctx context.Context) {
	repairer, ok := s.taskService.(routingDriftRepairer)
	if !ok || repairer == nil {
		return
	}
	now := time.Now().UTC()
	if !s.lastRoutingRepairAt.IsZero() && now.Sub(s.lastRoutingRepairAt) < 10*time.Second {
		return
	}
	s.lastRoutingRepairAt = now
	n, err := repairer.RepairRoutingDrift(ctx, 400)
	if err != nil {
		slog.Warn("routing drift repair failed", "component", "supervisor", "error", err)
		return
	}
	if n > 0 {
		slog.Info("routing drift repaired", "component", "supervisor", "repaired", n)
	}
}

func collectSessionRunIDs(sess types.Session) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(sess.Runs)+1)
	if id := strings.TrimSpace(sess.CurrentRunID); id != "" {
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, id := range sess.Runs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func (s *runtimeSupervisor) ensureLoopState() {
	if s == nil {
		return
	}
	if s.cmdCh == nil {
		s.cmdCh = make(chan supervisorCmd, 256)
	}
	if s.handles == nil {
		s.handles = map[string]*runHandle{}
	}
	if s.snapshots == nil {
		s.snapshots = map[string]runStateSnapshot{}
	}
}

func (s *runtimeSupervisor) updateSnapshot(runID string, h *runHandle, persistedStatus string) {
	if s == nil {
		return
	}
	s.ensureLoopState()
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return
	}
	s.snapshotMu.Lock()
	defer s.snapshotMu.Unlock()

	prev := s.snapshots[runID]
	if strings.TrimSpace(persistedStatus) == "" {
		persistedStatus = prev.PersistedStatus
	}
	snap := runStateSnapshot{
		RunID:           runID,
		PersistedStatus: strings.TrimSpace(persistedStatus),
	}
	if h != nil {
		snap.SessionID = strings.TrimSpace(h.sessionID)
		snap.HandleState = h.state
		snap.rt = h.rt
		if h.rt != nil {
			snap.WorkerPresent = h.rt.done == nil || !workerDone(h.rt.done)
			snap.Model = strings.TrimSpace(h.rt.CurrentModel())
			snap.LastHeartbeatAt = h.rt.LastHeartbeatAt().UTC()
		}
	}
	s.snapshots[runID] = snap
}

func (s *runtimeSupervisor) setSnapshotPersistedStatus(runID, persistedStatus string) {
	if s == nil {
		return
	}
	s.ensureLoopState()
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return
	}
	s.snapshotMu.Lock()
	defer s.snapshotMu.Unlock()
	snap := s.snapshots[runID]
	snap.RunID = runID
	snap.PersistedStatus = strings.TrimSpace(persistedStatus)
	s.snapshots[runID] = snap
}

func (s *runtimeSupervisor) deleteSnapshot(runID string) {
	if s == nil {
		return
	}
	s.ensureLoopState()
	s.snapshotMu.Lock()
	delete(s.snapshots, strings.TrimSpace(runID))
	s.snapshotMu.Unlock()
}

func (s *runtimeSupervisor) getSnapshot(runID string) (runStateSnapshot, bool) {
	if s == nil {
		return runStateSnapshot{}, false
	}
	s.ensureLoopState()
	s.snapshotMu.RLock()
	defer s.snapshotMu.RUnlock()
	snap, ok := s.snapshots[strings.TrimSpace(runID)]
	return snap, ok
}

func (s *runtimeSupervisor) snapshotValues() []runStateSnapshot {
	if s == nil {
		return nil
	}
	s.ensureLoopState()
	s.snapshotMu.RLock()
	defer s.snapshotMu.RUnlock()
	out := make([]runStateSnapshot, 0, len(s.snapshots))
	for _, snap := range s.snapshots {
		out = append(out, snap)
	}
	return out
}

func (s *runtimeSupervisor) sendCmd(cmd supervisorCmd) {
	if s == nil {
		return
	}
	s.ensureLoopState()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case s.cmdCh <- cmd:
	case <-timer.C:
		slog.Error("runtime supervisor command enqueue timed out", "component", "supervisor", "kind", cmd.kind, "run_id", strings.TrimSpace(cmd.runID))
	}
}

func (s *runtimeSupervisor) trySendCmd(cmd supervisorCmd) error {
	if s == nil {
		return nil
	}
	s.ensureLoopState()
	select {
	case s.cmdCh <- cmd:
		return nil
	default:
		return fmt.Errorf("runtime supervisor command queue full")
	}
}

func (s *runtimeSupervisor) processCmd(ctx context.Context, cmd supervisorCmd) {
	s.ensureLoopState()
	switch cmd.kind {
	case cmdSpawn:
		if err := s.handleSpawn(ctx, cmd); err != nil {
			slog.Error("runtime supervisor spawn failed", "component", "supervisor", "run_id", strings.TrimSpace(cmd.runID), "error", err)
		}
	case cmdStop:
		s.handleStop(cmd)
	case cmdWorkerExited:
		if err := s.handleWorkerExited(ctx, cmd); err != nil {
			slog.Error("runtime supervisor exit handling failed", "component", "supervisor", "run_id", strings.TrimSpace(cmd.runID), "error", err)
		}
	case cmdRepairDrift:
		s.maybeRepairRoutingDrift(ctx)
	}
}

func (s *runtimeSupervisor) handleSpawn(ctx context.Context, cmd supervisorCmd) error {
	s.ensureLoopState()
	runID := strings.TrimSpace(cmd.runID)
	if runID == "" {
		return nil
	}
	run, err := s.loadRunWithTimeout(ctx, runID)
	if err != nil {
		return err
	}
	status := strings.TrimSpace(run.Status)
	if status == "" || isTerminalRunStatus(status) {
		delete(s.handles, runID)
		s.deleteSnapshot(runID)
		return nil
	}
	paused := strings.EqualFold(status, types.RunStatusPaused)

	if existing, ok := s.handles[runID]; ok && existing != nil {
		switch existing.state {
		case handleStateSpawning, handleStateRunning, handleStatePaused:
			if existing.rt != nil && existing.rt.session != nil {
				existing.rt.session.SetPaused(paused)
			}
			if existing.state != handleStateSpawning {
				if paused {
					existing.state = handleStatePaused
				} else {
					existing.state = handleStateRunning
				}
			}
			s.updateSnapshot(runID, existing, status)
			return nil
		case handleStateDraining:
			return nil
		}
	}

	sess := cmd.sess
	if sess == nil || strings.TrimSpace(sess.SessionID) == "" {
		loaded, lerr := s.loadSessionWithTimeout(ctx, strings.TrimSpace(run.SessionID))
		if lerr != nil {
			return lerr
		}
		sess = &loaded
	}
	if sess == nil {
		return fmt.Errorf("session is required for spawn")
	}

	if teamID := strings.TrimSpace(sess.TeamID); teamID != "" && run.Runtime != nil && strings.TrimSpace(run.Runtime.Role) == "" {
		if _, roleByRun := loadTeamManifestRunRolesFromStore(ctx, team.NewFileManifestStore(s.cfg), teamID); len(roleByRun) != 0 {
			if role := strings.TrimSpace(roleByRun[strings.TrimSpace(run.RunID)]); role != "" {
				run.Runtime.Role = role
				// Best-effort: save run state
				if err := s.sessionService.SaveRun(ctx, run); err != nil {
					slog.Debug("failed to save run", "runID", run.RunID, "error", err)
				}
			}
		}
	}

	h := &runHandle{
		runID:     runID,
		sessionID: strings.TrimSpace(sess.SessionID),
		state:     handleStateSpawning,
	}
	s.handles[runID] = h
	s.updateSnapshot(runID, h, status)

	startFn := s.spawnOverride
	if startFn == nil {
		startFn = s.spawnManagedRun
	}
	managed, err := startFn(ctx, *sess, runID)
	if err != nil {
		delete(s.handles, runID)
		s.deleteSnapshot(runID)
		return err
	}
	if managed == nil {
		delete(s.handles, runID)
		s.deleteSnapshot(runID)
		return fmt.Errorf("managed runtime is nil")
	}

	h.rt = managed
	if h.stopPending {
		if h.rt.session != nil {
			h.rt.session.SetPaused(h.stopPaused)
		}
		if h.rt.cancel != nil {
			h.rt.cancel()
		}
		h.state = handleStateDraining
		s.updateSnapshot(runID, h, status)
		s.startExitWatcher(managed, runID, h)
		return nil
	}
	if paused {
		h.state = handleStatePaused
	} else {
		h.state = handleStateRunning
	}
	s.updateSnapshot(runID, h, status)
	s.startExitWatcher(managed, runID, h)
	return nil
}

func (s *runtimeSupervisor) handleStop(cmd supervisorCmd) {
	s.ensureLoopState()
	runID := strings.TrimSpace(cmd.runID)
	if runID == "" {
		return
	}
	h, ok := s.handles[runID]
	if !ok || h == nil || h.state == handleStateDraining {
		return
	}
	if h.state == handleStateSpawning || h.rt == nil {
		h.stopPending = true
		h.stopPaused = cmd.paused
		s.updateSnapshot(runID, h, "")
		return
	}
	if h.rt.session != nil {
		h.rt.session.SetPaused(cmd.paused)
	}
	if h.rt.cancel != nil {
		h.rt.cancel()
	}
	h.state = handleStateDraining
	h.stopPending = false
	h.stopPaused = cmd.paused
	s.updateSnapshot(runID, h, "")
}

func (s *runtimeSupervisor) handleWorkerExited(ctx context.Context, cmd supervisorCmd) error {
	s.ensureLoopState()
	runID := strings.TrimSpace(cmd.runID)
	if runID == "" {
		return nil
	}
	h, ok := s.handles[runID]
	if !ok || h != cmd.handle {
		return nil
	}
	delete(s.handles, runID)
	s.deleteSnapshot(runID)

	run, err := s.loadRunWithTimeout(ctx, runID)
	if err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(run.Status), types.RunStatusRunning) {
		return nil
	}
	sess, err := s.loadSessionWithTimeout(ctx, strings.TrimSpace(run.SessionID))
	if err != nil {
		return err
	}
	return s.handleSpawn(ctx, supervisorCmd{
		kind:      cmdSpawn,
		runID:     runID,
		sessionID: strings.TrimSpace(run.SessionID),
		sess:      &sess,
	})
}

func (s *runtimeSupervisor) startExitWatcher(managed *managedRuntime, runID string, h *runHandle) {
	if s == nil || managed == nil || managed.done == nil {
		return
	}
	go func() {
		<-managed.done
		s.sendCmd(supervisorCmd{kind: cmdWorkerExited, runID: strings.TrimSpace(runID), handle: h})
	}()
}

func (s *runtimeSupervisor) loadAndSpawnActiveRuns(ctx context.Context) error {
	if s == nil || s.sessionService == nil {
		return nil
	}
	s.ensureLoopState()
	runs, err := s.sessionService.ListRunsByStatus(ctx, []string{types.RunStatusRunning, types.RunStatusPaused})
	if err != nil {
		return err
	}
	for _, run := range runs {
		sess, lerr := s.loadSessionWithTimeout(ctx, strings.TrimSpace(run.SessionID))
		if lerr != nil {
			slog.Warn("load session for run", "component", "supervisor", "run_id", run.RunID, "error", lerr)
			continue
		}
		if err := s.handleSpawn(ctx, supervisorCmd{
			kind:      cmdSpawn,
			runID:     strings.TrimSpace(run.RunID),
			sessionID: strings.TrimSpace(run.SessionID),
			sess:      &sess,
		}); err != nil {
			slog.Error("managed run start failed", "component", "supervisor", "run_id", run.RunID, "error", err)
		}
	}
	return nil
}

func (s *runtimeSupervisor) handleTaskWake(ctx context.Context) {
	if s == nil || s.sessionService == nil {
		return
	}
	s.ensureLoopState()
	runs, err := s.sessionService.ListRunsByStatus(ctx, []string{types.RunStatusRunning})
	if err != nil {
		slog.Warn("runtime supervisor wake scan failed", "component", "supervisor", "error", err)
		return
	}
	const maxWakeScanSize = 50
	if len(runs) > maxWakeScanSize {
		runs = runs[:maxWakeScanSize]
	}
	for _, run := range runs {
		runID := strings.TrimSpace(run.RunID)
		if runID == "" {
			continue
		}
		if h := s.handles[runID]; h != nil {
			switch h.state {
			case handleStateSpawning, handleStateRunning, handleStatePaused, handleStateDraining:
				continue
			}
		}
		sess, lerr := s.loadSessionWithTimeout(ctx, strings.TrimSpace(run.SessionID))
		if lerr != nil {
			slog.Warn("load session for wake spawn", "component", "supervisor", "run_id", runID, "error", lerr)
			continue
		}
		if err := s.handleSpawn(ctx, supervisorCmd{
			kind:      cmdSpawn,
			runID:     runID,
			sessionID: strings.TrimSpace(run.SessionID),
			sess:      &sess,
		}); err != nil {
			slog.Error("wake spawn failed", "component", "supervisor", "run_id", runID, "error", err)
		}
	}
}

func (s *runtimeSupervisor) stopAllHandles() {
	s.ensureLoopState()
	for runID, h := range s.handles {
		if h == nil || h.rt == nil {
			delete(s.handles, runID)
			s.deleteSnapshot(runID)
			continue
		}
		if h.rt.cancel != nil {
			h.rt.cancel()
		}
		if h.rt.done != nil {
			<-h.rt.done
		}
		delete(s.handles, runID)
		s.deleteSnapshot(runID)
	}
}

func (s *runtimeSupervisor) loadRunWithTimeout(ctx context.Context, runID string) (types.Run, error) {
	ctx = nonNilContext(ctx)
	ioCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return s.sessionService.LoadRun(ioCtx, strings.TrimSpace(runID))
}

func (s *runtimeSupervisor) loadSessionWithTimeout(ctx context.Context, sessionID string) (types.Session, error) {
	ctx = nonNilContext(ctx)
	ioCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return s.sessionService.LoadSession(ioCtx, strings.TrimSpace(sessionID))
}

func isTerminalRunStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case types.RunStatusCanceled, types.RunStatusSucceeded, types.RunStatusFailed:
		return true
	default:
		return false
	}
}

type spawnRunState struct {
	run    types.Run
	paused bool
}

type spawnProfileAndRole struct {
	prof                     *profile.Profile
	profDir                  string
	teamID                   string
	roleName                 string
	activeProfile            *profile.Profile
	coordinatorRole          string
	teamRoles                []string
	teamRoleDescriptions     map[string]string
	isCoordinator            bool
	isReviewer               bool
	allowSubagents           bool
	roleCodeExecOnlyOverride *bool
	reviewerRole             string
	reviewerCfg              *profile.ReviewerConfig
}

type spawnModelAndReasoning struct {
	model            string
	modelSource      string
	reasoningEffort  string
	reasoningSummary string
}

type spawnRuntimeParts struct {
	rt             *runtime.Runtime
	orderedEmitter *emit.OrderedEmitter[events.Event]
	emitEvent      func(context.Context, events.Event)
}

type spawnToolWiring struct {
	modelRegistry  *agent.HostToolRegistry
	bridgeRegistry *agent.HostToolRegistry
	codeExecOnly   bool
}

type spawnWorkerLoopConfig struct {
	run            types.Run
	activeProfile  *profile.Profile
	modelSource    string
	workerSession  *agentsession.Session
	rt             *runtime.Runtime
	orderedEmitter *emit.OrderedEmitter[events.Event]
	emitEvent      func(context.Context, events.Event)
	managed        *managedRuntime
	wakeCancel     func()
}

func (s *runtimeSupervisor) loadSpawnRunState(parent context.Context, sess types.Session, runID string) (spawnRunState, error) {
	run, err := s.sessionService.LoadRun(parent, runID)
	if err != nil {
		return spawnRunState{}, err
	}
	paused := strings.EqualFold(strings.TrimSpace(run.Status), types.RunStatusPaused)
	if run.Runtime == nil {
		run.Runtime = &types.RunRuntimeConfig{}
	}
	if strings.TrimSpace(run.SessionID) == "" {
		run.SessionID = strings.TrimSpace(sess.SessionID)
	}
	return spawnRunState{run: run, paused: paused}, nil
}

func (s *runtimeSupervisor) resolveSpawnProfileAndRole(parent context.Context, sess types.Session, run types.Run) (spawnProfileAndRole, error) {
	profRef := strings.TrimSpace(run.Runtime.Profile)
	if profRef == "" {
		profRef = strings.TrimSpace(sess.Profile)
	}
	if profRef == "" && s.defaultProfile != nil {
		profRef = strings.TrimSpace(s.defaultProfile.ID)
	}
	if profRef == "" {
		profRef = "general"
	}
	prof, profDir, err := resolveProfileRef(s.cfg, profRef)
	if err != nil {
		return spawnProfileAndRole{}, fmt.Errorf("resolve profile %q: %w", profRef, err)
	}
	if prof == nil {
		return spawnProfileAndRole{}, fmt.Errorf("profile %q not found", profRef)
	}

	out := spawnProfileAndRole{
		prof:                 prof,
		profDir:              profDir,
		teamID:               strings.TrimSpace(run.Runtime.TeamID),
		roleName:             strings.TrimSpace(run.Runtime.Role),
		activeProfile:        prof,
		teamRoleDescriptions: map[string]string{},
		allowSubagents:       true, // standalone: allow by default (current behavior)
	}
	if out.teamID == "" {
		out.teamID = strings.TrimSpace(sess.TeamID)
	}
	if strings.TrimSpace(out.teamID) == "" {
		return out, nil
	}

	sessionRoles, err := prof.RolesForSession()
	if err != nil {
		return spawnProfileAndRole{}, fmt.Errorf("roles for session: %w", err)
	}
	roles, coord, err := team.ValidateTeamRoles(sessionRoles)
	if err != nil {
		return spawnProfileAndRole{}, err
	}
	out.teamRoles = roles
	out.coordinatorRole = coord
	if cfg, ok := prof.ReviewerForSession(); ok && cfg != nil {
		out.reviewerCfg = cfg
		out.reviewerRole = strings.TrimSpace(cfg.EffectiveName())
		if desc := strings.TrimSpace(cfg.Description); desc != "" {
			out.teamRoleDescriptions[out.reviewerRole] = desc
		}
	} else {
		out.reviewerRole = out.coordinatorRole
	}
	if out.roleName == "" {
		_, roleByRun := loadTeamManifestRunRolesFromStore(parent, team.NewFileManifestStore(s.cfg), out.teamID)
		out.roleName = strings.TrimSpace(roleByRun[strings.TrimSpace(run.RunID)])
	}
	if out.roleName == "" {
		return spawnProfileAndRole{}, fmt.Errorf("team run %s has no role mapping", strings.TrimSpace(run.RunID))
	}

	isChildRun := strings.TrimSpace(run.ParentRunID) != ""
	isSubagent := isChildRun && isSubagentRoleClass(run)
	if isSubagent {
		out.isCoordinator = false
		out.allowSubagents = false
		out.roleCodeExecOnlyOverride = nil
		out.activeProfile = prof
		out.teamRoleDescriptions[out.roleName] = "Spawned worker"
		return out, nil
	}

	if out.reviewerCfg != nil && strings.EqualFold(strings.TrimSpace(out.roleName), strings.TrimSpace(out.reviewerRole)) {
		out.isCoordinator = false
		out.isReviewer = true
		out.allowSubagents = false
		out.roleCodeExecOnlyOverride = out.reviewerCfg.CodeExecOnly
		out.activeProfile = buildReviewerRuntimeProfile(*out.reviewerCfg)
		return out, nil
	}

	var roleCfg *profile.RoleConfig
	for i := range sessionRoles {
		r := sessionRoles[i]
		name := strings.TrimSpace(r.Name)
		if name != "" {
			out.teamRoleDescriptions[name] = strings.TrimSpace(r.Description)
		}
		if strings.EqualFold(name, out.roleName) {
			copy := r
			roleCfg = &copy
		}
	}
	if roleCfg == nil {
		return spawnProfileAndRole{}, fmt.Errorf("role %q not found in profile %q", out.roleName, prof.ID)
	}
	out.isCoordinator = strings.EqualFold(strings.TrimSpace(roleCfg.Name), out.coordinatorRole)
	out.allowSubagents = roleCfg.AllowSubagents
	out.roleCodeExecOnlyOverride = roleCfg.CodeExecOnly
	out.activeProfile = buildRoleRuntimeProfile(*roleCfg)
	return out, nil
}

func (s *runtimeSupervisor) resolveSpawnModelAndReasoning(sess types.Session, run types.Run) (spawnModelAndReasoning, error) {
	model, modelSource := resolveRunModel(sess, run, strings.TrimSpace(s.resolved.Model))
	if model == "" {
		return spawnModelAndReasoning{}, fmt.Errorf("run %s has no configured model", strings.TrimSpace(run.RunID))
	}
	resolvedEffort, resolvedSummary := sessionReasoningForModel(
		sess,
		model,
		strings.TrimSpace(s.resolved.ReasoningEffort),
		strings.TrimSpace(s.resolved.ReasoningSummary),
	)
	return spawnModelAndReasoning{
		model:            model,
		modelSource:      modelSource,
		reasoningEffort:  resolvedEffort,
		reasoningSummary: resolvedSummary,
	}, nil
}

func (s *runtimeSupervisor) buildSpawnRuntime(
	parent context.Context,
	run types.Run,
	prof *profile.Profile,
	role spawnProfileAndRole,
	model spawnModelAndReasoning,
	soulVersion int,
) (spawnRuntimeParts, error) {
	traceStore := implstore.SQLiteTraceStore{Cfg: s.cfg, RunID: run.RunID}
	historyStore, err := implstore.NewSQLiteHistoryStore(s.cfg, run.SessionID)
	if err != nil {
		return spawnRuntimeParts{}, err
	}

	store := s.eventsStore
	if store == nil {
		store = eventsvc.NewService(s.cfg)
	}
	emitter := &events.Emitter{
		RunID: run.RunID,
		Sink: events.StoreSink{
			Store: store,
		},
	}
	orderedEmitter := emit.NewOrderedEmitter[events.Event](emitter)
	emitEvent := func(ctx context.Context, ev events.Event) {
		if ev.Data == nil {
			ev.Data = map[string]string{}
		}
		if role.teamID != "" {
			ev.Data["teamId"] = role.teamID
			// StoreData, when non-nil, replaces Data in StoreSink before persistence
			// and broadcast. Set role/teamId there too so the event.append notification
			// received by the TUI includes the correct role immediately (not just after
			// the next activity.list poll where the server re-attaches the role).
			if ev.StoreData != nil {
				ev.StoreData["teamId"] = role.teamID
			}
		}
		if role.roleName != "" {
			ev.Data["role"] = role.roleName
			if ev.StoreData != nil {
				ev.StoreData["role"] = role.roleName
			}
		}
		if err := orderedEmitter.Emit(ctx, ev); err != nil && !errorsIsDropped(err) {
			slog.Warn("emit failed", "component", "supervisor", "run_id", strings.TrimSpace(run.RunID), "error", err)
		}
	}

	sharedWorkspaceDir := ""
	if role.teamID != "" {
		sharedWorkspaceDir = fsutil.GetTeamWorkspaceDir(s.cfg.DataDir, role.teamID)
		if err := os.MkdirAll(sharedWorkspaceDir, 0o755); err != nil {
			orderedEmitter.Close()
			return spawnRuntimeParts{}, fmt.Errorf("prepare team workspace mount: %w", err)
		}
	}

	rt, err := runtime.Build(runtime.BuildConfig{
		Cfg:                   s.cfg,
		Run:                   run,
		Profile:               strings.TrimSpace(prof.ID),
		ProfileConfig:         prof,
		WorkdirAbs:            s.workdirAbs,
		SharedWorkspaceDir:    sharedWorkspaceDir,
		Model:                 model.model,
		ReasoningEffort:       model.reasoningEffort,
		ReasoningSummary:      model.reasoningSummary,
		ApprovalsMode:         strings.TrimSpace(s.resolved.ApprovalsMode),
		HistoryStore:          historyStore,
		MemoryStore:           s.memoryStore,
		TraceStore:            traceStore,
		ConstructorStore:      s.constructorStore,
		Emit:                  emitEvent,
		IncludeHistoryOps:     derefBool(s.resolved.IncludeHistoryOps, true),
		RecentHistoryPairs:    s.resolved.RecentHistoryPairs,
		MaxMemoryBytes:        s.resolved.MaxMemoryBytes,
		MaxTraceBytes:         s.resolved.MaxTraceBytes,
		PriceInPerMTokensUSD:  s.resolved.PriceInPerMTokensUSD,
		PriceOutPerMTokensUSD: s.resolved.PriceOutPerMTokensUSD,
		SoulVersionSeen:       soulVersion,
		PersistRun: func(r types.Run) error {
			return s.sessionService.SaveRun(parent, r)
		},
		LoadSession: func(sessionID string) (types.Session, error) {
			return s.sessionService.LoadSession(parent, sessionID)
		},
		SaveSession: func(session types.Session) error {
			return s.sessionService.SaveSession(parent, session)
		},
	})
	if err != nil {
		orderedEmitter.Close()
		return spawnRuntimeParts{}, err
	}
	return spawnRuntimeParts{
		rt:             rt,
		orderedEmitter: orderedEmitter,
		emitEvent:      emitEvent,
	}, nil
}

func (s *runtimeSupervisor) configureSpawnAgent(
	parent context.Context,
	run types.Run,
	rt *runtime.Runtime,
	model spawnModelAndReasoning,
	emitEvent func(context.Context, events.Event),
) (agent.AgentConfig, *managedRuntime, bool) {
	agentCfg := agent.DefaultConfig()
	agentCfg.Model = model.model
	agentCfg.ReasoningEffort = model.reasoningEffort
	agentCfg.ReasoningSummary = model.reasoningSummary
	agentCfg.ApprovalsMode = strings.TrimSpace(s.resolved.ApprovalsMode)
	agentCfg.EnableWebSearch = s.resolved.WebSearchEnabled
	isChildRun := strings.TrimSpace(run.ParentRunID) != ""
	promptSource := agent.PromptSource(rt.Constructor)
	if rt.Updater != nil {
		promptSource = rt.Updater
	}
	agentCfg.PromptSource = promptSource

	managed := &managedRuntime{
		runID:     strings.TrimSpace(run.RunID),
		sessionID: strings.TrimSpace(run.SessionID),
		model:     strings.TrimSpace(model.model),
	}
	// Thinking-event state tracked across stream chunks.
	// Text is line-buffered: we accumulate tokens and emit one summary event
	// per newline-delimited line, drastically reducing event count.
	var thinkingMu sync.Mutex
	thinkingActive := false
	thinkingStep := 0
	var thinkingBuf strings.Builder

	// flushThinkingLines emits complete lines from thinkingBuf.
	// Lines are split on newlines and also on sentence boundaries (". ") when
	// the buffer exceeds a soft limit, so that models which emit reasoning as
	// a single continuous block still produce separate, readable summary chunks.
	// We prefer splitting at ". **" so that markdown bold headings start a
	// new chunk (title-at-start), keeping TUI chunk boundaries aligned with
	// section titles in a model-agnostic way.
	// If final is true, also emits any remaining partial text.
	// Must be called with thinkingMu held.
	const sentenceLimit = 120 // soft char limit before splitting on sentence boundary
	flushThinkingLines := func(final bool) {
		text := thinkingBuf.String()
		stepStr := strconv.Itoa(thinkingStep)

		emitLine := func(line string) {
			line = strings.TrimRight(line, " \t\r")
			if line != "" {
				emitEvent(parent, events.Event{
					Type:    "model.thinking.summary",
					Message: "Thinking",
					Data:    map[string]string{"step": stepStr, "text": line},
				})
			}
		}

		for {
			// Prefer splitting on newlines first.
			idx := strings.IndexByte(text, '\n')
			if idx >= 0 {
				emitLine(text[:idx])
				text = text[idx+1:]
				continue
			}
			// If the buffer is long enough, try splitting on a sentence boundary.
			if len(text) >= sentenceLimit {
				search := text[sentenceLimit:]
				// Prefer ". **" so bold headings start a new chunk (title-at-start).
				if si := strings.Index(search, ". **"); si >= 0 {
					cut := sentenceLimit + si + 2 // include ". " so next chunk starts with "**"
					emitLine(text[:cut])
					text = strings.TrimLeft(text[cut:], " ")
					continue
				}
				// Fallback: any ". " after the soft limit.
				if si := strings.Index(search, ". "); si >= 0 {
					cut := sentenceLimit + si + 2
					emitLine(text[:cut])
					text = strings.TrimLeft(text[cut:], " ")
					continue
				}
			}
			break
		}
		if final && strings.TrimSpace(text) != "" {
			emitLine(text)
			text = ""
		}
		thinkingBuf.Reset()
		thinkingBuf.WriteString(text)
	}

	agentCfg.Hooks = agent.Hooks{
		OnLLMUsage: newCostUsageHook(
			s.cfg,
			run,
			model.model,
			s.resolved.PriceInPerMTokensUSD,
			s.resolved.PriceOutPerMTokensUSD,
			s.sessionService,
			func() string {
				return managed.CurrentModel()
			},
			emitEvent,
		),
		OnStreamChunk: func(step int, chunk llmtypes.LLMStreamChunk) {
			thinkingMu.Lock()
			defer thinkingMu.Unlock()

			stepStr := strconv.Itoa(step)
			if chunk.IsReasoning {
				if !thinkingActive {
					thinkingActive = true
					thinkingStep = step
					thinkingBuf.Reset()
					emitEvent(parent, events.Event{
						Type:    "model.thinking.start",
						Message: "Thinking started",
						Data:    map[string]string{"step": stepStr},
					})
				}
				if chunk.Text != "" {
					thinkingBuf.WriteString(chunk.Text)
					flushThinkingLines(false)
				}
			} else if thinkingActive {
				flushThinkingLines(true)
				thinkingActive = false
				emitEvent(parent, events.Event{
					Type:    "model.thinking.end",
					Message: "Thinking ended",
					Data:    map[string]string{"step": strconv.Itoa(thinkingStep)},
				})
			}
		},
		OnStep: func(step int, model, effectiveModel, summary string) {
			// Close any open thinking block when the step ends.
			thinkingMu.Lock()
			if thinkingActive {
				flushThinkingLines(true)
				thinkingActive = false
				emitEvent(parent, events.Event{
					Type:    "model.thinking.end",
					Message: "Thinking ended",
					Data:    map[string]string{"step": strconv.Itoa(thinkingStep)},
				})
			}
			thinkingMu.Unlock()

			data := map[string]string{
				"step":  strconv.Itoa(step),
				"model": strings.TrimSpace(model),
			}
			if em := strings.TrimSpace(effectiveModel); em != "" {
				data["effectiveModel"] = em
			}
			if s := strings.TrimSpace(summary); s != "" {
				data["reasoningSummary"] = s
			}
			emitEvent(parent, events.Event{Type: "agent.step", Message: fmt.Sprintf("Step %d completed", step), Data: data})
		},
		OnCompaction: func(step int, beforeTokens, afterTokens int, serverSide bool) {
			emitEvent(parent, events.Event{
				Type:    "context.compacted",
				Message: fmt.Sprintf("Context compacted (%dk → %dk tokens)", beforeTokens/1000, afterTokens/1000),
				Data: map[string]string{
					"step":         strconv.Itoa(step),
					"beforeTokens": strconv.Itoa(beforeTokens),
					"afterTokens":  strconv.Itoa(afterTokens),
					"serverSide":   strconv.FormatBool(serverSide),
				},
			})
		},
		OnContextSize: func(step int, currentTokens, budgetTokens int) {
			emitEvent(parent, events.Event{
				Type:    "context.size",
				Message: fmt.Sprintf("Context: %dk/%dk tokens", currentTokens/1000, budgetTokens/1000),
				Data: map[string]string{
					"step":          strconv.Itoa(step),
					"currentTokens": strconv.Itoa(currentTokens),
					"budgetTokens":  strconv.Itoa(budgetTokens),
				},
			})
		},
	}

	return agentCfg, managed, isChildRun
}

func (s *runtimeSupervisor) wireSpawnTools(
	parent context.Context,
	run types.Run,
	role spawnProfileAndRole,
	prof *profile.Profile,
	activeProfile *profile.Profile,
	model string,
	isChildRun bool,
	rt *runtime.Runtime,
	orderedEmitter *emit.OrderedEmitter[events.Event],
	emitEvent func(context.Context, events.Event),
) (spawnToolWiring, error) {
	registry, err := agent.DefaultHostToolRegistry()
	if err != nil {
		orderedEmitter.Close()
		// Best-effort: shutdown runtime
		if err := rt.Shutdown(parent); err != nil {
			slog.Debug("runtime shutdown error", "error", err)
		}
		return spawnToolWiring{}, err
	}

	teamID := strings.TrimSpace(role.teamID)
	roleName := strings.TrimSpace(role.roleName)
	isTeam := teamID != ""
	tool := &hosttools.TaskCreateTool{
		Store:      s.taskService,
		SessionID:  run.SessionID,
		RunID:      run.RunID,
		IsChildRun: isChildRun,
	}
	if teamID != "" {
		tool.TeamID = teamID
		tool.RoleName = roleName
		tool.IsCoordinator = role.isCoordinator
		tool.CoordinatorRole = role.coordinatorRole
		tool.ReviewerRole = role.reviewerRole
		tool.ReviewerOnly = role.reviewerCfg != nil
		tool.ValidRoles = role.teamRoles
	}
	allowSpawnWorker := !isChildRun && role.allowSubagents
	if allowSpawnWorker && isTeam && role.isCoordinator && len(role.teamRoles) > 1 {
		// Team-only delegation constitution (without policy engine): coordinators delegate to roles.
		allowSpawnWorker = false
		emitEvent(parent, events.Event{
			Type:    "delegation.policy.enforced",
			Message: "Coordinator subagent spawning disabled in multi-role team",
			Data: map[string]string{
				"teamId":          teamID,
				"role":            roleName,
				"coordinatorRole": role.coordinatorRole,
			},
		})
	}
	if allowSpawnWorker {
		tool.SpawnWorker = s.makeSpawnWorkerFunc(run, model, emitEvent)
	}
	if err := registry.Register(tool); err != nil {
		orderedEmitter.Close()
		// Best-effort: shutdown runtime
		if err := rt.Shutdown(parent); err != nil {
			slog.Debug("runtime shutdown error", "error", err)
		}
		return spawnToolWiring{}, err
	}
	// Register task_review tool for agents that can receive callbacks (non-child runs).
	if !isChildRun {
		reviewTool := &hosttools.TaskReviewTool{
			Store:      s.taskService,
			SessionID:  run.SessionID,
			RunID:      run.RunID,
			Supervisor: s,
		}
		if err := registry.Register(reviewTool); err != nil {
			orderedEmitter.Close()
			// Best-effort: shutdown runtime
			if err := rt.Shutdown(parent); err != nil {
				slog.Debug("runtime shutdown error", "error", err)
			}
			return spawnToolWiring{}, err
		}
	}
	if s.soulService != nil {
		if err := registry.Register(&hosttools.SoulUpdateTool{Updater: s.soulService, Actor: pkgsoul.ActorAgent}); err != nil {
			orderedEmitter.Close()
			// Best-effort: shutdown runtime
			if err := rt.Shutdown(parent); err != nil {
				slog.Debug("runtime shutdown error", "error", err)
			}
			return spawnToolWiring{}, err
		}
	}
	if err := registry.Register(&hosttools.ObsidianTool{ProjectRoot: s.workdirAbs}); err != nil {
		orderedEmitter.Close()
		// Best-effort: shutdown runtime
		if err := rt.Shutdown(parent); err != nil {
			slog.Debug("runtime shutdown error", "error", err)
		}
		return spawnToolWiring{}, err
	}

	var allowedToolsForRun []string
	if !isChildRun {
		roleAllowedTools, removedTools := sanitizeAllowedToolsForRole(activeProfile.AllowedTools, teamID, role.isCoordinator, role.isReviewer)
		if len(removedTools) > 0 {
			emitEvent(parent, events.Event{
				Type:    "daemon.warning",
				Message: "Removed disallowed tool(s) for non-coordinator role",
				Data: map[string]string{
					"teamId": teamID,
					"role":   roleName,
					"tools":  strings.Join(removedTools, ","),
				},
			})
		}
		allowedToolsForRun = roleAllowedTools
	}
	codeExecDefault := activeProfile.CodeExecOnly
	if isTeam {
		codeExecDefault = prof.CodeExecOnly
	}
	codeExecOnly := resolveCodeExecOnly(codeExecDefault, role.roleCodeExecOnlyOverride)
	activeProfile.CodeExecOnly = codeExecOnly
	resolvedCodeExecRequiredImports := resolveCodeExecRequiredImports(s.cfg.CodeExec.RequiredPackages)

	modelRegistry, bridgeRegistry, err := resolveToolRegistries(registry, allowedToolsForRun, codeExecOnly)
	if err != nil {
		orderedEmitter.Close()
		// Best-effort: shutdown runtime
		if err := rt.Shutdown(parent); err != nil {
			slog.Debug("runtime shutdown error", "error", err)
		}
		return spawnToolWiring{}, err
	}
	if err := configureCodeExecRuntime(parent, rt, s.cfg, modelRegistry, bridgeRegistry, resolvedCodeExecRequiredImports, codeExecOnly, emitEvent); err != nil {
		orderedEmitter.Close()
		// Best-effort: shutdown runtime
		if err := rt.Shutdown(parent); err != nil {
			slog.Debug("runtime shutdown error", "error", err)
		}
		return spawnToolWiring{}, err
	}
	return spawnToolWiring{
		modelRegistry:  modelRegistry,
		bridgeRegistry: bridgeRegistry,
		codeExecOnly:   codeExecOnly,
	}, nil
}

func (s *runtimeSupervisor) spawnManagedRun(parent context.Context, sess types.Session, runID string) (*managedRuntime, error) {
	state, err := s.loadSpawnRunState(parent, sess, runID)
	if err != nil {
		return nil, err
	}
	run := state.run
	paused := state.paused

	role, err := s.resolveSpawnProfileAndRole(parent, sess, run)
	if err != nil {
		return nil, err
	}

	soulContent := ""
	soulVersion := 0
	if s.soulService != nil {
		if doc, err := s.soulService.Get(parent); err == nil {
			soulContent = strings.TrimSpace(doc.Content)
			soulVersion = doc.Version
		}
	}
	if soulVersion > 0 && sess.SoulVersionSeen != soulVersion {
		sess.SoulVersionSeen = soulVersion
		// Best-effort: save session state
		if err := s.sessionService.SaveSession(parent, sess); err != nil {
			slog.Debug("failed to save session", "error", err)
		}
	}

	if run.Runtime == nil {
		run.Runtime = &types.RunRuntimeConfig{}
	}
	if strings.TrimSpace(run.ParentRunID) != "" {
		run.Runtime.WorkerClass = "ephemeral"
	} else if strings.TrimSpace(run.Runtime.WorkerClass) == "" {
		run.Runtime.WorkerClass = "persistent"
	}
	run.Runtime.Profile = strings.TrimSpace(role.prof.ID)
	run.Runtime.TeamID = role.teamID
	run.Runtime.Role = role.roleName
	run.Runtime.SoulVersionSeen = soulVersion

	modelInfo, err := s.resolveSpawnModelAndReasoning(sess, run)
	if err != nil {
		return nil, err
	}
	run.Runtime.Model = modelInfo.model
	// Best-effort: save run state
	if err := s.sessionService.SaveRun(parent, run); err != nil {
		slog.Debug("failed to save run", "runID", run.RunID, "error", err)
	}

	rtParts, err := s.buildSpawnRuntime(parent, run, role.prof, role, modelInfo, soulVersion)
	if err != nil {
		return nil, err
	}
	rt := rtParts.rt
	orderedEmitter := rtParts.orderedEmitter
	emitEvent := rtParts.emitEvent

	prof := role.prof
	profDir := role.profDir
	teamID := role.teamID
	roleName := role.roleName
	activeProfile := role.activeProfile
	coordinatorRole := role.coordinatorRole
	teamRoles := role.teamRoles
	teamRoleDescriptions := role.teamRoleDescriptions
	isCoordinator := role.isCoordinator
	reviewerRole := role.reviewerRole

	model := modelInfo.model
	modelSource := modelInfo.modelSource
	agentCfg, managed, isChildRun := s.configureSpawnAgent(parent, run, rt, modelInfo, emitEvent)

	toolWiring, err := s.wireSpawnTools(parent, run, role, prof, activeProfile, model, isChildRun, rt, orderedEmitter, emitEvent)
	if err != nil {
		return nil, err
	}
	modelRegistry := toolWiring.modelRegistry
	bridgeRegistry := toolWiring.bridgeRegistry
	codeExecOnly := toolWiring.codeExecOnly

	promptToolSpec := agent.PromptToolSpecFromSources(modelRegistry, nil)
	if codeExecOnly {
		promptToolSpec = agent.PromptToolSpecForCodeExecOnly(modelRegistry, bridgeRegistry, nil)
	}
	if isChildRun {
		agentCfg.SystemPrompt = prompts.DefaultSubAgentSystemPromptWithTools(promptToolSpec)
	} else {
		agentCfg.SystemPrompt = prompts.DefaultAutonomousSystemPromptWithTools(promptToolSpec)
	}
	agentCfg.HostToolRegistry = modelRegistry

	runLLMClient := withRetryDiagnostics(s.llmClient, emitEvent)
	a, err := agent.NewAgent(runLLMClient, rt.Executor, agentCfg)
	if err != nil {
		orderedEmitter.Close()
		// Best-effort: shutdown runtime
		if err := rt.Shutdown(parent); err != nil {
			slog.Debug("runtime shutdown error", "error", err)
		}
		return nil, err
	}
	if reviewerRole == "" && coordinatorRole != "" {
		reviewerRole = strings.TrimSpace(coordinatorRole)
	}
	runConvStore, err := implstore.NewSQLiteRunConversationStoreFromConfig(s.cfg)
	if err != nil {
		orderedEmitter.Close()
		// Best-effort: shutdown runtime
		if err := rt.Shutdown(parent); err != nil {
			slog.Debug("runtime shutdown error", "error", err)
		}
		return nil, fmt.Errorf("run conversation store: %w", err)
	}

	var wakeCh <-chan struct{}
	var wakeCancel func()
	if wakeSub, ok := s.taskService.(taskWakeSubscriber); ok && wakeSub != nil {
		wakeTeamID := strings.TrimSpace(teamID)
		wakeRunID := strings.TrimSpace(run.RunID)
		// In team mode, messages are claimed by assignee (role/agent/team), so workers
		// must all receive team-scoped wake signals regardless of task.RunID.
		if wakeTeamID != "" {
			wakeRunID = ""
		}
		wakeCh, wakeCancel = wakeSub.SubscribeWake(wakeTeamID, wakeRunID)
	}
	messageBus, ok := s.taskService.(agentsession.MessageBus)
	if !ok || messageBus == nil {
		orderedEmitter.Close()
		// Best-effort: shutdown runtime
		if err := rt.Shutdown(parent); err != nil {
			slog.Debug("runtime shutdown error", "error", err)
		}
		return nil, fmt.Errorf("task service does not implement message bus")
	}

	workerSession, err := agentsession.New(agentsession.Config{
		Agent:      a,
		Profile:    activeProfile,
		ProfileDir: profDir,
		ResolveProfile: func(ref string) (*profile.Profile, string, error) {
			return resolveProfileRef(s.cfg, strings.TrimSpace(ref))
		},
		TaskStore:            s.taskService,
		MessageBus:           messageBus,
		Events:               orderedEmitter,
		RunConversationStore: runConvStore,
		Memory: &validatingMemoryProvider{
			inner: &textMemoryAdapter{store: s.memoryStore},
			store: s.memoryStore,
		},
		MemorySearchLimit:    3,
		Notifier:             &subagentCleanupNotifier{supervisor: s, store: s.taskService, next: s.notifier},
		SoulContent:          soulContent,
		SoulVersion:          soulVersion,
		PollInterval:         s.pollInterval,
		WakeCh:               wakeCh,
		RequireWakeCh:        true,
		MaxReadBytes:         256 * 1024,
		LeaseTTL:             2 * time.Minute,
		MaxRetries:           3,
		MaxPending:           50,
		SessionID:            run.SessionID,
		RunID:                run.RunID,
		TeamID:               teamID,
		RoleName:             roleName,
		IsCoordinator:        isCoordinator,
		CoordinatorRole:      coordinatorRole,
		ReviewerRole:         reviewerRole,
		TeamRoles:            teamRoles,
		TeamRoleDescriptions: teamRoleDescriptions,
		ParentRunID:          strings.TrimSpace(run.ParentRunID),
		SpawnIndex:           run.SpawnIndex,
		SingleTask:           isChildRun,
		InstanceID:           run.RunID,
		Logf: func(format string, args ...any) {
			slog.Info(fmt.Sprintf(format, args...), "component", "supervisor", "run_id", run.RunID)
		},
	})
	if err != nil {
		orderedEmitter.Close()
		// Best-effort: shutdown runtime
		if err := rt.Shutdown(parent); err != nil {
			slog.Debug("runtime shutdown error", "error", err)
		}
		return nil, err
	}
	workerSession.SetPaused(paused)
	managed.TouchHeartbeat()

	emitEvent(parent, events.Event{
		Type:    "run.conversation.enabled",
		Message: "Run conversation persistence enabled",
		Data:    map[string]string{"runId": run.RunID},
	})
	s.startManagedWorkerLoop(parent, spawnWorkerLoopConfig{
		run:            run,
		activeProfile:  activeProfile,
		modelSource:    modelSource,
		workerSession:  workerSession,
		rt:             rt,
		orderedEmitter: orderedEmitter,
		emitEvent:      emitEvent,
		managed:        managed,
		wakeCancel:     wakeCancel,
	})
	return managed, nil
}

func (s *runtimeSupervisor) startManagedWorkerLoop(parent context.Context, cfg spawnWorkerLoopConfig) {
	run := cfg.run
	activeProfile := cfg.activeProfile
	modelSource := cfg.modelSource
	workerSession := cfg.workerSession
	rt := cfg.rt
	orderedEmitter := cfg.orderedEmitter
	emitEvent := cfg.emitEvent
	managed := cfg.managed
	wakeCancel := cfg.wakeCancel
	isChildRun := strings.TrimSpace(run.ParentRunID) != ""

	workerCtx, cancel := context.WithCancel(parent)
	done := make(chan struct{})

	go func() {
		defer close(done)
		defer orderedEmitter.Close()
		// Keep cleanup independent from workerCtx cancellation so runtime resources always release.
		defer func() {
		// Best-effort: runtime shutdown during cleanup
		if err := rt.Shutdown(context.Background()); err != nil {
			slog.Debug("runtime shutdown error", "error", err)
		}
	}()
		defer cancel()
		if wakeCancel != nil {
			defer wakeCancel()
		}

		emitEvent(workerCtx, events.Event{
			Type:    "run.start",
			Message: "Agent started",
			Data: map[string]string{
				"runId":        run.RunID,
				"sessionId":    run.SessionID,
				"profile":      strings.TrimSpace(activeProfile.ID),
				"model.source": modelSource,
			},
		})

		if isChildRun && run.Runtime != nil {
			run.Runtime.LifecycleState = "active"
			// Best-effort: save run state
			if err := s.sessionService.SaveRun(parent, run); err != nil {
				slog.Debug("failed to save run", "runID", run.RunID, "error", err)
			}
		}

		syncRuntimeControls := func() {
			loaded, err := s.sessionService.LoadSession(workerCtx, strings.TrimSpace(run.SessionID))
			if err != nil {
				return
			}
			// Only sync model for standalone top-level runs. Team runs must only update
			// model via explicit control.setModel to avoid unexpected resets.
			if shouldSyncModelFromSession(run, loaded) {
				targetModel := strings.TrimSpace(loaded.ActiveModel)
				if targetModel != "" {
					same := strings.EqualFold(targetModel, managed.CurrentModel())
					if !same {
						if err := workerSession.SetModel(workerCtx, targetModel); err == nil {
							managed.SetCurrentModel(targetModel)
							emitEvent(workerCtx, events.Event{
								Type:    "control.success",
								Message: "Model synchronized from session state",
								Data: map[string]string{
									"command": "set_model",
									"model":   targetModel,
									"source":  controlSourceSession,
								},
							})
						}
					}
				}
			}
			// Only sync reasoning from session for top-level runs; subagents keep profile/env settings from spawn.
			if !isChildRun {
				targetModel := strings.TrimSpace(loaded.ActiveModel)
				targetEffort, targetSummary := sessionReasoningForModel(
					loaded,
					targetModel,
					strings.TrimSpace(s.resolved.ReasoningEffort),
					strings.TrimSpace(s.resolved.ReasoningSummary),
				)
				// Best-effort: set reasoning configuration
		if err := workerSession.SetReasoning(workerCtx, targetEffort, targetSummary); err != nil {
			slog.Debug("failed to set reasoning", "error", err)
		}
			}
		}

		backoff := 2 * time.Second
		for {
			runLoopCtx, stopRunLoop := context.WithCancel(workerCtx)
			errCh := make(chan error, 1)
			go func() { errCh <- workerSession.Run(runLoopCtx) }()

			ticker := time.NewTicker(2 * time.Second)
			stopped := false
			for !stopped {
				select {
				case <-workerCtx.Done():
					stopRunLoop()
					<-errCh
					ticker.Stop()
					return
				case <-ticker.C:
					managed.TouchHeartbeat()
					syncRuntimeControls()
				case err := <-errCh:
					stopRunLoop()
					ticker.Stop()
					if workerCtx.Err() != nil {
						return
					}
					if errors.Is(err, agentsession.ErrSingleTaskComplete) {
						emitEvent(workerCtx, events.Event{
							Type:    "subagent.finished",
							Message: "Spawned worker completed its task",
							Data:    map[string]string{"runId": run.RunID},
						})

						// Transition to awaiting_review instead of stopping
						r, lerr := s.sessionService.LoadRun(workerCtx, run.RunID)
						if lerr == nil && r.Runtime != nil {
							r.Runtime.LifecycleState = "awaiting_review"
							// Best-effort: save run state
							if err := s.sessionService.SaveRun(workerCtx, r); err != nil {
								slog.Debug("failed to save run", "runID", r.RunID, "error", err)
							}
						}

						// Bound awaiting_review residency to prevent leaked subagent runtimes
						// when the expected review/cleanup path never arrives.
						waitTimeout := s.subagentAwaitingReviewTimeout()
						timer := time.NewTimer(waitTimeout)
						defer timer.Stop()
						select {
						case <-workerCtx.Done():
							return
						case <-timer.C:
							emitEvent(workerCtx, events.Event{
								Type:    "subagent.awaiting_review.timeout",
								Message: "Sub-agent archived after awaiting-review timeout",
								Data: map[string]string{
									"runId":   run.RunID,
									"timeout": waitTimeout.String(),
								},
							})
							if rr, serr := s.sessionService.LoadRun(workerCtx, run.RunID); serr == nil {
								if rr.Runtime != nil {
									rr.Runtime.LifecycleState = lifecycleArchived
									// Best-effort: save run state
									if err := s.sessionService.SaveRun(workerCtx, rr); err != nil {
										slog.Debug("failed to save run", "runID", rr.RunID, "error", err)
									}
								}
							}
							// stop run result is best-effort
							// Stop result is best-effort; worker may already be stopped
							_, _ = s.sessionService.StopRun(workerCtx, run.RunID, types.RunStatusSucceeded, stopReasonArchived)
							return
						}
					}
					errMsg := "unknown error"
					if err != nil {
						errMsg = err.Error()
					}
					emitEvent(workerCtx, events.Event{
						Type:    "daemon.runner.error",
						Message: "Runner exited unexpectedly; restarting",
						Data:    map[string]string{"error": errMsg},
					})
					select {
					case <-workerCtx.Done():
						return
					case <-time.After(backoff):
					}
					if backoff < 60*time.Second {
						backoff *= 2
						if backoff > 60*time.Second {
							backoff = 60 * time.Second
						}
					}
					stopped = true
				}
			}
		}
	}()

	managed.session = workerSession
	managed.cancel = cancel
	managed.done = done
}

func (s *runtimeSupervisor) PauseRun(ctx context.Context, runID string) error {
	return s.pauseRun(ctx, runID)
}

func (s *runtimeSupervisor) pauseRun(ctx context.Context, runID string) error {
	if s == nil {
		return nil
	}
	ctx = nonNilContext(ctx)
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return fmt.Errorf("run id is required")
	}
	run, err := s.sessionService.LoadRun(ctx, runID)
	if err != nil {
		return err
	}
	if strings.EqualFold(strings.TrimSpace(run.Status), types.RunStatusPaused) {
		s.setSnapshotPersistedStatus(runID, types.RunStatusPaused)
		if sendErr := s.trySendCmd(supervisorCmd{
			kind:      cmdStop,
			runID:     runID,
			sessionID: strings.TrimSpace(run.SessionID),
			paused:    true,
		}); sendErr != nil {
			return sendErr
		}
		return nil
	}
	run.Status = types.RunStatusPaused
	run.FinishedAt = nil
	run.Error = nil
	if err := s.sessionService.SaveRun(ctx, run); err != nil {
		return err
	}
	s.setSnapshotPersistedStatus(runID, types.RunStatusPaused)
	if sendErr := s.trySendCmd(supervisorCmd{
		kind:      cmdStop,
		runID:     runID,
		sessionID: strings.TrimSpace(run.SessionID),
		paused:    true,
	}); sendErr != nil {
		return sendErr
	}
	return nil
}

func (s *runtimeSupervisor) ResumeRun(ctx context.Context, runID string) error {
	if s == nil {
		return nil
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return fmt.Errorf("run id is required")
	}
	run, err := s.sessionService.LoadRun(ctx, runID)
	if err != nil {
		return err
	}
	status := strings.ToLower(strings.TrimSpace(run.Status))
	if status == types.RunStatusCanceled || status == types.RunStatusSucceeded || status == types.RunStatusFailed {
		return fmt.Errorf("run %s is terminal (%s)", runID, run.Status)
	}
	run.Status = types.RunStatusRunning
	run.FinishedAt = nil
	run.Error = nil
	if err := s.sessionService.SaveRun(ctx, run); err != nil {
		return err
	}
	s.setSnapshotPersistedStatus(runID, types.RunStatusRunning)

	if s.sessionService == nil {
		return nil
	}
	sess, err := s.sessionService.LoadSession(ctx, strings.TrimSpace(run.SessionID))
	if err != nil {
		return err
	}
	if err := s.trySendCmd(supervisorCmd{
		kind:      cmdSpawn,
		runID:     runID,
		sessionID: strings.TrimSpace(run.SessionID),
		sess:      &sess,
	}); err != nil {
		return err
	}
	if notifier, ok := s.taskService.(taskWakeNotifier); ok {
		teamID := ""
		if run.Runtime != nil {
			teamID = strings.TrimSpace(run.Runtime.TeamID)
		}
		notifier.NotifyWake(teamID, runID)
	}
	return nil
}

func (s *runtimeSupervisor) StopRun(ctx context.Context, runID string) error {
	return s.stopRun(ctx, runID)
}

func (s *runtimeSupervisor) stopRun(ctx context.Context, runID string) error {
	if s == nil {
		return nil
	}
	ctx = nonNilContext(ctx)
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return fmt.Errorf("run id is required")
	}
	run, err := s.sessionService.LoadRun(ctx, runID)
	if err != nil {
		return err
	}
	run.Status = types.RunStatusPaused
	run.FinishedAt = nil
	run.Error = nil
	if err := s.sessionService.SaveRun(ctx, run); err != nil {
		return err
	}
	s.setSnapshotPersistedStatus(runID, types.RunStatusPaused)

	_, err = s.taskService.CancelActiveTasksByRun(ctx, runID, "run stopped")
	if sendErr := s.trySendCmd(supervisorCmd{
		kind:      cmdStop,
		runID:     runID,
		sessionID: strings.TrimSpace(run.SessionID),
		paused:    true,
	}); sendErr != nil {
		return sendErr
	}
	return err
}

func nonNilContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func (s *runtimeSupervisor) PauseSession(ctx context.Context, sessionID string) ([]string, error) {
	return s.runSessionAction(ctx, sessionID, "pause", s.pauseRun)
}

func (s *runtimeSupervisor) ResumeSession(ctx context.Context, sessionID string) ([]string, error) {
	return s.runSessionAction(ctx, sessionID, "resume", s.ResumeRun)
}

func (s *runtimeSupervisor) StopSession(ctx context.Context, sessionID string) ([]string, error) {
	return s.runSessionAction(ctx, sessionID, "stop", s.stopRun)
}

func (s *runtimeSupervisor) runSessionAction(
	ctx context.Context,
	sessionID string,
	action string,
	apply func(context.Context, string) error,
) ([]string, error) {
	if s == nil {
		return nil, nil
	}
	action = strings.TrimSpace(action)
	if action == "" {
		return nil, fmt.Errorf("action is required")
	}
	if apply == nil {
		return nil, fmt.Errorf("apply function is required")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("session id is required")
	}
	if s.sessionService == nil {
		return nil, fmt.Errorf("session store not configured")
	}
	sess, err := s.sessionService.LoadSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	runIDs := collectSessionRunIDs(sess)
	affected := make([]string, 0, len(runIDs))
	var mu sync.Mutex
	var wg sync.WaitGroup
	errs := make([]string, 0, len(runIDs))
	sem := make(chan struct{}, sessionRunActionConcurrency)
	for _, runID := range runIDs {
		runID := strings.TrimSpace(runID)
		if runID == "" {
			continue
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(rid string) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := apply(ctx, rid); err != nil {
				mu.Lock()
				errs = append(errs, rid+": "+err.Error())
				mu.Unlock()
				return
			}
			mu.Lock()
			affected = append(affected, rid)
			mu.Unlock()
		}(runID)
	}
	wg.Wait()
	if len(errs) != 0 {
		return affected, fmt.Errorf("%s session partial failure: %s", action, strings.Join(errs, "; "))
	}
	return affected, nil
}

func (s *runtimeSupervisor) ApplySessionReasoning(ctx context.Context, sessionID, targetRunID, effort, summary string) ([]string, error) {
	if s == nil {
		return nil, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	targetRunID = strings.TrimSpace(targetRunID)
	if sessionID == "" {
		return nil, fmt.Errorf("session id is required")
	}
	effort = strings.TrimSpace(effort)
	summary = strings.TrimSpace(summary)
	if effort == "" && summary == "" {
		return nil, nil
	}
	snapshots := s.snapshotValues()
	applied := make([]string, 0, len(snapshots))
	for _, snap := range snapshots {
		worker := snap.rt
		if worker == nil || worker.session == nil {
			continue
		}
		if strings.TrimSpace(snap.SessionID) != sessionID {
			continue
		}
		runID := strings.TrimSpace(snap.RunID)
		if targetRunID != "" && targetRunID != runID {
			continue
		}
		if err := worker.session.SetReasoning(ctx, effort, summary); err != nil {
			return applied, err
		}
		applied = append(applied, runID)
	}
	return applied, nil
}

func (s *runtimeSupervisor) ApplySessionModel(ctx context.Context, sessionID, targetRunID, model string) ([]string, error) {
	if s == nil {
		return nil, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	targetRunID = strings.TrimSpace(targetRunID)
	model = strings.TrimSpace(model)
	if sessionID == "" {
		return nil, fmt.Errorf("session id is required")
	}
	if model == "" {
		return nil, fmt.Errorf("model is required")
	}
	snapshots := s.snapshotValues()
	applied := make([]string, 0, len(snapshots))
	for _, snap := range snapshots {
		worker := snap.rt
		if worker == nil || worker.session == nil {
			continue
		}
		if strings.TrimSpace(snap.SessionID) != sessionID {
			continue
		}
		runID := strings.TrimSpace(snap.RunID)
		if targetRunID != "" && targetRunID != runID {
			continue
		}
		// Do not apply session model to child runs (sub-agents); they keep the model set at spawn.
		wr, err := s.sessionService.LoadRun(ctx, runID)
		if err != nil {
			continue
		}
		if strings.TrimSpace(wr.ParentRunID) != "" {
			continue
		}
		runtimeModel := ""
		if wr.Runtime != nil {
			runtimeModel = strings.TrimSpace(wr.Runtime.Model)
		}
		currentModelMatches := strings.EqualFold(strings.TrimSpace(worker.CurrentModel()), model)
		runtimeModelMatches := strings.EqualFold(runtimeModel, model)
		if currentModelMatches && runtimeModelMatches {
			continue
		}
		if !currentModelMatches {
			if err := worker.session.SetModel(ctx, model); err != nil {
				return applied, err
			}
			worker.SetCurrentModel(model)
		}
		if !runtimeModelMatches {
			if wr.Runtime == nil {
				wr.Runtime = &types.RunRuntimeConfig{}
			}
			wr.Runtime.Model = model
			if err := s.sessionService.SaveRun(ctx, wr); err != nil {
				return applied, err
			}
		}
		applied = append(applied, runID)
	}
	return applied, nil
}

// makeSpawnWorkerFunc returns a SpawnWorkerFunc that creates a child Run and
// adds it to the session. The supervisor discovers the new run via syncOnce.
func (s *runtimeSupervisor) makeSpawnWorkerFunc(
	parentRun types.Run,
	parentModel string,
	parentEmit events.EmitFunc,
) hosttools.SpawnWorkerFunc {
	return func(ctx context.Context, goal, sessionID, parentRunID string) (string, string, error) {
		// Caller gates spawn via allowSubagents; we only get here when spawn is allowed.
		// Count existing children to determine spawn index.
		children, _ := s.sessionService.ListChildRuns(ctx, parentRunID)
		spawnIndex := len(children) + 1

		childRun := types.NewChildRun(parentRunID, goal, sessionID, spawnIndex)

		// Resolve subagent model: explicit override > parent role-level profile setting >
		// profile-level setting > default profile role-level/profile-level > parent model.
		subagentModel := strings.TrimSpace(s.resolved.SubagentModel)
		parentRole := ""
		parentProfileRef := ""
		if parentRun.Runtime != nil {
			parentRole = strings.TrimSpace(parentRun.Runtime.Role)
			parentProfileRef = strings.TrimSpace(parentRun.Runtime.Profile)
		}
		if subagentModel == "" && parentProfileRef != "" {
			if prof, _, err := resolveProfileRef(s.cfg, parentProfileRef); err == nil && prof != nil {
				subagentModel = resolveSubagentModelForRole(prof, parentRole)
			}
		}
		if subagentModel == "" && s.defaultProfile != nil {
			subagentModel = resolveSubagentModelForRole(s.defaultProfile, parentRole)
		}
		if subagentModel == "" {
			subagentModel = parentModel
		}

		childRun.Runtime = &types.RunRuntimeConfig{
			DataDir:        s.cfg.DataDir,
			Model:          subagentModel,
			Role:           fmt.Sprintf("Subagent-%d", spawnIndex),
			WorkerClass:    "ephemeral",
			LifecycleState: "spawn_requested",
			LeaseID:        "lease-" + childRun.RunID,
		}
		if parentRun.Runtime != nil {
			childRun.Runtime.Profile = parentRun.Runtime.Profile
		}

		if err := s.sessionService.SaveRun(ctx, childRun); err != nil {
			return "", "", fmt.Errorf("save child run: %w", err)
		}

		// Add child run to session's run list so the supervisor discovers it.
		sess, err := s.sessionService.LoadSession(ctx, sessionID)
		if err != nil {
			return "", "", fmt.Errorf("load session for spawn: %w", err)
		}
		sess.Runs = append(sess.Runs, childRun.RunID)
		if err := s.sessionService.SaveSession(ctx, sess); err != nil {
			return "", "", fmt.Errorf("save session for spawn: %w", err)
		}

		if parentEmit != nil {
			parentEmit(ctx, events.Event{
				Type:    "subagent.spawned",
				Message: fmt.Sprintf("Spawned worker agent #%d: %s", spawnIndex, goal),
				Data: map[string]string{
					"childRunId":  childRun.RunID,
					"parentRunId": parentRunID,
					"spawnIndex":  strconv.Itoa(spawnIndex),
					"goal":        goal,
					"model":       subagentModel,
				},
			})
		}

		return childRun.RunID, strings.TrimSpace(childRun.Runtime.Role), nil
	}
}

func resolveSubagentModelForRole(prof *profile.Profile, roleName string) string {
	if prof == nil {
		return ""
	}
	roleName = strings.TrimSpace(roleName)
	if prof.Team != nil && roleName != "" {
		for i := range prof.Team.Roles {
			r := prof.Team.Roles[i]
			if strings.EqualFold(strings.TrimSpace(r.Name), roleName) {
				if m := strings.TrimSpace(r.SubagentModel); m != "" {
					return m
				}
				break
			}
		}
	}
	return strings.TrimSpace(prof.SubagentModel)
}

func (s *runtimeSupervisor) GetRunState(ctx context.Context, sessionID, runID string) (protocol.RuntimeRunState, error) {
	runID = strings.TrimSpace(runID)
	sessionID = strings.TrimSpace(sessionID)
	if runID == "" || sessionID == "" {
		return protocol.RuntimeRunState{}, fmt.Errorf("sessionID and runID are required")
	}
	run, err := s.sessionService.LoadRun(ctx, runID)
	if err != nil {
		return protocol.RuntimeRunState{}, err
	}
	state := protocol.RuntimeRunState{
		SessionID:       sessionID,
		RunID:           runID,
		Model:           "",
		PersistedStatus: strings.TrimSpace(run.Status),
		EffectiveStatus: strings.TrimSpace(run.Status),
	}
	if run.Runtime != nil {
		state.Model = strings.TrimSpace(run.Runtime.Model)
	}
	if stats, statsErr := s.taskService.GetRunStats(ctx, runID); statsErr == nil {
		state.RunTotalTokens = stats.TotalTokens
		state.RunTotalCostUSD = stats.TotalCost
	}
	snap, hasSnap := s.getSnapshot(runID)
	if hasSnap {
		state.WorkerPresent = snap.WorkerPresent
		if snap.HandleState == handleStateSpawning {
			state.EffectiveStatus = "spawning"
		}
		if snap.HandleState == handleStateDraining {
			state.EffectiveStatus = "draining"
		}
		if strings.TrimSpace(snap.Model) != "" {
			state.Model = strings.TrimSpace(snap.Model)
		}
		lastBeat := snap.LastHeartbeatAt
		if !lastBeat.IsZero() {
			state.LastHeartbeatAt = lastBeat.UTC().Format(time.RFC3339Nano)
		}
	}
	if state.WorkerPresent && strings.EqualFold(state.PersistedStatus, types.RunStatusRunning) {
		if hasSnap && (snap.HandleState == handleStateSpawning || snap.HandleState == handleStateDraining) {
			// Keep transitional state visible.
		} else {
			state.EffectiveStatus = types.RunStatusRunning
		}
	}
	state.PausedFlag = strings.EqualFold(state.PersistedStatus, types.RunStatusPaused)
	return state, nil
}

func (s *runtimeSupervisor) GetSessionState(ctx context.Context, sessionID string) ([]protocol.RuntimeRunState, error) {
	sess, err := s.sessionService.LoadSession(ctx, strings.TrimSpace(sessionID))
	if err != nil {
		return nil, err
	}
	out := make([]protocol.RuntimeRunState, 0, len(sess.Runs))
	for _, rid := range sess.Runs {
		rid = strings.TrimSpace(rid)
		if rid == "" {
			continue
		}
		st, err := s.GetRunState(ctx, strings.TrimSpace(sessionID), rid)
		if err != nil {
			continue
		}
		out = append(out, st)
	}
	return out, nil
}

// RetrySubagent creates a new retry task for an existing child run so the
// sub-agent can re-attempt its work with the parent's feedback.
func (s *runtimeSupervisor) RetrySubagent(ctx context.Context, childRunID, feedback string) error {
	return s.taskService.CreateRetryTask(ctx, childRunID, feedback)
}

// EscalateTask creates an escalation task with structured metadata. In team mode
// it routes to the coordinator; in standalone mode it routes to the parent run
// and emits a user-facing event.
func (s *runtimeSupervisor) EscalateTask(ctx context.Context, callbackTaskID string, data hosttools.EscalationData) error {
	if err := s.taskService.CreateEscalationTask(ctx, callbackTaskID, data); err != nil {
		return err
	}
	// Stop the child run after escalation.
	childRunID := strings.TrimSpace(data.SourceRunID)
	if childRunID != "" {
		var stopErr error
		if err := s.StopRun(ctx, childRunID); err != nil {
			stopErr = errors.Join(stopErr, fmt.Errorf("stop child runtime %s: %w", childRunID, err))
		}
		if s.sessionService != nil {
			if _, err := s.sessionService.StopRun(ctx, childRunID, types.RunStatusFailed, "escalated"); err != nil {
				stopErr = errors.Join(stopErr, fmt.Errorf("persist child run stop %s: %w", childRunID, err))
			}
		}
		if stopErr != nil {
			return stopErr
		}
	}
	return nil
}
