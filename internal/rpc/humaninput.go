package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	humaninputapp "github.com/tinoosan/agen8-mcp-server/internal/services/humaninput/app"
	humaninputdomain "github.com/tinoosan/agen8-mcp-server/internal/services/humaninput/domain"
	"github.com/tinoosan/agen8-mcp-server/pkg/protocol"
)

func RegisterHumanInput(reg *Registry, svc *humaninputapp.Service, wake *humaninputapp.MemoryWakeRegistry) error {
	if svc == nil {
		return fmt.Errorf("human input service is required")
	}
	handler := humanInputHandler{svc: svc, wake: wake}
	return RegisterHandlers(
		func() error {
			return AddBoundHandler(reg, protocol.MethodChannelHumanInputPending, false, withHumanInputIdentity(handler.Pending))
		},
		func() error {
			return AddBoundHandler(reg, protocol.MethodChannelHumanInputSubmit, false, withHumanInputIdentity(handler.Submit))
		},
		func() error {
			return AddBoundHandler(reg, protocol.MethodChannelHumanInputCancel, false, withHumanInputIdentity(handler.Cancel))
		},
	)
}

type humanInputHandler struct {
	svc  *humaninputapp.Service
	wake *humaninputapp.MemoryWakeRegistry
}

func (h humanInputHandler) Pending(ctx context.Context, params protocol.ChannelHumanInputPendingParams) (protocol.ChannelHumanInputPendingResult, error) {
	channelID := strings.TrimSpace(params.ChannelID)
	if channelID == "" {
		return protocol.ChannelHumanInputPendingResult{}, InvalidParams("channelId is required")
	}
	pending, err := h.svc.ListPending(ctx, humaninputdomain.Filter{ChannelID: channelID, Limit: 1})
	if err != nil {
		return protocol.ChannelHumanInputPendingResult{}, err
	}
	if len(pending) == 0 {
		return protocol.ChannelHumanInputPendingResult{}, nil
	}
	projection := projectPending(pending[0])
	return protocol.ChannelHumanInputPendingResult{Pending: &projection}, nil
}

func (h humanInputHandler) Submit(ctx context.Context, params protocol.ChannelHumanInputSubmitParams) (protocol.ChannelHumanInputSubmitResult, error) {
	req, err := h.findPendingByToolCall(ctx, params.SpaceID, params.MemberID, params.ToolCallID)
	if err != nil {
		return protocol.ChannelHumanInputSubmitResult{}, err
	}
	identity, err := RequireIdentity(ctx)
	if err != nil {
		return protocol.ChannelHumanInputSubmitResult{}, err
	}
	resolved, err := h.svc.Resolve(ctx, humaninputapp.ResolveCommand{
		RequestID:        req.ID,
		ExpectedVersion:  req.Version,
		Result:           append(json.RawMessage(nil), params.Result...),
		ResolverUserID:   identity.UserID,
		ResolverMemberID: identity.MemberID,
	})
	if err != nil {
		return protocol.ChannelHumanInputSubmitResult{}, err
	}
	if h.wake != nil {
		h.wake.Notify(resolved)
	}
	return protocol.ChannelHumanInputSubmitResult{OK: true}, nil
}

func (h humanInputHandler) Cancel(ctx context.Context, params protocol.ChannelHumanInputCancelParams) (protocol.ChannelHumanInputCancelResult, error) {
	req, err := h.findPendingByToolCall(ctx, params.SpaceID, params.MemberID, params.ToolCallID)
	if err != nil {
		return protocol.ChannelHumanInputCancelResult{}, err
	}
	identity, err := RequireIdentity(ctx)
	if err != nil {
		return protocol.ChannelHumanInputCancelResult{}, err
	}
	cancelled, err := h.svc.Cancel(ctx, req.ID, req.Version, identity.UserID, identity.MemberID)
	if err != nil {
		return protocol.ChannelHumanInputCancelResult{}, err
	}
	if h.wake != nil {
		h.wake.Notify(cancelled)
	}
	return protocol.ChannelHumanInputCancelResult{OK: true}, nil
}

func (h humanInputHandler) findPendingByToolCall(ctx context.Context, spaceID, memberID, toolCallID string) (humaninputdomain.Request, error) {
	spaceID = strings.TrimSpace(spaceID)
	memberID = strings.TrimSpace(memberID)
	toolCallID = strings.TrimSpace(toolCallID)
	if spaceID == "" {
		return humaninputdomain.Request{}, InvalidParams("spaceId is required")
	}
	if memberID == "" {
		return humaninputdomain.Request{}, InvalidParams("memberId is required")
	}
	if toolCallID == "" {
		return humaninputdomain.Request{}, InvalidParams("toolCallId is required")
	}
	pending, err := h.svc.ListPending(ctx, humaninputdomain.Filter{SpaceID: spaceID, MemberID: memberID, Limit: 100})
	if err != nil {
		return humaninputdomain.Request{}, err
	}
	for _, req := range pending {
		if string(req.ToolCallID) == toolCallID {
			return req, nil
		}
	}
	return humaninputdomain.Request{}, InvalidParams("pending human input request not found")
}

func projectPending(req humaninputdomain.Request) protocol.PendingHumanInput {
	return protocol.PendingHumanInput{
		SpaceID:     req.SpaceID,
		MemberID:    req.AskerMemberID,
		ChannelID:   req.ChannelID,
		ToolCallID:  string(req.ToolCallID),
		ToolName:    req.ToolName,
		Primitive:   string(req.Declaration.Kind),
		PayloadJSON: append(json.RawMessage(nil), req.Declaration.Payload...),
		ProjectID:   req.ProjectID,
		CreatedAt:   req.CreatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"),
	}
}

func withHumanInputIdentity[Params any, Result any](fn func(context.Context, Params) (Result, error)) func(context.Context, Params) (Result, error) {
	return func(ctx context.Context, params Params) (Result, error) {
		if _, err := RequireIdentity(ctx); err != nil {
			var zero Result
			return zero, err
		}
		return fn(ctx, params)
	}
}
