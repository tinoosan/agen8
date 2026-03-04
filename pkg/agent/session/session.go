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

	"github.com/tinoosan/agen8/pkg/agent"
	hosttools "github.com/tinoosan/agen8/pkg/agent/hosttools"
	"github.com/tinoosan/agen8/pkg/agent/state"
	"github.com/tinoosan/agen8/pkg/cost"
	"github.com/tinoosan/agen8/pkg/emit"
	"github.com/tinoosan/agen8/pkg/events"
	"github.com/tinoosan/agen8/pkg/fsutil"
	"github.com/tinoosan/agen8/pkg/llm"
	llmtypes "github.com/tinoosan/agen8/pkg/llm/types"
	"github.com/tinoosan/agen8/pkg/profile"
	"github.com/tinoosan/agen8/pkg/store"
	"github.com/tinoosan/agen8/pkg/timeutil"
	"github.com/tinoosan/agen8/pkg/types"
)

// ErrSingleTaskComplete is returned by Run when SingleTask mode is enabled
// and the session has completed its assigned task.
var ErrSingleTaskComplete = errors.New("single task completed")

const (
	taskMetaSource                  = "source"
	taskSourceHeartbeat             = "heartbeat"
	taskSourceSpawnWorker           = "spawn_worker"
	taskSourceRetry                 = "retry"
	taskSourceReviewHandoff         = "review.handoff"
	taskSourceSubagentCallback      = "subagent.callback"
	taskSourceTeamCallback          = "team.callback"
	taskSourceSubagentBatchCallback = "subagent.batch.callback"
	taskSourceTeamBatchCallback     = "team.batch.callback"
	reviewDecisionApprove           = "approve"
	reviewDecisionRetry             = "retry"
	reviewDecisionEscalate          = "escalate"
)

type ResolveProfileFunc func(ref string) (*profile.Profile, string, error)

type MessageBus interface {
	state.MessageStore
}

type Config struct {
	Agent agent.Agent

	Profile        *profile.Profile
	ProfileDir     string
	ResolveProfile ResolveProfileFunc

	TaskStore  state.TaskStore
	MessageBus MessageBus
	Events     emit.Emitter[events.Event]

	RunConversationStore store.RunConversationStore

	Memory            agent.MemoryRecallProvider
	MemorySearchLimit int
	Notifier          agent.Notifier

	SoulContent string
	SoulVersion int

	PollInterval time.Duration
	// WakeCh, when signaled, nudges the session loop to drain the inbox immediately.
	// This is used by the app server to provide low-latency turn.create.
	WakeCh        <-chan struct{}
	RequireWakeCh bool
	MaxReadBytes  int

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
	ReviewerRole         string
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
	if cfg.MessageBus == nil {
		if mb, ok := cfg.TaskStore.(MessageBus); ok {
			cfg.MessageBus = mb
		}
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
	if cfg.RequireWakeCh && cfg.WakeCh == nil {
		return nil, fmt.Errorf("wake channel is required when RequireWakeCh is enabled")
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
		cfg.ReviewerRole = strings.TrimSpace(cfg.ReviewerRole)
		if cfg.RoleName == "" {
			return nil, fmt.Errorf("roleName is required in team mode")
		}
		if cfg.CoordinatorRole == "" {
			cfg.CoordinatorRole = cfg.RoleName
		}
		if cfg.ReviewerRole == "" {
			cfg.ReviewerRole = cfg.CoordinatorRole
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

	tick := func() error {
		hadCandidates, err := s.drainInbox(ctx)
		if err != nil {
			s.logf("inbox drain error: %v", err)
			s.emitBestEffort(ctx, events.Event{Type: "daemon.error", Message: "Inbox drain error", Data: map[string]string{"error": err.Error()}})
		}
		_ = hadCandidates
		return err
	}

	if !s.IsPaused() {
		_ = tick()
	}

	if s.cfg.RequireWakeCh {
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
					continue
				}
				_ = tick()
			}
		}
	}

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

	resetTimer := func(next time.Duration) {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(next)
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
				resetTimer(poll)
				continue
			}
			hadCandidates, err := s.drainInbox(ctx)
			if err != nil {
				s.logf("inbox drain error: %v", err)
				s.emitBestEffort(ctx, events.Event{Type: "daemon.error", Message: "Inbox drain error", Data: map[string]string{"error": err.Error()}})
			}
			if hadCandidates {
				poll = basePoll
			}
			resetTimer(poll)
		case <-timer.C:
			if s.IsPaused() {
				poll = basePoll
				resetTimer(poll)
				continue
			}
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
			resetTimer(poll)
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
	for _, job := range s.activeProfile.EffectiveHeartbeats() {
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
		isChildRun := strings.TrimSpace(s.cfg.ParentRunID) != ""
		if isChildRun {
			filter = state.TaskFilter{
				TeamID:         s.cfg.TeamID,
				RunID:          s.cfg.RunID,
				AssignedToType: "agent",
				AssignedTo:     s.cfg.RunID,
				Status:         []types.TaskStatus{types.TaskStatusPending},
			}
		} else {
			filter = state.TaskFilter{
				TeamID:         s.cfg.TeamID,
				AssignedToType: "role",
				AssignedTo:     s.cfg.RoleName,
				Status:         []types.TaskStatus{types.TaskStatusPending},
			}
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
			taskMetaSource:    taskSourceHeartbeat,
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
	s.emitTaskQueuedOnce(ctx, taskID, task.Goal, taskSourceHeartbeat)
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
	if s == nil || s.cfg.MessageBus == nil {
		return false, fmt.Errorf("message bus not configured")
	}
	return s.drainInboxMessages(ctx)
}

func (s *Session) drainInboxMessages(ctx context.Context) (bool, error) {
	var errs error
	hadWork := false
	s.maybeFlushStagedBatchCallbacks(ctx, true)
	if err := s.cfg.MessageBus.RequeueExpiredClaims(ctx); err != nil {
		errs = errors.Join(errs, fmt.Errorf("message requeue expired claims: %w", err))
	}
	limit := s.cfg.MaxPending
	if limit <= 0 {
		limit = 50
	}
	claimFilters := s.buildMessageClaimFilters()
	for i := 0; i < limit; i++ {
		msg, ok, err := s.claimNextScopedMessage(ctx, claimFilters)
		if err != nil {
			errs = errors.Join(errs, err)
			break
		}
		if !ok {
			break
		}
		hadWork = true
		if err := s.processClaimedMessage(ctx, msg); err != nil {
			errs = errors.Join(errs, err)
		}
	}
	return hadWork, errs
}

func (s *Session) buildMessageClaimFilters() []state.MessageClaimFilter {
	base := state.MessageClaimFilter{
		ThreadID: strings.TrimSpace(s.cfg.SessionID),
		TeamID:   strings.TrimSpace(s.cfg.TeamID),
		Channel:  types.MessageChannelInbox,
		Kinds:    []string{types.MessageKindTask, types.MessageKindUserInput},
	}
	filters := []state.MessageClaimFilter{
		{
			ThreadID:       base.ThreadID,
			TeamID:         base.TeamID,
			Channel:        base.Channel,
			Kinds:          base.Kinds,
			AssignedToType: "agent",
			AssignedTo:     strings.TrimSpace(s.cfg.RunID),
		},
	}
	teamID := strings.TrimSpace(s.cfg.TeamID)
	roleName := strings.TrimSpace(s.cfg.RoleName)
	if teamID != "" && roleName != "" {
		filters = append(filters, state.MessageClaimFilter{
			ThreadID:       base.ThreadID,
			TeamID:         base.TeamID,
			Channel:        base.Channel,
			Kinds:          base.Kinds,
			AssignedToType: "role",
			AssignedTo:     roleName,
		})
		if s.cfg.IsCoordinator {
			filters = append(filters, state.MessageClaimFilter{
				ThreadID:       base.ThreadID,
				TeamID:         base.TeamID,
				Channel:        base.Channel,
				Kinds:          base.Kinds,
				AssignedToType: "team",
				AssignedTo:     teamID,
			})
		}
	}
	return filters
}

func (s *Session) claimNextScopedMessage(ctx context.Context, filters []state.MessageClaimFilter) (types.AgentMessage, bool, error) {
	tryClaim := func(pass []state.MessageClaimFilter) (types.AgentMessage, bool, error) {
		var claimErr error
		for _, filter := range pass {
			msg, err := s.cfg.MessageBus.ClaimNextMessage(ctx, filter, s.cfg.LeaseTTL, strings.TrimSpace(s.cfg.RunID))
			if err == nil {
				return msg, true, nil
			}
			if errors.Is(err, state.ErrMessageNotFound) {
				continue
			}
			claimErr = errors.Join(claimErr, fmt.Errorf("claim next message: %w", err))
		}
		if claimErr != nil {
			return types.AgentMessage{}, false, claimErr
		}
		return types.AgentMessage{}, false, nil
	}
	msg, ok, err := tryClaim(filters)
	if err != nil || ok {
		return msg, ok, err
	}
	// Team runs may use distinct per-role sessions. If nothing matched in the role-local
	// thread, retry without thread pinning and rely on team+assignee routing.
	if strings.TrimSpace(s.cfg.TeamID) == "" {
		return types.AgentMessage{}, false, nil
	}
	fallback := make([]state.MessageClaimFilter, 0, len(filters))
	for _, filter := range filters {
		if strings.TrimSpace(filter.ThreadID) == "" {
			continue
		}
		relaxed := filter
		relaxed.ThreadID = ""
		fallback = append(fallback, relaxed)
	}
	if len(fallback) == 0 {
		return types.AgentMessage{}, false, nil
	}
	msg, ok, err = tryClaim(fallback)
	if err != nil || ok {
		return msg, ok, err
	}
	return types.AgentMessage{}, false, nil
}

func (s *Session) processClaimedMessage(ctx context.Context, msg types.AgentMessage) error {
	kind := strings.TrimSpace(msg.Kind)
	switch kind {
	case types.MessageKindTask, types.MessageKindUserInput:
		return s.processTaskMessage(ctx, msg)
	default:
		_ = s.cfg.MessageBus.AckMessage(ctx, strings.TrimSpace(msg.MessageID), state.MessageAckResult{
			Status: types.MessageStatusDeadletter,
			Error:  "unsupported message kind: " + kind,
		})
		return nil
	}
}

func (s *Session) processTaskMessage(ctx context.Context, msg types.AgentMessage) error {
	task, err := s.resolveMessageTask(ctx, msg)
	if err != nil {
		_ = s.cfg.MessageBus.AckMessage(ctx, strings.TrimSpace(msg.MessageID), state.MessageAckResult{
			Status: types.MessageStatusDeadletter,
			Error:  err.Error(),
		})
		return err
	}
	taskID := strings.TrimSpace(task.TaskID)
	if taskID == "" {
		_ = s.cfg.MessageBus.AckMessage(ctx, strings.TrimSpace(msg.MessageID), state.MessageAckResult{
			Status: types.MessageStatusDeadletter,
			Error:  "taskID is required for task-backed message",
		})
		return fmt.Errorf("task-backed message missing taskID")
	}
	if !isPollableTask(task) {
		_ = s.cfg.MessageBus.AckMessage(ctx, strings.TrimSpace(msg.MessageID), state.MessageAckResult{
			Status: types.MessageStatusAcked,
			Metadata: map[string]any{
				"skipped": "staged_callback",
				"source":  getTaskSource(task),
			},
		})
		return nil
	}
	claimCtx := state.WithPreclaimedMessage(ctx, state.PreclaimedMessage{
		MessageID:  strings.TrimSpace(msg.MessageID),
		LeaseOwner: strings.TrimSpace(msg.LeaseOwner),
	})
	if err := s.cfg.TaskStore.ClaimTask(claimCtx, taskID, s.cfg.LeaseTTL); err != nil {
		switch {
		case errors.Is(err, state.ErrTaskClaimed), errors.Is(err, state.ErrMessageClaimed), errors.Is(err, state.ErrMessageNotClaimable), isSQLiteBusyErr(err):
			retryAt := time.Now().UTC()
			_ = s.cfg.MessageBus.NackMessage(ctx, strings.TrimSpace(msg.MessageID), err.Error(), &retryAt)
			return nil
		case errors.Is(err, state.ErrTaskTerminal), errors.Is(err, state.ErrTaskNotFound), errors.Is(err, state.ErrMessageTerminal):
			_ = s.cfg.MessageBus.AckMessage(ctx, strings.TrimSpace(msg.MessageID), state.MessageAckResult{
				Status: types.MessageStatusAcked,
				Error:  err.Error(),
			})
			return nil
		default:
			retryAt := time.Now().UTC()
			_ = s.cfg.MessageBus.NackMessage(ctx, strings.TrimSpace(msg.MessageID), err.Error(), &retryAt)
			return fmt.Errorf("claim task %s from message %s: %w", taskID, strings.TrimSpace(msg.MessageID), err)
		}
	}

	claimed, err := s.cfg.TaskStore.GetTask(ctx, taskID)
	if err != nil {
		retryAt := time.Now().UTC()
		_ = s.cfg.MessageBus.NackMessage(ctx, strings.TrimSpace(msg.MessageID), err.Error(), &retryAt)
		return fmt.Errorf("get claimed %s: %w", taskID, err)
	}
	claimed.ClaimedByAgentID = strings.TrimSpace(s.cfg.RunID)
	if strings.TrimSpace(s.cfg.TeamID) != "" {
		claimed.RoleSnapshot = strings.TrimSpace(s.cfg.RoleName)
	}
	if err := s.cfg.TaskStore.UpdateTask(ctx, claimed); err != nil {
		retryAt := time.Now().UTC()
		_ = s.cfg.MessageBus.NackMessage(ctx, strings.TrimSpace(msg.MessageID), err.Error(), &retryAt)
		return fmt.Errorf("update claimed %s: %w", taskID, err)
	}
	if claimed.Attempts > s.cfg.MaxRetries {
		if err := s.quarantineTask(ctx, claimed); err != nil {
			return err
		}
		_ = s.cfg.MessageBus.AckMessage(ctx, strings.TrimSpace(msg.MessageID), state.MessageAckResult{
			Status: types.MessageStatusDeadletter,
			Error:  "max task retries exceeded",
		})
		return nil
	}
	if err := s.runTask(ctx, taskID, claimed); err != nil {
		retryAt := time.Now().UTC()
		_ = s.cfg.MessageBus.NackMessage(ctx, strings.TrimSpace(msg.MessageID), err.Error(), &retryAt)
		return err
	}
	_ = s.cfg.MessageBus.AckMessage(ctx, strings.TrimSpace(msg.MessageID), state.MessageAckResult{
		Status: types.MessageStatusAcked,
	})
	return nil
}

func (s *Session) resolveMessageTask(ctx context.Context, msg types.AgentMessage) (types.Task, error) {
	if msg.Task != nil {
		return *msg.Task, nil
	}
	taskRef := strings.TrimSpace(msg.TaskRef)
	if taskRef == "" && msg.Body != nil {
		if v, ok := msg.Body["taskId"]; ok {
			taskRef = strings.TrimSpace(fmt.Sprint(v))
		}
	}
	if taskRef == "" {
		return types.Task{}, fmt.Errorf("taskRef is required for task-backed message")
	}
	task, err := s.cfg.TaskStore.GetTask(ctx, taskRef)
	if err != nil {
		return types.Task{}, fmt.Errorf("load task %s: %w", taskRef, err)
	}
	return task, nil
}

func (s *Session) drainInboxTasks(ctx context.Context) (bool, error) {
	// Execute pending tasks from SQLite only (DB-routed protocol v2).
	var errs error
	hadWork := false
	s.maybeFlushStagedBatchCallbacks(ctx, true)
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
		if err := s.cfg.TaskStore.UpdateTask(ctx, claimed); err != nil {
			errs = errors.Join(errs, fmt.Errorf("update claimed %s: %w", taskID, err))
			continue
		}

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
	if raw, ok := t.Metadata[taskMetaSource]; ok {
		return strings.TrimSpace(fmt.Sprint(raw))
	}
	return ""
}

func pollableTaskStatuses() []types.TaskStatus {
	return []types.TaskStatus{
		types.TaskStatusPending,
		types.TaskStatusReviewPending,
	}
}

func isPollableTask(task types.Task) bool {
	if task.Status != types.TaskStatusReviewPending {
		return true
	}
	// Team-only review pipeline: only synthetic batch callbacks are reviewable.
	source := getTaskSource(task)
	return source == taskSourceSubagentBatchCallback || source == taskSourceTeamBatchCallback
}

func filterPollableTasks(ctx context.Context, store state.TaskStore, tasks []types.Task) []types.Task {
	out := make([]types.Task, 0, len(tasks))
	for _, task := range tasks {
		if len(task.Metadata) == 0 && store != nil {
			taskID := strings.TrimSpace(task.TaskID)
			if taskID != "" {
				if loaded, err := store.GetTask(ctx, taskID); err == nil {
					task = loaded
				}
			}
		}
		if !isPollableTask(task) {
			continue
		}
		out = append(out, task)
	}
	return out
}

func (s *Session) listPendingTasks(ctx context.Context) ([]types.Task, error) {
	isTeam := strings.TrimSpace(s.cfg.TeamID) != ""
	isChildRun := strings.TrimSpace(s.cfg.ParentRunID) != ""
	if !isTeam || isChildRun {
		tasks, err := s.cfg.TaskStore.ListTasks(ctx, state.TaskFilter{
			TeamID:         strings.TrimSpace(s.cfg.TeamID),
			RunID:          s.cfg.RunID,
			AssignedToType: "agent",
			AssignedTo:     s.cfg.RunID,
			Status:         pollableTaskStatuses(),
			SortBy:         "priority",
			Limit:          s.cfg.MaxPending,
		})
		if err != nil {
			return nil, err
		}
		tasks = filterPollableTasks(ctx, s.cfg.TaskStore, tasks)
		// Prioritize subagent callbacks so the parent processes them before the overarching task.
		sort.SliceStable(tasks, func(i, j int) bool {
			sourceI := getTaskSource(tasks[i])
			sourceJ := getTaskSource(tasks[j])
			si := isCallbackSource(sourceI)
			sj := isCallbackSource(sourceJ)
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
	roleTasks, err := s.cfg.TaskStore.ListTasks(ctx, state.TaskFilter{
		TeamID:         s.cfg.TeamID,
		AssignedToType: "role",
		AssignedTo:     s.cfg.RoleName,
		Status:         pollableTaskStatuses(),
		SortBy:         "priority",
		Limit:          limit,
	})
	if err != nil {
		return nil, err
	}
	roleTasks = filterPollableTasks(ctx, s.cfg.TaskStore, roleTasks)
	// Team roles can also receive agent-addressed callback tasks (e.g. subagent callbacks
	// routed back to the spawning run). Include those so callbacks are not missed.
	agentTasks, err := s.cfg.TaskStore.ListTasks(ctx, state.TaskFilter{
		TeamID:         s.cfg.TeamID,
		RunID:          s.cfg.RunID,
		AssignedToType: "agent",
		AssignedTo:     s.cfg.RunID,
		Status:         pollableTaskStatuses(),
		SortBy:         "priority",
		Limit:          limit,
	})
	if err != nil {
		return nil, err
	}
	agentTasks = filterPollableTasks(ctx, s.cfg.TaskStore, agentTasks)
	tasks := make([]types.Task, 0, len(roleTasks)+len(agentTasks))
	seen := map[string]struct{}{}
	for _, task := range roleTasks {
		id := strings.TrimSpace(task.TaskID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		tasks = append(tasks, task)
	}
	for _, task := range agentTasks {
		id := strings.TrimSpace(task.TaskID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		tasks = append(tasks, task)
	}
	if !s.cfg.IsCoordinator {
		return tasks, nil
	}
	unassigned, err := s.cfg.TaskStore.ListTasks(ctx, state.TaskFilter{
		TeamID:         s.cfg.TeamID,
		AssignedToType: "team",
		AssignedTo:     s.cfg.TeamID,
		Status:         pollableTaskStatuses(),
		SortBy:         "priority",
		Limit:          limit,
	})
	if err != nil {
		return nil, err
	}
	unassigned = filterPollableTasks(ctx, s.cfg.TaskStore, unassigned)
	out := make([]types.Task, 0, len(tasks)+len(unassigned))
	seen = map[string]struct{}{}
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
	if err := s.cfg.TaskStore.UpdateTask(ctx, task); err != nil {
		return fmt.Errorf("mark task active %s: %w", taskID, err)
	}

	taskKind := strings.TrimSpace(task.TaskKind)
	// Infer from task ID when kind is missing or stored as "other" (e.g. tasks created without TaskKind).
	if taskKind == "" || strings.EqualFold(taskKind, state.TaskKindOther) {
		switch {
		case strings.HasPrefix(strings.TrimSpace(task.TaskID), "heartbeat-"):
			taskKind = state.TaskKindHeartbeat
		case strings.HasPrefix(strings.TrimSpace(task.TaskID), "callback-"):
			taskKind = state.TaskKindCallback
		case strings.HasPrefix(strings.TrimSpace(task.TaskID), "task-"):
			taskKind = state.TaskKindTask
		default:
			if taskKind == "" {
				taskKind = state.TaskKindOther
			}
		}
	}
	taskSource := ""
	taskHeartbeatJob := ""
	taskHeartbeatInterval := ""
	if task.Metadata != nil {
		if raw, ok := task.Metadata[taskMetaSource]; ok {
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
		taskSource = taskSourceHeartbeat
	}
	if s.cfg.Events != nil {
		data := map[string]string{
			"taskId":   taskID,
			"profile":  s.activeProfile.ID,
			"goal":     truncateText(task.Goal, 100),
			"taskKind": taskKind,
		}
		if taskSource != "" {
			data[taskMetaSource] = taskSource
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
			s.cfg.SoulContent,
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
		if in, out, ok := cost.LookupPricing(ctx, modelID); ok {
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

	taskCtx := hosttools.WithBatchWaveState(hosttools.WithParentTaskID(ctx, taskID))
	var runRes agent.RunResult
	var err error

	if s.cfg.RunConversationStore != nil && taskKind == state.TaskKindTask {
		if s.cfg.Events != nil {
			s.emitBestEffort(ctx, events.Event{
				Type:    "run.conversation.active",
				Message: "Run conversation branch active",
				Data:    map[string]string{"runId": s.cfg.RunID},
			})
		}
		var msgs []llmtypes.LLMMessage
		var loadFailed bool
		msgs, err = s.cfg.RunConversationStore.LoadMessages(taskCtx, s.cfg.RunID)
		if err != nil {
			loadFailed = true
			if s.cfg.Events != nil {
				s.emitBestEffort(ctx, events.Event{
					Type:    "run.conversation.load_failed",
					Message: "Run conversation load failed",
					Data: map[string]string{
						"runId": s.cfg.RunID,
						"error": err.Error(),
					},
				})
			}
			msgs = nil
		}
		loadedCount := len(msgs)
		if s.cfg.Events != nil {
			s.emitBestEffort(ctx, events.Event{
				Type:    "run.conversation.loaded",
				Message: "Run conversation loaded",
				Data: map[string]string{
					"runId":              s.cfg.RunID,
					"loadedMessageCount": fmt.Sprintf("%d", loadedCount),
				},
			})
		}
		msgs = append(msgs, llmtypes.LLMMessage{
			Role:    "user",
			Content: strings.TrimSpace(task.Goal),
		})

		var updatedMsgs []llmtypes.LLMMessage
		runRes, updatedMsgs, _, err = runAgent.RunConversation(taskCtx, msgs)
		if err == nil && !loadFailed {
			if saveErr := s.cfg.RunConversationStore.SaveMessages(taskCtx, s.cfg.RunID, updatedMsgs); saveErr != nil {
				if s.cfg.Events != nil {
					s.emitBestEffort(ctx, events.Event{
						Type:    "run.conversation.save_failed",
						Message: "Run conversation save failed",
						Data: map[string]string{
							"runId": s.cfg.RunID,
							"error": saveErr.Error(),
						},
					})
				}
			} else if s.cfg.Events != nil {
				s.emitBestEffort(ctx, events.Event{
					Type:    "run.conversation.saved",
					Message: "Run conversation saved",
					Data: map[string]string{
						"runId":             s.cfg.RunID,
						"savedMessageCount": fmt.Sprintf("%d", len(updatedMsgs)),
					},
				})
			}
		} else if err == nil && loadFailed {
			if s.cfg.Events != nil {
				s.emitBestEffort(ctx, events.Event{
					Type:    "run.conversation.save_skipped",
					Message: "Run conversation save skipped due to prior load failure",
					Data: map[string]string{
						"runId": s.cfg.RunID,
					},
				})
			}
		}
	} else {
		if s.cfg.Events != nil {
			reason := "store_nil"
			if s.cfg.RunConversationStore != nil {
				reason = "task_kind"
			}
			s.emitBestEffort(ctx, events.Event{
				Type:    "run.conversation.skipped",
				Message: "Run conversation branch skipped",
				Data:    map[string]string{"runId": s.cfg.RunID, "reason": reason, "taskKind": taskKind},
			})
		}
		runRes, err = runAgent.Run(taskCtx, strings.TrimSpace(task.Goal))
	}

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
			var repeatedInvalidErr *agent.RepeatedInvalidToolCallError
			if errors.As(err, &repeatedInvalidErr) {
				payload := map[string]string{
					"taskId":             taskID,
					"tool":               strings.TrimSpace(repeatedInvalidErr.ToolName),
					"reason":             strings.TrimSpace(repeatedInvalidErr.LastError),
					"consecutiveInvalid": fmt.Sprintf("%d", repeatedInvalidErr.Count),
					"elapsedSeconds":     fmt.Sprintf("%d", int(repeatedInvalidErr.Elapsed.Round(time.Second).Seconds())),
				}
				s.emitBestEffort(ctx, events.Event{
					Type:    "task.tool.invalid_repeated",
					Message: "Task failed due to repeated invalid tool calls",
					Data:    payload,
				})
			}
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
	if source := metadataString(task.Metadata, taskMetaSource); source == taskSourceSubagentBatchCallback || source == taskSourceTeamBatchCallback {
		decisions, _ := task.Metadata["batchItemDecisions"].(map[string]any)
		approved, retried, escalated := countDecisionMap(decisions)
		if len(decisions) != 0 {
			summarySuffix := fmt.Sprintf("(approved=%d retry=%d escalate=%d)", approved, retried, escalated)
			if strings.TrimSpace(tr.Summary) == "" {
				tr.Summary = "Batch review decisions applied " + summarySuffix
			} else if !strings.Contains(tr.Summary, "approved=") {
				tr.Summary = strings.TrimSpace(tr.Summary) + " " + summarySuffix
			}
			s.emitBestEffort(ctx, events.Event{
				Type:    "callback.batch.item.reviewed",
				Message: "Batch review decisions updated",
				Data: map[string]string{
					"parentTaskId":         metadataString(task.Metadata, "batchParentTaskId"),
					"approved":             fmt.Sprintf("%d", approved),
					reviewDecisionRetry:    fmt.Sprintf("%d", retried),
					reviewDecisionEscalate: fmt.Sprintf("%d", escalated),
				},
			})
		}
	}
	tr.Summary = synthesizeBatchSummary(task, tr)

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
			data[taskMetaSource] = taskSource
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
		if strings.EqualFold(taskSource, taskSourceHeartbeat) {
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
	if err := s.cfg.TaskStore.UpdateTask(completeCtx, task); err != nil {
		return fmt.Errorf("persist completed task %s: %w", taskID, err)
	}

	if s.cfg.Notifier != nil {
		if err := s.cfg.Notifier.Notify(completeCtx, task, tr); err != nil {
			s.emitBestEffort(completeCtx, events.Event{Type: "task.notify.error", Message: "Task notification failed", Data: map[string]string{"taskId": taskID, "error": err.Error()}})
		}
	}
	if s.cfg.SingleTask {
		// Sub-agents spawned via spawn_worker or retry should not exit immediately;
		// they enter a review-wait state so the parent can approve or retry.
		if taskSource := ""; task.Metadata != nil {
			taskSource, _ = task.Metadata[taskMetaSource].(string)
			if taskSource == taskSourceSpawnWorker || taskSource == reviewDecisionRetry {
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
		payload[taskMetaSource] = strings.TrimSpace(source)
	}
	s.emitBestEffort(ctx, events.Event{Type: "task.queued", Message: "Task queued", Data: payload})
}

type batchGroupScope struct {
	mode         string
	parentTaskID string
	waveID       string
	reviewerID   string
}

type batchCloseAndHandoffStore interface {
	CloseBatchAndHandoff(ctx context.Context, batchTaskID, reviewerIdentity, reviewSummary string) (string, error)
}

type atomicBatchCloseAndHandoffStore interface {
	CloseBatchAndHandoffAtomic(ctx context.Context, batchTaskID, reviewerIdentity, reviewSummary string) (handoffTaskID string, approved, retried, escalated int, err error)
}

func (s *Session) maybeCreateCoordinatorCallback(ctx context.Context, task types.Task, tr types.TaskResult) {
	source := metadataString(task.Metadata, taskMetaSource)
	if source == taskSourceReviewHandoff {
		return
	}
	if source == taskSourceTeamBatchCallback && tr.Status == types.TaskStatusSucceeded {
		s.maybeCreateReviewerToCoordinatorHandoff(ctx, task, tr)
	}
	if isCallbackSource(source) {
		return
	}
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
	batchParentTaskID := metadataString(task.Metadata, "batchParentTaskId")
	batchWaveID := metadataString(task.Metadata, "batchWaveId")
	if batchParentTaskID == "" {
		batchParentTaskID = taskID
	}
	if batchWaveID == "" {
		// Always place callbacks in a deterministic wave so review delivery is batch-only.
		batchWaveID = normalizeWaveID(batchParentTaskID, "")
	}
	var callback types.Task
	var group batchGroupScope
	if isSubagentWorker {
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
			TeamID:         strings.TrimSpace(s.cfg.TeamID),
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
				taskMetaSource:      taskSourceSubagentCallback,
				"callbackForTaskId": taskID,
				"sourceRunId":       strings.TrimSpace(s.cfg.RunID),
				"sourceTeamId":      strings.TrimSpace(s.cfg.TeamID),
				"sourceTaskStatus":  string(tr.Status),
				"reviewGate":        true,
				"reviewActions":     []string{reviewDecisionApprove, reviewDecisionRetry, reviewDecisionEscalate},
				"retryBudget":       float64(3),
				"retryCount":        float64(0),
			},
		}
		group = batchGroupScope{mode: "agent", parentTaskID: batchParentTaskID, waveID: batchWaveID, reviewerID: parentRunID}
	} else {
		coordinatorRole := strings.TrimSpace(s.cfg.CoordinatorRole)
		reviewerRole := strings.TrimSpace(s.cfg.ReviewerRole)
		if reviewerRole == "" {
			reviewerRole = coordinatorRole
		}
		fallbackToCoordinator := false
		if reviewerRole == "" {
			reviewerRole = coordinatorRole
			fallbackToCoordinator = true
		}
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
			SessionID:      strings.TrimSpace(s.cfg.SessionID),
			RunID:          strings.TrimSpace(s.cfg.RunID),
			TeamID:         strings.TrimSpace(s.cfg.TeamID),
			AssignedRole:   reviewerRole,
			AssignedToType: "role",
			AssignedTo:     reviewerRole,
			CreatedBy:      strings.TrimSpace(s.cfg.RoleName),
			TaskKind:       state.TaskKindCallback,
			Goal:           callbackGoal,
			Inputs:         inputs,
			Priority:       1,
			Status:         types.TaskStatusPending,
			CreatedAt:      &now,
			Metadata: map[string]any{
				taskMetaSource:      taskSourceTeamCallback,
				"callbackForTaskId": taskID,
				"sourceRole":        strings.TrimSpace(s.cfg.RoleName),
				"sourceRunId":       strings.TrimSpace(s.cfg.RunID),
				"sourceTaskStatus":  string(tr.Status),
				"reviewerRole":      reviewerRole,
			},
		}
		if fallbackToCoordinator {
			callback.Metadata["reviewerFallback"] = "coordinator"
		}
		group = batchGroupScope{mode: "team", parentTaskID: batchParentTaskID, waveID: batchWaveID, reviewerID: reviewerRole}
	}
	if callback.Metadata == nil {
		callback.Metadata = map[string]any{}
	}
	callback.Status = types.TaskStatusReviewPending
	callback.Metadata["batchMode"] = true
	callback.Metadata["batchParentTaskId"] = batchParentTaskID
	callback.Metadata["batchWaveId"] = batchWaveID
	callback.Metadata["batchDelivered"] = false

	if err := s.cfg.TaskStore.CreateTask(ctx, callback); err != nil {
		return // idempotent via deterministic callback task id.
	}

	s.emitBatchProgress(ctx, group)
	s.maybeFlushBatchGroup(ctx, group)
}

func (s *Session) maybeCreateReviewerToCoordinatorHandoff(ctx context.Context, task types.Task, tr types.TaskResult) {
	if s == nil || s.cfg.TaskStore == nil {
		return
	}
	batchTaskID := strings.TrimSpace(task.TaskID)
	if batchTaskID == "" {
		return
	}
	reviewerIdentity := strings.TrimSpace(task.AssignedRole)
	if reviewerIdentity == "" {
		reviewerIdentity = strings.TrimSpace(s.cfg.RoleName)
	}
	if reviewerIdentity == "" {
		reviewerIdentity = strings.TrimSpace(task.AssignedTo)
	}
	if reviewerIdentity == "" {
		reviewerIdentity = "reviewer"
	}
	summary := strings.TrimSpace(tr.Summary)
	if summary == "" {
		summary = "Batch review completed."
	}

	if closer, ok := s.cfg.TaskStore.(batchCloseAndHandoffStore); ok {
		if _, err := closer.CloseBatchAndHandoff(ctx, batchTaskID, reviewerIdentity, summary); err != nil {
			s.emitBestEffort(ctx, events.Event{
				Type:    "callback.batch.close.error",
				Message: "Failed to create reviewer->coordinator handoff",
				Data: map[string]string{
					"taskId":        batchTaskID,
					"reviewedBy":    reviewerIdentity,
					"closeError":    err.Error(),
					taskMetaSource:  taskSourceTeamBatchCallback,
					"ensureHandoff": "true",
				},
			})
		}
		return
	}
	if atomicCloser, ok := s.cfg.TaskStore.(atomicBatchCloseAndHandoffStore); ok {
		if _, _, _, _, err := atomicCloser.CloseBatchAndHandoffAtomic(ctx, batchTaskID, reviewerIdentity, summary); err != nil {
			s.emitBestEffort(ctx, events.Event{
				Type:    "callback.batch.close.error",
				Message: "Failed to create reviewer->coordinator handoff",
				Data: map[string]string{
					"taskId":        batchTaskID,
					"reviewedBy":    reviewerIdentity,
					"closeError":    err.Error(),
					taskMetaSource:  taskSourceTeamBatchCallback,
					"ensureHandoff": "true",
				},
			})
		}
	}
}

func containsRoleCI(roles []string, want string) bool {
	want = strings.TrimSpace(want)
	if want == "" {
		return false
	}
	for _, role := range roles {
		if strings.EqualFold(strings.TrimSpace(role), want) {
			return true
		}
	}
	return false
}

func (s *Session) maybeFlushStagedBatchCallbacks(ctx context.Context, _ bool) {
	groups := s.collectStagedBatchGroups(ctx)
	for _, group := range groups {
		s.maybeFlushBatchGroup(ctx, group)
	}
}

func (s *Session) collectStagedBatchGroups(ctx context.Context) []batchGroupScope {
	filter := state.TaskFilter{
		Status: []types.TaskStatus{types.TaskStatusReviewPending},
		SortBy: "created_at",
		Limit:  500,
	}
	if strings.TrimSpace(s.cfg.TeamID) != "" {
		filter.TeamID = strings.TrimSpace(s.cfg.TeamID)
	} else if strings.TrimSpace(s.cfg.ParentRunID) != "" {
		filter.SessionID = strings.TrimSpace(s.cfg.SessionID)
	} else {
		return nil
	}
	tasks, err := s.cfg.TaskStore.ListTasks(ctx, filter)
	if err != nil {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]batchGroupScope, 0, 8)
	for _, task := range tasks {
		task = s.loadTaskDetails(ctx, task)
		if !metadataBool(task.Metadata, "batchMode") || metadataBool(task.Metadata, "batchDelivered") {
			continue
		}
		parentTaskID := metadataString(task.Metadata, "batchParentTaskId")
		if parentTaskID == "" {
			continue
		}
		source := metadataString(task.Metadata, taskMetaSource)
		group := batchGroupScope{
			parentTaskID: parentTaskID,
			waveID:       metadataString(task.Metadata, "batchWaveId"),
		}
		switch source {
		case taskSourceSubagentCallback:
			group.mode = "agent"
			group.reviewerID = strings.TrimSpace(task.AssignedTo)
		case taskSourceTeamCallback:
			group.mode = "team"
			group.reviewerID = strings.TrimSpace(task.AssignedRole)
		default:
			continue
		}
		if group.reviewerID == "" {
			continue
		}
		group.waveID = normalizeWaveID(group.parentTaskID, group.waveID)
		key := group.mode + "|" + group.parentTaskID + "|" + group.waveID + "|" + group.reviewerID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, group)
	}
	return out
}

func (s *Session) maybeFlushBatchGroup(ctx context.Context, group batchGroupScope) {
	group.parentTaskID = strings.TrimSpace(group.parentTaskID)
	group.waveID = normalizeWaveID(group.parentTaskID, group.waveID)
	group.reviewerID = strings.TrimSpace(group.reviewerID)
	if group.parentTaskID == "" || group.reviewerID == "" {
		return
	}

	expected := s.listBatchExpectedTasks(ctx, group)
	expectedCount := len(expected)

	callbacks := s.listBatchCallbacks(ctx, group)
	if len(callbacks) == 0 {
		return
	}
	completedCount := len(callbacks)
	if expectedCount == 0 {
		// Hard cutover: support singleton/non-staged sources as implicit batch-of-1.
		expectedCount = completedCount
	}
	undelivered := make([]types.Task, 0, len(callbacks))
	for _, cb := range callbacks {
		if metadataBool(cb.Metadata, "batchDelivered") {
			continue
		}
		undelivered = append(undelivered, cb)
	}
	if len(undelivered) == 0 {
		return
	}

	allComplete := completedCount >= expectedCount
	if !allComplete {
		return
	}
	if s.hasOpenSyntheticBatchCallback(ctx, group) {
		return
	}

	now := time.Now().UTC()
	flushReason := "all_complete"
	batchTaskID := fmt.Sprintf("callback-batch-%s-%d", group.parentTaskID, now.UnixNano())
	source := taskSourceSubagentBatchCallback
	taskKind := state.TaskKindReview
	assignedToType := "agent"
	sessionID := strings.TrimSpace(s.cfg.SessionID)
	runID := group.reviewerID
	teamID := ""
	assignedRole := ""
	createdBy := strings.TrimSpace(s.cfg.RunID)
	if group.mode == "team" {
		source = taskSourceTeamBatchCallback
		taskKind = state.TaskKindCallback
		assignedToType = "role"
		sessionID = strings.TrimSpace(s.cfg.SessionID)
		runID = strings.TrimSpace(s.cfg.RunID)
		teamID = strings.TrimSpace(s.cfg.TeamID)
		assignedRole = group.reviewerID
		createdBy = strings.TrimSpace(s.cfg.RoleName)
	} else if strings.TrimSpace(s.cfg.TeamID) != "" {
		teamID = strings.TrimSpace(s.cfg.TeamID)
	}

	items := make([]any, 0, len(undelivered))
	for _, cb := range undelivered {
		item := map[string]any{
			"callbackTaskId": strings.TrimSpace(cb.TaskID),
			"sourceTaskId":   metadataString(cb.Metadata, "callbackForTaskId"),
			"sourceStatus":   metadataString(cb.Metadata, "sourceTaskStatus"),
			"sourceRunId":    firstNonEmpty(metadataString(cb.Metadata, "sourceRunId"), metadataString(cb.Metadata, "sourceRunID")),
			"sourceRole":     metadataString(cb.Metadata, "sourceRole"),
			"summary":        inputString(cb.Inputs, "summary"),
			"error":          inputString(cb.Inputs, "error"),
			"artifacts":      inputStringSlice(cb.Inputs, "artifacts"),
			"decision":       "",
		}
		items = append(items, item)
	}

	goal := fmt.Sprintf("BATCH REVIEW ONLY: [batch %d items] Review delegated results for parent task %s. Decide per child item using task_review (approve/retry/escalate). Provide a final review summary and next actions. If work quality is weak/incomplete, delegate concrete follow-up tasks before finishing.", len(undelivered), truncateText(group.parentTaskID, 32))

	batch := types.Task{
		TaskID:         batchTaskID,
		SessionID:      sessionID,
		RunID:          runID,
		TeamID:         teamID,
		AssignedRole:   assignedRole,
		AssignedToType: assignedToType,
		AssignedTo:     group.reviewerID,
		CreatedBy:      createdBy,
		TaskKind:       taskKind,
		Goal:           goal,
		Inputs: map[string]any{
			"batchParentTaskId": group.parentTaskID,
			"batchWaveId":       group.waveID,
			"items":             items,
		},
		Priority:  1,
		Status:    types.TaskStatusPending,
		CreatedAt: &now,
		Metadata: map[string]any{
			taskMetaSource:        source,
			"batchMode":           true,
			"batchSynthetic":      true,
			"batchParentTaskId":   group.parentTaskID,
			"batchWaveId":         group.waveID,
			"coordinatorRole":     strings.TrimSpace(s.cfg.CoordinatorRole),
			"batchExpectedCount":  expectedCount,
			"batchCompletedCount": completedCount,
			"batchWaveExpected":   expectedCount,
			"batchWaveCompleted":  completedCount,
			"batchPartial":        false,
			"batchFlushReason":    flushReason,
			"batchItemDecisions":  map[string]any{},
			"reviewGate":          true,
			"reviewActions":       []string{reviewDecisionApprove, reviewDecisionRetry, reviewDecisionEscalate},
		},
	}
	if err := s.cfg.TaskStore.CreateTask(ctx, batch); err != nil {
		return
	}
	for _, cb := range undelivered {
		if cb.Metadata == nil {
			cb.Metadata = map[string]any{}
		}
		cb.Metadata["batchDelivered"] = true
		cb.Metadata["batchIncludedIn"] = batchTaskID
		_ = s.cfg.TaskStore.UpdateTask(ctx, cb)
	}
	s.emitTaskQueuedOnce(ctx, batchTaskID, batch.Goal, source)
	s.emitBestEffort(ctx, events.Event{
		Type:    "callback.batch.queued",
		Message: "Batched callback queued",
		Data: map[string]string{
			"taskId":               batchTaskID,
			"parentTaskId":         group.parentTaskID,
			"batchWaveId":          group.waveID,
			"items":                fmt.Sprintf("%d", len(undelivered)),
			"batchExpectedCount":   fmt.Sprintf("%d", expectedCount),
			"batchCompletedCount":  fmt.Sprintf("%d", completedCount),
			"batchFlushReason":     flushReason,
			"batchPartial":         "false",
			"batchReviewer":        group.reviewerID,
			"batchReviewerMode":    group.mode,
			"batchSyntheticSource": source,
		},
	})
	s.emitBatchProgress(ctx, group)
}

func (s *Session) emitBatchProgress(ctx context.Context, group batchGroupScope) {
	expectedCount := len(s.listBatchExpectedTasks(ctx, group))
	completedCount := len(s.listBatchCallbacks(ctx, group))
	if expectedCount == 0 {
		return
	}
	s.emitBestEffort(ctx, events.Event{
		Type:    "callback.batch.progress",
		Message: "Batch callback progress updated",
		Data: map[string]string{
			"parentTaskId":        group.parentTaskID,
			"batchWaveId":         group.waveID,
			"batchExpectedCount":  fmt.Sprintf("%d", expectedCount),
			"batchCompletedCount": fmt.Sprintf("%d", completedCount),
			"batchReviewer":       group.reviewerID,
			"batchReviewerMode":   group.mode,
		},
	})
}

func (s *Session) listBatchExpectedTasks(ctx context.Context, group batchGroupScope) []types.Task {
	filter := state.TaskFilter{SortBy: "created_at", Limit: 500}
	if group.mode == "team" {
		filter.TeamID = strings.TrimSpace(s.cfg.TeamID)
	} else {
		filter.SessionID = strings.TrimSpace(s.cfg.SessionID)
	}
	tasks, err := s.cfg.TaskStore.ListTasks(ctx, filter)
	if err != nil {
		return nil
	}
	out := make([]types.Task, 0, len(tasks))
	for _, task := range tasks {
		task = s.loadTaskDetails(ctx, task)
		if !metadataBool(task.Metadata, "batchMode") {
			continue
		}
		if metadataString(task.Metadata, "batchParentTaskId") != group.parentTaskID {
			continue
		}
		if !waveMatches(group.parentTaskID, group.waveID, metadataString(task.Metadata, "batchWaveId")) {
			continue
		}
		if isCallbackSource(metadataString(task.Metadata, taskMetaSource)) {
			continue
		}
		if group.mode == "agent" {
			if metadataString(task.Metadata, taskMetaSource) != taskSourceSpawnWorker {
				continue
			}
			if metadataString(task.Metadata, "parentRunId") != group.reviewerID {
				continue
			}
		}
		out = append(out, task)
	}
	return out
}

func (s *Session) listBatchCallbacks(ctx context.Context, group batchGroupScope) []types.Task {
	filter := state.TaskFilter{
		Status: []types.TaskStatus{types.TaskStatusReviewPending},
		SortBy: "created_at",
		Limit:  500,
	}
	if group.mode == "team" {
		filter.TeamID = strings.TrimSpace(s.cfg.TeamID)
	} else {
		filter.SessionID = strings.TrimSpace(s.cfg.SessionID)
	}
	tasks, err := s.cfg.TaskStore.ListTasks(ctx, filter)
	if err != nil {
		return nil
	}
	out := make([]types.Task, 0, len(tasks))
	for _, task := range tasks {
		task = s.loadTaskDetails(ctx, task)
		if !metadataBool(task.Metadata, "batchMode") {
			continue
		}
		if metadataString(task.Metadata, "batchParentTaskId") != group.parentTaskID {
			continue
		}
		if !waveMatches(group.parentTaskID, group.waveID, metadataString(task.Metadata, "batchWaveId")) {
			continue
		}
		source := metadataString(task.Metadata, taskMetaSource)
		if group.mode == "agent" && source != taskSourceSubagentCallback {
			continue
		}
		if group.mode == "team" && source != taskSourceTeamCallback {
			continue
		}
		if group.mode == "agent" && strings.TrimSpace(task.AssignedTo) != group.reviewerID {
			continue
		}
		if group.mode == "team" && strings.TrimSpace(task.AssignedRole) != group.reviewerID {
			continue
		}
		out = append(out, task)
	}
	return out
}

func (s *Session) hasOpenSyntheticBatchCallback(ctx context.Context, group batchGroupScope) bool {
	filter := state.TaskFilter{
		Status: []types.TaskStatus{types.TaskStatusPending, types.TaskStatusActive},
		SortBy: "created_at",
		Limit:  200,
	}
	if group.mode == "team" {
		filter.TeamID = strings.TrimSpace(s.cfg.TeamID)
	} else {
		filter.SessionID = strings.TrimSpace(s.cfg.SessionID)
	}
	tasks, err := s.cfg.TaskStore.ListTasks(ctx, filter)
	if err != nil {
		return false
	}
	wantSource := taskSourceSubagentBatchCallback
	if group.mode == "team" {
		wantSource = taskSourceTeamBatchCallback
	}
	for _, task := range tasks {
		task = s.loadTaskDetails(ctx, task)
		if !metadataBool(task.Metadata, "batchSynthetic") {
			continue
		}
		if metadataString(task.Metadata, taskMetaSource) != wantSource {
			continue
		}
		if metadataString(task.Metadata, "batchParentTaskId") != group.parentTaskID {
			continue
		}
		if !waveMatches(group.parentTaskID, group.waveID, metadataString(task.Metadata, "batchWaveId")) {
			continue
		}
		if group.mode == "agent" && strings.TrimSpace(task.AssignedTo) != group.reviewerID {
			continue
		}
		if group.mode == "team" && strings.TrimSpace(task.AssignedRole) != group.reviewerID {
			continue
		}
		return true
	}
	return false
}

func metadataString(m map[string]any, key string) string {
	if len(m) == 0 {
		return ""
	}
	raw, ok := m[key]
	if !ok || raw == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(raw))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if v := strings.TrimSpace(value); v != "" {
			return v
		}
	}
	return ""
}

func metadataBool(m map[string]any, key string) bool {
	if len(m) == 0 {
		return false
	}
	raw, ok := m[key]
	if !ok || raw == nil {
		return false
	}
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	case float64:
		return v != 0
	case int:
		return v != 0
	default:
		return strings.EqualFold(strings.TrimSpace(fmt.Sprint(v)), "true")
	}
}

func inputString(inputs map[string]any, key string) string {
	if len(inputs) == 0 {
		return ""
	}
	raw, ok := inputs[key]
	if !ok || raw == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(raw))
}

func inputStringSlice(inputs map[string]any, key string) []string {
	if len(inputs) == 0 {
		return nil
	}
	raw, ok := inputs[key]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		out := make([]string, 0, len(v))
		for _, item := range v {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s := strings.TrimSpace(fmt.Sprint(item))
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func countDecisionMap(decisions map[string]any) (approved, retried, escalated int) {
	for _, raw := range decisions {
		switch strings.ToLower(strings.TrimSpace(fmt.Sprint(raw))) {
		case reviewDecisionApprove:
			approved++
		case reviewDecisionRetry:
			retried++
		case reviewDecisionEscalate:
			escalated++
		}
	}
	return approved, retried, escalated
}

func isCallbackSource(source string) bool {
	switch strings.TrimSpace(source) {
	case taskSourceSubagentCallback, taskSourceTeamCallback, taskSourceSubagentBatchCallback, taskSourceTeamBatchCallback, taskSourceReviewHandoff:
		return true
	default:
		return false
	}
}

func waveMatches(parentTaskID, groupWaveID, taskWaveID string) bool {
	return normalizeWaveID(parentTaskID, groupWaveID) == normalizeWaveID(parentTaskID, taskWaveID)
}

func normalizeWaveID(parentTaskID, waveID string) string {
	waveID = strings.TrimSpace(waveID)
	if waveID != "" {
		return waveID
	}
	parentTaskID = strings.TrimSpace(parentTaskID)
	if parentTaskID == "" {
		return ""
	}
	return "legacy-" + parentTaskID
}

func synthesizeBatchSummary(task types.Task, tr types.TaskResult) string {
	source := metadataString(task.Metadata, taskMetaSource)
	if source != taskSourceSubagentBatchCallback && source != taskSourceTeamBatchCallback {
		return strings.TrimSpace(tr.Summary)
	}
	current := strings.TrimSpace(tr.Summary)
	if len(current) >= 40 && strings.Contains(strings.ToLower(current), "review") {
		return current
	}

	items, _ := task.Inputs["items"].([]any)
	decisions, _ := task.Metadata["batchItemDecisions"].(map[string]any)
	approved, retried, escalated := countDecisionMap(decisions)
	parts := []string{
		fmt.Sprintf("Batch review completed for %d item(s): approved=%d retry=%d escalate=%d.", len(items), approved, retried, escalated),
	}
	if len(items) != 0 {
		preview := make([]string, 0, len(items))
		for _, raw := range items {
			item, _ := raw.(map[string]any)
			callbackID := strings.TrimSpace(fmt.Sprint(item["callbackTaskId"]))
			role := strings.TrimSpace(fmt.Sprint(item["sourceRole"]))
			decision := strings.TrimSpace(fmt.Sprint(item["decision"]))
			if decision == "" && callbackID != "" && decisions != nil {
				decision = strings.TrimSpace(fmt.Sprint(decisions[callbackID]))
			}
			if decision == "" {
				decision = "pending"
			}
			tag := truncateText(callbackID, 20)
			if tag == "" {
				tag = truncateText(strings.TrimSpace(fmt.Sprint(item["sourceTaskId"])), 20)
			}
			if role != "" {
				preview = append(preview, fmt.Sprintf("%s (%s): %s", tag, role, decision))
			} else {
				preview = append(preview, fmt.Sprintf("%s: %s", tag, decision))
			}
			if len(preview) >= 6 {
				break
			}
		}
		if len(preview) != 0 {
			parts = append(parts, "Items: "+strings.Join(preview, "; ")+".")
		}
	}
	if strings.TrimSpace(current) != "" {
		parts = append(parts, "Agent notes: "+current)
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func (s *Session) loadTaskDetails(ctx context.Context, task types.Task) types.Task {
	taskID := strings.TrimSpace(task.TaskID)
	if taskID == "" {
		return task
	}
	loaded, err := s.cfg.TaskStore.GetTask(ctx, taskID)
	if err != nil {
		return task
	}
	return loaded
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
		if getTaskSource(candidate) == taskSourceSpawnWorker {
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
		taskSource, _ := task.Metadata[taskMetaSource].(string)
		if taskSource == taskSourceSpawnWorker || taskSource == reviewDecisionRetry {
			s.emitBestEffort(ctx, events.Event{
				Type:    "task.escalation.auto",
				Message: "Sub-agent task quarantined; escalation required",
				Data: map[string]string{
					"taskId":       taskID,
					"runId":        s.cfg.RunID,
					"parentId":     s.cfg.ParentRunID,
					"goal":         truncateText(task.Goal, 200),
					"error":        fallback(strings.TrimSpace(task.Error), "max retries exceeded"),
					taskMetaSource: taskSource,
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
	if msg, ok := llmErrorClassMessages[strings.TrimSpace(info.Class)]; ok {
		return msg
	}
	return "LLM request failed"
}

var llmErrorClassMessages = map[string]string{
	"quota":           "LLM quota/credits exhausted",
	"rate_limit":      "LLM rate limit reached",
	"network":         "LLM network error",
	"timeout":         "LLM request timed out",
	"auth":            "LLM authentication failed",
	"permission":      "LLM permission denied",
	"policy":          "LLM request blocked by provider data policy (check OpenRouter privacy settings for free models)",
	"server":          "LLM provider server error",
	"invalid_request": "LLM request rejected",
}

func isSQLiteBusyErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "sqlite_busy") || strings.Contains(msg, "database is locked")
}
