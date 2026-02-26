package harness

import (
	"context"
	"fmt"
)

// RunTaskFunc executes a task using the native agen8 runner.
type RunTaskFunc func(ctx context.Context, req TaskRequest) (TaskResult, error)

// NativeAdapter delegates to a caller-provided native task runner implementation.
type NativeAdapter struct {
	runTask RunTaskFunc
}

func NewNativeAdapter(runTask RunTaskFunc) *NativeAdapter {
	return &NativeAdapter{runTask: runTask}
}

func (a *NativeAdapter) ID() string { return NativeAdapterID }

func (a *NativeAdapter) RunTask(ctx context.Context, req TaskRequest) (TaskResult, error) {
	if a == nil || a.runTask == nil {
		return TaskResult{}, fmt.Errorf("native adapter is not configured")
	}
	result, err := a.runTask(ctx, req)
	if err != nil {
		return TaskResult{}, err
	}
	return NormalizeResult(result), nil
}
