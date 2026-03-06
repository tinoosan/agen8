package agent

import (
	"context"
	"encoding/json"
	"testing"

	pkgtools "github.com/tinoosan/agen8/pkg/tools"
	"github.com/tinoosan/agen8/pkg/types"
)

func TestHostOpExecutor_Pipe_ReadUpperWrite(t *testing.T) {
	exec := &HostOpExecutor{FS: newMountedWorkspaceFS(t)}
	if err := exec.FS.Write("/workspace/in.txt", []byte("hello\n")); err != nil {
		t.Fatal(err)
	}
	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op: types.HostOpPipe,
		PipeSteps: []types.PipeStep{
			{Type: "tool", Tool: types.HostOpFSRead, Args: map[string]any{"path": "/workspace/in.txt"}, Output: "text"},
			{Type: "transform", Transform: "uppercase"},
			{Type: "tool", Tool: types.HostOpFSWrite, Args: map[string]any{"path": "/workspace/out.txt"}, InputArg: "text"},
		},
		PipeOptions: &types.PipeOptions{Debug: true, MaxValueBytes: 4096},
	})
	if !resp.Ok {
		t.Fatalf("pipe failed: %#v", resp)
	}
	out, err := exec.FS.Read("/workspace/out.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "HELLO\n" {
		t.Fatalf("unexpected output: %q", string(out))
	}
	if len(resp.PipeStepResults) != 3 {
		t.Fatalf("expected debug step results, got %#v", resp.PipeStepResults)
	}
}

func TestHostOpExecutor_Pipe_HTTPJSONToWrite(t *testing.T) {
	inv := &stubToolInvoker{
		result: pkgtools.ToolCallResult{Output: json.RawMessage(`{"finalUrl":"https://example.com","status":200,"body":"{\"name\":\"Agen8\"}"}`)},
	}
	exec := &HostOpExecutor{FS: newMountedWorkspaceFS(t), HTTPInvoker: inv}
	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op: types.HostOpPipe,
		PipeSteps: []types.PipeStep{
			{Type: "tool", Tool: types.HostOpHTTPFetch, Args: map[string]any{"url": "https://example.com"}, Output: "body"},
			{Type: "transform", Transform: "json_parse"},
			{Type: "transform", Transform: "get", Field: "name"},
			{Type: "transform", Transform: "json_stringify"},
			{Type: "tool", Tool: types.HostOpFSWrite, Args: map[string]any{"path": "/workspace/name.json"}, InputArg: "text"},
		},
	})
	if !resp.Ok {
		t.Fatalf("pipe failed: %#v", resp)
	}
	out, err := exec.FS.Read("/workspace/name.json")
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `"Agen8"` {
		t.Fatalf("unexpected written payload: %q", string(out))
	}
}

func TestHostOpExecutor_Pipe_ShellTrim(t *testing.T) {
	inv := &stubToolInvoker{
		result: pkgtools.ToolCallResult{Output: json.RawMessage(`{"exitCode":0,"stdout":"  hi  ","stderr":"","warning":"","vfsPathTranslated":false,"vfsPathMounts":"","scriptPathNormalized":false,"scriptAntiPattern":""}`)},
	}
	exec := &HostOpExecutor{FS: newMountedWorkspaceFS(t), ShellInvoker: inv}
	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op: types.HostOpPipe,
		PipeSteps: []types.PipeStep{
			{Type: "tool", Tool: types.HostOpShellExec, Args: map[string]any{"command": "echo hi", "cwd": ".", "stdin": ""}, Output: "stdout"},
			{Type: "transform", Transform: "trim"},
		},
	})
	if !resp.Ok {
		t.Fatalf("pipe failed: %#v", resp)
	}
	if string(resp.PipeValue) != `"hi"` {
		t.Fatalf("unexpected final value: %s", string(resp.PipeValue))
	}
}

func TestHostOpExecutor_Pipe_FailsOnMissingSelector(t *testing.T) {
	exec := &HostOpExecutor{FS: newMountedWorkspaceFS(t)}
	if err := exec.FS.Write("/workspace/in.txt", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op: types.HostOpPipe,
		PipeSteps: []types.PipeStep{
			{Type: "tool", Tool: types.HostOpFSRead, Args: map[string]any{"path": "/workspace/in.txt"}, Output: "missing"},
		},
	})
	if resp.Ok || resp.PipeFailedAtStep != 1 {
		t.Fatalf("expected selector failure, got %#v", resp)
	}
}
