package agent

import (
	"strings"
	"testing"

	llmtypes "github.com/tinoosan/agen8/pkg/llm/types"
)

func TestPromptToolSpecFromSources_IncludesFinalAnswerDedupesAndSorts(t *testing.T) {
	reg := NewHostToolRegistry()
	if err := reg.Register(&promptTestTool{name: "z_tool", desc: "Z"}); err != nil {
		t.Fatalf("register z_tool: %v", err)
	}
	if err := reg.Register(&promptTestTool{name: "a_tool", desc: "A"}); err != nil {
		t.Fatalf("register a_tool: %v", err)
	}

	spec := PromptToolSpecFromSources(reg, []llmtypes.Tool{
		{
			Type: "function",
			Function: llmtypes.ToolFunction{
				Name:        "a_tool",
				Description: "A override",
			},
		},
		{
			Type: "function",
			Function: llmtypes.ToolFunction{
				Name:        "b_tool",
				Description: "B",
			},
		},
	})

	if len(spec.Tools) != 4 {
		t.Fatalf("expected 4 tools (a,b,final,z), got %d", len(spec.Tools))
	}
	gotOrder := []string{
		spec.Tools[0].Name,
		spec.Tools[1].Name,
		spec.Tools[2].Name,
		spec.Tools[3].Name,
	}
	wantOrder := []string{"a_tool", "b_tool", "final_answer", "z_tool"}
	if strings.Join(gotOrder, ",") != strings.Join(wantOrder, ",") {
		t.Fatalf("unexpected sort order: got %v want %v", gotOrder, wantOrder)
	}
	for _, tool := range spec.Tools {
		if tool.Name == "final_answer" && strings.TrimSpace(tool.Description) == "" {
			t.Fatalf("expected final_answer description")
		}
	}
}

func TestSortedToolNamesFromRegistry_SortsAndDedupes(t *testing.T) {
	reg := NewHostToolRegistry()
	if err := reg.Register(&promptTestTool{name: "z_tool", desc: "Z"}); err != nil {
		t.Fatalf("register z_tool: %v", err)
	}
	if err := reg.Register(&promptTestTool{name: "a_tool", desc: "A"}); err != nil {
		t.Fatalf("register a_tool: %v", err)
	}

	got := SortedToolNamesFromRegistry(reg)
	want := []string{"a_tool", "z_tool"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected sorted names: got %v want %v", got, want)
	}
}

func TestPromptToolSpecForCodeExecOnly_UsesBridgeToolsForGuidance(t *testing.T) {
	modelReg := NewHostToolRegistry()
	if err := modelReg.Register(&promptTestTool{name: "code_exec", desc: "Run Python."}); err != nil {
		t.Fatalf("register code_exec: %v", err)
	}
	bridgeReg := NewHostToolRegistry()
	if err := bridgeReg.Register(&promptTestTool{name: "fs_read", desc: "Read files."}); err != nil {
		t.Fatalf("register fs_read: %v", err)
	}
	if err := bridgeReg.Register(&promptTestTool{name: "http_fetch", desc: "Fetch HTTP."}); err != nil {
		t.Fatalf("register http_fetch: %v", err)
	}
	// Should be filtered out from bridge guidance.
	if err := bridgeReg.Register(&promptTestTool{name: "code_exec", desc: "Run Python."}); err != nil {
		t.Fatalf("register code_exec bridge: %v", err)
	}

	spec := PromptToolSpecForCodeExecOnly(modelReg, bridgeReg, nil)
	if !spec.CodeExecOnly {
		t.Fatalf("expected code_exec_only guidance enabled")
	}
	if len(spec.CodeExecBridgeTools) != 2 {
		t.Fatalf("expected 2 bridge guidance tools, got %d", len(spec.CodeExecBridgeTools))
	}
	if spec.CodeExecBridgeTools[0].Name != "fs_read" || spec.CodeExecBridgeTools[1].Name != "http_fetch" {
		t.Fatalf("unexpected bridge tool order: %+v", spec.CodeExecBridgeTools)
	}
}
