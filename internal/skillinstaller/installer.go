package skillinstaller

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed embedded/agen8/SKILL.md
var agen8Skill []byte

const skillFileMode = 0o644

type Harness string

const (
	HarnessCodex     Harness = "codex"
	HarnessClaudeCLI Harness = "claude-cli"
)

type Options struct {
	Harness Harness
	HomeDir string
}

type Result struct {
	Harness Harness
	Path    string
}

func Install(opts Options) (Result, error) {
	harness, err := NormalizeHarness(opts.Harness)
	if err != nil {
		return Result{}, err
	}
	homeDir := strings.TrimSpace(opts.HomeDir)
	if homeDir == "" {
		homeDir, err = os.UserHomeDir()
		if err != nil {
			return Result{}, fmt.Errorf("resolve home directory: %w", err)
		}
	}
	path := SkillPath(homeDir, harness)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Result{}, fmt.Errorf("create skill directory: %w", err)
	}
	if err := os.WriteFile(path, agen8Skill, skillFileMode); err != nil {
		return Result{}, fmt.Errorf("write skill: %w", err)
	}
	return Result{Harness: harness, Path: path}, nil
}

func NormalizeHarness(value Harness) (Harness, error) {
	normalized := strings.ToLower(strings.TrimSpace(string(value)))
	switch normalized {
	case "codex":
		return HarnessCodex, nil
	case "claude", "claude-cli", "claude-code":
		return HarnessClaudeCLI, nil
	default:
		return "", fmt.Errorf("unsupported harness %q; use codex or claude-cli", value)
	}
}

func SkillPath(homeDir string, harness Harness) string {
	switch harness {
	case HarnessCodex:
		return filepath.Join(homeDir, ".codex", "skills", "agen8", "SKILL.md")
	case HarnessClaudeCLI:
		return filepath.Join(homeDir, ".claude", "skills", "agen8", "SKILL.md")
	default:
		return ""
	}
}

func EmbeddedSkill() []byte {
	copied := make([]byte, len(agen8Skill))
	copy(copied, agen8Skill)
	return copied
}
