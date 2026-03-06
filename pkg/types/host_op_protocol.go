package types

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tinoosan/agen8/pkg/checksumutil"
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
	// HostOpFSBatchEdit applies the same structured edits across multiple files.
	HostOpFSBatchEdit = "fs_batch_edit"
	// HostOpFSTxn applies a sequence of mutating fs_* operations atomically.
	HostOpFSTxn = "fs_txn"
	// HostOpFSArchiveCreate creates a zip/tar/tar.gz archive from VFS paths.
	HostOpFSArchiveCreate = "fs_archive_create"
	// HostOpFSArchiveExtract extracts an archive into a VFS destination.
	HostOpFSArchiveExtract = "fs_archive_extract"
	// HostOpFSArchiveList lists archive entries without extracting.
	HostOpFSArchiveList = "fs_archive_list"
	// HostOpShellExec executes a shell command.
	HostOpShellExec = "shell_exec"
	// HostOpPipe runs a declarative pipeline of tool calls and transforms.
	HostOpPipe = "pipe"
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
	Pattern   string          `json:"pattern,omitempty"`
	Limit     int             `json:"limit,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	TimeoutMs int             `json:"timeoutMs,omitempty"`
	MaxBytes  int             `json:"maxBytes,omitempty"`
	Text      string          `json:"text,omitempty"`
	Verify    bool            `json:"verify,omitempty"`
	Mode      string          `json:"mode,omitempty"`
	Checksum  string          `json:"checksum,omitempty"`
	Checksums []string        `json:"checksums,omitempty"`
	// ChecksumExpected optionally enforces an expected digest value for the selected algorithm.
	ChecksumExpected string        `json:"checksumExpected,omitempty"`
	Atomic           bool          `json:"atomic,omitempty"`
	Sync             bool          `json:"sync,omitempty"`
	DryRun           bool          `json:"dryRun,omitempty"`
	Verbose          bool          `json:"verbose,omitempty"`
	BatchEditEdits   []BatchEdit   `json:"batchEditEdits,omitempty"`
	BatchEditOptions *BatchOptions `json:"batchEditOptions,omitempty"`
	TxnSteps         []FSTxnStep   `json:"txnSteps,omitempty"`
	TxnOptions       *FSTxnOptions `json:"txnOptions,omitempty"`
	Destination      string        `json:"destination,omitempty"`
	Format           string        `json:"format,omitempty"`
	Overwrite        bool          `json:"overwrite,omitempty"`
	Glob             string        `json:"glob,omitempty"`
	Exclude          []string      `json:"exclude,omitempty"`
	PreviewLines     int           `json:"previewLines,omitempty"`
	IncludeMetadata  bool          `json:"includeMetadata,omitempty"`
	MaxSizeBytes     int64         `json:"maxSizeBytes,omitempty"`
	Tag              string        `json:"tag,omitempty"`
	PipeSteps        []PipeStep    `json:"pipeSteps,omitempty"`
	PipeOptions      *PipeOptions  `json:"pipeOptions,omitempty"`
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
	case HostOpFSList, HostOpFSStat, HostOpFSRead, HostOpFSSearch, HostOpFSWrite, HostOpFSAppend, HostOpFSEdit, HostOpFSPatch, HostOpFSBatchEdit, HostOpFSTxn, HostOpFSArchiveCreate, HostOpFSArchiveExtract, HostOpFSArchiveList, HostOpShellExec, HostOpPipe, HostOpHTTPFetch, HostOpBrowser, HostOpTrace, HostOpEmail, HostOpCodeExec, HostOpNoop, HostOpToolResult, HostOpFinal:
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
		if err := validateReadChecksums(r.Checksum, r.Checksums); err != nil {
			return err
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
		if strings.TrimSpace(r.Query) == "" && strings.TrimSpace(r.Pattern) == "" {
			return fmt.Errorf("query or pattern is required")
		}
		if r.Limit < 0 {
			return fmt.Errorf("limit must be >= 0")
		}
		if r.PreviewLines < 0 {
			return fmt.Errorf("previewLines must be >= 0")
		}
		if r.MaxSizeBytes < 0 {
			return fmt.Errorf("maxSizeBytes must be >= 0")
		}
		return nil

	case HostOpFSWrite:
		if err := validate.NonEmpty("path", r.Path); err != nil {
			return err
		}
		if !strings.HasPrefix(strings.TrimSpace(r.Path), "/") {
			return fmt.Errorf("path must be an absolute VFS path (start with /)")
		}
		if err := validateWriteChecksum(r.Checksum); err != nil {
			return err
		}
		if err := validateWriteChecksumExpected(r.Checksum, r.ChecksumExpected); err != nil {
			return err
		}
		if err := validateWriteMode(r.Mode); err != nil {
			return err
		}

		if strings.HasPrefix(strings.TrimSpace(r.Path), "/memory/") {
			if err := validateMemoryWritePath(r.Path); err != nil {
				return err
			}
		}
		return nil

	case HostOpFSAppend:
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

	case HostOpFSBatchEdit:
		if err := validate.NonEmpty("path", r.Path); err != nil {
			return err
		}
		if !strings.HasPrefix(strings.TrimSpace(r.Path), "/") {
			return fmt.Errorf("path must be an absolute VFS path (start with /)")
		}
		if err := validate.NonEmpty("glob", r.Glob); err != nil {
			return err
		}
		if len(r.BatchEditEdits) == 0 {
			return fmt.Errorf("batchEditEdits must be non-empty")
		}
		for i, edit := range r.BatchEditEdits {
			if err := validateBatchEdit(i, edit); err != nil {
				return err
			}
		}
		if r.BatchEditOptions != nil {
			if r.BatchEditOptions.Apply && r.BatchEditOptions.DryRun {
				return fmt.Errorf("batchEditOptions.apply and batchEditOptions.dryRun cannot both be true")
			}
			if r.BatchEditOptions.MaxFiles < 0 {
				return fmt.Errorf("batchEditOptions.maxFiles must be >= 0")
			}
		}
		return nil

	case HostOpFSTxn:
		if len(r.TxnSteps) == 0 {
			return fmt.Errorf("txnSteps is required")
		}
		for i, step := range r.TxnSteps {
			if err := validateTxnStep(i, step); err != nil {
				return err
			}
		}
		if r.TxnOptions != nil {
			if r.TxnOptions.Apply && r.TxnOptions.DryRun {
				return fmt.Errorf("txnOptions.apply and txnOptions.dryRun cannot both be true")
			}
		}
		return nil

	case HostOpFSArchiveCreate:
		if err := validate.NonEmpty("path", r.Path); err != nil {
			return err
		}
		if !strings.HasPrefix(strings.TrimSpace(r.Path), "/") {
			return fmt.Errorf("path must be an absolute VFS path (start with /)")
		}
		if err := validate.NonEmpty("destination", r.Destination); err != nil {
			return err
		}
		if !strings.HasPrefix(strings.TrimSpace(r.Destination), "/") {
			return fmt.Errorf("destination must be an absolute VFS path (start with /)")
		}
		if err := validateArchiveFormat(r.Format); err != nil {
			return err
		}
		return nil

	case HostOpFSArchiveExtract:
		if err := validate.NonEmpty("path", r.Path); err != nil {
			return err
		}
		if !strings.HasPrefix(strings.TrimSpace(r.Path), "/") {
			return fmt.Errorf("path must be an absolute VFS path (start with /)")
		}
		if err := validate.NonEmpty("destination", r.Destination); err != nil {
			return err
		}
		if !strings.HasPrefix(strings.TrimSpace(r.Destination), "/") {
			return fmt.Errorf("destination must be an absolute VFS path (start with /)")
		}
		return nil

	case HostOpFSArchiveList:
		if err := validate.NonEmpty("path", r.Path); err != nil {
			return err
		}
		if !strings.HasPrefix(strings.TrimSpace(r.Path), "/") {
			return fmt.Errorf("path must be an absolute VFS path (start with /)")
		}
		if r.Limit < 0 {
			return fmt.Errorf("limit must be >= 0")
		}
		return nil

	case HostOpShellExec:
		if len(r.Argv) == 0 {
			return fmt.Errorf("argv is required")
		}
		return nil

	case HostOpPipe:
		if len(r.PipeSteps) == 0 {
			return fmt.Errorf("pipeSteps is required")
		}
		maxSteps := 16
		maxValueBytes := 65536
		if r.PipeOptions != nil {
			if r.PipeOptions.MaxSteps != 0 {
				maxSteps = r.PipeOptions.MaxSteps
			}
			if r.PipeOptions.MaxValueBytes != 0 {
				maxValueBytes = r.PipeOptions.MaxValueBytes
			}
		}
		if maxSteps <= 0 {
			return fmt.Errorf("pipeOptions.maxSteps must be >= 1")
		}
		if maxValueBytes <= 0 {
			return fmt.Errorf("pipeOptions.maxValueBytes must be >= 1")
		}
		if len(r.PipeSteps) > maxSteps {
			return fmt.Errorf("pipeSteps exceeds maxSteps (%d)", maxSteps)
		}
		for i, step := range r.PipeSteps {
			if err := validatePipeStep(i, step); err != nil {
				return err
			}
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

func validateWriteChecksum(checksum string) error {
	checksum = checksumutil.NormalizeAlgorithm(checksum)
	if checksum == "" || checksumutil.IsSupportedAlgorithm(checksum) {
		return nil
	}
	return fmt.Errorf("checksum must be one of %s", checksumutil.SupportedAlgorithmsDisplay())
}

func validateWriteChecksumExpected(algo, expected string) error {
	algo = checksumutil.NormalizeAlgorithm(algo)
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return nil
	}
	if algo == "" {
		return fmt.Errorf("checksum is required when checksumExpected is set")
	}
	wantLen, ok := checksumutil.HexLength(algo)
	if !ok {
		return fmt.Errorf("checksum must be one of %s", checksumutil.SupportedAlgorithmsDisplay())
	}
	if len(expected) != wantLen {
		return fmt.Errorf("checksumExpected for %s must be %d hex chars", algo, wantLen)
	}
	for _, ch := range expected {
		isDigit := ch >= '0' && ch <= '9'
		isLowerHex := ch >= 'a' && ch <= 'f'
		isUpperHex := ch >= 'A' && ch <= 'F'
		if !isDigit && !isLowerHex && !isUpperHex {
			return fmt.Errorf("checksumExpected for %s must be hex", algo)
		}
	}
	return nil
}

func validateReadChecksums(single string, many []string) error {
	if s := checksumutil.NormalizeAlgorithm(single); s != "" && !checksumutil.IsSupportedAlgorithm(s) {
		return fmt.Errorf("checksum must be one of %s", checksumutil.SupportedAlgorithmsDisplay())
	}
	for _, raw := range many {
		algo := checksumutil.NormalizeAlgorithm(raw)
		if algo == "" {
			return fmt.Errorf("checksums entries must be non-empty")
		}
		if !checksumutil.IsSupportedAlgorithm(algo) {
			return fmt.Errorf("checksums entries must be one of %s", checksumutil.SupportedAlgorithmsDisplay())
		}
	}
	return nil
}

func validateWriteMode(mode string) error {
	switch normalizeWriteMode(mode) {
	case "w", "a":
		return nil
	default:
		return fmt.Errorf("mode must be one of w|a")
	}
}

func normalizeWriteMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "", "w", "overwrite":
		return "w"
	case "a", "append":
		return "a"
	default:
		return mode
	}
}

func validateArchiveFormat(format string) error {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "zip", "tar", "tar.gz":
		return nil
	default:
		return fmt.Errorf("format must be one of zip|tar|tar.gz")
	}
}

// SearchResult is one result returned by fs_search.
type SearchResult struct {
	Title         string   `json:"title,omitempty"`
	Path          string   `json:"path,omitempty"`
	Snippet       string   `json:"snippet,omitempty"`
	Score         float64  `json:"score,omitempty"`
	PreviewBefore []string `json:"previewBefore,omitempty"`
	PreviewMatch  string   `json:"previewMatch,omitempty"`
	PreviewAfter  []string `json:"previewAfter,omitempty"`
	SizeBytes     *int64   `json:"sizeBytes,omitempty"`
	Mtime         *int64   `json:"mtime,omitempty"`
}

// FSTxnStep describes one fs_* mutating step inside fs_txn.
type FSTxnStep struct {
	Op               string          `json:"op,omitempty"`
	Path             string          `json:"path,omitempty"`
	Text             string          `json:"text,omitempty"`
	Input            json.RawMessage `json:"input,omitempty"`
	Mode             string          `json:"mode,omitempty"`
	Verify           bool            `json:"verify,omitempty"`
	Checksum         string          `json:"checksum,omitempty"`
	ChecksumExpected string          `json:"checksumExpected,omitempty"`
	Atomic           bool            `json:"atomic,omitempty"`
	Sync             bool            `json:"sync,omitempty"`
	Verbose          bool            `json:"verbose,omitempty"`
}

// BatchEdit describes one exact-match replacement for fs_batch_edit.
type BatchEdit struct {
	Old        string `json:"old,omitempty"`
	New        string `json:"new,omitempty"`
	Occurrence string `json:"occurrence,omitempty"` // all|1|2|...
}

// BatchOptions controls dry-run/apply and safety behavior for fs_batch_edit.
type BatchOptions struct {
	DryRun          bool `json:"dryRun,omitempty"`
	Apply           bool `json:"apply,omitempty"`
	RollbackOnError bool `json:"rollbackOnError,omitempty"`
	MaxFiles        int  `json:"maxFiles,omitempty"`
}

// BatchEditResult captures one file outcome for fs_batch_edit.
type BatchEditResult struct {
	Path         string `json:"path,omitempty"`
	Ok           bool   `json:"ok"`
	Changed      bool   `json:"changed,omitempty"`
	EditsApplied int    `json:"editsApplied,omitempty"`
	Reason       string `json:"reason,omitempty"`
	Error        string `json:"error,omitempty"`
}

// PipeStep describes one declarative pipeline step.
type PipeStep struct {
	Type        string         `json:"type,omitempty"` // tool|transform
	Tool        string         `json:"tool,omitempty"`
	Args        map[string]any `json:"args,omitempty"`
	InputArg    string         `json:"inputArg,omitempty"`
	Output      string         `json:"output,omitempty"`
	Transform   string         `json:"transform,omitempty"`
	Field       string         `json:"field,omitempty"`
	Separator   string         `json:"separator,omitempty"`
	Pattern     string         `json:"pattern,omitempty"`
	Replacement string         `json:"replacement,omitempty"`
}

// PipeOptions controls debug mode and limits for pipe.
type PipeOptions struct {
	Debug         bool `json:"debug,omitempty"`
	MaxSteps      int  `json:"maxSteps,omitempty"`
	MaxValueBytes int  `json:"maxValueBytes,omitempty"`
}

// PipeStepResult summarizes one executed pipeline step.
type PipeStepResult struct {
	Index         int    `json:"index,omitempty"`
	Type          string `json:"type,omitempty"`
	Name          string `json:"name,omitempty"`
	DurationMs    int64  `json:"durationMs,omitempty"`
	OutputType    string `json:"outputType,omitempty"`
	OutputPreview string `json:"outputPreview,omitempty"`
	Error         string `json:"error,omitempty"`
}

// FSTxnOptions controls transaction mode and rollback behavior.
type FSTxnOptions struct {
	DryRun          bool `json:"dryRun,omitempty"`
	Apply           bool `json:"apply,omitempty"`
	RollbackOnError bool `json:"rollbackOnError,omitempty"`
}

// FSTxnStepResult captures one step result.
type FSTxnStepResult struct {
	Index            int               `json:"index,omitempty"`
	Op               string            `json:"op,omitempty"`
	Path             string            `json:"path,omitempty"`
	Ok               bool              `json:"ok"`
	Error            string            `json:"error,omitempty"`
	WriteMode        string            `json:"writeMode,omitempty"`
	WriteBytes       *int64            `json:"writeBytes,omitempty"`
	PatchDiagnostics *PatchDiagnostics `json:"patchDiagnostics,omitempty"`
}

// FSTxnDiagnostics summarizes transaction outcome and rollback.
type FSTxnDiagnostics struct {
	StepsTotal        int      `json:"stepsTotal,omitempty"`
	StepsApplied      int      `json:"stepsApplied,omitempty"`
	FailedStep        int      `json:"failedStep,omitempty"`
	ApplyMode         string   `json:"applyMode,omitempty"` // dry_run|apply
	RollbackPerformed bool     `json:"rollbackPerformed,omitempty"`
	RollbackFailed    bool     `json:"rollbackFailed,omitempty"`
	RollbackErrors    []string `json:"rollbackErrors,omitempty"`
}

// SearchRequest describes a structured filesystem search request.
type SearchRequest struct {
	Query           string   `json:"query,omitempty"`
	Pattern         string   `json:"pattern,omitempty"`
	Limit           int      `json:"limit,omitempty"`
	Glob            string   `json:"glob,omitempty"`
	Exclude         []string `json:"exclude,omitempty"`
	PreviewLines    int      `json:"previewLines,omitempty"`
	IncludeMetadata bool     `json:"includeMetadata,omitempty"`
	MaxSizeBytes    int64    `json:"maxSizeBytes,omitempty"`
}

// SearchResponse captures structured fs_search results and truncation metadata.
type SearchResponse struct {
	Results   []SearchResult `json:"results,omitempty"`
	Total     int            `json:"total,omitempty"`
	Returned  int            `json:"returned,omitempty"`
	Truncated bool           `json:"truncated,omitempty"`
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

// ArchiveEntry represents one entry in an archive file.
type ArchiveEntry struct {
	Name      string `json:"name,omitempty"`
	IsDir     bool   `json:"isDir,omitempty"`
	SizeBytes int64  `json:"sizeBytes,omitempty"`
	Mtime     *int64 `json:"mtime,omitempty"`
}

// HostOpResponse is the minimal "host primitive" response envelope.
type HostOpResponse struct {
	Op                         string            `json:"op"`
	Ok                         bool              `json:"ok"`
	Error                      string            `json:"error,omitempty"`
	ErrorCode                  string            `json:"errorCode,omitempty"`
	Entries                    []string          `json:"entries,omitempty"`
	Results                    []SearchResult    `json:"results,omitempty"`
	ArchiveEntries             []ArchiveEntry    `json:"archiveEntries,omitempty"`
	BatchEditDetails           []BatchEditResult `json:"details,omitempty"`
	PipeStepResults            []PipeStepResult  `json:"steps,omitempty"`
	TxnStepResults             []FSTxnStepResult `json:"txnStepResults,omitempty"`
	TxnDiagnostics             *FSTxnDiagnostics `json:"txnDiagnostics,omitempty"`
	ArchiveFormat              string            `json:"archiveFormat,omitempty"`
	FilesAdded                 int               `json:"filesAdded,omitempty"`
	FilesExtracted             int               `json:"filesExtracted,omitempty"`
	TotalSizeBytes             int64             `json:"totalSizeBytes,omitempty"`
	ArchiveSizeBytes           int64             `json:"archiveSizeBytes,omitempty"`
	CompressionRatio           float64           `json:"compressionRatio,omitempty"`
	Skipped                    []string          `json:"skipped,omitempty"`
	MatchedFiles               int               `json:"matchedFiles,omitempty"`
	ModifiedFiles              int               `json:"modifiedFiles,omitempty"`
	FailedFiles                int               `json:"failedFiles,omitempty"`
	SkippedFiles               int               `json:"skippedFiles,omitempty"`
	BatchEditDryRun            bool              `json:"batchEditDryRun,omitempty"`
	BatchEditApplied           bool              `json:"batchEditApplied,omitempty"`
	BatchEditRollbackPerformed bool              `json:"batchEditRollbackPerformed,omitempty"`
	BatchEditRollbackFailed    bool              `json:"batchEditRollbackFailed,omitempty"`
	PipeFailedAtStep           int               `json:"failedAtStep,omitempty"`
	PipeValue                  json.RawMessage   `json:"value,omitempty"`
	PipeDebug                  bool              `json:"pipeDebug,omitempty"`
	ResultsTotal               int               `json:"resultsTotal,omitempty"`
	ResultsReturned            int               `json:"resultsReturned,omitempty"`
	ResultsTruncated           bool              `json:"resultsTruncated,omitempty"`
	Exists                     *bool             `json:"exists,omitempty"`
	IsDir                      *bool             `json:"isDir,omitempty"`
	SizeBytes                  *int64            `json:"sizeBytes,omitempty"`
	Mtime                      *int64            `json:"mtime,omitempty"`
	// fs_read checksum metadata.
	ReadChecksums map[string]string `json:"readChecksums,omitempty"`
	// Patch diagnostics are emitted for fs_patch success/failure and dry-run validation.
	PatchDiagnostics *PatchDiagnostics `json:"patchDiagnostics,omitempty"`
	PatchDryRun      bool              `json:"patchDryRun,omitempty"`
	// fs_write verification/checksum metadata.
	WriteVerified         *bool  `json:"writeVerified,omitempty"`
	WriteChecksumMatch    *bool  `json:"writeChecksumMatch,omitempty"`
	WriteChecksumAlgo     string `json:"writeChecksumAlgo,omitempty"`
	WriteChecksum         string `json:"writeChecksum,omitempty"`
	WriteChecksumExpected string `json:"writeChecksumExpected,omitempty"`
	WriteRequestMode      string `json:"writeRequestMode,omitempty"` // w|a
	WriteMode             string `json:"writeMode,omitempty"`        // created|overwritten
	WriteBytes            *int64 `json:"writeBytes,omitempty"`
	WriteFinalSize        *int64 `json:"writeFinalSize,omitempty"`
	WriteAtomicRequested  bool   `json:"writeAtomicRequested,omitempty"`
	WriteSyncRequested    bool   `json:"writeSyncRequested,omitempty"`
	WriteMismatchAt       *int64 `json:"writeMismatchAt,omitempty"`
	WriteExpectedBytes    *int64 `json:"writeExpectedBytes,omitempty"`
	WriteActualBytes      *int64 `json:"writeActualBytes,omitempty"`
	BytesLen              int    `json:"bytesLen,omitempty"`
	Text                  string `json:"text,omitempty"`
	BytesB64              string `json:"bytesB64,omitempty"`
	Truncated             bool   `json:"truncated,omitempty"`
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

func validateTxnStep(index int, step FSTxnStep) error {
	op := strings.ToLower(strings.TrimSpace(step.Op))
	switch op {
	case HostOpFSWrite, HostOpFSAppend, HostOpFSEdit, HostOpFSPatch:
	default:
		return fmt.Errorf("txnSteps[%d].op must be one of fs_write|fs_append|fs_edit|fs_patch", index)
	}
	if err := validate.NonEmpty(fmt.Sprintf("txnSteps[%d].path", index), step.Path); err != nil {
		return err
	}
	if !strings.HasPrefix(strings.TrimSpace(step.Path), "/") {
		return fmt.Errorf("txnSteps[%d].path must be an absolute VFS path (start with /)", index)
	}

	switch op {
	case HostOpFSWrite:
		if err := validateWriteChecksum(step.Checksum); err != nil {
			return fmt.Errorf("txnSteps[%d]: %w", index, err)
		}
		if err := validateWriteChecksumExpected(step.Checksum, step.ChecksumExpected); err != nil {
			return fmt.Errorf("txnSteps[%d]: %w", index, err)
		}
		if err := validateWriteMode(step.Mode); err != nil {
			return fmt.Errorf("txnSteps[%d]: %w", index, err)
		}
	case HostOpFSAppend:
		if err := validate.NonEmpty(fmt.Sprintf("txnSteps[%d].text", index), step.Text); err != nil {
			return err
		}
	case HostOpFSEdit:
		if len(step.Input) == 0 {
			return fmt.Errorf("txnSteps[%d].input is required", index)
		}
	case HostOpFSPatch:
		if err := validate.NonEmpty(fmt.Sprintf("txnSteps[%d].text", index), step.Text); err != nil {
			return err
		}
	}
	return nil
}

func validateBatchEdit(index int, edit BatchEdit) error {
	if err := validate.NonEmpty(fmt.Sprintf("batchEditEdits[%d].old", index), edit.Old); err != nil {
		return err
	}
	occurrence := strings.TrimSpace(edit.Occurrence)
	if occurrence == "" || strings.EqualFold(occurrence, "all") {
		return nil
	}
	n, err := strconv.Atoi(occurrence)
	if err != nil || n <= 0 {
		return fmt.Errorf("batchEditEdits[%d].occurrence must be \"all\" or an integer >= 1", index)
	}
	return nil
}

func validatePipeStep(index int, step PipeStep) error {
	stepType := strings.ToLower(strings.TrimSpace(step.Type))
	switch stepType {
	case "tool":
		switch strings.ToLower(strings.TrimSpace(step.Tool)) {
		case HostOpFSRead, HostOpFSWrite, HostOpFSAppend, HostOpFSSearch, HostOpFSStat, HostOpHTTPFetch, HostOpShellExec, HostOpEmail:
		default:
			return fmt.Errorf("pipeSteps[%d].tool is not supported in pipe v1", index)
		}
		if strings.TrimSpace(step.Transform) != "" {
			return fmt.Errorf("pipeSteps[%d].transform must be empty for tool steps", index)
		}
		if len(step.Args) == 0 && strings.TrimSpace(step.InputArg) == "" {
			return fmt.Errorf("pipeSteps[%d].args is required for tool steps", index)
		}
	case "transform":
		switch strings.ToLower(strings.TrimSpace(step.Transform)) {
		case "uppercase", "lowercase", "trim", "json_parse", "json_stringify", "get", "join", "split", "regex_replace":
		default:
			return fmt.Errorf("pipeSteps[%d].transform is not supported in pipe v1", index)
		}
		if strings.TrimSpace(step.Tool) != "" {
			return fmt.Errorf("pipeSteps[%d].tool must be empty for transform steps", index)
		}
		switch strings.ToLower(strings.TrimSpace(step.Transform)) {
		case "get":
			if err := validate.NonEmpty(fmt.Sprintf("pipeSteps[%d].field", index), step.Field); err != nil {
				return err
			}
		case "regex_replace":
			if err := validate.NonEmpty(fmt.Sprintf("pipeSteps[%d].pattern", index), step.Pattern); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("pipeSteps[%d].type must be tool or transform", index)
	}
	output := strings.TrimSpace(step.Output)
	if strings.Contains(output, "[") || strings.Contains(output, "]") {
		return fmt.Errorf("pipeSteps[%d].output does not support array indexing in v1", index)
	}
	if strings.TrimSpace(step.InputArg) == "." {
		return fmt.Errorf("pipeSteps[%d].inputArg is invalid", index)
	}
	return nil
}
