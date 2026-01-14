package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tinoosan/workbench-core/internal/types"
)

var builtinBashManifest = []byte(`{"id":"builtin.bash","version":"0.1.0","kind":"builtin","displayName":"Builtin Bash (restricted)","description":"Runs a restricted set of CLI commands inside a host-configured root directory.","actions":[{"id":"exec","displayName":"Execute command","description":"Execute an allowlisted command with argv and return exitCode/stdout/stderr. stdout/stderr may be truncated; full output may be written as artifacts.","inputSchema":{"type":"object","properties":{"argv":{"type":"array","items":{"type":"string"},"minItems":1},"cwd":{"type":"string"},"stdin":{"type":"string"}},"required":["argv"]},"outputSchema":{"type":"object","properties":{"exitCode":{"type":"integer"},"stdout":{"type":"string"},"stderr":{"type":"string"},"stdoutPath":{"type":"string"},"stderrPath":{"type":"string"}},"required":["exitCode","stdout","stderr"]}}]}`)

func init() {
	registerBuiltin(BuiltinDef{
		ID:       types.ToolID("builtin.bash"),
		Manifest: builtinBashManifest,
		NewInvoker: func(cfg BuiltinConfig) ToolInvoker {
			return NewBuiltinBashInvoker(cfg.BashRootDir)
		},
	})
}

const (
	defaultBashMaxOutputBytes = 64 * 1024
)

// BuiltinBashInvoker implements the builtin tool "builtin.bash" (action: "exec").
//
// This tool executes a restricted set of CLI commands inside a host-configured
// directory root. It is discovered via /tools and executed via tool.run, just
// like any other tool.
//
// Agent-facing discovery:
//   - fs.List("/tools") includes "/tools/builtin.bash"
//   - fs.Read("/tools/builtin.bash") returns the manifest JSON bytes
//
// Agent-facing execution (host primitive):
//
//	{
//	  "op": "tool.run",
//	  "toolId": "builtin.bash",
//	  "actionId": "exec",
//	  "input": {
//	    "argv": ["ls", "-la"],
//	    "cwd": "."
//	  },
//	  "timeoutMs": 5000
//	}
//
// Results persisted by the runner:
//   - /results/<callId>/response.json
//   - /results/<callId>/stdout.txt (only when stdout is truncated)
//   - /results/<callId>/stderr.txt (only when stderr is truncated)
//
// Output policy:
//   - stdout/stderr are captured with a response cap (default 64KB each).
//   - If output exceeds the cap, the response includes a truncated string and
//     the full bytes are written as artifacts (stdout.txt / stderr.txt).
//   - A non-zero exit code is still a successful tool call (Ok=true).
//     Only failures to start/execute (including timeouts) return Ok=false.
//
// Working directory confinement:
//   - RootDir is an absolute OS path configured by the host.
//   - input.cwd is a relative path under RootDir (default ".").
//   - Absolute paths and escape attempts ("..") are rejected.
//
// Note: "bash" here is a name only. Execution is argv-based; this does NOT
// invoke "bash -lc ...".
type BuiltinBashInvoker struct {
	RootDir  string
	Allow    map[string]bool
	MaxBytes int
}

// NewBuiltinBashInvoker returns a BuiltinBashInvoker with the default allowlist
// and max output cap (64KB).
//
// rootDir must be an absolute OS path. It is used as the sandbox root for "cwd".
func NewBuiltinBashInvoker(rootDir string) *BuiltinBashInvoker {
	return &BuiltinBashInvoker{
		RootDir:  rootDir,
		Allow:    DefaultBashAllowlist(),
		MaxBytes: defaultBashMaxOutputBytes,
	}
}

// DefaultBashAllowlist returns the initial allowlist for builtin.bash exec.
//
// The allowlist is intentionally strict; expand only as needed.
func DefaultBashAllowlist() map[string]bool {
	return map[string]bool{
		// File/text inspection (confined by RootDir + cwd rules).
		"ls":   true,
		"cat":  true,
		"rg":   true,
		"find": true,
		"head": true,
		"tail": true,
		"wc":   true,
		"grep": true,
		"sed":  true,
		"awk":  true,
		"jq":   true,
		"stat": true,

		// Basic system/IT inspection (read-only style commands).
		"date":   true,
		"uname":  true,
		"whoami": true,
		"id":     true,
		"env":    true,
		"which":  true,

		"ps":   true,
		"top":  true,
		"df":   true,
		"du":   true,
		"free": true,

		"netstat": true,
		"lsof":    true,

		// Network-capable tools.
		// Still confined to RootDir for cwd, but can access the network.
		// Keep this list explicit and add commands only when needed.
		"curl":       true,
		"wget":       true,
		"ping":       true,
		"traceroute": true,
		"dig":        true,
		"nslookup":   true,
	}
}

type bashExecInput struct {
	Argv  []string `json:"argv"`
	Cwd   string   `json:"cwd,omitempty"`
	Stdin string   `json:"stdin,omitempty"`
}

type bashExecOutput struct {
	ExitCode   int    `json:"exitCode"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	StdoutPath string `json:"stdoutPath,omitempty"`
	StderrPath string `json:"stderrPath,omitempty"`
}

// Invoke executes builtin.bash exec.
//
// Input JSON shape:
//   - argv: required array of strings (argv[0] is the command)
//   - cwd: optional relative path under RootDir (default ".")
//   - stdin: optional string provided on stdin
//
// Success vs failure:
//   - Non-zero exit codes are treated as success (Ok=true) and returned in output.exitCode.
//   - Failures to start/execute (including timeouts) return an InvokeError so the runner
//     persists ToolResponse.Ok=false with an appropriate error code.
func (b *BuiltinBashInvoker) Invoke(ctx context.Context, req types.ToolRequest) (ToolCallResult, error) {
	if b == nil {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: "builtin.bash invoker is nil"}
	}
	if err := req.Validate(); err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: err.Error()}
	}
	if req.ActionID != "exec" {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("unsupported action %q (allowed: exec)", req.ActionID)}
	}

	root := strings.TrimSpace(b.RootDir)
	if root == "" {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: "rootDir is required"}
	}
	if !filepath.IsAbs(root) {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: fmt.Sprintf("rootDir must be absolute, got %q", root)}
	}

	maxBytes := b.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultBashMaxOutputBytes
	}

	var in bashExecInput
	if err := json.Unmarshal(req.Input, &in); err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("invalid input JSON: %v", err)}
	}
	if len(in.Argv) == 0 {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: "argv is required"}
	}

	cmdName := in.Argv[0]
	if b.Allow != nil && !b.Allow[cmdName] {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("command %q is not allowed", cmdName)}
	}

	cwd := strings.TrimSpace(in.Cwd)
	if cwd == "" {
		cwd = "."
	}

	absDir, err := resolveUnderRoot(root, cwd)
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: err.Error()}
	}
	if st, err := os.Stat(absDir); err != nil || !st.IsDir() {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("cwd %q is not a directory", cwd)}
	}

	cmd := exec.CommandContext(ctx, cmdName, in.Argv[1:]...)
	cmd.Dir = absDir
	if in.Stdin != "" {
		cmd.Stdin = strings.NewReader(in.Stdin)
	}

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	exitCode := 0
	runErr := cmd.Run()
	if runErr != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(runErr, context.DeadlineExceeded) {
			return ToolCallResult{}, &InvokeError{Code: "timeout", Message: "command timed out", Retryable: true, Err: runErr}
		}

		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: runErr.Error(), Err: runErr}
		}
	}

	stdoutFull := stdoutBuf.Bytes()
	stderrFull := stderrBuf.Bytes()

	out := bashExecOutput{
		ExitCode: exitCode,
		Stdout:   truncateString(string(stdoutFull), maxBytes),
		Stderr:   truncateString(string(stderrFull), maxBytes),
	}

	artifacts := make([]ToolArtifactWrite, 0, 2)
	if len(stdoutFull) > maxBytes {
		out.StdoutPath = "stdout.txt"
		artifacts = append(artifacts, ToolArtifactWrite{
			Path:      "stdout.txt",
			Bytes:     stdoutFull,
			MediaType: "text/plain",
		})
	}
	if len(stderrFull) > maxBytes {
		out.StderrPath = "stderr.txt"
		artifacts = append(artifacts, ToolArtifactWrite{
			Path:      "stderr.txt",
			Bytes:     stderrFull,
			MediaType: "text/plain",
		})
	}

	outputJSON, err := json.Marshal(out)
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: fmt.Sprintf("marshal output: %v", err), Err: err}
	}

	return ToolCallResult{Output: outputJSON, Artifacts: artifacts}, nil
}

// truncateString returns s truncated to maxBytes bytes.
//
// This is a byte-level truncation (not rune-aware). It is used only for human-facing
// previews in ToolResponse.Output; the full bytes are preserved in artifacts.
func truncateString(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return s
	}
	if len(s) <= maxBytes {
		return s
	}
	return s[:maxBytes]
}

// resolveUnderRoot resolves a relative cwd under rootDir and rejects any escape attempt.
//
// Policy:
//   - rel must be resource-relative (not absolute)
//   - rel must not contain ".." segments
//   - the resulting path must stay under rootDir after cleaning/joining
func resolveUnderRoot(rootDir, rel string) (string, error) {
	if strings.HasPrefix(rel, "/") || filepath.IsAbs(rel) {
		return "", fmt.Errorf("cwd must be relative")
	}

	// Reject any explicit parent segments, even if they would clean away.
	for _, seg := range strings.Split(rel, string(filepath.Separator)) {
		if seg == ".." {
			return "", fmt.Errorf("cwd escapes root")
		}
	}
	for _, seg := range strings.Split(rel, "/") {
		if seg == ".." {
			return "", fmt.Errorf("cwd escapes root")
		}
	}

	cleanRel := filepath.Clean(rel)
	if cleanRel == ".." || strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("cwd escapes root")
	}

	rootClean := filepath.Clean(rootDir)
	abs := filepath.Join(rootClean, cleanRel)

	relToRoot, err := filepath.Rel(rootClean, abs)
	if err != nil {
		return "", fmt.Errorf("cwd resolution failed")
	}
	if relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("cwd escapes root")
	}
	return abs, nil
}
