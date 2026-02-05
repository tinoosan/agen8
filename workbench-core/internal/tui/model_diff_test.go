package tui

import (
	"context"
	"io/fs"
	"strings"
	"testing"

	"github.com/tinoosan/workbench-core/pkg/events"
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
			"op":   "fs_write",
			"path": "/workspace/a.txt",
		},
	}
	_ = m.onEvent(req)

	resp := events.Event{
		Type: "agent.op.response",
		Data: map[string]string{
			"op":   "fs_write",
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

	// Assert against the raw transcript item text (stable; avoids ANSI styling concerns).
	raw := ""
	for i := len(updated3.transcriptItems) - 1; i >= 0; i-- {
		if updated3.transcriptItems[i].kind == transcriptFileChange {
			raw = strings.TrimSpace(updated3.transcriptItems[i].text)
			break
		}
	}
	if raw == "" {
		t.Fatalf("expected transcript to include a file-change item")
	}
	if !strings.HasPrefix(raw, "workspace/a.txt  +1 -1") {
		t.Fatalf("expected file-change header %q, got:\n%s", "workspace/a.txt  +1 -1", raw)
	}
	// Glamour renders code blocks without literal ``` fences; assert numbered diff markers.
	view := updated3.transcript.View()
	if !strings.Contains(view, "-2 | there") {
		t.Fatalf("expected diff to include removed line with number, got:\n%s", view)
	}
	if !strings.Contains(view, "+2 | world") {
		t.Fatalf("expected diff to include added line with number, got:\n%s", view)
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
		Data: map[string]string{"op": "fs_write", "path": "/workspace/new.txt"},
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
		Data: map[string]string{"op": "fs_write", "path": "/workspace/new.txt", "ok": "true"},
	})
	if cmd2 == nil {
		t.Fatalf("expected after-read cmd")
	}
	m3, _ := m.Update(cmd2())
	updated3 := m3.(Model)

	raw := ""
	for i := len(updated3.transcriptItems) - 1; i >= 0; i-- {
		if updated3.transcriptItems[i].kind == transcriptFileChange {
			raw = strings.TrimSpace(updated3.transcriptItems[i].text)
			break
		}
	}
	if raw == "" {
		t.Fatalf("expected transcript to include a file-change item")
	}
	if !strings.HasPrefix(raw, "workspace/new.txt  +1 -0") {
		t.Fatalf("expected file-change header %q, got:\n%s", "workspace/new.txt  +1 -0", raw)
	}
	view := updated3.transcript.View()
	if !strings.Contains(view, "+1 | hello") {
		t.Fatalf("expected create diff to include numbered added line, got:\n%s", view)
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
			"op":   "fs_write",
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
			"op":   "fs_write",
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

	raw := ""
	for i := len(updated4.transcriptItems) - 1; i >= 0; i-- {
		if updated4.transcriptItems[i].kind == transcriptFileChange {
			raw = strings.TrimSpace(updated4.transcriptItems[i].text)
			break
		}
	}
	if raw == "" {
		t.Fatalf("expected transcript to include a file-change item")
	}
	// For no-op writes, we omit the +0/-0 counts (user preference).
	if !strings.HasPrefix(raw, "workspace/existing.txt") {
		t.Fatalf("expected file-change header to start with %q, got:\n%s", "workspace/existing.txt", raw)
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
			"op":             "fs_patch",
			"path":           "/workspace/b.txt",
			"patchPreview":   "--- a/b.txt\n+++ b/b.txt\n@@ -1 +1 @@\n-before\n+after\n",
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
			"op":   "fs_patch",
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

	raw := ""
	for i := len(updated3.transcriptItems) - 1; i >= 0; i-- {
		if updated3.transcriptItems[i].kind == transcriptFileChange {
			raw = strings.TrimSpace(updated3.transcriptItems[i].text)
			break
		}
	}
	if raw == "" {
		t.Fatalf("expected transcript to include a file-change item")
	}
	if !strings.HasPrefix(raw, "workspace/b.txt  +1 -1") {
		t.Fatalf("expected file-change header %q, got:\n%s", "workspace/b.txt  +1 -1", raw)
	}
	view := updated3.transcript.View()
	// Glamour renders code blocks without literal ``` fences; assert numbered patch markers.
	if !strings.Contains(view, "-1 | before") {
		t.Fatalf("expected patch to include numbered removed line, got:\n%s", view)
	}
	if !strings.Contains(view, "+1 | after") {
		t.Fatalf("expected patch to include numbered added line, got:\n%s", view)
	}
}
