package rules

import (
	"fmt"
	"sort"
	"strings"
)

// RuleOverride adds a new rule for a specific composition call.
type RuleOverride[Ctx any, Key comparable] struct {
	Rule   Rule[Ctx, Key]
	Source Source
}

// DisableOverride disables a named rule.
type DisableOverride struct {
	Name   string
	Source Source
}

// AppendOverride appends lines to a named rule.
type AppendOverride struct {
	Name   string
	Lines  []string
	Source Source
}

// ComposeOptions configures one prompt composition call.
type ComposeOptions[Ctx any, Key comparable] struct {
	Registry *Registry[Ctx, Key]
	Key      Key
	Context  Ctx
	Add      []RuleOverride[Ctx, Key]
	Disable  []DisableOverride
	Append   []AppendOverride
}

type ruleDefinition struct {
	name   string
	locked bool
}

type appendContribution struct {
	lines  []string
	source Source
}

type composeEntry[Ctx any, Key comparable] struct {
	rule     Rule[Ctx, Key]
	source   Source
	builtIn  bool
	index    int
	disabled bool
	appends  []appendContribution
}

// Compose renders prompt text and provenance from registry rules and overrides.
func Compose[Ctx any, Key comparable](opts ComposeOptions[Ctx, Key]) (ComposeResult, error) {
	if opts.Registry == nil {
		return ComposeResult{}, fmt.Errorf("registry is required")
	}

	definitions := make(map[string]ruleDefinition)
	for _, item := range opts.Registry.All() {
		norm := normalizeRuleName(item.Name)
		definitions[norm] = ruleDefinition{name: strings.TrimSpace(item.Name), locked: item.Locked}
	}

	entriesByName := make(map[string]*composeEntry[Ctx, Key])
	entries := make([]*composeEntry[Ctx, Key], 0)
	for _, item := range opts.Registry.entriesForKey(opts.Key) {
		norm := normalizeRuleName(item.rule.Name)
		entry := &composeEntry[Ctx, Key]{
			rule:    item.rule.clone(),
			source:  Source{Kind: "built-in", Path: strings.TrimSpace(item.rule.Name)},
			builtIn: true,
			index:   item.index,
		}
		entries = append(entries, entry)
		entriesByName[norm] = entry
	}

	addBaseIndex := len(opts.Registry.entries)
	for i, override := range opts.Add {
		rule := override.Rule
		if err := rule.Validate(); err != nil {
			return ComposeResult{}, err
		}
		norm := normalizeRuleName(rule.Name)
		if _, exists := definitions[norm]; exists {
			return ComposeResult{}, fmt.Errorf("rule %q already exists", strings.TrimSpace(rule.Name))
		}
		definitions[norm] = ruleDefinition{name: strings.TrimSpace(rule.Name), locked: rule.Locked}
		if !rule.appliesTo(opts.Key) {
			continue
		}
		source := override.Source
		if source.Kind == "" {
			source = Source{Kind: "override", Path: strings.TrimSpace(rule.Name)}
		}
		entry := &composeEntry[Ctx, Key]{
			rule:    rule.clone(),
			source:  source,
			builtIn: false,
			index:   addBaseIndex + i,
		}
		entries = append(entries, entry)
		entriesByName[norm] = entry
	}

	for _, disable := range opts.Disable {
		norm := normalizeRuleName(disable.Name)
		if norm == "" {
			return ComposeResult{}, fmt.Errorf("disable rule name is required")
		}
		def, ok := definitions[norm]
		if !ok {
			return ComposeResult{}, fmt.Errorf("unknown rule %q", strings.TrimSpace(disable.Name))
		}
		if def.locked {
			return ComposeResult{}, fmt.Errorf("rule %q is locked and cannot be disabled", def.name)
		}
		if entry := entriesByName[norm]; entry != nil {
			entry.disabled = true
		}
	}

	for _, appendOverride := range opts.Append {
		norm := normalizeRuleName(appendOverride.Name)
		if norm == "" {
			return ComposeResult{}, fmt.Errorf("append rule name is required")
		}
		def, ok := definitions[norm]
		if !ok {
			return ComposeResult{}, fmt.Errorf("unknown rule %q", strings.TrimSpace(appendOverride.Name))
		}
		if def.locked {
			return ComposeResult{}, fmt.Errorf("rule %q is locked and cannot be appended", def.name)
		}
		lines := normalizeAppendLines(appendOverride.Lines)
		if len(lines) == 0 {
			return ComposeResult{}, fmt.Errorf("append for rule %q must include at least one non-empty line", def.name)
		}
		if entry := entriesByName[norm]; entry != nil {
			source := appendOverride.Source
			if source.Kind == "" {
				source = Source{Kind: "append", Path: def.name}
			}
			entry.appends = append(entry.appends, appendContribution{lines: lines, source: source})
		}
	}

	sort.SliceStable(entries, func(i, j int) bool {
		left := entries[i]
		right := entries[j]
		if left.rule.Order != right.rule.Order {
			return left.rule.Order < right.rule.Order
		}
		if left.index != right.index {
			return left.index < right.index
		}
		return normalizeRuleName(left.rule.Name) < normalizeRuleName(right.rule.Name)
	})

	var out strings.Builder
	provenance := make([]Provenance, 0)
	for _, entry := range entries {
		if entry.disabled {
			continue
		}
		base := entry.rule.renderBase(opts.Context)
		baseLines := renderedLines(strings.TrimSpace(base))
		combinedRaw := base
		if len(baseLines) > 0 {
			provenance = append(provenance, Provenance{
				RuleName: strings.TrimSpace(entry.rule.Name),
				Order:    entry.rule.Order,
				Locked:   entry.rule.Locked,
				BuiltIn:  entry.builtIn,
				Source:   entry.source,
				Lines:    baseLines,
			})
		}
		for _, appendContribution := range entry.appends {
			appendText := joinLines(appendContribution.lines)
			if appendText == "" {
				continue
			}
			combinedRaw += appendText
			provenance = append(provenance, Provenance{
				RuleName: strings.TrimSpace(entry.rule.Name),
				Order:    entry.rule.Order,
				Locked:   entry.rule.Locked,
				BuiltIn:  false,
				Source:   appendContribution.source,
				Lines:    renderedLines(strings.TrimSpace(appendText)),
			})
		}
		combined := strings.TrimSpace(combinedRaw)
		if combined == "" {
			continue
		}
		if out.Len() > 0 {
			out.WriteString("\n")
		}
		out.WriteString(combined)
	}

	return ComposeResult{Prompt: out.String(), Provenance: provenance}, nil
}

func normalizeAppendLines(lines []string) []string {
	if len(lines) == 0 {
		return nil
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
