package runtime

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/tinoosan/agen8/pkg/agent"
	"github.com/tinoosan/agen8/pkg/events"
	"github.com/tinoosan/agen8/pkg/resources"
	"github.com/tinoosan/agen8/pkg/types"
	"github.com/tinoosan/agen8/pkg/vfs"
)

func TestGuardMiddleware_ShortCircuits(t *testing.T) {
	called := false
	base := types.HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		called = true
		return types.HostOpResponse{Op: req.Op, Ok: true}
	})
	exec := ChainExecutor(base, &guardMiddleware{
		guard: func(req types.HostOpRequest) *types.HostOpResponse {
			return &types.HostOpResponse{Op: req.Op, Ok: false, Error: "blocked"}
		},
	})
	resp := exec.Exec(context.Background(), types.HostOpRequest{Op: types.HostOpFSRead, Path: "/workspace"})
	if called {
		t.Fatalf("expected base executor not to run")
	}
	if resp.Ok || resp.Error != "blocked" {
		t.Fatalf("expected blocked response, got %+v", resp)
	}
}

func TestDiffMiddleware_EmitsPatchPreview(t *testing.T) {
	dir := t.TempDir()
	res, err := resources.NewWorkdirResource(dir)
	if err != nil {
		t.Fatalf("workdir resource: %v", err)
	}
	fs := vfs.NewFS()
	if err := fs.Mount(vfs.MountProject, res); err != nil {
		t.Fatalf("mount project: %v", err)
	}

	base := &agent.HostOpExecutor{FS: fs, DefaultMaxBytes: 4096}

	var got events.Event
	exec := NewExecutor(base, ExecutorOptions{
		Emit: func(ctx context.Context, ev events.Event) {
			if ev.Type == "agent.op.response" {
				got = ev
			}
		},
		FS: fs,
	})

	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:   types.HostOpFSWrite,
		Path: "/project/hello.txt",
		Text: "hello",
	})
	if !resp.Ok {
		t.Fatalf("expected ok response, got %+v", resp)
	}
	preview := got.Data["patchPreview"]
	if !strings.Contains(preview, "+hello") {
		t.Fatalf("expected patch preview to include write diff, got %q", preview)
	}

	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("temp dir should exist: %v", err)
	}
}

func TestEventMiddleware_EmailReqDataIncludesToAndSubject(t *testing.T) {
	base := types.HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		return types.HostOpResponse{Op: req.Op, Ok: true}
	})

	var gotReq events.Event
	seq := uint64(0)
	exec := ChainExecutor(base, &eventMiddleware{
		emit: func(ctx context.Context, ev events.Event) {
			if ev.Type == "agent.op.request" {
				gotReq = ev
			}
		},
		seq:        &seq,
		metaKey:    opContextKey{},
		operations: newHostOperationRegistry(nil),
	})

	input, err := json.Marshal(map[string]string{
		"to":      "team@example.com",
		"subject": "Build completed",
		"body":    "done",
	})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}

	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:    types.HostOpEmail,
		Input: input,
	})
	if !resp.Ok {
		t.Fatalf("expected ok response, got %+v", resp)
	}
	if got := strings.TrimSpace(gotReq.Data["to"]); got != "team@example.com" {
		t.Fatalf("expected reqData.to to be set, got %q", got)
	}
	if got := strings.TrimSpace(gotReq.Data["subject"]); got != "Build completed" {
		t.Fatalf("expected reqData.subject to be set, got %q", got)
	}
	if _, ok := gotReq.Data["body"]; ok {
		t.Fatalf("expected reqData.body to be omitted")
	}
}

func TestEventMiddleware_FSReadMaxBytesFromOperation(t *testing.T) {
	base := types.HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		return types.HostOpResponse{Op: req.Op, Ok: true}
	})

	var gotReq events.Event
	seq := uint64(0)
	exec := ChainExecutor(base, &eventMiddleware{
		emit: func(ctx context.Context, ev events.Event) {
			if ev.Type == "agent.op.request" {
				gotReq = ev
			}
		},
		seq:        &seq,
		metaKey:    opContextKey{},
		operations: newHostOperationRegistry(nil),
	})

	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:        types.HostOpFSRead,
		Path:      "/workspace/file.txt",
		MaxBytes:  123,
		Checksums: []string{"sha256", "md5"},
	})
	if !resp.Ok {
		t.Fatalf("expected ok response, got %+v", resp)
	}
	if got := gotReq.Data["maxBytes"]; got != "123" {
		t.Fatalf("expected reqData.maxBytes=123, got %q", got)
	}
	if got := gotReq.StoreData["maxBytes"]; got != "123" {
		t.Fatalf("expected storeData.maxBytes=123, got %q", got)
	}
	if got := gotReq.Data["checksums"]; got != "md5,sha256" {
		t.Fatalf("expected reqData.checksums=md5,sha256, got %q", got)
	}
	if got := gotReq.StoreData["checksums"]; got != "md5,sha256" {
		t.Fatalf("expected storeData.checksums=md5,sha256, got %q", got)
	}

	resp = exec.Exec(context.Background(), types.HostOpRequest{
		Op:   types.HostOpFSRead,
		Path: "/workspace/file.txt",
	})
	if !resp.Ok {
		t.Fatalf("expected ok response, got %+v", resp)
	}
	if _, ok := gotReq.Data["maxBytes"]; ok {
		t.Fatalf("expected reqData.maxBytes to be omitted when zero")
	}
}

func TestEventMiddleware_HTTPFetchRequestEnrichment(t *testing.T) {
	base := types.HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		return types.HostOpResponse{Op: req.Op, Ok: true}
	})

	var gotReq events.Event
	seq := uint64(0)
	exec := ChainExecutor(base, &eventMiddleware{
		emit: func(ctx context.Context, ev events.Event) {
			if ev.Type == "agent.op.request" {
				gotReq = ev
			}
		},
		seq:        &seq,
		metaKey:    opContextKey{},
		operations: newHostOperationRegistry(nil),
	})

	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:     types.HostOpHTTPFetch,
		URL:    "https://example.com",
		Method: "post",
		Body:   strings.Repeat("x", maxHTTPBodyPreviewBytes+100),
	})
	if !resp.Ok {
		t.Fatalf("expected ok response, got %+v", resp)
	}
	if got := gotReq.Data["method"]; got != "POST" {
		t.Fatalf("expected method normalization to POST, got %q", got)
	}
	if got := gotReq.StoreData["method"]; got != "POST" {
		t.Fatalf("expected store method normalization to POST, got %q", got)
	}
	if got := gotReq.Data["bodyTruncated"]; got != "true" {
		t.Fatalf("expected bodyTruncated=true, got %q", got)
	}

	resp = exec.Exec(context.Background(), types.HostOpRequest{
		Op:   types.HostOpHTTPFetch,
		URL:  "https://example.com",
		Body: "Authorization: Bearer sk-SECRET",
	})
	if !resp.Ok {
		t.Fatalf("expected ok response, got %+v", resp)
	}
	if got := gotReq.Data["method"]; got != "GET" {
		t.Fatalf("expected empty method default GET, got %q", got)
	}
	if got := gotReq.Data["body"]; got != "<omitted>" {
		t.Fatalf("expected redacted request body, got %q", got)
	}
	if got := gotReq.StoreData["body"]; got != "<omitted>" {
		t.Fatalf("expected redacted stored request body, got %q", got)
	}
}

func TestEventMiddleware_HTTPFetchResponseEnrichment(t *testing.T) {
	base := types.HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		return types.HostOpResponse{
			Op:       req.Op,
			Ok:       true,
			Status:   201,
			FinalURL: "https://example.com/final",
			Body:     strings.Repeat("z", 1200),
		}
	})

	var gotResp events.Event
	seq := uint64(0)
	exec := ChainExecutor(base, &eventMiddleware{
		emit: func(ctx context.Context, ev events.Event) {
			if ev.Type == "agent.op.response" {
				gotResp = ev
			}
		},
		seq:        &seq,
		metaKey:    opContextKey{},
		operations: newHostOperationRegistry(nil),
	})

	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:  types.HostOpHTTPFetch,
		URL: "https://example.com",
	})
	if !resp.Ok {
		t.Fatalf("expected ok response, got %+v", resp)
	}
	if got := gotResp.Data["status"]; got != "201" {
		t.Fatalf("expected status=201, got %q", got)
	}
	if got := gotResp.Data["finalUrl"]; got != "https://example.com/final" {
		t.Fatalf("expected finalUrl to be set, got %q", got)
	}
	if got := gotResp.Data["bodyTruncated"]; got != "true" {
		t.Fatalf("expected bodyTruncated=true, got %q", got)
	}
	if got := gotResp.StoreData["status"]; got != "201" {
		t.Fatalf("expected store status=201, got %q", got)
	}
}

func TestDispatchMiddleware_UnregisteredOpFallsBackToBase(t *testing.T) {
	called := false
	base := types.HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		called = true
		return types.HostOpResponse{Op: req.Op, Ok: true}
	})

	exec := ChainExecutor(base, &dispatchMiddleware{
		operations: newHostOperationRegistry(nil),
	})

	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:   "custom_op",
		Path: "/workspace",
	})
	if !called {
		t.Fatalf("expected base executor to run for unregistered op")
	}
	if !resp.Ok {
		t.Fatalf("expected ok response, got %+v", resp)
	}
}

func TestEventMiddleware_AgentSpawnNoopIsReclassifiedAndEnriched(t *testing.T) {
	base := types.HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		return types.HostOpResponse{Op: req.Op, Ok: true, Text: "child done"}
	})

	var gotReq events.Event
	var gotResp events.Event
	seq := uint64(0)
	exec := ChainExecutor(base, &eventMiddleware{
		emit: func(ctx context.Context, ev events.Event) {
			if ev.Type == "agent.op.request" {
				gotReq = ev
			}
			if ev.Type == "agent.op.response" {
				gotResp = ev
			}
		},
		seq:        &seq,
		metaKey:    opContextKey{},
		operations: newHostOperationRegistry(nil),
	})

	input, err := json.Marshal(map[string]any{
		"goal":            "Compute 40+2",
		"model":           "gpt-5-mini",
		"maxTokens":       128,
		"backgroundCount": 2,
		"currentDepth":    0,
		"maxDepth":        3,
	})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}

	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:     types.HostOpNoop,
		Action: "agent_spawn",
		Input:  input,
		Text:   "child done",
	})
	if !resp.Ok {
		t.Fatalf("expected ok response, got %+v", resp)
	}
	if got := strings.TrimSpace(gotReq.Data["op"]); got != "agent_spawn" {
		t.Fatalf("expected request op to be reclassified as agent_spawn, got %q", got)
	}
	if got := strings.TrimSpace(gotReq.Data["goal"]); got != "Compute 40+2" {
		t.Fatalf("expected request goal to be set, got %q", got)
	}
	if got := strings.TrimSpace(gotResp.Data["op"]); got != "agent_spawn" {
		t.Fatalf("expected response op to be reclassified as agent_spawn, got %q", got)
	}
	if got := strings.TrimSpace(gotResp.Data["outputPreview"]); got != "child done" {
		t.Fatalf("expected response outputPreview to be set, got %q", got)
	}
}

func TestEventMiddleware_EmitsRequestTextForRepresentativeOps(t *testing.T) {
	base := types.HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		return types.HostOpResponse{Op: req.Op, Ok: true}
	})

	var gotReq events.Event
	seq := uint64(0)
	exec := ChainExecutor(base, &eventMiddleware{
		emit: func(ctx context.Context, ev events.Event) {
			if ev.Type == "agent.op.request" {
				gotReq = ev
			}
		},
		seq:        &seq,
		metaKey:    opContextKey{},
		operations: newHostOperationRegistry(nil),
	})

	emailInput, err := json.Marshal(map[string]string{
		"to":      "team@example.com",
		"subject": "Daily report",
		"body":    "done",
	})
	if err != nil {
		t.Fatalf("marshal email input: %v", err)
	}

	taskCreateInput, err := json.Marshal(map[string]any{
		"goal":       "Write integration tests",
		"taskId":     "task-1",
		"childRunId": "run-1",
	})
	if err != nil {
		t.Fatalf("marshal task_create input: %v", err)
	}

	tests := []struct {
		name string
		req  types.HostOpRequest
		want string
	}{
		{
			name: "fs_stat",
			req:  types.HostOpRequest{Op: types.HostOpFSStat, Path: "/workspace/a.txt"},
			want: "Stat /workspace/a.txt",
		},
		{
			name: "fs_patch",
			req:  types.HostOpRequest{Op: types.HostOpFSPatch, Path: "/workspace/a.txt", Text: "@@ -1 +1 @@\n-old\n+new\n", DryRun: true},
			want: "Patch /workspace/a.txt",
		},
		{
			name: "fs_batch_edit",
			req: types.HostOpRequest{
				Op:   types.HostOpFSBatchEdit,
				Path: "/knowledge",
				Glob: "**/*.md",
				BatchEditEdits: []types.BatchEdit{
					{Old: "[[old]]", New: "[[new]]", Occurrence: "all"},
				},
			},
			want: "Batch edit /knowledge (**/*.md)",
		},
		{
			name: "fs_txn",
			req: types.HostOpRequest{
				Op: types.HostOpFSTxn,
				TxnSteps: []types.FSTxnStep{
					{Op: types.HostOpFSWrite, Path: "/workspace/a.txt", Text: "hello"},
					{Op: types.HostOpFSAppend, Path: "/workspace/a.txt", Text: "\nworld"},
				},
			},
			want: "Txn 2 steps",
		},
		{
			name: "fs_archive_create",
			req: types.HostOpRequest{
				Op:          types.HostOpFSArchiveCreate,
				Path:        "/workspace/journals",
				Destination: "/workspace/journals.tar.gz",
				Format:      "tar.gz",
			},
			want: "Archive /workspace/journals -> /workspace/journals.tar.gz",
		},
		{
			name: "shell_exec",
			req:  types.HostOpRequest{Op: types.HostOpShellExec, Argv: []string{"rg", "-n", "todo"}},
			want: "rg -n todo",
		},
		{
			name: "http_fetch",
			req:  types.HostOpRequest{Op: types.HostOpHTTPFetch, URL: "https://example.com", Method: "GET"},
			want: "GET https://example.com",
		},
		{
			name: "code_exec",
			req:  types.HostOpRequest{Op: types.HostOpCodeExec, Language: "python", Code: "print('ok')"},
			want: "Run python code",
		},
		{
			name: "browser",
			req:  types.HostOpRequest{Op: types.HostOpBrowser, Input: json.RawMessage(`{"action":"navigate","url":"https://example.com"}`)},
			want: "browse.navigate https://example.com",
		},
		{
			name: "email",
			req:  types.HostOpRequest{Op: types.HostOpEmail, Input: emailInput},
			want: "Email team@example.com: Daily report",
		},
		{
			name: "task_create",
			req:  types.HostOpRequest{Op: types.HostOpToolResult, Tag: "task_create", Input: taskCreateInput},
			want: "Create task",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := exec.Exec(context.Background(), tc.req)
			if !resp.Ok {
				t.Fatalf("expected ok response, got %+v", resp)
			}
			if got := strings.TrimSpace(gotReq.Data["requestText"]); got != tc.want {
				t.Fatalf("requestText = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEventMiddleware_EmitsResponseTextForRepresentativeOps(t *testing.T) {
	base := types.HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		switch req.Op {
		case types.HostOpFSWrite:
			verified := true
			return types.HostOpResponse{Op: req.Op, Ok: true, WriteVerified: &verified, WriteChecksumAlgo: "sha256"}
		case types.HostOpFSStat:
			isDir := false
			sizeBytes := int64(12)
			return types.HostOpResponse{Op: req.Op, Ok: true, IsDir: &isDir, SizeBytes: &sizeBytes}
		case types.HostOpFSPatch:
			return types.HostOpResponse{
				Op:          req.Op,
				Ok:          true,
				PatchDryRun: true,
				PatchDiagnostics: &types.PatchDiagnostics{
					Mode:         "dry_run",
					HunksApplied: 1,
					HunksTotal:   2,
				},
			}
		case types.HostOpFSTxn:
			return types.HostOpResponse{
				Op: req.Op,
				Ok: true,
				TxnDiagnostics: &types.FSTxnDiagnostics{
					ApplyMode:    "apply",
					StepsApplied: 2,
					StepsTotal:   2,
				},
			}
		case types.HostOpFSBatchEdit:
			return types.HostOpResponse{
				Op:              req.Op,
				Ok:              true,
				MatchedFiles:    6,
				ModifiedFiles:   4,
				BatchEditDryRun: true,
			}
		case types.HostOpFSRead:
			return types.HostOpResponse{Op: req.Op, Ok: true, Truncated: true}
		case types.HostOpFSArchiveCreate:
			return types.HostOpResponse{
				Op:               req.Op,
				Ok:               true,
				FilesAdded:       3,
				CompressionRatio: 0.34,
			}
		case types.HostOpFSArchiveExtract:
			return types.HostOpResponse{
				Op:             req.Op,
				Ok:             true,
				FilesExtracted: 2,
				Skipped:        []string{"/workspace/out/existing.md"},
			}
		case types.HostOpFSArchiveList:
			return types.HostOpResponse{
				Op:             req.Op,
				Ok:             true,
				ArchiveEntries: []types.ArchiveEntry{{Name: "a.md"}, {Name: "b.md"}},
				Truncated:      true,
			}
		case types.HostOpShellExec:
			return types.HostOpResponse{Op: req.Op, Ok: true, ExitCode: 0}
		case types.HostOpHTTPFetch:
			return types.HostOpResponse{Op: req.Op, Ok: true, Status: 200}
		case types.HostOpCodeExec:
			return types.HostOpResponse{Op: req.Op, Ok: true, ExitCode: 0}
		case types.HostOpBrowser:
			return types.HostOpResponse{Op: "browser.navigate", Ok: true, Text: `{"title":"Example Domain"}`}
		case types.HostOpEmail:
			return types.HostOpResponse{Op: req.Op, Ok: true}
		default:
			return types.HostOpResponse{Op: req.Op, Ok: true}
		}
	})

	var gotResp events.Event
	seq := uint64(0)
	exec := ChainExecutor(base, &eventMiddleware{
		emit: func(ctx context.Context, ev events.Event) {
			if ev.Type == "agent.op.response" {
				gotResp = ev
			}
		},
		seq:        &seq,
		metaKey:    opContextKey{},
		operations: newHostOperationRegistry(nil),
	})

	tests := []struct {
		name string
		req  types.HostOpRequest
		want string
	}{
		{name: "fs_write", req: types.HostOpRequest{Op: types.HostOpFSWrite, Path: "/workspace/a.txt", Text: "hello"}, want: "✓ verified (sha256)"},
		{name: "fs_stat", req: types.HostOpRequest{Op: types.HostOpFSStat, Path: "/workspace/a.txt"}, want: "✓ file 12 bytes"},
		{name: "fs_patch", req: types.HostOpRequest{Op: types.HostOpFSPatch, Path: "/workspace/a.txt", Text: "@@ -1 +1 @@\n-old\n+new\n", DryRun: true}, want: "✓ dry-run 1/2 hunks"},
		{name: "fs_txn", req: types.HostOpRequest{Op: types.HostOpFSTxn, TxnSteps: []types.FSTxnStep{{Op: types.HostOpFSWrite, Path: "/workspace/a.txt", Text: "hello"}, {Op: types.HostOpFSAppend, Path: "/workspace/a.txt", Text: " world"}}}, want: "✓ txn applied 2/2 steps"},
		{name: "fs_batch_edit", req: types.HostOpRequest{Op: types.HostOpFSBatchEdit, Path: "/knowledge", Glob: "**/*.md", BatchEditEdits: []types.BatchEdit{{Old: "old", New: "new", Occurrence: "all"}}}, want: "✓ batch edit dry-run 6 matched, 4 modified"},
		{name: "fs_archive_create", req: types.HostOpRequest{Op: types.HostOpFSArchiveCreate, Path: "/workspace/journals", Destination: "/workspace/journals.tar.gz", Format: "tar.gz"}, want: "✓ archived 3 files (0.3400)"},
		{name: "fs_archive_extract", req: types.HostOpRequest{Op: types.HostOpFSArchiveExtract, Path: "/workspace/journals.tar.gz", Destination: "/workspace/out"}, want: "✓ extracted 2 files (1 skipped)"},
		{name: "fs_archive_list", req: types.HostOpRequest{Op: types.HostOpFSArchiveList, Path: "/workspace/journals.tar.gz"}, want: "✓ listed 2 entries (truncated)"},
		{name: "fs_read", req: types.HostOpRequest{Op: types.HostOpFSRead, Path: "/workspace/a.txt"}, want: "✓ truncated"},
		{name: "shell_exec", req: types.HostOpRequest{Op: types.HostOpShellExec, Argv: []string{"echo", "ok"}}, want: "✓ exit 0"},
		{name: "http_fetch", req: types.HostOpRequest{Op: types.HostOpHTTPFetch, URL: "https://example.com"}, want: "✓ 200"},
		{name: "code_exec", req: types.HostOpRequest{Op: types.HostOpCodeExec, Language: "python", Code: "print('ok')"}, want: "✓ ok"},
		{name: "browser.navigate", req: types.HostOpRequest{Op: types.HostOpBrowser, Input: json.RawMessage(`{"action":"navigate","url":"https://example.com"}`)}, want: `✓ navigated "Example Domain"`},
		{name: "email", req: types.HostOpRequest{Op: types.HostOpEmail, Input: json.RawMessage(`{"to":"team@example.com","subject":"Daily report","body":"done"}`)}, want: "✓ sent"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := exec.Exec(context.Background(), tc.req)
			if !resp.Ok {
				t.Fatalf("expected ok response, got %+v", resp)
			}
			if got := strings.TrimSpace(gotResp.Data["responseText"]); got != tc.want {
				t.Fatalf("responseText = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEventMiddleware_FSTxnRequestAndResponseEnrichment(t *testing.T) {
	base := types.HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		return types.HostOpResponse{
			Op: req.Op,
			Ok: false,
			TxnDiagnostics: &types.FSTxnDiagnostics{
				ApplyMode:         "apply",
				StepsTotal:        3,
				StepsApplied:      1,
				FailedStep:        2,
				RollbackPerformed: true,
				RollbackFailed:    true,
				RollbackErrors:    []string{"restore /workspace/a.txt: permission denied"},
			},
			TxnStepResults: []types.FSTxnStepResult{
				{Index: 1, Op: types.HostOpFSWrite, Path: "/workspace/a.txt", Ok: true},
				{Index: 2, Op: types.HostOpFSPatch, Path: "/workspace/a.txt", Ok: false, Error: "context mismatch"},
			},
		}
	})

	var gotReq events.Event
	var gotResp events.Event
	seq := uint64(0)
	exec := ChainExecutor(base, &eventMiddleware{
		emit: func(ctx context.Context, ev events.Event) {
			if ev.Type == "agent.op.request" {
				gotReq = ev
			}
			if ev.Type == "agent.op.response" {
				gotResp = ev
			}
		},
		seq:        &seq,
		metaKey:    opContextKey{},
		operations: newHostOperationRegistry(nil),
	})

	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op: types.HostOpFSTxn,
		TxnSteps: []types.FSTxnStep{
			{Op: types.HostOpFSWrite, Path: "/workspace/a.txt", Text: "hello"},
			{Op: types.HostOpFSPatch, Path: "/workspace/a.txt", Text: "@@ -1 +1 @@\n-hello\n+hi\n"},
			{Op: types.HostOpFSAppend, Path: "/workspace/a.txt", Text: "\nend"},
		},
		TxnOptions: &types.FSTxnOptions{Apply: true, RollbackOnError: true},
	})
	if resp.Ok {
		t.Fatalf("expected txn response to fail")
	}
	for key, want := range map[string]string{
		"steps":           "3",
		"dryRun":          "false",
		"apply":           "true",
		"rollbackOnError": "true",
	} {
		if gotReq.Data[key] != want {
			t.Fatalf("expected fs_txn request enrichment %s=%q, got %q (data=%v)", key, want, gotReq.Data[key], gotReq.Data)
		}
	}
	for key, want := range map[string]string{
		"txnMode":              "apply",
		"txnStepsTotal":        "3",
		"txnStepsApplied":      "1",
		"txnFailedStep":        "2",
		"txnRollbackPerformed": "true",
		"txnRollbackFailed":    "true",
		"txnStepResults":       "2",
	} {
		if gotResp.Data[key] != want {
			t.Fatalf("expected fs_txn response enrichment %s=%q, got %q (data=%v)", key, want, gotResp.Data[key], gotResp.Data)
		}
	}
	if gotResp.Data["txnRollbackErrors"] == "" || !strings.Contains(gotResp.Data["txnRollbackErrors"], "permission denied") {
		t.Fatalf("expected rollback errors in response data, got %v", gotResp.Data)
	}
}

func TestEventMiddleware_FSArchiveRequestAndResponseEnrichment(t *testing.T) {
	base := types.HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		return types.HostOpResponse{
			Op:               req.Op,
			Ok:               true,
			ArchiveFormat:    "zip",
			FilesAdded:       2,
			FilesExtracted:   0,
			TotalSizeBytes:   256,
			ArchiveSizeBytes: 128,
			CompressionRatio: 0.5,
			ArchiveEntries: []types.ArchiveEntry{
				{Name: "a.txt", SizeBytes: 12},
				{Name: "b.txt", SizeBytes: 20},
			},
			Skipped:   []string{"/workspace/out/a.txt"},
			Truncated: true,
		}
	})

	var gotReq events.Event
	var gotResp events.Event
	seq := uint64(0)
	exec := ChainExecutor(base, &eventMiddleware{
		emit: func(ctx context.Context, ev events.Event) {
			if ev.Type == "agent.op.request" {
				gotReq = ev
			}
			if ev.Type == "agent.op.response" {
				gotResp = ev
			}
		},
		seq:        &seq,
		metaKey:    opContextKey{},
		operations: newHostOperationRegistry(nil),
	})

	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:              types.HostOpFSArchiveCreate,
		Path:            "/workspace/src",
		Destination:     "/workspace/src.zip",
		Format:          "zip",
		Exclude:         []string{"*.tmp"},
		IncludeMetadata: true,
	})
	if !resp.Ok {
		t.Fatalf("expected ok response")
	}
	for key, want := range map[string]string{
		"destination":     "/workspace/src.zip",
		"format":          "zip",
		"exclude":         "*.tmp",
		"includeMetadata": "true",
	} {
		if gotReq.Data[key] != want {
			t.Fatalf("expected archive request enrichment %s=%q, got %q (data=%v)", key, want, gotReq.Data[key], gotReq.Data)
		}
	}
	for key, want := range map[string]string{
		"archiveFormat":    "zip",
		"filesAdded":       "2",
		"archiveEntries":   "2",
		"totalSizeBytes":   "256",
		"archiveSizeBytes": "128",
		"truncated":        "true",
	} {
		if gotResp.Data[key] != want {
			t.Fatalf("expected archive response enrichment %s=%q, got %q (data=%v)", key, want, gotResp.Data[key], gotResp.Data)
		}
	}
	if gotResp.Data["skipped"] == "" || !strings.Contains(gotResp.Data["skipped"], "/workspace/out/a.txt") {
		t.Fatalf("expected skipped list in response enrichment, got %v", gotResp.Data)
	}
}

func TestEventMiddleware_FSBatchEditRequestAndResponseEnrichment(t *testing.T) {
	base := types.HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		return types.HostOpResponse{
			Op:                         req.Op,
			Ok:                         true,
			MatchedFiles:               12,
			ModifiedFiles:              9,
			SkippedFiles:               3,
			BatchEditDryRun:            true,
			BatchEditRollbackPerformed: true,
			BatchEditDetails: []types.BatchEditResult{
				{Path: "/knowledge/a.md", Ok: true, Changed: true, EditsApplied: 2},
			},
			Truncated: true,
		}
	})

	var gotReq events.Event
	var gotResp events.Event
	seq := uint64(0)
	exec := ChainExecutor(base, &eventMiddleware{
		emit: func(ctx context.Context, ev events.Event) {
			if ev.Type == "agent.op.request" {
				gotReq = ev
			}
			if ev.Type == "agent.op.response" {
				gotResp = ev
			}
		},
		seq:        &seq,
		metaKey:    opContextKey{},
		operations: newHostOperationRegistry(nil),
	})

	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:   types.HostOpFSBatchEdit,
		Path: "/knowledge",
		Glob: "**/*.md",
		Exclude: []string{
			"archive/**",
		},
		BatchEditEdits: []types.BatchEdit{
			{Old: "[[old]]", New: "[[new]]", Occurrence: "all"},
		},
		BatchEditOptions: &types.BatchOptions{DryRun: true, MaxFiles: 50},
	})
	if !resp.Ok {
		t.Fatalf("expected ok response")
	}
	for key, want := range map[string]string{
		"glob":     "**/*.md",
		"exclude":  "archive/**",
		"edits":    "1",
		"dryRun":   "true",
		"apply":    "false",
		"maxFiles": "50",
	} {
		if gotReq.Data[key] != want {
			t.Fatalf("expected batch edit request enrichment %s=%q, got %q (data=%v)", key, want, gotReq.Data[key], gotReq.Data)
		}
	}
	for key, want := range map[string]string{
		"matchedFiles":               "12",
		"modifiedFiles":              "9",
		"skippedFiles":               "3",
		"batchEditDryRun":            "true",
		"batchEditRollbackPerformed": "true",
		"details":                    "1",
		"truncated":                  "true",
	} {
		if gotResp.Data[key] != want {
			t.Fatalf("expected batch edit response enrichment %s=%q, got %q (data=%v)", key, want, gotResp.Data[key], gotResp.Data)
		}
	}
	if gotResp.Data["batchEditDetails"] == "" || !strings.Contains(gotResp.Data["batchEditDetails"], "/knowledge/a.md") {
		t.Fatalf("expected details list in response enrichment, got %v", gotResp.Data)
	}
}

func TestEventMiddleware_PipeRequestAndResponseEnrichment(t *testing.T) {
	base := types.HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		return types.HostOpResponse{
			Op:               req.Op,
			Ok:               false,
			Error:            "selector missing",
			PipeFailedAtStep: 2,
			PipeDebug:        true,
			PipeValue:        json.RawMessage(`"HI"`),
			PipeStepResults: []types.PipeStepResult{
				{Index: 1, Type: "tool", Name: "fs_read", DurationMs: 2, OutputType: "string", OutputPreview: "hi"},
				{Index: 2, Type: "transform", Name: "uppercase", DurationMs: 1, Error: "selector missing"},
			},
		}
	})

	var gotReq events.Event
	var gotResp events.Event
	seq := uint64(0)
	exec := ChainExecutor(base, &eventMiddleware{
		emit: func(ctx context.Context, ev events.Event) {
			if ev.Type == "agent.op.request" {
				gotReq = ev
			}
			if ev.Type == "agent.op.response" {
				gotResp = ev
			}
		},
		seq:        &seq,
		metaKey:    opContextKey{},
		operations: newHostOperationRegistry(nil),
	})

	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op: types.HostOpPipe,
		PipeSteps: []types.PipeStep{
			{Type: "tool", Tool: types.HostOpFSRead, Args: map[string]any{"path": "/workspace/a.txt"}, Output: "text"},
			{Type: "transform", Transform: "uppercase"},
		},
		PipeOptions: &types.PipeOptions{Debug: true, MaxSteps: 4, MaxValueBytes: 2048},
	})
	if resp.Ok {
		t.Fatalf("expected pipe failure response")
	}
	for key, want := range map[string]string{
		"steps":         "2",
		"debug":         "true",
		"maxSteps":      "4",
		"maxValueBytes": "2048",
	} {
		if gotReq.Data[key] != want {
			t.Fatalf("expected pipe request enrichment %s=%q, got %q (data=%v)", key, want, gotReq.Data[key], gotReq.Data)
		}
	}
	for key, want := range map[string]string{
		"failedAtStep":   "2",
		"pipeDebug":      "true",
		"pipeSteps":      "2",
		"pipeValueType":  "string",
		"pipeValueBytes": "4",
	} {
		if gotResp.Data[key] != want {
			t.Fatalf("expected pipe response enrichment %s=%q, got %q (data=%v)", key, want, gotResp.Data[key], gotResp.Data)
		}
	}
	if gotResp.Data["responseText"] != "✗ pipe failed at step 2" {
		t.Fatalf("unexpected pipe response text: %q", gotResp.Data["responseText"])
	}
	if gotResp.Data["pipeStepResults"] == "" || !strings.Contains(gotResp.Data["pipeStepResults"], "uppercase") {
		t.Fatalf("expected pipe step results enrichment, got %v", gotResp.Data)
	}
}

func TestEventMiddleware_FSStatResponseEnrichment(t *testing.T) {
	base := types.HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		exists := true
		isDir := false
		sizeBytes := int64(99)
		mtime := int64(1700000000)
		return types.HostOpResponse{Op: req.Op, Ok: true, Exists: &exists, IsDir: &isDir, SizeBytes: &sizeBytes, Mtime: &mtime}
	})

	var gotResp events.Event
	seq := uint64(0)
	exec := ChainExecutor(base, &eventMiddleware{
		emit: func(ctx context.Context, ev events.Event) {
			if ev.Type == "agent.op.response" {
				gotResp = ev
			}
		},
		seq:        &seq,
		metaKey:    opContextKey{},
		operations: newHostOperationRegistry(nil),
	})

	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:   types.HostOpFSStat,
		Path: "/workspace/a.txt",
	})
	if !resp.Ok {
		t.Fatalf("expected ok response, got %+v", resp)
	}
	if gotResp.Data["exists"] != "true" || gotResp.Data["isDir"] != "false" || gotResp.Data["sizeBytes"] != "99" || gotResp.Data["mtime"] != "1700000000" {
		t.Fatalf("expected fs_stat response enrichment, got data=%v", gotResp.Data)
	}
	if gotResp.StoreData["exists"] != "true" || gotResp.StoreData["isDir"] != "false" || gotResp.StoreData["sizeBytes"] != "99" || gotResp.StoreData["mtime"] != "1700000000" {
		t.Fatalf("expected fs_stat store response enrichment, got storeData=%v", gotResp.StoreData)
	}
}

func TestEventMiddleware_FSReadResponseEnrichment(t *testing.T) {
	base := types.HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		return types.HostOpResponse{
			Op:            req.Op,
			Ok:            true,
			ReadChecksums: map[string]string{"md5": "abc", "sha256": "def"},
		}
	})

	var gotResp events.Event
	seq := uint64(0)
	exec := ChainExecutor(base, &eventMiddleware{
		emit: func(ctx context.Context, ev events.Event) {
			if ev.Type == "agent.op.response" {
				gotResp = ev
			}
		},
		seq:        &seq,
		metaKey:    opContextKey{},
		operations: newHostOperationRegistry(nil),
	})

	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:   types.HostOpFSRead,
		Path: "/workspace/a.txt",
	})
	if !resp.Ok {
		t.Fatalf("expected ok response, got %+v", resp)
	}
	if gotResp.Data["readChecksumAlgos"] != "md5,sha256" {
		t.Fatalf("expected fs_read checksum algos enrichment, got data=%v", gotResp.Data)
	}
	if gotResp.StoreData["readChecksumAlgos"] != "md5,sha256" {
		t.Fatalf("expected fs_read checksum algos store enrichment, got storeData=%v", gotResp.StoreData)
	}
	if !strings.Contains(gotResp.Data["readChecksums"], `"md5":"abc"`) {
		t.Fatalf("expected fs_read checksum map enrichment, got data=%v", gotResp.Data)
	}
}

func TestEventMiddleware_RequestEnrichmentMovedToOperations(t *testing.T) {
	base := types.HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		return types.HostOpResponse{Op: req.Op, Ok: true}
	})

	var gotReq events.Event
	seq := uint64(0)
	exec := ChainExecutor(base, &eventMiddleware{
		emit: func(ctx context.Context, ev events.Event) {
			if ev.Type == "agent.op.request" {
				gotReq = ev
			}
		},
		seq:        &seq,
		metaKey:    opContextKey{},
		operations: newHostOperationRegistry(nil),
	})

	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:              types.HostOpFSSearch,
		Path:            "/workspace",
		Query:           "needle",
		Pattern:         "n.*e",
		Glob:            "**/*.go",
		Exclude:         []string{"vendor/**", "**/*_test.go"},
		PreviewLines:    2,
		IncludeMetadata: true,
		MaxSizeBytes:    2048,
		Limit:           25,
	})
	if !resp.Ok {
		t.Fatalf("expected ok response, got %+v", resp)
	}
	for key, want := range map[string]string{
		"query":           "needle",
		"pattern":         "n.*e",
		"glob":            "**/*.go",
		"exclude":         "vendor/**,**/*_test.go",
		"previewLines":    "2",
		"includeMetadata": "true",
		"maxSizeBytes":    "2048",
		"maxResults":      "25",
	} {
		if gotReq.Data[key] != want {
			t.Fatalf("expected fs_search request enrichment %s=%q, got %q (data=%v)", key, want, gotReq.Data[key], gotReq.Data)
		}
	}

	resp = exec.Exec(context.Background(), types.HostOpRequest{
		Op:     types.HostOpTrace,
		Action: "events.latest",
		Input:  json.RawMessage(`{"limit":5}`),
	})
	if !resp.Ok {
		t.Fatalf("expected ok response, got %+v", resp)
	}
	if gotReq.Data["traceAction"] != "events.latest" || gotReq.Data["traceInput"] != `{"limit":5}` {
		t.Fatalf("expected trace request enrichment, got data=%v", gotReq.Data)
	}

	resp = exec.Exec(context.Background(), types.HostOpRequest{
		Op:               types.HostOpFSWrite,
		Path:             "/workspace/data.json",
		Text:             `{"x":1}`,
		Mode:             "a",
		Verify:           true,
		Checksum:         "sha256",
		ChecksumExpected: "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
		Atomic:           true,
		Sync:             true,
	})
	if !resp.Ok {
		t.Fatalf("expected ok response, got %+v", resp)
	}
	if gotReq.Data["textPreview"] == "" || gotReq.Data["textIsJSON"] != "true" {
		t.Fatalf("expected fs_write preview enrichment, got data=%v", gotReq.Data)
	}
	for k, want := range map[string]string{
		"verify":           "true",
		"checksum":         "sha256",
		"checksumExpected": "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
		"mode":             "a",
		"atomic":           "true",
		"sync":             "true",
	} {
		if gotReq.Data[k] != want {
			t.Fatalf("expected fs_write request enrichment %s=%q, got %q (data=%v)", k, want, gotReq.Data[k], gotReq.Data)
		}
		if gotReq.StoreData[k] != want {
			t.Fatalf("expected fs_write request store enrichment %s=%q, got %q (store=%v)", k, want, gotReq.StoreData[k], gotReq.StoreData)
		}
	}

	resp = exec.Exec(context.Background(), types.HostOpRequest{
		Op:      types.HostOpFSPatch,
		Path:    "/workspace/a.txt",
		Text:    "@@ -1 +1 @@\n-old\n+new\n",
		DryRun:  true,
		Verbose: true,
	})
	if !resp.Ok {
		t.Fatalf("expected ok response, got %+v", resp)
	}
	if gotReq.Data["patchPreview"] == "" {
		t.Fatalf("expected fs_patch preview enrichment, got data=%v", gotReq.Data)
	}
	if gotReq.Data["dryRun"] != "true" || gotReq.Data["verbose"] != "true" {
		t.Fatalf("expected fs_patch dryRun/verbose request enrichment, got data=%v", gotReq.Data)
	}
}

func TestEventMiddleware_FSSearchResponseEnrichment(t *testing.T) {
	base := types.HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		size := int64(42)
		mtime := int64(1700000000)
		return types.HostOpResponse{
			Op:               req.Op,
			Ok:               true,
			Results:          []types.SearchResult{{Title: "a.go", Path: "/workspace/a.go", PreviewMatch: "needle", SizeBytes: &size, Mtime: &mtime}},
			ResultsTotal:     12,
			ResultsReturned:  5,
			ResultsTruncated: true,
		}
	})

	var gotResp events.Event
	seq := uint64(0)
	exec := ChainExecutor(base, &eventMiddleware{
		emit: func(ctx context.Context, ev events.Event) {
			if ev.Type == "agent.op.response" {
				gotResp = ev
			}
		},
		seq:        &seq,
		metaKey:    opContextKey{},
		operations: newHostOperationRegistry(nil),
	})

	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:    types.HostOpFSSearch,
		Path:  "/workspace",
		Query: "needle",
	})
	if !resp.Ok {
		t.Fatalf("expected ok response, got %+v", resp)
	}
	for key, want := range map[string]string{
		"results":             "5",
		"resultsTotal":        "12",
		"resultsReturned":     "5",
		"resultsTruncated":    "true",
		"resultsHavePreview":  "true",
		"resultsHaveMetadata": "true",
	} {
		if gotResp.Data[key] != want {
			t.Fatalf("expected fs_search response enrichment %s=%q, got %q (data=%v)", key, want, gotResp.Data[key], gotResp.Data)
		}
	}
}

func TestEventMiddleware_FSWriteResponseEnrichment(t *testing.T) {
	base := types.HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		verified := true
		checksumMatch := true
		mismatchAt := int64(0)
		expectedBytes := int64(12)
		actualBytes := int64(12)
		writeBytes := int64(12)
		writeFinalSize := int64(12)
		return types.HostOpResponse{
			Op:                    req.Op,
			Ok:                    true,
			WriteVerified:         &verified,
			WriteChecksumMatch:    &checksumMatch,
			WriteChecksumAlgo:     "sha256",
			WriteChecksum:         "abc123",
			WriteChecksumExpected: "abc123",
			WriteRequestMode:      "a",
			WriteMode:             "created",
			WriteBytes:            &writeBytes,
			WriteFinalSize:        &writeFinalSize,
			WriteAtomicRequested:  true,
			WriteSyncRequested:    true,
			WriteMismatchAt:       &mismatchAt,
			WriteExpectedBytes:    &expectedBytes,
			WriteActualBytes:      &actualBytes,
		}
	})

	var gotResp events.Event
	seq := uint64(0)
	exec := ChainExecutor(base, &eventMiddleware{
		emit: func(ctx context.Context, ev events.Event) {
			if ev.Type == "agent.op.response" {
				gotResp = ev
			}
		},
		seq:        &seq,
		metaKey:    opContextKey{},
		operations: newHostOperationRegistry(nil),
	})

	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:   types.HostOpFSWrite,
		Path: "/workspace/a.txt",
		Text: "hello",
	})
	if !resp.Ok {
		t.Fatalf("expected success response")
	}
	for k, want := range map[string]string{
		"writeVerified":         "true",
		"writeChecksumMatch":    "true",
		"writeChecksumAlgo":     "sha256",
		"writeChecksum":         "abc123",
		"writeChecksumExpected": "abc123",
		"writeRequestMode":      "a",
		"writeMode":             "created",
		"writeBytes":            "12",
		"writeFinalSize":        "12",
		"writeAtomicRequested":  "true",
		"writeSyncRequested":    "true",
		"writeMismatchAt":       "0",
		"writeExpectedBytes":    "12",
		"writeActualBytes":      "12",
	} {
		if gotResp.Data[k] != want {
			t.Fatalf("expected response enrichment %s=%q, got %q (data=%v)", k, want, gotResp.Data[k], gotResp.Data)
		}
		if gotResp.StoreData[k] != want {
			t.Fatalf("expected store enrichment %s=%q, got %q (store=%v)", k, want, gotResp.StoreData[k], gotResp.StoreData)
		}
	}
}

func TestEventMiddleware_FSPatchResponseEnrichment(t *testing.T) {
	base := types.HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		return types.HostOpResponse{
			Op:          req.Op,
			Ok:          false,
			Error:       "patch did not apply cleanly (context mismatch)",
			PatchDryRun: true,
			PatchDiagnostics: &types.PatchDiagnostics{
				Mode:            "dry_run",
				HunksTotal:      3,
				HunksApplied:    1,
				FailedHunk:      2,
				HunkHeader:      "@@ -7,3 +7,3 @@",
				TargetLine:      9,
				FailureReason:   "context_mismatch",
				ExpectedContext: []string{"# Title"},
				ActualContext:   []string{"# Different Title"},
				Suggestion:      "Re-read and regenerate patch.",
			},
		}
	})

	var gotResp events.Event
	seq := uint64(0)
	exec := ChainExecutor(base, &eventMiddleware{
		emit: func(ctx context.Context, ev events.Event) {
			if ev.Type == "agent.op.response" {
				gotResp = ev
			}
		},
		seq:        &seq,
		metaKey:    opContextKey{},
		operations: newHostOperationRegistry(nil),
	})

	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:   types.HostOpFSPatch,
		Path: "/workspace/a.txt",
		Text: "@@ -1 +1 @@\n-old\n+new\n",
	})
	if resp.Ok {
		t.Fatalf("expected failure response")
	}
	for k, want := range map[string]string{
		"patchDryRun":        "true",
		"patchMode":          "dry_run",
		"patchHunksTotal":    "3",
		"patchHunksApplied":  "1",
		"patchFailedHunk":    "2",
		"patchTargetLine":    "9",
		"patchFailureReason": "context_mismatch",
		"patchHunkHeader":    "@@ -7,3 +7,3 @@",
		"patchSuggestion":    "Re-read and regenerate patch.",
	} {
		if gotResp.Data[k] != want {
			t.Fatalf("expected response enrichment %s=%q, got %q (data=%v)", k, want, gotResp.Data[k], gotResp.Data)
		}
		if gotResp.StoreData[k] != want {
			t.Fatalf("expected store enrichment %s=%q, got %q (store=%v)", k, want, gotResp.StoreData[k], gotResp.StoreData)
		}
	}
	if gotResp.Data["patchExpectedContext"] == "" || gotResp.Data["patchActualContext"] == "" {
		t.Fatalf("expected serialized context diagnostics, got data=%v", gotResp.Data)
	}
}

func TestEventMiddleware_BrowserRequestAndResponseEnrichment(t *testing.T) {
	base := types.HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		if req.Op == types.HostOpBrowser {
			return types.HostOpResponse{
				Op:   "browser.navigate",
				Ok:   true,
				Text: `{"sessionId":"sess-1","pageId":"p-1","title":"Example Domain","url":"https://example.com","count":2}`,
			}
		}
		return types.HostOpResponse{Op: req.Op, Ok: true}
	})

	var gotReq events.Event
	var gotResp events.Event
	seq := uint64(0)
	exec := ChainExecutor(base, &eventMiddleware{
		emit: func(ctx context.Context, ev events.Event) {
			if ev.Type == "agent.op.request" {
				gotReq = ev
			}
			if ev.Type == "agent.op.response" {
				gotResp = ev
			}
		},
		seq:        &seq,
		metaKey:    opContextKey{},
		operations: newHostOperationRegistry(nil),
	})

	reqInput := json.RawMessage(`{"action":"navigate","sessionId":"sess-1","url":"https://example.com","selector":"#main","values":["a","b"],"filename":"shot.png"}`)
	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:    types.HostOpBrowser,
		Input: reqInput,
	})
	if !resp.Ok {
		t.Fatalf("expected ok response, got %+v", resp)
	}
	if gotReq.Data["action"] != "navigate" || gotReq.Data["url"] != "https://example.com" || gotReq.Data["valuesCount"] != "2" {
		t.Fatalf("expected browser request enrichment, got data=%v", gotReq.Data)
	}
	if gotResp.Data["browserOp"] != "browser.navigate" || gotResp.Data["title"] == "" || gotResp.Data["count"] != "2" {
		t.Fatalf("expected browser response enrichment, got data=%v", gotResp.Data)
	}
}

func TestEventMiddleware_RequestActionPropagation(t *testing.T) {
	base := types.HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		return types.HostOpResponse{Op: req.Op, Ok: true}
	})

	var gotReq events.Event
	seq := uint64(0)
	exec := ChainExecutor(base, &eventMiddleware{
		emit: func(ctx context.Context, ev events.Event) {
			if ev.Type == "agent.op.request" {
				gotReq = ev
			}
		},
		seq:        &seq,
		metaKey:    opContextKey{},
		operations: newHostOperationRegistry(nil),
	})

	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:     types.HostOpFSRead,
		Path:   "/workspace/a.txt",
		Action: "code_exec_bridge",
	})
	if !resp.Ok {
		t.Fatalf("expected ok response, got %+v", resp)
	}
	if gotReq.Data["action"] != "code_exec_bridge" {
		t.Fatalf("expected action propagation, got data=%v", gotReq.Data)
	}
}

func TestResolveOperationForResponse_Aliases(t *testing.T) {
	reg := newHostOperationRegistry(nil)

	if op := resolveOperationForResponse(reg, types.HostOpRequest{Op: types.HostOpBrowser}, types.HostOpResponse{Op: "browser.navigate"}); op == nil || op.Op() != types.HostOpBrowser {
		t.Fatalf("expected browser alias to resolve to browser operation, got %+v", op)
	}

	if op := resolveOperationForResponse(reg, types.HostOpRequest{Op: types.HostOpToolResult, Tag: "task_create"}, types.HostOpResponse{Op: "task_create"}); op == nil || op.Op() != types.HostOpToolResult {
		t.Fatalf("expected task_create alias to resolve to tool_result operation, got %+v", op)
	}

	if op := resolveOperationForResponse(reg, types.HostOpRequest{Op: types.HostOpShellExec}, types.HostOpResponse{Op: "unknown.op"}); op == nil || op.Op() != types.HostOpShellExec {
		t.Fatalf("expected fallback to request op, got %+v", op)
	}
}

func TestEventMiddleware_CodeExecEnrichment(t *testing.T) {
	base := types.HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		if req.Op != types.HostOpCodeExec {
			return types.HostOpResponse{Op: req.Op, Ok: true}
		}
		return types.HostOpResponse{
			Op:       req.Op,
			Ok:       true,
			ExitCode: 0,
			Stdout:   "ran",
			Text:     `{"ok":true,"result":{"status":"ok"},"stdout":"ran","stderr":"","toolCallCount":2,"runtimeMs":17}`,
		}
	})

	var gotReq events.Event
	var gotResp events.Event
	seq := uint64(0)
	exec := ChainExecutor(base, &eventMiddleware{
		emit: func(ctx context.Context, ev events.Event) {
			if ev.Type == "agent.op.request" {
				gotReq = ev
			}
			if ev.Type == "agent.op.response" {
				gotResp = ev
			}
		},
		seq:        &seq,
		metaKey:    opContextKey{},
		operations: newHostOperationRegistry(nil),
	})

	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:       types.HostOpCodeExec,
		Language: "python",
		Cwd:      "/workspace",
		Code:     "result = {'status': 'ok'}",
	})
	if !resp.Ok {
		t.Fatalf("expected ok response, got %+v", resp)
	}
	if gotReq.Data["code"] != "result = {'status': 'ok'}" {
		t.Fatalf("expected full code in request event, got %q", gotReq.Data["code"])
	}
	if gotReq.Data["codeBytes"] == "" {
		t.Fatalf("expected codeBytes in request event")
	}
	if gotResp.Data["result"] == "" || gotResp.Data["outputPreview"] == "" {
		t.Fatalf("expected result/outputPreview in response event, got data=%v", gotResp.Data)
	}
	if gotResp.Data["toolCallCount"] != "2" || gotResp.Data["runtimeMs"] != "17" {
		t.Fatalf("expected toolCallCount/runtimeMs enrichment, got data=%v", gotResp.Data)
	}
}

func TestEventMiddleware_ToolResultObsidianEnrichment(t *testing.T) {
	base := types.HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		return types.HostOpResponse{Op: req.Op, Ok: true, Text: req.Text}
	})

	var gotReq events.Event
	var gotResp events.Event
	seq := uint64(0)
	exec := ChainExecutor(base, &eventMiddleware{
		emit: func(ctx context.Context, ev events.Event) {
			if ev.Type == "agent.op.request" {
				gotReq = ev
			}
			if ev.Type == "agent.op.response" {
				gotResp = ev
			}
		},
		seq:        &seq,
		metaKey:    opContextKey{},
		operations: newHostOperationRegistry(nil),
	})

	input, err := json.Marshal(map[string]any{
		"command": "graph",
		"data": map[string]any{
			"status": "ok",
		},
	})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}

	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:    types.HostOpToolResult,
		Tag:   "obsidian",
		Text:  string(input),
		Input: input,
	})
	if !resp.Ok {
		t.Fatalf("expected ok response, got %+v", resp)
	}
	if gotReq.Data["op"] != "obsidian" || gotReq.Data["command"] != "graph" {
		t.Fatalf("expected obsidian request enrichment, got data=%v", gotReq.Data)
	}
	if gotResp.Data["op"] != "obsidian" || gotResp.Data["command"] != "graph" {
		t.Fatalf("expected obsidian response enrichment, got data=%v", gotResp.Data)
	}
}

func TestEventMiddleware_ToolResultTaskCreateAssignedRoleEnrichment(t *testing.T) {
	base := types.HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		return types.HostOpResponse{Op: req.Op, Ok: true, Text: req.Text}
	})

	var gotReq events.Event
	seq := uint64(0)
	exec := ChainExecutor(base, &eventMiddleware{
		emit: func(ctx context.Context, ev events.Event) {
			if ev.Type == "agent.op.request" {
				gotReq = ev
			}
		},
		seq:        &seq,
		metaKey:    opContextKey{},
		operations: newHostOperationRegistry(nil),
	})

	input, err := json.Marshal(map[string]any{
		"goal":         "Add regression test",
		"taskId":       "task-42",
		"assignedRole": "reviewer",
	})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}

	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:    types.HostOpToolResult,
		Tag:   "task_create",
		Text:  "Task created",
		Input: input,
	})
	if !resp.Ok {
		t.Fatalf("expected ok response, got %+v", resp)
	}
	if gotReq.Data["op"] != "task_create" || gotReq.Data["assignedRole"] != "reviewer" {
		t.Fatalf("expected task_create assignedRole enrichment, got data=%v", gotReq.Data)
	}
}
