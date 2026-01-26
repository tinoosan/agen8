package agent

import (
	"context"
	"encoding/json"

	"github.com/tinoosan/workbench-core/pkg/llm"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// HostTool defines a single host tool that can be exposed to the LLM.
// Tools are responsible for both schema definition and input decoding.
type HostTool interface {
	// Definition returns the LLM tool schema (name, description, params).
	Definition() llm.Tool
	// Execute converts raw JSON arguments into a HostOpRequest.
	Execute(ctx context.Context, args json.RawMessage) (types.HostOpRequest, error)
}
