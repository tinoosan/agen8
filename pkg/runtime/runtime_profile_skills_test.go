package runtime

import (
	"reflect"
	"testing"

	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/profile"
	"github.com/tinoosan/agen8/pkg/types"
)

func TestResolveProfileSkillScope_StandaloneProfileSkillsOnly(t *testing.T) {
	cfg := BuildConfig{
		Cfg: config.Config{DataDir: t.TempDir()},
		ProfileConfig: &profile.Profile{
			ID:     "general",
			Skills: []string{"coding", "planning"},
			Team: &profile.TeamConfig{
				Roles: []profile.RoleConfig{
					{Name: "backend", Skills: []string{"coding", "data-engineering"}},
					{Name: "qa", Skills: []string{"automation", "coding"}},
				},
			},
		},
		Run: types.Run{Runtime: &types.RunRuntimeConfig{}},
	}

	got := resolveProfileSkillScope(cfg)
	want := []string{"coding", "planning"}
	if !reflect.DeepEqual(got.AllowedSkills, want) {
		t.Fatalf("skills mismatch: got %v want %v", got.AllowedSkills, want)
	}
	if !got.EnforceAllowlist {
		t.Fatalf("expected enforce allowlist true")
	}
	if got.Scope != "standalone" {
		t.Fatalf("scope=%q", got.Scope)
	}
}

func TestResolveProfileSkillScope_StandaloneEmptySkillsFailClosed(t *testing.T) {
	cfg := BuildConfig{
		Cfg: config.Config{DataDir: t.TempDir()},
		ProfileConfig: &profile.Profile{
			ID: "general",
		},
		Run: types.Run{Runtime: &types.RunRuntimeConfig{}},
	}

	got := resolveProfileSkillScope(cfg)
	if got.AllowedSkills != nil {
		t.Fatalf("expected nil skills, got %v", got.AllowedSkills)
	}
	if !got.EnforceAllowlist {
		t.Fatalf("expected enforce allowlist true")
	}
	if got.Scope != "standalone" {
		t.Fatalf("scope=%q", got.Scope)
	}
}

func TestResolveProfileSkillScope_TeamRoleOnlySkills(t *testing.T) {
	cfg := BuildConfig{
		Cfg: config.Config{DataDir: t.TempDir()},
		ProfileConfig: &profile.Profile{
			ID:     "dev_team",
			Skills: []string{"global-coding"},
			Team: &profile.TeamConfig{
				Roles: []profile.RoleConfig{
					{Name: "backend", Skills: []string{"coding", "data-engineering"}},
					{Name: "qa", Skills: []string{"automation", "coding"}},
				},
			},
		},
		Run: types.Run{Runtime: &types.RunRuntimeConfig{
			TeamID: "team-1",
			Role:   "backend",
		}},
	}

	got := resolveProfileSkillScope(cfg)
	want := []string{"coding", "data-engineering"}
	if !reflect.DeepEqual(got.AllowedSkills, want) {
		t.Fatalf("skills mismatch: got %v want %v", got.AllowedSkills, want)
	}
	if got.Scope != "team-role" {
		t.Fatalf("scope=%q", got.Scope)
	}
	if got.Role != "backend" {
		t.Fatalf("role=%q", got.Role)
	}
	if got.FailClosedReason != "" {
		t.Fatalf("unexpected fail closed reason: %q", got.FailClosedReason)
	}
}

func TestResolveProfileSkillScope_TeamRoleUnmappedFailsClosed(t *testing.T) {
	cfg := BuildConfig{
		Cfg: config.Config{DataDir: t.TempDir()},
		ProfileConfig: &profile.Profile{
			ID: "dev_team",
			Team: &profile.TeamConfig{
				Roles: []profile.RoleConfig{
					{Name: "backend", Skills: []string{"coding"}},
				},
			},
		},
		Run: types.Run{Runtime: &types.RunRuntimeConfig{
			TeamID: "team-1",
			Role:   "qa",
		}},
	}
	got := resolveProfileSkillScope(cfg)
	if got.AllowedSkills != nil {
		t.Fatalf("expected nil skills, got %v", got.AllowedSkills)
	}
	if !got.EnforceAllowlist {
		t.Fatalf("expected enforce allowlist true")
	}
	if got.Scope != "team-role" {
		t.Fatalf("scope=%q", got.Scope)
	}
	if got.FailClosedReason != "team_role_unmapped" {
		t.Fatalf("reason=%q", got.FailClosedReason)
	}
}

func TestResolveProfileSkillScope_TeamRoleUnmappedWithFallbackUsesProfileSkills(t *testing.T) {
	cfg := BuildConfig{
		Cfg: config.Config{
			DataDir:                             t.TempDir(),
			SkillsUnmappedRoleFallbackToProfile: true,
		},
		ProfileConfig: &profile.Profile{
			ID:     "dev_team",
			Skills: []string{"global-coding", "planning"},
			Team: &profile.TeamConfig{
				Roles: []profile.RoleConfig{
					{Name: "backend", Skills: []string{"coding"}},
				},
			},
		},
		Run: types.Run{Runtime: &types.RunRuntimeConfig{
			TeamID: "team-1",
			Role:   "qa",
		}},
	}
	got := resolveProfileSkillScope(cfg)
	want := []string{"global-coding", "planning"}
	if !reflect.DeepEqual(got.AllowedSkills, want) {
		t.Fatalf("skills mismatch: got %v want %v", got.AllowedSkills, want)
	}
	if got.Scope != "team-role-fallback" {
		t.Fatalf("scope=%q", got.Scope)
	}
	if got.FailClosedReason != "" {
		t.Fatalf("unexpected fail closed reason: %q", got.FailClosedReason)
	}
}

func TestResolveProfileSkillScope_TeamRoleCaseInsensitiveMatch(t *testing.T) {
	cfg := BuildConfig{
		Cfg: config.Config{DataDir: t.TempDir()},
		ProfileConfig: &profile.Profile{
			ID:     "dev_team",
			Skills: []string{"global"},
			Team: &profile.TeamConfig{
				Roles: []profile.RoleConfig{
					{Name: "backend", Skills: []string{"coding", "data-engineering"}},
					{Name: "qa", Skills: []string{"automation"}},
				},
			},
		},
		Run: types.Run{Runtime: &types.RunRuntimeConfig{
			TeamID: "team-1",
			Role:   "BACKEND",
		}},
	}
	got := resolveProfileSkillScope(cfg)
	want := []string{"coding", "data-engineering"}
	if !reflect.DeepEqual(got.AllowedSkills, want) {
		t.Fatalf("skills mismatch: got %v want %v", got.AllowedSkills, want)
	}
	if got.Scope != "team-role" {
		t.Fatalf("scope=%q", got.Scope)
	}
	if got.Role != "BACKEND" {
		t.Fatalf("role=%q", got.Role)
	}
}

func TestResolveProfileSkillScope_TeamIDWithStandaloneProfile_UsesSyntheticRoleSkills(t *testing.T) {
	cfg := BuildConfig{
		Cfg: config.Config{DataDir: t.TempDir()},
		ProfileConfig: &profile.Profile{
			ID:          "general",
			Name:        "General Agent",
			Description: "standalone profile",
			Skills:      []string{"coding", "planning"},
		},
		Run: types.Run{Runtime: &types.RunRuntimeConfig{
			TeamID: "team-1",
			Role:   "General Agent",
		}},
	}

	got := resolveProfileSkillScope(cfg)
	want := []string{"coding", "planning"}
	if !reflect.DeepEqual(got.AllowedSkills, want) {
		t.Fatalf("skills mismatch: got %v want %v", got.AllowedSkills, want)
	}
	if got.Scope != "team-role" {
		t.Fatalf("scope=%q", got.Scope)
	}
	if got.FailClosedReason != "" {
		t.Fatalf("unexpected fail closed reason: %q", got.FailClosedReason)
	}
}

func TestNormalizeSkillList_DedupesExactNames(t *testing.T) {
	got := normalizeSkillList([]string{"coding", "coding", "planning", " ", "planning"})
	want := []string{"coding", "planning"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("skills mismatch: got %v want %v", got, want)
	}
}
