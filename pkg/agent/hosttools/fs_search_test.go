package hosttools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tinoosan/agen8/pkg/types"
)

func TestFSSearchTool_DefinitionRequiredIncludesOnlyPath(t *testing.T) {
	tool := (&FSSearchTool{}).Definition()
	params, ok := tool.Function.Parameters.(map[string]any)
	if !ok {
		t.Fatalf("expected parameters to be a map, got %T", tool.Function.Parameters)
	}
	req, ok := params["required"].([]any)
	if !ok {
		t.Fatalf("expected required to be []any, got %T", params["required"])
	}
	if len(req) != 1 || req[0] != "path" {
		t.Fatalf("expected required to be [path], got %#v", req)
	}
}

func TestFSSearchTool_ExecuteMapsExtendedArgs(t *testing.T) {
	args := json.RawMessage(`{
		"path": "/project",
		"pattern": "TODO\\(",
		"glob": "**/*.go",
		"exclude": ["vendor/**", "**/*_test.go"],
		"previewLines": 2,
		"maxResults": 9,
		"includeMetadata": true,
		"maxSizeBytes": 2048
	}`)
	req, err := (&FSSearchTool{}).Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if req.Op != types.HostOpFSSearch {
		t.Fatalf("unexpected op: %q", req.Op)
	}
	if req.Path != "/project" || req.Pattern != `TODO\(` || req.Glob != "**/*.go" {
		t.Fatalf("unexpected mapped request: %+v", req)
	}
	if req.Limit != 9 || req.PreviewLines != 2 || !req.IncludeMetadata || req.MaxSizeBytes != 2048 {
		t.Fatalf("unexpected mapped request: %+v", req)
	}
	if len(req.Exclude) != 2 || req.Exclude[0] != "vendor/**" || req.Exclude[1] != "**/*_test.go" {
		t.Fatalf("unexpected excludes: %#v", req.Exclude)
	}
}

func TestFSSearchTool_ExecuteSupportsLimitAliasAndSingleExclude(t *testing.T) {
	args := json.RawMessage(`{
		"path": "/workspace",
		"query": "needle",
		"exclude": "*.tmp",
		"limit": 3
	}`)
	req, err := (&FSSearchTool{}).Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if req.Query != "needle" || req.Limit != 3 {
		t.Fatalf("unexpected mapped request: %+v", req)
	}
	if len(req.Exclude) != 1 || req.Exclude[0] != "*.tmp" {
		t.Fatalf("unexpected excludes: %#v", req.Exclude)
	}
}
