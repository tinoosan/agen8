package sessionsync

import (
	"fmt"
	"strings"

	"github.com/tinoosan/agen8/internal/app"
)

// ResolveActiveSessionID returns the active session from <project>/.agen8/state.json.
func ResolveActiveSessionID(projectRoot string) (string, error) {
	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot == "" {
		return "", fmt.Errorf("project root is required")
	}
	ctx, err := app.LoadProjectContext(projectRoot)
	if err != nil {
		return "", err
	}
	if !ctx.Exists {
		return "", fmt.Errorf("project context not found")
	}
	sessionID := strings.TrimSpace(ctx.State.ActiveSessionID)
	if sessionID == "" {
		return "", fmt.Errorf("active session is not set")
	}
	return sessionID, nil
}
