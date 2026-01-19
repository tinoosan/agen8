package tui

import (
	"context"
	"io/fs"
	"strings"
	"testing"

	"github.com/tinoosan/workbench-core/internal/events"
)

type stubRunnerWithReadWrite struct {
	stubRunnerWithRead
}

func (s stubRunnerWithReadWrite) ReadVFS(ctx context.Context, path string, maxBytes int) (text string, bytesLen int, truncated bool, err error) {
	_ = ctx
	if s.files == nil {
		return "", 0, false, fs.ErrNotExist
	}
	txt, ok := s.files[path]
	if !ok {
		return "", 0, false, fs.ErrNotExist
	}
	b := []byte(txt)
	bytesLen = len(b)
	if maxBytes > 0 && len(b) > maxBytes {
		b = b[:maxBytes]
		truncated = true
	}
	return string(b), bytesLen, truncated, nil
}

func (s stubRunnerWithReadWrite) WriteVFS(ctx context.Context, path string, data []byte) error {
	_ = ctx
	_ = path
	_ = data
	return nil
}

func TestTranscript_FileWrite_EmitsDiffBlock(t *testing.T) {
	r := stubRunnerWithReadWrite{stubRunnerWithRead{
		stubRunner: stubRunner{final: "ok"},
		files: map[string]string{
			"/workspace/a.txt": "hello\nworld\n",
		},
	}}
	m := New(context.Background(), r, make(chan events.Event))
	m.width = 120
	m.height = 40
	m.layout()

	// Pretend we had a previous snapshot (so we render Updated).
	m.fileSnapCache = map[string]string{"/workspace/a.txt": "hello\nthere\n"}

	req := events.Event{
		Type: "agent.op.request",
		Data: map[string]string{
			"op":   "fs.write",
			"path": "/workspace/a.txt",
		},
	}
	_ = m.onEvent(req)

	resp := events.Event{
		Type: "agent.op.response",
		Data: map[string]string{
			"op":   "fs.write",
			"path": "/workspace/a.txt",
			"ok":   "true",
		},
	}
	cmd := m.onEvent(resp)
	if cmd == nil {
		t.Fatalf("expected cmd to read file after write")
	}

	msg := cmd()
	m2, _ := m.Update(msg)
	updated3 := m2.(Model)

	view := updated3.transcript.View()
	if !strings.Contains(view, "Updated") {
		t.Fatalf("expected transcript to contain Updated header, got:\n%s", view)
	}
	// Glamour renders code blocks without literal ``` fences; assert diff markers.
	if !strings.Contains(view, "--- a/workspace/a.txt") || !strings.Contains(view, "+++ b/workspace/a.txt") {
		t.Fatalf("expected transcript to contain unified diff headers, got:\n%s", view)
	}
	if !strings.Contains(view, "-there") && !strings.Contains(view, "-there\n") {
		t.Fatalf("expected diff to include removed line, got:\n%s", view)
	}
	if !strings.Contains(view, "+world") {
		t.Fatalf("expected diff to include added line, got:\n%s", view)
	}
}

func TestTranscript_FileWrite_Create_ShowsDevNullDiff(t *testing.T) {
	r := stubRunnerWithReadWrite{stubRunnerWithRead{
		stubRunner: stubRunner{final: "ok"},
		files: map[string]string{
			"/workspace/new.txt": "hello\n",
		},
	}}
	m := New(context.Background(), r, make(chan events.Event))
	m.width = 120
	m.height = 40
	m.layout()

	// No cache; simulate create by ensuring the pre-read returns not-exist.
	delete(r.files, "/workspace/new.txt")

	cmd := m.onEvent(events.Event{
		Type: "agent.op.request",
		Data: map[string]string{"op": "fs.write", "path": "/workspace/new.txt"},
	})
	if cmd != nil {
		// Run the pre-read (expected to fail not-exist and be ignored).
		m2, _ := m.Update(cmd())
		m = m2.(Model)
	}

	// Now simulate the post-write read returning the created content.
	r.files["/workspace/new.txt"] = "hello\n"

	cmd2 := m.onEvent(events.Event{
		Type: "agent.op.response",
		Data: map[string]string{"op": "fs.write", "path": "/workspace/new.txt", "ok": "true"},
	})
	if cmd2 == nil {
		t.Fatalf("expected after-read cmd")
	}
	m3, _ := m.Update(cmd2())
	updated3 := m3.(Model)

	view := updated3.transcript.View()
	if !strings.Contains(view, "Created") {
		t.Fatalf("expected Created header, got:\n%s", view)
	}
	if !strings.Contains(view, "--- /dev/null") || !strings.Contains(view, "+++ b/workspace/new.txt") {
		t.Fatalf("expected /dev/null create diff headers, got:\n%s", view)
	}
}

func TestTranscript_FileWrite_LabelsUpdated_WhenFileExistsButNotCached(t *testing.T) {
	r := stubRunnerWithReadWrite{stubRunnerWithRead{
		stubRunner: stubRunner{final: "ok"},
		files: map[string]string{
			"/workspace/existing.txt": "old\n",
		},
	}}
	m := New(context.Background(), r, make(chan events.Event))
	m.width = 120
	m.height = 40
	m.layout()

	// No cache set (unknown).

	// Request should trigger a pre-read cmd to detect existence.
	cmd := m.onEvent(events.Event{
		Type: "agent.op.request",
		Data: map[string]string{
			"op":   "fs.write",
			"path": "/workspace/existing.txt",
		},
	})
	if cmd == nil {
		t.Fatalf("expected pre-read cmd for before content")
	}
	m2, _ := m.Update(cmd())
	updated2 := m2.(Model)

	// Response triggers after read and then file-change block.
	cmd2 := updated2.onEvent(events.Event{
		Type: "agent.op.response",
		Data: map[string]string{
			"op":   "fs.write",
			"path": "/workspace/existing.txt",
			"ok":   "true",
		},
	})
	updated3 := updated2
	if cmd2 == nil {
		t.Fatalf("expected after-read cmd")
	}
	m3, _ := updated3.Update(cmd2())
	updated4 := m3.(Model)

	view := updated4.transcript.View()
	if !strings.Contains(view, "Updated") {
		t.Fatalf("expected Updated (not Created) when file existed but wasn't cached, got:\n%s", view)
	}
}

func TestTranscript_FilePatch_EmitsPatchBlock(t *testing.T) {
	r := stubRunnerWithReadWrite{stubRunnerWithRead{
		stubRunner: stubRunner{final: "ok"},
		files: map[string]string{
			"/workspace/b.txt": "after\n",
		},
	}}
	m := New(context.Background(), r, make(chan events.Event))
	m.width = 120
	m.height = 40
	m.layout()

	req := events.Event{
		Type: "agent.op.request",
		Data: map[string]string{
			"op":            "fs.patch",
			"path":          "/workspace/b.txt",
			"patchPreview":  "--- a/b.txt\n+++ b/b.txt\n@@ -1 +1 @@\n-before\n+after\n",
			"patchTruncated": "false",
		},
	}
	_ = m.onEvent(req)

	// Ensure the request action line is clean.
	if got := m.transcript.View(); !strings.Contains(got, "Patch /workspace/b.txt") {
		t.Fatalf("expected action line %q, got:\n%s", "Patch /workspace/b.txt", got)
	}

	resp := events.Event{
		Type: "agent.op.response",
		Data: map[string]string{
			"op":   "fs.patch",
			"path": "/workspace/b.txt",
			"ok":   "true",
		},
	}
	cmd := m.onEvent(resp)
	if cmd == nil {
		t.Fatalf("expected cmd to read file after patch")
	}
	msg := cmd()
	m2, _ := m.Update(msg)
	updated3 := m2.(Model)

	view := updated3.transcript.View()
	// Glamour renders code blocks without literal ``` fences; assert patch markers.
	if !strings.Contains(view, "--- a/b.txt") || !strings.Contains(view, "+++ b/b.txt") {
		t.Fatalf("expected transcript to contain patch headers, got:\n%s", view)
	}
	if !strings.Contains(view, "@@") {
		t.Fatalf("expected patch to be rendered, got:\n%s", view)
	}
}

