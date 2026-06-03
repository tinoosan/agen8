package membertype

import (
	"fmt"
	"strings"
	"sync"
)

var (
	ruleRegistryMu    sync.RWMutex
	ruleByName        = map[string]PromptRule{}
	registeredRuleSeq []string
)

// RegisterRule adds a prompt rule to the global registry.
// Panics when the rule is invalid or conflicts with an existing registration.
func RegisterRule(rule PromptRule) {
	name := normalizeRuleName(rule.Name)
	if name == "" {
		panic("membertype: rule name is required")
	}
	if err := rule.Validate(); err != nil {
		panic(fmt.Sprintf("membertype: invalid rule %q: %v", rule.Name, err))
	}

	rule.Name = name

	ruleRegistryMu.Lock()
	defer ruleRegistryMu.Unlock()

	if _, exists := ruleByName[name]; exists {
		panic(fmt.Sprintf("membertype: duplicate rule registration for %q", name))
	}

	ruleByName[name] = clonePromptRule(rule)
	registeredRuleSeq = append(registeredRuleSeq, name)
}

func allRegisteredRules() []PromptRule {
	ruleRegistryMu.RLock()
	defer ruleRegistryMu.RUnlock()

	if len(registeredRuleSeq) == 0 {
		return nil
	}
	out := make([]PromptRule, 0, len(registeredRuleSeq))
	for _, name := range registeredRuleSeq {
		rule, ok := ruleByName[name]
		if !ok {
			continue
		}
		out = append(out, clonePromptRule(rule))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeRuleName(name string) string {
	return strings.TrimSpace(name)
}

func clonePromptRule(rule PromptRule) PromptRule {
	out := rule
	if len(rule.AppliesTo) > 0 {
		out.AppliesTo = append([]MemberTypeName(nil), rule.AppliesTo...)
	}
	if len(rule.Lines) > 0 {
		out.Lines = append([]string(nil), rule.Lines...)
	}
	return out
}
