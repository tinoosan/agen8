package hosttools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tinoosan/agen8/pkg/types"
)

func TestFSArchiveCreateTool_DefinitionAndExecute(t *testing.T) {
	tool := (&FSArchiveCreateTool{}).Definition()
	params := tool.Function.Parameters.(map[string]any)
	props := params["properties"].(map[string]any)
	if _, ok := props["path"]; !ok {
		t.Fatalf("expected path property")
	}
	if _, ok := props["destination"]; !ok {
		t.Fatalf("expected destination property")
	}
	if _, ok := props["format"]; !ok {
		t.Fatalf("expected format property")
	}

	req, err := (&FSArchiveCreateTool{}).Execute(context.Background(), json.RawMessage(`{
		"path":"notes",
		"destination":"/workspace/notes.zip",
		"format":"zip",
		"exclude":["*.tmp"],
		"includeMetadata":false
	}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if req.Op != types.HostOpFSArchiveCreate {
		t.Fatalf("op=%q", req.Op)
	}
	if req.Path != "/project/notes" || req.Destination != "/workspace/notes.zip" {
		t.Fatalf("unexpected paths: %+v", req)
	}
	if req.Format != "zip" || req.IncludeMetadata {
		t.Fatalf("unexpected format/metadata: %+v", req)
	}
}

func TestFSArchiveExtractTool_Execute(t *testing.T) {
	req, err := (&FSArchiveExtractTool{}).Execute(context.Background(), json.RawMessage(`{
		"path":"archive.tgz",
		"destination":"restored",
		"overwrite":true,
		"pattern":"*.md"
	}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if req.Op != types.HostOpFSArchiveExtract {
		t.Fatalf("op=%q", req.Op)
	}
	if req.Path != "/project/archive.tgz" || req.Destination != "/project/restored" {
		t.Fatalf("unexpected paths: %+v", req)
	}
	if !req.Overwrite || req.Pattern != "*.md" {
		t.Fatalf("unexpected overwrite/pattern: %+v", req)
	}
}

func TestFSArchiveListTool_Execute(t *testing.T) {
	req, err := (&FSArchiveListTool{}).Execute(context.Background(), json.RawMessage(`{
		"path":"backup.tar.gz",
		"limit":42
	}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if req.Op != types.HostOpFSArchiveList {
		t.Fatalf("op=%q", req.Op)
	}
	if req.Path != "/project/backup.tar.gz" || req.Limit != 42 {
		t.Fatalf("unexpected request: %+v", req)
	}
}
