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
	// HostOpFSEdit applies structured edits to a file in the VFS (host-generated diff).
	HostOpFSEdit = "fs.edit"
	// HostOpFSPatch applies a unified diff patch to a file in the VFS.
	HostOpFSPatch = "fs.patch"
	// HostOpToolRun runs a discovered tool via the ToolRunner.
	HostOpToolRun = "tool.run"
	// HostOpShellExec executes a shell command.
	HostOpShellExec = "shell_exec"
	// HostOpHTTPFetch issues an HTTP request.
	HostOpHTTPFetch = "http_fetch"
	// HostOpTrace manages reasoning traces.
	HostOpTrace = "trace"
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
	// Shell execution parameters
	Argv  []string `json:"argv,omitempty"`
	Cwd   string   `json:"cwd,omitempty"`
	Stdin string   `json:"stdin,omitempty"`
	// HTTP fetch parameters
	URL             string            `json:"url,omitempty"`
	Method          string            `json:"method,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	Body            string            `json:"body,omitempty"`
	FollowRedirects *bool             `json:"followRedirects,omitempty"`
	// Trace parameters
	Action string `json:"action,omitempty"`
	Key    string `json:"key,omitempty"`
	Value  string `json:"value,omitempty"`
}

// Validate checks the request is well-formed for its declared Op.
//
// This is intentionally strict for ops that the agent frequently gets wrong (tool.run, final),
// and intentionally lenient for file ops where JSON unmarshalling can't distinguish "missing"
// vs "present but empty" for string fields like Text.
func (r HostOpRequest) Validate() error {
	r.Op = normalizeHostOp(strings.TrimSpace(r.Op))
	switch r.Op {
	case HostOpFSList, HostOpFSRead, HostOpFSWrite, HostOpFSAppend, HostOpFSEdit, HostOpFSPatch, HostOpToolRun, HostOpShellExec, HostOpHTTPFetch, HostOpTrace, HostOpFinal:
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
		if !strings.HasPrefix(strings.TrimSpace(r.Path), "/") {
			return fmt.Errorf("path must be an absolute VFS path (start with /)")
		}
		if r.MaxBytes < 0 {
			return fmt.Errorf("maxBytes must be >= 0")
		}
		return nil

	case HostOpFSWrite, HostOpFSAppend:
		if err := validate.NonEmpty("path", r.Path); err != nil {
			return err
		}
		if !strings.HasPrefix(strings.TrimSpace(r.Path), "/") {
			return fmt.Errorf("path must be an absolute VFS path (start with /)")
		}
		// For fs.write/fs.append, require text to avoid silently writing empty files when
		// models emit unsupported fields (e.g. {"data":...,"encoding":...}) instead of "text".
		//
		// If callers truly want an empty write, they can pass "\n" explicitly.
		if err := validate.NonEmpty("text", r.Text); err != nil {
			return err
		}
		return nil

	case HostOpFSEdit:
		if err := validate.NonEmpty("path", r.Path); err != nil {
			return err
		}
		if !strings.HasPrefix(strings.TrimSpace(r.Path), "/") {
			return fmt.Errorf("path must be an absolute VFS path (start with /)")
		}
		if r.Input == nil {
			return fmt.Errorf("input is required")
		}
		return nil

	case HostOpFSPatch:
		if err := validate.NonEmpty("path", r.Path); err != nil {
			return err
		}
		if !strings.HasPrefix(strings.TrimSpace(r.Path), "/") {
			return fmt.Errorf("path must be an absolute VFS path (start with /)")
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

	case HostOpShellExec:
		if len(r.Argv) == 0 {
			return fmt.Errorf("argv is required")
		}
		return nil

	case HostOpHTTPFetch:
		if err := validate.NonEmpty("url", r.URL); err != nil {
			return err
		}
		if r.MaxBytes < 0 {
			return fmt.Errorf("maxBytes must be >= 0")
		}
		return nil

	case HostOpTrace:
		action := strings.ToLower(strings.TrimSpace(r.Action))
		if action == "" {
			return fmt.Errorf("trace.action is required")
		}
		switch action {
		case "write", "read":
			if err := validate.NonEmpty("trace.key", r.Key); err != nil {
				return err
			}
		case "list":
		default:
			return fmt.Errorf("trace.action must be one of write/list/read")
		}
		return nil
	}

	return nil
}

func normalizeHostOp(op string) string {
	op = strings.TrimSpace(op)
	if op == "" {
		return ""
	}
	// Be permissive to common model mistakes: underscores instead of dots and casing.
	op = strings.ToLower(op)
	switch op {
	case "fs_list":
		return HostOpFSList
	case "fs_read":
		return HostOpFSRead
	case "fs_write":
		return HostOpFSWrite
	case "fs_append":
		return HostOpFSAppend
	case "fs_edit":
		return HostOpFSEdit
	case "fs_patch":
		return HostOpFSPatch
	case "tool_run":
		return HostOpToolRun
	case "shell_exec":
		return HostOpShellExec
	case "shell.exec":
		return HostOpShellExec
	case "http_fetch":
		return HostOpHTTPFetch
	case "http.fetch":
		return HostOpHTTPFetch
	case "trace":
		return HostOpTrace
	default:
		// Already dotted (or unknown): keep as-is (lowercased).
		return op
	}
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
	// Shell output
	ExitCode   int    `json:"exitCode,omitempty"`
	Stdout     string `json:"stdout,omitempty"`
	Stderr     string `json:"stderr,omitempty"`
	StdoutPath string `json:"stdoutPath,omitempty"`
	StderrPath string `json:"stderrPath,omitempty"`
	// HTTP response
	FinalURL      string              `json:"finalUrl,omitempty"`
	Status        int                 `json:"status,omitempty"`
	Headers       map[string][]string `json:"headers,omitempty"`
	ContentType   string              `json:"contentType,omitempty"`
	BytesRead     int                 `json:"bytesRead,omitempty"`
	Body          string              `json:"body,omitempty"`
	BodyTruncated bool                `json:"bodyTruncated,omitempty"`
	BodyPath      string              `json:"bodyPath,omitempty"`
	Warning       string              `json:"warning,omitempty"`
	// Trace output
	TraceKeys  []string `json:"traceKeys,omitempty"`
	TraceValue string   `json:"value,omitempty"`
}
