package harness

import (
	"context"
	"strings"

	"github.com/tinoosan/agen8/pkg/types"
)

const (
	// NativeAdapterID identifies the built-in agen8 task runner.
	NativeAdapterID = "agen8-native"
	// DefaultHarnessEnvVar controls the process-wide default harness selection.
	DefaultHarnessEnvVar = "AGEN8_DEFAULT_HARNESS"
)

// TaskRequest is the adapter-level unit of work.
type TaskRequest struct {
	TaskID    string
	RunID     string
	SessionID string
	Goal      string
	TaskKind  string
	Metadata  map[string]any
	Workdir   string
}

// TaskResult is the adapter-level execution result.
type TaskResult struct {
	Status       types.TaskStatus
	Text         string
	Error        string
	Artifacts    []string
	InputTokens  int
	OutputTokens int
	TotalTokens  int
	CostUSD      float64
	AdapterRunID string
}

// Adapter executes a task for a specific harness implementation.
type Adapter interface {
	ID() string
	RunTask(ctx context.Context, req TaskRequest) (TaskResult, error)
}

// NormalizeResult ensures task result invariants that callers rely on.
func NormalizeResult(in TaskResult) TaskResult {
	out := in
	if out.TotalTokens <= 0 {
		out.TotalTokens = out.InputTokens + out.OutputTokens
	}
	status := types.TaskStatus(strings.ToLower(strings.TrimSpace(string(out.Status))))
	switch status {
	case types.TaskStatusSucceeded, types.TaskStatusFailed, types.TaskStatusCanceled:
		out.Status = status
	default:
		if strings.TrimSpace(out.Error) != "" {
			out.Status = types.TaskStatusFailed
		} else {
			out.Status = types.TaskStatusSucceeded
		}
	}
	return out
}
