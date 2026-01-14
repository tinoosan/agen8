package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tinoosan/workbench-core/internal/tools"
	"github.com/tinoosan/workbench-core/internal/types"
)

func TestBuiltinFormat_JSONPretty(t *testing.T) {
	inv := tools.BuiltinFormatInvoker{}

	req := types.ToolRequest{
		Version:  "v1",
		CallID:   "c1",
		ToolID:   types.ToolID("builtin.format"),
		ActionID: "json.pretty",
		Input:    json.RawMessage(`{"text":"{\"b\":2,\"a\":1}","indent":2}`),
	}
	res, err := inv.Invoke(context.Background(), req)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	var out struct {
		Text    string `json:"text"`
		Changed bool   `json:"changed"`
	}
	if err := json.Unmarshal(res.Output, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !out.Changed {
		t.Fatalf("expected changed=true")
	}
	if out.Text == "" || out.Text[0] != '{' || out.Text[len(out.Text)-1] != '\n' {
		t.Fatalf("unexpected output: %q", out.Text)
	}
	if want := "\"b\": 2"; !contains(out.Text, want) {
		t.Fatalf("expected pretty JSON to include %q, got:\n%s", want, out.Text)
	}
}

func TestBuiltinFormat_JSONPretty_SortKeys(t *testing.T) {
	inv := tools.BuiltinFormatInvoker{}

	req := types.ToolRequest{
		Version:  "v1",
		CallID:   "c1",
		ToolID:   types.ToolID("builtin.format"),
		ActionID: "json.pretty",
		Input:    json.RawMessage(`{"text":"{\"b\":2,\"a\":1}","indent":2,"sortKeys":true}`),
	}
	res, err := inv.Invoke(context.Background(), req)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(res.Output, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	// "a" should appear before "b" in the canonicalized JSON.
	ia := strings.Index(out.Text, "\"a\"")
	ib := strings.Index(out.Text, "\"b\"")
	if ia == -1 || ib == -1 || ia >= ib {
		t.Fatalf("expected sorted keys, got:\n%s", out.Text)
	}
}

func TestBuiltinFormat_JSONPretty_Invalid(t *testing.T) {
	inv := tools.BuiltinFormatInvoker{}
	req := types.ToolRequest{
		Version:  "v1",
		CallID:   "c1",
		ToolID:   types.ToolID("builtin.format"),
		ActionID: "json.pretty",
		Input:    json.RawMessage(`{"text":"{not json}"}`),
	}
	_, err := inv.Invoke(context.Background(), req)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestBuiltinFormat_HTMLPretty(t *testing.T) {
	inv := tools.BuiltinFormatInvoker{}
	req := types.ToolRequest{
		Version:  "v1",
		CallID:   "c1",
		ToolID:   types.ToolID("builtin.format"),
		ActionID: "html.pretty",
		Input:    json.RawMessage(`{"text":"<html><body><div>Hello</div></body></html>","indent":2}`),
	}
	res, err := inv.Invoke(context.Background(), req)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	var out struct {
		Text    string `json:"text"`
		Changed bool   `json:"changed"`
	}
	if err := json.Unmarshal(res.Output, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !out.Changed {
		t.Fatalf("expected changed=true")
	}
	if !contains(out.Text, "\n  <body>") {
		t.Fatalf("expected indentation, got:\n%s", out.Text)
	}
}

func contains(s, sub string) bool { return strings.Index(s, sub) >= 0 }
