package rules

import (
	"fmt"
	"sort"
	"strings"
)

type registryEntry[Ctx any, Key comparable] struct {
	rule  Rule[Ctx, Key]
	index int
}

// Registry stores validated rules and provides lookup/query helpers.
type Registry[Ctx any, Key comparable] struct {
	entries    []registryEntry[Ctx, Key]
	nameToRule map[string]Rule[Ctx, Key]
}

// NewRegistry validates and constructs a registry.
func NewRegistry[Ctx any, Key comparable](rules []Rule[Ctx, Key]) (*Registry[Ctx, Key], error) {
	entries := make([]registryEntry[Ctx, Key], 0, len(rules))
	nameToRule := make(map[string]Rule[Ctx, Key], len(rules))
	for i, item := range rules {
		if err := item.Validate(); err != nil {
			return nil, err
		}
		name := normalizeRuleName(item.Name)
		if _, exists := nameToRule[name]; exists {
			return nil, fmt.Errorf("duplicate rule name %q", strings.TrimSpace(item.Name))
		}
		cloned := item.clone()
		entries = append(entries, registryEntry[Ctx, Key]{rule: cloned, index: i})
		nameToRule[name] = cloned
	}
	return &Registry[Ctx, Key]{entries: entries, nameToRule: nameToRule}, nil
}

// All returns all rules in registration order.
func (r *Registry[Ctx, Key]) All() []Rule[Ctx, Key] {
	if r == nil || len(r.entries) == 0 {
		return nil
	}
	out := make([]Rule[Ctx, Key], 0, len(r.entries))
	for _, entry := range r.entries {
		out = append(out, entry.rule.clone())
	}
	return out
}

// Has reports whether a named rule exists.
func (r *Registry[Ctx, Key]) Has(name string) bool {
	if r == nil {
		return false
	}
	_, ok := r.nameToRule[normalizeRuleName(name)]
	return ok
}

// RuleByName resolves a named rule.
func (r *Registry[Ctx, Key]) RuleByName(name string) (Rule[Ctx, Key], bool) {
	if r == nil {
		var zero Rule[Ctx, Key]
		return zero, false
	}
	rule, ok := r.nameToRule[normalizeRuleName(name)]
	if !ok {
		var zero Rule[Ctx, Key]
		return zero, false
	}
	return rule.clone(), true
}

// RulesForKey returns rules applicable to key in deterministic order.
func (r *Registry[Ctx, Key]) RulesForKey(key Key) []Rule[Ctx, Key] {
	entries := r.entriesForKey(key)
	if len(entries) == 0 {
		return nil
	}
	out := make([]Rule[Ctx, Key], 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.rule.clone())
	}
	return out
}

func (r *Registry[Ctx, Key]) entriesForKey(key Key) []registryEntry[Ctx, Key] {
	if r == nil || len(r.entries) == 0 {
		return nil
	}
	out := make([]registryEntry[Ctx, Key], 0, len(r.entries))
	for _, entry := range r.entries {
		if !entry.rule.appliesTo(key) {
			continue
		}
		out = append(out, entry)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].rule.Order != out[j].rule.Order {
			return out[i].rule.Order < out[j].rule.Order
		}
		if out[i].index != out[j].index {
			return out[i].index < out[j].index
		}
		return normalizeRuleName(out[i].rule.Name) < normalizeRuleName(out[j].rule.Name)
	})
	return out
}
