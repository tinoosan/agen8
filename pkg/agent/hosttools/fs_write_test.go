package hosttools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tinoosan/agen8/pkg/types"
)

func TestFSWriteTool_DefinitionOptionalWriteVerifyFlags(t *testing.T) {
	tool := (&FSWriteTool{}).Definition()
	params, ok := tool.Function.Parameters.(map[string]any)
	if !ok {
		t.Fatalf("expected parameters to be a map, got %T", tool.Function.Parameters)
	}
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties to be a map, got %T", params["properties"])
	}
	for _, key := range []string{"verify", "checksum", "atomic", "sync"} {
		if _, ok := props[key]; !ok {
			t.Fatalf("expected %s property in schema", key)
		}
	}
	req, ok := params["required"].([]any)
	if !ok {
		t.Fatalf("expected required to be []any, got %T", params["required"])
	}
	if len(req) != 2 || req[0] != "path" || req[1] != "text" {
		t.Fatalf("expected required=[path text], got %#v", req)
	}
}

func TestFSWriteTool_ExecuteMapsWriteVerifyFlags(t *testing.T) {
	args := json.RawMessage(`{"path":"README.md","text":"hello","verify":true,"checksum":"sha256","atomic":true,"sync":true}`)
	req, err := (&FSWriteTool{}).Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if req.Op != types.HostOpFSWrite {
		t.Fatalf("op = %q, want %q", req.Op, types.HostOpFSWrite)
	}
	if req.Path != "/project/README.md" {
		t.Fatalf("path = %q, want %q", req.Path, "/project/README.md")
	}
	if req.Text != "hello" {
		t.Fatalf("text = %q, want %q", req.Text, "hello")
	}
	if !req.Verify || req.Checksum != "sha256" || !req.Atomic || !req.Sync {
		t.Fatalf("unexpected mapped flags %+v", req)
	}
}
