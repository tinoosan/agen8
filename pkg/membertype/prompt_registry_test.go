package membertype

import (
	"slices"
	"testing"
)

func TestDefaultRegistry_Constructs(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	if reg == nil {
		t.Fatalf("expected registry")
	}
}

func TestDefaultRegistry_RulesForWorker_AreOrdered(t *testing.T) {
	reg := MustDefaultRegistry()
	rules := reg.RulesForKey(TypeWorker)
	if len(rules) == 0 {
		t.Fatalf("expected worker rules")
	}
	for i := 1; i < len(rules); i++ {
		if rules[i-1].Order > rules[i].Order {
			t.Fatalf("rules out of order: %s(%d) before %s(%d)", rules[i-1].Name, rules[i-1].Order, rules[i].Name, rules[i].Order)
		}
	}
}

func TestDefaultRegistry_LockedCoreRules(t *testing.T) {
	reg := MustDefaultRegistry()
	for _, name := range []string{"turn_contract", "review_handling", "ai_identity"} {
		rule, ok := reg.RuleByName(name)
		if !ok {
			t.Fatalf("missing rule %q", name)
		}
		if !rule.Locked {
			t.Fatalf("rule %q must be locked", name)
		}
	}
}

func TestDefaultRegistry_HasAllBuiltinRules(t *testing.T) {
	reg := MustDefaultRegistry()
	all := reg.All()
	if len(all) != 9 {
		t.Fatalf("rule count=%d want=9", len(all))
	}

	names := make([]string, 0, len(all))
	for _, rule := range all {
		names = append(names, rule.Name)
	}

	for _, expected := range []string{
		"turn_contract",
		"coordination_tools",
		"delegation",
		"ai_identity",
		"mission_and_kr",
		"graph_query_autonomy",
		"operator_loop",
		"review_handling",
		"tool_guidance",
	} {
		if !slices.Contains(names, expected) {
			t.Fatalf("missing rule %q in %v", expected, names)
		}
	}
}

func TestBuiltinRuleCatalogMatchesRegistry(t *testing.T) {
	reg := MustDefaultRegistry()

	catalog := BuiltinRuleCatalog()
	if len(catalog) != 9 {
		t.Fatalf("catalog count=%d want=9", len(catalog))
	}

	for _, entry := range catalog {
		rule, ok := reg.RuleByName(entry.Name)
		if !ok {
			t.Fatalf("catalog rule %q missing from registry", entry.Name)
		}
		if rule.Locked != entry.Locked {
			t.Fatalf("catalog lock mismatch for %q: catalog=%v registry=%v", entry.Name, entry.Locked, rule.Locked)
		}
	}
}
