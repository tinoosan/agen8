package types

import (
	"encoding/json"
	"testing"
)

func TestHostOpRequest_TraceValidation_AllowsTraceActions(t *testing.T) {
	req := HostOpRequest{
		Op:     HostOpTrace,
		Action: "events.latest",
		Input:  json.RawMessage(`{}`),
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestHostOpRequest_TraceValidation_RejectsUnknownActions(t *testing.T) {
	req := HostOpRequest{
		Op:     HostOpTrace,
		Action: "write",
		Input:  json.RawMessage(`{}`),
	}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected error for unsupported action")
	}
}

func TestHostOpRequest_BrowserValidation_RequiresInput(t *testing.T) {
	req := HostOpRequest{Op: HostOpBrowser}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected error for missing input")
	}
}

func TestHostOpRequest_BrowserValidation_AllowsInput(t *testing.T) {
	req := HostOpRequest{
		Op:    HostOpBrowser,
		Input: json.RawMessage(`{"action":"start"}`),
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestHostOpRequest_EmailValidation_RequiresInput(t *testing.T) {
	req := HostOpRequest{Op: HostOpEmail}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected error for missing input")
	}
}

func TestHostOpRequest_EmailValidation_AllowsInput(t *testing.T) {
	req := HostOpRequest{
		Op:    HostOpEmail,
		Input: json.RawMessage(`{"to":"a@example.com","subject":"s","body":"b"}`),
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestHostOpRequest_NoopValidation_AllowsEmpty(t *testing.T) {
	req := HostOpRequest{Op: HostOpNoop}
	if err := req.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestHostOpRequest_CodeExecValidation_RequiresPythonAndCode(t *testing.T) {
	req := HostOpRequest{Op: HostOpCodeExec}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected error for missing language/code")
	}

	req = HostOpRequest{
		Op:       HostOpCodeExec,
		Language: "javascript",
		Code:     "console.log('x')",
	}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected error for non-python language")
	}

	req = HostOpRequest{
		Op:       HostOpCodeExec,
		Language: "python",
		Code:     "print('ok')",
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	req = HostOpRequest{
		Op:       HostOpCodeExec,
		Language: "python",
		Code:     "print('ok')",
		Cwd:      "..",
	}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected error for cwd escaping root")
	}
}

func TestHostOpRequest_FSStatValidation(t *testing.T) {
	req := HostOpRequest{Op: HostOpFSStat, Path: "/workspace/a.txt"}
	if err := req.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	req = HostOpRequest{Op: HostOpFSStat, Path: ""}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected error for empty path")
	}

	req = HostOpRequest{Op: HostOpFSStat, Path: "workspace/a.txt"}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected error for relative path")
	}
}

func TestHostOpRequest_FSPatchValidation_AllowsDryRunVerbose(t *testing.T) {
	req := HostOpRequest{
		Op:      HostOpFSPatch,
		Path:    "/workspace/a.txt",
		Text:    "@@ -1 +1 @@\n-old\n+new\n",
		DryRun:  true,
		Verbose: true,
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	req = HostOpRequest{
		Op:   HostOpFSPatch,
		Path: "/workspace/a.txt",
		Text: "",
	}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected error for empty patch text")
	}

	req = HostOpRequest{
		Op:   HostOpFSPatch,
		Path: "workspace/a.txt",
		Text: "@@ -1 +1 @@\n-old\n+new\n",
	}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected error for relative path")
	}
}

func TestHostOpRequest_FSWriteValidation_AllowsWriteVerifyFlags(t *testing.T) {
	req := HostOpRequest{
		Op:               HostOpFSWrite,
		Path:             "/workspace/a.txt",
		Text:             "hello",
		Verify:           true,
		Checksum:         "sha256",
		ChecksumExpected: "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
		Atomic:           true,
		Sync:             true,
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	req = HostOpRequest{
		Op:       HostOpFSWrite,
		Path:     "/workspace/a.txt",
		Text:     "hello",
		Checksum: "crc32",
	}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected error for unsupported checksum")
	}

	req = HostOpRequest{
		Op:               HostOpFSWrite,
		Path:             "/workspace/a.txt",
		Text:             "hello",
		ChecksumExpected: "5d41402abc4b2a76b9719d911017c592",
	}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected error when checksumExpected is set without checksum algorithm")
	}

	req = HostOpRequest{
		Op:               HostOpFSWrite,
		Path:             "/workspace/a.txt",
		Text:             "hello",
		Checksum:         "md5",
		ChecksumExpected: "invalid-hex",
	}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected error for invalid checksumExpected format")
	}
}
