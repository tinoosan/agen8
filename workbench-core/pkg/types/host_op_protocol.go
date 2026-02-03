package types

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/pkg/validate"
)

const (
	// HostOpFSList lists directory entries in the VFS.
	HostOpFSList = "fs.list"
	// HostOpFSRead reads a file from the VFS.
	HostOpFSRead = "fs.read"
	// HostOpFSSearch searches a VFS mount for matching content (e.g. /memory vector search).
	HostOpFSSearch = "fs.search"
	// HostOpFSWrite writes/replaces a file in the VFS.
	HostOpFSWrite = "fs.write"
	// HostOpFSAppend appends to a file in the VFS.
	HostOpFSAppend = "fs.append"
	// HostOpFSEdit applies structured edits to a file in the VFS (host-generated diff).
	HostOpFSEdit = "fs.edit"
	// HostOpFSPatch applies a unified diff patch to a file in the VFS.
	HostOpFSPatch = "fs.patch"
	// HostOpShellExec executes a shell command.
	HostOpShellExec = "shell.exec"
	// HostOpHTTPFetch issues an HTTP request.
	HostOpHTTPFetch = "http.fetch"
	// HostOpBrowser performs an interactive browser action (stateful session).
	HostOpBrowser = "browser"
	// HostOpTrace runs a trace action (e.g. events.latest).
	HostOpTrace = "trace.run"
	// HostOpFinal ends the agent loop for a user turn.
	HostOpFinal = "agent.final"

	CommandRejectedErrorCode    = "command_rejected"
	CommandRejectedErrorMessage = "User denied this command. This is a normal part of the workflow. Do not treat this as a system failure. You should propose an alternative command, ask the user for specialized instructions, or proceed with independent work if possible."
)

// HostOpRequest is the minimal "host primitive" request envelope.
type HostOpRequest struct {
	Op        string          `json:"op"`
	Path      string          `json:"path,omitempty"`
	Query     string          `json:"query,omitempty"`
	Limit     int             `json:"limit,omitempty"`
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
func (r HostOpRequest) Validate() error {
	r.Op = normalizeHostOp(strings.TrimSpace(r.Op))
	switch r.Op {
	case HostOpFSList, HostOpFSRead, HostOpFSSearch, HostOpFSWrite, HostOpFSAppend, HostOpFSEdit, HostOpFSPatch, HostOpShellExec, HostOpHTTPFetch, HostOpBrowser, HostOpTrace, HostOpFinal:
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

	case HostOpFSSearch:
		if err := validate.NonEmpty("path", r.Path); err != nil {
			return err
		}
		if !strings.HasPrefix(strings.TrimSpace(r.Path), "/") {
			return fmt.Errorf("path must be an absolute VFS path (start with /)")
		}
		if err := validate.NonEmpty("query", r.Query); err != nil {
			return err
		}
		if r.Limit < 0 {
			return fmt.Errorf("limit must be >= 0")
		}
		return nil

	case HostOpFSWrite, HostOpFSAppend:
		if err := validate.NonEmpty("path", r.Path); err != nil {
			return err
		}
		if !strings.HasPrefix(strings.TrimSpace(r.Path), "/") {
			return fmt.Errorf("path must be an absolute VFS path (start with /)")
		}
		if err := validate.NonEmpty("text", r.Text); err != nil {
			return err
		}

		if strings.HasPrefix(strings.TrimSpace(r.Path), "/memory/") {
			if err := validateMemoryWritePath(r.Path); err != nil {
				return err
			}
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

	case HostOpBrowser:
		if r.Input == nil {
			return fmt.Errorf("browser.input is required")
		}
		return nil

	case HostOpTrace:
		action := strings.ToLower(strings.TrimSpace(r.Action))
		if action == "" {
			return fmt.Errorf("trace.action is required")
		}
		switch action {
		case "events.since", "events.latest", "events.summary":
			if r.Input == nil {
				return fmt.Errorf("trace.input is required")
			}
		default:
			return fmt.Errorf("trace.action must be one of events.since/events.latest/events.summary")
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
	op = strings.ToLower(op)
	// Backwards-compat: legacy snake_case op aliases are deprecated but still accepted.
	switch op {
	case "fs_list":
		return HostOpFSList
	case "fs_read":
		return HostOpFSRead
	case "fs_search":
		return HostOpFSSearch
	case "fs_write":
		return HostOpFSWrite
	case "fs_append":
		return HostOpFSAppend
	case "fs_edit":
		return HostOpFSEdit
	case "fs_patch":
		return HostOpFSPatch
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
	case "trace.run":
		return HostOpTrace
	case "final":
		return HostOpFinal
	case "agent.final":
		return HostOpFinal
	default:
		return op
	}
}

// validateMemoryWritePath enforces that memory writes target only today's daily memory file.
func validateMemoryWritePath(path string) error {
	trimmed := strings.TrimSpace(path)

	// Allow master instructions file; write protection handled by resource layer.
	if strings.EqualFold(filepath.Base(trimmed), "MEMORY.MD") {
		return nil
	}

	base := filepath.Base(trimmed)
	if !strings.HasSuffix(base, "-memory.md") {
		return fmt.Errorf("memory files must use format YYYY-MM-DD-memory.md")
	}

	datePart := strings.TrimSuffix(base, "-memory.md")
	fileDate, err := time.Parse("2006-01-02", datePart)
	if err != nil {
		return fmt.Errorf("invalid date format in memory filename: %w", err)
	}

	today := time.Now().Format("2006-01-02")
	if fileDate.Format("2006-01-02") != today {
		return fmt.Errorf("can only write to today's memory file: /memory/%s-memory.md", today)
	}

	return nil
}

// SearchResult is one result returned by fs.search.
type SearchResult struct {
	Title   string  `json:"title,omitempty"`
	Path    string  `json:"path,omitempty"`
	Snippet string  `json:"snippet,omitempty"`
	Score   float64 `json:"score,omitempty"`
}

// HostOpResponse is the minimal "host primitive" response envelope.
type HostOpResponse struct {
	Op        string         `json:"op"`
	Ok        bool           `json:"ok"`
	Error     string         `json:"error,omitempty"`
	ErrorCode string         `json:"errorCode,omitempty"`
	Entries   []string       `json:"entries,omitempty"`
	Results   []SearchResult `json:"results,omitempty"`
	BytesLen  int            `json:"bytesLen,omitempty"`
	Text      string         `json:"text,omitempty"`
	BytesB64  string         `json:"bytesB64,omitempty"`
	Truncated bool           `json:"truncated,omitempty"`
	// Shell output
	ExitCode int    `json:"exitCode,omitempty"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	// HTTP response
	FinalURL      string              `json:"finalUrl,omitempty"`
	Status        int                 `json:"status,omitempty"`
	Headers       map[string][]string `json:"headers,omitempty"`
	ContentType   string              `json:"contentType,omitempty"`
	BytesRead     int                 `json:"bytesRead,omitempty"`
	Body          string              `json:"body,omitempty"`
	BodyTruncated bool                `json:"bodyTruncated,omitempty"`
	Warning       string              `json:"warning,omitempty"`
	// Trace output
	TraceKeys  []string `json:"traceKeys,omitempty"`
	TraceValue string   `json:"value,omitempty"`
}
