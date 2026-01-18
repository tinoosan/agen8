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

	"github.com/tinoosan/workbench-core/internal/pathutil"
	"github.com/tinoosan/workbench-core/internal/types"
)

var builtinBashManifest = []byte(`{"id":"builtin.bash","version":"0.1.0","kind":"builtin","displayName":"Builtin Bash (restricted)","description":"Runs CLI commands inside a host-configured root directory with a small denylist (shells/interpreters/privilege escalation).","actions":[{"id":"exec","displayName":"Execute command","description":"Execute a command with argv and return exitCode/stdout/stderr. Some command names are denied. Absolute path arguments are rejected. stdout/stderr may be truncated; full output may be written as artifacts.","inputSchema":{"type":"object","properties":{"argv":{"type":"array","items":{"type":"string"},"minItems":1},"cwd":{"type":"string"},"stdin":{"type":"string"}},"required":["argv"]},"outputSchema":{"type":"object","properties":{"exitCode":{"type":"integer"},"stdout":{"type":"string"},"stderr":{"type":"string"},"stdoutPath":{"type":"string"},"stderrPath":{"type":"string"}},"required":["exitCode","stdout","stderr"]}}]}`)

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
	Deny     map[string]bool
	MaxBytes int
}

// NewBuiltinBashInvoker returns a BuiltinBashInvoker with the default denylist
// and max output cap (64KB).
//
// rootDir must be an absolute OS path. It is used as the sandbox root for "cwd".
func NewBuiltinBashInvoker(rootDir string) *BuiltinBashInvoker {
	return &BuiltinBashInvoker{
		RootDir:  rootDir,
		Deny:     DefaultBashDenylist(),
		MaxBytes: defaultBashMaxOutputBytes,
	}
}

// DefaultBashDenylist returns the initial denylist for builtin.bash exec.
//
// Rationale:
//   - An allowlist becomes a maintenance burden as the agent grows more capable.
//   - A denylist is smaller and easier to evolve while still blocking the most
//     risky classes of commands (shells, interpreters, privilege escalation).
//
// Security note:
//   - builtin.bash is still confined by RootDir + cwd rules.
//   - Additionally, builtin.bash rejects absolute file paths in argv to avoid
//     accidental reads/writes outside RootDir.
func DefaultBashDenylist() map[string]bool {
	return map[string]bool{
		// Shells (would allow the agent to construct arbitrary pipelines/scripts).
		"bash": true,
		"sh":   true,
		"zsh":  true,
		"fish": true,
		"ksh":  true,
		"dash": true,
		"tcsh": true,
		"csh":  true,

		// Interpreters / runtimes (arbitrary scripting).
		"python":  true,
		"python3": true,
		"node":    true,
		"deno":    true,
		"bun":     true,
		"ruby":    true,
		"perl":    true,
		"php":     true,
		"lua":     true,
		"java":    true,

		// Privilege escalation.
		"sudo": true,
		"su":   true,
		"doas": true,

		// Remote shells / file transfer (avoid turning the tool into an SSH client).
		"ssh":   true,
		"scp":   true,
		"sftp":  true,
		"rsync": true,

		// Package managers / installers (can mutate the host and exfiltrate).
		"brew":    true,
		"apt":     true,
		"apt-get": true,
		"yum":     true,
		"dnf":     true,
		"pacman":  true,
		"apk":     true,
		"pip":     true,
		"pip3":    true,
		"npm":     true,
		"npx":     true,
		"gem":     true,

		// Network pivot tools.
		"nc":     true,
		"ncat":   true,
		"netcat": true,
		"socat":  true,
		"telnet": true,

		// Network fetchers:
		// - Prefer builtin.http for outbound HTTP.
		// - Deny common downloaders by default.
		"curl": true,
		"wget": true,

		// System control / destructive disk ops.
		"shutdown": true,
		"reboot":   true,
		"halt":     true,
		"poweroff": true,
		"dd":       true,
		"mkfs":     true,
		"mount":    true,
		"umount":   true,

		// Permission and ownership changes (broadly unsafe).
		"chmod": true,
		"chown": true,

		// Service control.
		"systemctl": true,
		"launchctl": true,

		// Deletion (prefer host-mediated writes; avoid destructive shell ops).
		"rm": true,
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
	if strings.Contains(cmdName, "/") || strings.Contains(cmdName, string(filepath.Separator)) {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: "command must be a bare executable name (no path separators)"}
	}
	if b.Deny != nil && b.Deny[cmdName] {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("command %q is denied", cmdName)}
	}
	// Prevent absolute path arguments (keeps file IO under RootDir + cwd).
	for _, a := range in.Argv[1:] {
		if looksLikeAbsPathOrFlagValue(a) {
			return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("absolute paths are not allowed in argv (got %q); use relative paths under cwd", a)}
		}
	}

	cwd := strings.TrimSpace(in.Cwd)
	if cwd == "" {
		cwd = "."
	}

	absDir, err := pathutil.SafeJoinBaseDir(root, cwd)
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

func looksLikeAbsPath(s string) bool {
	if strings.TrimSpace(s) == "" {
		return false
	}
	// Unix-like absolute path.
	if strings.HasPrefix(s, "/") {
		return true
	}
	// Windows-like drive path.
	if filepath.IsAbs(s) {
		return true
	}
	return false
}

func looksLikeAbsPathOrFlagValue(s string) bool {
	if looksLikeAbsPath(s) {
		return true
	}
	_, v, ok := strings.Cut(s, "=")
	if ok && looksLikeAbsPath(v) {
		return true
	}
	return false
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
