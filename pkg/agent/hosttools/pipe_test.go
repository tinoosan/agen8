package hosttools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tinoosan/agen8/pkg/types"
)

func TestPipeTool_DefinitionAndExecute(t *testing.T) {
	tool := (&PipeTool{}).Definition()
	params := tool.Function.Parameters.(map[string]any)
	props := params["properties"].(map[string]any)
	if _, ok := props["steps"]; !ok {
		t.Fatalf("expected steps property")
	}
	if _, ok := props["options"]; !ok {
		t.Fatalf("expected options property")
	}

	req, err := (&PipeTool{}).Execute(context.Background(), json.RawMessage(`{
		"steps": [
			{"type":"tool","tool":"fs_read","args":{"path":"/workspace/a.txt"},"output":"text"},
			{"type":"transform","transform":"trim"},
			{"type":"tool","tool":"fs_write","args":{"path":"/workspace/out.txt"},"inputArg":"text"}
		],
		"options": {"debug": true, "maxSteps": 4, "maxValueBytes": 2048}
	}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if req.Op != types.HostOpPipe {
		t.Fatalf("op=%q", req.Op)
	}
	if len(req.PipeSteps) != 3 {
		t.Fatalf("unexpected pipe steps: %#v", req.PipeSteps)
	}
	if req.PipeSteps[0].Tool != "fs_read" || req.PipeSteps[0].Output != "text" {
		t.Fatalf("unexpected first step: %#v", req.PipeSteps[0])
	}
	if req.PipeSteps[2].InputArg != "text" {
		t.Fatalf("unexpected inputArg: %#v", req.PipeSteps[2])
	}
	if req.PipeOptions == nil || !req.PipeOptions.Debug || req.PipeOptions.MaxSteps != 4 || req.PipeOptions.MaxValueBytes != 2048 {
		t.Fatalf("unexpected options: %#v", req.PipeOptions)
	}
}
