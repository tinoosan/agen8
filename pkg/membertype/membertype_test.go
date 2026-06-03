package membertype

import (
	"slices"
	"strings"
	"testing"
)

// --- Test helpers: create ToolContext for each scenario ---

func workerToolCtx() ToolContext {
	return ToolContext{SpaceID: "space-1", MemberCount: 3}
}

func loneCoordinatorToolCtx() ToolContext {
	return ToolContext{SpaceID: "space-1", MemberCount: 1}
}

func spaceCoordinatorToolCtx() ToolContext {
	return ToolContext{SpaceID: "space-1", MemberCount: 3, HasReviewer: true}
}

func spaceCoordinatorNoReviewerToolCtx() ToolContext {
	return ToolContext{SpaceID: "space-1", MemberCount: 3, HasReviewer: false}
}

func reviewerToolCtx() ToolContext {
	return ToolContext{SpaceID: "space-1", MemberCount: 3, HasReviewer: true}
}

// --- Registry tests ---

func TestLookup_AllRegistered(t *testing.T) {
	for _, name := range []MemberTypeName{TypeCoordinator, TypeLoneCoordinator, TypeWorker, TypeReviewer} {
		at, err := Lookup(name)
		if err != nil {
			t.Fatalf("Lookup(%q) error: %v", name, err)
		}
		if at.Name() != name {
			t.Fatalf("Lookup(%q).Name() = %q", name, at.Name())
		}
	}
}

func TestLookup_Unknown(t *testing.T) {
	_, err := Lookup("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
}

func TestAll_ReturnsAllTypes(t *testing.T) {
	all := All()
	if len(all) != 4 {
		t.Fatalf("expected 4 registered types, got %d", len(all))
	}
}

// --- Resolve tests ---

func TestResolve_Coordinator(t *testing.T) {
	at := Resolve(true, false, 3)
	if at.Name() != TypeCoordinator {
		t.Fatalf("got %q, want %q", at.Name(), TypeCoordinator)
	}
}

func TestResolve_LoneCoordinator(t *testing.T) {
	at := Resolve(true, false, 1)
	if at.Name() != TypeLoneCoordinator {
		t.Fatalf("got %q, want %q", at.Name(), TypeLoneCoordinator)
	}
}

func TestResolve_Worker(t *testing.T) {
	at := Resolve(false, false, 3)
	if at.Name() != TypeWorker {
		t.Fatalf("got %q, want %q", at.Name(), TypeWorker)
	}
}

func TestResolve_Reviewer(t *testing.T) {
	at := Resolve(false, true, 3)
	if at.Name() != TypeReviewer {
		t.Fatalf("got %q, want %q", at.Name(), TypeReviewer)
	}
}

func TestResolve_ReviewerTakesPriority(t *testing.T) {
	// If both isCoordinator and isReviewer are true, reviewer wins.
	at := Resolve(true, true, 3)
	if at.Name() != TypeReviewer {
		t.Fatalf("got %q, want %q", at.Name(), TypeReviewer)
	}
}

// --- SystemTools tests (parity with policy_test.go) ---

func TestSystemTools_Worker(t *testing.T) {
	tools := (&WorkerType{}).SystemTools(workerToolCtx())
	// F39: workers get mission as a system tool for on-demand mission context.
	for _, want := range []string{"task", "mission", "plan", "space", "operator", "decision"} {
		if !slices.Contains(tools, want) {
			t.Fatalf("expected %q in worker tools, got %v", want, tools)
		}
	}
	for _, noWant := range []string{"task_create", "task_list", "task_review"} {
		if slices.Contains(tools, noWant) {
			t.Fatalf("expected %q NOT in worker tools, got %v", noWant, tools)
		}
	}
}

func TestSystemTools_WorkerBasicTools(t *testing.T) {
	ctx := ToolContext{SpaceID: "space-1", MemberCount: 3}
	tools := (&WorkerType{}).SystemTools(ctx)
	for _, want := range []string{"task", "mission", "plan", "space", "operator", "decision"} {
		if !slices.Contains(tools, want) {
			t.Fatalf("expected %q in worker tools, got %v", want, tools)
		}
	}
	for _, noWant := range []string{"task_create"} {
		if slices.Contains(tools, noWant) {
			t.Fatalf("expected %q NOT in worker tools, got %v", noWant, tools)
		}
	}
}

func TestSystemTools_LoneCoordinator(t *testing.T) {
	tools := (&LoneCoordinatorType{}).SystemTools(loneCoordinatorToolCtx())
	for _, want := range []string{"space", "operator", "mission", "plan", "task", "decision"} {
		if !slices.Contains(tools, want) {
			t.Fatalf("expected %q in lone coordinator tools, got %v", want, tools)
		}
	}
	for _, noWant := range []string{"task_create", "task_list", "task_review"} {
		if slices.Contains(tools, noWant) {
			t.Fatalf("expected %q NOT in lone coordinator tools, got %v", noWant, tools)
		}
	}
}

func TestSystemTools_LoneCoordinatorBasicTools(t *testing.T) {
	ctx := ToolContext{SpaceID: "space-1", MemberCount: 1}
	tools := (&LoneCoordinatorType{}).SystemTools(ctx)
	for _, want := range []string{"space", "heartbeat", "operator", "mission", "plan", "task", "decision"} {
		if !slices.Contains(tools, want) {
			t.Fatalf("expected %q in lone coordinator tools, got %v", want, tools)
		}
	}
	for _, noWant := range []string{"task_create", "task_list"} {
		if slices.Contains(tools, noWant) {
			t.Fatalf("expected %q NOT in lone coordinator tools, got %v", noWant, tools)
		}
	}
}

func TestSystemTools_CoordinatorBasicTools(t *testing.T) {
	ctx := ToolContext{SpaceID: "space-1", MemberCount: 3, HasReviewer: true}
	tools := (&CoordinatorType{}).SystemTools(ctx)
	for _, want := range []string{"space", "heartbeat", "operator", "mission", "plan", "task", "decision"} {
		if !slices.Contains(tools, want) {
			t.Fatalf("expected %q in coordinator SystemTools, got %v", want, tools)
		}
	}
	ctxNoReviewer := ToolContext{SpaceID: "space-1", MemberCount: 3, HasReviewer: false}
	toolsNoReviewer := (&CoordinatorType{}).SystemTools(ctxNoReviewer)
	if !slices.Contains(toolsNoReviewer, "task") {
		t.Fatalf("expected task tool when no reviewer, got %v", toolsNoReviewer)
	}
}

func TestSystemTools_ReviewerBasicTools(t *testing.T) {
	ctx := ToolContext{SpaceID: "space-1", MemberCount: 3, HasReviewer: true}
	tools := (&ReviewerType{}).SystemTools(ctx)
	for _, want := range []string{"operator", "task", "space", "mission", "plan", "decision"} {
		if !slices.Contains(tools, want) {
			t.Fatalf("expected %q in reviewer tools, got %v", want, tools)
		}
	}
	for _, noWant := range []string{"task_create", "task_list", "code_exec"} {
		if slices.Contains(tools, noWant) {
			t.Fatalf("expected %q NOT in reviewer tools, got %v", noWant, tools)
		}
	}
}

func TestSystemTools_CoordinatorWithWorkersWithReviewer(t *testing.T) {
	tools := (&CoordinatorType{}).SystemTools(spaceCoordinatorToolCtx())
	for _, want := range []string{"space", "operator", "mission", "plan", "task", "decision"} {
		if !slices.Contains(tools, want) {
			t.Fatalf("expected %q in coordinator-with-workers tools, got %v", want, tools)
		}
	}
}

func TestSystemTools_CoordinatorWithWorkersNoReviewer(t *testing.T) {
	tools := (&CoordinatorType{}).SystemTools(spaceCoordinatorNoReviewerToolCtx())
	for _, want := range []string{"space", "operator", "mission", "plan", "task", "decision"} {
		if !slices.Contains(tools, want) {
			t.Fatalf("expected %q in coordinator (no reviewer) tools, got %v", want, tools)
		}
	}
}

func TestSystemTools_Reviewer(t *testing.T) {
	tools := (&ReviewerType{}).SystemTools(reviewerToolCtx())
	for _, want := range []string{"task", "plan", "decision"} {
		if !slices.Contains(tools, want) {
			t.Fatalf("expected %q in reviewer tools, got %v", want, tools)
		}
	}
	// F39: reviewers also get mission via SystemAlwaysTools so they can check
	// what KR/mission a task under review is serving.
	if !slices.Contains(tools, "mission") {
		t.Fatalf("expected mission in reviewer tools, got %v", tools)
	}
	for _, noWant := range []string{"task_create", "task_list"} {
		if slices.Contains(tools, noWant) {
			t.Fatalf("expected %q NOT in reviewer tools, got %v", noWant, tools)
		}
	}
}

// --- Authorize tests (parity with policy_test.go) ---

func TestAuthorize_DefaultWorkerSet(t *testing.T) {
	result := (&WorkerType{}).Authorize(workerToolCtx(), nil)
	if len(result.Removed) != 0 {
		t.Fatalf("removed=%v", result.Removed)
	}
	defaults := DefaultWorkerAllowedTools
	if len(result.Allowed) != len(defaults) {
		t.Fatalf("allowed=%v want %v", result.Allowed, defaults)
	}
	for i, want := range defaults {
		if result.Allowed[i] != want {
			t.Fatalf("allowed[%d]=%q want %q", i, result.Allowed[i], want)
		}
	}
}

func TestAuthorize_CoordinatorBypassesDefaultWorkerSet(t *testing.T) {
	result := (&CoordinatorType{}).Authorize(spaceCoordinatorToolCtx(), nil)
	if len(result.Removed) != 0 {
		t.Fatalf("removed=%v", result.Removed)
	}
	if result.Allowed != nil {
		t.Fatalf("expected coordinator empty allowed list to bypass filtering, got %v", result.Allowed)
	}
}

func TestAuthorize_ReviewerBypassesDefaultWorkerSet(t *testing.T) {
	result := (&ReviewerType{}).Authorize(reviewerToolCtx(), nil)
	if len(result.Removed) != 0 {
		t.Fatalf("removed=%v", result.Removed)
	}
	if len(result.Allowed) != 0 {
		t.Fatalf("expected reviewer empty allowed, got %v", result.Allowed)
	}
}

func TestAuthorize_WorkerPassesThroughUserTools(t *testing.T) {
	result := (&WorkerType{}).Authorize(workerToolCtx(), []string{" http ", "", "operator"})
	if len(result.Removed) != 1 || result.Removed[0] != "operator" {
		t.Fatalf("removed=%v", result.Removed)
	}
	if len(result.Allowed) != 1 || result.Allowed[0] != "http" {
		t.Fatalf("allowed=%v", result.Allowed)
	}
}

func TestAuthorize_WorkerStripsCoordinatorTools(t *testing.T) {
	result := (&WorkerType{}).Authorize(workerToolCtx(), []string{"http", "space"})
	if len(result.Removed) != 1 || result.Removed[0] != "space" {
		t.Fatalf("removed=%v", result.Removed)
	}
	if len(result.Allowed) != 1 || result.Allowed[0] != "http" {
		t.Fatalf("allowed=%v", result.Allowed)
	}
}

func TestAuthorize_CoordinatorMergesSystemTools(t *testing.T) {
	at := &CoordinatorType{}
	ctx := spaceCoordinatorNoReviewerToolCtx()
	result := at.Authorize(ctx, []string{"operator"})
	if len(result.Removed) != 1 || result.Removed[0] != "operator" {
		t.Fatalf("removed=%v", result.Removed)
	}
	for _, name := range at.SystemTools(ctx) {
		if !slices.Contains(result.Allowed, name) {
			t.Fatalf("expected system tool %q in %v", name, result.Allowed)
		}
	}
	if !slices.Contains(result.Allowed, "operator") {
		t.Fatalf("expected operator in allowed, got %v", result.Allowed)
	}
}

func TestAuthorize_CoordinatorStripsSystemToolsInUserList(t *testing.T) {
	at := &CoordinatorType{}
	ctx := spaceCoordinatorToolCtx()
	result := at.Authorize(ctx, []string{"operator", "space", "read_file"})
	if len(result.Removed) != 2 {
		t.Fatalf("expected 2 removed, got removed=%v", result.Removed)
	}
	if !slices.Contains(result.Removed, "operator") || !slices.Contains(result.Removed, "space") {
		t.Fatalf("expected operator/space stripped, got removed=%v", result.Removed)
	}
	if !slices.Contains(result.Allowed, "operator") {
		t.Fatalf("expected operator in allowed, got %v", result.Allowed)
	}
}

func TestAuthorize_ReviewerStripsSystemTools(t *testing.T) {
	result := (&ReviewerType{}).Authorize(reviewerToolCtx(), []string{"operator", "task", "read_file"})
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
	result := (&WorkerType{}).Authorize(workerToolCtx(), []string{"operator", "read_file", "write_file"})
	if len(result.Removed) != 1 {
		t.Fatalf("expected 1 removed, got removed=%v", result.Removed)
	}
	if len(result.Allowed) != 2 || !slices.Contains(result.Allowed, "read_file") || !slices.Contains(result.Allowed, "write_file") {
		t.Fatalf("expected file ops to remain user tools, got %v", result.Allowed)
	}
}

func TestAuthorize_NoSpaceID_PassThrough(t *testing.T) {
	ctx := ToolContext{} // empty SpaceID
	for _, at := range []MemberType{&CoordinatorType{}, &WorkerType{}, &ReviewerType{}} {
		result := at.Authorize(ctx, []string{"anything"})
		if len(result.Allowed) != 1 || result.Allowed[0] != "anything" {
			t.Fatalf("%s: expected pass-through, got %v", at.Name(), result.Allowed)
		}
	}
}

// --- PromptRules tests ---

func TestPromptRules_CoordinatorContainsTurnContract(t *testing.T) {
	rules := (&CoordinatorType{}).PromptRules(PromptContext{})
	mustContain := []string{
		"TURN CONTRACT",
		"end the turn",
		"Do not call task(action=\"list\")",
		"acceptanceCriteria",
		"task(action=\"review\")",
		"Do not delegate review lifecycle actions to workers",
		"Workers execute (claim/submit/block/release)",
		"review decisions (approve/retry/fail) are coordinator/reviewer responsibilities",
		"Do not perform specialist work",
		"space(action=\"message\")",
		"MUST delegate to specialists",
		"You are a coordinator, not a worker",
		"delegate ALL needed tasks",
		"mission(action=\"list\"|\"get\")",
		"GRAPH CONTEXT LOOP (autonomous execution)",
		`graph_query(action="search", node_type="all", query=...)`,
		"Never guess IDs or relationships",
		"COORDINATION TOOLS (coordinator)",
		"Agen8 coordination rules are your primary operating policy",
		"Repository docs such as AGENTS.md are implementation constraints",
		"Tool schemas and allowed actions are provided by the active runtime or harness",
		`task(action="create") to add work to your own queue`,
	}
	for _, s := range mustContain {
		if !strings.Contains(rules, s) {
			t.Errorf("coordinator rules missing %q", s)
		}
	}
	operatorMustContain := []string{
		`decision(action="ask_user") — use when a coordinator needs structured human input before continuing:`,
		`operator(action="request") — use when the operator must ACT`,
		`Do not use operator(action="request") for "which option should we choose?"`,
		`Treat a returned ask_user answer as authoritative and resolved.`,
		`link the synthesis decision to each workstream decision it draws from`,
	}
	for _, s := range operatorMustContain {
		if !strings.Contains(rules, s) {
			t.Errorf("coordinator operator guidance missing %q", s)
		}
	}
}

func TestPromptRules_CoordinatorDoesNotContainLoneText(t *testing.T) {
	rules := (&CoordinatorType{}).PromptRules(PromptContext{})
	if strings.Contains(rules, "lone coordinator") {
		t.Error("coordinator-with-workers rules should not mention 'lone coordinator'")
	}
}

func TestPromptRules_LoneCoordinatorNoTaskCreate(t *testing.T) {
	rules := (&LoneCoordinatorType{}).PromptRules(PromptContext{})
	mustContain := []string{
		"lone coordinator",
		"space(action=\"list\"|\"message\")",
		"mission",
		"GRAPH CONTEXT LOOP (autonomous execution)",
		`graph_query(action="node", node_id=..., depth=2)`,
		"COORDINATION TOOLS (lone coordinator)",
	}
	for _, s := range mustContain {
		if !strings.Contains(rules, s) {
			t.Errorf("lone coordinator rules missing %q", s)
		}
	}
	mustNotContain := []string{
		"task_create",
		"assignedRole",
		"task(action=\"review\")",
	}
	for _, s := range mustNotContain {
		if strings.Contains(rules, s) {
			t.Errorf("lone coordinator rules should not mention %q", s)
		}
	}
}

func TestPromptRules_WorkerHubAndSpoke(t *testing.T) {
	rules := (&WorkerType{}).PromptRules(PromptContext{SpaceName: "insights", MemberLabel: "researcher"})
	mustContain := []string{
		"hub-and-spoke",
		"communicate only with your coordinator",
		"workspace/insights/researcher/report.md",
		"COORDINATION TOOLS (worker)",
		`Do not use decision(action="ask_user")`,
	}
	for _, s := range mustContain {
		if !strings.Contains(rules, s) {
			t.Errorf("worker rules missing %q", s)
		}
	}
	operatorMustContain := []string{
		`Workers do not use decision(action="ask_user")`,
		`operator(action="request") — use when completing your task requires the operator to ACT`,
		`surface that to your coordinator instead of requesting an action`,
		`After decision(action="log"), immediately call graph_query(action="link")`,
		`edge_type="informed_by"`,
	}
	for _, s := range operatorMustContain {
		if !strings.Contains(rules, s) {
			t.Errorf("worker operator guidance missing %q", s)
		}
	}
	if strings.Contains(rules, "/workspace/") {
		t.Fatalf("worker rules should not instruct legacy /workspace paths; got: %s", rules)
	}
}

func TestPromptRules_ReviewerContract(t *testing.T) {
	rules := (&ReviewerType{}).PromptRules(PromptContext{})
	mustContain := []string{
		"You are the reviewer",
		"task(action=\"review\")",
		"criteria_checked",
		"prose-only reply is not a review decision",
	}
	for _, s := range mustContain {
		if !strings.Contains(rules, s) {
			t.Errorf("reviewer rules missing %q", s)
		}
	}
}

// --- StripPlanning tests ---

func TestStripPlanning(t *testing.T) {
	cases := []struct {
		at   MemberType
		want bool
	}{
		{&CoordinatorType{}, false},
		{&LoneCoordinatorType{}, false},
		{&WorkerType{}, true},
		{&ReviewerType{}, true},
	}
	for _, tc := range cases {
		if got := tc.at.StripPlanning(); got != tc.want {
			t.Errorf("%s.StripPlanning() = %v, want %v", tc.at.Name(), got, tc.want)
		}
	}
}

// --- CanClaimSpaceMessages tests ---

func TestCanClaimSpaceMessages(t *testing.T) {
	cases := []struct {
		at   MemberType
		want bool
	}{
		{&CoordinatorType{}, true},
		{&LoneCoordinatorType{}, true},
		{&WorkerType{}, false},
		{&ReviewerType{}, false},
	}
	for _, tc := range cases {
		if got := tc.at.CanClaimSpaceMessages(); got != tc.want {
			t.Errorf("%s.CanClaimSpaceMessages() = %v, want %v", tc.at.Name(), got, tc.want)
		}
	}
}

// --- CanClaimReviewerMessages tests ---

func TestCanClaimReviewerMessages(t *testing.T) {
	cases := []struct {
		at   MemberType
		ctx  ToolContext
		want bool
	}{
		{&CoordinatorType{}, ToolContext{HasReviewer: false}, true},
		{&CoordinatorType{}, ToolContext{HasReviewer: true}, false},
		{&LoneCoordinatorType{}, ToolContext{}, false},
		{&WorkerType{}, ToolContext{}, false},
		{&ReviewerType{}, ToolContext{}, true},
	}
	for _, tc := range cases {
		if got := tc.at.CanClaimReviewerMessages(tc.ctx); got != tc.want {
			t.Errorf("%s.CanClaimReviewerMessages(HasReviewer=%v) = %v, want %v",
				tc.at.Name(), tc.ctx.HasReviewer, got, tc.want)
		}
	}
}

// --- ToolManifest tests ---

func TestToolManifest_Coordinator(t *testing.T) {
	entries := (&CoordinatorType{}).ToolManifest(spaceCoordinatorToolCtx())
	names := toolEntryNames(entries)
	for _, want := range []string{"task", "space", "mission", "plan"} {
		if !slices.Contains(names, want) {
			t.Errorf("coordinator manifest missing %q, got %v", want, names)
		}
	}
}

func TestToolManifest_LoneCoordinator(t *testing.T) {
	entries := (&LoneCoordinatorType{}).ToolManifest(loneCoordinatorToolCtx())
	names := toolEntryNames(entries)
	for _, want := range []string{"task", "space", "mission", "plan"} {
		if !slices.Contains(names, want) {
			t.Errorf("lone coordinator manifest missing %q, got %v", want, names)
		}
	}
	for _, noWant := range []string{"task_create", "task_list", "task_review"} {
		if slices.Contains(names, noWant) {
			t.Errorf("lone coordinator manifest should not have %q, got %v", noWant, names)
		}
	}
}

func TestToolManifest_Worker(t *testing.T) {
	entries := (&WorkerType{}).ToolManifest(workerToolCtx())
	names := toolEntryNames(entries)
	for _, want := range []string{"task", "plan"} {
		if !slices.Contains(names, want) {
			t.Errorf("worker manifest missing %q, got %v", want, names)
		}
	}
}

func TestToolManifest_Reviewer(t *testing.T) {
	entries := (&ReviewerType{}).ToolManifest(reviewerToolCtx())
	names := toolEntryNames(entries)
	for _, want := range []string{"task", "plan"} {
		if !slices.Contains(names, want) {
			t.Errorf("reviewer manifest missing %q, got %v", want, names)
		}
	}
}

// --- IsCoordinatorType tests ---

func TestIsCoordinatorType(t *testing.T) {
	cases := []struct {
		at   MemberType
		want bool
	}{
		{&CoordinatorType{}, true},
		{&LoneCoordinatorType{}, true},
		{&WorkerType{}, false},
		{&ReviewerType{}, false},
	}
	for _, tc := range cases {
		if got := IsCoordinatorType(tc.at); got != tc.want {
			t.Errorf("IsCoordinatorType(%s) = %v, want %v", tc.at.Name(), got, tc.want)
		}
	}
}

// --- Extension test: proves OCP works ---

type stubPlannerType struct{}

func (s *stubPlannerType) Name() MemberTypeName               { return "planner" }
func (s *stubPlannerType) SystemTools(_ ToolContext) []string { return []string{"plan_create"} }
func (s *stubPlannerType) Authorize(_ ToolContext, req []string) AuthorizationResult {
	return AuthorizationResult{Allowed: req}
}
func (s *stubPlannerType) PromptRules(_ PromptContext) string { return "- You are a planner.\n" }
func (s *stubPlannerType) ToolManifest(_ ToolContext) []ToolEntry {
	return []ToolEntry{{Name: "plan_create"}}
}
func (s *stubPlannerType) StripPlanning() bool                         { return false }
func (s *stubPlannerType) CanClaimSpaceMessages() bool                 { return false }
func (s *stubPlannerType) CanClaimReviewerMessages(_ ToolContext) bool { return false }
func (s *stubPlannerType) ShowAllRoleDescriptions() bool               { return false }
func (s *stubPlannerType) ShowFullProjectTopology() bool               { return false }

func TestExtension_NewTypeCanBeRegistered(t *testing.T) {
	// We can't re-register in the global registry (would panic on duplicate),
	// so we just verify the interface is satisfied and the stub works.
	var at MemberType = &stubPlannerType{}
	if at.Name() != "planner" {
		t.Fatalf("got %q", at.Name())
	}
	if tools := at.SystemTools(ToolContext{}); len(tools) != 1 || tools[0] != "plan_create" {
		t.Fatalf("got %v", tools)
	}
}

// helpers

func toolEntryNames(entries []ToolEntry) []string {
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name
	}
	return names
}
