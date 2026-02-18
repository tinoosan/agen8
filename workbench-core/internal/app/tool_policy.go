package app

import (
	"fmt"
	"strings"

	"github.com/tinoosan/workbench-core/pkg/agent"
)

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
