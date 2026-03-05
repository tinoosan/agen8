package hosttools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tinoosan/agen8/pkg/types"
)

func TestFSTxnTool_DefinitionIncludesStepsAndOptions(t *testing.T) {
	tool := (&FSTxnTool{}).Definition()
	params, ok := tool.Function.Parameters.(map[string]any)
	if !ok {
		t.Fatalf("expected parameters map, got %T", tool.Function.Parameters)
	}
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties map, got %T", params["properties"])
	}
	if _, ok := props["steps"]; !ok {
		t.Fatalf("expected steps property")
	}
	if _, ok := props["options"]; !ok {
		t.Fatalf("expected options property")
	}
	req, ok := params["required"].([]any)
	if !ok || len(req) != 1 || req[0] != "steps" {
		t.Fatalf("expected required=[steps], got %#v", params["required"])
	}
}

func TestFSTxnTool_ExecuteMapsToHostRequest(t *testing.T) {
	args := json.RawMessage(`{
		"steps":[
			{"op":"fs_write","path":"README.md","text":"x","mode":"w"},
			{"op":"fs_patch","path":"README.md","text":"@@ -1 +1 @@\n-x\n+y\n","verbose":true}
		],
		"options":{"dryRun":true,"apply":false,"rollbackOnError":true}
	}`)
	req, err := (&FSTxnTool{}).Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if req.Op != types.HostOpFSTxn {
		t.Fatalf("op=%q, want %q", req.Op, types.HostOpFSTxn)
	}
	if len(req.TxnSteps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(req.TxnSteps))
	}
	if req.TxnSteps[0].Path != "/project/README.md" {
		t.Fatalf("unexpected mapped path: %q", req.TxnSteps[0].Path)
	}
	if req.TxnOptions == nil || !req.TxnOptions.DryRun || req.TxnOptions.Apply || !req.TxnOptions.RollbackOnError {
		t.Fatalf("unexpected options: %+v", req.TxnOptions)
	}
}
