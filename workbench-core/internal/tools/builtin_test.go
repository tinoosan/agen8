package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	iofs "io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/vfsutil"
)

var builtinTestManifest = []byte(`{
  "id": "builtin.test",
  "version": "0.1.0",
  "kind": "builtin",
  "displayName": "Builtin Test",
  "description": "Detects a project's test framework and runs/tests lists with bounded output.",
  "actions": [
    {
      "id": "detect",
      "displayName": "Detect test framework",
      "description": "Detect test framework from files in cwd (go.mod, package.json, pyproject.toml, pytest.ini).",
      "inputSchema": {
        "type": "object",
        "properties": { "cwd": { "type": "string" } }
      },
      "outputSchema": {
        "type": "object",
        "properties": {
          "framework": { "type": "string" },
          "details": { "type": "object" }
        },
        "required": ["framework"]
      }
    },
    {
      "id": "run",
      "displayName": "Run tests",
      "description": "Run tests for the detected framework. Returns exitCode/stdout/stderr and writes full output as artifacts when truncated.",
      "inputSchema": {
        "type": "object",
        "properties": {
          "cwd": { "type": "string" },
          "filter": { "type": "string" },
          "verbose": { "type": "boolean" }
        }
      },
      "outputSchema": {
        "type": "object",
        "properties": {
          "framework": { "type": "string" },
          "exitCode": { "type": "integer" },
          "passed": { "type": "boolean" },
          "stdout": { "type": "string" },
          "stderr": { "type": "string" },
          "stdoutPath": { "type": "string" },
          "stderrPath": { "type": "string" }
        },
        "required": ["framework","exitCode","passed","stdout","stderr"]
      }
    },
    {
      "id": "list",
      "displayName": "List tests",
      "description": "List test files (and, for Go, test function names) under cwd.",
      "inputSchema": {
        "type": "object",
        "properties": { "cwd": { "type": "string" } }
      },
      "outputSchema": {
        "type": "object",
        "properties": {
          "framework": { "type": "string" },
          "files": { "type": "array", "items": { "type": "string" } },
          "tests": { "type": "array", "items": { "type": "string" } }
        },
        "required": ["framework","files","tests"]
      }
    }
  ]
}`)

func init() {
	registerBuiltin(BuiltinDef{
		ID:       types.ToolID("builtin.test"),
		Manifest: builtinTestManifest,
		NewInvoker: func(cfg BuiltinConfig) ToolInvoker {
			return NewBuiltinTestInvoker(cfg.BashRootDir)
		},
	})
}

const (
	defaultTestMaxOutputBytes = 64 * 1024
)

type BuiltinTestInvoker struct {
	RootDir  string
	MaxBytes int
}

func NewBuiltinTestInvoker(rootDir string) *BuiltinTestInvoker {
	return &BuiltinTestInvoker{
		RootDir:  rootDir,
		MaxBytes: defaultTestMaxOutputBytes,
	}
}

type testDetectInput struct {
	Cwd string `json:"cwd,omitempty"`
}

type testDetectOutput struct {
	Framework string            `json:"framework"`
	Details   map[string]string `json:"details,omitempty"`
}

type testRunInput struct {
	Cwd     string `json:"cwd,omitempty"`
	Filter  string `json:"filter,omitempty"`
	Verbose bool   `json:"verbose,omitempty"`
}

type testRunOutput struct {
	Framework string `json:"framework"`
	ExitCode  int    `json:"exitCode"`
	Passed    bool   `json:"passed"`
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`

	StdoutPath string `json:"stdoutPath,omitempty"`
	StderrPath string `json:"stderrPath,omitempty"`
}

type testListInput struct {
	Cwd string `json:"cwd,omitempty"`
}

type testListOutput struct {
	Framework string   `json:"framework"`
	Files     []string `json:"files"`
	Tests     []string `json:"tests"`
}

func (t *BuiltinTestInvoker) Invoke(ctx context.Context, req types.ToolRequest) (ToolCallResult, error) {
	if t == nil {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: "builtin.test invoker is nil"}
	}
	if err := req.Validate(); err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: err.Error()}
	}

	// Safety: if callers omit timeoutMs and the context has no deadline, apply a default.
	if req.TimeoutMs == 0 {
		if _, hasDeadline := ctx.Deadline(); !hasDeadline {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, 10*time.Minute)
			defer cancel()
		}
	}

	switch req.ActionID {
	case "detect":
		return t.detect(ctx, req)
	case "run":
		return t.run(ctx, req)
	case "list":
		return t.list(ctx, req)
	default:
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("unsupported action %q (allowed: detect, run, list)", req.ActionID)}
	}
}

func (t *BuiltinTestInvoker) rootAndDir(cwd string) (root, absDir string, err error) {
	root = strings.TrimSpace(t.RootDir)
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

func (t *BuiltinTestInvoker) maxBytes() int {
	if t.MaxBytes <= 0 {
		return defaultTestMaxOutputBytes
	}
	return t.MaxBytes
}

func (t *BuiltinTestInvoker) detect(_ context.Context, req types.ToolRequest) (ToolCallResult, error) {
	var in testDetectInput
	if err := json.Unmarshal(req.Input, &in); err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("invalid input JSON: %v", err)}
	}
	_, absDir, err := t.rootAndDir(in.Cwd)
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: err.Error()}
	}

	fw, details := detectTestFramework(absDir)
	outJSON, err := json.Marshal(testDetectOutput{Framework: fw, Details: details})
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: fmt.Sprintf("marshal output: %v", err), Err: err}
	}
	return ToolCallResult{Output: outJSON}, nil
}

func detectTestFramework(absDir string) (framework string, details map[string]string) {
	exists := func(name string) bool {
		_, err := os.Stat(filepath.Join(absDir, name))
		return err == nil
	}
	details = map[string]string{}

	// Prefer Go when present (common for this repo).
	if exists("go.mod") {
		details["marker"] = "go.mod"
		return "go", details
	}
	if exists("package.json") {
		details["marker"] = "package.json"
		return "node", details
	}
	if exists("pyproject.toml") {
		details["marker"] = "pyproject.toml"
		return "python", details
	}
	if exists("pytest.ini") {
		details["marker"] = "pytest.ini"
		return "python", details
	}

	return "unknown", nil
}

func (t *BuiltinTestInvoker) run(ctx context.Context, req types.ToolRequest) (ToolCallResult, error) {
	var in testRunInput
	if err := json.Unmarshal(req.Input, &in); err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("invalid input JSON: %v", err)}
	}
	_, absDir, err := t.rootAndDir(in.Cwd)
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: err.Error()}
	}

	framework, _ := detectTestFramework(absDir)
	argv, err := buildTestRunArgv(framework, strings.TrimSpace(in.Filter), in.Verbose)
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: err.Error()}
	}

	exitCode, stdoutFull, stderrFull, err := runCmd(ctx, absDir, argv)
	if err != nil {
		return ToolCallResult{}, err
	}

	maxBytes := t.maxBytes()
	stdoutPreview, stdoutArtifact, stdoutTruncated := capBytesToArtifact("stdout.txt", stdoutFull, maxBytes)
	stderrPreview, stderrArtifact, stderrTruncated := capBytesToArtifact("stderr.txt", stderrFull, maxBytes)

	out := testRunOutput{
		Framework: framework,
		ExitCode:  exitCode,
		Passed:    exitCode == 0,
		Stdout:    stdoutPreview,
		Stderr:    stderrPreview,
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

func buildTestRunArgv(framework, filter string, verbose bool) ([]string, error) {
	switch framework {
	case "go":
		argv := []string{"go", "test", "./..."}
		if verbose {
			argv = append(argv[:2], append([]string{"-v"}, argv[2:]...)...)
		}
		if strings.TrimSpace(filter) != "" {
			argv = append(argv[:2], append([]string{"-run", filter}, argv[2:]...)...)
		}
		return argv, nil
	case "python":
		argv := []string{"pytest"}
		if verbose {
			argv = append(argv, "-v")
		}
		if strings.TrimSpace(filter) != "" {
			argv = append(argv, "-k", filter)
		}
		return argv, nil
	case "node":
		// Best-effort: forward args after "--" to the configured test runner.
		argv := []string{"npm", "test"}
		extra := []string{}
		if verbose {
			extra = append(extra, "--verbose")
		}
		if strings.TrimSpace(filter) != "" {
			// Single arg pattern; avoid splitting on whitespace.
			if strings.ContainsAny(filter, "\n\r") {
				return nil, fmt.Errorf("filter must not contain newlines")
			}
			extra = append(extra, filter)
		}
		if len(extra) > 0 {
			argv = append(argv, "--")
			argv = append(argv, extra...)
		}
		return argv, nil
	case "unknown":
		return nil, fmt.Errorf("could not detect test framework")
	default:
		return nil, fmt.Errorf("unsupported framework %q", framework)
	}
}

func (t *BuiltinTestInvoker) list(ctx context.Context, req types.ToolRequest) (ToolCallResult, error) {
	var in testListInput
	if err := json.Unmarshal(req.Input, &in); err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("invalid input JSON: %v", err)}
	}
	root, absDir, err := t.rootAndDir(in.Cwd)
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: err.Error()}
	}

	framework, _ := detectTestFramework(absDir)
	files, tests := listTestsBestEffort(root, absDir, framework)
	outJSON, err := json.Marshal(testListOutput{Framework: framework, Files: files, Tests: tests})
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: fmt.Sprintf("marshal output: %v", err), Err: err}
	}
	return ToolCallResult{Output: outJSON}, nil
}

func listTestsBestEffort(rootDir, absDir, framework string) (files []string, tests []string) {
	files = []string{}
	tests = []string{}

	// Go: find *_test.go and parse func TestXxx.
	if framework == "go" {
		testFn := regexp.MustCompile(`(?m)^\s*func\s+(Test[[:alnum:]_]+)\s*\(`)
		_ = filepath.WalkDir(absDir, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() && d.Name() == ".git" {
				return iofs.SkipDir
			}
			if d.IsDir() {
				return nil
			}
			name := d.Name()
			if !strings.HasSuffix(name, "_test.go") {
				return nil
			}
			rel, relErr := vfsutil.RelUnderBaseDir(rootDir, p)
			if relErr != nil || rel == "." {
				return nil
			}
			files = append(files, rel)
			b, readErr := os.ReadFile(p)
			if readErr != nil {
				return nil
			}
			m := testFn.FindAllSubmatch(b, -1)
			for _, mm := range m {
				if len(mm) < 2 {
					continue
				}
				tests = append(tests, string(mm[1]))
			}
			return nil
		})
		return uniqueStrings(files), uniqueStrings(tests)
	}

	// Python/Node/Unknown: list common test file patterns (best-effort).
	var suffixes []string
	switch framework {
	case "python":
		suffixes = []string{"_test.py", "test_.py"} // second is only a hint; handled below
	case "node":
		suffixes = []string{".test.js", ".test.ts", ".spec.js", ".spec.ts"}
	default:
		suffixes = []string{}
	}

	_ = filepath.WalkDir(absDir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && d.Name() == ".git" {
			return iofs.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		matches := false
		if framework == "python" {
			if strings.HasSuffix(name, "_test.py") || strings.HasPrefix(name, "test_") && strings.HasSuffix(name, ".py") {
				matches = true
			}
		} else {
			for _, s := range suffixes {
				if strings.HasSuffix(name, s) {
					matches = true
					break
				}
			}
		}
		if !matches {
			return nil
		}
		rel, relErr := vfsutil.RelUnderBaseDir(rootDir, p)
		if relErr != nil || rel == "." {
			return nil
		}
		files = append(files, rel)
		return nil
	})
	return uniqueStrings(files), []string{}
}

func runCmd(ctx context.Context, absDir string, argv []string) (exitCode int, stdout, stderr []byte, err error) {
	if len(argv) == 0 {
		return 0, nil, nil, &InvokeError{Code: "invalid_input", Message: "argv is required"}
	}
	if _, lookErr := exec.LookPath(argv[0]); lookErr != nil {
		return 0, nil, nil, &InvokeError{Code: "tool_failed", Message: fmt.Sprintf("%s binary not found", argv[0]), Err: lookErr}
	}

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = absDir

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	exitCode = 0
	runErr := cmd.Run()
	if runErr != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(runErr, context.DeadlineExceeded) {
			return 0, nil, nil, &InvokeError{Code: "timeout", Message: "test command timed out", Retryable: true, Err: runErr}
		}
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			return 0, nil, nil, &InvokeError{Code: "tool_failed", Message: runErr.Error(), Err: runErr}
		}
	}

	return exitCode, stdoutBuf.Bytes(), stderrBuf.Bytes(), nil
}

func capBytesToArtifact(path string, b []byte, maxBytes int) (preview string, artifact *ToolArtifactWrite, truncated bool) {
	s := string(b)
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s, nil, false
	}
	preview = s[:maxBytes]
	return preview, &ToolArtifactWrite{Path: path, Bytes: append([]byte(nil), b...), MediaType: "text/plain"}, true
}

