package skillinstaller

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed embedded
var embedded embed.FS

const (
	embeddedRoot  = "embedded"
	skillFileMode = 0o644
	skillDirMode  = 0o755
)

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
	// Root is the harness skills directory the skills were written under.
	Root string
	// Skills are the skill names installed (one per embedded/<name>/ directory).
	Skills []string
}

// Install writes every embedded skill (each embedded/<name>/SKILL.md and any
// sibling files) into the harness's skills directory, preserving the per-skill
// directory structure. Re-running refreshes the files in place.
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
	root := SkillsDir(homeDir, harness)
	if root == "" {
		return Result{}, fmt.Errorf("no skills directory for harness %q", harness)
	}

	var skills []string
	walkErr := fs.WalkDir(embedded, embeddedRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(embeddedRoot, path)
		if err != nil {
			return err
		}
		data, err := embedded.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read embedded skill %s: %w", rel, err)
		}
		dest := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dest), skillDirMode); err != nil {
			return fmt.Errorf("create skill directory for %s: %w", rel, err)
		}
		if err := os.WriteFile(dest, data, skillFileMode); err != nil {
			return fmt.Errorf("write skill %s: %w", rel, err)
		}
		if name := skillName(rel); name != "" {
			skills = appendUnique(skills, name)
		}
		return nil
	})
	if walkErr != nil {
		return Result{}, walkErr
	}
	sort.Strings(skills)
	return Result{Harness: harness, Root: root, Skills: skills}, nil
}

// skillName is the first path segment of an embedded file's relative path, i.e.
// the skill directory it belongs to ("agen8/SKILL.md" -> "agen8").
func skillName(rel string) string {
	return strings.SplitN(filepath.ToSlash(rel), "/", 2)[0]
}

func appendUnique(list []string, value string) []string {
	for _, existing := range list {
		if existing == value {
			return list
		}
	}
	return append(list, value)
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

// SkillsDir is the harness's skills root directory under homeDir.
func SkillsDir(homeDir string, harness Harness) string {
	switch harness {
	case HarnessCodex:
		return filepath.Join(homeDir, ".codex", "skills")
	case HarnessClaudeCLI:
		return filepath.Join(homeDir, ".claude", "skills")
	default:
		return ""
	}
}
