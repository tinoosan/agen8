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
	ID                      string          `yaml:"id"`
	Name                    string          `yaml:"name,omitempty"` // Display name for standalone profiles; used as synthetic role name when set
	Description             string          `yaml:"description"`
	Model                   string          `yaml:"model,omitempty"`
	SubagentModel           string          `yaml:"subagentModel,omitempty"`
	CodeExecOnly            bool            `yaml:"codeExecOnly,omitempty"`
	CodeExecRequiredImports []string        `yaml:"codeExecRequiredImports,omitempty"`
	AllowedTools            []string        `yaml:"allowedTools,omitempty"`
	Prompts                 PromptConfig    `yaml:"prompts,omitempty"`
	Skills                  []string        `yaml:"skills,omitempty"`
	Heartbeat               HeartbeatConfig `yaml:"heartbeat,omitempty"`
	Team                    *TeamConfig     `yaml:"team,omitempty"`
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

// PromptFragment represents a single composable prompt fragment — either an inline
// string or a path to a file (relative to the profile directory).
type PromptFragment struct {
	Path   string `yaml:"path,omitempty"`
	Inline string `yaml:"inline,omitempty"`
}

type PromptConfig struct {
	SystemPrompt     string           `yaml:"systemPrompt,omitempty"`
	SystemPromptPath string           `yaml:"systemPromptPath,omitempty"`
	SystemFragments  []PromptFragment `yaml:"systemFragments,omitempty"`
}

// EffectiveFragments normalizes legacy systemPrompt / systemPromptPath into a
// []PromptFragment list. If systemFragments is already set, returns it directly.
func (pc PromptConfig) EffectiveFragments() []PromptFragment {
	if len(pc.SystemFragments) > 0 {
		return pc.SystemFragments
	}
	if s := strings.TrimSpace(pc.SystemPrompt); s != "" {
		return []PromptFragment{{Inline: s}}
	}
	if s := strings.TrimSpace(pc.SystemPromptPath); s != "" {
		return []PromptFragment{{Path: s}}
	}
	return nil
}

type HeartbeatJob struct {
	Name     string        `yaml:"name"`
	Interval time.Duration `yaml:"interval"`
	Goal     string        `yaml:"goal"`
}

// RolesForSession returns the roles to use when starting a session.
// For team profiles, returns Team.Roles. For standalone profiles (no team block),
// returns a synthetic single role "agent" with coordinator=true and allowSubagents=true.
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
	Model    string          `yaml:"model,omitempty"`
	Reviewer *ReviewerConfig `yaml:"reviewer,omitempty"`
	Roles    []RoleConfig    `yaml:"roles"`
}

type ReviewerConfig struct {
	RoleRef                 string          `yaml:"roleRef,omitempty"`
	Enabled                 bool            `yaml:"enabled,omitempty"`
	Name                    string          `yaml:"name,omitempty"`
	Description             string          `yaml:"description,omitempty"`
	Prompts                 PromptConfig    `yaml:"prompts,omitempty"`
	Skills                  []string        `yaml:"skills,omitempty"`
	CodeExecOnly            *bool           `yaml:"codeExecOnly,omitempty"`
	CodeExecRequiredImports []string        `yaml:"codeExecRequiredImports,omitempty"`
	AllowedTools            []string        `yaml:"allowedTools,omitempty"`
	Model                   string          `yaml:"model,omitempty"`
	Heartbeat               HeartbeatConfig `yaml:"heartbeat,omitempty"`
}

type RoleConfig struct {
	RoleRef                 string          `yaml:"roleRef,omitempty"`
	Name                    string          `yaml:"name"`
	Description             string          `yaml:"description"`
	Prompts                 PromptConfig    `yaml:"prompts,omitempty"`
	Skills                  []string        `yaml:"skills,omitempty"`
	CodeExecOnly            *bool           `yaml:"codeExecOnly,omitempty"`
	CodeExecRequiredImports []string        `yaml:"codeExecRequiredImports,omitempty"`
	AllowedTools            []string        `yaml:"allowedTools,omitempty"`
	Model                   string          `yaml:"model,omitempty"`
	SubagentModel           string          `yaml:"subagentModel,omitempty"`
	Coordinator             bool            `yaml:"coordinator,omitempty"`
	AllowSubagents          bool            `yaml:"allowSubagents,omitempty"`
	Heartbeat               HeartbeatConfig `yaml:"heartbeat,omitempty"`
	Replicas                *int            `yaml:"replicas,omitempty"`
}

func (r ReviewerConfig) EffectiveName() string {
	name := strings.TrimSpace(r.Name)
	if name == "" {
		return "reviewer"
	}
	return name
}

func (p *Profile) ReviewerForSession() (*ReviewerConfig, bool) {
	if p == nil || p.Team == nil || p.Team.Reviewer == nil || !p.Team.Reviewer.Enabled {
		return nil, false
	}
	reviewer := *p.Team.Reviewer
	reviewer.Name = reviewer.EffectiveName()
	return &reviewer, true
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
	if strings.TrimSpace(p.Prompts.SystemPrompt) == "" && strings.TrimSpace(p.Prompts.SystemPromptPath) == "" && len(p.Prompts.SystemFragments) == 0 {
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
	normalizeFragments(p.Prompts.SystemFragments)
	p.Skills = normalizeStringList(p.Skills)

	for i := range p.Heartbeat.Jobs {
		p.Heartbeat.Jobs[i].Name = strings.TrimSpace(p.Heartbeat.Jobs[i].Name)
		p.Heartbeat.Jobs[i].Goal = strings.TrimSpace(p.Heartbeat.Jobs[i].Goal)
	}
	if p.Team != nil {
		p.Team.Model = strings.TrimSpace(p.Team.Model)
		// Resolve role refs before normalization/validation.
		if p.Team.Reviewer != nil {
			if err := resolveReviewerRef(profileDir, p.Team.Reviewer); err != nil {
				return Profile{}, err
			}
		}
		for i := range p.Team.Roles {
			if err := resolveRoleRef(profileDir, &p.Team.Roles[i]); err != nil {
				return Profile{}, err
			}
		}
		if p.Team.Reviewer != nil {
			r := p.Team.Reviewer
			r.Name = strings.TrimSpace(r.Name)
			r.Description = strings.TrimSpace(r.Description)
			r.Prompts.SystemPrompt = strings.TrimSpace(r.Prompts.SystemPrompt)
			r.Prompts.SystemPromptPath = strings.TrimSpace(r.Prompts.SystemPromptPath)
			normalizeFragments(r.Prompts.SystemFragments)
			r.Model = strings.TrimSpace(r.Model)
			r.CodeExecRequiredImports = normalizeStringList(r.CodeExecRequiredImports)
			r.AllowedTools = normalizeStringList(r.AllowedTools)
			r.Skills = normalizeStringList(r.Skills)
			for j := range r.Heartbeat.Jobs {
				r.Heartbeat.Jobs[j].Name = strings.TrimSpace(r.Heartbeat.Jobs[j].Name)
				r.Heartbeat.Jobs[j].Goal = strings.TrimSpace(r.Heartbeat.Jobs[j].Goal)
			}
		}
		for i := range p.Team.Roles {
			r := &p.Team.Roles[i]
			r.Name = strings.TrimSpace(r.Name)
			r.Description = strings.TrimSpace(r.Description)
			r.Prompts.SystemPrompt = strings.TrimSpace(r.Prompts.SystemPrompt)
			r.Prompts.SystemPromptPath = strings.TrimSpace(r.Prompts.SystemPromptPath)
			normalizeFragments(r.Prompts.SystemFragments)
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
	if err := validatePromptConfig(profileDir, p.Prompts, fmt.Sprintf("profile %s", p.ID), p.Team == nil); err != nil {
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
		for i, role := range p.Team.Roles {
			ref := fmt.Sprintf("profile %s role[%d]", p.ID, i)
			if role.Name == "" {
				return fmt.Errorf("%s: name is required", ref)
			}
			if role.Description == "" {
				return fmt.Errorf("%s (%s): description is required", ref, role.Name)
			}
			if err := validatePromptConfig(profileDir, role.Prompts, fmt.Sprintf("%s (%s)", ref, role.Name), true); err != nil {
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
			if role.Replicas != nil && *role.Replicas < 1 {
				return fmt.Errorf("%s (%s): replicas must be >= 1", ref, role.Name)
			}
			if role.Coordinator && role.Replicas != nil {
				return fmt.Errorf("%s (%s): coordinator role cannot have replicas", ref, role.Name)
			}
			key := strings.ToLower(role.Name)
			if _, ok := seenRoles[key]; ok {
				return fmt.Errorf("profile %s: duplicate team role name %q", p.ID, role.Name)
			}
			seenRoles[key] = struct{}{}
			if role.Coordinator {
				coordinators++
			}
		}
		if coordinators != 1 {
			return fmt.Errorf("profile %s: exactly one team role must set coordinator: true", p.ID)
		}
		if p.Team.Reviewer != nil && p.Team.Reviewer.Enabled {
			reviewerName := p.Team.Reviewer.EffectiveName()
			if _, ok := seenRoles[strings.ToLower(reviewerName)]; ok {
				return fmt.Errorf("profile %s: team.reviewer.name %q collides with team role name", p.ID, reviewerName)
			}
			if strings.TrimSpace(p.Team.Reviewer.Description) == "" {
				return fmt.Errorf("profile %s: team.reviewer.description is required when reviewer is enabled", p.ID)
			}
			if err := validatePromptConfig(profileDir, p.Team.Reviewer.Prompts, fmt.Sprintf("profile %s reviewer", p.ID), true); err != nil {
				return err
			}
		}
	}
	return nil
}

// validatePromptConfig validates that a PromptConfig is correctly configured.
// requirePrompt controls whether at least one prompt source is required.
func validatePromptConfig(profileDir string, pc PromptConfig, scope string, requirePrompt bool) error {
	hasLegacy := strings.TrimSpace(pc.SystemPrompt) != "" || strings.TrimSpace(pc.SystemPromptPath) != ""
	hasFragments := len(pc.SystemFragments) > 0
	if hasLegacy && hasFragments {
		return fmt.Errorf("%s: cannot mix systemPrompt/systemPromptPath with systemFragments", scope)
	}
	if requirePrompt && !hasLegacy && !hasFragments {
		return fmt.Errorf("%s: prompts.systemPrompt, prompts.systemPromptPath, or prompts.systemFragments is required", scope)
	}
	if err := validatePromptPath(profileDir, pc.SystemPromptPath, scope); err != nil {
		return err
	}
	for i, frag := range pc.SystemFragments {
		frag.Path = strings.TrimSpace(frag.Path)
		frag.Inline = strings.TrimSpace(frag.Inline)
		if frag.Path == "" && frag.Inline == "" {
			return fmt.Errorf("%s: systemFragments[%d] must have path or inline", scope, i)
		}
		if frag.Path != "" && frag.Inline != "" {
			return fmt.Errorf("%s: systemFragments[%d] must have path or inline, not both", scope, i)
		}
		if frag.Path != "" {
			if err := validateFragmentPath(profileDir, frag.Path, fmt.Sprintf("%s systemFragments[%d]", scope, i)); err != nil {
				return err
			}
		}
	}
	return nil
}

// validatePromptPath validates a legacy systemPromptPath (no .. traversal allowed).
func validatePromptPath(profileDir, promptPath, scope string) error {
	promptPath = strings.TrimSpace(promptPath)
	if promptPath == "" {
		return nil
	}
	if strings.Contains(promptPath, string(filepath.Separator)+string(filepath.Separator)) {
		return fmt.Errorf("%s: prompts.systemPromptPath is invalid", scope)
	}
	if strings.Contains(promptPath, "..") {
		return fmt.Errorf("%s: prompts.systemPromptPath must be relative to profile dir", scope)
	}
	if strings.TrimSpace(profileDir) != "" {
		pth := filepath.Join(profileDir, promptPath)
		if _, err := os.Stat(pth); err != nil {
			return fmt.Errorf("%s: prompt file not found: %s", scope, promptPath)
		}
	}
	return nil
}

// validateFragmentPath validates a fragment path. Unlike legacy systemPromptPath,
// relative traversal with ".." is allowed for composable prompts.
func validateFragmentPath(profileDir, fragPath, scope string) error {
	fragPath = strings.TrimSpace(fragPath)
	if fragPath == "" {
		return nil
	}
	if filepath.IsAbs(fragPath) {
		return fmt.Errorf("%s: fragment path must be relative", scope)
	}
	if strings.TrimSpace(profileDir) != "" {
		pth := filepath.Join(profileDir, fragPath)
		if _, err := os.Stat(pth); err != nil {
			return fmt.Errorf("%s: fragment file not found: %s", scope, fragPath)
		}
	}
	return nil
}

// resolveRoleRef loads a base role YAML from roleRef and shallow-merges inline
// fields on top. Inline fields override base values. coordinator and allowSubagents
// are always inherited from the base (the profile can override them).
func resolveRoleRef(profileDir string, r *RoleConfig) error {
	ref := strings.TrimSpace(r.RoleRef)
	if ref == "" {
		return nil
	}
	refPath := ref
	if !filepath.IsAbs(refPath) && strings.TrimSpace(profileDir) != "" {
		refPath = filepath.Join(profileDir, refPath)
	}
	raw, err := os.ReadFile(refPath)
	if err != nil {
		return fmt.Errorf("resolve roleRef %s: %w", ref, err)
	}
	var base RoleConfig
	if err := yaml.Unmarshal(raw, &base); err != nil {
		return fmt.Errorf("parse roleRef %s: %w", ref, err)
	}
	// Resolve base prompt paths relative to the ref file's directory.
	baseDir := filepath.Dir(refPath)
	base.Prompts = resolvePromptPaths(base.Prompts, baseDir, profileDir)
	// Shallow merge: inline fields override base.
	mergeRoleConfig(&base, r)
	*r = base
	r.RoleRef = ref // preserve original ref
	return nil
}

// resolveReviewerRef loads a base reviewer/role YAML from roleRef and shallow-merges.
func resolveReviewerRef(profileDir string, r *ReviewerConfig) error {
	ref := strings.TrimSpace(r.RoleRef)
	if ref == "" {
		return nil
	}
	refPath := ref
	if !filepath.IsAbs(refPath) && strings.TrimSpace(profileDir) != "" {
		refPath = filepath.Join(profileDir, refPath)
	}
	raw, err := os.ReadFile(refPath)
	if err != nil {
		return fmt.Errorf("resolve reviewer roleRef %s: %w", ref, err)
	}
	var base ReviewerConfig
	if err := yaml.Unmarshal(raw, &base); err != nil {
		return fmt.Errorf("parse reviewer roleRef %s: %w", ref, err)
	}
	baseDir := filepath.Dir(refPath)
	base.Prompts = resolvePromptPaths(base.Prompts, baseDir, profileDir)
	mergeReviewerConfig(&base, r)
	*r = base
	r.RoleRef = ref
	return nil
}

// resolvePromptPaths rewrites prompt paths from a base role's directory to be
// relative to the profile directory.
func resolvePromptPaths(pc PromptConfig, baseDir, profileDir string) PromptConfig {
	if strings.TrimSpace(profileDir) == "" || strings.TrimSpace(baseDir) == "" {
		return pc
	}
	if p := strings.TrimSpace(pc.SystemPromptPath); p != "" && !filepath.IsAbs(p) {
		absPath := filepath.Join(baseDir, p)
		if rel, err := filepath.Rel(profileDir, absPath); err == nil {
			pc.SystemPromptPath = rel
		}
	}
	for i, f := range pc.SystemFragments {
		if p := strings.TrimSpace(f.Path); p != "" && !filepath.IsAbs(p) {
			absPath := filepath.Join(baseDir, p)
			if rel, err := filepath.Rel(profileDir, absPath); err == nil {
				pc.SystemFragments[i].Path = rel
			}
		}
	}
	return pc
}

// mergeRoleConfig applies inline overrides from src onto base (shallow merge).
func mergeRoleConfig(base, src *RoleConfig) {
	if s := strings.TrimSpace(src.Name); s != "" {
		base.Name = s
	}
	if s := strings.TrimSpace(src.Description); s != "" {
		base.Description = s
	}
	if src.Prompts.SystemPrompt != "" || src.Prompts.SystemPromptPath != "" || len(src.Prompts.SystemFragments) > 0 {
		base.Prompts = src.Prompts
	}
	if len(src.Skills) > 0 {
		base.Skills = src.Skills
	}
	if src.CodeExecOnly != nil {
		base.CodeExecOnly = src.CodeExecOnly
	}
	if len(src.CodeExecRequiredImports) > 0 {
		base.CodeExecRequiredImports = src.CodeExecRequiredImports
	}
	if len(src.AllowedTools) > 0 {
		base.AllowedTools = src.AllowedTools
	}
	if s := strings.TrimSpace(src.Model); s != "" {
		base.Model = s
	}
	if s := strings.TrimSpace(src.SubagentModel); s != "" {
		base.SubagentModel = s
	}
	// coordinator and allowSubagents: inline overrides apply (base defaults carry through)
	if src.Coordinator {
		base.Coordinator = true
	}
	if src.AllowSubagents {
		base.AllowSubagents = true
	}
	if len(src.Heartbeat.Jobs) > 0 || src.Heartbeat.Enabled != nil {
		base.Heartbeat = src.Heartbeat
	}
	if src.Replicas != nil {
		base.Replicas = src.Replicas
	}
}

// mergeReviewerConfig applies inline overrides from src onto base.
func mergeReviewerConfig(base, src *ReviewerConfig) {
	if src.Enabled {
		base.Enabled = true
	}
	if s := strings.TrimSpace(src.Name); s != "" {
		base.Name = s
	}
	if s := strings.TrimSpace(src.Description); s != "" {
		base.Description = s
	}
	if src.Prompts.SystemPrompt != "" || src.Prompts.SystemPromptPath != "" || len(src.Prompts.SystemFragments) > 0 {
		base.Prompts = src.Prompts
	}
	if len(src.Skills) > 0 {
		base.Skills = src.Skills
	}
	if src.CodeExecOnly != nil {
		base.CodeExecOnly = src.CodeExecOnly
	}
	if len(src.CodeExecRequiredImports) > 0 {
		base.CodeExecRequiredImports = src.CodeExecRequiredImports
	}
	if len(src.AllowedTools) > 0 {
		base.AllowedTools = src.AllowedTools
	}
	if s := strings.TrimSpace(src.Model); s != "" {
		base.Model = s
	}
	if len(src.Heartbeat.Jobs) > 0 || src.Heartbeat.Enabled != nil {
		base.Heartbeat = src.Heartbeat
	}
}

func normalizeFragments(frags []PromptFragment) {
	for i := range frags {
		frags[i].Path = strings.TrimSpace(frags[i].Path)
		frags[i].Inline = strings.TrimSpace(frags[i].Inline)
	}
}

// ResolveFragments resolves a PromptConfig's effective fragments into a concatenated
// prompt string. File-based fragments are read relative to profileDir.
func ResolveFragments(profileDir string, pc PromptConfig) (string, error) {
	frags := pc.EffectiveFragments()
	if len(frags) == 0 {
		return "", nil
	}
	parts := make([]string, 0, len(frags))
	for _, f := range frags {
		if f.Inline != "" {
			parts = append(parts, strings.TrimSpace(f.Inline))
			continue
		}
		if f.Path != "" && strings.TrimSpace(profileDir) != "" {
			raw, err := os.ReadFile(filepath.Join(profileDir, f.Path))
			if err != nil {
				return "", fmt.Errorf("read fragment %s: %w", f.Path, err)
			}
			parts = append(parts, strings.TrimSpace(string(raw)))
		}
	}
	return strings.Join(parts, "\n\n"), nil
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
