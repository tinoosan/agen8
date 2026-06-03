package membertype

import (
	"strings"
)

// WorkerType implements MemberType for specialist workers.
type WorkerType struct{}

func (w *WorkerType) Name() MemberTypeName { return TypeWorker }

func (w *WorkerType) SystemTools(_ ToolContext) []string {
	tools := append([]string{}, SystemAlwaysTools...)
	// Workers only get base system tools.
	return tools
}

func (w *WorkerType) Authorize(ctx ToolContext, requestedTools []string) AuthorizationResult {
	if strings.TrimSpace(ctx.SpaceID) == "" {
		return AuthorizationResult{Allowed: append([]string(nil), requestedTools...)}
	}

	// Workers: use defaults if no allowedTools specified.
	if len(requestedTools) == 0 {
		sanitized := make([]string, 0, len(DefaultWorkerAllowedTools))
		for _, name := range DefaultWorkerAllowedTools {
			trimmed := strings.TrimSpace(name)
			if trimmed == "" {
				continue
			}
			sanitized = append(sanitized, trimmed)
		}
		return AuthorizationResult{Allowed: sanitized}
	}

	sysTools := w.SystemTools(ctx)
	userTools, stripped := StripSystemTools(sysTools, requestedTools)

	// Filter out coordinator-only system tools from what remains.
	coordOnly := map[string]struct{}{}
	for _, name := range CoordinatorBaseTools {
		coordOnly[strings.ToLower(name)] = struct{}{}
	}
	for _, name := range CoordinatorWithWorkersTools {
		coordOnly[strings.ToLower(name)] = struct{}{}
	}

	sanitized := make([]string, 0, len(userTools))
	for _, name := range userTools {
		if _, ok := coordOnly[strings.ToLower(name)]; ok {
			stripped = append(stripped, name)
			continue
		}
		sanitized = append(sanitized, name)
	}
	return AuthorizationResult{Allowed: sanitized, Removed: stripped}
}

func (w *WorkerType) PromptRules(ctx PromptContext) string {
	return renderPromptRules(TypeWorker, ctx)
}

func (w *WorkerType) ToolManifest(_ ToolContext) []ToolEntry {
	return []ToolEntry{
		{Name: "task", Required: true},
		{Name: "plan", Required: true},
	}
}

func (w *WorkerType) StripPlanning() bool { return true }

func (w *WorkerType) CanClaimSpaceMessages() bool { return false }

func (w *WorkerType) CanClaimReviewerMessages(_ ToolContext) bool { return false }

func (w *WorkerType) ShowAllRoleDescriptions() bool { return false }
func (w *WorkerType) ShowFullProjectTopology() bool { return false }
