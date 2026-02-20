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

	"github.com/tinoosan/agen8/pkg/fsutil"
	"github.com/tinoosan/agen8/pkg/vfsutil"
	"gopkg.in/yaml.v3"
)

type SkillEntry struct {
	Dir   string
	Skill *Skill
}

type Manager struct {
	roots         []string
	WritableRoot  string
	entries       map[string]*Skill
	AllowedSkills []string // when non-empty, only these skill names are visible
	// EnforceAllowlist turns AllowedSkills into a strict allowlist.
	// When true and AllowedSkills is empty, no skills are visible.
	EnforceAllowlist bool
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
			if !de.IsDir() {
				continue
			}
			dirName := strings.TrimSpace(de.Name())
			if dirName == "" || strings.HasPrefix(dirName, ".") {
				continue
			}
			if _, err := sanitizeSkillName(dirName); err != nil {
				continue
			}
			if _, ok := entries[dirName]; ok {
				continue
			}
			skillDir := filepath.Join(root, dirName)
			skillPath := filepath.Join(skillDir, "SKILL.md")
			skill, err := m.loadSkillFile(dirName, skillDir, skillPath)
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

func (m *Manager) Entries() []SkillEntry {
	if len(m.entries) == 0 {
		return nil
	}
	allowed, restricted := m.allowedSet()
	dirs := make([]string, 0, len(m.entries))
	for dir := range m.entries {
		if restricted {
			if _, ok := allowed[dir]; !ok {
				continue
			}
		}
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
	if !m.isAllowed(dir) {
		return nil, false
	}
	skill, ok := m.entries[dir]
	return skill, ok
}

func (m *Manager) ScriptsManifest() []SkillScripts {
	entries := m.Entries()
	if len(entries) == 0 {
		return nil
	}

	out := make([]SkillScripts, 0, len(entries))
	for _, entry := range entries {
		if entry.Skill == nil || strings.TrimSpace(entry.Skill.Dir) == "" {
			continue
		}
		scriptsDir := filepath.Join(entry.Skill.Dir, "scripts")
		des, err := os.ReadDir(scriptsDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			continue
		}

		scripts := make([]ScriptEntry, 0, len(des))
		for _, de := range des {
			if de.IsDir() {
				continue
			}
			name := strings.TrimSpace(de.Name())
			if name == "" || strings.HasPrefix(name, ".") {
				continue
			}
			info, err := de.Info()
			if err != nil || !info.Mode().IsRegular() {
				continue
			}
			scripts = append(scripts, ScriptEntry{
				Name: name,
				Rel:  filepath.ToSlash(filepath.Join("scripts", name)),
			})
		}
		if len(scripts) == 0 {
			continue
		}
		sort.Slice(scripts, func(i, j int) bool { return scripts[i].Name < scripts[j].Name })
		out = append(out, SkillScripts{
			Skill:   entry.Dir,
			Scripts: scripts,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Skill < out[j].Skill })
	return out
}

func (m *Manager) allowedSet() (map[string]struct{}, bool) {
	if len(m.AllowedSkills) == 0 {
		if m != nil && m.EnforceAllowlist {
			return map[string]struct{}{}, true
		}
		return nil, false
	}
	allowed := make(map[string]struct{}, len(m.AllowedSkills))
	for _, name := range m.AllowedSkills {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		allowed[name] = struct{}{}
	}
	if len(allowed) == 0 {
		if m != nil && m.EnforceAllowlist {
			return map[string]struct{}{}, true
		}
		return nil, false
	}
	return allowed, true
}

func (m *Manager) isAllowed(dir string) bool {
	allowed, restricted := m.allowedSet()
	if !restricted {
		return true
	}
	_, ok := allowed[dir]
	return ok
}

func (m *Manager) loadSkillFile(skillName, skillDir, skillFilePath string) (*Skill, error) {
	data, err := os.ReadFile(skillFilePath)
	if err != nil {
		return nil, err
	}
	meta, err := parseSkillFrontMatter(data)
	if err != nil {
		return nil, err
	}
	return &Skill{
		Name:          defaultIfBlank(strings.TrimSpace(meta.Name), skillName),
		Description:   strings.TrimSpace(meta.Description),
		Compatibility: strings.TrimSpace(meta.Compatibility),
		Dir:           skillDir,
		Path:          skillFilePath,
	}, nil
}

type skillFrontMatter struct {
	Name          string `yaml:"name"`
	Description   string `yaml:"description"`
	Compatibility string `yaml:"compatibility"`
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
	target, err := m.resolveWritableSkillFile(name)
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

func (m *Manager) resolveWritableSkillFile(skillName string) (string, error) {
	return m.resolveWritableFile(skillName, "SKILL.md")
}

func (m *Manager) resolveWritableFile(skillName string, rel string) (string, error) {
	root := strings.TrimSpace(m.WritableRoot)
	if root == "" {
		return "", fmt.Errorf("skills writable root is not configured")
	}
	sanitized, err := sanitizeSkillName(skillName)
	if err != nil {
		return "", err
	}
	rel = strings.TrimLeft(strings.TrimSpace(rel), "/")
	if rel == "" {
		return "", fmt.Errorf("skill relative path is required")
	}
	target, err := vfsutil.SafeJoinBaseDir(root, filepath.ToSlash(filepath.Join(sanitized, rel)))
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
