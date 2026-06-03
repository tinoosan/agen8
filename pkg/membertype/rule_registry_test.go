package membertype

import (
	"fmt"
	"testing"
)

func withIsolatedRuleRegistryState(t *testing.T) {
	t.Helper()

	ruleRegistryMu.Lock()
	originalByName := make(map[string]PromptRule, len(ruleByName))
	for name, rule := range ruleByName {
		originalByName[name] = clonePromptRule(rule)
	}
	originalSeq := append([]string(nil), registeredRuleSeq...)
	ruleByName = map[string]PromptRule{}
	registeredRuleSeq = nil
	ruleRegistryMu.Unlock()

	t.Cleanup(func() {
		ruleRegistryMu.Lock()
		ruleByName = originalByName
		registeredRuleSeq = originalSeq
		ruleRegistryMu.Unlock()
	})
}

func TestRegisterRuleDuplicatePanics(t *testing.T) {
	withIsolatedRuleRegistryState(t)

	rule := PromptRule{
		Name:      "dup_rule",
		Order:     100,
		AppliesTo: []MemberTypeName{TypeCoordinator},
		Build:     func(PromptContext) string { return "- dup\n" },
	}
	RegisterRule(rule)

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatalf("expected panic")
		}
		if msg := fmt.Sprint(recovered); msg == "" {
			t.Fatalf("panic message missing")
		}
	}()

	RegisterRule(rule)
}

func TestRegisterRuleMissingBuildPanics(t *testing.T) {
	withIsolatedRuleRegistryState(t)

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatalf("expected panic")
		}
	}()

	RegisterRule(PromptRule{
		Name:      "missing_build",
		Order:     100,
		AppliesTo: []MemberTypeName{TypeCoordinator},
	})
}

func TestAllRegisteredRulesReturnsClone(t *testing.T) {
	withIsolatedRuleRegistryState(t)

	RegisterRule(PromptRule{
		Name:      "alpha_rule",
		Order:     100,
		AppliesTo: []MemberTypeName{TypeCoordinator, TypeWorker},
		Build:     func(PromptContext) string { return "- alpha\n" },
	})

	registered := allRegisteredRules()
	if len(registered) != 1 {
		t.Fatalf("allRegisteredRules len=%d", len(registered))
	}
	if registered[0].Name != "alpha_rule" {
		t.Fatalf("allRegisteredRules name=%q", registered[0].Name)
	}

	registered[0].Name = "mutated"
	registered[0].AppliesTo[0] = TypeReviewer

	again := allRegisteredRules()
	if len(again) != 1 {
		t.Fatalf("allRegisteredRules len=%d", len(again))
	}
	if again[0].Name != "alpha_rule" {
		t.Fatalf("registry mutated name=%q", again[0].Name)
	}
	if again[0].AppliesTo[0] != TypeCoordinator {
		t.Fatalf("registry mutated appliesTo=%v", again[0].AppliesTo)
	}
}
