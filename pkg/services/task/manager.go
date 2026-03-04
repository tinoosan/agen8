package task

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/agen8/pkg/agent/hosttools"
	"github.com/tinoosan/agen8/pkg/agent/state"
	"github.com/tinoosan/agen8/pkg/types"
)

// activeTaskCanceler is implemented by stores that support efficient cancel-by-run (e.g. SQLite).
type activeTaskCanceler interface {
	CancelActiveTasksByRun(ctx context.Context, runID, reason string) (int, error)
}

// Manager implements state.TaskStore, RetryEscalationCreator, ActiveTaskCanceler, and ArtifactIndexerProvider.
// It delegates CRUD to the store and implements callback lifecycle (retry/escalation) and cancel-by-run.
type Manager struct {
	store        state.TaskStore
	messageStore state.MessageStore
	runLoader    RunLoader
	oracle       *RoutingOracle
	events       taskEventAppender

	watchersMu sync.Mutex
	watchers   map[string]taskWakeWatcher
}

type taskEventAppender interface {
	Append(ctx context.Context, event types.EventRecord) error
}

type batchCloseStore interface {
	CloseBatchAndHandoffAtomic(ctx context.Context, batchTaskID, reviewerIdentity, reviewSummary string) (handoffTaskID string, approved, retried, escalated int, err error)
}

type taskWakeWatcher struct {
	teamID string
	runID  string
	ch     chan struct{}
}

var ErrRunLoaderNotConfigured = errors.New("run loader not configured")

// NewManager creates a new task service manager. runLoader may be nil and set later via SetRunLoader.
func NewManager(store state.TaskStore, runLoader RunLoader) *Manager {
	var messageStore state.MessageStore
	if ms, ok := store.(state.MessageStore); ok {
		messageStore = ms
	}
	return &Manager{
		store:        store,
		messageStore: messageStore,
		runLoader:    runLoader,
		watchers:     map[string]taskWakeWatcher{},
	}
}

// SetRunLoader sets the run loader (e.g. after session service is constructed in daemon wiring).
func (m *Manager) SetRunLoader(runLoader RunLoader) {
	m.runLoader = runLoader
}

// SetRoutingOracle configures routing validation/canonicalization for task writes.
func (m *Manager) SetRoutingOracle(oracle *RoutingOracle) {
	m.oracle = oracle
}

func (m *Manager) SetEventsStore(store taskEventAppender) {
	m.events = store
}

func (m *Manager) SetMessageStore(store state.MessageStore) {
	m.messageStore = store
}

// SubscribeWake returns a channel that receives best-effort wake signals when matching
// tasks are created/updated/completed. Filters are optional; when both are empty, all
// task mutations trigger signals.
func (m *Manager) SubscribeWake(teamID, runID string) (<-chan struct{}, func()) {
	if m == nil {
		ch := make(chan struct{})
		close(ch)
		return ch, func() {}
	}
	id := uuid.NewString()
	w := taskWakeWatcher{
		teamID: strings.TrimSpace(teamID),
		runID:  strings.TrimSpace(runID),
		ch:     make(chan struct{}, 1),
	}
	m.watchersMu.Lock()
	m.watchers[id] = w
	m.watchersMu.Unlock()
	cancel := func() {
		m.watchersMu.Lock()
		ww, ok := m.watchers[id]
		if ok {
			delete(m.watchers, id)
		}
		m.watchersMu.Unlock()
		if ok {
			close(ww.ch)
		}
	}
	return w.ch, cancel
}

func (m *Manager) notifyWake(task types.Task) {
	if m == nil {
		return
	}
	taskTeam := strings.TrimSpace(task.TeamID)
	taskRun := strings.TrimSpace(task.RunID)
	m.watchersMu.Lock()
	defer m.watchersMu.Unlock()
	for _, w := range m.watchers {
		if w.teamID != "" && taskTeam != w.teamID {
			continue
		}
		if w.runID != "" && taskRun != w.runID {
			continue
		}
		select {
		case w.ch <- struct{}{}:
		default:
		}
	}
}

// CreateRetryTask creates a retry task for a child run (loads run via RunLoader, builds task, persists).
func (m *Manager) CreateRetryTask(ctx context.Context, childRunID, feedback string) error {
	childRunID = strings.TrimSpace(childRunID)
	if childRunID == "" {
		return fmt.Errorf("childRunID is required")
	}
	if m.runLoader == nil {
		return ErrRunLoaderNotConfigured
	}
	run, err := m.runLoader.LoadRun(ctx, childRunID)
	if err != nil {
		return fmt.Errorf("load child run: %w", err)
	}
	retryTask := types.Task{
		TaskID:         fmt.Sprintf("retry-%s-%s", childRunID, uuid.NewString()[:8]),
		SessionID:      run.SessionID,
		RunID:          childRunID,
		AssignedToType: "agent",
		AssignedTo:     childRunID,
		TaskKind:       "task",
		Goal:           fmt.Sprintf("RETRY with feedback:\n%s\n\nOriginal goal: %s", feedback, strings.TrimSpace(run.Goal)),
		Priority:       1,
		Status:         types.TaskStatusPending,
		Metadata: map[string]any{
			"source":        "retry",
			"parentRunId":   strings.TrimSpace(run.ParentRunID),
			"retryFeedback": feedback,
		},
	}
	return m.CreateTask(ctx, retryTask)
}

// CreateEscalationTask creates an escalation task from a callback task (loads callback via store, builds escalation task, persists).
func (m *Manager) CreateEscalationTask(ctx context.Context, callbackTaskID string, data hosttools.EscalationData) error {
	callbackTaskID = strings.TrimSpace(callbackTaskID)
	if callbackTaskID == "" {
		return fmt.Errorf("callbackTaskID is required")
	}
	task, err := m.store.GetTask(ctx, callbackTaskID)
	if err != nil {
		return fmt.Errorf("load callback task: %w", err)
	}
	now := time.Now().UTC()
	escalationTaskID := fmt.Sprintf("escalation-%s-%s", callbackTaskID, uuid.NewString()[:8])
	escMeta := map[string]any{
		"source":         "escalation",
		"reason":         data.Reason,
		"attemptSummary": data.AttemptSummary,
		"recommendation": data.Recommendation,
		"originalGoal":   data.OriginalGoal,
		"retryCount":     data.RetryCount,
		"sourceRunId":    data.SourceRunID,
		"sourceTaskId":   data.SourceTaskID,
	}
	if len(data.Artifacts) > 0 {
		escMeta["artifacts"] = data.Artifacts
	}
	escalationGoal := fmt.Sprintf("ESCALATION: %s\n\nOriginal goal: %s\nAttempts: %d\nRecommendation: %s",
		data.Reason, data.OriginalGoal, data.RetryCount, data.Recommendation)
	escalationTask := types.Task{
		TaskID:    escalationTaskID,
		SessionID: task.SessionID,
		TaskKind:  "task",
		Goal:      escalationGoal,
		Priority:  0,
		Status:    types.TaskStatusPending,
		CreatedAt: &now,
		Metadata:  escMeta,
	}
	teamID := strings.TrimSpace(task.TeamID)
	if teamID != "" {
		coordinatorRole := ""
		if cr, ok := task.Metadata["coordinatorRole"].(string); ok {
			coordinatorRole = strings.TrimSpace(cr)
		}
		if coordinatorRole == "" {
			coordinatorRole = strings.TrimSpace(task.AssignedRole)
		}
		escalationTask.TeamID = teamID
		escalationTask.AssignedRole = coordinatorRole
		escalationTask.AssignedToType = "role"
		escalationTask.AssignedTo = coordinatorRole
	} else {
		parentRunID := strings.TrimSpace(task.RunID)
		escalationTask.RunID = parentRunID
		escalationTask.AssignedToType = "agent"
		escalationTask.AssignedTo = parentRunID
	}
	if err := m.CreateTask(ctx, escalationTask); err != nil {
		return fmt.Errorf("create escalation task: %w", err)
	}
	return nil
}

// CancelActiveTasksByRun delegates to the store if it implements activeTaskCanceler, else falls back to ListTasks + CompleteTask.
func (m *Manager) CancelActiveTasksByRun(ctx context.Context, runID, reason string) (int, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return 0, fmt.Errorf("runID is required")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "run paused"
	}
	if c, ok := m.store.(activeTaskCanceler); ok {
		return c.CancelActiveTasksByRun(ctx, runID, reason)
	}
	totalCanceled := 0
	for {
		tasks, err := m.store.ListTasks(ctx, state.TaskFilter{
			RunID:  runID,
			Status: []types.TaskStatus{types.TaskStatusActive},
			Limit:  500,
		})
		if err != nil {
			return totalCanceled, err
		}
		if len(tasks) == 0 {
			return totalCanceled, nil
		}
		doneAt := time.Now().UTC()
		canceledThisPage := 0
		for _, task := range tasks {
			taskID := strings.TrimSpace(task.TaskID)
			if taskID == "" {
				continue
			}
			if err := m.store.CompleteTask(ctx, taskID, types.TaskResult{
				TaskID:      taskID,
				Status:      types.TaskStatusCanceled,
				CompletedAt: &doneAt,
				Error:       reason,
			}); err != nil {
				return totalCanceled, err
			}
			canceledThisPage++
		}
		totalCanceled += canceledThisPage
		if canceledThisPage == 0 {
			return totalCanceled, nil
		}
	}
}

// ArtifactIndexer returns the underlying store as state.ArtifactIndexer if it implements it.
func (m *Manager) ArtifactIndexer() (state.ArtifactIndexer, bool) {
	if m.store == nil {
		return nil, false
	}
	idx, ok := m.store.(state.ArtifactIndexer)
	return idx, ok
}

// TaskStore delegation (state.TaskStore)

func (m *Manager) GetTask(ctx context.Context, taskID string) (types.Task, error) {
	return m.store.GetTask(ctx, taskID)
}

func (m *Manager) GetRunStats(ctx context.Context, runID string) (state.RunStats, error) {
	return m.store.GetRunStats(ctx, runID)
}

func (m *Manager) ListTasks(ctx context.Context, filter state.TaskFilter) ([]types.Task, error) {
	return m.store.ListTasks(ctx, filter)
}

func (m *Manager) CountTasks(ctx context.Context, filter state.TaskFilter) (int, error) {
	return m.store.CountTasks(ctx, filter)
}

func (m *Manager) CreateTask(ctx context.Context, task types.Task) error {
	if m.oracle != nil {
		normalized, err := m.oracle.NormalizeCreate(ctx, m.runLoader, task)
		if err != nil {
			m.emitRoutingEvent(ctx, task, "routing.violation", err.Error(), map[string]string{
				"taskId": strings.TrimSpace(task.TaskID),
			})
			return err
		}
		task = normalized
	}
	if err := m.store.CreateTask(ctx, task); err != nil {
		return err
	}
	if err := m.publishTaskMessage(ctx, task); err != nil {
		_ = m.store.DeleteTask(ctx, task.TaskID)
		return fmt.Errorf("publish task message: %w", err)
	}
	m.emitRoutingEvent(ctx, task, "routing.validated", "Task routing validated", map[string]string{
		"taskId":       strings.TrimSpace(task.TaskID),
		"assignedType": strings.TrimSpace(task.AssignedToType),
		"assignedTo":   strings.TrimSpace(task.AssignedTo),
		"teamId":       strings.TrimSpace(task.TeamID),
	})
	m.notifyWake(task)
	return nil
}

func (m *Manager) publishTaskMessage(ctx context.Context, task types.Task) error {
	if m == nil || m.messageStore == nil {
		return nil
	}
	now := time.Now().UTC()
	taskCopy := task
	meta := task.Metadata
	kind := strings.TrimSpace(metadataString(meta, "messageKind"))
	if kind == "" {
		kind = types.MessageKindTask
	}
	intentID := strings.TrimSpace(metadataString(meta, "intentId"))
	if intentID == "" {
		intentID = "task.create:" + strings.TrimSpace(task.TaskID)
	}
	correlationID := strings.TrimSpace(metadataString(meta, "correlationId"))
	if correlationID == "" {
		correlationID = strings.TrimSpace(task.TaskID)
	}
	msg := types.AgentMessage{
		MessageID:     "msg-" + uuid.NewString(),
		IntentID:      intentID,
		CorrelationID: correlationID,
		CausationID:   strings.TrimSpace(metadataString(meta, "causationId")),
		Producer:      strings.TrimSpace(metadataString(meta, "producer")),
		ThreadID:      strings.TrimSpace(task.SessionID),
		RunID:         strings.TrimSpace(task.RunID),
		TeamID:        strings.TrimSpace(task.TeamID),
		Channel:       types.MessageChannelInbox,
		Kind:          kind,
		Body: map[string]any{
			"goal":     strings.TrimSpace(task.Goal),
			"taskKind": strings.TrimSpace(task.TaskKind),
		},
		TaskRef:     strings.TrimSpace(task.TaskID),
		Task:        &taskCopy,
		Status:      types.MessageStatusPending,
		VisibleAt:   now,
		Priority:    task.Priority,
		Metadata:    map[string]any{"source": "task.create"},
		CreatedAt:   &now,
		UpdatedAt:   &now,
		ProcessedAt: nil,
	}
	if msg.Producer == "" {
		msg.Producer = "task.manager.create"
	}
	_, err := m.messageStore.PublishMessage(ctx, msg)
	return err
}

func (m *Manager) DeleteTask(ctx context.Context, taskID string) error {
	return m.store.DeleteTask(ctx, taskID)
}

func (m *Manager) UpdateTask(ctx context.Context, task types.Task) error {
	if m.oracle != nil {
		normalized, err := m.oracle.NormalizeUpdate(ctx, m.runLoader, task)
		if err != nil {
			m.emitRoutingEvent(ctx, task, "routing.violation", err.Error(), map[string]string{
				"taskId": strings.TrimSpace(task.TaskID),
			})
			return err
		}
		task = normalized
	}
	if err := m.store.UpdateTask(ctx, task); err != nil {
		return err
	}
	m.emitRoutingEvent(ctx, task, "routing.validated", "Task routing validated", map[string]string{
		"taskId":       strings.TrimSpace(task.TaskID),
		"assignedType": strings.TrimSpace(task.AssignedToType),
		"assignedTo":   strings.TrimSpace(task.AssignedTo),
		"teamId":       strings.TrimSpace(task.TeamID),
	})
	m.notifyWake(task)
	return nil
}

func (m *Manager) CompleteTask(ctx context.Context, taskID string, result types.TaskResult) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("taskID is required")
	}
	current, err := m.store.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	if _, err := m.ensureTaskHasBackingMessage(ctx, current); err != nil {
		return err
	}
	if m.oracle != nil {
		if err := m.oracle.ValidateCompletion(ctx, m.runLoader, current); err != nil {
			return err
		}
	}
	if err := m.store.CompleteTask(ctx, taskID, result); err != nil {
		return err
	}
	updated, err := m.store.GetTask(ctx, taskID)
	if err == nil {
		_ = m.syncTaskMessagesTerminal(ctx, updated)
		m.notifyWake(updated)
	}
	return nil
}

func (m *Manager) syncTaskMessagesTerminal(ctx context.Context, task types.Task) error {
	if m == nil || m.messageStore == nil {
		return nil
	}
	taskID := strings.TrimSpace(task.TaskID)
	if taskID == "" {
		return nil
	}
	msgs, err := m.listTaskMessages(ctx, task, state.MessageFilter{
		ThreadID: strings.TrimSpace(task.SessionID),
		TeamID:   strings.TrimSpace(task.TeamID),
		TaskRef:  taskID,
		Channel:  types.MessageChannelInbox,
		Statuses: []string{types.MessageStatusPending, types.MessageStatusClaimed, types.MessageStatusNacked},
		Limit:    200,
		SortBy:   "created_at",
	})
	if err != nil {
		return err
	}
	for _, msg := range msgs {
		if strings.TrimSpace(msg.TaskRef) != taskID {
			continue
		}
		_ = m.messageStore.AckMessage(ctx, strings.TrimSpace(msg.MessageID), state.MessageAckResult{
			Status:   types.MessageStatusAcked,
			Metadata: map[string]any{"taskStatus": strings.TrimSpace(string(task.Status))},
		})
	}
	return nil
}

func (m *Manager) ClaimTask(ctx context.Context, taskID string, ttl time.Duration) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("taskID is required")
	}
	task, err := m.store.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	msg := types.AgentMessage{}
	if preclaimed, ok := state.PreclaimedMessageFromContext(ctx); ok {
		msg.MessageID = preclaimed.MessageID
		msg.LeaseOwner = preclaimed.LeaseOwner
	} else {
		msg, err = m.claimBackingMessageForTask(ctx, task, ttl)
		if err != nil {
			return err
		}
	}
	if err := m.store.ClaimTask(ctx, taskID, ttl); err != nil {
		if m.messageStore != nil && strings.TrimSpace(msg.MessageID) != "" {
			retryAt := time.Now().UTC()
			switch {
			case errors.Is(err, state.ErrTaskTerminal):
				_ = m.messageStore.AckMessage(ctx, msg.MessageID, state.MessageAckResult{
					Status: types.MessageStatusAcked,
					Error:  err.Error(),
				})
			default:
				_ = m.messageStore.NackMessage(ctx, msg.MessageID, err.Error(), &retryAt)
			}
		}
		return err
	}
	if task, err := m.store.GetTask(ctx, taskID); err == nil {
		m.notifyWake(task)
	}
	return nil
}

func (m *Manager) ensureTaskHasBackingMessage(ctx context.Context, task types.Task) (types.AgentMessage, error) {
	if m == nil || m.messageStore == nil {
		return types.AgentMessage{}, nil
	}
	msgs, err := m.listTaskMessages(ctx, task, state.MessageFilter{
		ThreadID: strings.TrimSpace(task.SessionID),
		TeamID:   strings.TrimSpace(task.TeamID),
		TaskRef:  strings.TrimSpace(task.TaskID),
		Channel:  types.MessageChannelInbox,
		Limit:    1,
		SortBy:   "created_at",
	})
	if err != nil {
		return types.AgentMessage{}, err
	}
	if len(msgs) == 0 {
		return types.AgentMessage{}, state.ErrTaskMissingMessage
	}
	return msgs[0], nil
}

func (m *Manager) claimBackingMessageForTask(ctx context.Context, task types.Task, ttl time.Duration) (types.AgentMessage, error) {
	if m == nil || m.messageStore == nil {
		return types.AgentMessage{}, nil
	}
	consumerID := strings.TrimSpace(task.ClaimedByAgentID)
	if consumerID == "" && strings.EqualFold(strings.TrimSpace(task.AssignedToType), "agent") {
		consumerID = strings.TrimSpace(task.AssignedTo)
	}
	if consumerID == "" {
		consumerID = strings.TrimSpace(task.RunID)
	}
	if consumerID == "" {
		consumerID = "task-service"
	}
	filter := state.MessageClaimFilter{
		ThreadID:       strings.TrimSpace(task.SessionID),
		TeamID:         strings.TrimSpace(task.TeamID),
		TaskRef:        strings.TrimSpace(task.TaskID),
		AssignedToType: strings.TrimSpace(task.AssignedToType),
		AssignedTo:     strings.TrimSpace(task.AssignedTo),
		Channel:        types.MessageChannelInbox,
		Kinds:          []string{types.MessageKindTask, types.MessageKindUserInput},
	}
	msg, err := m.messageStore.ClaimNextMessage(ctx, filter, ttl, consumerID)
	if err == nil {
		return msg, nil
	}
	// Team tasks can be consumed from a role session different from the task/session thread.
	// Retry without thread pinning so team+assignee routing remains authoritative.
	if errors.Is(err, state.ErrMessageNotFound) && strings.TrimSpace(task.TeamID) != "" {
		relaxed := filter
		relaxed.ThreadID = ""
		msg, err = m.messageStore.ClaimNextMessage(ctx, relaxed, ttl, consumerID)
		if err == nil {
			return msg, nil
		}
	}
	if !errors.Is(err, state.ErrMessageNotFound) {
		return types.AgentMessage{}, err
	}
	msgs, lerr := m.listTaskMessages(ctx, task, state.MessageFilter{
		ThreadID: strings.TrimSpace(task.SessionID),
		TeamID:   strings.TrimSpace(task.TeamID),
		TaskRef:  strings.TrimSpace(task.TaskID),
		Channel:  types.MessageChannelInbox,
		Limit:    50,
		SortBy:   "created_at",
	})
	if lerr != nil {
		return types.AgentMessage{}, lerr
	}
	if len(msgs) == 0 {
		return types.AgentMessage{}, state.ErrTaskMissingMessage
	}
	claimedByOther := false
	seenTerminal := false
	for _, m := range msgs {
		switch strings.TrimSpace(m.Status) {
		case types.MessageStatusClaimed:
			if consumerID != "" && strings.TrimSpace(m.LeaseOwner) == consumerID {
				// Session already claimed this message at the bus layer. Treat as usable.
				return m, nil
			}
			claimedByOther = true
		case types.MessageStatusAcked, types.MessageStatusDeadletter:
			seenTerminal = true
		}
	}
	if claimedByOther {
		return types.AgentMessage{}, state.ErrMessageClaimed
	}
	if seenTerminal {
		return types.AgentMessage{}, state.ErrMessageTerminal
	}
	return types.AgentMessage{}, state.ErrMessageNotClaimable
}

func (m *Manager) ExtendLease(ctx context.Context, taskID string, ttl time.Duration) error {
	return m.store.ExtendLease(ctx, taskID, ttl)
}

func (m *Manager) ReleaseLease(ctx context.Context, taskID string) error {
	if err := m.store.ReleaseLease(ctx, taskID); err != nil {
		return err
	}
	if task, err := m.store.GetTask(ctx, taskID); err == nil {
		m.notifyWake(task)
	}
	return nil
}

func (m *Manager) DelegateTask(ctx context.Context, taskID string) error {
	if err := m.store.DelegateTask(ctx, taskID); err != nil {
		return err
	}
	if task, err := m.store.GetTask(ctx, taskID); err == nil {
		m.notifyWake(task)
	}
	return nil
}

func (m *Manager) ResumeTask(ctx context.Context, taskID string) error {
	if err := m.store.ResumeTask(ctx, taskID); err != nil {
		return err
	}
	if task, err := m.store.GetTask(ctx, taskID); err == nil {
		m.notifyWake(task)
	}
	return nil
}

func (m *Manager) RecoverExpiredLeases(ctx context.Context) error {
	return m.store.RecoverExpiredLeases(ctx)
}

func (m *Manager) PublishMessage(ctx context.Context, msg types.AgentMessage) (types.AgentMessage, error) {
	if m == nil || m.messageStore == nil {
		return types.AgentMessage{}, fmt.Errorf("message store is not configured")
	}
	out, err := m.messageStore.PublishMessage(ctx, msg)
	if err == nil && out.Task != nil {
		m.notifyWake(*out.Task)
	}
	return out, err
}

func (m *Manager) ClaimNextMessage(ctx context.Context, filter state.MessageClaimFilter, ttl time.Duration, consumerID string) (types.AgentMessage, error) {
	if m == nil || m.messageStore == nil {
		return types.AgentMessage{}, fmt.Errorf("message store is not configured")
	}
	return m.messageStore.ClaimNextMessage(ctx, filter, ttl, consumerID)
}

func (m *Manager) AckMessage(ctx context.Context, messageID string, result state.MessageAckResult) error {
	if m == nil || m.messageStore == nil {
		return fmt.Errorf("message store is not configured")
	}
	if err := m.messageStore.AckMessage(ctx, messageID, result); err != nil {
		return err
	}
	m.notifyWakeForMessage(ctx, messageID)
	return nil
}

func (m *Manager) NackMessage(ctx context.Context, messageID string, reason string, retryAt *time.Time) error {
	if m == nil || m.messageStore == nil {
		return fmt.Errorf("message store is not configured")
	}
	if err := m.messageStore.NackMessage(ctx, messageID, reason, retryAt); err != nil {
		return err
	}
	m.notifyWakeForMessage(ctx, messageID)
	return nil
}

func (m *Manager) RequeueExpiredClaims(ctx context.Context) error {
	if m == nil || m.messageStore == nil {
		return nil
	}
	return m.messageStore.RequeueExpiredClaims(ctx)
}

func (m *Manager) GetMessage(ctx context.Context, messageID string) (types.AgentMessage, error) {
	if m == nil || m.messageStore == nil {
		return types.AgentMessage{}, fmt.Errorf("message store is not configured")
	}
	return m.messageStore.GetMessage(ctx, messageID)
}

func (m *Manager) ListMessages(ctx context.Context, filter state.MessageFilter) ([]types.AgentMessage, error) {
	if m == nil || m.messageStore == nil {
		return nil, fmt.Errorf("message store is not configured")
	}
	return m.messageStore.ListMessages(ctx, filter)
}

func (m *Manager) CountMessages(ctx context.Context, filter state.MessageFilter) (int, error) {
	if m == nil || m.messageStore == nil {
		return 0, fmt.Errorf("message store is not configured")
	}
	return m.messageStore.CountMessages(ctx, filter)
}

// RepairRoutingDrift scans for callback tasks with missing/broken routing fields and
// applies deterministic repairs. Returns number of updated tasks.
func (m *Manager) RepairRoutingDrift(ctx context.Context, limit int) (int, error) {
	if m == nil || m.oracle == nil {
		return 0, nil
	}
	if limit <= 0 {
		limit = 200
	}
	tasks, err := m.store.ListTasks(ctx, state.TaskFilter{
		Status: []types.TaskStatus{types.TaskStatusPending, types.TaskStatusReviewPending, types.TaskStatusActive},
		SortBy: "created_at",
		Limit:  limit,
	})
	if err != nil {
		return 0, err
	}
	updated := 0
	for _, task := range tasks {
		norm, changed, err := m.oracle.RepairTask(ctx, m.runLoader, task)
		if err != nil {
			continue
		}
		if !changed {
			continue
		}
		if err := m.store.UpdateTask(ctx, norm); err != nil {
			continue
		}
		updated++
		m.emitRoutingEvent(ctx, norm, "routing.repaired", "Routing drift repaired", map[string]string{
			"taskId": strings.TrimSpace(norm.TaskID),
			"teamId": strings.TrimSpace(norm.TeamID),
		})
		m.notifyWake(norm)
	}
	return updated, nil
}

func (m *Manager) emitRoutingEvent(ctx context.Context, task types.Task, typ, msg string, data map[string]string) {
	if m == nil || m.events == nil {
		return
	}
	evData := map[string]string{}
	for k, v := range data {
		evData[k] = strings.TrimSpace(v)
	}
	if evData["teamId"] == "" {
		evData["teamId"] = strings.TrimSpace(task.TeamID)
	}
	if evData["runId"] == "" {
		evData["runId"] = strings.TrimSpace(task.RunID)
	}
	_ = m.events.Append(ctx, types.EventRecord{
		EventID:   uuid.NewString(),
		RunID:     strings.TrimSpace(task.RunID),
		Type:      strings.TrimSpace(typ),
		Message:   strings.TrimSpace(msg),
		Data:      evData,
		Timestamp: time.Now().UTC(),
	})
}

func (m *Manager) notifyWakeForMessage(ctx context.Context, messageID string) {
	if m == nil || m.messageStore == nil {
		return
	}
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return
	}
	msg, err := m.messageStore.GetMessage(ctx, messageID)
	if err != nil {
		return
	}
	taskRef := strings.TrimSpace(msg.TaskRef)
	if taskRef == "" {
		return
	}
	task, err := m.store.GetTask(ctx, taskRef)
	if err != nil {
		return
	}
	m.notifyWake(task)
}

func (m *Manager) listTaskMessages(ctx context.Context, task types.Task, filter state.MessageFilter) ([]types.AgentMessage, error) {
	if m == nil || m.messageStore == nil {
		return nil, nil
	}
	msgs, err := m.messageStore.ListMessages(ctx, filter)
	if err != nil {
		return nil, err
	}
	if len(msgs) != 0 || strings.TrimSpace(filter.TeamID) == "" || strings.TrimSpace(filter.ThreadID) == "" {
		return msgs, nil
	}
	// Team fallback: allow cross-session role workers to locate the same authoritative message.
	relaxed := filter
	relaxed.ThreadID = ""
	return m.messageStore.ListMessages(ctx, relaxed)
}

// CloseBatchAndHandoff atomically closes a synthetic batch callback and creates
// one deterministic coordinator handoff task.
func (m *Manager) CloseBatchAndHandoff(ctx context.Context, batchTaskID, reviewerIdentity, reviewSummary string) (string, error) {
	if m == nil || m.store == nil {
		return "", fmt.Errorf("task manager is not configured")
	}
	batchTaskID = strings.TrimSpace(batchTaskID)
	if batchTaskID == "" {
		return "", fmt.Errorf("batchTaskID is required")
	}
	reviewerIdentity = strings.TrimSpace(reviewerIdentity)
	if reviewerIdentity == "" {
		reviewerIdentity = "reviewer"
	}
	preBatch, preErr := m.store.GetTask(ctx, batchTaskID)
	if preErr == nil && metadataBool(preBatch.Metadata, "batchClosed") {
		handoffTaskID := strings.TrimSpace(metadataString(preBatch.Metadata, "batchHandoffTaskId"))
		if handoffTaskID == "" {
			handoffTaskID = "review-handoff-" + batchTaskID
		}
		return handoffTaskID, nil
	}
	closer, ok := m.store.(batchCloseStore)
	if !ok {
		return "", fmt.Errorf("task store does not support atomic batch close")
	}
	handoffTaskID, approved, retried, escalated, err := closer.CloseBatchAndHandoffAtomic(ctx, batchTaskID, reviewerIdentity, strings.TrimSpace(reviewSummary))
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(handoffTaskID) == "" {
		return "", fmt.Errorf("atomic batch close did not return handoff task id")
	}
	closedBatch, batchErr := m.store.GetTask(ctx, batchTaskID)
	if m.events != nil {
		now := time.Now().UTC()
		decisionIDs := batchDecisionTaskIDs(closedBatch.Metadata)
		_ = m.events.Append(ctx, types.EventRecord{
			EventID:   uuid.NewString(),
			Type:      "callback.batch.closed",
			Message:   "Batch callback closed and coordinator handoff queued",
			Timestamp: now,
			Data: map[string]string{
				"taskId":         batchTaskID,
				"batchTaskId":    batchTaskID,
				"handoffTaskId":  strings.TrimSpace(handoffTaskID),
				"approved":       fmt.Sprintf("%d", approved),
				"retry":          fmt.Sprintf("%d", retried),
				"escalate":       fmt.Sprintf("%d", escalated),
				"reviewedBy":     reviewerIdentity,
				"closeTimestamp": now.Format(time.RFC3339Nano),
			},
		})
		receiptData := map[string]string{
			"taskId":            batchTaskID,
			"batchTaskId":       batchTaskID,
			"batchWaveId":       metadataString(closedBatch.Metadata, "batchWaveId"),
			"handoffTaskId":     strings.TrimSpace(handoffTaskID),
			"approved":          fmt.Sprintf("%d", approved),
			"retry":             fmt.Sprintf("%d", retried),
			"escalate":          fmt.Sprintf("%d", escalated),
			"reviewedBy":        reviewerIdentity,
			"batchCloseTxnId":   metadataString(closedBatch.Metadata, "batchCloseTxnId"),
			"reviewedItemCount": fmt.Sprintf("%d", len(decisionIDs)),
			"reviewedItemIds":   strings.Join(decisionIDs, ","),
			"receiptTimestamp":  now.Format(time.RFC3339Nano),
		}
		_ = m.events.Append(ctx, types.EventRecord{
			EventID:   uuid.NewString(),
			Type:      "review.closure.receipt",
			Message:   "Batch review closure receipt recorded",
			Timestamp: now,
			Data:      receiptData,
		})
	}
	if handoffTask, err := m.store.GetTask(ctx, strings.TrimSpace(handoffTaskID)); err == nil {
		m.notifyWake(handoffTask)
	}
	if batchErr == nil {
		m.notifyWake(closedBatch)
	} else if batchTask, err := m.store.GetTask(ctx, batchTaskID); err == nil {
		m.notifyWake(batchTask)
	}
	return handoffTaskID, nil
}

func metadataString(meta map[string]any, key string) string {
	if len(meta) == 0 {
		return ""
	}
	raw, ok := meta[key]
	if !ok || raw == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(raw))
}

func metadataBool(meta map[string]any, key string) bool {
	if len(meta) == 0 {
		return false
	}
	raw, ok := meta[key]
	if !ok || raw == nil {
		return false
	}
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	default:
		return strings.EqualFold(strings.TrimSpace(fmt.Sprint(v)), "true")
	}
}

func batchDecisionTaskIDs(meta map[string]any) []string {
	if len(meta) == 0 {
		return nil
	}
	raw, ok := meta["batchItemDecisions"]
	if !ok || raw == nil {
		return nil
	}
	decisions, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(decisions))
	for taskID := range decisions {
		taskID = strings.TrimSpace(taskID)
		if taskID == "" {
			continue
		}
		out = append(out, taskID)
	}
	sort.Strings(out)
	return out
}
