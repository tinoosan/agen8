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
	for _, key := range []string{"verify", "checksum", "checksumExpected", "checksumMd5", "checksumSha1", "checksumSha256", "atomic", "sync"} {
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
	args := json.RawMessage(`{"path":"README.md","text":"hello","verify":true,"checksum":"sha256","checksumExpected":"2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824","atomic":true,"sync":true}`)
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
	if !req.Verify || req.Checksum != "sha256" || req.ChecksumExpected == "" || !req.Atomic || !req.Sync {
		t.Fatalf("unexpected mapped flags %+v", req)
	}
}

func TestFSWriteTool_ExecuteMapsLegacyChecksumAlias(t *testing.T) {
	args := json.RawMessage(`{"path":"README.md","text":"hello","checksumMd5":"5d41402abc4b2a76b9719d911017c592"}`)
	req, err := (&FSWriteTool{}).Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if req.Checksum != "md5" {
		t.Fatalf("checksum = %q, want %q", req.Checksum, "md5")
	}
	if req.ChecksumExpected != "5d41402abc4b2a76b9719d911017c592" {
		t.Fatalf("checksumExpected = %q", req.ChecksumExpected)
	}
}

func TestFSWriteTool_ExecuteRejectsUnknownProperty(t *testing.T) {
	args := json.RawMessage(`{"path":"README.md","text":"hello","checksumTypo":"sha256"}`)
	if _, err := (&FSWriteTool{}).Execute(context.Background(), args); err == nil {
		t.Fatalf("expected error for unknown property")
	}
}
