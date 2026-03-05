package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
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

	mu                  sync.Mutex
	workers             map[string]*managedRuntime
	lastRoutingRepairAt time.Time

	spawnOverride func(context.Context, types.Session, string) (*managedRuntime, error)
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
			log.Printf("daemon: deactivate subagent: save run %s: %v", runID, err)
		}
	}

	s.mu.Lock()
	worker := s.workers[runID]
	s.mu.Unlock()

	if worker != nil {
		if worker.cancel != nil {
			worker.cancel()
		}
		if worker.done != nil {
			timeout := s.workerShutdownTimeout()
			if !waitWorkerDone(worker.done, timeout) {
				log.Printf("daemon: timed out waiting for worker shutdown during subagent cleanup: run=%s timeout=%s", runID, timeout)
			}
		}
	}

	if run, err := s.sessionService.LoadRun(ctx, runID); err == nil {
		if run.Runtime != nil {
			run.Runtime.LifecycleState = lifecycleArchived
			if err := s.sessionService.SaveRun(ctx, run); err != nil {
				log.Printf("daemon: archive subagent: save run %s: %v", runID, err)
			}
		}
		_, _ = s.sessionService.StopRun(ctx, runID, types.RunStatusSucceeded, stopReasonArchived)
	}

	// Remove worker only after cancellation + terminal run status update to avoid
	// a syncOnce respawn window while the run still appears running.
	s.mu.Lock()
	if current, ok := s.workers[runID]; ok && current == worker {
		delete(s.workers, runID)
	}
	s.mu.Unlock()
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
		workers:          map[string]*managedRuntime{},
	}
}

func (s *runtimeSupervisor) subagentAwaitingReviewTimeout() time.Duration {
	if s == nil {
		return defaultSubagentAwaitingReviewTimeout
	}
	raw := strings.TrimSpace(os.Getenv("AGEN8_SUBAGENT_AWAITING_REVIEW_TIMEOUT"))
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
	var (
		wakeCh     <-chan struct{}
		wakeCancel func()
	)
	if wakeSub, ok := s.sessionService.(sessionWakeSubscriber); ok && wakeSub != nil {
		wakeCh, wakeCancel = wakeSub.SubscribeWake("", "")
	}
	if wakeCancel != nil {
		defer wakeCancel()
	}
	if wakeCh == nil {
		// Compatibility fallback for non-pubsub session service implementations.
		if err := s.syncOnce(ctx); err != nil {
			log.Printf("daemon: runtime supervisor sync failed: %v", err)
		}
		s.runWithPollingFallback(ctx)
		return
	}
	if err := s.syncOnce(ctx); err != nil {
		log.Printf("daemon: runtime supervisor sync failed: %v", err)
	}
	for {
		select {
		case <-ctx.Done():
			s.stopAll()
			return
		case <-wakeCh:
			if err := s.syncOnce(ctx); err != nil {
				log.Printf("daemon: runtime supervisor sync failed: %v", err)
			}
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
				log.Printf("daemon: runtime supervisor sync failed: %v", err)
			}
		}
	}
}

func (s *runtimeSupervisor) stopAll() {
	s.mu.Lock()
	workers := make([]*managedRuntime, 0, len(s.workers))
	for _, w := range s.workers {
		workers = append(workers, w)
	}
	s.workers = map[string]*managedRuntime{}
	s.mu.Unlock()
	for _, w := range workers {
		if w == nil {
			continue
		}
		if w.cancel != nil {
			w.cancel()
		}
		if w.done != nil {
			<-w.done
		}
	}
}

func (s *runtimeSupervisor) syncOnce(ctx context.Context) error {
	if s == nil || s.sessionService == nil {
		return nil
	}
	runs, err := s.sessionService.ListRunsByStatus(ctx, []string{types.RunStatusRunning, types.RunStatusPaused})
	if err != nil {
		return err
	}
	for _, run := range runs {
		sess, lerr := s.sessionService.LoadSession(ctx, run.SessionID)
		if lerr != nil {
			log.Printf("daemon: load session for run %s: %v", run.RunID, lerr)
			continue
		}
		teamID := strings.TrimSpace(sess.TeamID)
		if teamID != "" && run.Runtime != nil && strings.TrimSpace(run.Runtime.Role) == "" {
			if _, roleByRun := loadTeamManifestRunRolesFromStore(ctx, team.NewFileManifestStore(s.cfg), teamID); len(roleByRun) != 0 {
				if role := strings.TrimSpace(roleByRun[strings.TrimSpace(run.RunID)]); role != "" {
					run.Runtime.Role = role
					_ = s.sessionService.SaveRun(ctx, run)
				}
			}
		}
		if err := s.ensureRun(ctx, sess, run.RunID); err != nil {
			log.Printf("daemon: managed run start failed for %s: %v", run.RunID, err)
		}
	}
	s.maybeRepairRoutingDrift(ctx)
	return nil
}

func (s *runtimeSupervisor) maybeRepairRoutingDrift(ctx context.Context) {
	repairer, ok := s.taskService.(routingDriftRepairer)
	if !ok || repairer == nil {
		return
	}
	now := time.Now().UTC()
	s.mu.Lock()
	if !s.lastRoutingRepairAt.IsZero() && now.Sub(s.lastRoutingRepairAt) < 10*time.Second {
		s.mu.Unlock()
		return
	}
	s.lastRoutingRepairAt = now
	s.mu.Unlock()
	n, err := repairer.RepairRoutingDrift(ctx, 400)
	if err != nil {
		log.Printf("daemon: routing drift repair failed: %v", err)
		return
	}
	if n > 0 {
		log.Printf("daemon: routing drift repaired %d task(s)", n)
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

func (s *runtimeSupervisor) ensureRun(ctx context.Context, sess types.Session, runID string) error {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil
	}
	run, err := s.sessionService.LoadRun(ctx, runID)
	if err != nil {
		return err
	}
	paused := strings.EqualFold(strings.TrimSpace(run.Status), types.RunStatusPaused)

	s.mu.Lock()
	if existing, ok := s.workers[runID]; ok {
		if existing != nil && !workerDone(existing.done) {
			s.mu.Unlock()
			if existing.session != nil {
				existing.session.SetPaused(paused)
			}
			return nil
		}
		delete(s.workers, runID)
	}
	s.mu.Unlock()

	isChild := strings.TrimSpace(run.ParentRunID) != ""
	if isChild && run.Runtime != nil && run.Runtime.LifecycleState == "spawn_requested" {
		run.Runtime.LifecycleState = "spawning"
		_ = s.sessionService.SaveRun(ctx, run)
	}

	startFn := s.spawnOverride
	if startFn == nil {
		startFn = s.spawnManagedRun
	}
	managed, err := startFn(ctx, sess, runID)
	if err != nil {
		return err
	}
	if managed == nil {
		return fmt.Errorf("managed runtime is nil")
	}

	s.mu.Lock()
	if existing, ok := s.workers[runID]; ok {
		if existing != nil && !workerDone(existing.done) {
			s.mu.Unlock()
			if managed.cancel != nil {
				managed.cancel()
			}
			if managed.done != nil {
				<-managed.done
			}
			return nil
		}
		delete(s.workers, runID)
	}
	s.workers[runID] = managed
	s.mu.Unlock()
	return nil
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
			log.Printf("daemon: emit failed (%s): %v", strings.TrimSpace(run.RunID), err)
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
		_ = rt.Shutdown(parent)
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
		_ = rt.Shutdown(parent)
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
			_ = rt.Shutdown(parent)
			return spawnToolWiring{}, err
		}
	}
	if s.soulService != nil {
		if err := registry.Register(&hosttools.SoulUpdateTool{Updater: s.soulService, Actor: pkgsoul.ActorAgent}); err != nil {
			orderedEmitter.Close()
			_ = rt.Shutdown(parent)
			return spawnToolWiring{}, err
		}
	}
	if err := registry.Register(&hosttools.ObsidianTool{ProjectRoot: s.workdirAbs}); err != nil {
		orderedEmitter.Close()
		_ = rt.Shutdown(parent)
		return spawnToolWiring{}, err
	}

	var allowedToolsForRun []string
	if !isChildRun {
		roleAllowedTools, removedTools := sanitizeAllowedToolsForRole(activeProfile.AllowedTools, teamID, role.isCoordinator)
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
		_ = rt.Shutdown(parent)
		return spawnToolWiring{}, err
	}
	if err := configureCodeExecRuntime(parent, rt, s.cfg, modelRegistry, bridgeRegistry, resolvedCodeExecRequiredImports, codeExecOnly, emitEvent); err != nil {
		orderedEmitter.Close()
		_ = rt.Shutdown(parent)
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
		_ = s.sessionService.SaveSession(parent, sess)
	}

	if run.Runtime == nil {
		run.Runtime = &types.RunRuntimeConfig{}
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
	_ = s.sessionService.SaveRun(parent, run)

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
		_ = rt.Shutdown(parent)
		return nil, err
	}
	if reviewerRole == "" && coordinatorRole != "" {
		reviewerRole = strings.TrimSpace(coordinatorRole)
	}
	runConvStore, err := implstore.NewSQLiteRunConversationStoreFromConfig(s.cfg)
	if err != nil {
		orderedEmitter.Close()
		_ = rt.Shutdown(parent)
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
		_ = rt.Shutdown(parent)
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
			log.Printf("daemon [%s]: "+format, append([]any{run.RunID}, args...)...)
		},
	})
	if err != nil {
		orderedEmitter.Close()
		_ = rt.Shutdown(parent)
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
		defer func() { _ = rt.Shutdown(context.Background()) }()
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
			_ = s.sessionService.SaveRun(parent, run)
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
				_ = workerSession.SetReasoning(workerCtx, targetEffort, targetSummary)
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
							_ = s.sessionService.SaveRun(workerCtx, r)
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
									_ = s.sessionService.SaveRun(workerCtx, rr)
								}
							}
							_, _ = s.sessionService.StopRun(workerCtx, run.RunID, types.RunStatusSucceeded, "archived: awaiting review timeout")
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
		// Cancel active tasks synchronously, then drain worker in background.
		_, err := s.taskService.CancelActiveTasksByRun(ctx, runID, "run paused")
		go s.stopWorker(runID, true)
		return err
	}
	run.Status = types.RunStatusPaused
	run.FinishedAt = nil
	run.Error = nil
	if err := s.sessionService.SaveRun(ctx, run); err != nil {
		return err
	}

	// Cancel active tasks first to help the in-flight LLM call abort cleanly.
	_, err = s.taskService.CancelActiveTasksByRun(ctx, runID, "run paused")
	// Stop worker asynchronously — run is already saved as paused in DB.
	// stopWorker holds its own pointer reference and only deletes its own entry,
	// so a concurrent ResumeRun+ensureRun spawning a fresh worker is safe.
	go s.stopWorker(runID, true)
	return err
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

	if s.sessionService == nil {
		return nil
	}
	sess, err := s.sessionService.LoadSession(ctx, strings.TrimSpace(run.SessionID))
	if err != nil {
		return err
	}
	// ensureRun handles all worker states:
	//   - live worker  → SetPaused(false), no new spawn
	//   - zombie worker → delete zombie, spawn fresh worker with startup tick()
	//   - no worker    → spawn fresh worker with startup tick()
	// This eliminates the TOCTOU race of checking workerDone then acting on it.
	if err := s.ensureRun(ctx, sess, runID); err != nil {
		return err
	}
	// Always send a wake signal. For live workers that ensureRun merely unpaused,
	// this kicks the session to drain inbox messages immediately instead of
	// waiting for the next external event. For freshly-spawned workers the
	// startup tick() already handles the inbox, but the signal is harmless.
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
	run.Status = types.RunStatusCanceled
	now := time.Now().UTC()
	run.FinishedAt = &now
	run.Error = nil
	if err := s.sessionService.SaveRun(ctx, run); err != nil {
		return err
	}

	// Cancel active tasks first to help the in-flight LLM call abort cleanly.
	_, err = s.taskService.CancelActiveTasksByRun(ctx, runID, "run stopped")

	// Stop worker asynchronously — run is already terminal in DB so syncOnce
	// won't respawn it. Cleanup (ctx cancel + map delete) happens in background.
	go s.stopWorker(runID, true)

	return err
}

func nonNilContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func (s *runtimeSupervisor) stopWorker(runID string, paused bool) {
	if s == nil {
		return
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return
	}
	s.mu.Lock()
	worker := s.workers[runID]
	s.mu.Unlock()
	if worker != nil && worker.session != nil {
		worker.session.SetPaused(paused)
	}
	if worker != nil && worker.cancel != nil {
		worker.cancel()
	}
	if worker != nil && worker.done != nil {
		<-worker.done
	}
	// Only delete our own entry. A concurrent ResumeRun → ensureRun may have
	// already replaced this zombie with a fresh worker; guard against clobbering it.
	s.mu.Lock()
	if s.workers[runID] == worker {
		delete(s.workers, runID)
	}
	s.mu.Unlock()

	// If the run was resumed while we were draining the zombie, spawn a fresh
	// worker now so the resumed session can pick up any pending inbox messages.
	// ensureRun is a no-op if a live worker already exists.
	if worker != nil && s.sessionService != nil {
		bg := context.Background()
		if run, err := s.sessionService.LoadRun(bg, runID); err == nil &&
			strings.TrimSpace(run.Status) == types.RunStatusRunning {
			if sess, err := s.sessionService.LoadSession(bg, strings.TrimSpace(run.SessionID)); err == nil {
				_ = s.ensureRun(bg, sess, runID)
			}
		}
	}
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
	s.mu.Lock()
	workers := make([]*managedRuntime, 0, len(s.workers))
	for _, w := range s.workers {
		workers = append(workers, w)
	}
	s.mu.Unlock()
	applied := make([]string, 0, len(workers))
	for _, worker := range workers {
		if worker == nil || worker.session == nil {
			continue
		}
		if strings.TrimSpace(worker.sessionID) != sessionID {
			continue
		}
		runID := strings.TrimSpace(worker.runID)
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
	s.mu.Lock()
	workers := make([]*managedRuntime, 0, len(s.workers))
	for _, w := range s.workers {
		workers = append(workers, w)
	}
	s.mu.Unlock()
	applied := make([]string, 0, len(workers))
	for _, worker := range workers {
		if worker == nil || worker.session == nil {
			continue
		}
		if strings.TrimSpace(worker.sessionID) != sessionID {
			continue
		}
		runID := strings.TrimSpace(worker.runID)
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
	s.mu.Lock()
	worker := s.workers[runID]
	s.mu.Unlock()
	if worker != nil {
		state.WorkerPresent = true
		if strings.TrimSpace(state.Model) == "" {
			state.Model = strings.TrimSpace(worker.CurrentModel())
		}
		lastBeat := worker.LastHeartbeatAt()
		if !lastBeat.IsZero() {
			state.LastHeartbeatAt = lastBeat.UTC().Format(time.RFC3339Nano)
		}
	}
	if state.WorkerPresent && strings.EqualFold(state.PersistedStatus, types.RunStatusRunning) {
		state.EffectiveStatus = types.RunStatusRunning
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
