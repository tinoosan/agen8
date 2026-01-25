package tools

import (
	"encoding/json"
	"testing"
)

func TestToolRequestValidate_RejectsInvalid(t *testing.T) {
	req := ToolRequest{
		Version:  "v1",
		CallID:   "c1",
		ToolID:   ToolID("github.com.acme.tool"),
		ActionID: "exec",
		Input:    nil,
	}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected input required error")
	}

	req.Input = json.RawMessage(`{invalid}`)
	if err := req.Validate(); err == nil {
		t.Fatalf("expected invalid json error")
	}
}

func TestToolResponseValidate_OKRequiresNilError(t *testing.T) {
	req := ToolRequest{
		Version:  "v1",
		CallID:   "c1",
		ToolID:   ToolID("github.com.acme.tool"),
		ActionID: "exec",
		Input:    json.RawMessage(`{}`),
	}
	resp := NewToolResponseOK(req, json.RawMessage(`{"ok":true}`), nil)
	if err := resp.Validate(); err != nil {
		t.Fatalf("unexpected validate error: %v", err)
	}

	resp.Error = &ToolError{Code: "tool_failed", Message: "nope"}
	if err := resp.Validate(); err == nil {
		t.Fatalf("expected error when ok=true and error present")
	}
}
