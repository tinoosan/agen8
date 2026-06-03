package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type Server struct {
	registry *Registry
}

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

func NewServer(registry *Registry) (*Server, error) {
	if registry == nil {
		return nil, fmt.Errorf("rpc registry is required")
	}
	return &Server{registry: registry}, nil
}

func (s *Server) Handle(ctx context.Context, raw []byte) ([]byte, error) {
	var req Request
	if err := json.Unmarshal(raw, &req); err != nil {
		return json.Marshal(errorResponse(nil, &Error{Code: CodeParseError, Message: "parse error"}))
	}
	resp := s.Dispatch(ctx, req)
	return json.Marshal(resp)
}

func (s *Server) Dispatch(ctx context.Context, req Request) Response {
	if s == nil || s.registry == nil {
		return errorResponse(req.ID, &Error{Code: CodeInternalError, Message: "rpc server is not configured"})
	}
	if strings.TrimSpace(req.JSONRPC) != "2.0" {
		return errorResponse(req.ID, &Error{Code: CodeInvalidRequest, Message: "jsonrpc must be 2.0"})
	}
	method := strings.TrimSpace(req.Method)
	if method == "" {
		return errorResponse(req.ID, &Error{Code: CodeInvalidRequest, Message: "method is required"})
	}
	handler, ok := s.registry.Handler(method)
	if !ok {
		return errorResponse(req.ID, &Error{Code: CodeMethodNotFound, Message: "method not found"})
	}
	result, err := handler.Handle(ctx, req.Params)
	if err != nil {
		return errorResponse(req.ID, errorFrom(err))
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return errorResponse(req.ID, internalError("internal error", err))
	}
	return Response{JSONRPC: "2.0", ID: req.ID, Result: raw}
}

func errorResponse(id json.RawMessage, rpcErr *Error) Response {
	if rpcErr == nil {
		rpcErr = &Error{Code: CodeInternalError, Message: "internal error"}
	}
	return Response{JSONRPC: "2.0", ID: id, Error: rpcErr}
}
