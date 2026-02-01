package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMaybeSeedRepoDefaults_CopiesRolesAndSkills(t *testing.T) {
	tmp := t.TempDir()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if err := os.MkdirAll(filepath.Join("defaults", "roles", "general"), 0o755); err != nil {
		t.Fatalf("mkdir roles: %v", err)
	}
	if err := os.WriteFile(filepath.Join("defaults", "roles", "general", "ROLE.md"), []byte("---\nid: general\ndescription: x\nobligations:\n- id: o\n  validity: \"1m\"\n  evidence: e\ntask_policy:\n  create_tasks_only_if: [obligation_unsatisfied]\n  max_tasks_per_cycle: 1\n---\n"), 0o644); err != nil {
		t.Fatalf("write role: %v", err)
	}
	if err := os.MkdirAll(filepath.Join("defaults", "skills"), 0o755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	if err := os.WriteFile(filepath.Join("defaults", "skills", "coding.md"), []byte("---\nname: Coding\n---\n"), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	dataDir := filepath.Join(tmp, "data")
	if err := maybeSeedRepoDefaults(dataDir); err != nil {
		t.Fatalf("maybeSeedRepoDefaults: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dataDir, "roles", "general", "ROLE.md")); err != nil {
		t.Fatalf("expected seeded role, stat: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "skills", "coding.md")); err != nil {
		t.Fatalf("expected seeded skill, stat: %v", err)
	}
}

