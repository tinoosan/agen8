package toolguidance

import (
	"strings"
	"testing"

	"github.com/tinoosan/agen8-mcp-server/pkg/membertype"
)

func TestBuildDoesNotEnumerateModuleUsageNotes(t *testing.T) {
	got := build(membertype.PromptContext{MemberType: membertype.TypeCoordinator})
	if !strings.Contains(got, "exact tool names, parameters, enum values, and availability come from the active runtime or harness schemas") {
		t.Fatalf("tool guidance missing runtime schema direction: %s", got)
	}
	if strings.Contains(got, "Prefer `") || strings.Contains(got, "External tool guidance:") {
		t.Fatalf("tool guidance should not enumerate concrete tools: %s", got)
	}
}
