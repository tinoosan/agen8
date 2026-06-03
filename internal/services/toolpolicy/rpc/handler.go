package rpc

import (
	"context"
	"fmt"

	"github.com/tinoosan/agen8-mcp-server/internal/services/toolpolicy/app"
	"github.com/tinoosan/agen8-mcp-server/pkg/membertype"
	"github.com/tinoosan/agen8-mcp-server/pkg/protocol"
)

// Handler adapts the tool policy application service to RPC protocol types.
type Handler struct {
	svc *app.Service
}

// NewHandler creates an RPC handler wrapping the tool policy application service.
func NewHandler(svc *app.Service) *Handler {
	return &Handler{svc: svc}
}

// Authorize handles toolpolicy.authorize RPC calls.
func (h *Handler) Authorize(ctx context.Context, p protocol.ToolpolicyAuthorizeParams) (protocol.ToolpolicyAuthorizeResult, error) {
	at, err := resolveMemberType(p.MemberType)
	if err != nil {
		return protocol.ToolpolicyAuthorizeResult{}, err
	}
	result, err := h.svc.Authorize(ctx, app.AuthorizeParams{
		SpaceID:      p.SpaceID,
		MemberType:   at,
		MemberCount:  p.MemberCount,
		HasReviewer:  p.HasReviewer,
		AllowedTools: p.AllowedTools,
	})
	if err != nil {
		return protocol.ToolpolicyAuthorizeResult{}, err
	}
	return protocol.ToolpolicyAuthorizeResult{
		Allowed: result.Allowed,
		Removed: result.Removed,
	}, nil
}

// SystemTools handles toolpolicy.systemTools RPC calls.
func (h *Handler) SystemTools(ctx context.Context, p protocol.ToolpolicySystemToolsParams) (protocol.ToolpolicySystemToolsResult, error) {
	at, err := resolveMemberType(p.MemberType)
	if err != nil {
		return protocol.ToolpolicySystemToolsResult{}, err
	}
	tools, err := h.svc.SystemTools(ctx, app.SystemToolsParams{
		MemberType:  at,
		MemberCount: p.MemberCount,
		HasReviewer: p.HasReviewer,
	})
	if err != nil {
		return protocol.ToolpolicySystemToolsResult{}, err
	}
	return protocol.ToolpolicySystemToolsResult{Tools: tools}, nil
}

// Defaults handles toolpolicy.defaults RPC calls.
func (h *Handler) Defaults(ctx context.Context, _ protocol.ToolpolicyDefaultsParams) (protocol.ToolpolicyDefaultsResult, error) {
	result := h.svc.Defaults(ctx)
	return protocol.ToolpolicyDefaultsResult{
		WorkerTools:            result.WorkerTools,
		CoordinatorBase:        result.CoordinatorBase,
		CoordinatorWithWorkers: result.CoordinatorWithWorkers,
	}, nil
}

// resolveMemberType converts an RPC member type string to an MemberType instance.
func resolveMemberType(name string) (membertype.MemberType, error) {
	if name == "" {
		return nil, fmt.Errorf("memberType is required")
	}
	return membertype.Lookup(membertype.MemberTypeName(name))
}
