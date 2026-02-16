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

func TestBaseWithoutRecursiveDelegation_RemovesRecursiveDelegationBlock(t *testing.T) {
	without := baseWithoutRecursiveDelegation()
	if strings.Contains(without, "recursive_delegation") {
		t.Error("baseWithoutRecursiveDelegation() should not contain recursive_delegation block")
	}
	if strings.Contains(without, "spawn_worker=true") {
		t.Error("baseWithoutRecursiveDelegation() should not contain spawn_worker reference")
	}
	if !strings.Contains(without, "fs_list") {
		t.Error("baseWithoutRecursiveDelegation() should still contain base content")
	}
}
