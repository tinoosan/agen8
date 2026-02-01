package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMaybeSeedRepoDefaults_CopiesProfilesAndSkills(t *testing.T) {
	tmp := t.TempDir()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if err := os.MkdirAll(filepath.Join("defaults", "profiles", "general"), 0o755); err != nil {
		t.Fatalf("mkdir profiles: %v", err)
	}
	if err := os.WriteFile(filepath.Join("defaults", "profiles", "general", "profile.yaml"), []byte("id: general\ndescription: x\nprompts:\n  system_prompt: hello\n"), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
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

	if _, err := os.Stat(filepath.Join(dataDir, "profiles", "general", "profile.yaml")); err != nil {
		t.Fatalf("expected seeded profile, stat: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "skills", "coding.md")); err != nil {
		t.Fatalf("expected seeded skill, stat: %v", err)
	}
}
