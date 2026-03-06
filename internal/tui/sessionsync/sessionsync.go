package sessionsync

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8/internal/app"
	"github.com/tinoosan/agen8/pkg/protocol"
)

// ResolveActiveSessionID derives the control session for the active project team.
func ResolveActiveSessionID(projectRoot, endpoint string) (string, error) {
	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot == "" {
		return "", fmt.Errorf("project root is required")
	}
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		endpoint = protocol.DefaultRPCEndpoint
	}
	ctx, err := app.LoadProjectContext(projectRoot)
	if err != nil {
		return "", err
	}
	if !ctx.Exists {
		return "", fmt.Errorf("project context not found")
	}
	teamID := strings.TrimSpace(ctx.State.ActiveTeamID)
	if teamID == "" {
		sessionID := strings.TrimSpace(ctx.State.ActiveSessionID)
		if sessionID == "" {
			return "", fmt.Errorf("active team is not set")
		}
		return sessionID, nil
	}
	var out protocol.ProjectGetTeamResult
	cli := protocol.TCPClient{Endpoint: endpoint, Timeout: 5 * time.Second}
	if err := cli.Call(context.Background(), protocol.MethodProjectGetTeam, protocol.ProjectGetTeamParams{
		ProjectRoot: projectRoot,
		TeamID:      teamID,
	}, &out); err != nil {
		return "", fmt.Errorf("rpc %s: %w", protocol.MethodProjectGetTeam, err)
	}
	sessionID := strings.TrimSpace(out.Team.PrimarySessionID)
	if sessionID == "" {
		return "", fmt.Errorf("active team %s has no control session", teamID)
	}
	return sessionID, nil
}
