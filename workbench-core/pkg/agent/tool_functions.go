package agent

import (
	"encoding/json"
	"strings"

	"github.com/tinoosan/workbench-core/pkg/llm"
	pkgtools "github.com/tinoosan/workbench-core/pkg/tools"
)

const defaultToolFunctionTimeoutMs = 30_000

// ToolRoute maps a function name back to the originating tool/action.
type ToolRoute struct {
	ToolID    pkgtools.ToolID
	ActionID  string
	TimeoutMs int
}

// ManifestToFunctionTools converts tool manifests into function tools and routing metadata.
func ManifestToFunctionTools(manifests []pkgtools.ToolManifest) (tools []llm.Tool, routes map[string]ToolRoute) {
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
					params = json.RawMessage(action.InputSchema)
				}
			}

			tools = append(tools, llm.Tool{
				Type: "function",
				Function: llm.ToolFunction{
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
