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
		case types.HostOpFSRead:
			return types.HostOpResponse{Op: req.Op, Ok: true, Truncated: true}
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
		Op:    types.HostOpFSSearch,
		Path:  "/workspace",
		Query: "needle",
		Limit: 25,
	})
	if !resp.Ok {
		t.Fatalf("expected ok response, got %+v", resp)
	}
	if gotReq.Data["query"] != "needle" || gotReq.Data["limit"] != "25" {
		t.Fatalf("expected fs_search query/limit enrichment, got data=%v", gotReq.Data)
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
