package membertype

// PromptRuleCatalogEntry contains non-rendering metadata for a built-in rule.
// This catalog is used by config validation paths that must not depend on
// runtime rule module imports.
type PromptRuleCatalogEntry struct {
	Name   string
	Locked bool
}

var builtinRuleCatalog = []PromptRuleCatalogEntry{
	{Name: "turn_contract", Locked: true},
	{Name: "coordination_tools", Locked: false},
	{Name: "delegation", Locked: false},
	{Name: "ai_identity", Locked: true},
	{Name: "mission_and_kr", Locked: false},
	{Name: "graph_query_autonomy", Locked: false},
	{Name: "operator_loop", Locked: false},
	{Name: "review_handling", Locked: true},
	{Name: "tool_guidance", Locked: false},
}

func BuiltinRuleCatalog() []PromptRuleCatalogEntry {
	out := make([]PromptRuleCatalogEntry, len(builtinRuleCatalog))
	copy(out, builtinRuleCatalog)
	return out
}
