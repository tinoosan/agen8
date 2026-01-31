package role

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type Manager struct {
	roots        []string
	WritableRoot string
	entries      map[string]Role
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
		entries: make(map[string]Role),
	}
}

func (m *Manager) Scan() error {
	entries := make(map[string]Role)
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
			name := de.Name()
			if !strings.HasSuffix(strings.ToLower(name), ".md") {
				continue
			}
			rolePath := filepath.Join(root, name)
			role, err := loadRoleFile(rolePath)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return fmt.Errorf("load role %s: %w", rolePath, err)
			}
			key := strings.ToLower(strings.TrimSpace(role.Name))
			if key == "" {
				key = strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
				role.Name = strings.TrimSuffix(name, filepath.Ext(name))
			}
			if key == "" {
				continue
			}
			if _, ok := entries[key]; ok {
				continue
			}
			entries[key] = role
		}
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

func (m *Manager) Get(name string) (Role, bool) {
	if m == nil || len(m.entries) == 0 {
		return Role{}, false
	}
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		return Role{}, false
	}
	if r, ok := m.entries[key]; ok {
		return r, true
	}
	for k, r := range m.entries {
		if strings.EqualFold(k, key) || strings.EqualFold(r.Name, name) {
			return r, true
		}
	}
	return Role{}, false
}

type roleFrontMatter struct {
	Name          string             `yaml:"name"`
	Description   string             `yaml:"description"`
	StandingGoals []string           `yaml:"standing_goals"`
	Triggers      []roleTriggerFront `yaml:"triggers"`
}

type roleTriggerFront struct {
	Type      string `yaml:"type"`
	Interval  string `yaml:"interval"`
	TimeOfDay string `yaml:"time_of_day"`
	Time      string `yaml:"time"`
	Goal      string `yaml:"goal"`
}

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
		Name:          strings.TrimSpace(meta.Name),
		Description:   strings.TrimSpace(meta.Description),
		StandingGoals: meta.StandingGoals,
		Content:       strings.TrimSpace(body),
	}
	for _, t := range meta.Triggers {
		trigger := Trigger{
			Type: strings.ToLower(strings.TrimSpace(t.Type)),
			Goal: strings.TrimSpace(t.Goal),
		}
		if trigger.Type == "" {
			continue
		}
		if trigger.Type == "interval" {
			interval := strings.TrimSpace(t.Interval)
			if interval == "" {
				continue
			}
			d, err := time.ParseDuration(interval)
			if err != nil {
				return Role{}, fmt.Errorf("parse interval %q: %w", interval, err)
			}
			trigger.Interval = d
		}
		if trigger.Type == "time_of_day" {
			tod := strings.TrimSpace(t.TimeOfDay)
			if tod == "" {
				tod = strings.TrimSpace(t.Time)
			}
			if tod == "" {
				continue
			}
			trigger.TimeOfDay = tod
		}
		role.Triggers = append(role.Triggers, trigger)
	}
	return role.Normalize(), nil
}

func parseRoleMarkdown(data []byte) (roleFrontMatter, string, error) {
	front, body, ok, err := splitFrontMatter(data)
	if err != nil {
		return roleFrontMatter{}, "", err
	}
	if !ok {
		return roleFrontMatter{}, strings.TrimSpace(string(data)), nil
	}
	var meta roleFrontMatter
	if err := yaml.Unmarshal(front, &meta); err != nil {
		return roleFrontMatter{}, "", fmt.Errorf("parse role front matter: %w", err)
	}
	return meta, strings.TrimSpace(body), nil
}

func splitFrontMatter(data []byte) ([]byte, string, bool, error) {
	r := bufio.NewReader(bytes.NewReader(data))
	first, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return nil, "", false, err
	}
	if strings.TrimSpace(first) != "---" {
		return nil, strings.TrimSpace(string(data)), false, nil
	}
	var front bytes.Buffer
	var body bytes.Buffer
	inFront := true
	for {
		line, err := r.ReadString('\n')
		if err != nil && err != io.EOF {
			return nil, "", false, err
		}
		if inFront && strings.TrimSpace(line) == "---" {
			inFront = false
			if err == io.EOF {
				break
			}
			continue
		}
		if inFront {
			front.WriteString(line)
		} else {
			body.WriteString(line)
		}
		if err == io.EOF {
			break
		}
	}
	return front.Bytes(), strings.TrimSpace(body.String()), true, nil
}

var (
	defaultManagerMu sync.RWMutex
	defaultManager   *Manager
)

func SetDefaultManager(m *Manager) {
	defaultManagerMu.Lock()
	defer defaultManagerMu.Unlock()
	defaultManager = m
}

func ReloadDefaultManager() {
	defaultManagerMu.RLock()
	m := defaultManager
	defaultManagerMu.RUnlock()
	if m == nil {
		return
	}
	_ = m.Scan()
}

func getDefaultRole(name string) (Role, bool) {
	defaultManagerMu.RLock()
	m := defaultManager
	defaultManagerMu.RUnlock()
	if m == nil {
		return Role{}, false
	}
	return m.Get(name)
}
