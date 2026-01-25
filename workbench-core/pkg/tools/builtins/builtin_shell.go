package builtins

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

	"github.com/tinoosan/workbench-core/pkg/types"
	"github.com/tinoosan/workbench-core/pkg/vfsutil"
	pkgtools "github.com/tinoosan/workbench-core/pkg/tools"
)

const defaultShellMaxOutputBytes = 64 * 1024

// BuiltinShellInvoker runs a guarded argv under the host workdir.
type BuiltinShellInvoker struct {
	RootDir      string
	VFSMountName string
	Deny         map[string]bool
	MaxBytes     int
	Confirm      func(ctx context.Context, argv []string, cwd string) (bool, error)
}

func NewBuiltinShellInvoker(rootDir string, confirm func(context.Context, []string, string) (bool, error), vfsMount string) *BuiltinShellInvoker {
	return &BuiltinShellInvoker{
		RootDir:      rootDir,
		VFSMountName: strings.TrimSpace(vfsMount),
		Deny:         DefaultShellDenylist(),
		MaxBytes:     defaultShellMaxOutputBytes,
		Confirm:      confirm,
	}
}

func DefaultShellDenylist() map[string]bool {
	return map[string]bool{
		"sudo": true, "su": true, "doas": true,
		"ssh": true, "scp": true, "sftp": true, "rsync": true,
		"nc": true, "ncat": true, "netcat": true, "socat": true, "telnet": true,
		"shutdown": true, "reboot": true, "halt": true, "poweroff": true, "dd": true, "mkfs": true, "mount": true, "umount": true,
		"systemctl": true, "launchctl": true,
		"rm": true,
	}
}

type shellExecInput struct {
	Argv  []string `json:"argv"`
	Cwd   string   `json:"cwd,omitempty"`
	Stdin string   `json:"stdin,omitempty"`
}

type shellExecOutput struct {
	ExitCode   int    `json:"exitCode"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	StdoutPath string `json:"stdoutPath,omitempty"`
	StderrPath string `json:"stderrPath,omitempty"`
}

func (s *BuiltinShellInvoker) Invoke(ctx context.Context, req pkgtools.ToolRequest) (pkgtools.ToolCallResult, error) {
	if s == nil {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "tool_failed", Message: "builtin.shell invoker is nil"}
	}
	if err := req.Validate(); err != nil {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "invalid_input", Message: err.Error()}
	}
	if req.ActionID != "exec" {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "invalid_input", Message: fmt.Sprintf("unsupported action %q (allowed: exec)", req.ActionID)}
	}

	root := strings.TrimSpace(s.RootDir)
	if root == "" || !filepath.IsAbs(root) {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "tool_failed", Message: "rootDir must be absolute"}
	}

	maxBytes := s.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultShellMaxOutputBytes
	}

	var in shellExecInput
	if err := json.Unmarshal(req.Input, &in); err != nil {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "invalid_input", Message: fmt.Sprintf("invalid input JSON: %v", err)}
	}
	if len(in.Argv) == 0 {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "invalid_input", Message: "argv is required"}
	}

	cmdName := in.Argv[0]
	if strings.Contains(cmdName, "/") || strings.Contains(cmdName, string(filepath.Separator)) {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "invalid_input", Message: "command must be a bare executable name (no path separators)"}
	}
	if s.Deny != nil && s.Deny[cmdName] {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "invalid_input", Message: fmt.Sprintf("command %q is denied", cmdName)}
	}
	for i := 1; i < len(in.Argv); i++ {
		if looksLikeAbsPathOrFlagValue(in.Argv[i]) {
			return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "invalid_input", Message: fmt.Sprintf("absolute paths are not allowed in argv (got %q); use relative paths", in.Argv[i])}
		}
	}

	cwd := strings.TrimSpace(in.Cwd)
	if cwd == "" {
		cwd = "."
	}
	absDir, err := vfsutil.SafeJoinBaseDir(root, cwd)
	if err != nil {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "invalid_input", Message: err.Error()}
	}
	if st, err := os.Stat(absDir); err != nil || !st.IsDir() {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "invalid_input", Message: fmt.Sprintf("cwd %q is not a directory", cwd)}
	}

	if s.Confirm != nil {
		ok, err := s.Confirm(ctx, in.Argv, cwd)
		if err != nil {
			return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "tool_failed", Message: err.Error(), Err: err}
		}
		if !ok {
			return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{
				Code:    types.CommandRejectedErrorCode,
				Message: types.CommandRejectedErrorMessage,
			}
		}
	}

	cmd := exec.CommandContext(ctx, cmdName, in.Argv[1:]...)
	cmd.Dir = absDir
	cmd.Env = filterShellEnv(os.Environ())
	if in.Stdin != "" {
		cmd.Stdin = strings.NewReader(in.Stdin)
	}

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	exitCode := 0
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "timeout", Message: "command timed out", Retryable: true, Err: err}
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "tool_failed", Message: err.Error(), Err: err}
		}
	}

	stdoutFull := stdoutBuf.Bytes()
	stderrFull := stderrBuf.Bytes()

	stdoutText := s.translateOutputPaths(string(stdoutFull))
	stderrText := s.translateOutputPaths(string(stderrFull))

	out := shellExecOutput{
		ExitCode: exitCode,
		Stdout:   truncateString(stdoutText, maxBytes),
		Stderr:   truncateString(stderrText, maxBytes),
	}

	artifacts := make([]pkgtools.ToolArtifactWrite, 0, 2)
	if len(stdoutFull) > maxBytes {
		out.StdoutPath = "stdout.txt"
		artifacts = append(artifacts, pkgtools.ToolArtifactWrite{
			Path:      "stdout.txt",
			Bytes:     []byte(stdoutText),
			MediaType: "text/plain",
		})
	}
	if len(stderrFull) > maxBytes {
		out.StderrPath = "stderr.txt"
		artifacts = append(artifacts, pkgtools.ToolArtifactWrite{
			Path:      "stderr.txt",
			Bytes:     []byte(stderrText),
			MediaType: "text/plain",
		})
	}

	outputJSON, err := json.Marshal(out)
	if err != nil {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "tool_failed", Message: fmt.Sprintf("marshal output: %v", err), Err: err}
	}

	return pkgtools.ToolCallResult{Output: outputJSON, Artifacts: artifacts}, nil
}

func filterShellEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if isShellSensitiveEnvKey(k) {
			continue
		}
		out = append(out, k+"="+v)
	}
	return out
}

func isShellSensitiveEnvKey(k string) bool {
	upper := strings.ToUpper(k)
	sensitivePrefixes := []string{
		"AWS_",
		"SECRET",
		"TOKEN",
		"KEY",
		"PASS",
		"OPENROUTER_",
	}
	for _, p := range sensitivePrefixes {
		if strings.HasPrefix(upper, p) {
			return true
		}
	}
	return false
}

func looksLikeAbsPath(s string) bool {
	if strings.TrimSpace(s) == "" {
		return false
	}
	if strings.HasPrefix(s, "/") {
		return true
	}
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

func truncateString(s string, max int) string {
	if max <= 0 {
		return s
	}
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func (s *BuiltinShellInvoker) translateOutputPaths(text string) string {
	mount := strings.TrimSpace(s.VFSMountName)
	if mount == "" {
		return text
	}
	root := strings.TrimSpace(s.RootDir)
	if root == "" {
		return text
	}
	hostRoot := filepath.Clean(root)
	prefix := "/" + mount
	out := strings.ReplaceAll(text, hostRoot, prefix)
	if slashRoot := filepath.ToSlash(hostRoot); slashRoot != hostRoot {
		out = strings.ReplaceAll(out, slashRoot, prefix)
	}
	return out
}
