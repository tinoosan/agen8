package domain

// RoleDefinitionTool optionally exposes a role-aware tool definition. This is
// used when the callable schema/description must differ by member role even
// though the canonical tool name remains the same.
type RoleDefinitionTool interface {
	DefinitionForRole(policy RolePolicy) ToolDefinition
}
