package agent

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/agen8/pkg/resources"
	pkgtools "github.com/tinoosan/agen8/pkg/tools"
	"github.com/tinoosan/agen8/pkg/types"
	"github.com/tinoosan/agen8/pkg/vfs"
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

type corruptingResource struct {
	files map[string][]byte
}

func (r *corruptingResource) SupportsNestedList() bool { return true }

func (r *corruptingResource) List(path string) ([]vfs.Entry, error) {
	path = strings.Trim(strings.TrimSpace(path), "/")
	if path != "" {
		if _, ok := r.files[path]; !ok {
			return nil, os.ErrNotExist
		}
		return []vfs.Entry{vfs.NewFileEntry(path, int64(len(r.files[path])), time.Now())}, nil
	}
	out := make([]vfs.Entry, 0, len(r.files))
	for p, b := range r.files {
		out = append(out, vfs.NewFileEntry(p, int64(len(b)), time.Now()))
	}
	return out, nil
}

func (r *corruptingResource) Read(path string) ([]byte, error) {
	b, ok := r.files[strings.Trim(strings.TrimSpace(path), "/")]
	if !ok {
		return nil, os.ErrNotExist
	}
	out := append([]byte(nil), b...)
	if len(out) != 0 {
		out[len(out)-1] ^= 0x01
	}
	return out, nil
}

func (r *corruptingResource) Write(path string, data []byte) error {
	if r.files == nil {
		r.files = map[string][]byte{}
	}
	r.files[strings.Trim(strings.TrimSpace(path), "/")] = append([]byte(nil), data...)
	return nil
}

func (r *corruptingResource) Append(path string, data []byte) error {
	key := strings.Trim(strings.TrimSpace(path), "/")
	if r.files == nil {
		r.files = map[string][]byte{}
	}
	r.files[key] = append(r.files[key], data...)
	return nil
}

func newCorruptMountedWorkspaceFS(t *testing.T) *vfs.FS {
	t.Helper()
	fs := vfs.NewFS()
	if err := fs.Mount(vfs.MountWorkspace, &corruptingResource{files: map[string][]byte{}}); err != nil {
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

func TestHostOpExecutor_FSRead_ComputesChecksums(t *testing.T) {
	exec := &HostOpExecutor{FS: newMountedWorkspaceFS(t)}
	if err := exec.FS.Write("/workspace/a.txt", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:        types.HostOpFSRead,
		Path:      "/workspace/a.txt",
		Checksums: []string{"md5", "sha256", "md5"},
	})
	if !resp.Ok {
		t.Fatalf("expected success, got %#v", resp)
	}
	if len(resp.ReadChecksums) != 2 {
		t.Fatalf("expected 2 checksums, got %#v", resp.ReadChecksums)
	}
	if len(resp.ReadChecksums["md5"]) != 32 || len(resp.ReadChecksums["sha256"]) != 64 {
		t.Fatalf("unexpected digest lengths %#v", resp.ReadChecksums)
	}
}

func TestHostOpExecutor_FSWrite_VerifyChecksumSuccess(t *testing.T) {
	exec := &HostOpExecutor{FS: newMountedWorkspaceFS(t)}

	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:               types.HostOpFSWrite,
		Path:             "/workspace/a.txt",
		Text:             "hello\n",
		Verify:           true,
		Checksum:         "sha256",
		ChecksumExpected: "5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03",
		Atomic:           true,
		Sync:             true,
	})
	if !resp.Ok {
		t.Fatalf("expected success, got %#v", resp)
	}
	if resp.WriteVerified == nil || !*resp.WriteVerified {
		t.Fatalf("expected writeVerified=true, got %#v", resp)
	}
	if resp.WriteChecksumAlgo != "sha256" || len(resp.WriteChecksum) != 64 {
		t.Fatalf("expected sha256 checksum metadata, got %#v", resp)
	}
	if resp.WriteChecksumMatch == nil || !*resp.WriteChecksumMatch {
		t.Fatalf("expected checksumMatch=true, got %#v", resp)
	}
	if resp.WriteMode != "created" {
		t.Fatalf("expected writeMode=created, got %#v", resp)
	}
	if resp.WriteBytes == nil || *resp.WriteBytes != int64(6) {
		t.Fatalf("expected writeBytes=6, got %#v", resp)
	}
	if resp.WriteFinalSize == nil || *resp.WriteFinalSize != int64(6) {
		t.Fatalf("expected writeFinalSize=6, got %#v", resp)
	}
	if !resp.WriteAtomicRequested || !resp.WriteSyncRequested {
		t.Fatalf("expected atomic/sync requested flags, got %#v", resp)
	}
}

func TestHostOpExecutor_FSWrite_AllowsEmptyTextAndReportsOverwriteMetadata(t *testing.T) {
	exec := &HostOpExecutor{FS: newMountedWorkspaceFS(t)}
	if err := exec.FS.Write("/workspace/a.txt", []byte("non-empty")); err != nil {
		t.Fatal(err)
	}

	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:   types.HostOpFSWrite,
		Path: "/workspace/a.txt",
		Text: "",
	})
	if !resp.Ok {
		t.Fatalf("expected success, got %#v", resp)
	}
	if resp.WriteMode != "overwritten" {
		t.Fatalf("expected writeMode=overwritten, got %#v", resp)
	}
	if resp.WriteBytes == nil || *resp.WriteBytes != 0 {
		t.Fatalf("expected writeBytes=0, got %#v", resp)
	}
	if resp.WriteFinalSize == nil || *resp.WriteFinalSize != 0 {
		t.Fatalf("expected writeFinalSize=0, got %#v", resp)
	}
	b, err := exec.FS.Read("/workspace/a.txt")
	if err != nil {
		t.Fatalf("read truncated file: %v", err)
	}
	if len(b) != 0 {
		t.Fatalf("expected empty file after truncate, got %q", string(b))
	}
}

func TestHostOpExecutor_FSWrite_AppendMode(t *testing.T) {
	exec := &HostOpExecutor{FS: newMountedWorkspaceFS(t)}
	if err := exec.FS.Write("/workspace/a.txt", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:     types.HostOpFSWrite,
		Path:   "/workspace/a.txt",
		Text:   "\nworld",
		Mode:   "a",
		Verify: true,
	})
	if !resp.Ok {
		t.Fatalf("expected append success, got %#v", resp)
	}
	if resp.WriteRequestMode != "a" {
		t.Fatalf("expected writeRequestMode=a, got %#v", resp)
	}
	if resp.WriteMode != "appended" {
		t.Fatalf("expected writeMode=appended, got %#v", resp)
	}
	if resp.WriteBytes == nil || *resp.WriteBytes != int64(6) {
		t.Fatalf("expected writeBytes=6, got %#v", resp)
	}
	b, err := exec.FS.Read("/workspace/a.txt")
	if err != nil {
		t.Fatalf("read appended file: %v", err)
	}
	if got := string(b); got != "hello\nworld" {
		t.Fatalf("unexpected appended content %q", got)
	}
}

func TestHostOpExecutor_FSWrite_ChecksumExpectedMismatchFails(t *testing.T) {
	exec := &HostOpExecutor{FS: newMountedWorkspaceFS(t)}
	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:               types.HostOpFSWrite,
		Path:             "/workspace/a.txt",
		Text:             "hello\n",
		Checksum:         "sha256",
		ChecksumExpected: "0000000000000000000000000000000000000000000000000000000000000000",
	})
	if resp.Ok {
		t.Fatalf("expected checksum mismatch failure")
	}
	if resp.WriteChecksumMatch == nil || *resp.WriteChecksumMatch {
		t.Fatalf("expected checksumMatch=false, got %#v", resp)
	}
	if resp.WriteChecksumExpected == "" || resp.WriteChecksum == "" {
		t.Fatalf("expected checksum expected/actual metadata, got %#v", resp)
	}
	if !strings.Contains(resp.Error, "checksum mismatch") {
		t.Fatalf("expected checksum mismatch error, got %q", resp.Error)
	}
}

func TestHostOpExecutor_FSWrite_VerifyMismatchDiagnostics(t *testing.T) {
	exec := &HostOpExecutor{FS: newCorruptMountedWorkspaceFS(t)}

	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:     types.HostOpFSWrite,
		Path:   "/workspace/a.txt",
		Text:   "hello",
		Verify: true,
	})
	if resp.Ok {
		t.Fatalf("expected verify mismatch failure")
	}
	if resp.WriteVerified == nil || *resp.WriteVerified {
		t.Fatalf("expected writeVerified=false, got %#v", resp)
	}
	if resp.WriteMismatchAt == nil || resp.WriteExpectedBytes == nil || resp.WriteActualBytes == nil {
		t.Fatalf("expected mismatch diagnostics, got %#v", resp)
	}
	if !strings.Contains(resp.Error, "verify failed") {
		t.Fatalf("expected verify failure error, got %q", resp.Error)
	}
	if got, want := *resp.WriteExpectedBytes, int64(5); got != want {
		t.Fatalf("expected expectedBytes=%d, got %d", want, got)
	}
	if got, want := *resp.WriteActualBytes, int64(5); got != want {
		t.Fatalf("expected actualBytes=%d, got %d", want, got)
	}
	if got := *resp.WriteMismatchAt; got < 0 || got >= int64(5) {
		t.Fatalf("expected mismatch offset in [0,4], got %d", got)
	}
}

func TestHostOpExecutor_FSStat_FileAndDirectory(t *testing.T) {
	exec := &HostOpExecutor{FS: newMountedWorkspaceFS(t)}
	if err := exec.FS.Write("/workspace/a.txt", []byte("hello")); err != nil {
		t.Fatal(err)
	}

	resp := exec.Exec(context.Background(), types.HostOpRequest{Op: types.HostOpFSStat, Path: "/workspace/a.txt"})
	if !resp.Ok {
		t.Fatalf("expected success, got %#v", resp)
	}
	if resp.IsDir == nil || *resp.IsDir {
		t.Fatalf("expected file stat with isDir=false, got %#v", resp)
	}
	if resp.Exists == nil || !*resp.Exists {
		t.Fatalf("expected exists=true for file stat, got %#v", resp)
	}
	if resp.SizeBytes == nil || *resp.SizeBytes != int64(5) {
		t.Fatalf("expected sizeBytes=5, got %#v", resp)
	}
	if resp.Mtime == nil {
		t.Fatalf("expected mtime for file stat, got %#v", resp)
	}

	resp = exec.Exec(context.Background(), types.HostOpRequest{Op: types.HostOpFSStat, Path: "/workspace"})
	if !resp.Ok {
		t.Fatalf("expected success, got %#v", resp)
	}
	if resp.IsDir == nil || !*resp.IsDir {
		t.Fatalf("expected directory stat with isDir=true, got %#v", resp)
	}
	if resp.Exists == nil || !*resp.Exists {
		t.Fatalf("expected exists=true for directory stat, got %#v", resp)
	}
	if resp.SizeBytes != nil {
		t.Fatalf("expected nil sizeBytes for directory stat, got %#v", resp)
	}
}

func TestHostOpExecutor_FSStat_MissingPathReturnsExistsFalse(t *testing.T) {
	exec := &HostOpExecutor{FS: newMountedWorkspaceFS(t)}
	resp := exec.Exec(context.Background(), types.HostOpRequest{Op: types.HostOpFSStat, Path: "/workspace/missing.txt"})
	if !resp.Ok {
		t.Fatalf("expected non-throwing missing stat response, got %#v", resp)
	}
	if resp.Exists == nil || *resp.Exists {
		t.Fatalf("expected exists=false for missing path, got %#v", resp)
	}
}

func TestHostOpExecutor_FSPatch_ApplySuccessReturnsDiagnostics(t *testing.T) {
	exec := &HostOpExecutor{FS: newMountedWorkspaceFS(t)}
	if err := exec.FS.Write("/workspace/a.txt", []byte("alpha\nbeta\n")); err != nil {
		t.Fatal(err)
	}

	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:   types.HostOpFSPatch,
		Path: "/workspace/a.txt",
		Text: "@@ -1,2 +1,2 @@\n alpha\n-beta\n+gamma\n",
	})
	if !resp.Ok {
		t.Fatalf("expected success, got %#v", resp)
	}
	if resp.PatchDryRun {
		t.Fatalf("expected apply mode, got dry-run response %#v", resp)
	}
	if resp.PatchDiagnostics == nil || resp.PatchDiagnostics.HunksApplied != 1 || resp.PatchDiagnostics.HunksTotal != 1 {
		t.Fatalf("expected patch diagnostics summary, got %#v", resp)
	}
	b, err := exec.FS.Read("/workspace/a.txt")
	if err != nil {
		t.Fatalf("read patched file: %v", err)
	}
	if got := string(b); got != "alpha\ngamma\n" {
		t.Fatalf("patched file content=%q", got)
	}
}

func TestHostOpExecutor_FSPatch_DryRunDoesNotWrite(t *testing.T) {
	exec := &HostOpExecutor{FS: newMountedWorkspaceFS(t)}
	if err := exec.FS.Write("/workspace/a.txt", []byte("alpha\nbeta\n")); err != nil {
		t.Fatal(err)
	}

	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:      types.HostOpFSPatch,
		Path:    "/workspace/a.txt",
		Text:    "@@ -1,2 +1,2 @@\n alpha\n-beta\n+gamma\n",
		DryRun:  true,
		Verbose: true,
	})
	if !resp.Ok {
		t.Fatalf("expected dry-run success, got %#v", resp)
	}
	if !resp.PatchDryRun {
		t.Fatalf("expected patchDryRun=true, got %#v", resp)
	}
	if resp.PatchDiagnostics == nil || resp.PatchDiagnostics.Mode != "dry_run" || resp.PatchDiagnostics.HunksApplied != 1 {
		t.Fatalf("unexpected diagnostics for dry-run %#v", resp)
	}
	b, err := exec.FS.Read("/workspace/a.txt")
	if err != nil {
		t.Fatalf("read file after dry-run: %v", err)
	}
	if got := string(b); got != "alpha\nbeta\n" {
		t.Fatalf("dry-run mutated file: %q", got)
	}
}

func TestHostOpExecutor_FSPatch_FailureReturnsDiagnostics(t *testing.T) {
	exec := &HostOpExecutor{FS: newMountedWorkspaceFS(t)}
	if err := exec.FS.Write("/workspace/a.txt", []byte("alpha\nbeta\n")); err != nil {
		t.Fatal(err)
	}

	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:      types.HostOpFSPatch,
		Path:    "/workspace/a.txt",
		Text:    "@@ -1,2 +1,2 @@\n gamma\n-beta\n+delta\n",
		Verbose: true,
	})
	if resp.Ok {
		t.Fatalf("expected failure for context mismatch")
	}
	if resp.PatchDiagnostics == nil {
		t.Fatalf("expected patch diagnostics on failure, got %#v", resp)
	}
	if resp.PatchDiagnostics.FailureReason != "context_mismatch" {
		t.Fatalf("unexpected failure reason %#v", resp.PatchDiagnostics)
	}
	if resp.PatchDiagnostics.FailedHunk != 1 || resp.PatchDiagnostics.TargetLine != 1 {
		t.Fatalf("unexpected failed hunk/line %#v", resp.PatchDiagnostics)
	}
}

func TestHostOpExecutor_FSTxn_DryRunDoesNotWrite(t *testing.T) {
	exec := &HostOpExecutor{FS: newMountedWorkspaceFS(t)}
	req := types.HostOpRequest{
		Op: types.HostOpFSTxn,
		TxnSteps: []types.FSTxnStep{
			{Op: types.HostOpFSWrite, Path: "/workspace/a.txt", Text: "hello"},
			{Op: types.HostOpFSPatch, Path: "/workspace/a.txt", Text: "@@ -1 +1 @@\n-hello\n+hi\n"},
		},
	}
	resp := exec.Exec(context.Background(), req)
	if !resp.Ok {
		t.Fatalf("expected dry-run success, got %#v", resp)
	}
	if resp.TxnDiagnostics == nil || resp.TxnDiagnostics.ApplyMode != "dry_run" || resp.TxnDiagnostics.StepsApplied != 2 {
		t.Fatalf("unexpected txn diagnostics %#v", resp.TxnDiagnostics)
	}
	if _, err := exec.FS.Read("/workspace/a.txt"); err == nil {
		t.Fatalf("dry-run should not write file")
	}
}

func TestHostOpExecutor_FSTxn_ApplySuccess(t *testing.T) {
	exec := &HostOpExecutor{FS: newMountedWorkspaceFS(t)}
	req := types.HostOpRequest{
		Op: types.HostOpFSTxn,
		TxnSteps: []types.FSTxnStep{
			{Op: types.HostOpFSWrite, Path: "/workspace/a.txt", Text: "hello\n"},
			{Op: types.HostOpFSAppend, Path: "/workspace/a.txt", Text: "world\n"},
		},
		TxnOptions: &types.FSTxnOptions{Apply: true},
	}
	resp := exec.Exec(context.Background(), req)
	if !resp.Ok {
		t.Fatalf("expected apply success, got %#v", resp)
	}
	if resp.TxnDiagnostics == nil || resp.TxnDiagnostics.ApplyMode != "apply" || resp.TxnDiagnostics.StepsApplied != 2 {
		t.Fatalf("unexpected txn diagnostics %#v", resp.TxnDiagnostics)
	}
	got, err := exec.FS.Read("/workspace/a.txt")
	if err != nil {
		t.Fatalf("read applied file: %v", err)
	}
	if string(got) != "hello\nworld\n" {
		t.Fatalf("unexpected applied file content %q", string(got))
	}
}

func TestHostOpExecutor_FSTxn_ApplyFailureRollsBack(t *testing.T) {
	exec := &HostOpExecutor{FS: newMountedWorkspaceFS(t)}
	req := types.HostOpRequest{
		Op: types.HostOpFSTxn,
		TxnSteps: []types.FSTxnStep{
			{Op: types.HostOpFSWrite, Path: "/workspace/a.txt", Text: "hello\n"},
			{Op: types.HostOpFSPatch, Path: "/workspace/a.txt", Text: "@@ -1 +1 @@\n-missing\n+new\n"},
		},
		TxnOptions: &types.FSTxnOptions{Apply: true, RollbackOnError: true},
	}
	resp := exec.Exec(context.Background(), req)
	if resp.Ok {
		t.Fatalf("expected txn failure, got %#v", resp)
	}
	if resp.TxnDiagnostics == nil || resp.TxnDiagnostics.FailedStep != 2 || !resp.TxnDiagnostics.RollbackPerformed || resp.TxnDiagnostics.RollbackFailed {
		t.Fatalf("unexpected txn diagnostics %#v", resp.TxnDiagnostics)
	}
	if _, err := exec.FS.Read("/workspace/a.txt"); err == nil {
		t.Fatalf("expected created file to be removed by rollback")
	}
}

func TestHostOpExecutor_FSTxn_ApplyFailureRollbackFailureSurfaced(t *testing.T) {
	exec := &HostOpExecutor{FS: newCorruptMountedWorkspaceFS(t)}
	req := types.HostOpRequest{
		Op: types.HostOpFSTxn,
		TxnSteps: []types.FSTxnStep{
			{Op: types.HostOpFSWrite, Path: "/workspace/a.txt", Text: "hello\n"},
			{Op: types.HostOpFSPatch, Path: "/workspace/a.txt", Text: "@@ -1 +1 @@\n-missing\n+new\n"},
		},
		TxnOptions: &types.FSTxnOptions{Apply: true, RollbackOnError: true},
	}
	resp := exec.Exec(context.Background(), req)
	if resp.Ok {
		t.Fatalf("expected txn failure with rollback failure, got %#v", resp)
	}
	if resp.TxnDiagnostics == nil || !resp.TxnDiagnostics.RollbackPerformed || !resp.TxnDiagnostics.RollbackFailed || len(resp.TxnDiagnostics.RollbackErrors) == 0 {
		t.Fatalf("expected rollback failure diagnostics, got %#v", resp.TxnDiagnostics)
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

func TestHostOpExecutor_CodeExec_ResponseMapping(t *testing.T) {
	inv := &stubToolInvoker{
		result: pkgtools.ToolCallResult{Output: json.RawMessage(`{"ok":true,"stdout":"hi","stderr":"","toolCallCount":2,"runtimeMs":11,"exitCode":0}`)},
	}
	exec := &HostOpExecutor{FS: newMountedWorkspaceFS(t), CodeExecInvoker: inv}
	req := types.HostOpRequest{
		Op:        types.HostOpCodeExec,
		Language:  "python",
		Code:      "print('hi')",
		Cwd:       "/workspace",
		TimeoutMs: 2000,
		MaxBytes:  8192,
		Input:     json.RawMessage(`{"maxToolCalls":5}`),
	}
	resp := exec.Exec(context.Background(), req)
	if !resp.Ok {
		t.Fatalf("expected success, got %#v", resp)
	}
	if resp.Stdout != "hi" || resp.ExitCode != 0 {
		t.Fatalf("unexpected code_exec response mapping %#v", resp)
	}

	var sent map[string]any
	if err := json.Unmarshal(inv.req.Input, &sent); err != nil {
		t.Fatalf("unmarshal forwarded payload: %v", err)
	}
	if sent["language"] != "python" {
		t.Fatalf("expected forwarded language python, got %#v", sent["language"])
	}
	if sent["maxToolCalls"] != float64(5) {
		t.Fatalf("expected forwarded maxToolCalls=5, got %#v", sent["maxToolCalls"])
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
