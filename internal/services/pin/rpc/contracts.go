package rpc

import (
	"fmt"
	"time"

	pindomain "github.com/tinoosan/agen8/internal/services/pin/domain"
)

// JSON-RPC error codes mirrored from the transport layer. Defined locally so
// the pin rpc package stays a leaf relative to internal/rpc, whose errorFrom
// maps any error implementing RPCCode() onto the wire response.
const (
	codeInvalidParams = -32602
	codeInternalError = -32603
)

type rpcError struct {
	code    int
	message string
}

func (e *rpcError) Error() string { return e.message }

func (e *rpcError) RPCCode() int { return e.code }

func invalidParams(message string) *rpcError {
	return &rpcError{code: codeInvalidParams, message: message}
}

func internalError(format string, args ...any) *rpcError {
	return &rpcError{code: codeInternalError, message: fmt.Sprintf(format, args...)}
}

// PinView is the wire-format read model for a pin.
type PinView struct {
	ProjectID string    `json:"projectId"`
	NodeRef   string    `json:"nodeRef"`
	NodeType  string    `json:"nodeType,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

func pinToView(p pindomain.Pin) PinView {
	return PinView{
		ProjectID: p.ProjectID,
		NodeRef:   p.NodeRef,
		NodeType:  p.NodeType,
		CreatedAt: p.CreatedAt,
	}
}

// ── pin.add ──────────────────────────────────────────────────────────────

type PinAddParams struct {
	ProjectID string `json:"projectId"`
	NodeRef   string `json:"nodeRef"`
	NodeType  string `json:"nodeType,omitempty"`
}

type PinAddResult struct {
	Pin PinView `json:"pin"`
}

// ── pin.remove ───────────────────────────────────────────────────────────

type PinRemoveParams struct {
	ProjectID string `json:"projectId"`
	NodeRef   string `json:"nodeRef"`
}

type PinRemoveResult struct {
	Removed bool `json:"removed"`
}

// ── pin.list ─────────────────────────────────────────────────────────────

type PinListParams struct {
	ProjectID string `json:"projectId"`
}

type PinListResult struct {
	Pins []PinView `json:"pins"`
}
