package app

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tinoosan/agen8/pkg/agent"
	hosttools "github.com/tinoosan/agen8/pkg/agent/hosttools"
	llmtypes "github.com/tinoosan/agen8/pkg/llm/types"
	"github.com/tinoosan/agen8/pkg/types"
)

func TestApplyAllowedTools(t *testing.T) {
	reg, err := agent.DefaultHostToolRegistry()
	if err != nil {
		t.Fatalf("DefaultHostToolRegistry: %v", err)
	}
	if err := applyAllowedTools(reg, []string{"shell_exec"}); err != nil {
		t.Fatalf("applyAllowedTools: %v", err)
	}
	if _, ok := reg.Get("shell_exec"); !ok {
		t.Fatalf("expected shell_exec to remain")
	}
	if _, ok := reg.Get("http_fetch"); ok {
		t.Fatalf("expected http_fetch to be removed")
	}
	// fs tools stay even though not in allowed list
	for _, name := range []string{"fs_list", "fs_stat", "fs_read", "fs_search", "fs_write", "fs_append", "fs_edit", "fs_patch", "fs_txn", "fs_archive_create", "fs_archive_extract", "fs_archive_list"} {
		if _, ok := reg.Get(name); !ok {
			t.Fatalf("expected always-enabled fs tool %q to remain", name)
		}
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

func TestResolveCodeExecOnly(t *testing.T) {
	profileDefault := true
	if got := resolveCodeExecOnly(profileDefault, nil); !got {
		t.Fatalf("resolveCodeExecOnly(nil) = %v, want true", got)
	}
	override := false
	if got := resolveCodeExecOnly(profileDefault, &override); got {
		t.Fatalf("resolveCodeExecOnly(false override) = %v, want false", got)
	}
}

func TestResolveToolRegistries_CodeExecOnly(t *testing.T) {
	base, err := agent.DefaultHostToolRegistry()
	if err != nil {
		t.Fatalf("DefaultHostToolRegistry: %v", err)
	}
	modelReg, bridgeReg, err := resolveToolRegistries(base, []string{"shell_exec"}, true)
	if err != nil {
		t.Fatalf("resolveToolRegistries: %v", err)
	}

	if _, ok := modelReg.Get("code_exec"); !ok {
		t.Fatalf("expected code_exec in model registry")
	}
	if _, ok := modelReg.Get("shell_exec"); ok {
		t.Fatalf("expected shell_exec hidden from model registry")
	}

	if _, ok := bridgeReg.Get("shell_exec"); !ok {
		t.Fatalf("expected shell_exec in bridge registry")
	}
	if _, ok := bridgeReg.Get("http_fetch"); ok {
		t.Fatalf("expected http_fetch removed from bridge registry")
	}
}

func TestResolveToolRegistries_Hybrid(t *testing.T) {
	base, err := agent.DefaultHostToolRegistry()
	if err != nil {
		t.Fatalf("DefaultHostToolRegistry: %v", err)
	}
	modelReg, bridgeReg, err := resolveToolRegistries(base, []string{"fs_read", "shell_exec"}, false)
	if err != nil {
		t.Fatalf("resolveToolRegistries: %v", err)
	}

	if _, ok := modelReg.Get("fs_read"); !ok {
		t.Fatalf("expected fs_read in model registry")
	}
	if _, ok := modelReg.Get("shell_exec"); !ok {
		t.Fatalf("expected shell_exec in model registry")
	}
	if _, ok := modelReg.Get("http_fetch"); ok {
		t.Fatalf("expected http_fetch removed from model registry")
	}

	if _, ok := bridgeReg.Get("fs_read"); !ok {
		t.Fatalf("expected fs_read in bridge registry")
	}
}

func TestApplyAllowedTools_AlwaysKeepsFsTools(t *testing.T) {
	reg, err := agent.DefaultHostToolRegistry()
	if err != nil {
		t.Fatalf("DefaultHostToolRegistry: %v", err)
	}
	// Only allow http_fetch — fs tools should still survive.
	if err := applyAllowedTools(reg, []string{"http_fetch"}); err != nil {
		t.Fatalf("applyAllowedTools: %v", err)
	}
	for _, name := range []string{"fs_list", "fs_stat", "fs_read", "fs_search", "fs_write", "fs_append", "fs_edit", "fs_patch", "fs_txn", "fs_archive_create", "fs_archive_extract", "fs_archive_list"} {
		if _, ok := reg.Get(name); !ok {
			t.Fatalf("expected always-enabled fs tool %q to survive, but it was removed", name)
		}
	}
	if _, ok := reg.Get("shell_exec"); ok {
		t.Fatalf("expected shell_exec to be removed since it was not in allowed list")
	}
}

func TestApplyAllowedTools_AlwaysKeepsObsidian(t *testing.T) {
	reg, err := agent.DefaultHostToolRegistry()
	if err != nil {
		t.Fatalf("DefaultHostToolRegistry: %v", err)
	}
	if err := reg.Register(&hosttools.ObsidianTool{}); err != nil {
		t.Fatalf("register obsidian: %v", err)
	}
	if err := applyAllowedTools(reg, []string{"fs_read"}); err != nil {
		t.Fatalf("applyAllowedTools: %v", err)
	}
	if _, ok := reg.Get("obsidian"); !ok {
		t.Fatalf("expected obsidian to remain")
	}
}

type testHostTool struct {
	name string
}

func (t testHostTool) Definition() llmtypes.Tool {
	return llmtypes.Tool{
		Type: "function",
		Function: llmtypes.ToolFunction{
			Name:        t.name,
			Description: "test tool",
			Parameters: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"required":             []any{},
				"additionalProperties": false,
			},
		},
	}
}

func (t testHostTool) Execute(context.Context, json.RawMessage) (types.HostOpRequest, error) {
	return types.HostOpRequest{}, nil
}

func TestApplyAllowedTools_AlwaysKeepsAnyFSPrefixedTool(t *testing.T) {
	reg := agent.NewHostToolRegistry()
	if err := reg.Register(testHostTool{name: "fs_custom"}); err != nil {
		t.Fatalf("register fs_custom: %v", err)
	}
	if err := reg.Register(testHostTool{name: "shell_exec"}); err != nil {
		t.Fatalf("register shell_exec: %v", err)
	}

	if err := applyAllowedTools(reg, []string{"shell_exec"}); err != nil {
		t.Fatalf("applyAllowedTools: %v", err)
	}
	if _, ok := reg.Get("fs_custom"); !ok {
		t.Fatalf("expected fs_custom to remain because fs_* tools are always enabled")
	}
}
