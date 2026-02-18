package hosttools

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tinoosan/workbench-core/pkg/types"
)

func TestCodeExecTool_Execute(t *testing.T) {
	tool := &CodeExecTool{}
	req, err := tool.Execute(nil, json.RawMessage(`{
		"language":"python",
		"code":"result = {'ok': True}",
		"cwd":"/workspace",
		"timeoutMs":1234,
		"maxOutputBytes":8192
	}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if req.Op != types.HostOpCodeExec {
		t.Fatalf("expected op %q, got %q", types.HostOpCodeExec, req.Op)
	}
	if req.Language != "python" {
		t.Fatalf("expected python language, got %q", req.Language)
	}
	if req.Code == "" {
		t.Fatalf("expected code to be set")
	}
	if req.Cwd != "/workspace" || req.TimeoutMs != 1234 || req.MaxBytes != 8192 {
		t.Fatalf("unexpected request mapping: %+v", req)
	}
	if len(req.Input) != 0 {
		t.Fatalf("expected no extra input metadata, got: %s", string(req.Input))
	}
}

func TestCodeExecTool_Execute_Validation(t *testing.T) {
	tool := &CodeExecTool{}
	if _, err := tool.Execute(nil, json.RawMessage(`{"language":"python","code":" "}`)); err == nil {
		t.Fatalf("expected error for empty code")
	}
	if _, err := tool.Execute(nil, json.RawMessage(`{"language":"js","code":"1"}`)); err == nil {
		t.Fatalf("expected error for non-python language")
	}
}

func TestCodeExecTool_Definition_DoesNotExposeMaxToolCalls(t *testing.T) {
	def := (&CodeExecTool{}).Definition()
	params, ok := def.Function.Parameters.(map[string]any)
	if !ok {
		t.Fatalf("expected parameters object map, got %T", def.Function.Parameters)
	}
	props, _ := params["properties"].(map[string]any)
	if _, exists := props["maxToolCalls"]; exists {
		t.Fatalf("did not expect maxToolCalls in public schema")
	}
	required, _ := params["required"].([]any)
	for _, item := range required {
		if s, _ := item.(string); s == "maxToolCalls" {
			t.Fatalf("did not expect maxToolCalls in required fields")
		}
	}
}

func TestCodeExecTool_Definition_IncludesDelegationAndImportGuidance(t *testing.T) {
	desc := (&CodeExecTool{}).Definition().Function.Description
	if !strings.Contains(desc, "tools.task_create(") {
		t.Fatalf("expected task_create guidance in description, got: %s", desc)
	}
	if !strings.Contains(desc, "Batch multiple tool calls in one invocation") {
		t.Fatalf("expected efficiency guidance in description, got: %s", desc)
	}
	if !strings.Contains(desc, "spawnWorker=True") {
		t.Fatalf("expected canonical spawnWorker python guidance in description, got: %s", desc)
	}
	if !strings.Contains(desc, "True`/`False`/`None") {
		t.Fatalf("expected python literal guidance in description, got: %s", desc)
	}
	if !strings.Contains(desc, "import tasks") {
		t.Fatalf("expected invalid import guidance in description, got: %s", desc)
	}
}
