package skills

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tinoosan/workbench-core/internal/fsutil"
	"github.com/tinoosan/workbench-core/internal/vfsutil"
	"gopkg.in/yaml.v3"
)

// SkillEntry pairs a directory name with its parsed Skill metadata.
type SkillEntry struct {
	// Dir is the directory name under the provided root (used for VFS paths).
	Dir string

	// Skill contains the descriptive metadata extracted from SKILL.md.
	Skill *Skill
}

// Manager discovers skills on disk and caches their metadata.
type Manager struct {
	roots []string
	// WritableRoot is the directory where new/updated skills are persisted.
	WritableRoot string
	// entries maps directory name -> skill metadata.
	entries map[string]*Skill
}

// NewManager creates a Manager that searches the provided roots.
func NewManager(roots []string) *Manager {
	copied := make([]string, 0, len(roots))
	for _, r := range roots {
		if strings.TrimSpace(r) == "" {
			continue
		}
		copied = append(copied, r)
	}
	return &Manager{
		roots:   copied,
		entries: make(map[string]*Skill),
	}
}

// Scan refreshes the in-memory registry of discovered skills.
func (m *Manager) Scan() error {
	entries := make(map[string]*Skill)
	for _, rawRoot := range m.roots {
		root, err := filepath.Abs(rawRoot)
		if err != nil {
			return fmt.Errorf("abs %s: %w", rawRoot, err)
		}
		st, err := os.Stat(root)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("stat %s: %w", root, err)
		}
		if !st.IsDir() {
			continue
		}

		dirEntries, err := os.ReadDir(root)
		if err != nil {
			return fmt.Errorf("read root %s: %w", root, err)
		}
		for _, de := range dirEntries {
			if !de.IsDir() {
				continue
			}
			dirName := de.Name()
			if _, ok := entries[dirName]; ok {
				// Use the first-discovered skill for a given directory name.
				continue
			}
			skillPath := filepath.Join(root, dirName)
			skill, err := m.loadSkill(skillPath)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return fmt.Errorf("load skill %s: %w", skillPath, err)
			}
			entries[dirName] = skill
		}
	}
	m.entries = entries
	return nil
}

// Entries returns the cached skills sorted by directory name.
func (m *Manager) Entries() []SkillEntry {
	if len(m.entries) == 0 {
		return nil
	}
	dirs := make([]string, 0, len(m.entries))
	for dir := range m.entries {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	out := make([]SkillEntry, 0, len(dirs))
	for _, dir := range dirs {
		out = append(out, SkillEntry{
			Dir:   dir,
			Skill: m.entries[dir],
		})
	}
	return out
}

// Get returns the skill for the given directory name.
func (m *Manager) Get(dir string) (*Skill, bool) {
	skill, ok := m.entries[dir]
	return skill, ok
}

func (m *Manager) loadSkill(skillPath string) (*Skill, error) {
	skillFile := filepath.Join(skillPath, "SKILL.md")
	data, err := os.ReadFile(skillFile)
	if err != nil {
		return nil, err
	}
	meta, err := parseSkillFrontMatter(data)
	if err != nil {
		return nil, err
	}
	return &Skill{
		Name:        strings.TrimSpace(meta.Name),
		Description: strings.TrimSpace(meta.Description),
		Path:        skillPath,
	}, nil
}

type skillFrontMatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

func parseSkillFrontMatter(data []byte) (skillFrontMatter, error) {
	front, ok, err := extractFrontMatter(data)
	if err != nil {
		return skillFrontMatter{}, err
	}
	if !ok {
		return skillFrontMatter{}, nil
	}
	var meta skillFrontMatter
	if err := yaml.Unmarshal(front, &meta); err != nil {
		return skillFrontMatter{}, fmt.Errorf("parse SKILL.md front matter: %w", err)
	}
	return meta, nil
}

func extractFrontMatter(data []byte) ([]byte, bool, error) {
	r := bufio.NewReader(bytes.NewReader(data))
	first, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return nil, false, err
	}
	if strings.TrimSpace(first) != "---" {
		return nil, false, nil
	}
	var buf bytes.Buffer
	for {
		line, err := r.ReadString('\n')
		if err != nil && err != io.EOF {
			return nil, false, err
		}
		if strings.TrimSpace(line) == "---" {
			return buf.Bytes(), true, nil
		}
		buf.WriteString(line)
		if err == io.EOF {
			return nil, false, nil
		}
	}
}

// AddSkill writes the provided SKILL.md contents and refreshes the registry.
func (m *Manager) AddSkill(name, skillMd string) error {
	if m == nil {
		return fmt.Errorf("skills manager is required")
	}

	target, err := m.resolveWritablePath(name, "SKILL.md")
	if err != nil {
		return err
	}

	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", parent, err)
	}
	if err := fsutil.WriteFileAtomic(target, []byte(skillMd), 0644); err != nil {
		return fmt.Errorf("write %s: %w", target, err)
	}
	return m.Scan()
}

func (m *Manager) resolveWritablePath(skillName, relPath string) (string, error) {
	root := strings.TrimSpace(m.WritableRoot)
	if root == "" {
		return "", fmt.Errorf("skills writable root is not configured")
	}
	sanitized, err := sanitizeSkillName(skillName)
	if err != nil {
		return "", err
	}
	skillBase, err := vfsutil.SafeJoinBaseDir(root, sanitized)
	if err != nil {
		return "", fmt.Errorf("skill directory %s: %w", sanitized, err)
	}
	cleanRel := strings.TrimLeft(relPath, "/")
	if cleanRel == "" || cleanRel == "." {
		return "", fmt.Errorf("relative path under %s is required", sanitized)
	}
	target, err := vfsutil.SafeJoinBaseDir(skillBase, cleanRel)
	if err != nil {
		return "", fmt.Errorf("skill path %s: %w", cleanRel, err)
	}
	return target, nil
}

func sanitizeSkillName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("skill name is required")
	}
	if name == "." || name == ".." {
		return "", fmt.Errorf("invalid skill name %q", name)
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return "", fmt.Errorf("invalid skill name %q", name)
	}
	return name, nil
}
