package domain

import (
	"slices"
	"strings"

	"github.com/tinoosan/agen8-mcp-server/pkg/membertype"
	"github.com/tinoosan/agen8-mcp-server/pkg/toolcontract"
	tooldomain "github.com/tinoosan/agen8-mcp-server/internal/mcp/tools/domain"
)

// RoleToolContext is the input for constructing a RoleToolPolicy.
type RoleToolContext struct {
	MemberType     membertype.MemberType
	SpaceID        string
	MemberCount    int      // number of members in the space; 1 = lone coordinator
	HasReviewer    bool     // space has a dedicated reviewer member
	AllowedSources []string // coarse-grained source-level access control
}

// RoleToolPolicy is the aggregate root for tool authorization decisions.
// Delegates to MemberType for role-specific behavior.
type RoleToolPolicy struct {
	memberType     membertype.MemberType
	spaceID        string
	memberCount    int
	hasReviewer    bool
	allowedSources []string
}

// NewRoleToolPolicy constructs a RoleToolPolicy from a RoleToolContext.
func NewRoleToolPolicy(ctx RoleToolContext) RoleToolPolicy {
	return RoleToolPolicy{
		memberType:     ctx.MemberType,
		spaceID:        ctx.SpaceID,
		memberCount:    ctx.MemberCount,
		hasReviewer:    ctx.HasReviewer,
		allowedSources: ctx.AllowedSources,
	}
}

// AuthorizationResult holds the outcome of an Authorize call.
type AuthorizationResult struct {
	Allowed []string // final tool list for the role
	Removed []string // tools stripped from user request (for warning events)
}

// ToRolePolicy adapts the runtime policy to the unified tool registry policy shape.
func (p RoleToolPolicy) ToRolePolicy() tooldomain.RolePolicy {
	return tooldomain.RolePolicy{
		MemberType:     p.memberType,
		SpaceID:        p.spaceID,
		MemberCount:    p.memberCount,
		HasReviewer:    p.hasReviewer,
		AllowedSources: p.allowedSources,
	}
}

// toolContext builds the membertype.ToolContext from policy fields.
func (p RoleToolPolicy) toolContext() membertype.ToolContext {
	return membertype.ToolContext{
		SpaceID:     p.spaceID,
		MemberCount: p.memberCount,
		HasReviewer: p.hasReviewer,
	}
}

// SystemTools returns the complete list of system tools for this role.
func (p RoleToolPolicy) SystemTools() []string {
	if p.memberType == nil {
		return append([]string{}, membertype.SystemAlwaysTools...)
	}
	return p.memberType.SystemTools(p.toolContext())
}

// IsSystemTool returns true if a tool is a system tool that cannot be removed
// by the user's allowedTools configuration.
func (p RoleToolPolicy) IsSystemTool(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	if canonical, ok := toolcontract.ResolveCanonicalName(name); ok {
		name = strings.ToLower(strings.TrimSpace(canonical))
	}
	return slices.Contains(p.SystemTools(), name)
}

// Authorize processes the user-space allowedTools list for a role.
// System tools are handled separately by IsSystemTool and are never affected.
// Any system tools found in the user-provided list are stripped (returned in Removed).
func (p RoleToolPolicy) Authorize(requestedTools []string) AuthorizationResult {
	if p.memberType == nil {
		allowed, removed := stripDisallowedAgentTools(requestedTools)
		return AuthorizationResult{Allowed: allowed, Removed: removed}
	}
	result := p.memberType.Authorize(p.toolContext(), requestedTools)
	allowed, stripped := stripDisallowedAgentTools(result.Allowed)
	removed := append([]string(nil), result.Removed...)
	removed = append(removed, stripped...)
	return AuthorizationResult{Allowed: allowed, Removed: removed}
}

// DefaultWorkerTools returns a copy of the default user-space tools for workers
// with no allowedTools config.
func DefaultWorkerTools() []string {
	out := make([]string, len(membertype.DefaultWorkerAllowedTools))
	copy(out, membertype.DefaultWorkerAllowedTools)
	return out
}

// CoordinatorBaseToolNames returns a copy of the coordinator base tool names.
func CoordinatorBaseToolNames() []string {
	out := make([]string, len(membertype.CoordinatorBaseTools))
	copy(out, membertype.CoordinatorBaseTools)
	return out
}

// CoordinatorWithWorkersToolNames returns a copy of the coordinator-with-workers tool names.
func CoordinatorWithWorkersToolNames() []string {
	out := make([]string, len(membertype.CoordinatorWithWorkersTools))
	copy(out, membertype.CoordinatorWithWorkersTools)
	return out
}

var disallowedMemberTools = map[string]struct{}{
	"bash": {},
}

func stripDisallowedAgentTools(names []string) (allowed []string, removed []string) {
	if len(names) == 0 {
		return nil, nil
	}
	allowed = make([]string, 0, len(names))
	removed = make([]string, 0)
	for _, name := range names {
		candidate := strings.TrimSpace(name)
		if candidate == "" {
			continue
		}
		if _, blocked := disallowedMemberTools[strings.ToLower(candidate)]; blocked {
			removed = append(removed, candidate)
			continue
		}
		allowed = append(allowed, candidate)
	}
	return allowed, removed
}
