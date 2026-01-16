package events

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tinoosan/workbench-core/internal/store"
)

func TestHistorySink_AppendsJSONL(t *testing.T) {
	tmp := t.TempDir()
	hstore, err := store.NewDiskHistoryStoreFromPath(filepath.Join(tmp, "history.jsonl"))
	if err != nil {
		t.Fatalf("NewDiskHistoryStoreFromPath: %v", err)
	}
	sink := HistorySink{
		Store: hstore,
		Model: "openai/test-model",
		Now:   func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) },
	}

	if err := sink.Emit(context.Background(), "run-1", Event{
		Type:    "agent.final",
		Message: "done",
		Data:    map[string]string{"text": "ok"},
	}); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	b, err := os.ReadFile(filepath.Join(tmp, "history.jsonl"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	line := bytes.TrimSpace(b)

	var got struct {
		ID        string            `json:"id"`
		Timestamp string            `json:"ts"`
		RunID     string            `json:"runId"`
		Origin    string            `json:"origin"`
		Kind      string            `json:"kind"`
		Message   string            `json:"message"`
		Model     string            `json:"model"`
		Data      map[string]string `json:"data"`
	}
	if err := json.Unmarshal(line, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.RunID != "run-1" {
		t.Fatalf("runId: %q", got.RunID)
	}
	if got.Kind != "agent.final" {
		t.Fatalf("kind: %q", got.Kind)
	}
	if got.Origin != "agent" {
		t.Fatalf("origin: %q", got.Origin)
	}
	if got.Model != "openai/test-model" {
		t.Fatalf("model: %q", got.Model)
	}
	if got.Timestamp != "2026-01-01T00:00:00Z" {
		t.Fatalf("ts: %q", got.Timestamp)
	}
	if got.Data["text"] != "ok" {
		t.Fatalf("data.text: %q", got.Data["text"])
	}
	if got.ID == "" {
		t.Fatalf("id should not be empty")
	}
}
