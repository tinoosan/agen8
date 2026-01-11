package resources

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/fsutil"
	"github.com/tinoosan/workbench-core/internal/store"
)

func withTempDataDir(t *testing.T) func() {
	t.Helper()
	tmpDir := t.TempDir()
	old := config.DataDir
	config.DataDir = tmpDir
	return func() { config.DataDir = old }
}

func TestTraceResourceListRoot(t *testing.T) {
	defer withTempDataDir(t)()

	run, err := store.CreateRun("trace list", 100)
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	tr, err := NewTraceResource(run.RunId)
	if err != nil {
		t.Fatalf("NewTraceResource: %v", err)
	}

	entries, err := tr.List("")
	if err != nil {
		t.Fatalf("List root: %v", err)
	}

	want := map[string]bool{
		"events":        true,
		"events.since":  true,
		"events.latest": true,
	}
	if len(entries) != len(want) {
		t.Fatalf("expected %d entries, got %d", len(want), len(entries))
	}
	for _, e := range entries {
		if !want[e.Path] {
			t.Fatalf("unexpected entry path %q", e.Path)
		}
	}
}

func TestTraceResourceReadEventsSince(t *testing.T) {
	defer withTempDataDir(t)()

	run, err := store.CreateRun("trace since", 100)
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	tr, err := NewTraceResource(run.RunId)
	if err != nil {
		t.Fatalf("NewTraceResource: %v", err)
	}

	if err := store.AppendEvent(run.RunId, "a", "first", nil); err != nil {
		t.Fatalf("AppendEvent first: %v", err)
	}

	eventPath := fsutil.GetEventFilePath(config.DataDir, run.RunId)
	info, err := os.Stat(eventPath)
	if err != nil {
		t.Fatalf("stat events.jsonl: %v", err)
	}
	offset := info.Size()

	if err := store.AppendEvent(run.RunId, "b", "second", nil); err != nil {
		t.Fatalf("AppendEvent second: %v", err)
	}
	if err := store.AppendEvent(run.RunId, "c", "third", nil); err != nil {
		t.Fatalf("AppendEvent third: %v", err)
	}

	got, err := tr.Read("events.since/" + strconv.FormatInt(offset, 10))
	if err != nil {
		t.Fatalf("Read events.since: %v", err)
	}

	all, err := os.ReadFile(eventPath)
	if err != nil {
		t.Fatalf("ReadFile events.jsonl: %v", err)
	}
	if offset > int64(len(all)) {
		t.Fatalf("offset %d exceeds file length %d", offset, len(all))
	}
	want := all[offset:]
	if string(got) != string(want) {
		t.Fatalf("events.since mismatch:\nwant=%q\ngot=%q", string(want), string(got))
	}
}

func TestTraceResourceReadEventsLatest(t *testing.T) {
	defer withTempDataDir(t)()

	run, err := store.CreateRun("trace latest", 100)
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	tr, err := NewTraceResource(run.RunId)
	if err != nil {
		t.Fatalf("NewTraceResource: %v", err)
	}

	if err := store.AppendEvent(run.RunId, "a", "first", nil); err != nil {
		t.Fatalf("AppendEvent first: %v", err)
	}
	if err := store.AppendEvent(run.RunId, "b", "second", nil); err != nil {
		t.Fatalf("AppendEvent second: %v", err)
	}
	if err := store.AppendEvent(run.RunId, "c", "third", nil); err != nil {
		t.Fatalf("AppendEvent third: %v", err)
	}

	got, err := tr.Read("events.latest/2")
	if err != nil {
		t.Fatalf("Read events.latest: %v", err)
	}

	eventPath := fsutil.GetEventFilePath(config.DataDir, run.RunId)
	all, err := os.ReadFile(eventPath)
	if err != nil {
		t.Fatalf("ReadFile events.jsonl: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(all), "\n"), "\n")
	want := strings.Join(lines[len(lines)-2:], "\n") + "\n"
	if string(got) != want {
		t.Fatalf("events.latest mismatch:\nwant=%q\ngot=%q", want, string(got))
	}
}

func TestTraceResourceReadEvents(t *testing.T) {
	defer withTempDataDir(t)()

	run, err := store.CreateRun("trace events", 100)
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	tr, err := NewTraceResource(run.RunId)
	if err != nil {
		t.Fatalf("NewTraceResource: %v", err)
	}

	content := []byte("{\"eventId\":\"e1\"}\n")
	traceEvents := filepath.Join(tr.BaseDir, "events.jsonl")
	if err := os.WriteFile(traceEvents, content, 0644); err != nil {
		t.Fatalf("WriteFile trace events: %v", err)
	}

	got, err := tr.Read("events")
	if err != nil {
		t.Fatalf("Read events: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("events content mismatch: want %q, got %q", string(content), string(got))
	}
}
