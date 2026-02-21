package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaybeSeedRepoDefaults_SeedsAgentsSkillsAndProfiles(t *testing.T) {
	tmp := t.TempDir()
	withTestRepoDefaults(t, tmp)

	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	t.Setenv("HOME", home)

	dataDir := filepath.Join(tmp, "data")
	if err := maybeSeedRepoDefaults(dataDir); err != nil {
		t.Fatalf("maybeSeedRepoDefaults: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dataDir, "profiles", "general", "profile.yaml")); err != nil {
		t.Fatalf("expected seeded profile, stat: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "coding", "SKILL.md")); err != nil {
		t.Fatalf("expected seeded skill in ~/.agents/skills, stat: %v", err)
	}
}

func TestMaybeSeedRepoDefaults_MigratesLegacySkillsOnce(t *testing.T) {
	tmp := t.TempDir()
	withTestRepoDefaults(t, tmp)

	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	t.Setenv("HOME", home)

	dataDir := filepath.Join(tmp, "data")
	legacySkill := filepath.Join(dataDir, "skills", "legacy", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(legacySkill), 0o755); err != nil {
		t.Fatalf("mkdir legacy skill dir: %v", err)
	}
	if err := os.WriteFile(legacySkill, []byte("# legacy v1\n"), 0o644); err != nil {
		t.Fatalf("write legacy skill: %v", err)
	}

	if err := maybeSeedRepoDefaults(dataDir); err != nil {
		t.Fatalf("first maybeSeedRepoDefaults: %v", err)
	}
	dst := filepath.Join(home, ".agents", "skills", "legacy", "SKILL.md")
	first, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read migrated skill: %v", err)
	}
	if !strings.Contains(string(first), "legacy v1") {
		t.Fatalf("expected migrated legacy content, got %q", string(first))
	}

	if err := os.WriteFile(legacySkill, []byte("# legacy v2\n"), 0o644); err != nil {
		t.Fatalf("rewrite legacy skill: %v", err)
	}
	if err := maybeSeedRepoDefaults(dataDir); err != nil {
		t.Fatalf("second maybeSeedRepoDefaults: %v", err)
	}
	second, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read migrated skill after second run: %v", err)
	}
	if string(second) != string(first) {
		t.Fatalf("expected one-time migration to skip second legacy copy, before=%q after=%q", string(first), string(second))
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", skillsMigrationMarkerFile)); err != nil {
		t.Fatalf("expected migration marker, stat: %v", err)
	}
}

func TestMaybeSeedRepoDefaults_NonInteractiveKeepsExistingOnConflict(t *testing.T) {
	tmp := t.TempDir()
	withTestRepoDefaults(t, tmp)

	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	t.Setenv("HOME", home)

	dataDir := filepath.Join(tmp, "data")
	existingSkill := filepath.Join(home, ".agents", "skills", "coding", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(existingSkill), 0o755); err != nil {
		t.Fatalf("mkdir existing skill dir: %v", err)
	}
	if err := os.WriteFile(existingSkill, []byte("# keep me\n"), 0o644); err != nil {
		t.Fatalf("write existing skill: %v", err)
	}

	prevDetect := detectInteractiveTTY
	detectInteractiveTTY = func() bool { return false }
	t.Cleanup(func() { detectInteractiveTTY = prevDetect })

	if err := maybeSeedRepoDefaults(dataDir); err != nil {
		t.Fatalf("maybeSeedRepoDefaults: %v", err)
	}
	got, err := os.ReadFile(existingSkill)
	if err != nil {
		t.Fatalf("read existing skill: %v", err)
	}
	if string(got) != "# keep me\n" {
		t.Fatalf("expected existing skill to be kept, got %q", string(got))
	}
}

func TestCopyDirWithConflictStrategy_Overwrite(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	dst := filepath.Join(tmp, "dst")

	srcFile := filepath.Join(src, "coding", "SKILL.md")
	dstFile := filepath.Join(dst, "coding", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(srcFile), 0o755); err != nil {
		t.Fatalf("mkdir src dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(dstFile), 0o755); err != nil {
		t.Fatalf("mkdir dst dir: %v", err)
	}
	if err := os.WriteFile(srcFile, []byte("# source\n"), 0o644); err != nil {
		t.Fatalf("write src file: %v", err)
	}
	if err := os.WriteFile(dstFile, []byte("# old\n"), 0o644); err != nil {
		t.Fatalf("write dst file: %v", err)
	}

	resolver := newSeedConflictResolver(false, nil, nil)
	resolver.strategy = seedConflictOverwrite
	if err := copyDirWithConflictStrategy(src, dst, resolver); err != nil {
		t.Fatalf("copyDirWithConflictStrategy: %v", err)
	}
	got, err := os.ReadFile(dstFile)
	if err != nil {
		t.Fatalf("read dst file: %v", err)
	}
	if string(got) != "# source\n" {
		t.Fatalf("expected overwrite, got %q", string(got))
	}
}

func TestConflictStrategyFromEnv(t *testing.T) {
	t.Setenv(envSkillsSeedConflict, "overwrite")
	if got := conflictStrategyFromEnv(); got != seedConflictOverwrite {
		t.Fatalf("strategy=%v want=%v", got, seedConflictOverwrite)
	}
	t.Setenv(envSkillsSeedConflict, "keep")
	if got := conflictStrategyFromEnv(); got != seedConflictKeep {
		t.Fatalf("strategy=%v want=%v", got, seedConflictKeep)
	}
	t.Setenv(envSkillsSeedConflict, "abort")
	if got := conflictStrategyFromEnv(); got != seedConflictAbort {
		t.Fatalf("strategy=%v want=%v", got, seedConflictAbort)
	}
	t.Setenv(envSkillsSeedConflict, "nope")
	if got := conflictStrategyFromEnv(); got != seedConflictUnset {
		t.Fatalf("strategy=%v want=%v", got, seedConflictUnset)
	}
}

func withTestRepoDefaults(t *testing.T, root string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	profilePath := filepath.Join("defaults", "profiles", "general", "profile.yaml")
	if err := os.MkdirAll(filepath.Dir(profilePath), 0o755); err != nil {
		t.Fatalf("mkdir profiles: %v", err)
	}
	if err := os.WriteFile(profilePath, []byte("id: general\ndescription: x\nprompts:\n  system_prompt: hello\n"), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}

	skillPath := filepath.Join("defaults", "skills", "coding", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillPath), 0o755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	if err := os.WriteFile(skillPath, []byte("# default coding skill\n"), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
}
