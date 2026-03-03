package task

import (
	"context"
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
	store     state.TaskStore
	runLoader RunLoader
	oracle    *RoutingOracle
	events    taskEventAppender

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

// NewManager creates a new task service manager. runLoader may be nil and set later via SetRunLoader.
func NewManager(store state.TaskStore, runLoader RunLoader) *Manager {
	return &Manager{
		store:     store,
		runLoader: runLoader,
		watchers:  map[string]taskWakeWatcher{},
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
		return fmt.Errorf("run loader not configured")
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
	tasks, err := m.store.ListTasks(ctx, state.TaskFilter{
		RunID:  runID,
		Status: []types.TaskStatus{types.TaskStatusActive},
		Limit:  500,
	})
	if err != nil {
		return 0, err
	}
	doneAt := time.Now().UTC()
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
			return len(tasks), err
		}
	}
	return len(tasks), nil
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
	m.emitRoutingEvent(ctx, task, "routing.validated", "Task routing validated", map[string]string{
		"taskId":       strings.TrimSpace(task.TaskID),
		"assignedType": strings.TrimSpace(task.AssignedToType),
		"assignedTo":   strings.TrimSpace(task.AssignedTo),
		"teamId":       strings.TrimSpace(task.TeamID),
	})
	m.notifyWake(task)
	return nil
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
	if m.oracle != nil {
		current, err := m.store.GetTask(ctx, taskID)
		if err != nil {
			return err
		}
		if err := m.oracle.ValidateCompletion(ctx, m.runLoader, current); err != nil {
			return err
		}
	}
	if err := m.store.CompleteTask(ctx, taskID, result); err != nil {
		return err
	}
	updated, err := m.store.GetTask(ctx, taskID)
	if err == nil {
		m.notifyWake(updated)
	}
	return nil
}

func (m *Manager) ClaimTask(ctx context.Context, taskID string, ttl time.Duration) error {
	if err := m.store.ClaimTask(ctx, taskID, ttl); err != nil {
		return err
	}
	if task, err := m.store.GetTask(ctx, taskID); err == nil {
		m.notifyWake(task)
	}
	return nil
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
