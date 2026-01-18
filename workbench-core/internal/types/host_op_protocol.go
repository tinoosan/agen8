package types

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tinoosan/workbench-core/internal/validate"
)

const (
	// HostOpFSList lists directory entries in the VFS.
	HostOpFSList = "fs.list"
	// HostOpFSRead reads a file from the VFS.
	HostOpFSRead = "fs.read"
	// HostOpFSWrite writes/replaces a file in the VFS.
	HostOpFSWrite = "fs.write"
	// HostOpFSAppend appends to a file in the VFS.
	HostOpFSAppend = "fs.append"
	// HostOpFSPatch applies a unified diff patch to a file in the VFS.
	HostOpFSPatch = "fs.patch"
	// HostOpToolRun runs a discovered tool via the ToolRunner.
	HostOpToolRun = "tool.run"
	// HostOpFinal ends the agent loop for a user turn.
	HostOpFinal = "final"
)

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

// Validate checks the request is well-formed for its declared Op.
//
// This is intentionally strict for ops that the agent frequently gets wrong (tool.run, final),
// and intentionally lenient for file ops where JSON unmarshalling can't distinguish "missing"
// vs "present but empty" for string fields like Text.
func (r HostOpRequest) Validate() error {
	r.Op = strings.TrimSpace(r.Op)
	switch r.Op {
	case HostOpFSList, HostOpFSRead, HostOpFSWrite, HostOpFSAppend, HostOpFSPatch, HostOpToolRun, HostOpFinal:
	default:
		return fmt.Errorf("unknown op %q", r.Op)
	}

	switch r.Op {
	case HostOpFinal:
		if err := validate.NonEmpty("final.text", r.Text); err != nil {
			return err
		}
		return nil

	case HostOpFSList, HostOpFSRead:
		if err := validate.NonEmpty("path", r.Path); err != nil {
			return err
		}
		if r.MaxBytes < 0 {
			return fmt.Errorf("maxBytes must be >= 0")
		}
		return nil

	case HostOpFSWrite, HostOpFSAppend:
		if err := validate.NonEmpty("path", r.Path); err != nil {
			return err
		}
		return nil

	case HostOpFSPatch:
		if err := validate.NonEmpty("path", r.Path); err != nil {
			return err
		}
		if err := validate.NonEmpty("text", r.Text); err != nil {
			return err
		}
		return nil

	case HostOpToolRun:
		if err := validate.NonEmpty("toolId", r.ToolID.String()); err != nil {
			return err
		}
		if err := validate.NonEmpty("actionId", r.ActionID); err != nil {
			return err
		}
		if r.Input == nil {
			return fmt.Errorf("input is required")
		}
		if r.TimeoutMs < 0 {
			return fmt.Errorf("timeoutMs must be >= 0")
		}
		return nil
	}

	return nil
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
