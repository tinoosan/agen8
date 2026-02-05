package protocol

import (
	"encoding/json"
	"fmt"
)

// RPCError represents a JSON-RPC 2.0 error object.
//
// Code and Message are required. Data is optional and is intended for structured
// debugging details (never required for correct client behavior).
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Standard JSON-RPC 2.0 error codes.
// See: https://www.jsonrpc.org/specification#error_object
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

// Protocol-specific error codes.
//
// These live in the JSON-RPC "server error" range (-32000 to -32099).
const (
	CodeUnsupportedVersion = -32001
	CodeThreadNotFound     = -32002
	CodeTurnNotFound       = -32003
	CodeItemNotFound       = -32004
	CodeTurnNotCancelable  = -32005
	CodeInvalidState       = -32006
)

// ProtocolError is an error value suitable for returning as a JSON-RPC error.
type ProtocolError struct {
	Code    int
	Message string
	Data    json.RawMessage
	Cause   error
}

func (e *ProtocolError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Cause != nil {
		return fmt.Sprintf("protocol error %d: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("protocol error %d: %s", e.Code, e.Message)
}

// RPC returns an RPCError suitable for embedding in a JSON-RPC Message.
func (e *ProtocolError) RPC() *RPCError {
	if e == nil {
		return nil
	}
	return &RPCError{
		Code:    e.Code,
		Message: e.Message,
		Data:    e.Data,
	}
}

// NewProtocolError creates a ProtocolError with optional structured data.
func NewProtocolError(code int, message string, data any, cause error) (*ProtocolError, error) {
	var raw json.RawMessage
	if data != nil {
		b, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}
		raw = b
	}
	return &ProtocolError{
		Code:    code,
		Message: message,
		Data:    raw,
		Cause:   cause,
	}, nil
}
