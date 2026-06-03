package app

import (
	"strings"
	"testing"
)

func TestDefaultSystemPrompt_ContainsCoreContent(t *testing.T) {
	s := DefaultSystemPrompt()
	if s == "" {
		t.Fatal("DefaultSystemPrompt() is empty")
	}
	if !strings.Contains(s, "Use only the tools exposed by the active runtime.") {
		t.Error("DefaultSystemPrompt() should constrain tool use to the runtime surface")
	}
	if !strings.Contains(s, `id="tool_catalog_verification"`) || !strings.Contains(s, "inspect the live runtime tool catalog first") {
		t.Error("DefaultSystemPrompt() should require live catalog verification before answering tool availability questions")
	}
	if !strings.Contains(s, `id="agen8_mcp_tool_first"`) || !strings.Contains(s, "Empty MCP resource/template lists do not mean Agen8 tools are unavailable") {
		t.Error("DefaultSystemPrompt() should describe Agen8 MCP as tool-first")
	}
	if !strings.Contains(s, `id="human_input_usage"`) {
		t.Error("DefaultSystemPrompt() should contain human input usage rule")
	}
	if !strings.Contains(s, `id="http_credentials"`) || !strings.Contains(s, "Agen8 injects matching credentials automatically") {
		t.Error("DefaultSystemPrompt() should contain HTTP credential injection guidance")
	}
	if !strings.Contains(s, `decision(action="ask_user")`) || !strings.Contains(s, `operator(action="request")`) {
		t.Error("DefaultSystemPrompt() should describe ask_user vs operator request usage")
	}
	if !strings.Contains(s, `treat the returned answer as authoritative and resolved`) {
		t.Error("DefaultSystemPrompt() should instruct that ask_user answers are authoritative")
	}
	if !strings.Contains(s, `id="action_first"`) {
		t.Error("DefaultSystemPrompt() should contain action_first rule")
	}
	if !strings.Contains(s, "Use exposed tools directly when they are needed") {
		t.Error("DefaultSystemPrompt() should instruct direct tool execution")
	}
	if strings.Contains(s, `tool(action="search"`) || strings.Contains(s, "browse /tools") || strings.Contains(s, "Use `/tools/") {
		t.Error("DefaultSystemPrompt() should not reference stale tool discovery concepts")
	}
	if !strings.Contains(s, "use the plan tool when available") {
		t.Error("DefaultSystemPrompt() should contain planning tool guidance")
	}
	if strings.Contains(s, "/memory") {
		t.Error("DefaultSystemPrompt() should not reference /memory mounts")
	}
	if strings.Contains(s, "/skills") || strings.Contains(s, "SKILL.md") {
		t.Error("DefaultSystemPrompt() should not instruct path-based skill file access")
	}
	if strings.Contains(s, "ALWAYS check /skills first") {
		t.Error("DefaultSystemPrompt() should not include speculative skill-reading guidance")
	}
	if strings.Contains(s, "curiosity") {
		t.Error("DefaultSystemPrompt() should not contain curiosity rule (causes exploration spirals)")
	}
	if strings.Contains(s, "GATE") {
		t.Error("DefaultSystemPrompt() should not contain planning gate")
	}
}

func TestDefaultSystemPrompt_PathContractUsesCanonicalPaths(t *testing.T) {
	s := DefaultSystemPrompt()
	if !strings.Contains(s, `project-relative paths or workspace/...`) {
		t.Fatalf("expected canonical path guidance, got: %s", s)
	}
	if strings.Contains(s, `absolute VFS mount paths (/project, /workspace`) {
		t.Fatalf("should not instruct legacy absolute /project or /workspace paths, got: %s", s)
	}
	if strings.Contains(s, `{"path": "/project/file"`) {
		t.Fatalf("should not include legacy edit_file /project example, got: %s", s)
	}
}

func TestDefaultSystemPrompt_DoesNotEnumerateTools(t *testing.T) {
	s := DefaultSystemPrompt()
	if !strings.Contains(s, "<runtime>Use the tools exposed by the active model runtime or harness.</runtime>") {
		t.Fatalf("expected generic runtime tool entry, got: %s", s)
	}
}

func TestDefaultMemberModeSystemPrompt_UsesDirectResponseContract(t *testing.T) {
	s := DefaultMemberModeSystemPrompt()
	if !strings.Contains(s, `final_response_contract`) {
		t.Fatalf("expected direct response contract rule, got: %s", s)
	}
	if strings.Contains(s, `final_answer`) {
		t.Fatalf("did not expect final_answer references, got: %s", s)
	}
}

func TestDefaultMemberModeSystemPrompt_ExcludesSubagentWording(t *testing.T) {
	s := DefaultMemberModeSystemPrompt()
	exclude := []string{"spawn_worker", "task_review", "subagent", "/deliverables/subagents"}
	for _, word := range exclude {
		if strings.Contains(s, word) {
			t.Errorf("DefaultMemberModeSystemPrompt() must not contain %q (member mode has no subagents)", word)
		}
	}
	if strings.Contains(s, "callback") && strings.Contains(s, "worker") {
		t.Error("DefaultMemberModeSystemPrompt() must not mention worker callbacks")
	}
}

func TestDefaultSystemPrompt_IsDelegationAgnostic(t *testing.T) {
	base := DefaultSystemPrompt()
	if strings.Contains(base, "spawn_worker") {
		t.Error("DefaultSystemPrompt() (base) should not contain spawn_worker reference")
	}
	if strings.Contains(base, "spawnWorker") {
		t.Error("DefaultSystemPrompt() (base) should not contain spawnWorker reference")
	}
	if !strings.Contains(base, "Use only the tools exposed by the active runtime.") {
		t.Error("DefaultSystemPrompt() should contain base content")
	}
}

func TestDefaultAutonomousSystemPrompt_UsesSpaceDelegation(t *testing.T) {
	s := DefaultAutonomousSystemPrompt()
	if strings.Contains(s, "spawnWorker") {
		t.Fatalf("autonomous prompt should not reference spawnWorker")
	}
	if strings.Contains(s, "subagent") {
		t.Fatalf("autonomous prompt should not reference subagent")
	}
	if strings.Contains(s, "spawn_worker") {
		t.Fatalf("autonomous prompt should not reference spawn_worker")
	}
	if !strings.Contains(s, "assigned_role") {
		t.Fatalf("autonomous prompt should use assigned_role delegation")
	}
	if !strings.Contains(s, "After delegating tasks, end the turn") {
		t.Fatalf("autonomous prompt should include direct post-delegation turn guidance")
	}
}

func TestDefaultMemberModeSystemPrompt_TreatsAddressedTasksAsAuthoritative(t *testing.T) {
	s := DefaultMemberModeSystemPrompt()
	for _, want := range []string{
		"addressed_task_autonomy",
		"authoritative work addressed to you",
		"Do not ask the human for permission merely because the action uses this member identity",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("member mode prompt missing %q", want)
		}
	}
}

func TestDefaultMemberModeSystemPrompt_IncludesAgen8OperatingModel(t *testing.T) {
	s := DefaultMemberModeSystemPrompt()
	for _, want := range []string{
		`id="agen8_operating_model"`,
		"Missions, key results, and tasks are the default way to track non-trivial work",
		"Member messages are preferred for agent-to-agent coordination",
		"Operator actions are for human/manual execution gates",
		"Escalations are for policy decisions or approval gates",
		`decision(action="ask_user") instead of plain chat`,
		"graph links for mission context",
		"Task state is authoritative over notifications",
		"coordinator should not do worker tasks when a worker is available",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("member mode prompt missing %q", want)
		}
	}
}

func TestTaskRunnerPrompt_DoesNotIncludeDomainSpecificSharedRules(t *testing.T) {
	for name, prompt := range map[string]string{
		"autonomous": DefaultAutonomousSystemPrompt(),
		"member":     DefaultMemberModeSystemPrompt(),
	} {
		if strings.Contains(prompt, `id="initiative"`) {
			t.Fatalf("%s prompt should not contain shared initiative rule", name)
		}
		if strings.Contains(prompt, `id="turn_lifecycle"`) {
			t.Fatalf("%s prompt should not contain shared turn_lifecycle rule", name)
		}
	}
}
