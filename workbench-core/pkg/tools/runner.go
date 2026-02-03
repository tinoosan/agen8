package tools

import (
	"context"
	"encoding/json"
)

type ToolInvoker interface {
	Invoke(ctx context.Context, req ToolRequest) (ToolCallResult, error)
}

type InvokeError struct {
	Code      string
	Message   string
	Retryable bool
	Err       error
}

func (e *InvokeError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "tool invoke error"
}

func (e *InvokeError) Unwrap() error { return e.Err }

type ToolCallResult struct {
	Output json.RawMessage
}
