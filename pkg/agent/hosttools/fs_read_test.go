package hosttools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tinoosan/agen8/pkg/types"
)

func TestFSReadTool_DefinitionIncludesChecksum(t *testing.T) {
	tool := (&FSReadTool{}).Definition()
	params, ok := tool.Function.Parameters.(map[string]any)
	if !ok {
		t.Fatalf("expected parameters map, got %T", tool.Function.Parameters)
	}
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties map, got %T", params["properties"])
	}
	if _, ok := props["checksum"]; !ok {
		t.Fatalf("expected checksum property in schema")
	}
}

func TestFSReadTool_ExecuteMapsChecksumString(t *testing.T) {
	args := json.RawMessage(`{"path":"README.md","checksum":"sha256"}`)
	req, err := (&FSReadTool{}).Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if req.Op != types.HostOpFSRead {
		t.Fatalf("op = %q, want %q", req.Op, types.HostOpFSRead)
	}
	if req.Path != "/project/README.md" {
		t.Fatalf("path = %q, want %q", req.Path, "/project/README.md")
	}
	if len(req.Checksums) != 1 || req.Checksums[0] != "sha256" {
		t.Fatalf("checksums=%v want [sha256]", req.Checksums)
	}
}

func TestFSReadTool_ExecuteMapsChecksumArray(t *testing.T) {
	args := json.RawMessage(`{"path":"README.md","checksum":["md5","sha256","md5"]}`)
	req, err := (&FSReadTool{}).Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(req.Checksums) != 2 {
		t.Fatalf("checksums=%v want len=2", req.Checksums)
	}
}

func TestFSReadTool_ExecuteRejectsInvalidChecksum(t *testing.T) {
	args := json.RawMessage(`{"path":"README.md","checksum":"crc32"}`)
	if _, err := (&FSReadTool{}).Execute(context.Background(), args); err == nil {
		t.Fatalf("expected invalid checksum error")
	}
}
