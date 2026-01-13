package types

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	// ErrToolProtocolVersionRequired indicates that the "version" field is missing.
	ErrToolProtocolVersionRequired = errors.New("version is required")
	// ErrToolProtocolUnsupportedVersion indicates that the "version" field is not supported.
	ErrToolProtocolUnsupportedVersion = errors.New("unsupported version")

	// ErrToolCallIDRequired indicates that "callId" is missing.
	ErrToolCallIDRequired = errors.New("callId is required")
	// ErrToolIDRequired indicates that "toolId" is missing.
	ErrToolIDRequired = errors.New("toolId is required")
	// ErrToolActionIDRequired indicates that "actionId" is missing.
	ErrToolActionIDRequired = errors.New("actionId is required")

	// ErrToolRequestInputRequired indicates that "input" is missing (nil).
	ErrToolRequestInputRequired = errors.New("input is required")
	// ErrToolRequestTimeoutInvalid indicates that "timeoutMs" is invalid (< 0).
	ErrToolRequestTimeoutInvalid = errors.New("timeoutMs must be >= 0")

	// ErrToolResponseErrorMustBeNilWhenOK indicates that "error" must be nil when ok=true.
	ErrToolResponseErrorMustBeNilWhenOK = errors.New("error must be nil when ok=true")
	// ErrToolResponseErrorRequiredWhenNotOK indicates that "error" must be non-nil when ok=false.
	ErrToolResponseErrorRequiredWhenNotOK = errors.New("error is required when ok=false")

	// ErrToolErrorCodeRequired indicates that ToolError.Code is missing.
	ErrToolErrorCodeRequired = errors.New("error.code is required")
	// ErrToolErrorMessageRequired indicates that ToolError.Message is missing.
	ErrToolErrorMessageRequired = errors.New("error.message is required")

	// ErrToolArtifactPathRequired indicates that ToolArtifactRef.Path is missing.
	ErrToolArtifactPathRequired = errors.New("artifact.path is required")
	// ErrToolArtifactPathMustBeRelative indicates that ToolArtifactRef.Path is absolute.
	ErrToolArtifactPathMustBeRelative = errors.New("artifact.path must be relative")
	// ErrToolArtifactMediaTypeRequired indicates that ToolArtifactRef.MediaType is missing.
	ErrToolArtifactMediaTypeRequired = errors.New("artifact.mediaType is required")
)

// Tool protocol overview (explicit /tools + /results usage)
//
// The host/agent interacts with tools and tool outputs through two VFS mounts:
//   - /tools   (discovery + manifest reads)
//   - /results (responses + artifacts produced by tool calls)
//
// /tools: discovery + manifest only
//
// VFS API contract:
//
//   - fs.List("/tools")
//     => returns tool IDs as directory-like entries
//     e.g. "/tools/github.com.acme.stock"
//
//   - fs.Read("/tools/<toolId>")
//     => returns ONLY the tool manifest JSON bytes
//
// Notes:
//   - The agent does NOT need to know "manifest.json" as a filename.
//   - The VFS may also accept fs.Read("/tools/<toolId>/manifest.json") for explicitness.
//   - The agent should not list inside tool directories; the manifest is the interface surface.
//
// Example (agent/host side):
//
//	entries, _ := fs.List("/tools")
//	// entries include:
//	//   { Path: "/tools/github.com.acme.stock", IsDir: true }
//	//   { Path: "/tools/github.com.other.stock", IsDir: true }
//	manifestJSON, _ := fs.Read("/tools/github.com.acme.stock")
//
// Tool storage (implementation detail; should be invisible to the agent):
//
//   - Builtins may be in-memory but still appear under /tools.
//
//   - Custom tools may exist on disk as:
//
//     data/tools/<toolId>/manifest.json
//
// (Some deployments may choose:
//
//	data/tools/custom/<toolId>/manifest.json
//
// but the VFS interface remains the same.)
//
// /results: callId-first layout
//
// After a tool call finishes, outputs are stored under a call directory keyed by callId:
//   - /results/<callId>/response.json
//   - /results/<callId>/artifacts/<file>
//   - /results/index.jsonl                 (append-only index for discoverability)
//
// "artifact" means a file produced by the tool call that can be read later (JSON, CSV, PNG, etc).
// ToolResponse.Artifacts contains paths RELATIVE to the call directory, e.g. "artifacts/quote.json".
//
// Example (runner/host side; no IO implementation here, just the contract):
//
//	req := ToolRequest{
//	  Version:  "v1",
//	  CallID:   "<uuid>",
//	  ToolID:   "github.com.acme.stock",
//	  ActionID: "quote.latest",
//	  Input:    []byte(`{"symbol":"AAPL"}`),
//	}
//
//	resp := ToolResponse{
//	  Version:  "v1",
//	  CallID:   req.CallID,
//	  ToolID:   req.ToolID,
//	  ActionID: req.ActionID,
//	  Ok:       true,
//	  Output:   []byte(`{"price":123.45}`),
//	  Artifacts: []ToolArtifactRef{
//	    { Path: "artifacts/quote.json", MediaType: "application/json" },
//	  },
//	}
//
//	// Runner writes:
//	//   /results/<callId>/response.json
//	//   /results/<callId>/artifacts/quote.json
//	// and appends to:
//	//   /results/index.jsonl
//	line := NewIndexLineFromResponse(resp, "quote.latest AAPL ok")
//
// On-disk examples (implementation detail; for a specific runId):
//
//	data/runs/<runId>/results/<callId>/response.json
//	data/runs/<runId>/results/<callId>/artifacts/<file>
//	data/runs/<runId>/results/index.jsonl
//
// Rationale:
//   - Avoids encoding tool IDs into path segments.
//   - Guarantees uniqueness via callId.
//   - Supports concurrency cleanly.
//   - Tool identity is recorded in response.json (toolId/actionId), not inferred from directory structure.
//
// ToolRequest is the JSON request envelope the host writes for a tool call.
type ToolRequest struct {
	Version   string            `json:"version"`             // always "v1" for now
	CallID    string            `json:"callId"`              // generated by host (uuid as string)
	ToolID    ToolID            `json:"toolId"`              // dot-separated tool id
	ActionID  string            `json:"actionId"`            // action identifier within the tool
	Input     json.RawMessage   `json:"input"`               // raw JSON payload (must not be nil)
	TimeoutMs int               `json:"timeoutMs,omitempty"` // optional
	Meta      map[string]string `json:"meta,omitempty"`      // optional small metadata
}

func (r ToolRequest) Validate() error {
	if r.Version != "v1" {
		if r.Version == "" {
			return ErrToolProtocolVersionRequired
		}
		return fmt.Errorf("%w %q", ErrToolProtocolUnsupportedVersion, r.Version)
	}
	if r.CallID == "" {
		return ErrToolCallIDRequired
	}
	if r.ToolID.String() == "" {
		return ErrToolIDRequired
	}
	if r.ActionID == "" {
		return ErrToolActionIDRequired
	}
	if r.Input == nil {
		return ErrToolRequestInputRequired
	}
	if r.TimeoutMs < 0 {
		return ErrToolRequestTimeoutInvalid
	}
	return nil
}

// ToolError represents a structured error returned by a tool.
type ToolError struct {
	Code      string `json:"code"` // e.g. "invalid_input", "tool_failed", "timeout"
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

func (e ToolError) Validate() error {
	if e.Code == "" {
		return ErrToolErrorCodeRequired
	}
	if e.Message == "" {
		return ErrToolErrorMessageRequired
	}
	return nil
}

// ToolArtifactRef is a reference to an artifact written under /results/<runId>/.
type ToolArtifactRef struct {
	Path      string `json:"path"`      // relative path under results/<runId>/ e.g. "artifacts/quote.json"
	MediaType string `json:"mediaType"` // e.g. "application/json"
}

func (a ToolArtifactRef) Validate() error {
	if a.Path == "" {
		return ErrToolArtifactPathRequired
	}
	if strings.HasPrefix(a.Path, "/") {
		return ErrToolArtifactPathMustBeRelative
	}
	if a.MediaType == "" {
		return ErrToolArtifactMediaTypeRequired
	}
	return nil
}

// ToolResponse is the JSON response envelope the runner writes for a tool call.
type ToolResponse struct {
	Version   string            `json:"version"` // "v1"
	CallID    string            `json:"callId"`
	ToolID    ToolID            `json:"toolId"`
	ActionID  string            `json:"actionId"`
	Ok        bool              `json:"ok"`
	Error     *ToolError        `json:"error,omitempty"`  // present when Ok=false
	Output    json.RawMessage   `json:"output,omitempty"` // tool output payload
	Artifacts []ToolArtifactRef `json:"artifacts,omitempty"`
}

func (r ToolResponse) Validate() error {
	if r.Version != "v1" {
		if r.Version == "" {
			return ErrToolProtocolVersionRequired
		}
		return fmt.Errorf("%w %q", ErrToolProtocolUnsupportedVersion, r.Version)
	}
	if r.CallID == "" {
		return ErrToolCallIDRequired
	}
	if r.ToolID.String() == "" {
		return ErrToolIDRequired
	}
	if r.ActionID == "" {
		return ErrToolActionIDRequired
	}
	for _, a := range r.Artifacts {
		if err := a.Validate(); err != nil {
			return err
		}
	}

	if r.Ok {
		if r.Error != nil {
			return ErrToolResponseErrorMustBeNilWhenOK
		}
		return nil
	}

	if r.Error == nil {
		return ErrToolResponseErrorRequiredWhenNotOK
	}
	return r.Error.Validate()
}

func NewToolResponseOK(req ToolRequest, output json.RawMessage, artifacts []ToolArtifactRef) ToolResponse {
	return ToolResponse{
		Version:   req.Version,
		CallID:    req.CallID,
		ToolID:    req.ToolID,
		ActionID:  req.ActionID,
		Ok:        true,
		Error:     nil,
		Output:    output,
		Artifacts: artifacts,
	}
}

func NewToolResponseError(req ToolRequest, code, message string, retryable bool) ToolResponse {
	return ToolResponse{
		Version:  req.Version,
		CallID:   req.CallID,
		ToolID:   req.ToolID,
		ActionID: req.ActionID,
		Ok:       false,
		Error: &ToolError{
			Code:      code,
			Message:   message,
			Retryable: retryable,
		},
	}
}

// ToolResultIndexLine is appended to /results/<runId>/index.jsonl for agent discoverability.
type ToolResultIndexLine struct {
	CallID     string `json:"callId"`
	ToolID     ToolID `json:"toolId"`
	ActionID   string `json:"actionId"`
	Ok         bool   `json:"ok"`
	FinishedAt string `json:"finishedAt"`        // RFC3339 time string
	Summary    string `json:"summary,omitempty"` // optional short string
}

func NewIndexLineFromResponse(resp ToolResponse, summary string) ToolResultIndexLine {
	return ToolResultIndexLine{
		CallID:     resp.CallID,
		ToolID:     resp.ToolID,
		ActionID:   resp.ActionID,
		Ok:         resp.Ok,
		FinishedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Summary:    summary,
	}
}
