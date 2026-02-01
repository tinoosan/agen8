package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Profile struct {
	ID          string        `yaml:"id"`
	Description string        `yaml:"description"`
	Prompts     PromptConfig  `yaml:"prompts,omitempty"`
	Skills      []string      `yaml:"skills,omitempty"`
	Heartbeat   []HeartbeatJob `yaml:"heartbeat,omitempty"`
}

type PromptConfig struct {
	SystemPrompt     string `yaml:"system_prompt,omitempty"`
	SystemPromptPath string `yaml:"system_prompt_path,omitempty"`
}

type HeartbeatJob struct {
	Name     string        `yaml:"name"`
	Interval time.Duration `yaml:"interval"`
	Goal     string        `yaml:"goal"`
}

// Load reads one profile from a profile directory (containing profile.yaml).
// path may be a directory or a direct path to profile.yaml.
func Load(path string) (*Profile, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("profile path is required")
	}

	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	profileDir := path
	profileFile := ""
	if st.IsDir() {
		profileFile = filepath.Join(profileDir, "profile.yaml")
	} else {
		profileFile = path
		profileDir = filepath.Dir(path)
	}

	raw, err := os.ReadFile(profileFile)
	if err != nil {
		return nil, err
	}
	var p Profile
	if err := yaml.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("parse profile.yaml: %w", err)
	}

	// Default prompt.md if present and no prompts explicitly configured.
	if strings.TrimSpace(p.Prompts.SystemPrompt) == "" && strings.TrimSpace(p.Prompts.SystemPromptPath) == "" {
		if _, err := os.Stat(filepath.Join(profileDir, "prompt.md")); err == nil {
			p.Prompts.SystemPromptPath = "prompt.md"
		}
	}

	np, err := p.Normalize(profileDir)
	if err != nil {
		return nil, err
	}
	return &np, nil
}

func (p Profile) Normalize(profileDir string) (Profile, error) {
	p.ID = strings.TrimSpace(p.ID)
	p.Description = strings.TrimSpace(p.Description)
	p.Prompts.SystemPrompt = strings.TrimSpace(p.Prompts.SystemPrompt)
	p.Prompts.SystemPromptPath = strings.TrimSpace(p.Prompts.SystemPromptPath)

	// De-duplicate skills, keep order.
	uniq := make([]string, 0, len(p.Skills))
	seen := map[string]struct{}{}
	for _, s := range p.Skills {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		key := strings.ToLower(s)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		uniq = append(uniq, s)
	}
	p.Skills = uniq

	for i := range p.Heartbeat {
		p.Heartbeat[i].Name = strings.TrimSpace(p.Heartbeat[i].Name)
		p.Heartbeat[i].Goal = strings.TrimSpace(p.Heartbeat[i].Goal)
	}

	if err := p.Validate(profileDir); err != nil {
		return Profile{}, err
	}
	return p, nil
}

func (p Profile) Validate(profileDir string) error {
	if strings.TrimSpace(p.ID) == "" {
		return fmt.Errorf("profile id is required")
	}
	if strings.TrimSpace(p.Description) == "" {
		return fmt.Errorf("profile %s: description is required", p.ID)
	}
	if strings.TrimSpace(p.Prompts.SystemPrompt) == "" && strings.TrimSpace(p.Prompts.SystemPromptPath) == "" {
		return fmt.Errorf("profile %s: prompts.system_prompt or prompts.system_prompt_path is required", p.ID)
	}
	if strings.TrimSpace(p.Prompts.SystemPromptPath) != "" {
		if strings.Contains(p.Prompts.SystemPromptPath, string(filepath.Separator)+string(filepath.Separator)) {
			return fmt.Errorf("profile %s: prompts.system_prompt_path is invalid", p.ID)
		}
		if strings.Contains(p.Prompts.SystemPromptPath, "..") {
			return fmt.Errorf("profile %s: prompts.system_prompt_path must be relative to profile dir", p.ID)
		}
		if strings.TrimSpace(profileDir) != "" {
			pth := filepath.Join(profileDir, p.Prompts.SystemPromptPath)
			if _, err := os.Stat(pth); err != nil {
				return fmt.Errorf("profile %s: prompt file not found: %s", p.ID, p.Prompts.SystemPromptPath)
			}
		}
	}
	for _, hb := range p.Heartbeat {
		if strings.TrimSpace(hb.Name) == "" {
			return fmt.Errorf("profile %s: heartbeat job name is required", p.ID)
		}
		if hb.Interval <= 0 {
			return fmt.Errorf("profile %s: heartbeat job %s interval must be > 0", p.ID, hb.Name)
		}
		if strings.TrimSpace(hb.Goal) == "" {
			return fmt.Errorf("profile %s: heartbeat job %s goal is required", p.ID, hb.Name)
		}
	}
	return nil
}

