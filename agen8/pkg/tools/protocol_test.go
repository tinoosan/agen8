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
