package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tinoosan/agen8/pkg/config"
	pkgstore "github.com/tinoosan/agen8/pkg/store"
)

func TestHistoryStoreParity_BoundedReads(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{DataDir: t.TempDir()}
	sessionID := "sess-parity"

	disk, err := NewDiskHistoryStore(cfg, sessionID)
	if err != nil {
		t.Fatalf("NewDiskHistoryStore: %v", err)
	}
	sqlite, err := NewSQLiteHistoryStore(cfg, sessionID)
	if err != nil {
		t.Fatalf("NewSQLiteHistoryStore: %v", err)
	}

	lines := [][]byte{
		[]byte(`{"id":"1","ts":"2026-01-01T00:00:00Z","runId":"r1","origin":"user","kind":"message","message":"first"}`),
		[]byte(`{"id":"2","ts":"2026-01-01T00:00:01Z","runId":"r1","origin":"assistant","kind":"message","message":"second-longer"}`),
		[]byte(`{"id":"3","ts":"2026-01-01T00:00:02Z","runId":"r1","origin":"assistant","kind":"message","message":"third"}`),
	}
	for _, line := range lines {
		if err := disk.AppendLine(ctx, line); err != nil {
			t.Fatalf("disk AppendLine: %v", err)
		}
		if err := sqlite.AppendLine(ctx, line); err != nil {
			t.Fatalf("sqlite AppendLine: %v", err)
		}
	}

	sinceOpts := pkgstore.HistorySinceOptions{MaxBytes: len(lines[0]) + len(lines[1]) + 2, Limit: 2}
	diskSince, err := disk.LinesSince(ctx, pkgstore.HistoryCursorFromInt64(0), sinceOpts)
	if err != nil {
		t.Fatalf("disk LinesSince: %v", err)
	}
	sqliteSince, err := sqlite.LinesSince(ctx, pkgstore.HistoryCursorFromInt64(0), sinceOpts)
	if err != nil {
		t.Fatalf("sqlite LinesSince: %v", err)
	}
	assertHistoryParity(t, diskSince, sqliteSince)

	latestOpts := pkgstore.HistoryLatestOptions{MaxBytes: len(lines[0]) + len(lines[1]) + len(lines[2]) + 3, Limit: 2}
	diskLatest, err := disk.LinesLatest(ctx, latestOpts)
	if err != nil {
		t.Fatalf("disk LinesLatest: %v", err)
	}
	sqliteLatest, err := sqlite.LinesLatest(ctx, latestOpts)
	if err != nil {
		t.Fatalf("sqlite LinesLatest: %v", err)
	}
	assertHistoryParity(t, diskLatest, sqliteLatest)
}

func TestTraceStoreParity_BoundedReads(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{DataDir: t.TempDir()}
	runID := "run-parity"
	traceDir := filepath.Join(cfg.DataDir, "trace")

	disk := DiskTraceStore{DiskStore: DiskStore{Dir: traceDir}}
	if err := disk.ensure(); err != nil {
		t.Fatalf("disk ensure: %v", err)
	}
	sqlite := SQLiteTraceStore{Cfg: cfg, RunID: runID}

	db, err := getSQLiteDB(cfg)
	if err != nil {
		t.Fatalf("getSQLiteDB: %v", err)
	}

	rawEvents := []string{
		`{"timestamp":"2026-01-01T00:00:00Z","type":"step","message":"first"}`,
		`{"timestamp":"2026-01-01T00:00:01Z","type":"step","message":"second event"}`,
		`{"timestamp":`,
		`{"timestamp":"2026-01-01T00:00:02Z","type":"step","message":"third"}`,
		`{"timestamp":"2026-01-01T00:00:03Z","type":"step","message":"tail"}`,
	}
	for i, raw := range rawEvents {
		if _, err := db.Exec(
			`INSERT INTO events (event_id, run_id, ts, type, message, data_json, event_json) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			fmt.Sprintf("%s-%d", runID, i),
			runID,
			fmt.Sprintf("2026-01-01T00:00:0%dZ", i),
			"step",
			"msg",
			nil,
			raw,
		); err != nil {
			t.Fatalf("insert event %d: %v", i, err)
		}
	}
	if err := os.WriteFile(filepath.Join(traceDir, "events.jsonl"), []byte(strings.Join(rawEvents, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	sinceOpts := pkgstore.TraceSinceOptions{MaxBytes: len(rawEvents[0]) + len(rawEvents[1]) + len(rawEvents[2]) + len(rawEvents[3]) + 4, Limit: 3}
	diskSince, err := disk.EventsSince(ctx, pkgstore.TraceCursorFromInt64(0), sinceOpts)
	if err != nil {
		t.Fatalf("disk EventsSince: %v", err)
	}
	sqliteSince, err := sqlite.EventsSince(ctx, pkgstore.TraceCursorFromInt64(0), sinceOpts)
	if err != nil {
		t.Fatalf("sqlite EventsSince: %v", err)
	}
	assertTraceParity(t, diskSince, sqliteSince)

	latestOpts := pkgstore.TraceLatestOptions{MaxBytes: len(rawEvents[3]) + len(rawEvents[4]) + 2, Limit: 2}
	diskLatest, err := disk.EventsLatest(ctx, latestOpts)
	if err != nil {
		t.Fatalf("disk EventsLatest: %v", err)
	}
	sqliteLatest, err := sqlite.EventsLatest(ctx, latestOpts)
	if err != nil {
		t.Fatalf("sqlite EventsLatest: %v", err)
	}
	assertTraceParity(t, diskLatest, sqliteLatest)
}

func assertHistoryParity(t *testing.T, got, want pkgstore.HistoryBatch) {
	t.Helper()
	if !reflect.DeepEqual(got.Lines, want.Lines) {
		t.Fatalf("lines mismatch:\ndisk=%q\nsqlite=%q", got.Lines, want.Lines)
	}
	if got.Returned != want.Returned || got.LinesTotal != want.LinesTotal || got.ReturnedCapped != want.ReturnedCapped || got.Truncated != want.Truncated {
		t.Fatalf("history parity mismatch: disk=%+v sqlite=%+v", got, want)
	}
}

func assertTraceParity(t *testing.T, got, want pkgstore.TraceBatch) {
	t.Helper()
	if !reflect.DeepEqual(got.Events, want.Events) {
		t.Fatalf("events mismatch:\ndisk=%+v\nsqlite=%+v", got.Events, want.Events)
	}
	if got.Returned != want.Returned || got.LinesTotal != want.LinesTotal || got.ParseErrors != want.ParseErrors || got.ReturnedCapped != want.ReturnedCapped || got.Truncated != want.Truncated {
		t.Fatalf("trace parity mismatch: disk=%+v sqlite=%+v", got, want)
	}
}
