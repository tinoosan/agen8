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
	ID                      string         `yaml:"id"`
	Name                    string         `yaml:"name,omitempty"` // Display name for standalone profiles; used as synthetic role name when set
	Description             string         `yaml:"description"`
	Model                   string         `yaml:"model,omitempty"`
	SubagentModel           string         `yaml:"subagent_model,omitempty"`
	CodeExecOnly            bool           `yaml:"code_exec_only,omitempty"`
	CodeExecRequiredImports []string       `yaml:"code_exec_required_imports,omitempty"`
	AllowedTools            []string       `yaml:"allowed_tools,omitempty"`
	Prompts                 PromptConfig   `yaml:"prompts,omitempty"`
	Skills                  []string       `yaml:"skills,omitempty"`
	Heartbeat               HeartbeatConfig `yaml:"heartbeat,omitempty"`
	Team                    *TeamConfig    `yaml:"team,omitempty"`
}

// HeartbeatConfig holds heartbeat jobs and an optional enabled flag.
// YAML: heartbeat.enabled and heartbeat.jobs (or legacy: heartbeat as a list).
type HeartbeatConfig struct {
	Enabled *bool          `yaml:"enabled,omitempty"`
	Jobs    []HeartbeatJob `yaml:"jobs,omitempty"`
}

// UnmarshalYAML supports both formats:
//   - legacy: heartbeat: [{name: x, interval: 1m, goal: y}]
//   - new: heartbeat: {enabled: false, jobs: [{name: x, ...}]}
func (c *HeartbeatConfig) UnmarshalYAML(n *yaml.Node) error {
	if n == nil || n.Kind == 0 {
		return nil
	}
	if n.Kind == yaml.SequenceNode {
		var jobs []HeartbeatJob
		if err := n.Decode(&jobs); err != nil {
			return err
		}
		c.Jobs = jobs
		return nil
	}
	type raw HeartbeatConfig
	var tmp raw
	if err := n.Decode(&tmp); err != nil {
		return err
	}
	c.Enabled = tmp.Enabled
	c.Jobs = tmp.Jobs
	return nil
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

// RolesForSession returns the roles to use when starting a session.
// For team profiles, returns Team.Roles. For standalone profiles (no team block),
// returns a synthetic single role "agent" with coordinator=true and allow_subagents=true.
func (p *Profile) RolesForSession() ([]RoleConfig, error) {
	if p == nil {
		return nil, fmt.Errorf("profile is nil")
	}
	if p.Team != nil && len(p.Team.Roles) > 0 {
		return p.Team.Roles, nil
	}
	// Standalone: synthetic single role
	roleName := "agent"
	if n := strings.TrimSpace(p.Name); n != "" {
		roleName = n
	}
	codeExec := p.CodeExecOnly
	r := RoleConfig{
		Name:           roleName,
		Description:    strings.TrimSpace(p.Description),
		Prompts:        p.Prompts,
		Skills:         append([]string(nil), p.Skills...),
		CodeExecOnly:   &codeExec,
		AllowedTools:   append([]string(nil), p.AllowedTools...),
		Model:          strings.TrimSpace(p.Model),
		SubagentModel:  strings.TrimSpace(p.SubagentModel),
		Coordinator:    true,
		AllowSubagents: true,
	}
	return []RoleConfig{r}, nil
}

// TeamModelForSession returns the model to use for the session (team or standalone).
// For team profiles, returns Team.Model or first role model. For standalone, returns Model.
func (p *Profile) TeamModelForSession() string {
	if p == nil {
		return ""
	}
	if p.Team != nil {
		if m := strings.TrimSpace(p.Team.Model); m != "" {
			return m
		}
		for _, r := range p.Team.Roles {
			if m := strings.TrimSpace(r.Model); m != "" {
				return m
			}
		}
	}
	return strings.TrimSpace(p.Model)
}

// EffectiveHeartbeats returns the heartbeat jobs to run. When heartbeat.enabled is false,
// returns nil so heartbeats are disabled without removing the entries from the profile.
func (p *Profile) EffectiveHeartbeats() []HeartbeatJob {
	if p == nil {
		return nil
	}
	if p.Heartbeat.Enabled != nil && !*p.Heartbeat.Enabled {
		return nil
	}
	return p.Heartbeat.Jobs
}

type TeamConfig struct {
	Model string       `yaml:"model,omitempty"`
	Roles []RoleConfig `yaml:"roles"`
}

type RoleConfig struct {
	Name                    string           `yaml:"name"`
	Description             string           `yaml:"description"`
	Prompts                 PromptConfig     `yaml:"prompts,omitempty"`
	Skills                  []string         `yaml:"skills,omitempty"`
	CodeExecOnly            *bool            `yaml:"code_exec_only,omitempty"`
	CodeExecRequiredImports []string         `yaml:"code_exec_required_imports,omitempty"`
	AllowedTools            []string         `yaml:"allowed_tools,omitempty"`
	Model                   string           `yaml:"model,omitempty"`
	SubagentModel           string           `yaml:"subagent_model,omitempty"`
	Coordinator             bool             `yaml:"coordinator,omitempty"`
	Reviewer                bool             `yaml:"reviewer,omitempty"`
	AllowSubagents          bool             `yaml:"allow_subagents,omitempty"`
	Heartbeat               HeartbeatConfig  `yaml:"heartbeat,omitempty"`
}

// ResolveByRef resolves a profile reference to a loaded profile and its directory.
// requested may be: empty (defaults to "general"), a profile name (looked up under profilesDir), or a direct path.
// profilesDir is the base profiles directory (e.g. dataDir/profiles).
func ResolveByRef(profilesDir, requested string) (*Profile, string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		requested = "general"
	}
	if st, err := os.Stat(requested); err == nil {
		if st.IsDir() {
			p, err := Load(requested)
			return p, requested, err
		}
		dir := filepath.Dir(requested)
		p, err := Load(requested)
		return p, dir, err
	}
	dir := filepath.Join(strings.TrimSpace(profilesDir), requested)
	p, err := Load(dir)
	return p, dir, err
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
	p.Name = strings.TrimSpace(p.Name)
	p.Description = strings.TrimSpace(p.Description)
	p.Model = strings.TrimSpace(p.Model)
	p.SubagentModel = strings.TrimSpace(p.SubagentModel)
	p.CodeExecRequiredImports = normalizeStringList(p.CodeExecRequiredImports)
	p.AllowedTools = normalizeStringList(p.AllowedTools)
	p.Prompts.SystemPrompt = strings.TrimSpace(p.Prompts.SystemPrompt)
	p.Prompts.SystemPromptPath = strings.TrimSpace(p.Prompts.SystemPromptPath)
	p.Skills = normalizeStringList(p.Skills)

	for i := range p.Heartbeat.Jobs {
		p.Heartbeat.Jobs[i].Name = strings.TrimSpace(p.Heartbeat.Jobs[i].Name)
		p.Heartbeat.Jobs[i].Goal = strings.TrimSpace(p.Heartbeat.Jobs[i].Goal)
	}
	if p.Team != nil {
		p.Team.Model = strings.TrimSpace(p.Team.Model)
		for i := range p.Team.Roles {
			r := &p.Team.Roles[i]
			r.Name = strings.TrimSpace(r.Name)
			r.Description = strings.TrimSpace(r.Description)
			r.Prompts.SystemPrompt = strings.TrimSpace(r.Prompts.SystemPrompt)
			r.Prompts.SystemPromptPath = strings.TrimSpace(r.Prompts.SystemPromptPath)
			r.Model = strings.TrimSpace(r.Model)
			r.SubagentModel = strings.TrimSpace(r.SubagentModel)
			r.CodeExecRequiredImports = normalizeStringList(r.CodeExecRequiredImports)
			r.AllowedTools = normalizeStringList(r.AllowedTools)
			r.Skills = normalizeStringList(r.Skills)
			for j := range r.Heartbeat.Jobs {
				r.Heartbeat.Jobs[j].Name = strings.TrimSpace(r.Heartbeat.Jobs[j].Name)
				r.Heartbeat.Jobs[j].Goal = strings.TrimSpace(r.Heartbeat.Jobs[j].Goal)
			}
		}
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
	if p.Team == nil && strings.TrimSpace(p.Prompts.SystemPrompt) == "" && strings.TrimSpace(p.Prompts.SystemPromptPath) == "" {
		return fmt.Errorf("profile %s: prompts.system_prompt or prompts.system_prompt_path is required", p.ID)
	}
	if err := validatePromptPath(profileDir, p.Prompts.SystemPromptPath, fmt.Sprintf("profile %s", p.ID)); err != nil {
		return err
	}
	for _, hb := range p.Heartbeat.Jobs {
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
	if p.Team != nil {
		if len(p.Team.Roles) == 0 {
			return fmt.Errorf("profile %s: team.roles must contain at least one role", p.ID)
		}
		seenRoles := map[string]struct{}{}
		coordinators := 0
		reviewers := 0
		for i, role := range p.Team.Roles {
			ref := fmt.Sprintf("profile %s role[%d]", p.ID, i)
			if role.Name == "" {
				return fmt.Errorf("%s: name is required", ref)
			}
			if role.Description == "" {
				return fmt.Errorf("%s (%s): description is required", ref, role.Name)
			}
			if role.Prompts.SystemPrompt == "" && role.Prompts.SystemPromptPath == "" {
				return fmt.Errorf("%s (%s): prompts.system_prompt or prompts.system_prompt_path is required", ref, role.Name)
			}
			if err := validatePromptPath(profileDir, role.Prompts.SystemPromptPath, fmt.Sprintf("%s (%s)", ref, role.Name)); err != nil {
				return err
			}
			for _, hb := range role.Heartbeat.Jobs {
				if strings.TrimSpace(hb.Name) == "" {
					return fmt.Errorf("%s (%s): heartbeat job name is required", ref, role.Name)
				}
				if hb.Interval <= 0 {
					return fmt.Errorf("%s (%s): heartbeat job %s interval must be > 0", ref, role.Name, hb.Name)
				}
				if strings.TrimSpace(hb.Goal) == "" {
					return fmt.Errorf("%s (%s): heartbeat job %s goal is required", ref, role.Name, hb.Name)
				}
			}
			key := strings.ToLower(role.Name)
			if _, ok := seenRoles[key]; ok {
				return fmt.Errorf("profile %s: duplicate team role name %q", p.ID, role.Name)
			}
			seenRoles[key] = struct{}{}
			if role.Coordinator {
				coordinators++
			}
			if role.Reviewer {
				reviewers++
			}
		}
		if coordinators != 1 {
			return fmt.Errorf("profile %s: exactly one team role must set coordinator: true", p.ID)
		}
		if reviewers > 1 {
			return fmt.Errorf("profile %s: at most one team role may set reviewer: true", p.ID)
		}
	}
	return nil
}

func validatePromptPath(profileDir, promptPath, scope string) error {
	promptPath = strings.TrimSpace(promptPath)
	if promptPath == "" {
		return nil
	}
	if strings.Contains(promptPath, string(filepath.Separator)+string(filepath.Separator)) {
		return fmt.Errorf("%s: prompts.system_prompt_path is invalid", scope)
	}
	if strings.Contains(promptPath, "..") {
		return fmt.Errorf("%s: prompts.system_prompt_path must be relative to profile dir", scope)
	}
	if strings.TrimSpace(profileDir) != "" {
		pth := filepath.Join(profileDir, promptPath)
		if _, err := os.Stat(pth); err != nil {
			return fmt.Errorf("%s: prompt file not found: %s", scope, promptPath)
		}
	}
	return nil
}

func normalizeStringList(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, item := range in {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		key := strings.ToLower(item)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}
