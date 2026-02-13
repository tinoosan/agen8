package runtime

import (
	"reflect"
	"testing"

	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/profile"
)

func TestResolveProfileSkills_FromProfileConfig(t *testing.T) {
	cfg := BuildConfig{
		Cfg: config.Config{DataDir: t.TempDir()},
		ProfileConfig: &profile.Profile{
			ID:     "dev_team",
			Skills: []string{"coding", "planning"},
			Team: &profile.TeamConfig{
				Roles: []profile.RoleConfig{
					{Name: "backend", Skills: []string{"coding", "data_engineering"}},
					{Name: "qa", Skills: []string{"automation", "coding"}},
				},
			},
		},
	}

	got := resolveProfileSkills(cfg)
	want := []string{"coding", "planning", "data_engineering", "automation"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("skills mismatch: got %v want %v", got, want)
	}
}

func TestResolveProfileSkills_EmptyReturnsNoFilter(t *testing.T) {
	cfg := BuildConfig{
		Cfg:           config.Config{DataDir: t.TempDir()},
		ProfileConfig: &profile.Profile{ID: "general"},
	}
	if got := resolveProfileSkills(cfg); got != nil {
		t.Fatalf("expected nil skills, got %v", got)
	}
}

func TestResolveProfileSkills_DedupesExactNames(t *testing.T) {
	p := &profile.Profile{
		ID:     "x",
		Skills: []string{"coding", "coding", "planning"},
		Team: &profile.TeamConfig{
			Roles: []profile.RoleConfig{
				{Name: "r1", Skills: []string{"planning", "automation"}},
				{Name: "r2", Skills: []string{"automation", "coding"}},
			},
		},
	}
	got := uniqueProfileSkills(p)
	want := []string{"coding", "planning", "automation"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("skills mismatch: got %v want %v", got, want)
	}
}
