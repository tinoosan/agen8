package rpc

import (
	"context"
	"errors"
	"strings"

	pinapp "github.com/tinoosan/agen8-mcp-server/internal/services/pin/app"
	pindomain "github.com/tinoosan/agen8-mcp-server/internal/services/pin/domain"
)

// Handler adapts the pin application service to RPC protocol types.
type Handler struct {
	svc *pinapp.Service
}

// NewHandler creates an RPC handler wrapping the pin application service.
func NewHandler(svc *pinapp.Service) *Handler {
	return &Handler{svc: svc}
}

// Add handles pin.add RPC calls.
func (h *Handler) Add(ctx context.Context, p PinAddParams) (PinAddResult, error) {
	if strings.TrimSpace(p.ProjectID) == "" {
		return PinAddResult{}, invalidParams("projectId is required")
	}
	if strings.TrimSpace(p.NodeRef) == "" {
		return PinAddResult{}, invalidParams("nodeRef is required")
	}
	pin, err := h.svc.Pin(ctx, p.ProjectID, p.NodeRef, p.NodeType)
	if err != nil {
		return PinAddResult{}, internalError("add pin: %v", err)
	}
	return PinAddResult{Pin: pinToView(pin)}, nil
}

// Remove handles pin.remove RPC calls. Removing a node that is not pinned is
// treated as success (Removed=false) so the caller can unpin idempotently.
func (h *Handler) Remove(ctx context.Context, p PinRemoveParams) (PinRemoveResult, error) {
	if strings.TrimSpace(p.ProjectID) == "" {
		return PinRemoveResult{}, invalidParams("projectId is required")
	}
	if strings.TrimSpace(p.NodeRef) == "" {
		return PinRemoveResult{}, invalidParams("nodeRef is required")
	}
	err := h.svc.Unpin(ctx, p.ProjectID, p.NodeRef)
	if errors.Is(err, pindomain.ErrNotFound) {
		return PinRemoveResult{Removed: false}, nil
	}
	if err != nil {
		return PinRemoveResult{}, internalError("remove pin: %v", err)
	}
	return PinRemoveResult{Removed: true}, nil
}

// List handles pin.list RPC calls.
func (h *Handler) List(ctx context.Context, p PinListParams) (PinListResult, error) {
	if strings.TrimSpace(p.ProjectID) == "" {
		return PinListResult{}, invalidParams("projectId is required")
	}
	pins, err := h.svc.List(ctx, p.ProjectID)
	if err != nil {
		return PinListResult{}, internalError("list pins: %v", err)
	}
	views := make([]PinView, len(pins))
	for i, pin := range pins {
		views[i] = pinToView(pin)
	}
	return PinListResult{Pins: views}, nil
}
