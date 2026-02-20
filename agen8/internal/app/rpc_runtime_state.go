package app

import (
	"context"
	"strings"

	"github.com/tinoosan/agen8/pkg/protocol"
)

func registerRuntimeHandlers(s *RPCServer, reg methodRegistry) error {
	return registerHandlers(
		func() error {
			return addBoundHandler[protocol.RuntimeGetRunStateParams, protocol.RuntimeGetRunStateResult](reg, protocol.MethodRuntimeGetRunState, false, s.runtimeGetRunState)
		},
		func() error {
			return addBoundHandler[protocol.RuntimeGetSessionStateParams, protocol.RuntimeGetSessionStateResult](reg, protocol.MethodRuntimeGetSessionState, false, s.runtimeGetSessionState)
		},
	)
}

func (s *RPCServer) runtimeGetRunState(ctx context.Context, p protocol.RuntimeGetRunStateParams) (protocol.RuntimeGetRunStateResult, error) {
	sessionID := strings.TrimSpace(p.SessionID)
	runID := strings.TrimSpace(p.RunID)
	if sessionID == "" || runID == "" {
		return protocol.RuntimeGetRunStateResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "sessionId and runId are required"}
	}
	if s.runtimeState != nil {
		state, err := s.runtimeState.GetRunState(ctx, sessionID, runID)
		if err == nil {
			return protocol.RuntimeGetRunStateResult{State: state}, nil
		}
	}
	run, err := s.session.LoadRun(ctx, runID)
	if err != nil {
		return protocol.RuntimeGetRunStateResult{}, err
	}
	return protocol.RuntimeGetRunStateResult{
		State: protocol.RuntimeRunState{
			SessionID:       sessionID,
			RunID:           runID,
			PersistedStatus: strings.TrimSpace(run.Status),
			EffectiveStatus: strings.TrimSpace(run.Status),
		},
	}, nil
}

func (s *RPCServer) runtimeGetSessionState(ctx context.Context, p protocol.RuntimeGetSessionStateParams) (protocol.RuntimeGetSessionStateResult, error) {
	sessionID := strings.TrimSpace(p.SessionID)
	if sessionID == "" {
		return protocol.RuntimeGetSessionStateResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "sessionId is required"}
	}
	if s.runtimeState != nil {
		runs, err := s.runtimeState.GetSessionState(ctx, sessionID)
		if err == nil {
			return protocol.RuntimeGetSessionStateResult{SessionID: sessionID, Runs: runs}, nil
		}
	}
	sess, err := s.session.LoadSession(ctx, sessionID)
	if err != nil {
		return protocol.RuntimeGetSessionStateResult{}, err
	}
	out := make([]protocol.RuntimeRunState, 0, len(sess.Runs))
	for _, rid := range sess.Runs {
		rid = strings.TrimSpace(rid)
		if rid == "" {
			continue
		}
		run, err := s.session.LoadRun(ctx, rid)
		if err != nil {
			continue
		}
		status := strings.TrimSpace(run.Status)
		out = append(out, protocol.RuntimeRunState{
			SessionID:       sessionID,
			RunID:           rid,
			PersistedStatus: status,
			EffectiveStatus: status,
		})
	}
	return protocol.RuntimeGetSessionStateResult{SessionID: sessionID, Runs: out}, nil
}
