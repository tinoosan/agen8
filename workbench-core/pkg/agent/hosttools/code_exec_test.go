package hosttools

import (
	"encoding/json"
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
		"maxOutputBytes":8192,
		"maxToolCalls":10
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
	if len(req.Input) == 0 {
		t.Fatalf("expected maxToolCalls metadata in input")
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
