package hosttools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tinoosan/agen8/pkg/types"
)

func TestFSStatTool_DefinitionRequiresPath(t *testing.T) {
	tool := (&FSStatTool{}).Definition()
	params, ok := tool.Function.Parameters.(map[string]any)
	if !ok {
		t.Fatalf("expected parameters to be a map, got %T", tool.Function.Parameters)
	}
	req, ok := params["required"].([]any)
	if !ok {
		t.Fatalf("expected required to be []any, got %T", params["required"])
	}
	if len(req) != 1 || req[0] != "path" {
		t.Fatalf("expected required=[path], got %#v", req)
	}
}

func TestFSStatTool_ExecuteMapsToHostOpRequest(t *testing.T) {
	args := json.RawMessage(`{"path":"README.md"}`)
	req, err := (&FSStatTool{}).Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if req.Op != types.HostOpFSStat {
		t.Fatalf("op = %q, want %q", req.Op, types.HostOpFSStat)
	}
	if req.Path != "/project/README.md" {
		t.Fatalf("path = %q, want %q", req.Path, "/project/README.md")
	}
}
