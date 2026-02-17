package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tinoosan/workbench-core/pkg/agent"
	"github.com/tinoosan/workbench-core/pkg/agent/state"
	"github.com/tinoosan/workbench-core/pkg/cost"
	"github.com/tinoosan/workbench-core/pkg/emit"
	"github.com/tinoosan/workbench-core/pkg/events"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/llm"
	llmtypes "github.com/tinoosan/workbench-core/pkg/llm/types"
	"github.com/tinoosan/workbench-core/pkg/profile"
	"github.com/tinoosan/workbench-core/pkg/timeutil"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// ErrSingleTaskComplete is returned by Run when SingleTask mode is enabled
// and the session has completed its assigned task.
var ErrSingleTaskComplete = errors.New("single task completed")

type ResolveProfileFunc func(ref string) (*profile.Profile, string, error)

type Config struct {
	Agent agent.Agent

	Profile        *profile.Profile
	ProfileDir     string
	ResolveProfile ResolveProfileFunc

	TaskStore state.TaskStore
	Events    emit.Emitter[events.Event]

	Memory            agent.MemoryRecallProvider
	MemorySearchLimit int
	Notifier          agent.Notifier

	PollInterval time.Duration
	// WakeCh, when signaled, nudges the session loop to drain the inbox immediately.
	// This is used by the app server to provide low-latency turn.create.
	WakeCh       <-chan struct{}
	MaxReadBytes int

	LeaseTTL    time.Duration
	LeaseExtend time.Duration

	MaxRetries int
	MaxPending int

	SessionID string
	RunID     string

	TeamID               string
	RoleName             string
	IsCoordinator        bool
	CoordinatorRole      string
	TeamRoles            []string // all role names, for prompt injection
	TeamRoleDescriptions map[string]string

	// ParentRunID links this session's run to its parent when spawned as a worker.
	// Used to trigger coordinator callbacks in standalone mode.
	ParentRunID string
	// SpawnIndex is the 1-based standalone subagent ordinal under a parent run.
	SpawnIndex int
	// SingleTask causes the session to exit after completing one task.
	SingleTask bool

	InstanceID string
	Logf       func(format string, args ...any)
}

type Session struct {
	cfg Config

	activeProfile    *profile.Profile
	activeProfileDir string
	activePromptText string

	// lastTaskOutcome is a short, single-line summary of the most recently completed task.
	// It is used to provide immediate continuity between sequential tasks in the run loop.
	lastTaskOutcome string

	hbCh   chan profile.HeartbeatJob
	hbStop context.CancelFunc

	queuedEmitted map[string]time.Time

	pauseMu sync.RWMutex
	paused  bool
}

func New(cfg Config) (*Session, error) {
	if cfg.Agent == nil {
		return nil, fmt.Errorf("agent is required")
	}
	if cfg.Profile == nil {
		return nil, fmt.Errorf("profile is required")
	}
	if cfg.TaskStore == nil {
		return nil, fmt.Errorf("task store is required")
	}
	if strings.TrimSpace(cfg.SessionID) == "" {
		return nil, fmt.Errorf("sessionID is required")
	}
	if strings.TrimSpace(cfg.RunID) == "" {
		return nil, fmt.Errorf("runID is required")
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 2 * time.Second
	}
	if cfg.MaxReadBytes <= 0 {
		cfg.MaxReadBytes = 256 * 1024
	}
	if cfg.MemorySearchLimit <= 0 {
		cfg.MemorySearchLimit = 3
	}
	if cfg.LeaseTTL <= 0 {
		cfg.LeaseTTL = 2 * time.Minute
	}
	if cfg.LeaseExtend <= 0 {
		cfg.LeaseExtend = cfg.LeaseTTL / 2
		if cfg.LeaseExtend <= 0 {
			cfg.LeaseExtend = 30 * time.Second
		}
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	if cfg.MaxPending <= 0 {
		cfg.MaxPending = 50
	}
	if strings.TrimSpace(cfg.InstanceID) == "" {
		cfg.InstanceID = "instance"
	}
	if strings.TrimSpace(cfg.TeamID) != "" {
		cfg.TeamID = strings.TrimSpace(cfg.TeamID)
		cfg.RoleName = strings.TrimSpace(cfg.RoleName)
		cfg.CoordinatorRole = strings.TrimSpace(cfg.CoordinatorRole)
		if cfg.RoleName == "" {
			return nil, fmt.Errorf("roleName is required in team mode")
		}
		if cfg.CoordinatorRole == "" {
			cfg.CoordinatorRole = cfg.RoleName
		}
		roles := make([]string, 0, len(cfg.TeamRoles))
		seen := map[string]struct{}{}
		for _, role := range cfg.TeamRoles {
			role = strings.TrimSpace(role)
			if role == "" {
				continue
			}
			if _, ok := seen[role]; ok {
				continue
			}
			seen[role] = struct{}{}
			roles = append(roles, role)
		}
		if len(roles) == 0 {
			roles = append(roles, cfg.RoleName)
			if cfg.CoordinatorRole != cfg.RoleName {
				roles = append(roles, cfg.CoordinatorRole)
			}
		}
		cfg.TeamRoles = roles
	}

	s := &Session{cfg: cfg}
	s.queuedEmitted = make(map[string]time.Time)
	if err := s.setProfile(cfg.Profile, cfg.ProfileDir); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Session) Run(ctx context.Context) error {
	if s == nil {
		return fmt.Errorf("session is nil")
	}
	_ = s.cfg.TaskStore.RecoverExpiredLeases(ctx)
	s.startHeartbeats(ctx)
	defer s.stopHeartbeats()

	basePoll := s.cfg.PollInterval
	if basePoll <= 0 {
		basePoll = 2 * time.Second
	}
	maxPoll := basePoll * 8
	if maxPoll < 10*time.Second {
		maxPoll = 10 * time.Second
	}
	poll := basePoll
	timer := time.NewTimer(poll)
	defer timer.Stop()

	tick := func() error {
		hadCandidates, err := s.drainInbox(ctx)
		if err != nil {
			s.logf("inbox drain error: %v", err)
			s.emitBestEffort(ctx, events.Event{Type: "daemon.error", Message: "Inbox drain error", Data: map[string]string{"error": err.Error()}})
		}
		if hadCandidates {
			poll = basePoll
		} else if poll < maxPoll {
			poll *= 2
			if poll > maxPoll {
				poll = maxPoll
			}
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(poll)
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case job := <-s.hbCh:
			if s.IsPaused() {
				continue
			}
			s.handleHeartbeat(ctx, job)
		case <-s.cfg.WakeCh:
			if s.IsPaused() {
				poll = basePoll
				timer.Reset(poll)
				continue
			}
			_ = tick()
		case <-timer.C:
			if s.IsPaused() {
				poll = basePoll
				timer.Reset(poll)
				continue
			}
			_ = tick()
		}
	}
}

func (s *Session) SetPaused(paused bool) {
	if s == nil {
		return
	}
	s.pauseMu.Lock()
	changed := s.paused != paused
	s.paused = paused
	s.pauseMu.Unlock()
	if !changed {
		return
	}
	evType := "run.resumed"
	evMsg := "Run resumed"
	if paused {
		evType = "run.paused"
		evMsg = "Run paused"
	}
	s.emitBestEffort(context.Background(), events.Event{
		Type:    evType,
		Message: evMsg,
		Data: map[string]string{
			"runId":     strings.TrimSpace(s.cfg.RunID),
			"sessionId": strings.TrimSpace(s.cfg.SessionID),
		},
	})
}

func (s *Session) IsPaused() bool {
	if s == nil {
		return false
	}
	s.pauseMu.RLock()
	defer s.pauseMu.RUnlock()
	return s.paused
}

func (s *Session) emitBestEffort(ctx context.Context, ev events.Event) {
	if s == nil || s.cfg.Events == nil {
		return
	}
	if ev.Data == nil {
		ev.Data = map[string]string{}
	}
	if strings.TrimSpace(s.cfg.TeamID) != "" {
		ev.Data["teamId"] = s.cfg.TeamID
	}
	if strings.TrimSpace(s.cfg.RoleName) != "" {
		ev.Data["role"] = s.cfg.RoleName
	}
	_ = s.cfg.Events.Emit(ctx, ev)
}

func (s *Session) logf(format string, args ...any) {
	if s == nil || s.cfg.Logf == nil {
		return
	}
	s.cfg.Logf(format, args...)
}

func (s *Session) startHeartbeats(ctx context.Context) {
	s.stopHeartbeats()
	ch := make(chan profile.HeartbeatJob, 16)
	s.hbCh = ch
	hbCtx, cancel := context.WithCancel(ctx)
	s.hbStop = cancel

	// One ticker per job; fan-in into hbCh.
	for _, job := range s.activeProfile.Heartbeat {
		j := job
		if j.Interval <= 0 {
			continue
		}
		s.emitBestEffort(ctx, events.Event{
			Type:    "task.heartbeat.configured",
			Message: "Heartbeat job configured",
			Data: map[string]string{
				"profile":         s.activeProfile.ID,
				"job":             strings.TrimSpace(j.Name),
				"goal":            truncateText(strings.TrimSpace(j.Goal), 200),
				"interval":        j.Interval.String(),
				"intervalSeconds": fmt.Sprintf("%d", int64(j.Interval/time.Second)),
			},
		})
		go func(out chan<- profile.HeartbeatJob) {
			t := time.NewTicker(j.Interval)
			defer t.Stop()
			for {
				select {
				case <-hbCtx.Done():
					return
				case <-t.C:
					select {
					case <-hbCtx.Done():
						return
					case out <- j:
					default:
						// drop if overwhelmed; next tick will retry
					}
				}
			}
		}(ch)
	}
}

func (s *Session) stopHeartbeats() {
	if s == nil {
		return
	}
	if s.hbStop != nil {
		s.hbStop()
		s.hbStop = nil
	}
	s.hbCh = nil
}

func (s *Session) setProfile(p *profile.Profile, dir string) error {
	if p == nil {
		return fmt.Errorf("profile is nil")
	}
	promptText := ""
	switch {
	case strings.TrimSpace(p.Prompts.SystemPrompt) != "":
		promptText = strings.TrimSpace(p.Prompts.SystemPrompt)
	case strings.TrimSpace(p.Prompts.SystemPromptPath) != "" && strings.TrimSpace(dir) != "":
		raw, err := os.ReadFile(filepath.Join(dir, p.Prompts.SystemPromptPath))
		if err != nil {
			return fmt.Errorf("read profile prompt: %w", err)
		}
		promptText = strings.TrimSpace(string(raw))
	}
	s.activeProfile = p
	s.activeProfileDir = strings.TrimSpace(dir)
	s.activePromptText = promptText
	return nil
}

func (s *Session) handleHeartbeat(ctx context.Context, job profile.HeartbeatJob) {
	// Backpressure: if inbox is already large, skip emitting more tasks.
	filter := state.TaskFilter{RunID: s.cfg.RunID, Status: []types.TaskStatus{types.TaskStatusPending}}
	if strings.TrimSpace(s.cfg.TeamID) != "" {
		filter = state.TaskFilter{
			TeamID:         s.cfg.TeamID,
			AssignedToType: "role",
			AssignedTo:     s.cfg.RoleName,
			Status:         []types.TaskStatus{types.TaskStatusPending},
		}
	}
	if count, err := s.cfg.TaskStore.CountTasks(ctx, filter); err == nil && count > s.cfg.MaxPending {
		s.emitBestEffort(ctx, events.Event{
			Type:    "task.heartbeat.skipped",
			Message: "Heartbeat skipped due to backpressure",
			Data:    map[string]string{"profile": s.activeProfile.ID, "job": job.Name, "inboxCount": fmt.Sprintf("%d", count)},
		})
		return
	}

	now := time.Now().UTC()
	window := now.Truncate(job.Interval).Unix()
	taskID := fmt.Sprintf("heartbeat-%s-%s-%s-%d", s.activeProfile.ID, s.cfg.InstanceID, job.Name, window)

	if _, err := s.cfg.TaskStore.GetTask(ctx, taskID); err == nil {
		return
	} else if !errors.Is(err, state.ErrTaskNotFound) {
		s.logf("heartbeat get task failed: %v", err)
		return
	}

	task := types.Task{
		TaskID:         taskID,
		SessionID:      s.cfg.SessionID,
		RunID:          s.cfg.RunID,
		TeamID:         strings.TrimSpace(s.cfg.TeamID),
		AssignedRole:   strings.TrimSpace(s.cfg.RoleName),
		AssignedToType: "agent",
		AssignedTo:     strings.TrimSpace(s.cfg.RunID),
		CreatedBy:      strings.TrimSpace(s.cfg.RoleName),
		TaskKind:       state.TaskKindHeartbeat,
		Goal:           strings.TrimSpace(job.Goal),
		Priority:       5,
		Status:         types.TaskStatusPending,
		CreatedAt:      &now,
		Metadata: map[string]any{
			"source":          "heartbeat",
			"profile":         s.activeProfile.ID,
			"job":             strings.TrimSpace(job.Name),
			"window":          window,
			"interval":        job.Interval.String(),
			"intervalSeconds": int64(job.Interval / time.Second),
		},
	}
	if strings.TrimSpace(task.TeamID) != "" {
		task.AssignedToType = "role"
		task.AssignedTo = strings.TrimSpace(task.AssignedRole)
	}
	if err := s.cfg.TaskStore.CreateTask(ctx, task); err != nil {
		s.logf("heartbeat enqueue failed: %v", err)
		return
	}
	s.emitTaskQueuedOnce(ctx, taskID, task.Goal, "heartbeat")
	s.emitBestEffort(ctx, events.Event{
		Type:    "task.heartbeat.enqueued",
		Message: "Heartbeat task enqueued",
		Data: map[string]string{
			"taskId":          taskID,
			"profile":         s.activeProfile.ID,
			"job":             strings.TrimSpace(job.Name),
			"goal":            truncateText(task.Goal, 200),
			"interval":        job.Interval.String(),
			"intervalSeconds": fmt.Sprintf("%d", int64(job.Interval/time.Second)),
		},
	})
	// Also emit the generic queued event so the monitor queue panel can display it.
}

func (s *Session) drainInbox(ctx context.Context) (bool, error) {
	// Execute pending tasks from SQLite only (DB-routed protocol v2).
	var errs error
	hadWork := false
	pending, err := s.listPendingTasks(ctx)
	if err != nil {
		errs = errors.Join(errs, err)
		return hadWork, errs
	}
	if len(pending) != 0 {
		hadWork = true
	}

	for _, task := range pending {
		taskID := strings.TrimSpace(task.TaskID)
		if taskID == "" {
			continue
		}

		if err := s.cfg.TaskStore.ClaimTask(ctx, taskID, s.cfg.LeaseTTL); err != nil {
			switch {
			case errors.Is(err, state.ErrTaskClaimed), errors.Is(err, state.ErrTaskTerminal), errors.Is(err, state.ErrTaskNotFound), isSQLiteBusyErr(err):
				continue
			default:
				errs = errors.Join(errs, fmt.Errorf("claim %s: %w", taskID, err))
				continue
			}
		}

		claimed, err := s.cfg.TaskStore.GetTask(ctx, taskID)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("get claimed %s: %w", taskID, err))
			continue
		}
		claimed.ClaimedByAgentID = strings.TrimSpace(s.cfg.RunID)
		if strings.TrimSpace(s.cfg.TeamID) != "" {
			claimed.RoleSnapshot = strings.TrimSpace(s.cfg.RoleName)
		}
		_ = s.cfg.TaskStore.UpdateTask(ctx, claimed)

		if claimed.Attempts > s.cfg.MaxRetries {
			if err := s.quarantineTask(ctx, claimed); err != nil {
				errs = errors.Join(errs, err)
			}
			continue
		}

		if err := s.runTask(ctx, taskID, claimed); err != nil {
			errs = errors.Join(errs, err)
		}
	}

	return hadWork, errs
}

func getTaskSource(t types.Task) string {
	if t.Metadata == nil {
		return ""
	}
	if raw, ok := t.Metadata["source"]; ok {
		return strings.TrimSpace(fmt.Sprint(raw))
	}
	return ""
}

func (s *Session) listPendingTasks(ctx context.Context) ([]types.Task, error) {
	if strings.TrimSpace(s.cfg.TeamID) == "" {
		tasks, err := s.cfg.TaskStore.ListTasks(ctx, state.TaskFilter{
			RunID:          s.cfg.RunID,
			AssignedToType: "agent",
			AssignedTo:     s.cfg.RunID,
			Status:         []types.TaskStatus{types.TaskStatusPending},
			SortBy:         "priority",
			Limit:          s.cfg.MaxPending,
		})
		if err != nil {
			return nil, err
		}
		// Prioritize subagent callbacks so the parent processes them before the overarching task.
		sort.SliceStable(tasks, func(i, j int) bool {
			si := getTaskSource(tasks[i]) == "subagent.callback"
			sj := getTaskSource(tasks[j]) == "subagent.callback"
			if si && !sj {
				return true
			}
			if !si && sj {
				return false
			}
			if si && sj {
				return tasks[i].SortTime().Before(tasks[j].SortTime())
			}
			// Both non-callback: keep priority then sort time
			if tasks[i].Priority != tasks[j].Priority {
				return tasks[i].Priority < tasks[j].Priority
			}
			return tasks[i].SortTime().Before(tasks[j].SortTime())
		})
		return tasks, nil
	}

	limit := s.cfg.MaxPending
	if limit <= 0 {
		limit = 50
	}
	tasks, err := s.cfg.TaskStore.ListTasks(ctx, state.TaskFilter{
		TeamID:         s.cfg.TeamID,
		AssignedToType: "role",
		AssignedTo:     s.cfg.RoleName,
		Status:         []types.TaskStatus{types.TaskStatusPending},
		SortBy:         "priority",
		Limit:          limit,
	})
	if err != nil {
		return nil, err
	}
	if !s.cfg.IsCoordinator {
		return tasks, nil
	}
	unassigned, err := s.cfg.TaskStore.ListTasks(ctx, state.TaskFilter{
		TeamID:         s.cfg.TeamID,
		AssignedToType: "team",
		AssignedTo:     s.cfg.TeamID,
		Status:         []types.TaskStatus{types.TaskStatusPending},
		SortBy:         "priority",
		Limit:          limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]types.Task, 0, len(tasks)+len(unassigned))
	seen := map[string]struct{}{}
	for _, task := range tasks {
		if strings.TrimSpace(task.TaskID) == "" {
			continue
		}
		seen[task.TaskID] = struct{}{}
		out = append(out, task)
	}
	for _, task := range unassigned {
		if strings.TrimSpace(task.TaskID) == "" {
			continue
		}
		if _, ok := seen[task.TaskID]; ok {
			continue
		}
		seen[task.TaskID] = struct{}{}
		out = append(out, task)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		return out[i].SortTime().Before(out[j].SortTime())
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Session) runTask(ctx context.Context, taskID string, task types.Task) error {
	now := time.Now()
	task.Status = types.TaskStatusActive
	task.StartedAt = &now
	task.SessionID = s.cfg.SessionID
	task.RunID = s.cfg.RunID
	if strings.TrimSpace(s.cfg.TeamID) != "" {
		task.TeamID = s.cfg.TeamID
		if strings.TrimSpace(task.AssignedRole) == "" {
			task.AssignedRole = s.cfg.RoleName
		}
		task.AssignedToType = "role"
		task.AssignedTo = task.AssignedRole
		task.RoleSnapshot = strings.TrimSpace(s.cfg.RoleName)
		if strings.TrimSpace(task.CreatedBy) == "" {
			task.CreatedBy = s.cfg.RoleName
		}
	} else {
		task.AssignedToType = "agent"
		task.AssignedTo = strings.TrimSpace(s.cfg.RunID)
	}
	task.ClaimedByAgentID = strings.TrimSpace(s.cfg.RunID)
	_ = s.cfg.TaskStore.UpdateTask(ctx, task)

	taskKind := strings.TrimSpace(task.TaskKind)
	if taskKind == "" {
		switch {
		case strings.HasPrefix(strings.TrimSpace(task.TaskID), "heartbeat-"):
			taskKind = state.TaskKindHeartbeat
		case strings.HasPrefix(strings.TrimSpace(task.TaskID), "callback-"):
			taskKind = state.TaskKindCallback
		case strings.HasPrefix(strings.TrimSpace(task.TaskID), "task-"):
			taskKind = state.TaskKindTask
		}
	}
	taskSource := ""
	taskHeartbeatJob := ""
	taskHeartbeatInterval := ""
	if task.Metadata != nil {
		if raw, ok := task.Metadata["source"]; ok {
			taskSource = strings.TrimSpace(fmt.Sprint(raw))
		}
		if raw, ok := task.Metadata["job"]; ok {
			taskHeartbeatJob = strings.TrimSpace(fmt.Sprint(raw))
		}
		if raw, ok := task.Metadata["interval"]; ok {
			taskHeartbeatInterval = strings.TrimSpace(fmt.Sprint(raw))
		}
	}
	if taskSource == "" && strings.EqualFold(taskKind, state.TaskKindHeartbeat) {
		taskSource = "heartbeat"
	}
	if s.cfg.Events != nil {
		data := map[string]string{
			"taskId":   taskID,
			"profile":  s.activeProfile.ID,
			"goal":     truncateText(task.Goal, 100),
			"taskKind": taskKind,
		}
		if taskSource != "" {
			data["source"] = taskSource
		}
		if taskHeartbeatJob != "" {
			data["job"] = taskHeartbeatJob
		}
		if taskHeartbeatInterval != "" {
			data["interval"] = taskHeartbeatInterval
		}
		s.emitBestEffort(ctx, events.Event{
			Type:    "task.start",
			Message: "Task started",
			Data:    data,
		})
	}

	leaseCtx, leaseStop := context.WithCancel(ctx)
	defer leaseStop()
	go func() {
		t := time.NewTicker(s.cfg.LeaseExtend)
		defer t.Stop()
		for {
			select {
			case <-leaseCtx.Done():
				return
			case <-t.C:
				_ = s.cfg.TaskStore.ExtendLease(leaseCtx, taskID, s.cfg.LeaseTTL)
			}
		}
	}()

	runAgent := s.cfg.Agent
	if s.cfg.Memory != nil || s.activeProfile != nil {
		memSnips := []agent.MemorySnippet{}
		if s.cfg.Memory != nil {
			if ms, err := s.cfg.Memory.Search(ctx, strings.TrimSpace(task.Goal), s.cfg.MemorySearchLimit); err == nil {
				memSnips = ms
			}
		}
		aug := buildSystemPrompt(
			runAgent.GetSystemPrompt(),
			*s.activeProfile,
			s.activePromptText,
			memSnips,
			s.lastTaskOutcome,
			s.cfg.TeamID,
			s.cfg.RoleName,
			s.cfg.CoordinatorRole,
			s.cfg.TeamRoles,
			s.cfg.TeamRoleDescriptions,
		)
		if strings.TrimSpace(aug) != "" {
			cfg := runAgent.Config()
			cfg.SystemPrompt = aug
			if cloned, err := runAgent.CloneWithConfig(cfg); err == nil && cloned != nil {
				runAgent = cloned
			}
		}
	}

	var cumulativeInputTokens int
	var cumulativeOutputTokens int
	var costInPerM float64
	var costOutPerM float64
	var pricingKnown bool
	{
		modelID := strings.TrimSpace(runAgent.GetModel())
		if in, out, ok := cost.DefaultPricing().Lookup(modelID); ok {
			costInPerM = in
			costOutPerM = out
			pricingKnown = true
		}
		cfg := runAgent.Config()
		orig := cfg.Hooks.OnLLMUsage
		cfg.Hooks.OnLLMUsage = func(step int, usage llmtypes.LLMUsage) {
			if orig != nil {
				orig(step, usage)
			}
			cumulativeInputTokens += usage.InputTokens
			cumulativeOutputTokens += usage.OutputTokens
			if s.cfg.Events != nil {
				s.emitBestEffort(ctx, events.Event{
					Type:    "task.usage",
					Message: "Task LLM usage",
					Data: map[string]string{
						"taskId":       taskID,
						"step":         fmt.Sprintf("%d", step),
						"inputTokens":  fmt.Sprintf("%d", usage.InputTokens),
						"outputTokens": fmt.Sprintf("%d", usage.OutputTokens),
					},
				})
			}
		}
		cfg.PromptSource = agent.NewSteeringPromptSource(cfg.PromptSource)
		if cloned, err := runAgent.CloneWithConfig(cfg); err == nil && cloned != nil {
			runAgent = cloned
		}
	}

	runRes, err := runAgent.Run(ctx, strings.TrimSpace(task.Goal))
	doneAt := time.Now()
	totalTokens := cumulativeInputTokens + cumulativeOutputTokens
	costUSD := 0.0
	if pricingKnown {
		costUSD = (float64(cumulativeInputTokens)/1_000_000.0)*costInPerM + (float64(cumulativeOutputTokens)/1_000_000.0)*costOutPerM
	}
	tr := types.TaskResult{
		TaskID:       taskID,
		Status:       types.TaskStatusSucceeded,
		Summary:      strings.TrimSpace(runRes.Text),
		CompletedAt:  &doneAt,
		InputTokens:  cumulativeInputTokens,
		OutputTokens: cumulativeOutputTokens,
		TotalTokens:  totalTokens,
		CostUSD:      costUSD,
	}
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			tr.Status = types.TaskStatusCanceled
			tr.Error = "stopped by user"
		} else {
			tr.Status = types.TaskStatusFailed
			tr.Error = err.Error()
			if s.cfg.Events != nil {
				errInfo := llm.ClassifyError(err)
				payload := map[string]string{
					"taskId":     taskID,
					"class":      fallback(strings.TrimSpace(errInfo.Class), "unknown"),
					"retryable":  fmt.Sprintf("%t", errInfo.Retryable),
					"statusCode": fmt.Sprintf("%d", errInfo.StatusCode),
					"message":    fallback(strings.TrimSpace(errInfo.Message), strings.TrimSpace(err.Error())),
				}
				if code := strings.TrimSpace(errInfo.Code); code != "" {
					payload["code"] = code
				}
				s.emitBestEffort(ctx, events.Event{
					Type:    "llm.error",
					Message: llmErrorMessage(errInfo),
					Data:    payload,
				})
			}
		}
	} else {
		resStatus := strings.ToLower(strings.TrimSpace(string(runRes.Status)))
		if resStatus == "" {
			resStatus = string(types.TaskStatusSucceeded)
		}
		switch resStatus {
		case string(types.TaskStatusSucceeded):
			// ok
		case string(types.TaskStatusFailed):
			tr.Status = types.TaskStatusFailed
			tr.Error = strings.TrimSpace(runRes.Error)
			if tr.Error == "" {
				tr.Error = "task failed"
			}
		case string(types.TaskStatusCanceled):
			tr.Status = types.TaskStatusCanceled
			tr.Error = strings.TrimSpace(runRes.Error)
		default:
			// Unknown status: treat as a failure so tasks don't appear "green" accidentally.
			tr.Status = types.TaskStatusFailed
			tr.Error = fmt.Sprintf("invalid agent status %q", resStatus)
		}
	}

	// Preserve a tiny, immediate continuity signal for Task N+1 without relying on async memory indexing.
	// Stored value is strictly truncated and later re-sanitized at prompt construction.
	{
		outcome := strings.TrimSpace(tr.Summary)
		if outcome == "" {
			outcome = strings.TrimSpace(tr.Error)
		} else if tr.Status == types.TaskStatusFailed && strings.TrimSpace(tr.Error) != "" {
			outcome = strings.TrimSpace(outcome) + " | " + strings.TrimSpace(tr.Error)
		}
		s.lastTaskOutcome = truncateText(outcome, 150)
	}

	base := tasksBase(strings.TrimSpace(s.cfg.TeamID), strings.TrimSpace(s.cfg.RoleName), doneAt, taskID)
	artifacts := sanitizeArtifactPaths(dedupeArtifactPaths(runRes.Artifacts))
	if summaryPath := s.writeTaskSummary(ctx, base, taskID, task.Goal, tr, artifacts); summaryPath != "" {
		tr.Artifacts = append([]string{summaryPath}, artifacts...)
	} else {
		tr.Artifacts = artifacts
	}

	// Emit completion events before updating task state (ordering).
	if s.cfg.Events != nil {
		data := map[string]string{
			"taskId":   taskID,
			"status":   string(tr.Status),
			"profile":  s.activeProfile.ID,
			"taskKind": taskKind,
		}
		if taskSource != "" {
			data["source"] = taskSource
		}
		if taskHeartbeatJob != "" {
			data["job"] = taskHeartbeatJob
		}
		if taskHeartbeatInterval != "" {
			data["interval"] = taskHeartbeatInterval
		}
		if g := truncateText(task.Goal, 100); g != "" {
			data["goal"] = g
		}
		if sum := strings.TrimSpace(tr.Summary); sum != "" {
			data["summary"] = sum
		}
		if tr.Error != "" {
			data["error"] = tr.Error
		}
		if tr.TotalTokens > 0 {
			data["inputTokens"] = fmt.Sprintf("%d", tr.InputTokens)
			data["outputTokens"] = fmt.Sprintf("%d", tr.OutputTokens)
			data["totalTokens"] = fmt.Sprintf("%d", tr.TotalTokens)
		}
		if tr.CostUSD > 0 {
			data["costUSD"] = fmt.Sprintf("%.4f", tr.CostUSD)
		}
		if len(tr.Artifacts) != 0 {
			data["artifacts"] = fmt.Sprintf("%d", len(tr.Artifacts))
			// Always include the first artifact path (typically SUMMARY.md).
			data["artifact0"] = tr.Artifacts[0]
		}
		s.emitBestEffort(ctx, events.Event{Type: "task.done", Message: "Task finished", Data: data})
		if strings.EqualFold(taskSource, "heartbeat") {
			s.emitBestEffort(ctx, events.Event{
				Type:    "task.heartbeat.done",
				Message: "Heartbeat task finished",
				Data:    data,
			})
		}
		if len(tr.Artifacts) != 0 {
			s.emitBestEffort(ctx, events.Event{Type: "task.delivered", Message: "Task outputs recorded", Data: map[string]string{"taskId": taskID, "count": fmt.Sprintf("%d", len(tr.Artifacts)), "summaryPath": tr.Artifacts[0]}})
		}
	}

	completeCtx := ctx
	if tr.Status == types.TaskStatusCanceled && errors.Is(ctx.Err(), context.Canceled) {
		completeCtx = context.Background()
	}
	if err := s.cfg.TaskStore.CompleteTask(completeCtx, taskID, tr); err != nil {
		return err
	}
	s.maybeEmitCoordinatorPolicyWarn(ctx, task, tr)
	s.maybeCreateCoordinatorCallback(ctx, task, tr)

	task.Status = tr.Status
	task.CompletedAt = tr.CompletedAt
	task.Error = tr.Error
	task.Summary = tr.Summary
	task.Artifacts = append([]string(nil), tr.Artifacts...)
	task.InputTokens = tr.InputTokens
	task.OutputTokens = tr.OutputTokens
	task.TotalTokens = tr.TotalTokens
	task.CostUSD = tr.CostUSD
	if timeutil.IsSet(task.StartedAt) && timeutil.IsSet(tr.CompletedAt) {
		task.DurationSeconds = int(tr.CompletedAt.Sub(*task.StartedAt).Round(time.Second).Seconds())
	}
	_ = s.cfg.TaskStore.UpdateTask(completeCtx, task)

	if s.cfg.Notifier != nil {
		if err := s.cfg.Notifier.Notify(completeCtx, task, tr); err != nil {
			s.emitBestEffort(completeCtx, events.Event{Type: "task.notify.error", Message: "Task notification failed", Data: map[string]string{"taskId": taskID, "error": err.Error()}})
		}
	}
	if s.cfg.SingleTask {
		// Sub-agents spawned via spawn_worker or retry should not exit immediately;
		// they enter a review-wait state so the parent can approve or retry.
		if taskSource := ""; task.Metadata != nil {
			taskSource, _ = task.Metadata["source"].(string)
			if taskSource == "spawn_worker" || taskSource == "retry" {
				s.emitBestEffort(ctx, events.Event{
					Type:    "subagent.awaiting_review",
					Message: "Sub-agent completed; awaiting parent review",
					Data:    map[string]string{"runId": s.cfg.RunID, "taskId": taskID},
				})
				return nil // continue polling loop; parent may approve (cleanup) or retry (new task)
			}
		}
		return ErrSingleTaskComplete
	}
	return nil
}

func (s *Session) emitTaskQueuedOnce(ctx context.Context, taskID, goal, source string) {
	if s == nil || s.cfg.Events == nil {
		return
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return
	}
	if s.queuedEmitted == nil {
		s.queuedEmitted = make(map[string]time.Time)
	}
	if _, ok := s.queuedEmitted[taskID]; ok {
		return
	}
	now := time.Now().UTC()
	// Prevent unbounded growth for long-lived daemons with age-based eviction.
	if len(s.queuedEmitted) > 5000 {
		cutoff := now.Add(-20 * time.Minute)
		for k, ts := range s.queuedEmitted {
			if ts.Before(cutoff) {
				delete(s.queuedEmitted, k)
			}
		}
		// Fallback: if still too large, drop oldest entries.
		if len(s.queuedEmitted) > 5000 {
			type kv struct {
				key string
				at  time.Time
			}
			all := make([]kv, 0, len(s.queuedEmitted))
			for k, ts := range s.queuedEmitted {
				all = append(all, kv{key: k, at: ts})
			}
			sort.Slice(all, func(i, j int) bool { return all[i].at.Before(all[j].at) })
			drop := len(all) - 4500
			for i := 0; i < drop && i < len(all); i++ {
				delete(s.queuedEmitted, all[i].key)
			}
		}
	}
	s.queuedEmitted[taskID] = now
	payload := map[string]string{"taskId": taskID, "profile": s.activeProfile.ID, "goal": truncateText(goal, 200)}
	if strings.TrimSpace(source) != "" {
		payload["source"] = strings.TrimSpace(source)
	}
	s.emitBestEffort(ctx, events.Event{Type: "task.queued", Message: "Task queued", Data: payload})
}

func (s *Session) maybeCreateCoordinatorCallback(ctx context.Context, task types.Task, tr types.TaskResult) {
	parentRunID := strings.TrimSpace(s.cfg.ParentRunID)
	isSubagentWorker := parentRunID != ""
	isTeamCallbackEligible := s.isTeamCallbackEligible(task)

	if !isTeamCallbackEligible && !isSubagentWorker {
		return
	}

	taskID := strings.TrimSpace(task.TaskID)
	if taskID == "" {
		return
	}

	callbackTaskID := "callback-" + taskID
	now := time.Now().UTC()

	var callback types.Task
	if isSubagentWorker {
		// Standalone subagent: create callback assigned to parent run.
		// task is the task the subagent just completed; taskID and task.Goal refer to that work.
		sourceGoal := strings.TrimSpace(task.Goal)
		if sourceGoal == "" {
			sourceGoal = "(no goal text)"
		}
		subagentRunID := strings.TrimSpace(s.cfg.RunID)
		subagentLabel := fsutil.GetSubagentLabel(s.cfg.SpawnIndex)
		subagentWorkspaceDir := path.Join("/workspace", subagentLabel)
		subagentSummariesDir := path.Join("/tasks", subagentLabel)
		artifactsForParent := make([]string, 0, len(tr.Artifacts))
		seenArtifacts := map[string]struct{}{}
		for _, art := range tr.Artifacts {
			normalized := normalizeStandaloneSubagentCallbackArtifactPath(s.cfg.SpawnIndex, art)
			normalized = strings.TrimSpace(normalized)
			if normalized == "" {
				continue
			}
			if _, ok := seenArtifacts[normalized]; ok {
				continue
			}
			seenArtifacts[normalized] = struct{}{}
			artifactsForParent = append(artifactsForParent, normalized)
		}
		callbackGoal := fmt.Sprintf("SUBAGENT RESULT: Review %s result from spawned worker for task %s. The worker completed: %s. Use the task_review tool to approve, retry (with feedback), or escalate this work. Your overarching task (that led to spawning this worker) is only complete after you have reviewed this result and decided on next steps.", string(tr.Status), truncateText(taskID, 24), truncateText(sourceGoal, 120))
		callbackGoal += "\n\nSubagent task summaries are under " + subagentSummariesDir + " (e.g. <date>/<taskID>/SUMMARY.md). Subagent outputs are under " + subagentWorkspaceDir + ". Open and review the artifact paths below (e.g. with fs_read) before calling task_review."
		if len(artifactsForParent) > 0 {
			const maxPathsInGoal = 10
			pathsToShow := artifactsForParent
			if len(pathsToShow) > maxPathsInGoal {
				pathsToShow = pathsToShow[:maxPathsInGoal]
			}
			callbackGoal += "\nArtifacts to review: " + strings.Join(pathsToShow, ", ")
			if len(artifactsForParent) > maxPathsInGoal {
				callbackGoal += fmt.Sprintf(" (and %d more)", len(artifactsForParent)-maxPathsInGoal)
			}
			callbackGoal += "."
		}
		inputs := map[string]any{
			"sourceTaskId":         taskID,
			"sourceGoal":           sourceGoal,
			"sourceRunId":          subagentRunID,
			"sourceSpawnIndex":     s.cfg.SpawnIndex,
			"sourceStatus":         string(tr.Status),
			"summary":              strings.TrimSpace(tr.Summary),
			"error":                strings.TrimSpace(tr.Error),
			"artifacts":            artifactsForParent,
			"subagentWorkspaceDir": subagentWorkspaceDir,
			"subagentSummariesDir": subagentSummariesDir,
		}
		callback = types.Task{
			TaskID:         callbackTaskID,
			SessionID:      strings.TrimSpace(s.cfg.SessionID),
			RunID:          parentRunID,
			AssignedToType: "agent",
			AssignedTo:     parentRunID,
			CreatedBy:      strings.TrimSpace(s.cfg.RunID),
			TaskKind:       state.TaskKindReview,
			Goal:           callbackGoal,
			Inputs:         inputs,
			Priority:       1,
			Status:         types.TaskStatusPending,
			CreatedAt:      &now,
			Metadata: map[string]any{
				"source":            "subagent.callback",
				"callbackForTaskId": taskID,
				"sourceRunId":       strings.TrimSpace(s.cfg.RunID),
				"sourceTaskStatus":  string(tr.Status),
				"reviewGate":        true,
				"reviewActions":     []string{"approve", "retry", "escalate"},
				"retryBudget":       float64(3),
				"retryCount":        float64(0),
			},
		}
	} else {
		// Team mode: create callback assigned to coordinator role.
		coordinatorRole := strings.TrimSpace(s.cfg.CoordinatorRole)
		artifactsForCoordinator := make([]string, 0, len(tr.Artifacts))
		seenArtifacts := map[string]struct{}{}
		for _, art := range tr.Artifacts {
			normalized := normalizeTeamCallbackArtifactPath(strings.TrimSpace(s.cfg.RoleName), s.cfg.TeamRoles, art)
			normalized = strings.TrimSpace(normalized)
			if normalized == "" {
				continue
			}
			if _, ok := seenArtifacts[normalized]; ok {
				continue
			}
			seenArtifacts[normalized] = struct{}{}
			artifactsForCoordinator = append(artifactsForCoordinator, normalized)
		}
		callbackGoal := fmt.Sprintf("COORDINATOR REVIEW ONLY: Review %s result from role %q for task %s. Do not do specialist work yourself. Either delegate any needed follow-up to the appropriate role or mark the work complete and update overall team progress.", string(tr.Status), strings.TrimSpace(s.cfg.RoleName), truncateText(taskID, 24))
		inputs := map[string]any{
			"sourceTaskId": taskID,
			"sourceRole":   strings.TrimSpace(s.cfg.RoleName),
			"sourceStatus": string(tr.Status),
			"summary":      strings.TrimSpace(tr.Summary),
			"error":        strings.TrimSpace(tr.Error),
			"artifacts":    artifactsForCoordinator,
		}
		callback = types.Task{
			TaskID:         callbackTaskID,
			SessionID:      "team-" + strings.TrimSpace(s.cfg.TeamID),
			RunID:          "team-" + strings.TrimSpace(s.cfg.TeamID) + "-callback",
			TeamID:         strings.TrimSpace(s.cfg.TeamID),
			AssignedRole:   coordinatorRole,
			AssignedToType: "role",
			AssignedTo:     coordinatorRole,
			CreatedBy:      strings.TrimSpace(s.cfg.RoleName),
			TaskKind:       state.TaskKindCallback,
			Goal:           callbackGoal,
			Inputs:         inputs,
			Priority:       1,
			Status:         types.TaskStatusPending,
			CreatedAt:      &now,
			Metadata: map[string]any{
				"source":            "team.callback",
				"callbackForTaskId": taskID,
				"sourceRole":        strings.TrimSpace(s.cfg.RoleName),
				"sourceRunID":       strings.TrimSpace(s.cfg.RunID),
				"sourceTaskStatus":  string(tr.Status),
			},
		}
	}

	if err := s.cfg.TaskStore.CreateTask(ctx, callback); err != nil {
		return // idempotent via deterministic callback task id.
	}

	eventSource := "team.callback"
	if isSubagentWorker {
		eventSource = "subagent.callback"
	}
	s.emitTaskQueuedOnce(ctx, callbackTaskID, callback.Goal, eventSource)
	s.emitBestEffort(ctx, events.Event{
		Type:    eventSource + ".queued",
		Message: "Worker completion callback queued",
		Data: map[string]string{
			"taskId":            callbackTaskID,
			"callbackForTaskId": taskID,
		},
	})
}

func (s *Session) isTeamCallbackEligible(task types.Task) bool {
	if strings.TrimSpace(s.cfg.TeamID) == "" {
		return false
	}
	coordinatorRole := strings.TrimSpace(s.cfg.CoordinatorRole)
	if coordinatorRole == "" {
		return false
	}
	return !isCoordinatorSelfTask(task, coordinatorRole, strings.TrimSpace(s.cfg.RoleName))
}

func isCoordinatorSelfTask(task types.Task, coordinatorRole, defaultRole string) bool {
	coordinatorRole = strings.TrimSpace(coordinatorRole)
	if coordinatorRole == "" {
		return false
	}

	assignedRole := strings.TrimSpace(task.AssignedRole)
	if assignedRole == "" {
		assignedRole = strings.TrimSpace(defaultRole)
	}
	if !strings.EqualFold(assignedRole, coordinatorRole) {
		return false
	}

	createdBy := strings.ToLower(strings.TrimSpace(task.CreatedBy))
	if createdBy == strings.ToLower(coordinatorRole) {
		return true
	}
	return createdBy == "user" || createdBy == "webhook" || createdBy == "monitor"
}

func (s *Session) maybeEmitCoordinatorPolicyWarn(ctx context.Context, task types.Task, tr types.TaskResult) {
	if strings.TrimSpace(s.cfg.TeamID) == "" || !s.cfg.IsCoordinator {
		return
	}
	createdBy := strings.ToLower(strings.TrimSpace(task.CreatedBy))
	if createdBy != "user" && createdBy != "webhook" && createdBy != "monitor" {
		return
	}
	if strings.TrimSpace(task.TaskID) == "" {
		return
	}
	// Soft guardrail: for user-originated coordinator tasks, emit warning when no delegation evidence exists.
	// Delegation evidence = created tasks for another role, or spawn_worker tasks (callbacks will follow).
	tasks, err := s.cfg.TaskStore.ListTasks(ctx, state.TaskFilter{
		TeamID:   s.cfg.TeamID,
		SortBy:   "created_at",
		SortDesc: true,
		Limit:    200,
	})
	if err != nil {
		return
	}
	start := time.Time{}
	if timeutil.IsSet(task.StartedAt) {
		start = task.StartedAt.UTC()
	}
	end := time.Now().UTC()
	if timeutil.IsSet(tr.CompletedAt) {
		end = tr.CompletedAt.UTC()
	}
	delegated := false
	for _, candidate := range tasks {
		if strings.TrimSpace(candidate.TaskID) == "" || strings.EqualFold(candidate.TaskID, task.TaskID) {
			continue
		}
		if strings.TrimSpace(candidate.TeamID) != s.cfg.TeamID {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(candidate.CreatedBy), s.cfg.RoleName) {
			continue
		}
		if !timeutil.IsSet(candidate.CreatedAt) {
			continue
		}
		createdAt := candidate.CreatedAt.UTC()
		if !start.IsZero() && createdAt.Before(start.Add(-2*time.Second)) {
			continue
		}
		if createdAt.After(end.Add(2 * time.Second)) {
			continue
		}
		// Count as delegation: assigned to another role, or spawn_worker (callbacks created when workers finish).
		if getTaskSource(candidate) == "spawn_worker" {
			delegated = true
			break
		}
		if !strings.EqualFold(strings.TrimSpace(candidate.AssignedRole), s.cfg.RoleName) {
			delegated = true
			break
		}
	}
	if delegated {
		return
	}
	s.emitBestEffort(ctx, events.Event{
		Type:    "team.coordinator.policy.warn",
		Message: "Coordinator completed a user task without delegating to another role or spawning workers",
		Data: map[string]string{
			"taskId": task.TaskID,
			"role":   s.cfg.RoleName,
			"status": string(tr.Status),
		},
	})
}

func (s *Session) quarantineTask(ctx context.Context, task types.Task) error {
	taskID := strings.TrimSpace(task.TaskID)
	if taskID == "" {
		return fmt.Errorf("taskID is required")
	}
	poisonPath := path.Join("/workspace", "quarantine", time.Now().UTC().Format("20060102T150405Z")+"-"+taskID+".json")
	{
		task.Error = fallback(strings.TrimSpace(task.Error), "max retries exceeded")
		b, _ := json.MarshalIndent(task, "", "  ")
		if strings.TrimSpace(string(b)) != "" {
			_ = s.cfg.Agent.ExecHostOp(ctx, types.HostOpRequest{Op: types.HostOpFSWrite, Path: poisonPath, Text: string(b)})
		}
	}

	doneAt := time.Now()
	tr := types.TaskResult{
		TaskID:      taskID,
		Status:      types.TaskStatusFailed,
		Error:       "quarantined: max retries exceeded",
		CompletedAt: &doneAt,
	}
	_ = s.cfg.TaskStore.CompleteTask(ctx, taskID, tr)

	s.emitBestEffort(ctx, events.Event{
		Type:    "task.quarantined",
		Message: "Task quarantined",
		Data:    map[string]string{"taskId": taskID, "poisonPath": poisonPath, "error": fallback(strings.TrimSpace(task.Error), "max retries exceeded")},
	})

	// When a spawn_worker or retry task is quarantined, emit an auto-escalation
	// event so the TUI/user can be notified of the intervention needed.
	if task.Metadata != nil {
		taskSource, _ := task.Metadata["source"].(string)
		if taskSource == "spawn_worker" || taskSource == "retry" {
			s.emitBestEffort(ctx, events.Event{
				Type:    "task.escalation.auto",
				Message: "Sub-agent task quarantined; escalation required",
				Data: map[string]string{
					"taskId":   taskID,
					"runId":    s.cfg.RunID,
					"parentId": s.cfg.ParentRunID,
					"goal":     truncateText(task.Goal, 200),
					"error":    fallback(strings.TrimSpace(task.Error), "max retries exceeded"),
					"source":   taskSource,
				},
			})
		}
	}
	return nil
}

func dedupeArtifactPaths(artifacts []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(artifacts))
	for _, p := range artifacts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func sanitizeArtifactPaths(artifacts []string) []string {
	out := make([]string, 0, len(artifacts))
	for _, p := range artifacts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// Planning files are internal working state and should never be surfaced as deliverables.
		if strings.HasPrefix(p, "/plan/") || strings.HasPrefix(p, "/workspace/plan/") {
			continue
		}
		out = append(out, p)
	}
	return out
}

func normalizeTeamCallbackArtifactPath(role string, teamRoles []string, artifactPath string) string {
	roleSeg := sanitizeRoleForWorkspacePath(role)
	artifactPath = strings.TrimSpace(artifactPath)
	if artifactPath == "" {
		return ""
	}
	knownRoles := map[string]struct{}{
		roleSeg: {},
	}
	for _, teamRole := range teamRoles {
		seg := sanitizeRoleForWorkspacePath(teamRole)
		if seg == "" {
			continue
		}
		knownRoles[seg] = struct{}{}
	}
	switch {
	case strings.HasPrefix(artifactPath, "/workspace/"):
		rel := strings.TrimPrefix(artifactPath, "/workspace/")
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" {
			return path.Join("/workspace", roleSeg)
		}
		first := rel
		if cut, _, ok := strings.Cut(rel, "/"); ok {
			first = cut
		}
		if _, ok := knownRoles[first]; ok {
			return path.Join("/workspace", rel)
		}
		return path.Join("/workspace", roleSeg, rel)
	case strings.HasPrefix(artifactPath, "/tasks/"):
		rel := strings.TrimPrefix(artifactPath, "/tasks/")
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" {
			return path.Join("/tasks", roleSeg)
		}
		first := rel
		if cut, _, ok := strings.Cut(rel, "/"); ok {
			first = cut
		}
		if _, ok := knownRoles[first]; ok {
			return path.Join("/tasks", rel)
		}
		return path.Join("/tasks", roleSeg, rel)
	case strings.HasPrefix(artifactPath, "/deliverables/"):
		return artifactPath
	default:
		return artifactPath
	}
}

func normalizeStandaloneSubagentCallbackArtifactPath(spawnIndex int, artifactPath string) string {
	label := fsutil.GetSubagentLabel(spawnIndex)
	artifactPath = strings.TrimSpace(artifactPath)
	if artifactPath == "" {
		return ""
	}

	switch {
	case strings.HasPrefix(artifactPath, "/workspace/"):
		rel := strings.TrimPrefix(artifactPath, "/workspace/")
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" {
			return path.Join("/workspace", label)
		}
		first := rel
		if cut, _, ok := strings.Cut(rel, "/"); ok {
			first = cut
		}
		if first == label {
			return path.Join("/workspace", rel)
		}
		return path.Join("/workspace", label, rel)
	case strings.HasPrefix(artifactPath, "/tasks/"):
		rel := strings.TrimPrefix(artifactPath, "/tasks/")
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" {
			return path.Join("/tasks", label)
		}
		first := rel
		if cut, _, ok := strings.Cut(rel, "/"); ok {
			first = cut
		}
		if first == label {
			return path.Join("/tasks", rel)
		}
		return path.Join("/tasks", label, rel)
	default:
		return artifactPath
	}
}

func sanitizeRoleForWorkspacePath(role string) string {
	role = strings.TrimSpace(role)
	if role == "" {
		return "default"
	}
	var b strings.Builder
	for _, r := range role {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	s := b.String()
	if s == "" {
		return "default"
	}
	return s
}

func tasksBase(teamID, role string, when time.Time, taskID string) string {
	date := when.UTC().Format("2006-01-02")
	if strings.TrimSpace(teamID) != "" {
		return path.Join("/tasks", sanitizeRoleForWorkspacePath(role), date, taskID)
	}
	return path.Join("/tasks", date, taskID)
}

func (s *Session) writeTaskSummary(ctx context.Context, base, taskID, goal string, tr types.TaskResult, artifacts []string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		return ""
	}
	p := path.Join(base, "SUMMARY.md")
	var b strings.Builder
	b.WriteString("# Task Summary\n\n")
	b.WriteString("- TaskID: `")
	b.WriteString(taskID)
	b.WriteString("`\n")
	if strings.TrimSpace(goal) != "" {
		b.WriteString("- Goal: ")
		b.WriteString(strings.TrimSpace(goal))
		b.WriteString("\n")
	}
	b.WriteString("- Status: `")
	b.WriteString(string(tr.Status))
	b.WriteString("`\n")
	if timeutil.IsSet(tr.CompletedAt) {
		b.WriteString("- CompletedAt: `")
		b.WriteString(tr.CompletedAt.UTC().Format(time.RFC3339Nano))
		b.WriteString("`\n")
	}
	if strings.TrimSpace(s.activeProfile.ID) != "" {
		b.WriteString("- Profile: `")
		b.WriteString(s.activeProfile.ID)
		b.WriteString("`\n")
	}
	if strings.TrimSpace(tr.Error) != "" {
		b.WriteString("\n## Error\n\n")
		b.WriteString("```\n")
		b.WriteString(strings.TrimSpace(tr.Error))
		b.WriteString("\n```\n")
	}

	if tr.TotalTokens > 0 || tr.CostUSD > 0 {
		b.WriteString("\n## Usage\n\n")
		if tr.TotalTokens > 0 {
			b.WriteString(fmt.Sprintf("- Tokens: `%d` total (`%d` input + `%d` output)\n", tr.TotalTokens, tr.InputTokens, tr.OutputTokens))
		}
		if tr.CostUSD > 0 {
			b.WriteString(fmt.Sprintf("- Estimated cost: `$%.4f` USD\n", tr.CostUSD))
		}
	}

	b.WriteString("\n## Summary\n\n")
	if strings.TrimSpace(tr.Summary) == "" {
		b.WriteString("_No summary returned._\n")
	} else {
		b.WriteString(strings.TrimSpace(tr.Summary))
		b.WriteString("\n")
	}

	b.WriteString("\n## Deliverables\n\n")
	if len(artifacts) == 0 {
		b.WriteString("_No deliverables were recorded._\n")
	} else {
		for _, a := range artifacts {
			a = strings.TrimSpace(a)
			if a == "" {
				continue
			}
			b.WriteString("- `")
			b.WriteString(a)
			b.WriteString("`\n")
		}
	}

	resp := s.cfg.Agent.ExecHostOp(ctx, types.HostOpRequest{Op: types.HostOpFSWrite, Path: p, Text: b.String()})
	if !resp.Ok {
		return ""
	}
	return p
}

func (s *Session) readText(ctx context.Context, p string) (string, bool) {
	resp := s.cfg.Agent.ExecHostOp(ctx, types.HostOpRequest{Op: types.HostOpFSRead, Path: p, MaxBytes: s.cfg.MaxReadBytes})
	if !resp.Ok {
		return "", false
	}
	return resp.Text, true
}

func (s *Session) writeJSON(ctx context.Context, p string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	resp := s.cfg.Agent.ExecHostOp(ctx, types.HostOpRequest{Op: types.HostOpFSWrite, Path: p, Text: string(b)})
	if !resp.Ok {
		return fmt.Errorf("write %s: %s", p, resp.Error)
	}
	return nil
}

func normalizeListEntries(inboxPath string, entries []string) []string {
	out := make([]string, 0, len(entries))
	for _, p := range entries {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.HasPrefix(p, inboxPath) {
			out = append(out, p)
			continue
		}
		if strings.HasPrefix(p, "/") {
			out = append(out, p)
			continue
		}
		out = append(out, path.Join(inboxPath, p))
	}
	return out
}

func truncateText(s string, max int) string {
	if max <= 0 {
		return ""
	}
	s = strings.TrimSpace(s)
	if s == "" || len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

func fallback(v string, def string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return def
	}
	return v
}

func llmErrorMessage(info llm.ErrorInfo) string {
	switch strings.TrimSpace(info.Class) {
	case "quota":
		return "LLM quota/credits exhausted"
	case "rate_limit":
		return "LLM rate limit reached"
	case "network":
		return "LLM network error"
	case "timeout":
		return "LLM request timed out"
	case "auth":
		return "LLM authentication failed"
	case "permission":
		return "LLM permission denied"
	case "policy":
		return "LLM request blocked by provider data policy (check OpenRouter privacy settings for free models)"
	case "server":
		return "LLM provider server error"
	case "invalid_request":
		return "LLM request rejected"
	default:
		return "LLM request failed"
	}
}

func isSQLiteBusyErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "sqlite_busy") || strings.Contains(msg, "database is locked")
}
