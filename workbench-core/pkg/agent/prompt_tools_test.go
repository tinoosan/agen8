package agent

import (
	"strings"
	"testing"

	llmtypes "github.com/tinoosan/workbench-core/pkg/llm/types"
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
