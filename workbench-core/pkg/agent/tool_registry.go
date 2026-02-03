package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	llmtypes "github.com/tinoosan/workbench-core/pkg/llm/types"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// HostToolRegistry stores host tools.
type HostToolRegistry struct {
	tools map[string]HostTool
}

// NewHostToolRegistry constructs an empty registry.
func NewHostToolRegistry() *HostToolRegistry {
	return &HostToolRegistry{
		tools: make(map[string]HostTool),
	}
}

// Clone returns a shallow copy of the registry with copies of the maps so mutations
// on the clone do not affect the original.
func (r *HostToolRegistry) Clone() *HostToolRegistry {
	if r == nil {
		return nil
	}
	out := NewHostToolRegistry()
	for k, v := range r.tools {
		out.tools[k] = v
	}
	return out
}

// Register adds a HostTool to the registry by its Definition() name.
func (r *HostToolRegistry) Register(tool HostTool) error {
	if r == nil {
		return fmt.Errorf("tool registry is nil")
	}
	if tool == nil {
		return fmt.Errorf("tool is nil")
	}
	def := tool.Definition()
	name := strings.TrimSpace(def.Function.Name)
	if name == "" {
		return fmt.Errorf("tool name is required")
	}
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("duplicate tool name %q", name)
	}
	r.tools[name] = tool
	return nil
}

// Definitions returns all registered tool definitions.
func (r *HostToolRegistry) Definitions() []llmtypes.Tool {
	if r == nil {
		return nil
	}
	out := make([]llmtypes.Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		out = append(out, tool.Definition())
	}
	return out
}

// Dispatch resolves a tool call to a HostOpRequest.
func (r *HostToolRegistry) Dispatch(ctx context.Context, name string, args json.RawMessage) (types.HostOpRequest, error) {
	if r == nil {
		return types.HostOpRequest{}, fmt.Errorf("tool registry is nil")
	}
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return types.HostOpRequest{}, fmt.Errorf("tool name is required")
	}
	if tool, ok := r.tools[trimmed]; ok {
		return tool.Execute(ctx, args)
	}
	return types.HostOpRequest{}, fmt.Errorf("unknown tool function %q", trimmed)
}
