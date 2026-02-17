package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tinoosan/workbench-core/pkg/resources"
	pkgtools "github.com/tinoosan/workbench-core/pkg/tools"
	"github.com/tinoosan/workbench-core/pkg/types"
	"github.com/tinoosan/workbench-core/pkg/vfs"
)

type stubToolInvoker struct {
	result pkgtools.ToolCallResult
	err    error
	req    pkgtools.ToolRequest
}

func (s *stubToolInvoker) Invoke(_ context.Context, req pkgtools.ToolRequest) (pkgtools.ToolCallResult, error) {
	s.req = req
	return s.result, s.err
}

type stubEmailSender struct {
	to      string
	subject string
	body    string
	err     error
}

func (s *stubEmailSender) Send(to, subject, body string) error {
	s.to = to
	s.subject = subject
	s.body = body
	return s.err
}

func newMountedWorkspaceFS(t *testing.T) *vfs.FS {
	t.Helper()
	fs := vfs.NewFS()
	res, err := resources.NewDirResource(t.TempDir(), vfs.MountWorkspace)
	if err != nil {
		t.Fatalf("new workspace resource: %v", err)
	}
	if err := fs.Mount(vfs.MountWorkspace, res); err != nil {
		t.Fatalf("mount workspace: %v", err)
	}
	return fs
}

func TestHostOpExecutor_UnknownOp(t *testing.T) {
	exec := &HostOpExecutor{FS: newMountedWorkspaceFS(t)}
	resp := exec.Exec(context.Background(), types.HostOpRequest{Op: "unknown.op"})
	if resp.Ok {
		t.Fatalf("expected failure for unknown op")
	}
	if !strings.Contains(resp.Error, `unknown op "unknown.op"`) {
		t.Fatalf("unexpected error: %q", resp.Error)
	}
}

func TestHostOpExecutor_GuardsApplyBeforeDispatch(t *testing.T) {
	var nilExec *HostOpExecutor
	resp := nilExec.Exec(context.Background(), types.HostOpRequest{Op: types.HostOpNoop})
	if resp.Ok || resp.Error != "host executor missing FS" {
		t.Fatalf("expected missing FS guard, got %#v", resp)
	}

	exec := &HostOpExecutor{FS: vfs.NewFS()}
	resp = exec.Exec(context.Background(), types.HostOpRequest{Op: types.HostOpToolResult})
	if resp.Ok || resp.Error == "" {
		t.Fatalf("expected validate error for empty tool_result text, got %#v", resp)
	}
}

func TestHostOpExecutor_RegistryKnownOp(t *testing.T) {
	if _, ok := mockHostOperationFor(types.HostOpNoop); !ok {
		t.Fatalf("expected noop op to be registered")
	}
	exec := &HostOpExecutor{FS: newMountedWorkspaceFS(t)}
	resp := exec.Exec(context.Background(), types.HostOpRequest{Op: types.HostOpNoop, Text: "hello"})
	if !resp.Ok || resp.Text != "hello" {
		t.Fatalf("unexpected noop response %#v", resp)
	}
}

func TestHostOpExecutor_FSRead_TextBinaryAndTruncation(t *testing.T) {
	exec := &HostOpExecutor{FS: newMountedWorkspaceFS(t), DefaultMaxBytes: 5}

	if err := exec.FS.Write("/workspace/a.txt", []byte("hello world")); err != nil {
		t.Fatal(err)
	}
	resp := exec.Exec(context.Background(), types.HostOpRequest{Op: types.HostOpFSRead, Path: "/workspace/a.txt"})
	if !resp.Ok {
		t.Fatalf("expected success, got %#v", resp)
	}
	if !resp.Truncated || resp.Text != "hello" {
		t.Fatalf("expected truncated text payload, got %#v", resp)
	}

	if err := exec.FS.Write("/workspace/b.bin", []byte{0xff, 0xfe, 0xfd}); err != nil {
		t.Fatal(err)
	}
	resp = exec.Exec(context.Background(), types.HostOpRequest{Op: types.HostOpFSRead, Path: "/workspace/b.bin", MaxBytes: 3})
	if !resp.Ok {
		t.Fatalf("expected success, got %#v", resp)
	}
	if resp.Text != "" || resp.BytesB64 == "" {
		t.Fatalf("expected base64 binary payload, got %#v", resp)
	}
}

func TestHostOpExecutor_ShellExec_ResponseMapping(t *testing.T) {
	inv := &stubToolInvoker{
		result: pkgtools.ToolCallResult{Output: json.RawMessage(`{"exitCode":2,"stdout":"","stderr":"boom","warning":"","vfsPathTranslated":false,"vfsPathMounts":"  ","scriptPathNormalized":false,"scriptAntiPattern":" "}`)},
	}
	exec := &HostOpExecutor{FS: newMountedWorkspaceFS(t), ShellInvoker: inv}
	resp := exec.Exec(context.Background(), types.HostOpRequest{Op: types.HostOpShellExec, Argv: []string{"false"}})
	if resp.Ok {
		t.Fatalf("expected failed shell response")
	}
	if resp.Error != "boom" || resp.ExitCode != 2 {
		t.Fatalf("unexpected shell mapping %#v", resp)
	}
}

func TestHostOpExecutor_HTTPFetch_ResponseMapping(t *testing.T) {
	inv := &stubToolInvoker{
		result: pkgtools.ToolCallResult{Output: json.RawMessage(`{"finalUrl":"https://example.com","status":200,"headers":{"x":["y"]},"contentType":"text/plain","bytesRead":12,"truncated":true,"body":"ok","bodyTruncated":false,"warning":"warn"}`)},
	}
	exec := &HostOpExecutor{FS: newMountedWorkspaceFS(t), HTTPInvoker: inv}
	resp := exec.Exec(context.Background(), types.HostOpRequest{Op: types.HostOpHTTPFetch, URL: "https://example.com"})
	if !resp.Ok {
		t.Fatalf("expected success, got %#v", resp)
	}
	if resp.Status != 200 || resp.FinalURL != "https://example.com" || !resp.Truncated || resp.Warning != "warn" {
		t.Fatalf("unexpected http mapping %#v", resp)
	}
}

func TestHostOpExecutor_Email_MissingAndSuccess(t *testing.T) {
	exec := &HostOpExecutor{FS: newMountedWorkspaceFS(t)}
	req := types.HostOpRequest{Op: types.HostOpEmail, Input: json.RawMessage(`{"to":"a@example.com","subject":"s","body":"b"}`)}
	resp := exec.Exec(context.Background(), req)
	if resp.Ok || !strings.Contains(resp.Error, "email not configured") {
		t.Fatalf("expected missing client error, got %#v", resp)
	}

	mail := &stubEmailSender{}
	exec.EmailClient = mail
	resp = exec.Exec(context.Background(), req)
	if !resp.Ok {
		t.Fatalf("expected success, got %#v", resp)
	}
	if mail.to != "a@example.com" || mail.subject != "s" || mail.body != "b" {
		t.Fatalf("unexpected sent payload: %+v", mail)
	}
}

func TestHostOpExecutor_Trace_MissingAndSuccess(t *testing.T) {
	req := types.HostOpRequest{
		Op:     types.HostOpTrace,
		Action: "events.latest",
		Input:  json.RawMessage(`{"limit":1}`),
	}
	exec := &HostOpExecutor{FS: newMountedWorkspaceFS(t)}
	resp := exec.Exec(context.Background(), req)
	if resp.Ok || resp.Error != "trace invoker not configured" {
		t.Fatalf("unexpected missing trace response %#v", resp)
	}

	inv := &stubToolInvoker{result: pkgtools.ToolCallResult{Output: json.RawMessage(`{"ok":true}`)}}
	exec.TraceInvoker = inv
	resp = exec.Exec(context.Background(), req)
	if !resp.Ok || resp.Text != `{"ok":true}` {
		t.Fatalf("unexpected trace response %#v", resp)
	}
}
