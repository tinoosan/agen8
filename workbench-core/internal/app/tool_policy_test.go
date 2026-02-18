package app

import (
	"testing"

	"github.com/tinoosan/workbench-core/pkg/agent"
)

func TestApplyAllowedTools(t *testing.T) {
	reg, err := agent.DefaultHostToolRegistry()
	if err != nil {
		t.Fatalf("DefaultHostToolRegistry: %v", err)
	}
	if err := applyAllowedTools(reg, []string{"fs_read", "shell_exec"}); err != nil {
		t.Fatalf("applyAllowedTools: %v", err)
	}
	if _, ok := reg.Get("fs_read"); !ok {
		t.Fatalf("expected fs_read to remain")
	}
	if _, ok := reg.Get("shell_exec"); !ok {
		t.Fatalf("expected shell_exec to remain")
	}
	if _, ok := reg.Get("http_fetch"); ok {
		t.Fatalf("expected http_fetch to be removed")
	}
}

func TestApplyAllowedTools_Unknown(t *testing.T) {
	reg, err := agent.DefaultHostToolRegistry()
	if err != nil {
		t.Fatalf("DefaultHostToolRegistry: %v", err)
	}
	if err := applyAllowedTools(reg, []string{"nope_tool"}); err == nil {
		t.Fatalf("expected unknown tool error")
	}
}

func TestSanitizeAllowedToolsForRole_RemovesTaskCreateForTeamWorker(t *testing.T) {
	sanitized, removed := sanitizeAllowedToolsForRole(
		[]string{"fs_read", "task_create", "task_review"},
		"team-1",
		false,
	)
	if len(removed) != 1 || removed[0] != "task_create" {
		t.Fatalf("removed=%v", removed)
	}
	if len(sanitized) != 2 || sanitized[0] != "fs_read" || sanitized[1] != "task_review" {
		t.Fatalf("sanitized=%v", sanitized)
	}
}

func TestSanitizeAllowedToolsForRole_KeepsTaskCreateForCoordinator(t *testing.T) {
	sanitized, removed := sanitizeAllowedToolsForRole(
		[]string{"fs_read", "task_create", "task_review"},
		"team-1",
		true,
	)
	if len(removed) != 0 {
		t.Fatalf("removed=%v", removed)
	}
	if len(sanitized) != 3 {
		t.Fatalf("sanitized=%v", sanitized)
	}
}
