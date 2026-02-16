package hosttools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tinoosan/workbench-core/pkg/agent/state"
	llmtypes "github.com/tinoosan/workbench-core/pkg/llm/types"
	"github.com/tinoosan/workbench-core/pkg/types"
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
		TaskID         string          `json:"taskId"`
		Decision       string          `json:"decision"`
		Feedback       string          `json:"feedback"`
		EscalationData json.RawMessage `json:"escalationData"`
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

	// Verify this is a callback task with a review gate.
	if task.Metadata == nil {
		task.Metadata = map[string]any{}
	}
	source, _ := task.Metadata["source"].(string)
	if source != "subagent.callback" && source != "team.callback" {
		return types.HostOpRequest{}, fmt.Errorf("task_review: task %s is not a reviewable callback (source=%q)", taskID, source)
	}

	switch decision {
	case "approve":
		return t.handleApprove(ctx, task)
	case "retry":
		return t.handleRetry(ctx, task, feedback)
	case "escalate":
		return t.handleEscalate(ctx, task, feedback, payload.EscalationData)
	default:
		return types.HostOpRequest{}, fmt.Errorf("task_review: unknown decision %q", decision)
	}
}

func (t *TaskReviewTool) handleApprove(ctx context.Context, task types.Task) (types.HostOpRequest, error) {
	task.Metadata["reviewDecision"] = "approve"
	_ = t.Store.UpdateTask(ctx, task)

	return types.HostOpRequest{
		Op:   types.HostOpNoop,
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
		Op:   types.HostOpNoop,
		Tag:  "task_review",
		Text: fmt.Sprintf("Task %s escalated. Reason: %s", task.TaskID, feedback),
	}, nil
}
