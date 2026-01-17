package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/tinoosan/workbench-core/internal/types"
)

var builtinOpenManifest = []byte(`{
  "id": "builtin.open",
  "version": "0.1.0",
  "kind": "builtin",
  "displayName": "Builtin Open",
  "description": "Opens a file or directory in the host OS default application.",
  "actions": [
    {
      "id": "open",
      "displayName": "Open",
      "description": "Open a file or directory path (relative to the host-configured root directory).",
      "inputSchema": {
        "type": "object",
        "properties": {
          "path": { "type": "string" }
        },
        "required": ["path"]
      },
      "outputSchema": {
        "type": "object",
        "properties": {
          "opened": { "type": "boolean" },
          "path": { "type": "string" }
        },
        "required": ["opened", "path"]
      }
    }
  ]
}`)

func init() {
	registerBuiltin(BuiltinDef{
		ID:       types.ToolID("builtin.open"),
		Manifest: builtinOpenManifest,
		NewInvoker: func(cfg BuiltinConfig) ToolInvoker {
			// builtin.open follows the same sandbox root as builtin.bash by default:
			// the host sets BashRootDir to the current /workdir absolute path.
			return NewBuiltinOpenInvoker(cfg.BashRootDir)
		},
	})
}

// BuiltinOpenInvoker implements builtin.open (action: "open").
//
// This is a host UX primitive exposed as a tool:
//   - it is discovered via /tools (manifest JSON)
//   - it is executed via tool.run (so it appears in the Activity feed)
//
// Why this exists:
//   - The TUI wants a safe "/open @file" command without sprinkling OS-specific
//     exec calls throughout UI code.
//   - The agent/host already has a "tool.run" execution pathway and permissions
//     model; builtin.open fits that pattern.
//
// Safety:
//   - RootDir is an absolute OS path configured by the host.
//   - Input "path" must be relative and must not escape RootDir.
//   - The target must exist (file or directory).
type BuiltinOpenInvoker struct {
	RootDir string
	openFn  func(ctx context.Context, absPath string) error
}

// NewBuiltinOpenInvoker constructs a BuiltinOpenInvoker that uses the host OS "open" command.
func NewBuiltinOpenInvoker(rootDir string) *BuiltinOpenInvoker {
	return NewBuiltinOpenInvokerWithOpener(rootDir, defaultOSOpen)
}

// NewBuiltinOpenInvokerWithOpener constructs a BuiltinOpenInvoker with an injected opener.
//
// This is primarily used for testing so unit tests don't launch real applications.
func NewBuiltinOpenInvokerWithOpener(rootDir string, openFn func(ctx context.Context, absPath string) error) *BuiltinOpenInvoker {
	return &BuiltinOpenInvoker{RootDir: rootDir, openFn: openFn}
}

type openInput struct {
	Path string `json:"path"`
}

type openOutput struct {
	Opened bool   `json:"opened"`
	Path   string `json:"path"`
}

func (o *BuiltinOpenInvoker) Invoke(ctx context.Context, req types.ToolRequest) (ToolCallResult, error) {
	if o == nil {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: "builtin.open invoker is nil"}
	}
	if err := req.Validate(); err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: err.Error()}
	}
	if req.ActionID != "open" {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("unsupported action %q (allowed: open)", req.ActionID)}
	}
	root := strings.TrimSpace(o.RootDir)
	if root == "" {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: "rootDir is required"}
	}
	if !filepath.IsAbs(root) {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: fmt.Sprintf("rootDir must be absolute, got %q", root)}
	}
	if o.openFn == nil {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: "open function is not configured"}
	}

	var in openInput
	if err := json.Unmarshal(req.Input, &in); err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("invalid input JSON: %v", err)}
	}
	in.Path = strings.TrimSpace(in.Path)
	if in.Path == "" {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: "path is required"}
	}
	if err := validateRelativeToolPath(in.Path); err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("invalid path: %v", err)}
	}

	abs := filepath.Join(root, filepath.FromSlash(in.Path))
	if _, err := os.Stat(abs); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: "path does not exist"}
		}
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: err.Error(), Err: err}
	}

	if err := o.openFn(ctx, abs); err != nil {
		if ctx != nil && ctx.Err() != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return ToolCallResult{}, &InvokeError{Code: "timeout", Message: "open timed out", Retryable: true, Err: err}
			}
			return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: "open cancelled", Retryable: true, Err: err}
		}
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: err.Error(), Err: err}
	}

	out, err := json.Marshal(openOutput{Opened: true, Path: in.Path})
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: fmt.Sprintf("marshal output: %v", err), Err: err}
	}
	return ToolCallResult{Output: out}, nil
}

func defaultOSOpen(ctx context.Context, absPath string) error {
	absPath = strings.TrimSpace(absPath)
	if absPath == "" {
		return fmt.Errorf("path is required")
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "open", absPath)
	case "windows":
		// "start" is a shell builtin; invoke it via cmd.exe.
		cmd = exec.CommandContext(ctx, "cmd", "/c", "start", "", absPath)
	default:
		cmd = exec.CommandContext(ctx, "xdg-open", absPath)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s: %s", cmd.Path, msg)
	}
	return nil
}
