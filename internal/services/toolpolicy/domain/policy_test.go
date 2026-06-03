package domain

import (
	"slices"
	"testing"

	"github.com/tinoosan/agen8-mcp-server/pkg/membertype"
)

func workerCtx() RoleToolPolicy {
	return NewRoleToolPolicy(RoleToolContext{MemberType: &membertype.WorkerType{}, SpaceID: "space-1"})
}

func loneCoordinatorCtx() RoleToolPolicy {
	return NewRoleToolPolicy(RoleToolContext{MemberType: &membertype.LoneCoordinatorType{}, SpaceID: "space-1", MemberCount: 1})
}

func spaceCoordinatorCtx() RoleToolPolicy {
	return NewRoleToolPolicy(RoleToolContext{MemberType: &membertype.CoordinatorType{}, SpaceID: "space-1", MemberCount: 3, HasReviewer: true})
}

func spaceCoordinatorNoReviewerCtx() RoleToolPolicy {
	return NewRoleToolPolicy(RoleToolContext{MemberType: &membertype.CoordinatorType{}, SpaceID: "space-1", MemberCount: 3, HasReviewer: false})
}

func reviewerCtx() RoleToolPolicy {
	return NewRoleToolPolicy(RoleToolContext{MemberType: &membertype.ReviewerType{}, SpaceID: "space-1"})
}

// --- SystemTools taxonomy tests ---

func TestSystemTools_Worker(t *testing.T) {
	tools := workerCtx().SystemTools()
	for _, want := range []string{"task", "space", "mission"} {
		if !slices.Contains(tools, want) {
			t.Fatalf("expected %q in worker tools, got %v", want, tools)
		}
	}
	for _, noWant := range []string{"code_exec"} {
		if slices.Contains(tools, noWant) {
			t.Fatalf("expected %q NOT in worker tools, got %v", noWant, tools)
		}
	}
}

func TestSystemTools_WorkerBasicTools(t *testing.T) {
	p := NewRoleToolPolicy(RoleToolContext{MemberType: &membertype.WorkerType{}, SpaceID: "space-1"})
	tools := p.SystemTools()
	if !slices.Contains(tools, "space") {
		t.Fatalf("expected space in worker tools, got %v", tools)
	}
}

func TestSystemTools_LoneCoordinator(t *testing.T) {
	tools := loneCoordinatorCtx().SystemTools()
	for _, want := range []string{"space", "operator", "mission", "task"} {
		if !slices.Contains(tools, want) {
			t.Fatalf("expected %q in lone coordinator tools, got %v", want, tools)
		}
	}
}

func TestSystemTools_LoneCoordinatorBasicTools(t *testing.T) {
	p := NewRoleToolPolicy(RoleToolContext{MemberType: &membertype.LoneCoordinatorType{}, SpaceID: "space-1", MemberCount: 1})
	tools := p.SystemTools()
	if !slices.Contains(tools, "space") {
		t.Fatalf("expected space in lone coordinator tools, got %v", tools)
	}
}

func TestSystemTools_CoordinatorWithWorkersWithReviewer(t *testing.T) {
	tools := spaceCoordinatorCtx().SystemTools()
	for _, want := range []string{"space", "operator", "mission", "task"} {
		if !slices.Contains(tools, want) {
			t.Fatalf("expected %q in space coordinator (with reviewer) tools, got %v", want, tools)
		}
	}
}

func TestSystemTools_CoordinatorWithWorkersNoReviewer(t *testing.T) {
	tools := spaceCoordinatorNoReviewerCtx().SystemTools()
	for _, want := range []string{"space", "operator", "mission", "task"} {
		if !slices.Contains(tools, want) {
			t.Fatalf("expected %q in space coordinator (no reviewer) tools, got %v", want, tools)
		}
	}
}

func TestSystemTools_Reviewer(t *testing.T) {
	tools := reviewerCtx().SystemTools()
	for _, want := range []string{"task", "space", "mission"} {
		if !slices.Contains(tools, want) {
			t.Fatalf("expected %q in reviewer tools, got %v", want, tools)
		}
	}
}

// --- Authorize tests ---

func TestAuthorize_DefaultWorkerSet(t *testing.T) {
	result := workerCtx().Authorize(nil)
	if len(result.Removed) != 0 {
		t.Fatalf("removed=%v", result.Removed)
	}
	defaults := DefaultWorkerTools()
	if len(result.Allowed) != len(defaults) {
		t.Fatalf("allowed=%v want %v", result.Allowed, defaults)
	}
	for i, want := range defaults {
		if result.Allowed[i] != want {
			t.Fatalf("allowed[%d]=%q want %q", i, result.Allowed[i], want)
		}
	}
	if slices.Contains(result.Allowed, "task") {
		t.Fatalf("task should not be in default worker tools, got %v", result.Allowed)
	}
}

func TestAuthorize_DefaultWorkerSetExcludesFinalAnswer(t *testing.T) {
	result := workerCtx().Authorize(nil)
	for _, name := range result.Allowed {
		if name == "final_answer" {
			t.Fatalf("did not expect final_answer in worker default set")
		}
	}
}

func TestAuthorize_CoordinatorBypassesDefaultWorkerSet(t *testing.T) {
	result := spaceCoordinatorCtx().Authorize(nil)
	if len(result.Removed) != 0 {
		t.Fatalf("removed=%v", result.Removed)
	}
	if result.Allowed != nil {
		t.Fatalf("expected coordinator empty allowed list to bypass filtering, got %v", result.Allowed)
	}
}

func TestAuthorize_ReviewerBypassesDefaultWorkerSet(t *testing.T) {
	result := reviewerCtx().Authorize(nil)
	if len(result.Removed) != 0 {
		t.Fatalf("removed=%v", result.Removed)
	}
	if len(result.Allowed) != 0 {
		t.Fatalf("expected reviewer empty allowed list to bypass filtering, got %v", result.Allowed)
	}
}

func TestAuthorize_ProfileAllowedToolsPassedThrough(t *testing.T) {
	result := workerCtx().Authorize([]string{" http ", "", "operator"})
	if len(result.Removed) != 1 || result.Removed[0] != "operator" {
		t.Fatalf("removed=%v", result.Removed)
	}
	if len(result.Allowed) != 1 || result.Allowed[0] != "http" {
		t.Fatalf("allowed=%v", result.Allowed)
	}
}

func TestAuthorize_RemovesCoordinatorOnlyToolsForWorker(t *testing.T) {
	result := workerCtx().Authorize([]string{"http", "space"})
	if len(result.Removed) != 1 || result.Removed[0] != "space" {
		t.Fatalf("removed=%v", result.Removed)
	}
	if len(result.Allowed) != 1 || result.Allowed[0] != "http" {
		t.Fatalf("allowed=%v", result.Allowed)
	}
}

func TestAuthorize_CoordinatorMergesRequiredTools(t *testing.T) {
	p := spaceCoordinatorNoReviewerCtx()
	result := p.Authorize([]string{"http"})
	if len(result.Removed) != 0 {
		t.Fatalf("removed=%v", result.Removed)
	}
	for _, name := range p.SystemTools() {
		if !slices.Contains(result.Allowed, name) {
			t.Fatalf("expected system tool %q in %v", name, result.Allowed)
		}
	}
	if !slices.Contains(result.Allowed, "http") {
		t.Fatalf("expected original allowed tool http to remain in %v", result.Allowed)
	}
}

func TestAuthorize_CoordinatorWithSystemToolsInAllowed(t *testing.T) {
	p := spaceCoordinatorCtx()
	result := p.Authorize([]string{"operator", "space", "read_file"})
	if len(result.Removed) != 2 {
		t.Fatalf("expected 2 removed (operator, space), got removed=%v", result.Removed)
	}
	if !slices.Contains(result.Removed, "operator") || !slices.Contains(result.Removed, "space") {
		t.Fatalf("expected operator/space stripped, got removed=%v", result.Removed)
	}
	if !slices.Contains(result.Allowed, "operator") {
		t.Fatalf("expected operator in allowed, got %v", result.Allowed)
	}
	for _, name := range p.SystemTools() {
		if !slices.Contains(result.Allowed, name) {
			t.Fatalf("expected system tool %q in allowed %v", name, result.Allowed)
		}
	}
}

func TestAuthorize_ReviewerStripsSystemTools(t *testing.T) {
	result := reviewerCtx().Authorize([]string{"operator", "task", "read_file"})
	if len(result.Removed) != 2 {
		t.Fatalf("expected 2 removed, got removed=%v", result.Removed)
	}
	if !slices.Contains(result.Removed, "operator") || !slices.Contains(result.Removed, "task") {
		t.Fatalf("expected operator/task stripped, got removed=%v", result.Removed)
	}
	if len(result.Allowed) != 1 || result.Allowed[0] != "read_file" {
		t.Fatalf("expected read_file to remain user-scoped, got %v", result.Allowed)
	}
}

func TestAuthorize_WorkerRetainsNonSystemTools(t *testing.T) {
	result := workerCtx().Authorize([]string{"operator", "read_file", "write_file"})
	if len(result.Removed) != 1 {
		t.Fatalf("expected 1 removed (operator), got removed=%v", result.Removed)
	}
	if len(result.Allowed) != 2 || !slices.Contains(result.Allowed, "read_file") || !slices.Contains(result.Allowed, "write_file") {
		t.Fatalf("expected fs_* to remain user tools, got %v", result.Allowed)
	}
}

// --- ResolveCodeExecOnly tests ---

// --- StripSystemTools tests (via IsSystemTool) ---

func TestIsSystemTool_Contracts(t *testing.T) {
	p := workerCtx()
	if p.IsSystemTool("read_file") {
		t.Fatal("expected read_file to be non-system after removal")
	}
	if p.IsSystemTool("write_file") {
		t.Fatal("expected write_file to be non-system after removal")
	}
	if p.IsSystemTool("browser") {
		t.Fatal("expected browser to be non-system after removal")
	}
	if p.IsSystemTool("shell_exec") {
		t.Fatal("expected shell_exec to be non-system after removal")
	}
}

func TestIsSystemTool_CoordinatorSystemTools(t *testing.T) {
	p := spaceCoordinatorNoReviewerCtx()
	for _, name := range []string{"space", "task"} {
		if !p.IsSystemTool(name) {
			t.Fatalf("expected %q to be system tool for coordinator", name)
		}
	}
	if p.IsSystemTool("browser") {
		t.Fatal("expected browser to be non-system after removal")
	}
}
