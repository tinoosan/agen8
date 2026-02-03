package agent

import (
	"context"
	"encoding/json"

	llmtypes "github.com/tinoosan/workbench-core/pkg/llm/types"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// HostTool defines a single host tool that can be exposed to the LLM.
// Tools are responsible for both schema definition and input decoding.
type HostTool interface {
	// Definition returns the LLM tool schema (name, description, params).
	Definition() llmtypes.Tool
	// Execute converts raw JSON arguments into a HostOpRequest.
	Execute(ctx context.Context, args json.RawMessage) (types.HostOpRequest, error)
}
