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
	if !strings.Contains(s, "/plan/HEAD.md") {
		t.Error("DefaultSystemPrompt() should contain planning content")
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
