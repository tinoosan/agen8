package domain

import (
	"context"
	"encoding/json"

	humaninput "github.com/tinoosan/agen8-mcp-server/internal/services/humaninput/domain"
)

// HumanInputTool is an optional extension for tools that can park the loop and
// request operator input before returning a result.
type HumanInputTool interface {
	DeclareHumanInput(ctx context.Context, args json.RawMessage) (humaninput.Declaration, bool, error)
	ResolveHumanInput(ctx context.Context, args json.RawMessage, result json.RawMessage) (ToolResult, error)
}
