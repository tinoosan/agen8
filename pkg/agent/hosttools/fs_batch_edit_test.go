package hosttools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tinoosan/agen8/pkg/types"
)

func TestFSBatchEditTool_DefinitionAndExecute(t *testing.T) {
	tool := (&FSBatchEditTool{}).Definition()
	params := tool.Function.Parameters.(map[string]any)
	props := params["properties"].(map[string]any)
	if _, ok := props["path"]; !ok {
		t.Fatalf("expected path property")
	}
	if _, ok := props["glob"]; !ok {
		t.Fatalf("expected glob property")
	}
	if _, ok := props["edits"]; !ok {
		t.Fatalf("expected edits property")
	}

	req, err := (&FSBatchEditTool{}).Execute(context.Background(), json.RawMessage(`{
		"path":"/knowledge",
		"glob":"**/*.md",
		"exclude":["archive/**"],
		"edits":[
			{"old":"[[old-note]]","new":"[[new-note]]"},
			{"old":"old-note","new":"new-note","occurrence":2}
		],
		"options":{"dryRun":true,"apply":false,"rollbackOnError":true,"maxFiles":25}
	}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if req.Op != types.HostOpFSBatchEdit {
		t.Fatalf("op=%q", req.Op)
	}
	if req.Path != "/knowledge" || req.Glob != "**/*.md" {
		t.Fatalf("unexpected path/glob: %+v", req)
	}
	if len(req.Exclude) != 1 || req.Exclude[0] != "archive/**" {
		t.Fatalf("unexpected exclude: %#v", req.Exclude)
	}
	if len(req.BatchEditEdits) != 2 {
		t.Fatalf("unexpected edits: %#v", req.BatchEditEdits)
	}
	if req.BatchEditEdits[0].Occurrence != "all" || req.BatchEditEdits[1].Occurrence != "2" {
		t.Fatalf("unexpected occurrences: %#v", req.BatchEditEdits)
	}
	if req.BatchEditOptions == nil || !req.BatchEditOptions.DryRun || req.BatchEditOptions.Apply || !req.BatchEditOptions.RollbackOnError || req.BatchEditOptions.MaxFiles != 25 {
		t.Fatalf("unexpected options: %+v", req.BatchEditOptions)
	}
}
