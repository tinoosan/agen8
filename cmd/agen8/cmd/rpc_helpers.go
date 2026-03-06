package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8/pkg/protocol"
)

const detachedThreadID = protocol.ThreadID("detached-control")

func resolvedRPCEndpoint() string {
	endpoint := strings.TrimSpace(rpcEndpoint)
	if endpoint == "" {
		endpoint = protocol.DefaultRPCEndpoint
	}
	return endpoint
}

func rpcCall(ctx context.Context, method string, params any, out any) error {
	base := ctx
	if base == nil {
		base = context.Background()
	}
	c, cancel := context.WithTimeout(base, 5*time.Second)
	defer cancel()
	cli := protocol.TCPClient{
		Endpoint: resolvedRPCEndpoint(),
		Timeout:  5 * time.Second,
	}
	if err := cli.Call(c, method, params, out); err != nil {
		return fmt.Errorf("rpc %s: %w", method, err)
	}
	return nil
}

func rpcPing(ctx context.Context) error {
	var out protocol.SessionListResult
	return rpcCall(ctx, protocol.MethodSessionList, protocol.SessionListParams{
		ThreadID: detachedThreadID,
		Limit:    1,
		Offset:   0,
	}, &out)
}

func rpcListSessions(ctx context.Context, limit int) ([]protocol.SessionListItem, error) {
	if limit <= 0 {
		limit = 100
	}
	var out protocol.SessionListResult
	if err := rpcCall(ctx, protocol.MethodSessionList, protocol.SessionListParams{
		ThreadID: detachedThreadID,
		Limit:    limit,
		Offset:   0,
	}, &out); err != nil {
		return nil, err
	}
	return out.Sessions, nil
}

func rpcFindSession(ctx context.Context, sessionID string) (*protocol.SessionListItem, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("session id is required")
	}
	sessions, err := rpcListSessions(ctx, 500)
	if err != nil {
		return nil, err
	}
	for i := range sessions {
		if strings.TrimSpace(sessions[i].SessionID) == sessionID {
			return &sessions[i], nil
		}
	}
	return nil, fmt.Errorf("session %q not found", sessionID)
}

func rpcResolveCoordinatorRun(ctx context.Context, sessionID string) (runID string, teamID string, err error) {
	item, err := rpcFindSession(ctx, sessionID)
	if err != nil {
		return "", "", err
	}
	teamID = strings.TrimSpace(item.TeamID)
	if teamID != "" {
		var manifest protocol.TeamGetManifestResult
		if err := rpcCall(ctx, protocol.MethodTeamGetManifest, protocol.TeamGetManifestParams{
			ThreadID: protocol.ThreadID(sessionID),
			TeamID:   teamID,
		}, &manifest); err == nil {
			if coordinatorRun := strings.TrimSpace(manifest.CoordinatorRun); coordinatorRun != "" {
				return coordinatorRun, teamID, nil
			}
		}
	}

	if r := strings.TrimSpace(item.CurrentRunID); r != "" {
		return r, teamID, nil
	}

	var agents protocol.AgentListResult
	if err := rpcCall(ctx, protocol.MethodAgentList, protocol.AgentListParams{
		ThreadID:  protocol.ThreadID(sessionID),
		SessionID: sessionID,
	}, &agents); err != nil {
		return "", "", err
	}
	if len(agents.Agents) > 0 {
		return strings.TrimSpace(agents.Agents[0].RunID), teamID, nil
	}
	return "", "", fmt.Errorf("session %q has no active runs", sessionID)
}

func rpcResolveThread(ctx context.Context, sessionID string, runID string) (protocol.SessionResolveThreadResult, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return protocol.SessionResolveThreadResult{}, fmt.Errorf("session id is required")
	}
	var out protocol.SessionResolveThreadResult
	if err := rpcCall(ctx, protocol.MethodSessionResolveThread, protocol.SessionResolveThreadParams{
		SessionID: sessionID,
		RunID:     strings.TrimSpace(runID),
	}, &out); err != nil {
		return protocol.SessionResolveThreadResult{}, err
	}
	return out, nil
}

func rpcListProjectTeams(ctx context.Context, projectRoot string) ([]protocol.ProjectTeamSummary, error) {
	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot == "" {
		return nil, fmt.Errorf("project root is required")
	}
	var out protocol.ProjectListTeamsResult
	if err := rpcCall(ctx, protocol.MethodProjectListTeams, protocol.ProjectListTeamsParams{
		ProjectRoot: projectRoot,
	}, &out); err != nil {
		return nil, err
	}
	return out.Teams, nil
}

func rpcGetProjectTeam(ctx context.Context, projectRoot, teamID string) (protocol.ProjectTeamSummary, error) {
	projectRoot = strings.TrimSpace(projectRoot)
	teamID = strings.TrimSpace(teamID)
	if projectRoot == "" || teamID == "" {
		return protocol.ProjectTeamSummary{}, fmt.Errorf("project root and team id are required")
	}
	var out protocol.ProjectGetTeamResult
	if err := rpcCall(ctx, protocol.MethodProjectGetTeam, protocol.ProjectGetTeamParams{
		ProjectRoot: projectRoot,
		TeamID:      teamID,
	}, &out); err != nil {
		return protocol.ProjectTeamSummary{}, err
	}
	return out.Team, nil
}

func rpcDeleteTeam(ctx context.Context, teamID, projectRoot string) (protocol.TeamDeleteResult, error) {
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		return protocol.TeamDeleteResult{}, fmt.Errorf("team id is required")
	}
	var out protocol.TeamDeleteResult
	if err := rpcCall(ctx, protocol.MethodTeamDelete, protocol.TeamDeleteParams{
		ThreadID:    detachedThreadID,
		TeamID:      teamID,
		ProjectRoot: strings.TrimSpace(projectRoot),
	}, &out); err != nil {
		return protocol.TeamDeleteResult{}, err
	}
	return out, nil
}

func rpcDeleteProjectTeams(ctx context.Context, projectRoot string) (protocol.ProjectDeleteTeamsResult, error) {
	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot == "" {
		return protocol.ProjectDeleteTeamsResult{}, fmt.Errorf("project root is required")
	}
	var out protocol.ProjectDeleteTeamsResult
	if err := rpcCall(ctx, protocol.MethodProjectDeleteTeams, protocol.ProjectDeleteTeamsParams{
		ProjectRoot: projectRoot,
	}, &out); err != nil {
		return protocol.ProjectDeleteTeamsResult{}, err
	}
	return out, nil
}
