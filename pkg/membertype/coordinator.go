package membertype

import (
	"strings"
)

// CoordinatorType implements MemberType for coordinators that can delegate to workers.
type CoordinatorType struct{}

func (c *CoordinatorType) Name() MemberTypeName { return TypeCoordinator }

func (c *CoordinatorType) SystemTools(_ ToolContext) []string {
	tools := append([]string{}, SystemAlwaysTools...)
	tools = append(tools, CoordinatorBaseTools...)
	tools = append(tools, CoordinatorWithWorkersTools...)
	return tools
}

func (c *CoordinatorType) Authorize(ctx ToolContext, requestedTools []string) AuthorizationResult {
	if strings.TrimSpace(ctx.SpaceID) == "" {
		return AuthorizationResult{Allowed: append([]string(nil), requestedTools...)}
	}
	if len(requestedTools) == 0 {
		return AuthorizationResult{}
	}
	sysTools := c.SystemTools(ctx)
	userTools, stripped := StripSystemTools(sysTools, requestedTools)

	seen := make(map[string]struct{}, len(userTools)+len(sysTools))
	merged := make([]string, 0, len(userTools)+len(sysTools))
	for _, name := range userTools {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		merged = append(merged, name)
	}
	for _, name := range sysTools {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		merged = append(merged, name)
	}
	return AuthorizationResult{Allowed: merged, Removed: stripped}
}

func (c *CoordinatorType) PromptRules(ctx PromptContext) string {
	return renderPromptRules(TypeCoordinator, ctx)
}

func (c *CoordinatorType) ToolManifest(_ ToolContext) []ToolEntry {
	entries := []ToolEntry{
		{Name: "task", Required: true},
		{Name: "space", Required: true},
		{Name: "heartbeat", Required: true},
		{Name: "mission", Required: true},
		{Name: "plan", Required: true},
	}
	return entries
}

func (c *CoordinatorType) StripPlanning() bool { return false }

func (c *CoordinatorType) CanClaimSpaceMessages() bool { return true }

func (c *CoordinatorType) CanClaimReviewerMessages(ctx ToolContext) bool {
	// Coordinator acts as reviewer when there is no dedicated reviewer.
	return !ctx.HasReviewer
}

func (c *CoordinatorType) ShowAllRoleDescriptions() bool { return true }
func (c *CoordinatorType) ShowFullProjectTopology() bool { return true }
