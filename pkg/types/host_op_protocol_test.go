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

func TestHostOpRequest_FSReadValidation_AllowsChecksums(t *testing.T) {
	req := HostOpRequest{Op: HostOpFSRead, Path: "/workspace/a.txt", Checksum: "sha256"}
	if err := req.Validate(); err != nil {
		t.Fatalf("Validate single checksum: %v", err)
	}

	req = HostOpRequest{Op: HostOpFSRead, Path: "/workspace/a.txt", Checksums: []string{"md5", "sha1"}}
	if err := req.Validate(); err != nil {
		t.Fatalf("Validate checksum list: %v", err)
	}

	req = HostOpRequest{Op: HostOpFSRead, Path: "/workspace/a.txt", Checksums: []string{"md5", "crc32"}}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected error for unsupported checksum in list")
	}
}

func TestHostOpRequest_FSSearchValidation_AllowsQueryOrPattern(t *testing.T) {
	req := HostOpRequest{Op: HostOpFSSearch, Path: "/workspace", Query: "needle", Limit: 5}
	if err := req.Validate(); err != nil {
		t.Fatalf("Validate query search: %v", err)
	}

	req = HostOpRequest{
		Op:              HostOpFSSearch,
		Path:            "/workspace",
		Pattern:         `n.*e`,
		Limit:           5,
		PreviewLines:    2,
		IncludeMetadata: true,
		MaxSizeBytes:    2048,
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("Validate pattern search: %v", err)
	}

	req = HostOpRequest{Op: HostOpFSSearch, Path: "/workspace"}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected error when query and pattern are both empty")
	}

	req = HostOpRequest{Op: HostOpFSSearch, Path: "/workspace", Query: "needle", PreviewLines: -1}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected error for negative previewLines")
	}

	req = HostOpRequest{Op: HostOpFSSearch, Path: "/workspace", Query: "needle", MaxSizeBytes: -1}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected error for negative maxSizeBytes")
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
		Op:   HostOpFSWrite,
		Path: "/workspace/a.txt",
		Text: "hello",
		Mode: "a",
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("Validate append mode: %v", err)
	}

	req = HostOpRequest{
		Op:   HostOpFSWrite,
		Path: "/workspace/empty.txt",
		Text: "",
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("Validate empty fs_write text: %v", err)
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

	req = HostOpRequest{
		Op:   HostOpFSWrite,
		Path: "/workspace/a.txt",
		Text: "hello",
		Mode: "appendd",
	}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected error for invalid mode")
	}
}

func TestHostOpRequest_FSTxnValidation(t *testing.T) {
	req := HostOpRequest{
		Op: HostOpFSTxn,
		TxnSteps: []FSTxnStep{
			{Op: HostOpFSWrite, Path: "/workspace/a.txt", Text: "hello"},
			{Op: HostOpFSPatch, Path: "/workspace/a.txt", Text: "@@ -1 +1 @@\n-hello\n+hi\n"},
		},
		TxnOptions: &FSTxnOptions{DryRun: true},
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	req = HostOpRequest{
		Op:       HostOpFSTxn,
		TxnSteps: []FSTxnStep{},
	}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected error for empty txnSteps")
	}

	req = HostOpRequest{
		Op: HostOpFSTxn,
		TxnSteps: []FSTxnStep{
			{Op: "shell_exec", Path: "/workspace/a.txt"},
		},
	}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected error for unsupported txn step op")
	}

	req = HostOpRequest{
		Op: HostOpFSTxn,
		TxnSteps: []FSTxnStep{
			{Op: HostOpFSWrite, Path: "workspace/a.txt", Text: "hello"},
		},
	}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected error for relative txn step path")
	}
}

func TestHostOpRequest_FSBatchEditValidation(t *testing.T) {
	req := HostOpRequest{
		Op:   HostOpFSBatchEdit,
		Path: "/knowledge",
		Glob: "**/*.md",
		BatchEditEdits: []BatchEdit{
			{Old: "[[old]]", New: "[[new]]", Occurrence: "all"},
			{Old: "old", New: "new", Occurrence: "2"},
		},
		BatchEditOptions: &BatchOptions{DryRun: true, MaxFiles: 10},
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	req = HostOpRequest{
		Op:             HostOpFSBatchEdit,
		Path:           "/knowledge",
		BatchEditEdits: []BatchEdit{{Old: "old", New: "new"}},
	}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected error for missing glob")
	}

	req = HostOpRequest{
		Op:             HostOpFSBatchEdit,
		Path:           "knowledge",
		Glob:           "**/*.md",
		BatchEditEdits: []BatchEdit{{Old: "old", New: "new"}},
	}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected error for relative path")
	}

	req = HostOpRequest{
		Op:             HostOpFSBatchEdit,
		Path:           "/knowledge",
		Glob:           "**/*.md",
		BatchEditEdits: []BatchEdit{{Old: "old", New: "new", Occurrence: "zero"}},
	}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected error for invalid occurrence")
	}

	req = HostOpRequest{
		Op:             HostOpFSBatchEdit,
		Path:           "/knowledge",
		Glob:           "**/*.md",
		BatchEditEdits: []BatchEdit{{Old: "old", New: "new"}},
		BatchEditOptions: &BatchOptions{
			MaxFiles: -1,
		},
	}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected error for negative maxFiles")
	}
}

func TestHostOpRequest_PipeValidation(t *testing.T) {
	req := HostOpRequest{
		Op: HostOpPipe,
		PipeSteps: []PipeStep{
			{Type: "tool", Tool: HostOpFSRead, Args: map[string]any{"path": "/workspace/a.txt"}, Output: "text"},
			{Type: "transform", Transform: "trim"},
		},
		PipeOptions: &PipeOptions{Debug: true, MaxSteps: 4, MaxValueBytes: 2048},
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	req = HostOpRequest{Op: HostOpPipe}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected error for empty pipeSteps")
	}

	req = HostOpRequest{
		Op: HostOpPipe,
		PipeSteps: []PipeStep{
			{Type: "tool", Tool: HostOpCodeExec, Args: map[string]any{"code": "print(1)"}},
		},
	}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected error for unsupported nested tool")
	}

	req = HostOpRequest{
		Op: HostOpPipe,
		PipeSteps: []PipeStep{
			{Type: "transform", Transform: "map"},
		},
	}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected error for unsupported transform")
	}

	req = HostOpRequest{
		Op: HostOpPipe,
		PipeSteps: []PipeStep{
			{Type: "tool", Tool: HostOpFSRead, Args: map[string]any{"path": "/workspace/a.txt"}, Output: "results[0]"},
		},
	}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected error for array indexing selector")
	}

	req = HostOpRequest{
		Op: HostOpPipe,
		PipeSteps: []PipeStep{
			{Type: "transform", Transform: "trim"},
		},
		PipeOptions: &PipeOptions{MaxSteps: 0, MaxValueBytes: -1},
	}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected error for invalid maxValueBytes")
	}
}

func TestHostOpRequest_FSArchiveValidation(t *testing.T) {
	req := HostOpRequest{
		Op:              HostOpFSArchiveCreate,
		Path:            "/workspace/data",
		Destination:     "/workspace/data.zip",
		Format:          "zip",
		IncludeMetadata: true,
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("archive create validate: %v", err)
	}

	req = HostOpRequest{
		Op:          HostOpFSArchiveExtract,
		Path:        "/workspace/data.zip",
		Destination: "/workspace/out",
		Pattern:     "*.md",
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("archive extract validate: %v", err)
	}

	req = HostOpRequest{
		Op:    HostOpFSArchiveList,
		Path:  "/workspace/data.zip",
		Limit: 10,
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("archive list validate: %v", err)
	}

	req = HostOpRequest{Op: HostOpFSArchiveCreate, Path: "/workspace/data", Destination: "/workspace/out.zip", Format: "tar.bz2"}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected error for unsupported archive format")
	}

	req = HostOpRequest{Op: HostOpFSArchiveExtract, Path: "/workspace/data.zip", Destination: "workspace/out"}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected error for relative destination")
	}

	req = HostOpRequest{Op: HostOpFSArchiveList, Path: "/workspace/data.zip", Limit: -1}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected error for negative list limit")
	}
}
