package domain

var claudeCLIEfforts = []string{"low", "medium", "high", "xhigh", "max"}
var codexEfforts = []string{"low", "medium", "high", "xhigh"}

var claudeCLIModels = []ModelEntry{
	{ID: "claude-opus-4-8", Efforts: claudeCLIEfforts},
	{ID: "claude-opus-4-7", Efforts: claudeCLIEfforts},
	{ID: "claude-sonnet-4-6", Efforts: claudeCLIEfforts},
	{ID: "claude-opus-4-6", Efforts: claudeCLIEfforts},
	{ID: "claude-opus-4-5-20251101", Efforts: claudeCLIEfforts},
	{ID: "claude-haiku-4-5-20251001", Efforts: claudeCLIEfforts},
	{ID: "claude-sonnet-4-5-20250929", Efforts: claudeCLIEfforts},
}

var codexModels = []ModelEntry{
	{ID: "gpt-5.5", Efforts: codexEfforts},
	{ID: "gpt-5.4", Efforts: codexEfforts},
	{ID: "gpt-5.4-mini", Efforts: codexEfforts},
	{ID: "gpt-5.3-codex", Efforts: codexEfforts},
	{ID: "gpt-5.3-codex-spark", Efforts: codexEfforts},
	{ID: "gpt-5.2", Efforts: codexEfforts},
	{ID: "gpt-5.2-codex", Efforts: codexEfforts},
	{ID: "gpt-5.1-codex-max", Efforts: codexEfforts},
	{ID: "gpt-5.1-codex-mini", Efforts: codexEfforts},
}

var codexPermissionModes = []PermissionModeEntry{
	{
		ID:          "codex/default",
		Name:        "Default permissions",
		Description: "Use the user's Codex configuration without Agen8 permission overrides.",
	},
	{
		ID:          "codex/auto-review",
		Name:        "Auto-review",
		Description: "Route approval requests through Codex auto-review with a sandboxed execution profile.",
	},
	{
		ID:          "codex/full-access",
		Name:        "Full access",
		Description: "Run without approval prompts and without an outer sandbox.",
		Default:     true,
	},
	{
		ID:                "codex/custom-config",
		Name:              "Custom config.toml",
		Description:       "Load explicit Codex config overrides from a selected config file.",
		RequiresConfigRef: true,
	},
}

var claudeCLIPermissionModes = []PermissionModeEntry{
	{
		ID:          "claude-code/ask-permissions",
		Name:        "Ask permissions",
		Description: "Use Claude Code default mode and ask for protected tool use.",
	},
	{ID: "claude-code/accept-edits", Name: "Accept edits", Description: "Use Claude Code acceptEdits permission mode."},
	{ID: "claude-code/plan", Name: "Plan mode", Description: "Use Claude Code plan permission mode."},
	{ID: "claude-code/auto", Name: "Auto mode", Description: "Use Claude Code auto permission mode when supported by the installed binary."},
	{ID: "claude-code/bypass-permissions", Name: "Bypass permissions", Description: "Use Claude Code bypassPermissions mode.", Default: true},
}

func DefaultCatalog() *Catalog {
	return NewCatalog(
		HarnessEntry{Kind: "claude-cli", Models: claudeCLIModels, PermissionModes: claudeCLIPermissionModes},
		HarnessEntry{Kind: "codex", Models: codexModels, PermissionModes: codexPermissionModes},
	)
}
