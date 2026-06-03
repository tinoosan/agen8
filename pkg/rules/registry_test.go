package rules

import (
	"strings"
	"testing"
)

type testKey string

type testCtx struct{}

func TestNewRegistry_RejectsBothLinesAndBuild(t *testing.T) {
	_, err := NewRegistry([]Rule[testCtx, testKey]{
		{
			Name:      "bad",
			AppliesTo: []testKey{"worker"},
			Lines:     []string{"- one"},
			Build: func(_ testCtx) string {
				return "- two\n"
			},
		},
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "must not set both lines and build") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewRegistry_RejectsNeitherLinesNorBuild(t *testing.T) {
	_, err := NewRegistry([]Rule[testCtx, testKey]{
		{Name: "bad", AppliesTo: []testKey{"worker"}},
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "must set one of lines or build") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewRegistry_RejectsDuplicateNamesExact(t *testing.T) {
	_, err := NewRegistry([]Rule[testCtx, testKey]{
		{Name: "turn_contract", AppliesTo: []testKey{"worker"}, Lines: []string{"- one"}},
		{Name: "turn_contract", AppliesTo: []testKey{"coordinator"}, Lines: []string{"- two"}},
	})
	if err == nil {
		t.Fatalf("expected duplicate-name error")
	}
}

func TestRegistry_RulesForKey_ReturnsOrderedRules(t *testing.T) {
	reg, err := NewRegistry([]Rule[testCtx, testKey]{
		{Name: "third", Order: 30, AppliesTo: []testKey{"worker"}, Lines: []string{"- third"}},
		{Name: "first", Order: 10, AppliesTo: []testKey{"worker"}, Lines: []string{"- first"}},
		{Name: "second", Order: 20, AppliesTo: []testKey{"worker", "coordinator"}, Lines: []string{"- second"}},
		{Name: "other", Order: 5, AppliesTo: []testKey{"coordinator"}, Lines: []string{"- other"}},
	})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	got := reg.RulesForKey("worker")
	if len(got) != 3 {
		t.Fatalf("len=%d want=3", len(got))
	}
	if got[0].Name != "first" || got[1].Name != "second" || got[2].Name != "third" {
		t.Fatalf("unexpected order: %#v", []string{got[0].Name, got[1].Name, got[2].Name})
	}
}

func TestRegistry_RuleByName_IsCaseSensitive(t *testing.T) {
	reg, err := NewRegistry([]Rule[testCtx, testKey]{
		{Name: "review_handling", Order: 10, AppliesTo: []testKey{"worker"}, Lines: []string{"- review"}},
	})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	rule, ok := reg.RuleByName("review_handling")
	if !ok {
		t.Fatalf("expected lookup success")
	}
	if rule.Name != "review_handling" {
		t.Fatalf("name=%q", rule.Name)
	}
	if _, ok := reg.RuleByName("REVIEW_HANDLING"); ok {
		t.Fatalf("expected case-sensitive lookup miss")
	}
}
