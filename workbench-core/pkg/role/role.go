package role

import (
	"fmt"
	"strings"
	"time"
)

// Role represents a contract loaded from roles/<name>/ROLE.md.
// Only the front matter is authoritative; the markdown body is guidance only.
type Role struct {
	ID          string
	Description string
	SkillBias   []string
	Obligations []Obligation
	TaskPolicy  TaskPolicy
	Guidance    string // non-authoritative markdown body
}

// Obligation declares a requirement the agent must keep satisfied.
// Validity is either a duration (e.g. "30m") or a symbolic string.
type Obligation struct {
	ID               string        `yaml:"id"`
	ValidityRaw      string        `yaml:"validity"`
	ValidityDuration time.Duration `yaml:"-"`
	ValiditySymbolic string        `yaml:"-"`
	Evidence         string        `yaml:"evidence"`
}

// TaskPolicy bounds autonomous task creation.
type TaskPolicy struct {
	CreateTasksOnlyIf []string `yaml:"create_tasks_only_if"`
	MaxTasksPerCycle  int      `yaml:"max_tasks_per_cycle"`
}

// Normalize trims fields, parses validity, and applies defaults.
func (r Role) Normalize() (Role, error) {
	r.ID = strings.TrimSpace(r.ID)
	r.Description = strings.TrimSpace(r.Description)
	r.Guidance = strings.TrimSpace(r.Guidance)

	// De-duplicate skill bias entries, keep order.
	uniq := make([]string, 0, len(r.SkillBias))
	seen := map[string]struct{}{}
	for _, s := range r.SkillBias {
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
	r.SkillBias = uniq

	for i, ob := range r.Obligations {
		ob.ID = strings.TrimSpace(ob.ID)
		ob.Evidence = strings.TrimSpace(ob.Evidence)
		ob.ValidityRaw = strings.TrimSpace(ob.ValidityRaw)
		ob.ValidityDuration = 0
		ob.ValiditySymbolic = ""
		if ob.ValidityRaw != "" {
			if d, err := time.ParseDuration(ob.ValidityRaw); err == nil {
				ob.ValidityDuration = d
			} else {
				ob.ValiditySymbolic = ob.ValidityRaw
			}
		}
		r.Obligations[i] = ob
	}

	// Sanitize task policy.
	normalizedRules := make([]string, 0, len(r.TaskPolicy.CreateTasksOnlyIf))
	allowed := map[string]struct{}{"obligation_unsatisfied": {}, "obligation_expiring": {}}
	seenRules := map[string]struct{}{}
	for _, rule := range r.TaskPolicy.CreateTasksOnlyIf {
		rule = strings.ToLower(strings.TrimSpace(rule))
		if _, ok := allowed[rule]; !ok || rule == "" {
			continue
		}
		if _, dup := seenRules[rule]; dup {
			continue
		}
		seenRules[rule] = struct{}{}
		normalizedRules = append(normalizedRules, rule)
	}
	r.TaskPolicy.CreateTasksOnlyIf = normalizedRules
	if r.TaskPolicy.MaxTasksPerCycle <= 0 {
		r.TaskPolicy.MaxTasksPerCycle = 1
	}

	if err := r.Validate(); err != nil {
		return Role{}, err
	}
	return r, nil
}

// Validate enforces the required schema.
func (r Role) Validate() error {
	if strings.TrimSpace(r.ID) == "" {
		return fmt.Errorf("role id is required")
	}
	if strings.TrimSpace(r.Description) == "" {
		return fmt.Errorf("role %s: description is required", r.ID)
	}
	if len(r.Obligations) == 0 {
		return fmt.Errorf("role %s: at least one obligation is required", r.ID)
	}
	for _, ob := range r.Obligations {
		if strings.TrimSpace(ob.ID) == "" {
			return fmt.Errorf("role %s: obligation id is required", r.ID)
		}
		if strings.TrimSpace(ob.ValidityRaw) == "" {
			return fmt.Errorf("role %s: obligation %s validity is required", r.ID, ob.ID)
		}
		if strings.TrimSpace(ob.Evidence) == "" {
			return fmt.Errorf("role %s: obligation %s evidence is required", r.ID, ob.ID)
		}
	}
	if len(r.TaskPolicy.CreateTasksOnlyIf) == 0 {
		return fmt.Errorf("role %s: task_policy.create_tasks_only_if is required", r.ID)
	}
	allowed := map[string]struct{}{"obligation_unsatisfied": {}, "obligation_expiring": {}}
	for _, rule := range r.TaskPolicy.CreateTasksOnlyIf {
		if _, ok := allowed[strings.ToLower(strings.TrimSpace(rule))]; !ok {
			return fmt.Errorf("role %s: unsupported create_tasks_only_if rule %q", r.ID, rule)
		}
	}
	if r.TaskPolicy.MaxTasksPerCycle <= 0 {
		return fmt.Errorf("role %s: task_policy.max_tasks_per_cycle must be > 0", r.ID)
	}
	return nil
}
