package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/tools"
	"github.com/tinoosan/workbench-core/internal/types"
)

func TestBuiltinTrace_EventsSince_CursorAdvances(t *testing.T) {
	dir := t.TempDir()
	// Seed a minimal trace file with two JSONL lines matching the on-disk trace format.
	if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte(
		`{"eventId":"event-1","runId":"run-1","timestamp":"2026-01-01T00:00:00Z","type":"a","message":"m1"}`+"\n"+
			`{"eventId":"event-2","runId":"run-1","timestamp":"2026-01-01T00:00:01Z","type":"b","message":"m2"}`+"\n",
	), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	inv := tools.BuiltinTraceInvoker{Store: store.DiskTraceStore{Dir: dir}}
	req := types.ToolRequest{
		Version:  "v1",
		CallID:   "c1",
		ToolID:   "builtin.trace",
		ActionID: "events.since",
		Input:    json.RawMessage(`{"cursor":0,"maxBytes":4096,"limit":10}`),
	}
	res, err := inv.Invoke(context.Background(), req)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	var out struct {
		CursorBefore string           `json:"cursorBefore"`
		CursorAfter  string           `json:"cursorAfter"`
		Events       []map[string]any `json:"events"`
	}
	if err := json.Unmarshal(res.Output, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(out.Events) != 2 {
		t.Fatalf("events=%d", len(out.Events))
	}
	if out.CursorBefore == "" || out.CursorAfter == "" {
		t.Fatalf("expected cursors, got before=%q after=%q", out.CursorBefore, out.CursorAfter)
	}
}
