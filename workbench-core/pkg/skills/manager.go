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

	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/vfsutil"
	"gopkg.in/yaml.v3"
)

type SkillEntry struct {
	Dir   string
	Skill *Skill
}

type Manager struct {
	roots       []string
	WritableRoot string
	entries     map[string]*Skill
}

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
			if de.IsDir() {
				continue
			}
			filename := de.Name()
			if strings.HasPrefix(filename, ".") {
				continue
			}
			if strings.ToLower(filepath.Ext(filename)) != ".md" {
				continue
			}
			skillName := strings.TrimSuffix(filename, filepath.Ext(filename))
			if strings.TrimSpace(skillName) == "" {
				continue
			}
			if _, ok := entries[skillName]; ok {
				continue
			}
			skillPath := filepath.Join(root, filename)
			skill, err := m.loadSkillFile(skillName, skillPath)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return fmt.Errorf("load skill %s: %w", skillPath, err)
			}
			entries[skillName] = skill
		}
	}
	m.entries = entries
	return nil
}

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

func (m *Manager) Get(dir string) (*Skill, bool) {
	skill, ok := m.entries[dir]
	return skill, ok
}

func (m *Manager) loadSkillFile(skillName, skillFilePath string) (*Skill, error) {
	data, err := os.ReadFile(skillFilePath)
	if err != nil {
		return nil, err
	}
	meta, err := parseSkillFrontMatter(data)
	if err != nil {
		return nil, err
	}
	return &Skill{
		Name:        defaultIfBlank(strings.TrimSpace(meta.Name), skillName),
		Description: strings.TrimSpace(meta.Description),
		Path:        skillFilePath,
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
		return skillFrontMatter{}, fmt.Errorf("parse skill front matter: %w", err)
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

func (m *Manager) AddSkill(name, skillMd string) error {
	if m == nil {
		return fmt.Errorf("skills manager is required")
	}
	target, err := m.resolveWritablePath(name)
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

func (m *Manager) resolveWritablePath(skillName string) (string, error) {
	root := strings.TrimSpace(m.WritableRoot)
	if root == "" {
		return "", fmt.Errorf("skills writable root is not configured")
	}
	sanitized, err := sanitizeSkillName(skillName)
	if err != nil {
		return "", err
	}
	target, err := vfsutil.SafeJoinBaseDir(root, sanitized+".md")
	if err != nil {
		return "", fmt.Errorf("skill file %s: %w", sanitized, err)
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

func defaultIfBlank(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}
