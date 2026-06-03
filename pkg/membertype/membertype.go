// Package membertype defines the MemberType interface and concrete implementations
// for each member type in the system (coordinator, lone coordinator, worker, reviewer).
//
// Adding a new member type requires:
//  1. Create a new file (e.g. planner.go) implementing MemberType.
//  2. Register it in init() in registry.go.
package membertype

import (
	"strings"

	"github.com/tinoosan/agen8-mcp-server/pkg/toolcontract"
)

// MemberTypeName is a string enum for member type identification.
type MemberTypeName string

const (
	TypeCoordinator     MemberTypeName = "coordinator"
	TypeLoneCoordinator MemberTypeName = "lone_coordinator"
	TypeWorker          MemberTypeName = "worker"
	TypeReviewer        MemberTypeName = "reviewer"
)

// MemberType defines the behavioral contract for a member type.
// Each type owns its tool list, prompt rules, and authorization strategy.
// Implementations are stateless — all context comes via method parameters.
type MemberType interface {
	// Name returns the canonical type name for registry lookup and logging.
	Name() MemberTypeName

	// SystemTools returns the system tools this type gets automatically.
	SystemTools(ctx ToolContext) []string

	// Authorize processes user-space allowedTools for this type.
	// System tools are stripped from the user list; type-specific defaults are applied.
	Authorize(ctx ToolContext, requestedTools []string) AuthorizationResult

	// PromptRules returns the composed prompt rule text block for this member type.
	// Implementations delegate to the registry-backed rule renderer.
	PromptRules(ctx PromptContext) string

	// ToolManifest returns the set of host tools this type needs registered.
	// Each entry names a tool; the caller wires it to the concrete implementation.
	ToolManifest(ctx ToolContext) []ToolEntry

	// StripPlanning returns true if planning blocks should be removed from
	// the base system prompt for simple goals.
	StripPlanning() bool

	// CanClaimSpaceMessages returns true if this type claims space-addressed messages
	// (AssignedToType="space").
	CanClaimSpaceMessages() bool

	// CanClaimReviewerMessages returns true if this type claims reviewer-role messages
	// when the reviewer role differs from its own role.
	CanClaimReviewerMessages(ctx ToolContext) bool

	// ShowAllRoleDescriptions returns true if this type sees all space member role descriptions.
	// Coordinators see all; workers/reviewers see only the coordinator description.
	ShowAllRoleDescriptions() bool

	// ShowFullProjectTopology returns true if this type sees full project topology.
	// Coordinators see all spaces; workers see other spaces with escalation guidance.
	ShowFullProjectTopology() bool
}

// ToolContext provides the runtime context needed for tool/policy decisions.
type ToolContext struct {
	SpaceID     string
	MemberCount int  // number of members in the space; 1 = lone coordinator
	HasReviewer bool // space has a dedicated reviewer member
}

// PromptContext provides the context needed for prompt rule generation.
type PromptContext struct {
	MemberType        MemberTypeName
	MemberLabel       string
	SpaceName         string
	CoordinatorMember string
	ReviewerMember    string
	MemberRoles       []string
	// MemberRoleDescriptions is intentionally NOT included here because the
	// space block rendering (role descriptions, project topology) stays in
	// prompt.go — only the type-specific rule section moves to MemberType.
}

// AuthorizationResult holds the outcome of an Authorize call.
type AuthorizationResult struct {
	Allowed []string // final tool list for the member type
	Removed []string // tools stripped from user request (for warning events)
}

// ToolEntry declares a host tool that a member type needs registered.
type ToolEntry struct {
	Name     string // tool name, e.g. "task", "space"
	Required bool   // if true, tool must be present for this type to function
}

// --- Shared tool taxonomy (extracted from toolpolicy/domain/policy.go) ---

// SystemAlwaysTools are system tools enabled for all members.
// mission is included so all members — including workers — can query mission
// context on demand (F39 Layer 2: on-demand tool for all member types).
// plan is also globally available; per-member-type action visibility/enforcement is
// handled by the plan action policy.
// space is also globally available; per-member action visibility/enforcement is
// handled by the space action policy.
var SystemAlwaysTools = toolcontract.NamesFor(toolcontract.Criteria{
	Group: toolcontract.GroupSystemAlways,
})

// CoordinatorBaseTools are tools every coordinator gets regardless of space size.
// heartbeat manages schedules and fires heartbeat jobs on demand.
var CoordinatorBaseTools = toolcontract.NamesFor(toolcontract.Criteria{
	Group: toolcontract.GroupCoordinatorBase,
})

// CoordinatorWithWorkersTools are additional tools for coordinators with members.
var CoordinatorWithWorkersTools = toolcontract.NamesFor(toolcontract.Criteria{
	Group: toolcontract.GroupCoordinatorWithWorkers,
})

// DefaultWorkerAllowedTools are the default user-space tools for workers with no config.
var DefaultWorkerAllowedTools = []string{"http"}

// --- Shared helpers ---

// StripSystemTools removes system tools from a user-defined allowedTools list.
// Returns the filtered list and the names of any stripped tools.
func StripSystemTools(systemTools []string, allowed []string) (filtered []string, stripped []string) {
	sysSet := map[string]struct{}{}
	for _, name := range systemTools {
		sysSet[strings.ToLower(name)] = struct{}{}
	}
	filtered = make([]string, 0, len(allowed))
	for _, name := range allowed {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if _, ok := sysSet[lower]; ok {
			stripped = append(stripped, trimmed)
			continue
		}
		filtered = append(filtered, trimmed)
	}
	return filtered, stripped
}

// IsCoordinatorType returns true if the member type is a coordinator variant.
func IsCoordinatorType(t MemberType) bool {
	if t == nil {
		return false
	}
	switch t.Name() {
	case TypeCoordinator, TypeLoneCoordinator:
		return true
	default:
		return false
	}
}

func sanitizeWorkspaceSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "default"
	}
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "default"
	}
	return b.String()
}
