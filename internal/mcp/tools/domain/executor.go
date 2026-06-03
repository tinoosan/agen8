package domain

import (
	"context"
	"encoding/json"
)

// ToolExecutor executes a named tool and returns its final result envelope.
type ToolExecutor interface {
	Execute(ctx context.Context, name string, args json.RawMessage) (ToolResult, error)
}
