package builtins

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/tinoosan/agen8/pkg/types"
)

func TestHandleCodeExecToolCall_RejectsNestedCodeExec(t *testing.T) {
	frame := handleCodeExecToolCall(
		context.Background(),
		codeExecToolCallFrame{Type: "tool_call", ID: 1, Tool: "code_exec", Args: json.RawMessage(`{}`)},
		1,
		10,
		[]string{"code_exec"},
		func(context.Context, string, json.RawMessage) (types.HostOpRequest, error) {
			return types.HostOpRequest{}, nil
		},
		func(context.Context, types.HostOpRequest) types.HostOpResponse { return types.HostOpResponse{Ok: true} },
	)
	if frame.OK {
		t.Fatalf("expected nested code_exec to fail")
	}
	if frame.Error == "" {
		t.Fatalf("expected nested code_exec error")
	}
}

func TestHandleCodeExecToolCall_RejectsNonAllowlistedTool(t *testing.T) {
	frame := handleCodeExecToolCall(
		context.Background(),
		codeExecToolCallFrame{Type: "tool_call", ID: 1, Tool: "fs_read", Args: json.RawMessage(`{}`)},
		1,
		10,
		[]string{"fs_list"},
		func(context.Context, string, json.RawMessage) (types.HostOpRequest, error) {
			return types.HostOpRequest{}, nil
		},
		func(context.Context, types.HostOpRequest) types.HostOpResponse { return types.HostOpResponse{Ok: true} },
	)
	if frame.OK {
		t.Fatalf("expected non-allowlisted tool to fail")
	}
}

func TestHandleCodeExecToolCall_RespectsMaxToolCalls(t *testing.T) {
	frame := handleCodeExecToolCall(
		context.Background(),
		codeExecToolCallFrame{Type: "tool_call", ID: 1, Tool: "fs_read", Args: json.RawMessage(`{}`)},
		3,
		2,
		[]string{"fs_read"},
		func(context.Context, string, json.RawMessage) (types.HostOpRequest, error) {
			return types.HostOpRequest{}, nil
		},
		func(context.Context, types.HostOpRequest) types.HostOpResponse { return types.HostOpResponse{Ok: true} },
	)
	if frame.OK {
		t.Fatalf("expected max tool calls enforcement failure")
	}
}

func TestHandleCodeExecToolCall_DispatchAndBridge(t *testing.T) {
	frame := handleCodeExecToolCall(
		context.Background(),
		codeExecToolCallFrame{Type: "tool_call", ID: 7, Tool: "fs_read", Args: json.RawMessage(`{"path":"/workspace/a.txt"}`)},
		1,
		5,
		[]string{"fs_read"},
		func(_ context.Context, name string, _ json.RawMessage) (types.HostOpRequest, error) {
			if name != "fs_read" {
				t.Fatalf("dispatch tool name = %q", name)
			}
			return types.HostOpRequest{Op: types.HostOpFSRead, Path: "/workspace/a.txt"}, nil
		},
		func(_ context.Context, req types.HostOpRequest) types.HostOpResponse {
			if req.Op != types.HostOpFSRead {
				t.Fatalf("bridge op = %q", req.Op)
			}
			return types.HostOpResponse{Op: req.Op, Ok: true, Text: "ok"}
		},
	)
	if !frame.OK {
		t.Fatalf("expected bridge success, got %+v", frame)
	}
	if frame.Response == nil || frame.Response.Op != types.HostOpFSRead {
		t.Fatalf("expected bridged response, got %+v", frame.Response)
	}
}

func TestResolveCodeExecCwd(t *testing.T) {
	project := t.TempDir()
	workspace := t.TempDir()
	mounts := map[string]string{
		"project":   project,
		"workspace": workspace,
	}

	got, logical, err := resolveCodeExecCwd(project, mounts, "/workspace/sub")
	if err != nil {
		t.Fatalf("resolve absolute cwd: %v", err)
	}
	if logical != "/workspace/sub" {
		t.Fatalf("logical cwd = %q", logical)
	}
	want := filepath.Join(workspace, "sub")
	if got != want {
		t.Fatalf("resolved cwd = %q, want %q", got, want)
	}

	if _, _, err := resolveCodeExecCwd(project, mounts, "../escape"); err == nil {
		t.Fatalf("expected relative traversal error")
	}
}

func TestCodeExecFramePayload(t *testing.T) {
	if _, ok := codeExecFramePayload(`{"type":"final"}`); ok {
		t.Fatalf("expected non-prefixed frame to be ignored")
	}
	payload, ok := codeExecFramePayload(codeExecFramePrefix + `{"type":"final","ok":true}`)
	if !ok {
		t.Fatalf("expected prefixed frame to be parsed")
	}
	if got := strings.TrimSpace(string(payload)); got != `{"type":"final","ok":true}` {
		t.Fatalf("unexpected payload: %q", got)
	}
}

func TestNewBuiltinCodeExecInvoker_DefaultTimeoutPolicy(t *testing.T) {
	inv := NewBuiltinCodeExecInvoker(t.TempDir(), map[string]string{"workspace": t.TempDir()})
	if inv.DefaultTimeoutMs != 120_000 {
		t.Fatalf("DefaultTimeoutMs = %d, want 120000", inv.DefaultTimeoutMs)
	}
	if inv.MaxTimeoutMs != 420_000 {
		t.Fatalf("MaxTimeoutMs = %d, want 420000", inv.MaxTimeoutMs)
	}
}

func TestRunPython_AcceptsKeywordArgsAndPositionalDict(t *testing.T) {
	python := mustFindPython(t)
	project := t.TempDir()
	workspace := t.TempDir()
	inv := NewBuiltinCodeExecInvoker(project, map[string]string{
		"project":   project,
		"workspace": workspace,
	})

	cases := []string{
		`result = tools.fs_list(path="/project")`,
		`result = tools.fs_list({"path": "/project"})`,
		`import tools
result = tools.fs_list(path="/project")`,
	}
	for _, code := range cases {
		t.Run(code, func(t *testing.T) {
			calls := 0
			out, err := inv.runPython(context.Background(), python, workspace, codeExecRunConfig{
				Code:         code,
				Allowlist:    []string{"fs_list"},
				MaxToolCalls: 5,
				TimeoutMs:    4_000,
				MaxOutput:    8 * 1024,
				Dispatch: func(_ context.Context, toolName string, args json.RawMessage) (types.HostOpRequest, error) {
					calls++
					if strings.TrimSpace(toolName) != "fs_list" {
						t.Fatalf("tool name = %q, want fs_list", toolName)
					}
					var payload struct {
						Path string `json:"path"`
					}
					if err := json.Unmarshal(args, &payload); err != nil {
						t.Fatalf("unmarshal args: %v", err)
					}
					if payload.Path != "/project" {
						t.Fatalf("path = %q, want /project", payload.Path)
					}
					return types.HostOpRequest{Op: types.HostOpFSList, Path: payload.Path}, nil
				},
				Bridge: func(_ context.Context, req types.HostOpRequest) types.HostOpResponse {
					if req.Op != types.HostOpFSList {
						t.Fatalf("bridge op = %q, want fs_list", req.Op)
					}
					return types.HostOpResponse{Op: req.Op, Ok: true, Text: `{"entries":[]}`}
				},
				EnvAllowlist: inv.EnvAllowlist,
			})
			if err != nil {
				t.Fatalf("runPython error: %v", err)
			}
			if !out.OK {
				t.Fatalf("expected ok output, got %+v", out)
			}
			if calls != 1 {
				t.Fatalf("expected one tool call, got %d", calls)
			}
			if out.ToolCallCount != 1 {
				t.Fatalf("toolCallCount = %d, want 1", out.ToolCallCount)
			}
		})
	}
}

func TestRunPython_AcceptsImportToolsStyle(t *testing.T) {
	python := mustFindPython(t)
	project := t.TempDir()
	workspace := t.TempDir()
	inv := NewBuiltinCodeExecInvoker(project, map[string]string{
		"project":   project,
		"workspace": workspace,
	})

	out, err := inv.runPython(context.Background(), python, workspace, codeExecRunConfig{
		Code: `import tools
result = tools.fs_list(path="/project")`,
		Allowlist:    []string{"fs_list"},
		MaxToolCalls: 5,
		TimeoutMs:    4_000,
		MaxOutput:    8 * 1024,
		Dispatch: func(_ context.Context, toolName string, args json.RawMessage) (types.HostOpRequest, error) {
			if strings.TrimSpace(toolName) != "fs_list" {
				t.Fatalf("tool name = %q, want fs_list", toolName)
			}
			var payload struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(args, &payload); err != nil {
				t.Fatalf("unmarshal args: %v", err)
			}
			if payload.Path != "/project" {
				t.Fatalf("path = %q, want /project", payload.Path)
			}
			return types.HostOpRequest{Op: types.HostOpFSList, Path: payload.Path}, nil
		},
		Bridge: func(_ context.Context, req types.HostOpRequest) types.HostOpResponse {
			return types.HostOpResponse{Op: req.Op, Ok: true, Text: `{"entries":[]}`}
		},
		EnvAllowlist: inv.EnvAllowlist,
	})
	if err != nil {
		t.Fatalf("runPython error: %v", err)
	}
	if !out.OK {
		t.Fatalf("expected import tools style to succeed, got %+v", out)
	}
	if strings.Contains(strings.ToLower(out.Error), "no module named 'tools'") {
		t.Fatalf("unexpected missing tools module error: %q", out.Error)
	}
}

func TestRunPython_InvalidPositionalArgsReturnToolError(t *testing.T) {
	python := mustFindPython(t)
	project := t.TempDir()
	workspace := t.TempDir()
	inv := NewBuiltinCodeExecInvoker(project, map[string]string{
		"project":   project,
		"workspace": workspace,
	})
	out, err := inv.runPython(context.Background(), python, workspace, codeExecRunConfig{
		Code:         `result = tools.fs_list("/project")`,
		Allowlist:    []string{"fs_list"},
		MaxToolCalls: 5,
		TimeoutMs:    4_000,
		MaxOutput:    8 * 1024,
		Dispatch: func(_ context.Context, _ string, _ json.RawMessage) (types.HostOpRequest, error) {
			return types.HostOpRequest{Op: types.HostOpFSList, Path: "/project"}, nil
		},
		Bridge: func(_ context.Context, req types.HostOpRequest) types.HostOpResponse {
			return types.HostOpResponse{Op: req.Op, Ok: true}
		},
		EnvAllowlist: inv.EnvAllowlist,
	})
	if err != nil {
		t.Fatalf("runPython error: %v", err)
	}
	if out.OK {
		t.Fatalf("expected tool signature failure, got %+v", out)
	}
	if !strings.Contains(strings.ToLower(out.Error), "invalid call signature") {
		t.Fatalf("expected invalid call signature error, got %q", out.Error)
	}
}

func TestRunPython_AcceptsJSONStyleLiteralsViaCompatibilityAliases(t *testing.T) {
	python := mustFindPython(t)
	workspace := t.TempDir()
	inv := NewBuiltinCodeExecInvoker(workspace, map[string]string{"workspace": workspace})
	out, err := inv.runPython(context.Background(), python, workspace, codeExecRunConfig{
		Code:         `result = {"ok": true, "retry": false, "value": null}`,
		Allowlist:    []string{},
		MaxToolCalls: 2,
		TimeoutMs:    4_000,
		MaxOutput:    8 * 1024,
		Dispatch: func(_ context.Context, _ string, _ json.RawMessage) (types.HostOpRequest, error) {
			t.Fatalf("dispatch should not be called")
			return types.HostOpRequest{}, nil
		},
		Bridge: func(_ context.Context, _ types.HostOpRequest) types.HostOpResponse {
			t.Fatalf("bridge should not be called")
			return types.HostOpResponse{}
		},
		EnvAllowlist: inv.EnvAllowlist,
	})
	if err != nil {
		t.Fatalf("runPython error: %v", err)
	}
	if !out.OK {
		t.Fatalf("expected success with compatibility aliases, got %+v", out)
	}
	got, ok := out.Result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", out.Result)
	}
	if v, ok := got["ok"].(bool); !ok || !v {
		t.Fatalf("expected ok=true, got %#v", got["ok"])
	}
	if v, ok := got["retry"].(bool); !ok || v {
		t.Fatalf("expected retry=false, got %#v", got["retry"])
	}
	if v, exists := got["value"]; !exists || v != nil {
		t.Fatalf("expected value=null, got %#v", got["value"])
	}
}

func TestRunPython_BlocksDirectFilesystemWrites(t *testing.T) {
	python := mustFindPython(t)
	workspace := t.TempDir()
	inv := NewBuiltinCodeExecInvoker(workspace, map[string]string{"workspace": workspace})

	out, err := inv.runPython(context.Background(), python, workspace, codeExecRunConfig{
		Code:         `open("/workspace/a.txt", "w").write("x")`,
		Allowlist:    []string{"fs_write"},
		MaxToolCalls: 2,
		TimeoutMs:    4_000,
		MaxOutput:    8 * 1024,
		Dispatch: func(_ context.Context, _ string, _ json.RawMessage) (types.HostOpRequest, error) {
			t.Fatalf("dispatch should not be called for blocked direct write")
			return types.HostOpRequest{}, nil
		},
		Bridge: func(_ context.Context, _ types.HostOpRequest) types.HostOpResponse {
			t.Fatalf("bridge should not be called for blocked direct write")
			return types.HostOpResponse{}
		},
		EnvAllowlist: inv.EnvAllowlist,
	})
	if err != nil {
		t.Fatalf("runPython error: %v", err)
	}
	if out.OK {
		t.Fatalf("expected blocked direct write to fail, got %+v", out)
	}
	if out.ViolationType != codeExecPolicyDirectFSWrite || !out.PolicyViolation {
		t.Fatalf("expected direct_fs_write policy violation, got %+v", out)
	}
	if !strings.Contains(strings.ToLower(out.Error), "use tools.fs_write") {
		t.Fatalf("expected remediation guidance in error, got %q", out.Error)
	}
}

func TestRunPython_AllowsReadOnlyOpen(t *testing.T) {
	python := mustFindPython(t)
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	inv := NewBuiltinCodeExecInvoker(workspace, map[string]string{"workspace": workspace})

	out, err := inv.runPython(context.Background(), python, workspace, codeExecRunConfig{
		Code:         `result = open("/workspace/a.txt", "r").read()`,
		Allowlist:    []string{"fs_read"},
		MaxToolCalls: 2,
		TimeoutMs:    4_000,
		MaxOutput:    8 * 1024,
		Dispatch: func(_ context.Context, toolName string, args json.RawMessage) (types.HostOpRequest, error) {
			if toolName != "fs_read" {
				t.Fatalf("dispatch tool = %q, want fs_read", toolName)
			}
			var m struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(args, &m); err != nil {
				return types.HostOpRequest{}, err
			}
			return types.HostOpRequest{Op: types.HostOpFSRead, Path: m.Path}, nil
		},
		Bridge: func(_ context.Context, req types.HostOpRequest) types.HostOpResponse {
			if req.Op != types.HostOpFSRead || req.Path != "/workspace/a.txt" {
				t.Fatalf("bridge req = %+v", req)
			}
			return types.HostOpResponse{Op: req.Op, Ok: true, Text: "hello"}
		},
		EnvAllowlist: inv.EnvAllowlist,
	})
	if err != nil {
		t.Fatalf("runPython error: %v", err)
	}
	if !out.OK {
		t.Fatalf("expected read-only open to succeed, got %+v", out)
	}
	if got := strings.TrimSpace(fmt.Sprintf("%v", out.Result)); got != "hello" {
		t.Fatalf("result=%q want hello", got)
	}
}

func TestRunPython_VFSCompatShim_BlocksNonVFSPaths(t *testing.T) {
	// Non-VFS paths must not fall through to host; prevents sandbox escape.
	python := mustFindPython(t)
	workspace := t.TempDir()
	inv := NewBuiltinCodeExecInvoker(workspace, map[string]string{"workspace": workspace})

	for _, code := range []string{
		`open("/etc/passwd", "r")`,
		`import os; os.listdir("/")`,
		`open("relative.txt", "r")`,
	} {
		t.Run(code, func(t *testing.T) {
			out, err := inv.runPython(context.Background(), python, workspace, codeExecRunConfig{
				Code:         code,
				Allowlist:    []string{"fs_read", "fs_list"},
				MaxToolCalls: 2,
				TimeoutMs:    4_000,
				MaxOutput:    8 * 1024,
				Dispatch: func(_ context.Context, _ string, _ json.RawMessage) (types.HostOpRequest, error) {
					t.Fatalf("dispatch should not be called for blocked path")
					return types.HostOpRequest{}, nil
				},
				Bridge: func(_ context.Context, _ types.HostOpRequest) types.HostOpResponse {
					t.Fatalf("bridge should not be called for blocked path")
					return types.HostOpResponse{}
				},
				EnvAllowlist: inv.EnvAllowlist,
			})
			if err != nil {
				t.Fatalf("runPython error: %v", err)
			}
			if out.OK {
				t.Fatalf("expected blocked path to fail, got %+v", out)
			}
			if !strings.Contains(strings.ToLower(out.Error), "vfs") {
				t.Fatalf("expected VFS-related error, got %q", out.Error)
			}
		})
	}
}

func TestRunPython_PathAccessAllowlist_AllowsAccess(t *testing.T) {
	// path_access.allowlist permits access to dirs outside VFS.
	python := mustFindPython(t)
	workspace := t.TempDir()
	sharedDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(sharedDir, "data.txt"), []byte("from allowlist"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	inv := NewBuiltinCodeExecInvoker(workspace, map[string]string{"workspace": workspace})
	inv.SetPathAccess([]string{sharedDir}, true)

	filePath := filepath.Join(sharedDir, "data.txt")
	// Use forward slashes for Python string (works on all platforms)
	filePathPy := strings.ReplaceAll(filePath, "\\", "/")
	out, err := inv.runPython(context.Background(), python, workspace, codeExecRunConfig{
		Code:                `result = open("` + filePathPy + `", "r").read()`,
		Allowlist:           []string{},
		MaxToolCalls:        0,
		TimeoutMs:           4_000,
		MaxOutput:           8 * 1024,
		PathAccessAllowlist: []string{sharedDir},
		PathAccessReadOnly:  true,
		Dispatch:            nil,
		Bridge:              nil,
		EnvAllowlist:        inv.EnvAllowlist,
	})
	if err != nil {
		t.Fatalf("runPython error: %v", err)
	}
	if !out.OK {
		t.Fatalf("expected allowlisted path to succeed, got %+v", out)
	}
	if got := strings.TrimSpace(fmt.Sprintf("%v", out.Result)); got != "from allowlist" {
		t.Fatalf("result=%q want from allowlist", got)
	}
}

func TestRunPython_PathAccessAllowlist_BlocksPathsOutside(t *testing.T) {
	// With allowlist set, paths outside allowlist are still blocked.
	python := mustFindPython(t)
	workspace := t.TempDir()
	sharedDir := t.TempDir()
	inv := NewBuiltinCodeExecInvoker(workspace, map[string]string{"workspace": workspace})
	inv.SetPathAccess([]string{sharedDir}, true)

	out, err := inv.runPython(context.Background(), python, workspace, codeExecRunConfig{
		Code:              `open("/etc/passwd", "r")`,
		Allowlist:         []string{},
		MaxToolCalls:      0,
		TimeoutMs:         4_000,
		MaxOutput:         8 * 1024,
		PathAccessAllowlist: []string{sharedDir},
		PathAccessReadOnly:  true,
		Dispatch:            nil,
		Bridge:              nil,
		EnvAllowlist:        inv.EnvAllowlist,
	})
	if err != nil {
		t.Fatalf("runPython error: %v", err)
	}
	if out.OK {
		t.Fatalf("expected path outside allowlist to fail, got %+v", out)
	}
	if !strings.Contains(strings.ToLower(out.Error), "vfs") && !strings.Contains(strings.ToLower(out.Error), "allowlist") {
		t.Fatalf("expected VFS/allowlist error, got %q", out.Error)
	}
}

func TestRunPython_PathAccessAllowlist_ReadOnly_BlocksWrites(t *testing.T) {
	// When read_only=true, writes to allowlisted paths are blocked.
	python := mustFindPython(t)
	workspace := t.TempDir()
	sharedDir := t.TempDir()
	inv := NewBuiltinCodeExecInvoker(workspace, map[string]string{"workspace": workspace})
	inv.SetPathAccess([]string{sharedDir}, true)

	filePath := filepath.Join(sharedDir, "out.txt")
	filePathPy := strings.ReplaceAll(filePath, "\\", "/")
	out, err := inv.runPython(context.Background(), python, workspace, codeExecRunConfig{
		Code:                `open("` + filePathPy + `", "w").write("x")`,
		Allowlist:           []string{},
		MaxToolCalls:        0,
		TimeoutMs:           4_000,
		MaxOutput:           8 * 1024,
		PathAccessAllowlist: []string{sharedDir},
		PathAccessReadOnly:  true,
		Dispatch:            nil,
		Bridge:              nil,
		EnvAllowlist:        inv.EnvAllowlist,
	})
	if err != nil {
		t.Fatalf("runPython error: %v", err)
	}
	if out.OK {
		t.Fatalf("expected write to allowlisted path to fail when read_only, got %+v", out)
	}
	if !strings.Contains(strings.ToLower(out.Error), "read_only") {
		t.Fatalf("expected read_only error, got %q", out.Error)
	}
}

func TestRunPython_PathAccessAllowlist_ReadWrite_AllowsWrites(t *testing.T) {
	// When read_only=false, writes to allowlisted paths are allowed.
	python := mustFindPython(t)
	workspace := t.TempDir()
	sharedDir := t.TempDir()
	inv := NewBuiltinCodeExecInvoker(workspace, map[string]string{"workspace": workspace})
	inv.SetPathAccess([]string{sharedDir}, false)

	filePath := filepath.Join(sharedDir, "out.txt")
	filePathPy := strings.ReplaceAll(filePath, "\\", "/")
	out, err := inv.runPython(context.Background(), python, workspace, codeExecRunConfig{
		Code:                `open("` + filePathPy + `", "w").write("written"); result = open("` + filePathPy + `", "r").read()`,
		Allowlist:           []string{},
		MaxToolCalls:        0,
		TimeoutMs:           4_000,
		MaxOutput:           8 * 1024,
		PathAccessAllowlist: []string{sharedDir},
		PathAccessReadOnly:  false,
		Dispatch:            nil,
		Bridge:              nil,
		EnvAllowlist:        inv.EnvAllowlist,
	})
	if err != nil {
		t.Fatalf("runPython error: %v", err)
	}
	if !out.OK {
		t.Fatalf("expected write to allowlisted path to succeed when read_only=false, got %+v", out)
	}
	if got := strings.TrimSpace(fmt.Sprintf("%v", out.Result)); got != "written" {
		t.Fatalf("result=%q want written", got)
	}
}

func TestRunPython_VFSCompatShim_OpenAndListdir(t *testing.T) {
	// Models that use os/open instead of tools.fs_read/fs_list should still work.
	python := mustFindPython(t)
	workspace := t.TempDir()
	inv := NewBuiltinCodeExecInvoker(workspace, map[string]string{
		"project":   workspace,
		"workspace": workspace,
	})

	fsReadCalls := 0
	fsListCalls := 0
	out, err := inv.runPython(context.Background(), python, workspace, codeExecRunConfig{
		Code: `import os
# Model uses os.listdir (wrong pattern) - should route through bridge
entries = os.listdir("/workspace")
# Model uses open() (wrong pattern) - should route through bridge
content = open("/workspace/hello.txt", "r").read()
result = {"entries": entries, "content": content}`,
		Allowlist:    []string{"fs_read", "fs_list"},
		MaxToolCalls: 5,
		TimeoutMs:    4_000,
		MaxOutput:    8 * 1024,
		Dispatch: func(_ context.Context, toolName string, args json.RawMessage) (types.HostOpRequest, error) {
			toolName = strings.TrimSpace(toolName)
			var m struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(args, &m); err != nil {
				return types.HostOpRequest{}, err
			}
			path := m.Path
			if toolName == "fs_list" {
				return types.HostOpRequest{Op: types.HostOpFSList, Path: path}, nil
			}
			if toolName == "fs_read" {
				return types.HostOpRequest{Op: types.HostOpFSRead, Path: path}, nil
			}
			return types.HostOpRequest{}, fmt.Errorf("unknown tool %q", toolName)
		},
		Bridge: func(_ context.Context, req types.HostOpRequest) types.HostOpResponse {
			if req.Op == types.HostOpFSList {
				fsListCalls++
				return types.HostOpResponse{Op: req.Op, Ok: true, Entries: []string{"hello.txt", "other.txt"}}
			}
			if req.Op == types.HostOpFSRead {
				fsReadCalls++
				return types.HostOpResponse{Op: req.Op, Ok: true, Text: "hello from vfs"}
			}
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: "unknown op"}
		},
		EnvAllowlist: inv.EnvAllowlist,
	})
	if err != nil {
		t.Fatalf("runPython error: %v", err)
	}
	if !out.OK {
		t.Fatalf("expected VFS compat shim to succeed, got %+v", out)
	}
	if fsReadCalls != 1 {
		t.Fatalf("expected 1 fs_read bridge call, got %d", fsReadCalls)
	}
	if fsListCalls != 1 {
		t.Fatalf("expected 1 fs_list bridge call, got %d", fsListCalls)
	}
	got, ok := out.Result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map", out.Result)
	}
	content, _ := got["content"].(string)
	if content != "hello from vfs" {
		t.Fatalf("content=%q want hello from vfs", content)
	}
	entries, _ := got["entries"].([]any)
	if len(entries) != 2 {
		t.Fatalf("entries len=%d want 2, got %v", len(entries), entries)
	}
}

func TestRunPython_VFSCompatShim_SymlinkAllowlistBypass(t *testing.T) {
	// A symlink inside an allowlisted dir pointing outside must be blocked.
	python := mustFindPython(t)
	workspace := t.TempDir()
	allowedDir := t.TempDir()
	outsideDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outsideDir, "secret.txt"), []byte("secret-data"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	// Create symlink: allowedDir/escape -> outsideDir
	symlink := filepath.Join(allowedDir, "escape")
	if err := os.Symlink(outsideDir, symlink); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
	inv := NewBuiltinCodeExecInvoker(workspace, map[string]string{"workspace": workspace})

	escapePath := filepath.Join(allowedDir, "escape", "secret.txt")
	escapePathPy := strings.ReplaceAll(escapePath, "\\", "/")
	out, err := inv.runPython(context.Background(), python, workspace, codeExecRunConfig{
		Code:                `result = open("` + escapePathPy + `", "r").read()`,
		Allowlist:           []string{},
		MaxToolCalls:        0,
		TimeoutMs:           4_000,
		MaxOutput:           8 * 1024,
		PathAccessAllowlist: []string{allowedDir},
		PathAccessReadOnly:  true,
		Dispatch:            nil,
		Bridge:              nil,
		EnvAllowlist:        inv.EnvAllowlist,
	})
	if err != nil {
		t.Fatalf("runPython error: %v", err)
	}
	if out.OK {
		t.Fatalf("expected symlink escape to be blocked, got result=%v", out.Result)
	}
}

func TestRunPython_VFSCompatShim_ListdirCWD(t *testing.T) {
	// os.listdir() and os.listdir(".") should succeed (relative path fallthrough).
	python := mustFindPython(t)
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	inv := NewBuiltinCodeExecInvoker(workspace, map[string]string{"workspace": workspace})

	for _, code := range []string{
		`import os; result = os.listdir(".")`,
		`import os; result = os.listdir()`,
	} {
		t.Run(code, func(t *testing.T) {
			out, err := inv.runPython(context.Background(), python, workspace, codeExecRunConfig{
				Code:         code,
				Allowlist:    []string{},
				MaxToolCalls: 0,
				TimeoutMs:    4_000,
				MaxOutput:    8 * 1024,
				Dispatch:     nil,
				Bridge:       nil,
				EnvAllowlist: inv.EnvAllowlist,
			})
			if err != nil {
				t.Fatalf("runPython error: %v", err)
			}
			if !out.OK {
				t.Fatalf("expected os.listdir with relative path to succeed, got error=%q", out.Error)
			}
		})
	}
}

func TestRunPython_VFSCompatShim_OpenBinaryMode(t *testing.T) {
	// open(path, "rb") on VFS paths should return bytes, not str.
	python := mustFindPython(t)
	workspace := t.TempDir()
	inv := NewBuiltinCodeExecInvoker(workspace, map[string]string{"workspace": workspace})

	out, err := inv.runPython(context.Background(), python, workspace, codeExecRunConfig{
		Code:         `data = open("/workspace/f.bin", "rb").read(); result = {"type": type(data).__name__, "value": data.decode("utf-8")}`,
		Allowlist:    []string{"fs_read"},
		MaxToolCalls: 2,
		TimeoutMs:    4_000,
		MaxOutput:    8 * 1024,
		Dispatch: func(_ context.Context, toolName string, args json.RawMessage) (types.HostOpRequest, error) {
			var m struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(args, &m); err != nil {
				return types.HostOpRequest{}, err
			}
			return types.HostOpRequest{Op: types.HostOpFSRead, Path: m.Path}, nil
		},
		Bridge: func(_ context.Context, req types.HostOpRequest) types.HostOpResponse {
			return types.HostOpResponse{Op: req.Op, Ok: true, Text: "binary-content"}
		},
		EnvAllowlist: inv.EnvAllowlist,
	})
	if err != nil {
		t.Fatalf("runPython error: %v", err)
	}
	if !out.OK {
		t.Fatalf("expected binary open to succeed, got %+v", out)
	}
	got, ok := out.Result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map", out.Result)
	}
	if got["type"] != "bytes" {
		t.Fatalf("expected bytes type, got %q", got["type"])
	}
	if got["value"] != "binary-content" {
		t.Fatalf("expected binary-content, got %q", got["value"])
	}
}

func TestEnsureReady_MissingPythonBinary(t *testing.T) {
	inv := NewBuiltinCodeExecInvoker(t.TempDir(), map[string]string{"workspace": t.TempDir()})
	inv.PythonBin = "missing-python-binary-for-code-exec-tests"
	if err := inv.EnsureReady(context.Background()); err == nil {
		t.Fatalf("expected EnsureReady failure for missing python")
	}
}

func TestEnsureReady_MissingRequiredImport(t *testing.T) {
	python := mustFindPython(t)
	inv := NewBuiltinCodeExecInvoker(t.TempDir(), map[string]string{"workspace": t.TempDir()})
	inv.PythonBin = python
	inv.SetRequiredImports([]string{"module_that_should_not_exist_code_exec"})
	err := inv.EnsureReady(context.Background())
	if err == nil {
		t.Fatalf("expected missing import error")
	}
	if !strings.Contains(err.Error(), "missing python module") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureReady_MissingRequiredImportFromSetter(t *testing.T) {
	python := mustFindPython(t)
	inv := NewBuiltinCodeExecInvoker(t.TempDir(), map[string]string{"workspace": t.TempDir()})
	inv.PythonBin = python
	inv.SetRequiredImports([]string{"module_that_should_not_exist_code_exec_setter"})
	err := inv.EnsureReady(context.Background())
	if err == nil {
		t.Fatalf("expected missing import error")
	}
	if !strings.Contains(err.Error(), "missing python module") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureReady_Smoke(t *testing.T) {
	python := mustFindPython(t)
	inv := NewBuiltinCodeExecInvoker(t.TempDir(), map[string]string{"workspace": t.TempDir()})
	inv.PythonBin = python
	if err := inv.EnsureReady(context.Background()); err != nil {
		t.Fatalf("EnsureReady: %v", err)
	}
}

func TestRequiredCodeExecImports_MergesRuntimeDeterministically(t *testing.T) {
	got := requiredCodeExecImports([]string{"pandas", "a_mod", "requests"})
	expectedContains := []string{"a_mod", "contextlib", "io", "json", "pandas", "re", "requests"}
	for _, mod := range expectedContains {
		if !containsString(got, mod) {
			t.Fatalf("expected merged imports to include %q, got %v", mod, got)
		}
	}
	sorted := append([]string(nil), got...)
	sort.Strings(sorted)
	if strings.Join(sorted, ",") != strings.Join(got, ",") {
		t.Fatalf("expected deterministic lexical ordering, got %v", got)
	}
}

func containsString(items []string, needle string) bool {
	for _, item := range items {
		if item == needle {
			return true
		}
	}
	return false
}

func mustFindPython(t *testing.T) string {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not available")
	}
	return python
}
