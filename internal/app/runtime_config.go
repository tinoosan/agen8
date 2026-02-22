package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/tinoosan/agen8/pkg/config"
)

const envAgen8Config = "AGEN8_CONFIG"
const runtimeDefaultModel = "z-ai/GLM-5"

type runtimeConfig struct {
	Defaults runtimeConfigDefaults
	Env      map[string]string
	Skills   runtimeConfigSkills
	CodeExec runtimeConfigCodeExec
	Obsidian runtimeConfigObsidian
}

type runtimeConfigDefaults struct {
	Model         string
	SubagentModel string
	Profile       string
}

type runtimeConfigSkills struct {
	Conflict string
}

type runtimeConfigCodeExec struct {
	VenvPath          string
	RequiredPackages  []string
	HostPathAllowlist []string
}

type runtimeConfigObsidian struct {
	VaultPath string
}

type runtimeConfigFile struct {
	Defaults runtimeConfigDefaultsFile `toml:"defaults"`
	Env      map[string]string         `toml:"env"`
	Skills   runtimeConfigSkillsFile   `toml:"skills"`
	CodeExec runtimeConfigCodeExecFile `toml:"code_exec"`
	Obsidian runtimeConfigObsidianFile `toml:"obsidian"`
}

type runtimeConfigDefaultsFile struct {
	Model         string `toml:"model"`
	SubagentModel string `toml:"subagent_model"`
	Profile       string `toml:"profile"`
}

type runtimeConfigSkillsFile struct {
	Conflict string `toml:"conflict"`
}

type runtimeConfigCodeExecFile struct {
	VenvPath          string   `toml:"venv_path"`
	RequiredPackages  []string `toml:"required_packages"`
	HostPathAllowlist []string `toml:"host_path_allowlist"`
}

type runtimeConfigObsidianFile struct {
	VaultPath string `toml:"vault_path"`
}

func loadRuntimeConfig(dataDir string) (runtimeConfig, error) {
	out := runtimeConfig{
		Env: map[string]string{},
	}
	loaded := false

	explicit := strings.TrimSpace(os.Getenv(envAgen8Config))
	if explicit != "" {
		cfg, ok, err := decodeRuntimeConfigFile(explicit)
		if err != nil {
			return runtimeConfig{}, err
		}
		if ok {
			loaded = true
			out = mergeRuntimeConfig(out, cfg)
		}
		if !loaded {
			return runtimeConfig{}, fmt.Errorf("%s points to missing file: %s", envAgen8Config, explicit)
		}
		return out, nil
	}

	if strings.TrimSpace(dataDir) != "" {
		cfg, ok, err := decodeRuntimeConfigFile(filepath.Join(dataDir, "config.toml"))
		if err != nil {
			return runtimeConfig{}, err
		}
		if ok {
			loaded = true
			out = mergeRuntimeConfig(out, cfg)
		}
	}
	if !loaded {
		return runtimeConfig{Env: map[string]string{}}, nil
	}
	return out, nil
}

func ensureRuntimeConfigTemplate(dataDir string) (string, error) {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return "", nil
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir data dir: %w", err)
	}
	path := filepath.Join(dataDir, "config.toml")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("stat %s: %w", path, err)
	}

	body := strings.TrimSpace(`
# Agen8 runtime defaults (non-secret values only).
# Secrets such as API keys are stored in your OS keychain.

[defaults]
model = "`+runtimeDefaultModel+`"
# subagent_model = ""
# profile = "general"

[env]
# OPENROUTER_BASE_URL = "https://openrouter.ai/api/v1"

[skills]
# conflict = "keep"

[code_exec]
# venv_path = ""
# required_packages = []
# host_path_allowlist = []  # Absolute dirs agent may access outside VFS (read-only)

[obsidian]
# vault_path = ""
`) + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return path, nil
}

func decodeRuntimeConfigFile(path string) (runtimeConfig, bool, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return runtimeConfig{}, false, nil
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return runtimeConfig{}, false, nil
		}
		return runtimeConfig{}, false, fmt.Errorf("stat %s: %w", path, err)
	}

	var raw runtimeConfigFile
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		return runtimeConfig{}, false, fmt.Errorf("parse %s: %w", path, err)
	}
	out := runtimeConfig{
		Defaults: runtimeConfigDefaults{
			Model:         strings.TrimSpace(raw.Defaults.Model),
			SubagentModel: strings.TrimSpace(raw.Defaults.SubagentModel),
			Profile:       strings.TrimSpace(raw.Defaults.Profile),
		},
		Env: map[string]string{},
		Skills: runtimeConfigSkills{
			Conflict: strings.ToLower(strings.TrimSpace(raw.Skills.Conflict)),
		},
		CodeExec: runtimeConfigCodeExec{
			VenvPath:          strings.TrimSpace(raw.CodeExec.VenvPath),
			RequiredPackages:  normalizeStringList(raw.CodeExec.RequiredPackages),
			HostPathAllowlist: normalizeStringList(raw.CodeExec.HostPathAllowlist),
		},
		Obsidian: runtimeConfigObsidian{
			VaultPath: strings.TrimSpace(raw.Obsidian.VaultPath),
		},
	}
	for k, v := range raw.Env {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out.Env[k] = strings.TrimSpace(v)
	}
	return out, true, nil
}

func mergeRuntimeConfig(base, override runtimeConfig) runtimeConfig {
	out := base
	if out.Env == nil {
		out.Env = map[string]string{}
	}
	if model := strings.TrimSpace(override.Defaults.Model); model != "" {
		out.Defaults.Model = model
	}
	if model := strings.TrimSpace(override.Defaults.SubagentModel); model != "" {
		out.Defaults.SubagentModel = model
	}
	if profile := strings.TrimSpace(override.Defaults.Profile); profile != "" {
		out.Defaults.Profile = profile
	}
	for k, v := range override.Env {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out.Env[k] = strings.TrimSpace(v)
	}
	if c := normalizeSkillsConflict(override.Skills.Conflict); c != "" {
		out.Skills.Conflict = c
	}
	if vp := strings.TrimSpace(override.CodeExec.VenvPath); vp != "" {
		out.CodeExec.VenvPath = vp
	}
	if len(override.CodeExec.RequiredPackages) > 0 {
		set := map[string]struct{}{}
		merged := make([]string, 0, len(out.CodeExec.RequiredPackages)+len(override.CodeExec.RequiredPackages))
		for _, item := range out.CodeExec.RequiredPackages {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			if _, ok := set[item]; ok {
				continue
			}
			set[item] = struct{}{}
			merged = append(merged, item)
		}
		for _, item := range override.CodeExec.RequiredPackages {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			if _, ok := set[item]; ok {
				continue
			}
			set[item] = struct{}{}
			merged = append(merged, item)
		}
		sort.Strings(merged)
		out.CodeExec.RequiredPackages = merged
	}
	if len(override.CodeExec.HostPathAllowlist) > 0 {
		set := map[string]struct{}{}
		merged := make([]string, 0, len(out.CodeExec.HostPathAllowlist)+len(override.CodeExec.HostPathAllowlist))
		for _, item := range out.CodeExec.HostPathAllowlist {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			if _, ok := set[item]; ok {
				continue
			}
			set[item] = struct{}{}
			merged = append(merged, item)
		}
		for _, item := range override.CodeExec.HostPathAllowlist {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			if _, ok := set[item]; ok {
				continue
			}
			set[item] = struct{}{}
			merged = append(merged, item)
		}
		sort.Strings(merged)
		out.CodeExec.HostPathAllowlist = merged
	}
	if vaultPath := strings.TrimSpace(override.Obsidian.VaultPath); vaultPath != "" {
		out.Obsidian.VaultPath = vaultPath
	}
	return out
}

func applyRuntimeConfigHostDefaults(host config.Config, cfg runtimeConfig) config.Config {
	out := host
	out.CodeExec.VenvPath = strings.TrimSpace(cfg.CodeExec.VenvPath)
	out.CodeExec.RequiredPackages = normalizeStringList(cfg.CodeExec.RequiredPackages)
	out.CodeExec.HostPathAllowlist = normalizeStringList(cfg.CodeExec.HostPathAllowlist)
	return out
}

func applyRuntimeConfigEnvDefaults(cfg runtimeConfig) {
	for k, v := range cfg.Env {
		if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
			continue
		}
		if _, exists := os.LookupEnv(k); exists {
			continue
		}
		_ = os.Setenv(k, v)
	}
	setEnvIfUnset("OPENROUTER_MODEL", cfg.Defaults.Model)
	setEnvIfUnset("AGEN8_SUBAGENT_MODEL", cfg.Defaults.SubagentModel)
	setEnvIfUnset("AGEN8_PROFILE", cfg.Defaults.Profile)
	setEnvIfUnset("OBSIDIAN_VAULT_PATH", cfg.Obsidian.VaultPath)
	setEnvIfUnset(envSkillsSeedConflict, normalizeSkillsConflict(cfg.Skills.Conflict))
}

func setEnvIfUnset(key, value string) {
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key == "" || value == "" {
		return
	}
	if _, exists := os.LookupEnv(key); exists {
		return
	}
	_ = os.Setenv(key, value)
}

func normalizeSkillsConflict(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "overwrite", "keep", "abort", "prompt":
		return strings.ToLower(strings.TrimSpace(v))
	default:
		return ""
	}
}

func normalizeStringList(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, item := range in {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil
	}
	sort.Strings(out)
	return out
}
