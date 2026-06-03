package app

import (
	"context"
	"slices"
	"testing"

	"github.com/tinoosan/agen8-mcp-server/pkg/membertype"
)

func TestAuthorize_Worker(t *testing.T) {
	svc := NewService()
	result, err := svc.Authorize(context.Background(), AuthorizeParams{
		SpaceID:      "space-1",
		MemberType:   &membertype.WorkerType{},
		MemberCount:  3,
		AllowedTools: []string{"code_exec", "http"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Allowed) != 2 {
		t.Errorf("expected 2 allowed tools, got %d: %v", len(result.Allowed), result.Allowed)
	}
	if len(result.Removed) != 0 {
		t.Errorf("expected no tools removed, got %v", result.Removed)
	}
}

func TestAuthorize_WorkerDefaults(t *testing.T) {
	svc := NewService()
	result, err := svc.Authorize(context.Background(), AuthorizeParams{
		SpaceID:     "space-1",
		MemberType:  &membertype.WorkerType{},
		MemberCount: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Allowed) == 0 {
		t.Error("expected default tools for worker with empty allowedTools")
	}
}

func TestAuthorize_WorkerStripsSystemTools(t *testing.T) {
	svc := NewService()
	result, err := svc.Authorize(context.Background(), AuthorizeParams{
		SpaceID:      "space-1",
		MemberType:   &membertype.WorkerType{},
		MemberCount:  3,
		AllowedTools: []string{"code_exec", "read_file", "task"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// task is a system tool; read_file is now user-scoped.
	for _, tool := range result.Allowed {
		if tool == "task" {
			t.Errorf("system/coordinator tool %q should have been removed", tool)
		}
	}
	if len(result.Removed) != 1 || result.Removed[0] != "task" {
		t.Errorf("expected task removed, got %v", result.Removed)
	}
}

func TestAuthorize_Coordinator(t *testing.T) {
	svc := NewService()
	result, err := svc.Authorize(context.Background(), AuthorizeParams{
		SpaceID:      "space-1",
		MemberType:   &membertype.CoordinatorType{},
		MemberCount:  3,
		HasReviewer:  true,
		AllowedTools: []string{"code_exec", "shell_exec"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Coordinator should have system tools merged in
	toolSet := map[string]bool{}
	for _, t := range result.Allowed {
		toolSet[t] = true
	}
	if !toolSet["space"] {
		t.Error("coordinator should have space merged in")
	}
	if !toolSet["task"] {
		t.Error("space coordinator should have task merged in")
	}
}

func TestAuthorize_Reviewer(t *testing.T) {
	svc := NewService()
	result, err := svc.Authorize(context.Background(), AuthorizeParams{
		SpaceID:      "space-1",
		MemberType:   &membertype.ReviewerType{},
		MemberCount:  3,
		AllowedTools: []string{"code_exec", "http"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Allowed) != 2 {
		t.Errorf("reviewer should retain both allowed tools, got %v", result.Allowed)
	}
	if len(result.Removed) != 0 {
		t.Errorf("expected no tools removed, got %v", result.Removed)
	}
}

func TestAuthorize_IgnoresCodeExecOnly(t *testing.T) {
	svc := NewService()
	result, err := svc.Authorize(context.Background(), AuthorizeParams{
		SpaceID:      "space-1",
		MemberType:   &membertype.WorkerType{},
		MemberCount:  3,
		AllowedTools: []string{"http", "code_exec"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(result.Allowed, "code_exec") {
		t.Errorf("expected code_exec to remain allowed as a normal tool, got %v", result.Allowed)
	}
	if !slices.Contains(result.Allowed, "http") {
		t.Errorf("expected http to remain allowed as a normal tool, got %v", result.Allowed)
	}
}

func TestAuthorize_StripsShellExecAliasBash(t *testing.T) {
	svc := NewService()
	result, err := svc.Authorize(context.Background(), AuthorizeParams{
		SpaceID:      "space-1",
		MemberType:   &membertype.WorkerType{},
		MemberCount:  3,
		AllowedTools: []string{"code_exec", "bash", "http"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range result.Allowed {
		if tool == "bash" {
			t.Fatalf("bash must not be present in allowed tools: %v", result.Allowed)
		}
	}
	if len(result.Removed) != 1 || result.Removed[0] != "bash" {
		t.Fatalf("expected bash alias removed, got %v", result.Removed)
	}
}

func TestSystemTools_Coordinator(t *testing.T) {
	svc := NewService()
	tools, err := svc.SystemTools(context.Background(), SystemToolsParams{
		MemberType:  &membertype.CoordinatorType{},
		MemberCount: 3,
		HasReviewer: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	toolSet := map[string]bool{}
	for _, tool := range tools {
		toolSet[tool] = true
	}
	if !toolSet["space"] {
		t.Error("coordinator should have space")
	}
	if !toolSet["task"] {
		t.Error("space coordinator should have task")
	}
}

func TestSystemTools_Worker(t *testing.T) {
	svc := NewService()
	tools, err := svc.SystemTools(context.Background(), SystemToolsParams{
		MemberType:  &membertype.WorkerType{},
		MemberCount: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	toolSet := map[string]bool{}
	for _, tool := range tools {
		toolSet[tool] = true
	}
	if toolSet["shell_exec"] {
		t.Error("worker should not have shell_exec as a system tool")
	}
	if !toolSet["space"] {
		t.Error("worker should have space (list-only by action policy)")
	}
}

func TestSystemTools_Reviewer(t *testing.T) {
	svc := NewService()
	tools, err := svc.SystemTools(context.Background(), SystemToolsParams{
		MemberType:  &membertype.ReviewerType{},
		MemberCount: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	toolSet := map[string]bool{}
	for _, tool := range tools {
		toolSet[tool] = true
	}
	if !toolSet["task"] {
		t.Error("reviewer should have task")
	}
	if toolSet["shell_exec"] {
		t.Error("reviewer should not have shell_exec as a system tool")
	}
	if !toolSet["space"] {
		t.Error("reviewer should have space (list-only by action policy)")
	}
}

func TestDefaults(t *testing.T) {
	svc := NewService()
	result := svc.Defaults(context.Background())
	if len(result.WorkerTools) == 0 {
		t.Error("expected non-empty worker tools")
	}
	if len(result.CoordinatorBase) == 0 {
		t.Error("expected non-empty coordinator base tools")
	}
	if result.CoordinatorWithWorkers == nil {
		t.Error("expected coordinator space tools slice to be initialized")
	}
}
