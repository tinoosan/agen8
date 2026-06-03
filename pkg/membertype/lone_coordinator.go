package membertype

import "strings"

// LoneCoordinatorType implements MemberType for coordinators who are the only
// member in their space. No task creation, no review workflow.
type LoneCoordinatorType struct{}

func (l *LoneCoordinatorType) Name() MemberTypeName { return TypeLoneCoordinator }

func (l *LoneCoordinatorType) SystemTools(_ ToolContext) []string {
	tools := append([]string{}, SystemAlwaysTools...)
	tools = append(tools, CoordinatorBaseTools...)
	// No CoordinatorWithWorkersTools — no members to delegate to.
	return tools
}

func (l *LoneCoordinatorType) Authorize(ctx ToolContext, requestedTools []string) AuthorizationResult {
	if strings.TrimSpace(ctx.SpaceID) == "" {
		return AuthorizationResult{Allowed: append([]string(nil), requestedTools...)}
	}
	if len(requestedTools) == 0 {
		return AuthorizationResult{}
	}
	sysTools := l.SystemTools(ctx)
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

func (l *LoneCoordinatorType) PromptRules(ctx PromptContext) string {
	return renderPromptRules(TypeLoneCoordinator, ctx)
}

func (l *LoneCoordinatorType) ToolManifest(_ ToolContext) []ToolEntry {
	return []ToolEntry{
		{Name: "task", Required: true},
		{Name: "space", Required: true},
		{Name: "heartbeat", Required: true},
		{Name: "mission", Required: true},
		{Name: "plan", Required: true},
	}
}

func (l *LoneCoordinatorType) StripPlanning() bool { return false }

func (l *LoneCoordinatorType) CanClaimSpaceMessages() bool { return true }

func (l *LoneCoordinatorType) CanClaimReviewerMessages(_ ToolContext) bool { return false }

func (l *LoneCoordinatorType) ShowAllRoleDescriptions() bool { return false }
func (l *LoneCoordinatorType) ShowFullProjectTopology() bool { return true }
