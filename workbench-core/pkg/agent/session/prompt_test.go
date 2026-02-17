package session

import (
	"strings"
	"testing"

	"github.com/tinoosan/workbench-core/pkg/profile"
)

func TestBuildTeamBlock(t *testing.T) {
	block := buildTeamBlock("team-123", "researcher", "head-analyst", []string{"head-analyst", "researcher", "report-writer"}, nil)
	if block == "" {
		t.Fatalf("expected non-empty team block")
	}
	if !strings.Contains(block, `Your role: "researcher"`) {
		t.Fatalf("expected role in team block, got: %s", block)
	}
	if !strings.Contains(block, `assignedRole="head-analyst"`) {
		t.Fatalf("expected escalation target in team block, got: %s", block)
	}
	if !strings.Contains(block, `/workspace/<your-role>/...`) {
		t.Fatalf("expected worker workspace guidance in team block, got: %s", block)
	}
	if !strings.Contains(block, `/workspace/researcher/report.pdf`) {
		t.Fatalf("expected worker role-prefixed example path, got: %s", block)
	}
	if !strings.Contains(block, `/tasks/<your-role>/<date>/<taskID>/SUMMARY.md`) {
		t.Fatalf("expected worker task summary guidance in team block, got: %s", block)
	}
	if !strings.Contains(block, "Planning notes (plans/checklists) are internal working notes") {
		t.Fatalf("expected internal notes guidance in team block, got: %s", block)
	}
}

func TestBuildSystemPrompt_IncludesTeamBlock(t *testing.T) {
	p := profile.Profile{
		ID:          "team-profile",
		Description: "desc",
	}
	out := buildSystemPrompt("base prompt", p, "profile prompt", nil, "", "team-1", "head-analyst", "head-analyst", []string{"head-analyst", "researcher"}, nil)
	if !strings.Contains(out, "<team>") {
		t.Fatalf("expected team block in system prompt")
	}
	if !strings.Contains(out, "<context>") || !strings.Contains(out, "Current date and time:") {
		t.Fatalf("expected context block with current date and time, got: %s", out)
	}
	if !strings.Contains(out, "All roles: head-analyst, researcher") {
		t.Fatalf("expected role list in system prompt, got: %s", out)
	}
}

func TestBuildSystemPrompt_IncludesRoleDescriptions(t *testing.T) {
	p := profile.Profile{
		ID:          "team-profile",
		Description: "desc",
	}
	out := buildSystemPrompt(
		"base prompt",
		p,
		"profile prompt",
		nil,
		"",
		"team-1",
		"head-analyst",
		"head-analyst",
		[]string{"head-analyst", "researcher"},
		map[string]string{
			"head-analyst": "coordinates",
			"researcher":   "collects evidence",
		},
	)
	if !strings.Contains(out, "Role descriptions:") {
		t.Fatalf("expected role descriptions in system prompt")
	}
	if !strings.Contains(out, "- researcher: collects evidence") {
		t.Fatalf("expected researcher role description, got: %s", out)
	}
}

func TestBuildTeamBlock_CoordinatorRestrictionsAndMemoryPolicy(t *testing.T) {
	block := buildTeamBlock("team-1", "head-analyst", "head-analyst", []string{"head-analyst", "researcher"}, nil)
	if !strings.Contains(block, "MUST NOT perform specialist work unless it is a job for your role") {
		t.Fatalf("expected strict coordinator restriction, got: %s", block)
	}
	if !strings.Contains(block, "NEVER use web_search, file tools, or shell tools for specialist work") {
		t.Fatalf("expected coordinator tool restriction, got: %s", block)
	}
	if !strings.Contains(block, "Use WriteMemory and AppendMemory tools for memory updates") {
		t.Fatalf("expected memory tool guidance, got: %s", block)
	}
	if !strings.Contains(block, "/workspace/<target-role>/...") {
		t.Fatalf("expected coordinator team-root review guidance, got: %s", block)
	}
	if !strings.Contains(block, "/tasks/<role>/<date>/<taskID>/SUMMARY.md") {
		t.Fatalf("expected coordinator task summary review guidance, got: %s", block)
	}
	if !strings.Contains(block, "do not create or expect coordinator review callbacks") {
		t.Fatalf("expected explicit coordinator self-task callback guidance, got: %s", block)
	}
}

func TestBuildWorkerTeamRules_ExcludesCoordinatorOnlyRestrictions(t *testing.T) {
	rules := buildWorkerTeamRules("researcher")
	if strings.Contains(rules, "MUST NOT perform specialist research, analysis, or report writing") {
		t.Fatalf("worker rules should not include coordinator-only specialist restriction: %s", rules)
	}
	if !strings.Contains(rules, "/workspace/<your-role>/...") {
		t.Fatalf("expected worker workspace guidance, got: %s", rules)
	}
}

func TestBuildCoordinatorTeamRules_IncludesNoSelfReviewGuidance(t *testing.T) {
	rules := buildCoordinatorTeamRules()
	if !strings.Contains(rules, "do not create or expect coordinator review callbacks") {
		t.Fatalf("expected no-self-review guidance in coordinator rules, got: %s", rules)
	}
	if !strings.Contains(rules, "/workspace/<target-role>/...") {
		t.Fatalf("expected coordinator workspace guidance, got: %s", rules)
	}
}
