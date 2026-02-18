package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

const envWorkbenchConfig = "WORKBENCH_CONFIG"

type runtimeConfig struct {
	Defaults runtimeConfigDefaults
	Env      map[string]string
	Skills   runtimeConfigSkills
}

type runtimeConfigDefaults struct {
	Model         string
	SubagentModel string
	Profile       string
}

type runtimeConfigSkills struct {
	Conflict string
}

type runtimeConfigFile struct {
	Defaults runtimeConfigDefaultsFile `toml:"defaults"`
	Env      map[string]string         `toml:"env"`
	Skills   runtimeConfigSkillsFile   `toml:"skills"`
}

type runtimeConfigDefaultsFile struct {
	Model         string `toml:"model"`
	SubagentModel string `toml:"subagent_model"`
	Profile       string `toml:"profile"`
}

type runtimeConfigSkillsFile struct {
	Conflict string `toml:"conflict"`
}

func loadRuntimeConfig(dataDir string) (runtimeConfig, error) {
	out := runtimeConfig{
		Env: map[string]string{},
	}
	loaded := false

	explicit := strings.TrimSpace(os.Getenv(envWorkbenchConfig))
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
			return runtimeConfig{}, fmt.Errorf("%s points to missing file: %s", envWorkbenchConfig, explicit)
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
	if cwd, err := os.Getwd(); err == nil {
		cfg, ok, derr := decodeRuntimeConfigFile(filepath.Join(cwd, "config.toml"))
		if derr != nil {
			return runtimeConfig{}, derr
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
	setEnvIfUnset("WORKBENCH_SUBAGENT_MODEL", cfg.Defaults.SubagentModel)
	setEnvIfUnset("WORKBENCH_PROFILE", cfg.Defaults.Profile)
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
