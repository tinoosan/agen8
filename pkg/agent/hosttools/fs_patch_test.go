package hosttools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tinoosan/agen8/pkg/types"
)

func TestFSPatchTool_DefinitionOptionalDryRunVerbose(t *testing.T) {
	tool := (&FSPatchTool{}).Definition()
	params, ok := tool.Function.Parameters.(map[string]any)
	if !ok {
		t.Fatalf("expected parameters to be a map, got %T", tool.Function.Parameters)
	}
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties to be a map, got %T", params["properties"])
	}
	if _, ok := props["dryRun"]; !ok {
		t.Fatalf("expected dryRun property in schema")
	}
	if _, ok := props["verbose"]; !ok {
		t.Fatalf("expected verbose property in schema")
	}
	req, ok := params["required"].([]any)
	if !ok {
		t.Fatalf("expected required to be []any, got %T", params["required"])
	}
	if len(req) != 2 || req[0] != "path" || req[1] != "text" {
		t.Fatalf("expected required=[path text], got %#v", req)
	}
}

func TestFSPatchTool_ExecuteMapsToHostOpRequest(t *testing.T) {
	args := json.RawMessage(`{"path":"README.md","text":"@@ -1 +1 @@\n-old\n+new\n","dryRun":true,"verbose":true}`)
	req, err := (&FSPatchTool{}).Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if req.Op != types.HostOpFSPatch {
		t.Fatalf("op = %q, want %q", req.Op, types.HostOpFSPatch)
	}
	if req.Path != "/project/README.md" {
		t.Fatalf("path = %q, want %q", req.Path, "/project/README.md")
	}
	if req.Text == "" {
		t.Fatalf("expected patch text to be mapped")
	}
	if !req.DryRun || !req.Verbose {
		t.Fatalf("expected dryRun+verbose true, got %+v", req)
	}
}
