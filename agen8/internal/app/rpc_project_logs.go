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

func (s *RPCServer) projectSetActiveSession(_ context.Context, p protocol.ProjectSetActiveSessionParams) (protocol.ProjectSetActiveSessionResult, error) {
	ctx, err := SetActiveSession(strings.TrimSpace(p.Cwd), ProjectState{
		ActiveSessionID: strings.TrimSpace(p.ActiveSessionID),
		ActiveTeamID:    strings.TrimSpace(p.ActiveTeamID),
		ActiveRunID:     strings.TrimSpace(p.ActiveRunID),
		LastCommand:     strings.TrimSpace(p.LastCommand),
	})
	if err != nil {
		return protocol.ProjectSetActiveSessionResult{}, err
	}
	return protocol.ProjectSetActiveSessionResult{Context: toProtocolProjectContext(ctx)}, nil
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
