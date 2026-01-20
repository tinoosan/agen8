package agent

import (
	"encoding/json"
	"strings"

	"github.com/tinoosan/workbench-core/internal/types"
)

const defaultToolFunctionTimeoutMs = 30_000

// ToolRoute maps a function name back to the originating tool/action.
type ToolRoute struct {
	ToolID    types.ToolID
	ActionID  string
	TimeoutMs int
}

// ManifestToFunctionTools converts tool manifests into function tools and routing metadata.
//
// Only manifests with ExposeAsFunctions=true are converted. Each action becomes a function
// named "<toolId>_<actionId>" with dots replaced by underscores for compatibility.
func ManifestToFunctionTools(manifests []types.ToolManifest) (tools []types.Tool, routes map[string]ToolRoute) {
	routes = make(map[string]ToolRoute)

	for _, m := range manifests {
		if !m.ExposeAsFunctions {
			continue
		}
		toolPrefix := strings.ReplaceAll(m.ID.String(), ".", "_")
		for _, action := range m.Actions {
			fnName := toolPrefix + "_" + strings.ReplaceAll(action.ID.String(), ".", "_")

			desc := strings.TrimSpace(action.Description)
			if desc == "" {
				desc = strings.TrimSpace(action.DisplayName)
			}

			var params any = map[string]any{"type": "object"}
			if len(action.InputSchema) != 0 {
				var parsed any
				if err := json.Unmarshal(action.InputSchema, &parsed); err == nil {
					params = parsed
				} else {
					// Fall back to raw schema bytes if it can't be parsed into a Go value.
					params = json.RawMessage(action.InputSchema)
				}
			}

			tools = append(tools, types.Tool{
				Type: "function",
				Function: types.ToolFunction{
					Name:        fnName,
					Description: desc,
					Parameters:  params,
					Strict:      false,
				},
			})

			if _, exists := routes[fnName]; !exists {
				routes[fnName] = ToolRoute{
					ToolID:    m.ID,
					ActionID:  action.ID.String(),
					TimeoutMs: defaultToolFunctionTimeoutMs,
				}
			}
		}
	}
	return tools, routes
}
