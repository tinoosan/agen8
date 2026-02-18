package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRuntimeConfig_DataDirAndCWDMerge(t *testing.T) {
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir dataDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "config.toml"), []byte(`
[defaults]
model = "openai/gpt-5-mini"
[skills]
conflict = "keep"
[env]
OPENROUTER_API_KEY = "from-data-dir"
`), 0o644); err != nil {
		t.Fatalf("write dataDir config: %v", err)
	}

	cwd := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("mkdir cwd: %v", err)
	}
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cwd, "config.toml"), []byte(`
[defaults]
subagent_model = "openai/gpt-5-nano"
[skills]
conflict = "overwrite"
[env]
OPENROUTER_API_KEY = "from-cwd"
`), 0o644); err != nil {
		t.Fatalf("write cwd config: %v", err)
	}

	cfg, err := loadRuntimeConfig(dataDir)
	if err != nil {
		t.Fatalf("loadRuntimeConfig: %v", err)
	}
	if cfg.Defaults.Model != "openai/gpt-5-mini" {
		t.Fatalf("model=%q", cfg.Defaults.Model)
	}
	if cfg.Defaults.SubagentModel != "openai/gpt-5-nano" {
		t.Fatalf("subagent_model=%q", cfg.Defaults.SubagentModel)
	}
	if cfg.Skills.Conflict != "overwrite" {
		t.Fatalf("skills.conflict=%q", cfg.Skills.Conflict)
	}
	if got := cfg.Env["OPENROUTER_API_KEY"]; got != "from-cwd" {
		t.Fatalf("OPENROUTER_API_KEY=%q", got)
	}
}

func TestApplyRuntimeConfigEnvDefaults_DoesNotOverrideExisting(t *testing.T) {
	t.Setenv("OPENROUTER_MODEL", "existing-model")
	t.Setenv("WORKBENCH_PROFILE", "existing-profile")
	cfg := runtimeConfig{
		Defaults: runtimeConfigDefaults{
			Model:   "new-model",
			Profile: "new-profile",
		},
		Env: map[string]string{
			"OPENROUTER_API_KEY": "key-1",
		},
		Skills: runtimeConfigSkills{Conflict: "keep"},
	}
	applyRuntimeConfigEnvDefaults(cfg)
	if got := os.Getenv("OPENROUTER_MODEL"); got != "existing-model" {
		t.Fatalf("OPENROUTER_MODEL=%q", got)
	}
	if got := os.Getenv("WORKBENCH_PROFILE"); got != "existing-profile" {
		t.Fatalf("WORKBENCH_PROFILE=%q", got)
	}
	if got := os.Getenv("OPENROUTER_API_KEY"); got != "key-1" {
		t.Fatalf("OPENROUTER_API_KEY=%q", got)
	}
	if got := os.Getenv(envSkillsSeedConflict); got != "keep" {
		t.Fatalf("%s=%q", envSkillsSeedConflict, got)
	}
}
