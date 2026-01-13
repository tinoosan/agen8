package types

import "encoding/json"

// HostOpRequest is the minimal "host primitive" request envelope.
//
// These ops are not discovered via /tools; they are part of the agent runtime contract.
// They exist so the agent can explore its environment (VFS) and request tool execution.
//
// Example:
//
//	{"op":"fs.list","path":"/tools"}
//	{"op":"fs.read","path":"/tools/github.com.acme.stock"}
//	{"op":"tool.run","toolId":"github.com.acme.stock","actionId":"quote.latest","input":{"symbol":"AAPL"}}
type HostOpRequest struct {
	Op        string          `json:"op"`
	Path      string          `json:"path,omitempty"`
	ToolID    ToolID          `json:"toolId,omitempty"`
	ActionID  string          `json:"actionId,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	TimeoutMs int             `json:"timeoutMs,omitempty"`
	MaxBytes  int             `json:"maxBytes,omitempty"`
	Text      string          `json:"text,omitempty"`
}

// HostOpResponse is the minimal "host primitive" response envelope.
//
// For fs.* ops, the host can respond with Entries (for list) or Text/BytesLen (for read).
// For tool.run, the host returns ToolResponse, and the agent can then read persisted results
// via /results/<callId>/....
type HostOpResponse struct {
	Op        string   `json:"op"`
	Ok        bool     `json:"ok"`
	Error     string   `json:"error,omitempty"`
	Entries   []string `json:"entries,omitempty"`
	BytesLen  int      `json:"bytesLen,omitempty"`
	Text      string   `json:"text,omitempty"`
	BytesB64  string   `json:"bytesB64,omitempty"`
	Truncated bool     `json:"truncated,omitempty"`

	ToolResponse *ToolResponse `json:"toolResponse,omitempty"`
}
