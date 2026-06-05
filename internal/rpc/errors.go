package rpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

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
		return fmt.Sprintf("rpc error %d: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message)
}

func (e *ProtocolError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

type codedError interface {
	RPCCode() int
}

func InvalidParams(message string) error {
	return &ProtocolError{Code: CodeInvalidParams, Message: strings.TrimSpace(message)}
}

func InvalidRequest(message string) error {
	return &ProtocolError{Code: CodeInvalidRequest, Message: strings.TrimSpace(message)}
}

func internalError(message string, err error) *Error {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "internal error"
	}
	return &Error{Code: CodeInternalError, Message: message}
}

func errorFrom(err error) *Error {
	if err == nil {
		return nil
	}
	var protocolErr *ProtocolError
	if errors.As(err, &protocolErr) {
		message := strings.TrimSpace(protocolErr.Message)
		if message == "" {
			message = err.Error()
		}
		return &Error{Code: protocolErr.Code, Message: message, Data: protocolErr.Data}
	}
	var coded codedError
	if errors.As(err, &coded) {
		message := strings.TrimSpace(err.Error())
		if message == "" {
			message = "request failed"
		}
		return &Error{Code: coded.RPCCode(), Message: message}
	}
	return internalError("internal error", err)
}
