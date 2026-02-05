package protocol

import "encoding/json"

// Message is the JSON-RPC 2.0 framing envelope used by the Workbench protocol.
//
// Exactly one of (Method+Params) or (Result) or (Error) should be set.
// - Requests: JSONRPC, ID, Method, Params
// - Notifications: JSONRPC, Method, Params (no ID)
// - Responses: JSONRPC, ID, Result
// - Error responses: JSONRPC, ID (optional), Error
type Message struct {
	JSONRPC string          `json:"jsonrpc"` // Always "2.0".
	ID      *string         `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// NewRequest creates a JSON-RPC request message.
func NewRequest(id string, method string, params any) (Message, error) {
	raw, err := marshalOptional(params)
	if err != nil {
		return Message{}, err
	}
	return Message{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  method,
		Params:  raw,
	}, nil
}

// NewNotification creates a JSON-RPC notification message.
func NewNotification(method string, params any) (Message, error) {
	raw, err := marshalOptional(params)
	if err != nil {
		return Message{}, err
	}
	return Message{
		JSONRPC: "2.0",
		Method:  method,
		Params:  raw,
	}, nil
}

// NewResponse creates a JSON-RPC success response message.
func NewResponse(id string, result any) (Message, error) {
	raw, err := marshalOptional(result)
	if err != nil {
		return Message{}, err
	}
	return Message{
		JSONRPC: "2.0",
		ID:      &id,
		Result:  raw,
	}, nil
}

// NewErrorResponse creates a JSON-RPC error response message.
//
// Per JSON-RPC, notifications do not have an ID; in that case id should be nil.
func NewErrorResponse(id *string, rpcErr *RPCError) Message {
	return Message{
		JSONRPC: "2.0",
		ID:      id,
		Error:   rpcErr,
	}
}

func marshalOptional(v any) (json.RawMessage, error) {
	if v == nil {
		return nil, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return b, nil
}
