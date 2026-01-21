package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/vfsutil"
)

var builtinLintManifest = []byte(`{
  "id": "builtin.lint",
  "version": "0.1.0",
  "kind": "builtin",
  "displayName": "Builtin Lint",
  "description": "Detects and runs common linters/formatters with bounded stdout/stderr output.",
  "actions": [
    {
      "id": "detect",
      "displayName": "Detect linters/formatters",
      "description": "Detect available linters/formatters based on project markers and binaries on PATH.",
      "inputSchema": {
        "type": "object",
        "properties": { "cwd": { "type": "string" } }
      },
      "outputSchema": {
        "type": "object",
        "properties": {
          "linters": { "type": "array", "items": { "type": "string" } },
          "formatters": { "type": "array", "items": { "type": "string" } }
        },
        "required": ["linters","formatters"]
      }
    },
    {
      "id": "run",
      "displayName": "Run linter",
      "description": "Run the best available linter for the project (best-effort).",
      "inputSchema": {
        "type": "object",
        "properties": {
          "cwd": { "type": "string" },
          "fix": { "type": "boolean" }
        }
      },
      "outputSchema": {
        "type": "object",
        "properties": {
          "tool": { "type": "string" },
          "exitCode": { "type": "integer" },
          "stdout": { "type": "string" },
          "stderr": { "type": "string" },
          "stdoutPath": { "type": "string" },
          "stderrPath": { "type": "string" }
        },
        "required": ["tool","exitCode","stdout","stderr"]
      }
    },
    {
      "id": "format",
      "displayName": "Format code",
      "description": "Run the best available formatter for the project (best-effort).",
      "inputSchema": {
        "type": "object",
        "properties": {
          "cwd": { "type": "string" },
          "files": { "type": "array", "items": { "type": "string" } }
        }
      },
      "outputSchema": {
        "type": "object",
        "properties": {
          "tool": { "type": "string" },
          "exitCode": { "type": "integer" },
          "stdout": { "type": "string" },
          "stderr": { "type": "string" },
          "stdoutPath": { "type": "string" },
          "stderrPath": { "type": "string" },
          "changedFiles": { "type": "array", "items": { "type": "string" } }
        },
        "required": ["tool","exitCode","stdout","stderr","changedFiles"]
      }
    }
  ]
}`)

func init() {
	registerBuiltin(BuiltinDef{
		ID:       types.ToolID("builtin.lint"),
		Manifest: builtinLintManifest,
		NewInvoker: func(cfg BuiltinConfig) ToolInvoker {
			return NewBuiltinLintInvoker(cfg.ShellRootDir)
		},
	})
}

const (
	defaultLintMaxOutputBytes = 64 * 1024
)

type BuiltinLintInvoker struct {
	RootDir  string
	MaxBytes int
}

func NewBuiltinLintInvoker(rootDir string) *BuiltinLintInvoker {
	return &BuiltinLintInvoker{RootDir: rootDir, MaxBytes: defaultLintMaxOutputBytes}
}

type lintDetectInput struct {
	Cwd string `json:"cwd,omitempty"`
}

type lintDetectOutput struct {
	Linters    []string `json:"linters"`
	Formatters []string `json:"formatters"`
}

type lintRunInput struct {
	Cwd string `json:"cwd,omitempty"`
	Fix bool   `json:"fix,omitempty"`
}

type lintRunOutput struct {
	Tool     string `json:"tool"`
	ExitCode int    `json:"exitCode"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`

	StdoutPath string `json:"stdoutPath,omitempty"`
	StderrPath string `json:"stderrPath,omitempty"`
}

type lintFormatInput struct {
	Cwd   string   `json:"cwd,omitempty"`
	Files []string `json:"files,omitempty"`
}

type lintFormatOutput struct {
	Tool     string   `json:"tool"`
	ExitCode int      `json:"exitCode"`
	Stdout   string   `json:"stdout"`
	Stderr   string   `json:"stderr"`
	Changed  []string `json:"changedFiles"`

	StdoutPath string `json:"stdoutPath,omitempty"`
	StderrPath string `json:"stderrPath,omitempty"`
}

func (l *BuiltinLintInvoker) Invoke(ctx context.Context, req types.ToolRequest) (ToolCallResult, error) {
	if l == nil {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: "builtin.lint invoker is nil"}
	}
	if err := req.Validate(); err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: err.Error()}
	}

	// Safety: if callers omit timeoutMs and the context has no deadline, apply a default.
	if req.TimeoutMs == 0 {
		if _, hasDeadline := ctx.Deadline(); !hasDeadline {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, 5*time.Minute)
			defer cancel()
		}
	}

	switch req.ActionID {
	case "detect":
		return l.detect(ctx, req)
	case "run":
		return l.run(ctx, req)
	case "format":
		return l.format(ctx, req)
	default:
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("unsupported action %q (allowed: detect, run, format)", req.ActionID)}
	}
}

func (l *BuiltinLintInvoker) rootAndDir(cwd string) (root, absDir string, err error) {
	root = strings.TrimSpace(l.RootDir)
	if root == "" {
		return "", "", fmt.Errorf("rootDir is required")
	}
	if !filepath.IsAbs(root) {
		return "", "", fmt.Errorf("rootDir must be absolute, got %q", root)
	}

	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		cwd = "."
	}
	absDir, err = vfsutil.SafeJoinBaseDir(root, cwd)
	if err != nil {
		return "", "", err
	}
	return root, absDir, nil
}

func (l *BuiltinLintInvoker) maxBytes() int {
	if l.MaxBytes <= 0 {
		return defaultLintMaxOutputBytes
	}
	return l.MaxBytes
}

func (l *BuiltinLintInvoker) detect(_ context.Context, req types.ToolRequest) (ToolCallResult, error) {
	var in lintDetectInput
	if err := json.Unmarshal(req.Input, &in); err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("invalid input JSON: %v", err)}
	}
	_, absDir, err := l.rootAndDir(in.Cwd)
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: err.Error()}
	}

	linters, formatters := detectLintersAndFormatters(absDir)
	outJSON, err := json.Marshal(lintDetectOutput{Linters: linters, Formatters: formatters})
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: fmt.Sprintf("marshal output: %v", err), Err: err}
	}
	return ToolCallResult{Output: outJSON}, nil
}

func detectLintersAndFormatters(absDir string) (linters []string, formatters []string) {
	exists := func(name string) bool {
		_, err := os.Stat(filepath.Join(absDir, name))
		return err == nil
	}
	hasBin := func(name string) bool { _, err := exec.LookPath(name); return err == nil }

	linters = []string{}
	formatters = []string{}

	// Go.
	if exists("go.mod") {
		if hasBin("golangci-lint") {
			linters = append(linters, "golangci-lint")
		}
		// gofmt is always a formatter when Go is installed.
		if hasBin("gofmt") {
			formatters = append(formatters, "gofmt")
		}
	}

	// Node.
	if exists("package.json") {
		if hasBin("eslint") {
			linters = append(linters, "eslint")
		}
	}

	// Python.
	if exists("pyproject.toml") || exists("pytest.ini") {
		if hasBin("ruff") {
			linters = append(linters, "ruff")
			// ruff format exists in newer versions; treat as formatter too.
			formatters = append(formatters, "ruff")
		}
		if hasBin("black") {
			formatters = append(formatters, "black")
		}
	}

	return uniqueStrings(linters), uniqueStrings(formatters)
}

func (l *BuiltinLintInvoker) run(ctx context.Context, req types.ToolRequest) (ToolCallResult, error) {
	var in lintRunInput
	if err := json.Unmarshal(req.Input, &in); err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("invalid input JSON: %v", err)}
	}
	_, absDir, err := l.rootAndDir(in.Cwd)
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: err.Error()}
	}

	tool, argv, err := selectLintCommand(absDir, in.Fix)
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: err.Error()}
	}

	exitCode, stdoutFull, stderrFull, err := runCmdBounded(ctx, absDir, argv)
	if err != nil {
		return ToolCallResult{}, err
	}

	maxBytes := l.maxBytes()
	stdoutPreview, stdoutArtifact, stdoutTruncated := capBytesToTextArtifact("stdout.txt", stdoutFull, maxBytes)
	stderrPreview, stderrArtifact, stderrTruncated := capBytesToTextArtifact("stderr.txt", stderrFull, maxBytes)

	out := lintRunOutput{Tool: tool, ExitCode: exitCode, Stdout: stdoutPreview, Stderr: stderrPreview}
	artifacts := make([]ToolArtifactWrite, 0, 2)
	if stdoutTruncated {
		out.StdoutPath = "stdout.txt"
		artifacts = append(artifacts, *stdoutArtifact)
	}
	if stderrTruncated {
		out.StderrPath = "stderr.txt"
		artifacts = append(artifacts, *stderrArtifact)
	}

	outJSON, err := json.Marshal(out)
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: fmt.Sprintf("marshal output: %v", err), Err: err}
	}
	return ToolCallResult{Output: outJSON, Artifacts: artifacts}, nil
}

func selectLintCommand(absDir string, fix bool) (tool string, argv []string, err error) {
	linters, _ := detectLintersAndFormatters(absDir)
	has := func(name string) bool { return sliceContains(linters, name) }

	// Prefer golangci-lint for Go modules.
	if has("golangci-lint") {
		argv = []string{"golangci-lint", "run"}
		if fix {
			// golangci-lint supports --fix for some linters.
			argv = append(argv, "--fix")
		}
		return "golangci-lint", argv, nil
	}
	if has("ruff") {
		argv = []string{"ruff", "check", "."}
		if fix {
			argv = []string{"ruff", "check", ".", "--fix"}
		}
		return "ruff", argv, nil
	}
	if has("eslint") {
		argv = []string{"eslint", "."}
		if fix {
			argv = []string{"eslint", ".", "--fix"}
		}
		return "eslint", argv, nil
	}

	return "", nil, fmt.Errorf("no supported linter detected")
}

func (l *BuiltinLintInvoker) format(ctx context.Context, req types.ToolRequest) (ToolCallResult, error) {
	var in lintFormatInput
	if err := json.Unmarshal(req.Input, &in); err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("invalid input JSON: %v", err)}
	}
	root, absDir, err := l.rootAndDir(in.Cwd)
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: err.Error()}
	}

	tool, argv, absFiles, relFiles, err := selectFormatCommand(root, absDir, in.Files)
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: err.Error()}
	}

	before := make(map[string][]byte, len(absFiles))
	for i, af := range absFiles {
		if af == "" {
			continue
		}
		b, err := os.ReadFile(af)
		if err != nil {
			// Ignore non-readable files; formatter will report errors if needed.
			continue
		}
		before[relFiles[i]] = b
	}

	exitCode, stdoutFull, stderrFull, err := runCmdBounded(ctx, absDir, argv)
	if err != nil {
		return ToolCallResult{}, err
	}

	changed := make([]string, 0)
	for _, rf := range relFiles {
		if rf == "" {
			continue
		}
		af := filepath.Join(root, filepath.FromSlash(rf))
		after, err := os.ReadFile(af)
		if err != nil {
			continue
		}
		if b0, ok := before[rf]; ok {
			if !bytes.Equal(b0, after) {
				changed = append(changed, rf)
			}
		}
	}

	maxBytes := l.maxBytes()
	stdoutPreview, stdoutArtifact, stdoutTruncated := capBytesToTextArtifact("stdout.txt", stdoutFull, maxBytes)
	stderrPreview, stderrArtifact, stderrTruncated := capBytesToTextArtifact("stderr.txt", stderrFull, maxBytes)

	out := lintFormatOutput{
		Tool:     tool,
		ExitCode: exitCode,
		Stdout:   stdoutPreview,
		Stderr:   stderrPreview,
		Changed:  uniqueStrings(changed),
	}
	artifacts := make([]ToolArtifactWrite, 0, 2)
	if stdoutTruncated {
		out.StdoutPath = "stdout.txt"
		artifacts = append(artifacts, *stdoutArtifact)
	}
	if stderrTruncated {
		out.StderrPath = "stderr.txt"
		artifacts = append(artifacts, *stderrArtifact)
	}

	outJSON, err := json.Marshal(out)
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: fmt.Sprintf("marshal output: %v", err), Err: err}
	}
	return ToolCallResult{Output: outJSON, Artifacts: artifacts}, nil
}

func selectFormatCommand(rootDir, absDir string, files []string) (tool string, argv []string, absFiles []string, relFiles []string, err error) {
	_, formatters := detectLintersAndFormatters(absDir)
	has := func(name string) bool { return sliceContains(formatters, name) }

	absFiles, relFiles, err = normalizeFileList(rootDir, absDir, files)
	if err != nil {
		return "", nil, nil, nil, err
	}

	// Prefer gofmt for Go modules.
	if has("gofmt") {
		tool = "gofmt"
		argv = []string{"gofmt", "-w"}
		if len(relFiles) == 0 {
			argv = append(argv, ".")
			return tool, argv, []string{}, []string{}, nil
		}
		argv = append(argv, relFiles...)
		return tool, argv, absFiles, relFiles, nil
	}

	// Prefer black over ruff formatting when available.
	if has("black") {
		tool = "black"
		argv = []string{"black"}
		if len(relFiles) == 0 {
			argv = append(argv, ".")
			return tool, argv, []string{}, []string{}, nil
		}
		argv = append(argv, relFiles...)
		return tool, argv, absFiles, relFiles, nil
	}

	if has("ruff") {
		tool = "ruff"
		argv = []string{"ruff", "format"}
		if len(relFiles) == 0 {
			argv = append(argv, ".")
			return tool, argv, []string{}, []string{}, nil
		}
		argv = append(argv, relFiles...)
		return tool, argv, absFiles, relFiles, nil
	}

	return "", nil, nil, nil, fmt.Errorf("no supported formatter detected")
}

func normalizeFileList(rootDir, absDir string, files []string) (absFiles []string, relFiles []string, err error) {
	if len(files) == 0 {
		return []string{}, []string{}, nil
	}

	relFiles = make([]string, 0, len(files))
	absFiles = make([]string, 0, len(files))
	for _, f := range files {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		clean, err := vfsutil.CleanRelPath(f)
		if err != nil || clean == "." {
			return nil, nil, fmt.Errorf("invalid file path %q", f)
		}
		abs, err := vfsutil.SafeJoinBaseDir(rootDir, clean)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid file path %q", f)
		}
		// Ensure the file is under the requested cwd, too.
		if _, err := vfsutil.RelUnderBaseDir(absDir, abs); err != nil {
			return nil, nil, fmt.Errorf("file %q is not under cwd", f)
		}
		relFiles = append(relFiles, clean)
		absFiles = append(absFiles, abs)
	}
	return absFiles, relFiles, nil
}

func runCmdBounded(ctx context.Context, absDir string, argv []string) (exitCode int, stdout, stderr []byte, err error) {
	if len(argv) == 0 {
		return 0, nil, nil, &InvokeError{Code: "invalid_input", Message: "argv is required"}
	}
	if _, lookErr := exec.LookPath(argv[0]); lookErr != nil {
		return 0, nil, nil, &InvokeError{Code: "tool_failed", Message: fmt.Sprintf("%s binary not found", argv[0]), Err: lookErr}
	}

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = absDir

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return 0, nil, nil, &InvokeError{Code: "tool_failed", Message: fmt.Sprintf("stdout pipe: %v", err), Err: err}
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return 0, nil, nil, &InvokeError{Code: "tool_failed", Message: fmt.Sprintf("stderr pipe: %v", err), Err: err}
	}

	if err := cmd.Start(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return 0, nil, nil, &InvokeError{Code: "timeout", Message: "command timed out", Retryable: true, Err: err}
		}
		return 0, nil, nil, &InvokeError{Code: "tool_failed", Message: err.Error(), Err: err}
	}

	// Drain both streams to avoid deadlocks.
	type rr struct {
		b   []byte
		err error
	}
	chOut := make(chan rr, 1)
	chErr := make(chan rr, 1)
	go func() {
		b, e := io.ReadAll(stdoutPipe)
		chOut <- rr{b: b, err: e}
	}()
	go func() {
		b, e := io.ReadAll(stderrPipe)
		chErr <- rr{b: b, err: e}
	}()

	waitErr := cmd.Wait()
	outRes := <-chOut
	errRes := <-chErr
	if outRes.err != nil && errRes.err == nil {
		errRes.err = outRes.err
	}

	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(waitErr, context.DeadlineExceeded) {
		return 0, nil, nil, &InvokeError{Code: "timeout", Message: "command timed out", Retryable: true, Err: waitErr}
	}

	exitCode = 0
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			return 0, outRes.b, errRes.b, &InvokeError{Code: "tool_failed", Message: waitErr.Error(), Err: waitErr}
		}
	}

	return exitCode, outRes.b, errRes.b, nil
}

func capBytesToTextArtifact(path string, b []byte, maxBytes int) (preview string, artifact *ToolArtifactWrite, truncated bool) {
	s := string(b)
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s, nil, false
	}
	return s[:maxBytes], &ToolArtifactWrite{Path: path, Bytes: append([]byte(nil), b...), MediaType: "text/plain"}, true
}

// sliceContains is a small helper local to this file to avoid coupling to tests.
func sliceContains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
