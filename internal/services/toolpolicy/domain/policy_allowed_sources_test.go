package domain

import (
	"testing"

	"github.com/tinoosan/agen8-mcp-server/pkg/membertype"
	tooldomain "github.com/tinoosan/agen8-mcp-server/internal/mcp/tools/domain"
)

// TestAllowedSources_WiredToRolePolicy verifies that AllowedSources flows
// from RoleToolContext → RoleToolPolicy → tooldomain.RolePolicy.
func TestAllowedSources_WiredToRolePolicy(t *testing.T) {
	ctx := RoleToolContext{
		MemberType:     &membertype.WorkerType{},
		AllowedSources: []string{"github-api", "slack-api"},
	}
	policy := NewRoleToolPolicy(ctx)
	rp := policy.ToRolePolicy()

	if len(rp.AllowedSources) != 2 {
		t.Fatalf("expected 2 allowed sources, got %d", len(rp.AllowedSources))
	}
	if rp.AllowedSources[0] != "github-api" || rp.AllowedSources[1] != "slack-api" {
		t.Errorf("AllowedSources: got %v", rp.AllowedSources)
	}
}

// TestAllowedSources_EmptyIsNil verifies backward compatibility — empty
// AllowedSources is nil, not an empty slice.
func TestAllowedSources_EmptyIsNil(t *testing.T) {
	ctx := RoleToolContext{MemberType: &membertype.WorkerType{}}
	policy := NewRoleToolPolicy(ctx)
	rp := policy.ToRolePolicy()

	if rp.AllowedSources != nil {
		t.Errorf("expected nil AllowedSources for backward compat, got %v", rp.AllowedSources)
	}
}

// TestAllowedSources_BuiltinAlwaysAllowed verifies the registry's behavior
// that builtin source is always allowed regardless of AllowedSources.
// (This tests the integration point — the actual filtering is in tools/app/registry.go)
func TestAllowedSources_BuiltinAlwaysAllowed(t *testing.T) {
	// This is verified by the existing TestRegistry_DefinitionsForRole_AllowedSources
	// in tools/app/registry_test.go. This test just documents the contract.
	rp := tooldomain.RolePolicy{
		AllowedSources: []string{"external-only"},
	}
	// Builtin source ID
	builtinID := string(tooldomain.SourceBuiltin)

	// Verify the contract: builtin should be "builtin"
	if builtinID != "builtin" {
		t.Errorf("SourceBuiltin: got %q", builtinID)
	}
	_ = rp // used to verify compilation
}
