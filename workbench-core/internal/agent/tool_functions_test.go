package agent

import (
	"encoding/json"
	"testing"

	"github.com/tinoosan/workbench-core/internal/types"
)

func TestManifestToFunctionTools_ExposesFunctionsAndRoutes(t *testing.T) {
	manifest := types.ToolManifest{
		ID:                types.ToolID("builtin.shell"),
		Version:           "0.1.0",
		Kind:              types.ToolKindBuiltin,
		DisplayName:       "Builtin Shell",
		Description:       "shell",
		ExposeAsFunctions: true,
		Actions: []types.ToolAction{
			{
				ID:           types.ActionID("exec"),
				DisplayName:  "Exec",
				Description:  "run",
				InputSchema:  json.RawMessage(`{"type":"object"}`),
				OutputSchema: json.RawMessage(`{"type":"object"}`),
			},
		},
	}

	functions, routes := ManifestToFunctionTools([]types.ToolManifest{manifest})
	if len(functions) != 1 {
		t.Fatalf("expected 1 function tool, got %d", len(functions))
	}
	fn := functions[0].Function
	if fn.Name != "builtin_shell_exec" {
		t.Fatalf("unexpected function name %q", fn.Name)
	}
	route, ok := routes[fn.Name]
	if !ok {
		t.Fatalf("route for %q missing", fn.Name)
	}
	if route.ToolID != manifest.ID {
		t.Fatalf("unexpected route toolId %q", route.ToolID)
	}
	if route.ActionID != "exec" {
		t.Fatalf("unexpected route actionId %q", route.ActionID)
	}
	if route.TimeoutMs != defaultToolFunctionTimeoutMs {
		t.Fatalf("unexpected timeout %d", route.TimeoutMs)
	}
}
