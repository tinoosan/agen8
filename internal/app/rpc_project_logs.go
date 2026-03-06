package app

import (
	"context"
	"strings"

	"github.com/tinoosan/agen8/pkg/protocol"
	eventsvc "github.com/tinoosan/agen8/pkg/services/events"
)

func registerProjectHandlers(s *RPCServer, reg methodRegistry) error {
	return registerHandlers(
		func() error {
			return addBoundHandler[protocol.ProjectGetContextParams, protocol.ProjectGetContextResult](reg, protocol.MethodProjectGetContext, false, s.projectGetContext)
		},
		func() error {
			return addBoundHandler[protocol.ProjectSetActiveSessionParams, protocol.ProjectSetActiveSessionResult](reg, protocol.MethodProjectSetActive, false, s.projectSetActiveSession)
		},
		func() error {
			return addBoundHandler[protocol.ProjectListTeamsParams, protocol.ProjectListTeamsResult](reg, protocol.MethodProjectListTeams, false, s.projectListTeams)
		},
		func() error {
			return addBoundHandler[protocol.ProjectGetTeamParams, protocol.ProjectGetTeamResult](reg, protocol.MethodProjectGetTeam, false, s.projectGetTeam)
		},
		func() error {
			return addBoundHandler[protocol.LogsQueryParams, protocol.LogsQueryResult](reg, protocol.MethodLogsQuery, false, s.logsQuery)
		},
		func() error {
			return addBoundHandler[protocol.ActivityStreamParams, protocol.ActivityStreamResult](reg, protocol.MethodActivityStream, false, s.activityStream)
		},
	)
}

func toProtocolProjectContext(ctx ProjectContext) protocol.ProjectContext {
	return protocol.ProjectContext{
		Cwd:        strings.TrimSpace(ctx.Cwd),
		RootDir:    strings.TrimSpace(ctx.RootDir),
		ProjectDir: strings.TrimSpace(ctx.ProjectDir),
		ConfigPath: strings.TrimSpace(ctx.ConfigPath),
		StatePath:  strings.TrimSpace(ctx.StatePath),
		Exists:     ctx.Exists,
		Config: protocol.ProjectConfig{
			ProjectID:          strings.TrimSpace(ctx.Config.ProjectID),
			DefaultProfile:     strings.TrimSpace(ctx.Config.DefaultProfile),
			DefaultMode:        strings.TrimSpace(ctx.Config.DefaultMode),
			DefaultTeamProfile: strings.TrimSpace(ctx.Config.DefaultTeamProfile),
			RPCEndpoint:        strings.TrimSpace(ctx.Config.RPCEndpoint),
			DataDirOverride:    strings.TrimSpace(ctx.Config.DataDirOverride),
			ObsidianVaultPath:  strings.TrimSpace(ctx.Config.ObsidianVaultPath),
			ObsidianEnabled:    ctx.Config.ObsidianEnabled,
			CreatedAt:          strings.TrimSpace(ctx.Config.CreatedAt),
			Version:            ctx.Config.Version,
		},
		State: protocol.ProjectState{
			ActiveSessionID: strings.TrimSpace(ctx.State.ActiveSessionID),
			ActiveTeamID:    strings.TrimSpace(ctx.State.ActiveTeamID),
			ActiveRunID:     strings.TrimSpace(ctx.State.ActiveRunID),
			ActiveThreadID:  strings.TrimSpace(ctx.State.ActiveThreadID),
			LastAttachedAt:  strings.TrimSpace(ctx.State.LastAttachedAt),
			LastCommand:     strings.TrimSpace(ctx.State.LastCommand),
		},
	}
}

func (s *RPCServer) projectGetContext(_ context.Context, p protocol.ProjectGetContextParams) (protocol.ProjectGetContextResult, error) {
	ctx, err := LoadProjectContext(strings.TrimSpace(p.Cwd))
	if err != nil {
		return protocol.ProjectGetContextResult{}, err
	}
	return protocol.ProjectGetContextResult{Context: toProtocolProjectContext(ctx)}, nil
}

func (s *RPCServer) projectListTeams(ctx context.Context, p protocol.ProjectListTeamsParams) (protocol.ProjectListTeamsResult, error) {
	projectRoot, err := resolveProjectRootForRPC(strings.TrimSpace(p.Cwd), strings.TrimSpace(p.ProjectRoot))
	if err != nil {
		return protocol.ProjectListTeamsResult{}, err
	}
	if s.projectTeamSvc == nil {
		return protocol.ProjectListTeamsResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "project team service is not configured"}
	}
	teams, err := s.projectTeamSvc.ListTeams(ctx, projectRoot)
	if err != nil {
		return protocol.ProjectListTeamsResult{}, err
	}
	out := make([]protocol.ProjectTeamSummary, 0, len(teams))
	for _, summary := range teams {
		out = append(out, protocol.ProjectTeamSummary{
			ProjectID:        strings.TrimSpace(summary.ProjectID),
			ProjectRoot:      strings.TrimSpace(summary.ProjectRoot),
			TeamID:           strings.TrimSpace(summary.TeamID),
			ProfileID:        strings.TrimSpace(summary.ProfileID),
			PrimarySessionID: strings.TrimSpace(summary.PrimarySessionID),
			CoordinatorRunID: strings.TrimSpace(summary.CoordinatorRunID),
			Status:           strings.TrimSpace(summary.Status),
			CreatedAt:        strings.TrimSpace(summary.CreatedAt),
			UpdatedAt:        strings.TrimSpace(summary.UpdatedAt),
			ManifestPresent:  summary.ManifestPresent,
		})
	}
	return protocol.ProjectListTeamsResult{Teams: out}, nil
}

func (s *RPCServer) projectGetTeam(ctx context.Context, p protocol.ProjectGetTeamParams) (protocol.ProjectGetTeamResult, error) {
	projectRoot, err := resolveProjectRootForRPC(strings.TrimSpace(p.Cwd), strings.TrimSpace(p.ProjectRoot))
	if err != nil {
		return protocol.ProjectGetTeamResult{}, err
	}
	if s.projectTeamSvc == nil {
		return protocol.ProjectGetTeamResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "project team service is not configured"}
	}
	summary, err := s.projectTeamSvc.GetTeam(ctx, projectRoot, strings.TrimSpace(p.TeamID))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return protocol.ProjectGetTeamResult{}, &protocol.ProtocolError{Code: protocol.CodeItemNotFound, Message: err.Error()}
		}
		return protocol.ProjectGetTeamResult{}, err
	}
	return protocol.ProjectGetTeamResult{Team: protocol.ProjectTeamSummary{
		ProjectID:        strings.TrimSpace(summary.ProjectID),
		ProjectRoot:      strings.TrimSpace(summary.ProjectRoot),
		TeamID:           strings.TrimSpace(summary.TeamID),
		ProfileID:        strings.TrimSpace(summary.ProfileID),
		PrimarySessionID: strings.TrimSpace(summary.PrimarySessionID),
		CoordinatorRunID: strings.TrimSpace(summary.CoordinatorRunID),
		Status:           strings.TrimSpace(summary.Status),
		CreatedAt:        strings.TrimSpace(summary.CreatedAt),
		UpdatedAt:        strings.TrimSpace(summary.UpdatedAt),
		ManifestPresent:  summary.ManifestPresent,
	}}, nil
}

func (s *RPCServer) projectSetActiveSession(_ context.Context, p protocol.ProjectSetActiveSessionParams) (protocol.ProjectSetActiveSessionResult, error) {
	ctx, err := SetActiveSession(strings.TrimSpace(p.Cwd), ProjectState{
		ActiveSessionID: strings.TrimSpace(p.ActiveSessionID),
		ActiveTeamID:    strings.TrimSpace(p.ActiveTeamID),
		ActiveRunID:     strings.TrimSpace(p.ActiveRunID),
		ActiveThreadID:  strings.TrimSpace(p.ActiveThreadID),
		LastCommand:     strings.TrimSpace(p.LastCommand),
	})
	if err != nil {
		return protocol.ProjectSetActiveSessionResult{}, err
	}
	return protocol.ProjectSetActiveSessionResult{Context: toProtocolProjectContext(ctx)}, nil
}

func resolveProjectRootForRPC(cwd, explicitRoot string) (string, error) {
	if root := strings.TrimSpace(explicitRoot); root != "" {
		return root, nil
	}
	ctx, err := LoadProjectContext(strings.TrimSpace(cwd))
	if err != nil {
		return "", err
	}
	if !ctx.Exists || strings.TrimSpace(ctx.RootDir) == "" {
		return "", &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "project context not found"}
	}
	return strings.TrimSpace(ctx.RootDir), nil
}

func (s *RPCServer) resolveLogsRunID(ctx context.Context, runID string, sessionID string) (string, error) {
	runID = strings.TrimSpace(runID)
	if runID != "" {
		return runID, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "runId or sessionId is required"}
	}
	sess, err := s.session.LoadSession(ctx, sessionID)
	if err != nil {
		return "", err
	}
	if r := strings.TrimSpace(sess.CurrentRunID); r != "" {
		return r, nil
	}
	for _, candidate := range sess.Runs {
		candidate = strings.TrimSpace(candidate)
		if candidate != "" {
			return candidate, nil
		}
	}
	return "", &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "session has no runs"}
}

func (s *RPCServer) logsQuery(ctx context.Context, p protocol.LogsQueryParams) (protocol.LogsQueryResult, error) {
	if s.eventsService == nil {
		return protocol.LogsQueryResult{}, errEventsServiceNotConfigured
	}
	runID, err := s.resolveLogsRunID(ctx, p.RunID, p.SessionID)
	if err != nil {
		return protocol.LogsQueryResult{}, err
	}
	filter := eventsvc.Filter{
		RunID:    runID,
		Limit:    clampLimit(p.Limit, 200, 5000),
		Offset:   max(0, p.Offset),
		AfterSeq: p.AfterSeq,
		Types:    p.Types,
		SortDesc: p.SortDesc,
	}
	events, next, err := s.eventsService.ListPaginated(ctx, filter)
	if err != nil {
		return protocol.LogsQueryResult{}, err
	}
	return protocol.LogsQueryResult{Events: events, Next: next}, nil
}

func (s *RPCServer) activityStream(ctx context.Context, p protocol.ActivityStreamParams) (protocol.ActivityStreamResult, error) {
	if s.eventsService == nil {
		return protocol.ActivityStreamResult{}, errEventsServiceNotConfigured
	}
	runID, err := s.resolveLogsRunID(ctx, p.RunID, "")
	if err != nil {
		return protocol.ActivityStreamResult{}, err
	}
	filter := eventsvc.Filter{
		RunID:    runID,
		Limit:    clampLimit(p.Limit, 200, 5000),
		AfterSeq: p.AfterSeq,
		Types:    p.Types,
		SortDesc: false,
	}
	events, next, err := s.eventsService.ListPaginated(ctx, filter)
	if err != nil {
		return protocol.ActivityStreamResult{}, err
	}
	latest, _ := s.eventsService.LatestSeq(ctx, runID)
	return protocol.ActivityStreamResult{
		Events:    events,
		Next:      next,
		LatestSeq: latest,
	}, nil
}
