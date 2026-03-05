package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	authpkg "github.com/tinoosan/agen8/pkg/auth"
	"github.com/tinoosan/agen8/pkg/config"
)

func TestLoadRuntimeConfig_DataDirOnly(t *testing.T) {
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
[auth]
provider = "chatgpt_account"
allow_api_key_fallback_for_non_openai = true
[env]
OPENROUTER_API_KEY = "from-data-dir"
[code_exec]
venv_path = "exec/.venv"
required_packages = ["pandas"]
[path_access]
allowlist = ["/home/user/shared", "/var/cache/build"]
read_only = true
[obsidian]
vault_path = "/project/custom-vault"
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
[code_exec]
required_packages = ["requests"]
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
	if cfg.Defaults.SubagentModel != "" {
		t.Fatalf("subagent_model=%q", cfg.Defaults.SubagentModel)
	}
	if cfg.Skills.Conflict != "keep" {
		t.Fatalf("skills.conflict=%q", cfg.Skills.Conflict)
	}
	if cfg.Auth.AllowAPIKeyFallbackForNonOpenAI == nil || !*cfg.Auth.AllowAPIKeyFallbackForNonOpenAI {
		t.Fatalf("auth.allow_api_key_fallback_for_non_openai=%v", cfg.Auth.AllowAPIKeyFallbackForNonOpenAI)
	}
	if cfg.Auth.Provider != "chatgpt_account" {
		t.Fatalf("auth.provider=%q", cfg.Auth.Provider)
	}
	if got := cfg.Env["OPENROUTER_API_KEY"]; got != "from-data-dir" {
		t.Fatalf("OPENROUTER_API_KEY=%q", got)
	}
	if got := cfg.CodeExec.VenvPath; got != "exec/.venv" {
		t.Fatalf("venv_path=%q", got)
	}
	if got := strings.Join(cfg.CodeExec.RequiredPackages, ","); got != "pandas" {
		t.Fatalf("required_packages=%q", got)
	}
	if got := strings.Join(cfg.PathAccess.Allowlist, ","); got != "/home/user/shared,/var/cache/build" {
		t.Fatalf("path_access.allowlist=%q", got)
	}
	if !cfg.PathAccess.ReadOnly {
		t.Fatalf("path_access.read_only=%v, want true", cfg.PathAccess.ReadOnly)
	}
	if got := cfg.Obsidian.VaultPath; got != "/project/custom-vault" {
		t.Fatalf("obsidian.vault_path=%q", got)
	}
}

func TestApplyRuntimeConfigEnvDefaults_DoesNotOverrideExisting(t *testing.T) {
	t.Setenv("OPENROUTER_MODEL", "existing-model")
	t.Setenv("AGEN8_PROFILE", "existing-profile")
	t.Setenv("AGEN8_AUTH_PROVIDER", "existing-provider")
	cfg := runtimeConfig{
		Defaults: runtimeConfigDefaults{
			Model:   "new-model",
			Profile: "new-profile",
		},
		Env: map[string]string{
			"OPENROUTER_API_KEY": "key-1",
		},
		Skills: runtimeConfigSkills{Conflict: "keep"},
		Auth: runtimeConfigAuth{
			Provider:                        "chatgpt_account",
			AllowAPIKeyFallbackForNonOpenAI: boolPtr(true),
		},
		Obsidian: runtimeConfigObsidian{
			VaultPath: "/knowledge",
		},
	}
	applyRuntimeConfigEnvDefaults(cfg)
	if got := os.Getenv("OPENROUTER_MODEL"); got != "existing-model" {
		t.Fatalf("OPENROUTER_MODEL=%q", got)
	}
	if got := os.Getenv("AGEN8_PROFILE"); got != "existing-profile" {
		t.Fatalf("AGEN8_PROFILE=%q", got)
	}
	if got := os.Getenv("OPENROUTER_API_KEY"); got != "key-1" {
		t.Fatalf("OPENROUTER_API_KEY=%q", got)
	}
	if got := os.Getenv("AGEN8_AUTH_PROVIDER"); got != "existing-provider" {
		t.Fatalf("AGEN8_AUTH_PROVIDER=%q", got)
	}
	if got := os.Getenv("AGEN8_AUTH_CHATGPT_FALLBACK_API_KEY_NON_OPENAI"); got != "true" {
		t.Fatalf("AGEN8_AUTH_CHATGPT_FALLBACK_API_KEY_NON_OPENAI=%q", got)
	}
	if got := os.Getenv(envSkillsSeedConflict); got != "keep" {
		t.Fatalf("%s=%q", envSkillsSeedConflict, got)
	}
	if got := os.Getenv("OBSIDIAN_VAULT_PATH"); got != "/knowledge" {
		t.Fatalf("OBSIDIAN_VAULT_PATH=%q", got)
	}
}

func TestApplyRuntimeConfigEnvDefaults_SetsAuthProviderWhenUnset(t *testing.T) {
	prev, hadPrev := os.LookupEnv("AGEN8_AUTH_PROVIDER")
	_ = os.Unsetenv("AGEN8_AUTH_PROVIDER")
	t.Cleanup(func() {
		if hadPrev {
			_ = os.Setenv("AGEN8_AUTH_PROVIDER", prev)
		} else {
			_ = os.Unsetenv("AGEN8_AUTH_PROVIDER")
		}
	})
	applyRuntimeConfigEnvDefaults(runtimeConfig{
		Auth: runtimeConfigAuth{Provider: "chatgpt_account"},
		Env:  map[string]string{},
	})
	if got := os.Getenv("AGEN8_AUTH_PROVIDER"); got != "chatgpt_account" {
		t.Fatalf("AGEN8_AUTH_PROVIDER=%q", got)
	}
}

func TestApplyRuntimeConfigEnvDefaults_FromDataDir(t *testing.T) {
	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, "config.toml"), []byte(`
[defaults]
model = "openai/gpt-5-mini"
[auth]
provider = "chatgpt_account"
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	prevModel, hadModel := os.LookupEnv("OPENROUTER_MODEL")
	_ = os.Unsetenv("OPENROUTER_MODEL")
	t.Cleanup(func() {
		if hadModel {
			_ = os.Setenv("OPENROUTER_MODEL", prevModel)
		} else {
			_ = os.Unsetenv("OPENROUTER_MODEL")
		}
	})
	prevAuth, hadAuth := os.LookupEnv(authpkg.EnvAuthProvider)
	_ = os.Unsetenv(authpkg.EnvAuthProvider)
	t.Cleanup(func() {
		if hadAuth {
			_ = os.Setenv(authpkg.EnvAuthProvider, prevAuth)
		} else {
			_ = os.Unsetenv(authpkg.EnvAuthProvider)
		}
	})
	if err := ApplyRuntimeConfigEnvDefaults(dataDir); err != nil {
		t.Fatalf("ApplyRuntimeConfigEnvDefaults: %v", err)
	}
	if got := os.Getenv("OPENROUTER_MODEL"); got != "openai/gpt-5-mini" {
		t.Fatalf("OPENROUTER_MODEL=%q", got)
	}
	if got := os.Getenv(authpkg.EnvAuthProvider); got != authpkg.ProviderChatGPTAccount {
		t.Fatalf("%s=%q", authpkg.EnvAuthProvider, got)
	}
}

func TestEnsureRuntimeConfigTemplate_CreatesDefaultTemplate(t *testing.T) {
	dataDir := t.TempDir()
	path, err := ensureRuntimeConfigTemplate(dataDir)
	if err != nil {
		t.Fatalf("ensureRuntimeConfigTemplate: %v", err)
	}
	if strings.TrimSpace(path) == "" {
		t.Fatalf("expected config path")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, `model = "`+runtimeDefaultModel+`"`) {
		t.Fatalf("expected default model in template, got:\n%s", text)
	}
	if strings.Contains(text, `OPENROUTER_API_KEY = "`) || strings.Contains(text, `OPENAI_API_KEY = "`) {
		t.Fatalf("template should not include concrete API key values")
	}
	if !strings.Contains(text, "[code_exec]") {
		t.Fatalf("expected code_exec section in template, got:\n%s", text)
	}
	if !strings.Contains(text, "[auth]") {
		t.Fatalf("expected auth section in template, got:\n%s", text)
	}
	if !strings.Contains(text, "[obsidian]") {
		t.Fatalf("expected obsidian section in template, got:\n%s", text)
	}
}

func TestEnsureRuntimeConfigTemplate_Idempotent(t *testing.T) {
	dataDir := t.TempDir()
	path, err := ensureRuntimeConfigTemplate(dataDir)
	if err != nil {
		t.Fatalf("ensureRuntimeConfigTemplate: %v", err)
	}
	if err := os.WriteFile(path, []byte("[defaults]\nmodel = \"custom/model\"\n"), 0o644); err != nil {
		t.Fatalf("write custom config: %v", err)
	}
	path2, err := ensureRuntimeConfigTemplate(dataDir)
	if err != nil {
		t.Fatalf("ensureRuntimeConfigTemplate second call: %v", err)
	}
	if path2 != path {
		t.Fatalf("path mismatch: %q vs %q", path2, path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if strings.TrimSpace(string(raw)) != `[defaults]
model = "custom/model"` {
		t.Fatalf("existing config should remain untouched; got:\n%s", string(raw))
	}
}

func TestLoadRuntimeConfig_PathAccessReadOnlyDefaultsTrue(t *testing.T) {
	tmp := t.TempDir()
	// Provide allowlist but omit read_only — should default to true.
	if err := os.WriteFile(filepath.Join(tmp, "config.toml"), []byte(`
[path_access]
allowlist = ["/shared"]
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := loadRuntimeConfig(tmp)
	if err != nil {
		t.Fatalf("loadRuntimeConfig: %v", err)
	}
	if !cfg.PathAccess.ReadOnly {
		t.Fatalf("path_access.read_only=%v, want true (default when omitted)", cfg.PathAccess.ReadOnly)
	}
}

func TestApplyRuntimeConfigHostDefaults_CodeExec(t *testing.T) {
	base := config.Config{DataDir: "db"}
	out := applyRuntimeConfigHostDefaults(base, runtimeConfig{
		CodeExec: runtimeConfigCodeExec{
			VenvPath:         "exec/.venv",
			RequiredPackages: []string{"pandas", "requests"},
		},
		PathAccess: runtimeConfigPathAccess{
			Allowlist: []string{"/shared"},
			ReadOnly:  true,
		},
	})
	if out.CodeExec.VenvPath != "exec/.venv" {
		t.Fatalf("venv_path=%q", out.CodeExec.VenvPath)
	}
	if got := strings.Join(out.CodeExec.RequiredPackages, ","); got != "pandas,requests" {
		t.Fatalf("required_packages=%q", got)
	}
	if got := strings.Join(out.PathAccess.Allowlist, ","); got != "/shared" {
		t.Fatalf("path_access.allowlist=%q", got)
	}
	if !out.PathAccess.ReadOnly {
		t.Fatalf("path_access.read_only=%v, want true", out.PathAccess.ReadOnly)
	}
}

func boolPtr(v bool) *bool {
	return &v
}

func TestPersistRuntimeAuthProvider_WritesProviderToConfig(t *testing.T) {
	dataDir := t.TempDir()
	if _, err := ensureRuntimeConfigTemplate(dataDir); err != nil {
		t.Fatalf("ensure template: %v", err)
	}
	if err := PersistRuntimeAuthProvider(dataDir, authpkg.ProviderChatGPTAccount); err != nil {
		t.Fatalf("PersistRuntimeAuthProvider: %v", err)
	}
	cfg, err := loadRuntimeConfig(dataDir)
	if err != nil {
		t.Fatalf("loadRuntimeConfig: %v", err)
	}
	if got := strings.TrimSpace(cfg.Auth.Provider); got != authpkg.ProviderChatGPTAccount {
		t.Fatalf("auth.provider=%q", got)
	}
}
