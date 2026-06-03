package membertype

import "strings"

// ReviewerType implements MemberType for dedicated reviewer roles.
type ReviewerType struct{}

func (r *ReviewerType) Name() MemberTypeName { return TypeReviewer }

func (r *ReviewerType) SystemTools(_ ToolContext) []string {
	tools := append([]string{}, SystemAlwaysTools...)
	return tools
}

func (r *ReviewerType) Authorize(ctx ToolContext, requestedTools []string) AuthorizationResult {
	if strings.TrimSpace(ctx.SpaceID) == "" {
		return AuthorizationResult{Allowed: append([]string(nil), requestedTools...)}
	}
	sysTools := r.SystemTools(ctx)
	userTools, stripped := StripSystemTools(sysTools, requestedTools)
	return AuthorizationResult{Allowed: userTools, Removed: stripped}
}

// PromptRules returns the reviewer-specific contract rules.
func (r *ReviewerType) PromptRules(ctx PromptContext) string {
	return renderPromptRules(TypeReviewer, ctx)
}

func (r *ReviewerType) ToolManifest(_ ToolContext) []ToolEntry {
	return []ToolEntry{
		{Name: "task", Required: true},
		{Name: "plan", Required: true},
	}
}

func (r *ReviewerType) StripPlanning() bool { return true }

func (r *ReviewerType) CanClaimSpaceMessages() bool { return false }

func (r *ReviewerType) CanClaimReviewerMessages(_ ToolContext) bool { return true }

func (r *ReviewerType) ShowAllRoleDescriptions() bool { return false }
func (r *ReviewerType) ShowFullProjectTopology() bool { return false }
