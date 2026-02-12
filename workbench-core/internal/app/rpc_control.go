package app

import (
	"context"
	"strings"

	"github.com/tinoosan/workbench-core/pkg/protocol"
)

func registerControlHandlers(s *RPCServer, reg methodRegistry) error {
	if err := addBoundHandler[protocol.ControlSetModelParams, protocol.ControlSetModelResult](reg, protocol.MethodControlSetModel, false, s.controlSetModelHandler); err != nil {
		return err
	}
	if err := addBoundHandler[protocol.ControlSetReasoningParams, protocol.ControlSetReasoningResult](reg, protocol.MethodControlSetReasoning, false, s.controlSetReasoningHandler); err != nil {
		return err
	}
	if err := addBoundHandler[protocol.ControlSetProfileParams, protocol.ControlSetProfileResult](reg, protocol.MethodControlSetProfile, false, s.controlSetProfileHandler); err != nil {
		return err
	}
	return nil
}

func (s *RPCServer) controlSetModelHandler(ctx context.Context, p protocol.ControlSetModelParams) (protocol.ControlSetModelResult, error) {
	threadID, err := s.resolveThreadID(p.ThreadID)
	if err != nil {
		return protocol.ControlSetModelResult{}, err
	}
	model := strings.TrimSpace(p.Model)
	if model == "" {
		return protocol.ControlSetModelResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "model is required"}
	}
	if s.controlSetModel == nil {
		return protocol.ControlSetModelResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "control.setModel is unavailable"}
	}
	appliedTo, err := s.controlSetModel(ctx, threadID, strings.TrimSpace(p.Target), model)
	if err != nil {
		return protocol.ControlSetModelResult{}, err
	}
	return protocol.ControlSetModelResult{
		Accepted:  true,
		AppliedTo: append([]string(nil), appliedTo...),
	}, nil
}

func normalizeReasoningEffort(v string) (string, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "":
		return "", nil
	case "none", "minimal", "low", "medium", "high", "xhigh":
		return v, nil
	default:
		return "", &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "effort must be one of none|minimal|low|medium|high|xhigh"}
	}
}

func normalizeReasoningSummary(v string) (string, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "none" {
		v = "off"
	}
	switch v {
	case "":
		return "", nil
	case "off", "auto", "concise", "detailed":
		return v, nil
	default:
		return "", &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "summary must be one of off|auto|concise|detailed"}
	}
}

func (s *RPCServer) controlSetReasoningHandler(ctx context.Context, p protocol.ControlSetReasoningParams) (protocol.ControlSetReasoningResult, error) {
	threadID, err := s.resolveThreadID(p.ThreadID)
	if err != nil {
		return protocol.ControlSetReasoningResult{}, err
	}
	effort, err := normalizeReasoningEffort(p.Effort)
	if err != nil {
		return protocol.ControlSetReasoningResult{}, err
	}
	summary, err := normalizeReasoningSummary(p.Summary)
	if err != nil {
		return protocol.ControlSetReasoningResult{}, err
	}
	if effort == "" && summary == "" {
		return protocol.ControlSetReasoningResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "effort or summary is required"}
	}
	if s.controlSetReasoning == nil {
		return protocol.ControlSetReasoningResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "control.setReasoning is unavailable"}
	}
	appliedTo, err := s.controlSetReasoning(ctx, threadID, strings.TrimSpace(p.Target), effort, summary)
	if err != nil {
		return protocol.ControlSetReasoningResult{}, err
	}
	return protocol.ControlSetReasoningResult{
		Accepted:  true,
		AppliedTo: append([]string(nil), appliedTo...),
		Effort:    effort,
		Summary:   summary,
	}, nil
}

func (s *RPCServer) controlSetProfileHandler(ctx context.Context, p protocol.ControlSetProfileParams) (protocol.ControlSetProfileResult, error) {
	threadID, err := s.resolveThreadID(p.ThreadID)
	if err != nil {
		return protocol.ControlSetProfileResult{}, err
	}
	profile := strings.TrimSpace(p.Profile)
	if profile == "" {
		return protocol.ControlSetProfileResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "profile is required"}
	}
	if s.controlSetProfile == nil {
		return protocol.ControlSetProfileResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "control.setProfile is unavailable"}
	}
	appliedTo, err := s.controlSetProfile(ctx, threadID, strings.TrimSpace(p.Target), profile)
	if err != nil {
		return protocol.ControlSetProfileResult{}, err
	}
	return protocol.ControlSetProfileResult{
		Accepted:                true,
		AppliedTo:               append([]string(nil), appliedTo...),
		PreservesSessionContext: true,
	}, nil
}
