package role

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Manager discovers roles from the filesystem.
// Roles are expected at <dataDir>/roles/<role_name>/ROLE.md
// Only front matter is authoritative; body is guidance.
type Manager struct {
	roots        []string
	WritableRoot string
	entries      map[string]Role
}

func NewManager(roots []string) *Manager {
	filtered := make([]string, 0, len(roots))
	for _, r := range roots {
		if strings.TrimSpace(r) == "" {
			continue
		}
		filtered = append(filtered, r)
	}
	return &Manager{roots: filtered, entries: map[string]Role{}}
}

// Scan loads all roles. Fails if no valid roles are found.
func (m *Manager) Scan() error {
	entries := map[string]Role{}
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
			roleDir := filepath.Join(root, de.Name())
			rolePath := filepath.Join(roleDir, "ROLE.md")
			r, err := loadRoleFile(rolePath)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return fmt.Errorf("load role %s: %w", rolePath, err)
			}
			key := strings.ToLower(strings.TrimSpace(r.ID))
			if key == "" {
				key = strings.ToLower(strings.TrimSpace(de.Name()))
			}
			if key == "" {
				continue
			}
			if _, exists := entries[key]; exists {
				return fmt.Errorf("duplicate role id %s", key)
			}
			entries[key] = r
		}
	}
	if len(entries) == 0 {
		return fmt.Errorf("no valid roles found in roots: %v", m.roots)
	}
	m.entries = entries
	return nil
}

func (m *Manager) Entries() []Role {
	if len(m.entries) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m.entries))
	for k := range m.entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]Role, 0, len(keys))
	for _, k := range keys {
		out = append(out, m.entries[k])
	}
	return out
}

func (m *Manager) Get(id string) (Role, bool) {
	if m == nil {
		return Role{}, false
	}
	key := strings.ToLower(strings.TrimSpace(id))
	if key == "" {
		return Role{}, false
	}
	r, ok := m.entries[key]
	return r, ok
}

// loadRoleFile parses ROLE.md front matter using the required schema.
func loadRoleFile(path string) (Role, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Role{}, err
	}
	meta, body, err := parseRoleMarkdown(data)
	if err != nil {
		return Role{}, err
	}
	role := Role{
		ID:          strings.TrimSpace(meta.ID),
		Description: strings.TrimSpace(meta.Description),
		SkillBias:   meta.SkillBias,
		Obligations: meta.Obligations,
		TaskPolicy:  meta.TaskPolicy,
		Guidance:    strings.TrimSpace(body),
	}
	normalized, err := role.Normalize()
	if err != nil {
		return Role{}, err
	}
	return normalized, nil
}

type roleFrontMatter struct {
	// YAML uses snake_case for multiword keys to keep ROLE.md configuration consistent and readable.
	ID          string       `yaml:"id"`
	Description string       `yaml:"description"`
	SkillBias   []string     `yaml:"skill_bias"`
	Obligations []Obligation `yaml:"obligations"`
	TaskPolicy  TaskPolicy   `yaml:"task_policy"`
}

func parseRoleMarkdown(data []byte) (roleFrontMatter, string, error) {
	front, body, ok, err := splitFrontMatter(data)
	if err != nil {
		return roleFrontMatter{}, "", err
	}
	if !ok {
		return roleFrontMatter{}, "", fmt.Errorf("ROLE.md missing YAML front matter")
	}
	var meta roleFrontMatter
	if err := yaml.Unmarshal(front, &meta); err != nil {
		return roleFrontMatter{}, "", fmt.Errorf("parse ROLE.md front matter: %w", err)
	}
	return meta, strings.TrimSpace(body), nil
}

func splitFrontMatter(data []byte) ([]byte, string, bool, error) {
	r := bytes.NewReader(data)
	var frontBuf bytes.Buffer
	var bodyBuf bytes.Buffer
	inFront := false
	doneFront := false

	for {
		line, err := readLine(r)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "", false, err
		}
		trimmed := strings.TrimSpace(line)
		if !inFront {
			if trimmed == "---" {
				inFront = true
				continue
			}
			// no front matter
			return nil, strings.TrimSpace(string(data)), false, nil
		}
		if inFront && trimmed == "---" {
			doneFront = true
			break
		}
		if inFront {
			frontBuf.WriteString(line)
			if !strings.HasSuffix(line, "\n") {
				frontBuf.WriteString("\n")
			}
		}
	}

	if !doneFront {
		return nil, "", false, fmt.Errorf("unterminated front matter")
	}

	// Remainder is body.
	rest, _ := io.ReadAll(r)
	bodyBuf.Write(rest)
	return frontBuf.Bytes(), strings.TrimSpace(bodyBuf.String()), true, nil
}

func readLine(r *bytes.Reader) (string, error) {
	var buf bytes.Buffer
	for {
		b, err := r.ReadByte()
		if err != nil {
			if err == io.EOF {
				if buf.Len() == 0 {
					return "", io.EOF
				}
				return buf.String(), io.EOF
			}
			return "", err
		}
		buf.WriteByte(b)
		if b == '\n' {
			return buf.String(), nil
		}
	}
}

// Default manager for control plane updates.
var (
	defaultManager *Manager
)

func SetDefaultManager(m *Manager) {
	defaultManager = m
}

func ReloadDefaultManager() {
	if defaultManager == nil {
		return
	}
	_ = defaultManager.Scan()
}

func getDefaultRole(id string) (Role, bool) {
	if defaultManager == nil {
		return Role{}, false
	}
	return defaultManager.Get(id)
}

// GetDefault looks up a role from the default manager.
func GetDefault(id string) (Role, bool) {
	return getDefaultRole(id)
}
