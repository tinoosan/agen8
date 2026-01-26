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

			params := normalizeToolSchema(action.InputSchema)

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

func normalizeToolSchema(raw json.RawMessage) map[string]any {
	out := map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"additionalProperties": false,
	}
	if len(raw) == 0 {
		return out
	}
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return out
	}
	m, ok := parsed.(map[string]any)
	if !ok {
		return out
	}
	return normalizeObjectSchema(m)
}

func normalizeObjectSchema(schema map[string]any) map[string]any {
	if schema == nil {
		return map[string]any{
			"type":                 "object",
			"properties":           map[string]any{},
			"additionalProperties": false,
		}
	}
	if strings.TrimSpace(asString(schema["type"])) == "" {
		schema["type"] = "object"
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok || props == nil {
		props = map[string]any{}
		schema["properties"] = props
	}
	if req, ok := schema["required"].([]any); ok {
		req = filterRequired(req, props)
		if len(req) > 0 {
			schema["required"] = req
		} else {
			delete(schema, "required")
		}
	}
	schema["additionalProperties"] = false
	return schema
}

func filterRequired(req []any, props map[string]any) []any {
	out := make([]any, 0, len(req))
	for _, v := range req {
		s, ok := v.(string)
		if !ok {
			continue
		}
		if _, exists := props[s]; !exists {
			continue
		}
		out = append(out, s)
	}
	return out
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}
