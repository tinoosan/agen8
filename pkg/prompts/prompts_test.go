package prompts

import (
	"strings"
	"testing"
)

func TestDefaultSystemPrompt_ContainsCoreContent(t *testing.T) {
	s := DefaultSystemPrompt()
	if s == "" {
		t.Fatal("DefaultSystemPrompt() is empty")
	}
	if !strings.Contains(s, "fs_list") {
		t.Error("DefaultSystemPrompt() should contain fs_list")
	}
	if !strings.Contains(s, "fs_stat") {
		t.Error("DefaultSystemPrompt() should contain fs_stat")
	}
	if !strings.Contains(s, "fs_archive_list") {
		t.Error("DefaultSystemPrompt() should contain fs_archive_list")
	}
	if !strings.Contains(s, "fs_batch_edit") {
		t.Error("DefaultSystemPrompt() should contain fs_batch_edit")
	}
	if !strings.Contains(s, "pipe") {
		t.Error("DefaultSystemPrompt() should contain pipe")
	}
	if !strings.Contains(s, "prefer fs_stat before fs_read") {
		t.Error("DefaultSystemPrompt() should include fs_stat cost guidance")
	}
	if !strings.Contains(s, "Prefer fs_archive_list before fs_archive_extract") {
		t.Error("DefaultSystemPrompt() should include archive inspection guidance")
	}
	if !strings.Contains(s, "prefer fs_batch_edit over fs_search plus manual edit loops") {
		t.Error("DefaultSystemPrompt() should include fs_batch_edit guidance")
	}
	if !strings.Contains(s, "Prefer pipe for simple linear dataflow") {
		t.Error("DefaultSystemPrompt() should include pipe guidance")
	}
	if !strings.Contains(s, "/plan/HEAD.md") {
		t.Error("DefaultSystemPrompt() should contain planning content")
	}
}

func TestDefaultSystemPromptWithTools_RendersInjectedTools(t *testing.T) {
	s := DefaultSystemPromptWithTools(PromptToolSpec{
		Tools: []PromptTool{
			{Name: "zeta_tool", Description: "Zeta action."},
			{Name: "alpha_tool", Description: "Alpha action."},
		},
	})
	if !strings.Contains(s, `<op name="alpha_tool">Alpha action.</op>`) {
		t.Fatalf("expected injected alpha_tool op, got: %s", s)
	}
	if !strings.Contains(s, `<op name="zeta_tool">Zeta action.</op>`) {
		t.Fatalf("expected injected zeta_tool op, got: %s", s)
	}
	if !strings.Contains(s, "Use the available tools (alpha_tool, zeta_tool); do not invent other tools.") {
		t.Fatalf("expected injected tool usage rule, got: %s", s)
	}
	if strings.Index(s, "alpha_tool") > strings.Index(s, "zeta_tool") {
		t.Fatalf("expected deterministic lexical order in prompt")
	}
}

func TestDefaultSystemPromptWithTools_DeduplicatesAndEscapes(t *testing.T) {
	s := DefaultSystemPromptWithTools(PromptToolSpec{
		Tools: []PromptTool{
			{Name: "dup_tool", Description: "First <desc>"},
			{Name: "dup_tool", Description: "Second desc should not replace"},
			{Name: "tool<&>", Description: "Unsafe <xml>"},
		},
	})
	if strings.Count(s, `name="dup_tool"`) != 1 {
		t.Fatalf("expected deduped dup_tool entry, got: %s", s)
	}
	if !strings.Contains(s, `name="tool&lt;&amp;&gt;"`) {
		t.Fatalf("expected escaped tool name, got: %s", s)
	}
	if !strings.Contains(s, "First &lt;desc&gt;") {
		t.Fatalf("expected escaped description with first description retained, got: %s", s)
	}
}

func TestDefaultSystemPromptWithTools_EmptyToolsetFallback(t *testing.T) {
	s := DefaultSystemPromptWithTools(PromptToolSpec{})
	if !strings.Contains(s, `<op name="final_answer">`) {
		t.Fatalf("expected final_answer fallback op, got: %s", s)
	}
	if !strings.Contains(s, "Use the available tools for operations and diagnostics; do not invent other tools.") {
		t.Fatalf("expected generic fallback tool usage rule, got: %s", s)
	}
}

func TestDefaultSystemPromptWithTools_CodeExecGuidanceInjected(t *testing.T) {
	s := DefaultSystemPromptWithTools(PromptToolSpec{
		Tools:        []PromptTool{{Name: "code_exec", Description: "Run python code."}},
		CodeExecOnly: true,
		CodeExecBridgeTools: []PromptTool{
			{Name: "fs_read", Description: "Read files."},
			{Name: "http_fetch", Description: "Fetch over HTTP."},
		},
	})
	if !strings.Contains(s, `id="code_exec_orchestration"`) {
		t.Fatalf("expected code_exec orchestration rule, got: %s", s)
	}
	if !strings.Contains(s, `id="code_exec_efficiency"`) {
		t.Fatalf("expected code_exec efficiency rule, got: %s", s)
	}
	if !strings.Contains(s, `id="code_exec_bridge_hints"`) {
		t.Fatalf("expected bridge hints rule, got: %s", s)
	}
	if !strings.Contains(s, "tools.fs_read(...)") || !strings.Contains(s, "tools.http_fetch(...)") {
		t.Fatalf("expected bridge tool hints, got: %s", s)
	}
	if !strings.Contains(s, "tools.task_create(goal=") {
		t.Fatalf("expected task_create delegation guidance, got: %s", s)
	}
	if !strings.Contains(s, "spawnWorker=True") {
		t.Fatalf("expected canonical spawnWorker guidance, got: %s", s)
	}
	if !strings.Contains(s, "GOOD: one code_exec reads 3 files") {
		t.Fatalf("expected efficiency good/bad example guidance, got: %s", s)
	}
	if !strings.Contains(s, "True`/`False`/`None") {
		t.Fatalf("expected python literal guidance, got: %s", s)
	}
	if !strings.Contains(s, "import tasks") {
		t.Fatalf("expected explicit invalid import anti-pattern guidance, got: %s", s)
	}
	if strings.Index(s, "tools.fs_read(...)") > strings.Index(s, "tools.http_fetch(...)") {
		t.Fatalf("expected deterministic lexical order for bridge hints")
	}
}

func TestDefaultSystemPromptWithTools_CodeExecGuidanceInjected_NoBridgeTools(t *testing.T) {
	s := DefaultSystemPromptWithTools(PromptToolSpec{
		Tools:        []PromptTool{{Name: "code_exec", Description: "Run python code."}},
		CodeExecOnly: true,
	})
	if !strings.Contains(s, `id="code_exec_orchestration"`) {
		t.Fatalf("expected code_exec orchestration rule, got: %s", s)
	}
	if !strings.Contains(s, `id="code_exec_efficiency"`) {
		t.Fatalf("expected code_exec efficiency rule, got: %s", s)
	}
	if strings.Contains(s, `id="code_exec_bridge_hints"`) {
		t.Fatalf("did not expect bridge hints without bridge tools, got: %s", s)
	}
}

func TestDefaultSystemPromptWithTools_CodeExecGuidanceOmittedWhenOff(t *testing.T) {
	s := DefaultSystemPromptWithTools(PromptToolSpec{
		Tools: []PromptTool{{Name: "code_exec", Description: "Run python code."}},
	})
	if strings.Contains(s, `id="code_exec_orchestration"`) || strings.Contains(s, `id="code_exec_bridge_hints"`) {
		t.Fatalf("did not expect code_exec guidance when code_exec_only is off")
	}
}

func TestDefaultTeamModeSystemPrompt_ExcludesSubagentWording(t *testing.T) {
	s := DefaultTeamModeSystemPrompt()
	// Team co-agents must not be instructed about subagents, spawn_worker, task_review, or callbacks.
	exclude := []string{"spawn_worker", "task_review", "subagent", "/deliverables/subagents"}
	for _, word := range exclude {
		if strings.Contains(s, word) {
			t.Errorf("DefaultTeamModeSystemPrompt() must not contain %q (team mode has no subagents)", word)
		}
	}
	// "callback" in isolation could be generic; disallow the phrase that refers to worker callbacks
	if strings.Contains(s, "callback") && strings.Contains(s, "worker") {
		t.Error("DefaultTeamModeSystemPrompt() must not mention worker callbacks")
	}
}

func TestDefaultSystemPrompt_IsDelegationAgnostic(t *testing.T) {
	base := DefaultSystemPrompt()
	if strings.Contains(base, "recursive_delegation") {
		t.Error("DefaultSystemPrompt() (base) should not contain recursive_delegation block")
	}
	if strings.Contains(base, "spawn_worker") {
		t.Error("DefaultSystemPrompt() (base) should not contain spawn_worker reference")
	}
	if !strings.Contains(base, "fs_list") {
		t.Error("DefaultSystemPrompt() should contain base content")
	}
}

func TestDefaultAutonomousSystemPrompt_UsesStandaloneSubagentCanonicalPaths(t *testing.T) {
	s := DefaultAutonomousSystemPrompt()
	if strings.Contains(s, "/deliverables/subagents") {
		t.Fatalf("autonomous prompt should not reference legacy deliverables subagent paths")
	}
	if !strings.Contains(s, "/workspace/subagent-&lt;N&gt;/") {
		t.Fatalf("autonomous prompt should include canonical subagent workspace path")
	}
	if !strings.Contains(s, "/tasks/subagent-&lt;N&gt;/&lt;date&gt;/&lt;taskID&gt;/SUMMARY.md") {
		t.Fatalf("autonomous prompt should include canonical subagent summary path")
	}
	if !strings.Contains(s, "spawnWorker=true") {
		t.Fatalf("autonomous prompt should use canonical spawnWorker=true wording")
	}
}

func TestDefaultSubAgentSystemPrompt_WritesToWorkspace(t *testing.T) {
	s := DefaultSubAgentSystemPrompt()
	if strings.Contains(s, "/deliverables") {
		t.Fatalf("subagent prompt should not instruct /deliverables")
	}
	if !strings.Contains(s, "under /workspace") {
		t.Fatalf("subagent prompt should instruct writing under /workspace")
	}
}
