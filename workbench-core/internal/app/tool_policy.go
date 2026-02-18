package app

import (
	"fmt"
	"strings"

	"github.com/tinoosan/workbench-core/pkg/agent"
)

type teamToolRule struct {
	tool                string
	disallowForNonCoord bool
}

var defaultTeamToolRules = []teamToolRule{
	{tool: "task_create", disallowForNonCoord: true},
}

func sanitizeAllowedToolsForRole(allowed []string, teamID string, isCoordinator bool) (sanitized []string, removed []string) {
	teamID = strings.TrimSpace(teamID)
	if teamID == "" || isCoordinator {
		return append([]string(nil), allowed...), nil
	}
	restricted := map[string]struct{}{}
	for _, rule := range defaultTeamToolRules {
		tool := strings.TrimSpace(rule.tool)
		if tool == "" || !rule.disallowForNonCoord {
			continue
		}
		restricted[strings.ToLower(tool)] = struct{}{}
	}
	sanitized = make([]string, 0, len(allowed))
	for _, name := range allowed {
		trimmed := strings.TrimSpace(name)
		if _, ok := restricted[strings.ToLower(trimmed)]; ok {
			removed = append(removed, trimmed)
			continue
		}
		sanitized = append(sanitized, name)
	}
	return sanitized, removed
}

func applyAllowedTools(registry *agent.HostToolRegistry, allowed []string) error {
	if registry == nil || len(allowed) == 0 {
		return nil
	}
	allowedSet := map[string]struct{}{}
	for _, name := range allowed {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		allowedSet[trimmed] = struct{}{}
	}
	if len(allowedSet) == 0 {
		return nil
	}

	defs := registry.Definitions()
	if len(defs) == 0 {
		return nil
	}
	availableSet := map[string]struct{}{}
	for _, def := range defs {
		name := strings.TrimSpace(def.Function.Name)
		if name == "" {
			continue
		}
		availableSet[name] = struct{}{}
	}
	for name := range allowedSet {
		if _, ok := availableSet[name]; !ok {
			return fmt.Errorf("allowed_tools includes unknown tool %q", name)
		}
	}
	for name := range availableSet {
		if _, ok := allowedSet[name]; ok {
			continue
		}
		registry.Remove(name)
	}
	return nil
}

func resolveCodeExecOnly(profileDefault bool, roleOverride *bool) bool {
	if roleOverride != nil {
		return *roleOverride
	}
	return profileDefault
}

func resolveToolRegistries(base *agent.HostToolRegistry, allowed []string, codeExecOnly bool) (modelRegistry, bridgeRegistry *agent.HostToolRegistry, err error) {
	if base == nil {
		return nil, nil, fmt.Errorf("base tool registry is nil")
	}

	bridgeRegistry = base.Clone()
	if bridgeRegistry == nil {
		bridgeRegistry = agent.NewHostToolRegistry()
	}
	if err := applyAllowedTools(bridgeRegistry, allowed); err != nil {
		return nil, nil, err
	}
	if !codeExecOnly {
		return bridgeRegistry.Clone(), bridgeRegistry, nil
	}

	modelRegistry = agent.NewHostToolRegistry()
	codeExecTool, ok := base.Get("code_exec")
	if !ok {
		return nil, nil, fmt.Errorf("code_exec_only enabled but code_exec is not registered")
	}
	if err := modelRegistry.Register(codeExecTool); err != nil {
		return nil, nil, fmt.Errorf("register code_exec for model tool registry: %w", err)
	}
	return modelRegistry, bridgeRegistry, nil
}
