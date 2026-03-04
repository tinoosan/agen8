package types

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/tinoosan/agen8/pkg/validate"
)

const (
	// HostOpFSList lists directory entries in the VFS.
	HostOpFSList = "fs_list"
	// HostOpFSStat returns metadata for a VFS path without reading file content.
	HostOpFSStat = "fs_stat"
	// HostOpFSRead reads a file from the VFS.
	HostOpFSRead = "fs_read"
	// HostOpFSSearch searches a VFS mount for matching content (e.g. /memory vector search).
	HostOpFSSearch = "fs_search"
	// HostOpFSWrite writes/replaces a file in the VFS.
	HostOpFSWrite = "fs_write"
	// HostOpFSAppend appends to a file in the VFS.
	HostOpFSAppend = "fs_append"
	// HostOpFSEdit applies structured edits to a file in the VFS (host-generated diff).
	HostOpFSEdit = "fs_edit"
	// HostOpFSPatch applies a unified diff patch to a file in the VFS.
	HostOpFSPatch = "fs_patch"
	// HostOpShellExec executes a shell command.
	HostOpShellExec = "shell_exec"
	// HostOpHTTPFetch issues an HTTP request.
	HostOpHTTPFetch = "http_fetch"
	// HostOpBrowser performs an interactive browser action (stateful session).
	HostOpBrowser = "browser"
	// HostOpTrace runs a trace action (e.g. events.latest).
	HostOpTrace = "trace_run"
	// HostOpEmail sends an email notification.
	HostOpEmail = "email"
	// HostOpCodeExec executes model-provided code in a guarded runtime.
	HostOpCodeExec = "code_exec"
	// HostOpNoop returns a host response without side effects (internal use).
	HostOpNoop = "noop"
	// HostOpToolResult is a successful tool completion with a user-visible message; all tool calls should return a result the user can see.
	HostOpToolResult = "tool_result"
	// HostOpFinal ends the agent loop for a user turn.
	HostOpFinal = "agent_final"

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
	DryRun    bool            `json:"dryRun,omitempty"`
	Verbose   bool            `json:"verbose,omitempty"`
	Tag       string          `json:"tag,omitempty"`
	// Code execution parameters
	Language string `json:"language,omitempty"`
	Code     string `json:"code,omitempty"`
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
	r.Op = strings.ToLower(strings.TrimSpace(r.Op))
	switch r.Op {
	case HostOpFSList, HostOpFSStat, HostOpFSRead, HostOpFSSearch, HostOpFSWrite, HostOpFSAppend, HostOpFSEdit, HostOpFSPatch, HostOpShellExec, HostOpHTTPFetch, HostOpBrowser, HostOpTrace, HostOpEmail, HostOpCodeExec, HostOpNoop, HostOpToolResult, HostOpFinal:
	default:
		return fmt.Errorf("unknown op %q", r.Op)
	}

	switch r.Op {
	case HostOpNoop:
		return nil
	case HostOpToolResult:
		if err := validate.NonEmpty("tool_result.text", r.Text); err != nil {
			return err
		}
		return nil

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

	case HostOpFSStat:
		if err := validate.NonEmpty("path", r.Path); err != nil {
			return err
		}
		if !strings.HasPrefix(strings.TrimSpace(r.Path), "/") {
			return fmt.Errorf("path must be an absolute VFS path (start with /)")
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

	case HostOpEmail:
		if r.Input == nil {
			return fmt.Errorf("email.input is required")
		}
		return nil

	case HostOpCodeExec:
		lang := strings.ToLower(strings.TrimSpace(r.Language))
		if lang == "" {
			return fmt.Errorf("code_exec.language is required")
		}
		if lang != "python" {
			return fmt.Errorf("code_exec.language must be \"python\"")
		}
		if err := validate.NonEmpty("code_exec.code", r.Code); err != nil {
			return err
		}
		if r.TimeoutMs < 0 {
			return fmt.Errorf("timeoutMs must be >= 0")
		}
		if r.MaxBytes < 0 {
			return fmt.Errorf("maxBytes must be >= 0")
		}
		if err := validateToolCwd(r.Cwd); err != nil {
			return fmt.Errorf("code_exec.cwd invalid: %w", err)
		}
		return nil
	}

	return nil
}

func validateToolCwd(cwd string) error {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return nil
	}
	if strings.Contains(cwd, "\x00") {
		return fmt.Errorf("contains invalid null byte")
	}
	cleaned := filepath.Clean(cwd)
	if strings.HasPrefix(cwd, "/") {
		trimmed := strings.TrimPrefix(cleaned, "/")
		if trimmed == "" {
			return fmt.Errorf("must target a VFS mount path")
		}
		if strings.HasPrefix(trimmed, "..") {
			return fmt.Errorf("must not escape mount root")
		}
		return nil
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return fmt.Errorf("must not escape project root")
	}
	return nil
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

// SearchResult is one result returned by fs_search.
type SearchResult struct {
	Title   string  `json:"title,omitempty"`
	Path    string  `json:"path,omitempty"`
	Snippet string  `json:"snippet,omitempty"`
	Score   float64 `json:"score,omitempty"`
}

// PatchDiagnostics captures detailed fs_patch apply/validation outcomes.
type PatchDiagnostics struct {
	Mode            string   `json:"mode,omitempty"`
	HunksTotal      int      `json:"hunksTotal,omitempty"`
	HunksApplied    int      `json:"hunksApplied,omitempty"`
	FailedHunk      int      `json:"failedHunk,omitempty"`
	HunkHeader      string   `json:"hunkHeader,omitempty"`
	TargetLine      int      `json:"targetLine,omitempty"`
	FailureReason   string   `json:"failureReason,omitempty"`
	ExpectedContext []string `json:"expectedContext,omitempty"`
	ActualContext   []string `json:"actualContext,omitempty"`
	Suggestion      string   `json:"suggestion,omitempty"`
}

// HostOpResponse is the minimal "host primitive" response envelope.
type HostOpResponse struct {
	Op        string         `json:"op"`
	Ok        bool           `json:"ok"`
	Error     string         `json:"error,omitempty"`
	ErrorCode string         `json:"errorCode,omitempty"`
	Entries   []string       `json:"entries,omitempty"`
	Results   []SearchResult `json:"results,omitempty"`
	IsDir     *bool          `json:"isDir,omitempty"`
	SizeBytes *int64         `json:"sizeBytes,omitempty"`
	// Patch diagnostics are emitted for fs_patch success/failure and dry-run validation.
	PatchDiagnostics *PatchDiagnostics `json:"patchDiagnostics,omitempty"`
	PatchDryRun      bool              `json:"patchDryRun,omitempty"`
	BytesLen         int               `json:"bytesLen,omitempty"`
	Text             string            `json:"text,omitempty"`
	BytesB64         string            `json:"bytesB64,omitempty"`
	Truncated        bool              `json:"truncated,omitempty"`
	// Shell output
	ExitCode int    `json:"exitCode,omitempty"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	// HTTP response
	FinalURL          string              `json:"finalUrl,omitempty"`
	Status            int                 `json:"status,omitempty"`
	Headers           map[string][]string `json:"headers,omitempty"`
	ContentType       string              `json:"contentType,omitempty"`
	BytesRead         int                 `json:"bytesRead,omitempty"`
	Body              string              `json:"body,omitempty"`
	BodyTruncated     bool                `json:"bodyTruncated,omitempty"`
	Warning           string              `json:"warning,omitempty"`
	VFSPathTranslated bool                `json:"vfsPathTranslated,omitempty"`
	VFSPathMounts     string              `json:"vfsPathMounts,omitempty"`
	// Shell script mitigation telemetry
	ScriptPathNormalized bool   `json:"scriptPathNormalized,omitempty"`
	ScriptAntiPattern    string `json:"scriptAntiPattern,omitempty"`
	// Trace output
	TraceKeys  []string `json:"traceKeys,omitempty"`
	TraceValue string   `json:"value,omitempty"`
}
