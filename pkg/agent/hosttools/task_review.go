package hosttools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8/pkg/agent/state"
	llmtypes "github.com/tinoosan/agen8/pkg/llm/types"
	"github.com/tinoosan/agen8/pkg/types"
)

// EscalationData carries structured information when a parent escalates a sub-agent task.
type EscalationData struct {
	Reason         string   `json:"reason"`
	AttemptSummary string   `json:"attemptSummary"`
	Recommendation string   `json:"recommendation"`
	Artifacts      []string `json:"artifacts,omitempty"`
	OriginalGoal   string   `json:"originalGoal"`
	RetryCount     int      `json:"retryCount"`
	SourceRunID    string   `json:"sourceRunId"`
	SourceTaskID   string   `json:"sourceTaskId"`
}

// ReviewSupervisor is implemented by the daemon runtime supervisor to handle
// retry and escalation operations triggered by the task_review tool.
type ReviewSupervisor interface {
	RetrySubagent(ctx context.Context, childRunID string, feedback string) error
	EscalateTask(ctx context.Context, taskID string, data EscalationData) error
}

// TaskReviewTool implements the task_review host tool, providing a formal
// review gate for sub-agent work. The parent must approve, retry, or escalate.
type TaskReviewTool struct {
	Store      state.TaskStore
	SessionID  string
	RunID      string
	Supervisor ReviewSupervisor
}

type batchCloseAndHandoffCloser interface {
	CloseBatchAndHandoff(ctx context.Context, batchTaskID, reviewerIdentity, reviewSummary string) (string, error)
}

func (t *TaskReviewTool) Definition() llmtypes.Tool {
	return llmtypes.Tool{
		Type: "function",
		Function: llmtypes.ToolFunction{
			Name:        "task_review",
			Description: "[TASKS] Review a sub-agent's completed work. You must approve, retry (with feedback), or escalate.",
			Strict:      false,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"taskId": map[string]any{
						"type":        "string",
						"description": "The callback task ID being reviewed.",
					},
					"decision": map[string]any{
						"type":        "string",
						"enum":        []string{"approve", "retry", "escalate"},
						"description": "Review decision: approve the work, retry with feedback, or escalate.",
					},
					"feedback": map[string]any{
						"type":        "string",
						"description": "Reason for retry or escalation. Required for retry and escalate decisions.",
					},
					"batchItemTaskId": map[string]any{
						"type":        "string",
						"description": "Optional child callback task ID when reviewing a synthetic batch callback.",
					},
					"escalationData": map[string]any{
						"type":        "object",
						"description": "Structured escalation payload (for escalate decision).",
						"properties": map[string]any{
							"reason":         map[string]any{"type": "string"},
							"attemptSummary": map[string]any{"type": "string"},
							"recommendation": map[string]any{"type": "string"},
							"artifacts":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						},
						"additionalProperties": true,
					},
				},
				"required":             []any{"taskId", "decision"},
				"additionalProperties": false,
			},
		},
	}
}

func (t *TaskReviewTool) Execute(ctx context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	if t == nil || t.Store == nil {
		return types.HostOpRequest{}, fmt.Errorf("task_review: store is not configured")
	}

	var payload struct {
		TaskID          string          `json:"taskId"`
		Decision        string          `json:"decision"`
		Feedback        string          `json:"feedback"`
		BatchItemTaskID string          `json:"batchItemTaskId"`
		EscalationData  json.RawMessage `json:"escalationData"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return types.HostOpRequest{}, fmt.Errorf("task_review: %w", err)
	}

	taskID := strings.TrimSpace(payload.TaskID)
	if taskID == "" {
		return types.HostOpRequest{}, fmt.Errorf("task_review: taskId is required")
	}
	decision := strings.ToLower(strings.TrimSpace(payload.Decision))
	if decision != "approve" && decision != "retry" && decision != "escalate" {
		return types.HostOpRequest{}, fmt.Errorf("task_review: decision must be approve, retry, or escalate")
	}
	feedback := strings.TrimSpace(payload.Feedback)
	if (decision == "retry" || decision == "escalate") && feedback == "" {
		return types.HostOpRequest{}, fmt.Errorf("task_review: feedback is required for %s decisions", decision)
	}

	task, err := t.Store.GetTask(ctx, taskID)
	if err != nil {
		return types.HostOpRequest{}, fmt.Errorf("task_review: get task: %w", err)
	}

	// Verify this is a callback task with a review gate, or resolve delegated task to callback.
	if task.Metadata == nil {
		task.Metadata = map[string]any{}
	}
	source, _ := task.Metadata["source"].(string)
	if source == "spawn_worker" {
		// Agent passed the delegated task ID; resolve to the callback task for review.
		callbackTaskID := "callback-" + taskID
		callbackTask, err := t.Store.GetTask(ctx, callbackTaskID)
		if err != nil {
			return types.HostOpRequest{}, fmt.Errorf("task_review: no callback task found for %s (looked for %s): %w", taskID, callbackTaskID, err)
		}
		if callbackTask.Metadata == nil {
			callbackTask.Metadata = map[string]any{}
		}
		cbSource, _ := callbackTask.Metadata["source"].(string)
		if cbSource != "subagent.callback" {
			return types.HostOpRequest{}, fmt.Errorf("task_review: task %s is a delegated task but callback %s is not a subagent callback (source=%q). Pass the callback task ID (callback-<taskId>) or the delegated task ID.", taskID, callbackTaskID, cbSource)
		}
		task = callbackTask
	} else if source != "subagent.callback" && source != "team.callback" && source != "subagent.batch.callback" && source != "team.batch.callback" {
		return types.HostOpRequest{}, fmt.Errorf("task_review: task %s is not a reviewable callback (source=%q). Pass the callback task ID (e.g. callback-<taskId>) or the delegated task ID for the sub-agent work you want to review.", taskID, source)
	}

	source = strings.TrimSpace(source)
	if source == "subagent.batch.callback" || source == "team.batch.callback" {
		return t.handleBatchDecision(ctx, task, strings.TrimSpace(payload.BatchItemTaskID), decision, feedback, payload.EscalationData)
	}
	return t.handleDecision(ctx, task, decision, feedback, payload.EscalationData)
}

func (t *TaskReviewTool) handleDecision(ctx context.Context, task types.Task, decision, feedback string, rawEscData json.RawMessage) (types.HostOpRequest, error) {
	switch decision {
	case "approve":
		return t.handleApprove(ctx, task)
	case "retry":
		return t.handleRetry(ctx, task, feedback)
	case "escalate":
		return t.handleEscalate(ctx, task, feedback, rawEscData)
	default:
		return types.HostOpRequest{}, fmt.Errorf("task_review: unknown decision %q", decision)
	}
}

func (t *TaskReviewTool) handleBatchDecision(ctx context.Context, batchTask types.Task, batchItemTaskID, decision, feedback string, rawEscData json.RawMessage) (types.HostOpRequest, error) {
	if batchItemTaskID == "" {
		return types.HostOpRequest{}, fmt.Errorf("task_review: batchItemTaskId is required when reviewing a batch callback")
	}
	if batchTask.Inputs == nil {
		return types.HostOpRequest{}, fmt.Errorf("task_review: batch callback %s has no items", batchTask.TaskID)
	}
	itemsRaw, ok := batchTask.Inputs["items"].([]any)
	if !ok || len(itemsRaw) == 0 {
		return types.HostOpRequest{}, fmt.Errorf("task_review: batch callback %s has no items", batchTask.TaskID)
	}
	itemIdx := -1
	for i, raw := range itemsRaw {
		item, _ := raw.(map[string]any)
		if strings.TrimSpace(fmt.Sprint(item["callbackTaskId"])) == batchItemTaskID {
			itemIdx = i
			break
		}
	}
	if itemIdx < 0 {
		return types.HostOpRequest{}, fmt.Errorf("task_review: batch item %s is not part of callback %s", batchItemTaskID, batchTask.TaskID)
	}

	childTask, err := t.Store.GetTask(ctx, batchItemTaskID)
	if err != nil {
		return types.HostOpRequest{}, fmt.Errorf("task_review: get batch item callback: %w", err)
	}
	if childTask.Metadata == nil {
		childTask.Metadata = map[string]any{}
	}
	childSource := strings.TrimSpace(fmt.Sprint(childTask.Metadata["source"]))
	if childSource != "subagent.callback" && childSource != "team.callback" {
		return types.HostOpRequest{}, fmt.Errorf("task_review: batch item %s is not a callback task (source=%q)", batchItemTaskID, childSource)
	}
	if _, err := t.handleDecision(ctx, childTask, decision, feedback, rawEscData); err != nil {
		return types.HostOpRequest{}, err
	}
	childTask.Metadata["batchItemStatus"] = batchItemStatusFromDecision(decision)
	childTask.Metadata["batchReviewedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
	reviewerIdentity := strings.TrimSpace(batchTask.AssignedRole)
	if reviewerIdentity == "" {
		reviewerIdentity = strings.TrimSpace(batchTask.AssignedTo)
	}
	if reviewerIdentity == "" {
		reviewerIdentity = strings.TrimSpace(t.RunID)
	}
	childTask.Metadata["batchReviewedBy"] = reviewerIdentity
	if feedback != "" {
		childTask.Metadata["batchDecisionNote"] = feedback
	}
	_ = t.Store.UpdateTask(ctx, childTask)

	if batchTask.Metadata == nil {
		batchTask.Metadata = map[string]any{}
	}
	decisions, _ := batchTask.Metadata["batchItemDecisions"].(map[string]any)
	if decisions == nil {
		decisions = map[string]any{}
	}
	decisions[batchItemTaskID] = decision
	batchTask.Metadata["batchItemDecisions"] = decisions

	itemsRaw[itemIdx] = mergeBatchItemDecision(itemsRaw[itemIdx], decision)
	batchTask.Inputs["items"] = itemsRaw
	batchTask.Metadata["batchReviewedCount"] = float64(len(decisions))
	batchTask.Metadata["batchTotalItems"] = float64(len(itemsRaw))
	reviewComplete := len(decisions) >= len(itemsRaw)
	batchTask.Metadata["batchReviewComplete"] = reviewComplete
	_ = t.Store.UpdateTask(ctx, batchTask)

	approved, retried, escalated := countBatchDecisions(decisions)
	handoffTaskID := ""
	if reviewComplete {
		summary := fmt.Sprintf("Batch review complete: approved=%d retry=%d escalate=%d.", approved, retried, escalated)
		if closer, ok := t.Store.(batchCloseAndHandoffCloser); ok {
			hid, cerr := closer.CloseBatchAndHandoff(ctx, strings.TrimSpace(batchTask.TaskID), reviewerIdentity, summary)
			if cerr != nil {
				return types.HostOpRequest{}, fmt.Errorf("task_review: close batch and handoff: %w", cerr)
			}
			handoffTaskID = strings.TrimSpace(hid)
		}
	}
	if handoffTaskID != "" {
		return types.HostOpRequest{
			Op:   types.HostOpToolResult,
			Tag:  "task_review",
			Text: fmt.Sprintf("Batch callback %s: item %s set to %s (approved=%d retry=%d escalate=%d). Handoff queued to coordinator as %s.", batchTask.TaskID, batchItemTaskID, decision, approved, retried, escalated, handoffTaskID),
		}, nil
	}
	return types.HostOpRequest{
		Op:   types.HostOpToolResult,
		Tag:  "task_review",
		Text: fmt.Sprintf("Batch callback %s: item %s set to %s (approved=%d retry=%d escalate=%d).", batchTask.TaskID, batchItemTaskID, decision, approved, retried, escalated),
	}, nil
}

func mergeBatchItemDecision(raw any, decision string) map[string]any {
	item, _ := raw.(map[string]any)
	if item == nil {
		item = map[string]any{}
	}
	item["decision"] = strings.TrimSpace(decision)
	return item
}

func countBatchDecisions(decisions map[string]any) (approved, retried, escalated int) {
	for _, raw := range decisions {
		switch strings.ToLower(strings.TrimSpace(fmt.Sprint(raw))) {
		case "approve":
			approved++
		case "retry":
			retried++
		case "escalate":
			escalated++
		}
	}
	return approved, retried, escalated
}

func batchItemStatusFromDecision(decision string) string {
	switch strings.ToLower(strings.TrimSpace(decision)) {
	case "approve":
		return "approved"
	case "retry":
		return "retry"
	case "escalate":
		return "escalated"
	default:
		return "pending_review"
	}
}

func (t *TaskReviewTool) handleApprove(ctx context.Context, task types.Task) (types.HostOpRequest, error) {
	task.Metadata["reviewDecision"] = "approve"
	_ = t.Store.UpdateTask(ctx, task)

	return types.HostOpRequest{
		Op:   types.HostOpToolResult,
		Tag:  "task_review",
		Text: fmt.Sprintf("Task %s approved. Sub-agent work accepted and child run will be cleaned up.", task.TaskID),
	}, nil
}

func (t *TaskReviewTool) handleRetry(ctx context.Context, task types.Task, feedback string) (types.HostOpRequest, error) {
	// Check retry budget.
	retryBudget := 3
	if rb, ok := task.Metadata["retryBudget"].(float64); ok {
		retryBudget = int(rb)
	}
	retryCount := 0
	if rc, ok := task.Metadata["retryCount"].(float64); ok {
		retryCount = int(rc)
	}
	if retryCount >= retryBudget {
		return types.HostOpRequest{}, fmt.Errorf("task_review: retry budget exhausted (%d/%d); consider escalating instead", retryCount, retryBudget)
	}

	childRunID, _ := task.Metadata["sourceRunId"].(string)
	childRunID = strings.TrimSpace(childRunID)
	if childRunID == "" {
		return types.HostOpRequest{}, fmt.Errorf("task_review: no sourceRunId in callback metadata")
	}

	if t.Supervisor == nil {
		return types.HostOpRequest{}, fmt.Errorf("task_review: supervisor not configured for retry")
	}
	if err := t.Supervisor.RetrySubagent(ctx, childRunID, feedback); err != nil {
		return types.HostOpRequest{}, fmt.Errorf("task_review: retry: %w", err)
	}

	// Update retry count and decision in callback metadata.
	task.Metadata["retryCount"] = float64(retryCount + 1)
	task.Metadata["reviewDecision"] = "retry"
	_ = t.Store.UpdateTask(ctx, task)

	return types.HostOpRequest{
		Op:   types.HostOpNoop,
		Tag:  "task_review",
		Text: fmt.Sprintf("Task %s retry initiated (attempt %d/%d). Feedback sent to sub-agent.", task.TaskID, retryCount+1, retryBudget),
	}, nil
}

func (t *TaskReviewTool) handleEscalate(ctx context.Context, task types.Task, feedback string, rawEscData json.RawMessage) (types.HostOpRequest, error) {
	if t.Supervisor == nil {
		return types.HostOpRequest{}, fmt.Errorf("task_review: supervisor not configured for escalation")
	}

	retryCount := 0
	if rc, ok := task.Metadata["retryCount"].(float64); ok {
		retryCount = int(rc)
	}
	sourceRunID, _ := task.Metadata["sourceRunId"].(string)
	sourceTaskID, _ := task.Metadata["callbackForTaskId"].(string)
	originalGoal := ""
	if inputs, ok := task.Inputs["sourceGoal"].(string); ok {
		originalGoal = inputs
	}

	escData := EscalationData{
		Reason:       feedback,
		OriginalGoal: strings.TrimSpace(originalGoal),
		RetryCount:   retryCount,
		SourceRunID:  strings.TrimSpace(sourceRunID),
		SourceTaskID: strings.TrimSpace(sourceTaskID),
	}

	// Overlay any structured data from the caller.
	if len(rawEscData) > 0 {
		var overlay struct {
			AttemptSummary string   `json:"attemptSummary"`
			Recommendation string   `json:"recommendation"`
			Artifacts      []string `json:"artifacts"`
		}
		if err := json.Unmarshal(rawEscData, &overlay); err == nil {
			if s := strings.TrimSpace(overlay.AttemptSummary); s != "" {
				escData.AttemptSummary = s
			}
			if s := strings.TrimSpace(overlay.Recommendation); s != "" {
				escData.Recommendation = s
			}
			if len(overlay.Artifacts) > 0 {
				escData.Artifacts = overlay.Artifacts
			}
		}
	}

	if err := t.Supervisor.EscalateTask(ctx, task.TaskID, escData); err != nil {
		return types.HostOpRequest{}, fmt.Errorf("task_review: escalate: %w", err)
	}

	task.Metadata["reviewDecision"] = "escalate"
	_ = t.Store.UpdateTask(ctx, task)

	return types.HostOpRequest{
		Op:   types.HostOpToolResult,
		Tag:  "task_review",
		Text: fmt.Sprintf("Task %s escalated. Reason: %s", task.TaskID, feedback),
	}, nil
}
