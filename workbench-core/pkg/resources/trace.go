package resources

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/validate"
	"github.com/tinoosan/workbench-core/pkg/vfs"
	"github.com/tinoosan/workbench-core/pkg/vfsutil"
	_ "modernc.org/sqlite"
)

type TraceResource struct {
	vfs.ReadOnlyResource
	Cfg config.Config
	// BaseDir is the OS directory backing this resource (the sandbox root).
	BaseDir string
	// Mount is the virtual mount name used by the VFS.
	Mount string
	// RunID is the run this trace directory belongs to.
	RunID string
}

// TraceResource exposes a read-only event feed under the VFS mount "/log".
const (
	maxLatestCount = 200
	maxSinceBytes  = 64 * 1024
)

func NewTraceResource(cfg config.Config, runID string) (*TraceResource, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if err := validate.NonEmpty("runID", runID); err != nil {
		return nil, err
	}
	baseDir := fsutil.GetLogDir(cfg.DataDir, runID)
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("error creating trace directory %s: %w", baseDir, err)
	}
	return &TraceResource{
		ReadOnlyResource: vfs.ReadOnlyResource{Name: "trace"},
		Cfg:              cfg,
		BaseDir:          baseDir,
		Mount:            vfs.MountLog,
		RunID:            runID,
	}, nil
}

// NewTraceResourceFromRunDir creates a trace resource with log dir under the given run root.
// Use when the run dir is already computed (e.g. via fsutil.GetRunDir for child runs).
func NewTraceResourceFromRunDir(cfg config.Config, runDir, runID string) (*TraceResource, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if err := validate.NonEmpty("runID", runID); err != nil {
		return nil, err
	}
	baseDir := fsutil.GetLogDirFromRunDir(runDir)
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("error creating trace directory %s: %w", baseDir, err)
	}
	return &TraceResource{
		ReadOnlyResource: vfs.ReadOnlyResource{Name: "trace"},
		Cfg:              cfg,
		BaseDir:          baseDir,
		Mount:            vfs.MountLog,
		RunID:            runID,
	}, nil
}

func (tr *TraceResource) SupportsNestedList() bool {
	return false
}

// List lists entries under subpath relative to BaseDir.
func (tr *TraceResource) List(subpath string) ([]vfs.Entry, error) {
	clean, _, err := vfsutil.NormalizeResourceSubpath(subpath)
	if err != nil {
		return nil, err
	}
	if clean == "" || clean == "." {
		// events.since/<offset> and events.latest/<count> are virtual query endpoints.
		return []vfs.Entry{
			{Path: "events", IsDir: false},
			{Path: "events.latest", IsDir: false},
			{Path: "events.since", IsDir: false},
		}, nil
	}
	return nil, fmt.Errorf("invalid subpath %q: cannot list non-root", subpath)
}

// Read reads a file at subpath relative to BaseDir.
func (tr *TraceResource) Read(subpath string) ([]byte, error) {
	clean, parts, err := vfsutil.NormalizeResourceSubpath(subpath)
	if err != nil {
		return nil, fmt.Errorf("trace read: %w", err)
	}
	if clean == "" || clean == "." {
		return nil, fmt.Errorf("trace read: path required (try 'events')")
	}

	switch parts[0] {
	case "events":
		if len(parts) != 1 {
			return nil, fmt.Errorf("trace read: 'events' does not take a suffix (got %q)", clean)
		}
		return tr.readAllEventsJSONL()
	case "events.since":
		if len(parts) != 2 {
			return nil, fmt.Errorf("trace read: expected 'events.since/<offset>' (got %q)", clean)
		}
		offset, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("trace read: events.since offset must be a number (got %q)", clean)
		}
		if offset < 0 {
			return nil, fmt.Errorf("trace read: events.since offset must be non-negative (got %q)", clean)
		}
		b, _, err := tr.ReadEventsSince(offset)
		if err != nil {
			return nil, fmt.Errorf("trace read: error reading events.since: %w", err)
		}
		return b, nil
	case "events.latest":
		if len(parts) != 2 {
			return nil, fmt.Errorf("trace read: expected 'events.latest/<count>' (got %q)", clean)
		}
		count, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("trace read: events.latest count must be a number (got %q)", clean)
		}
		if count < 0 {
			return nil, fmt.Errorf("trace read: events.latest count must be non-negative (got %q)", clean)
		}
		if count > maxLatestCount {
			return nil, fmt.Errorf("trace read: events.latest count exceeds max %d", maxLatestCount)
		}
		b, err := tr.ReadLastEvents(count)
		if err != nil {
			return nil, fmt.Errorf("trace read: error reading events.latest: %w", err)
		}
		return b, nil
	default:
		return nil, fmt.Errorf(
			"trace read: unknown item %q (allowed: events, events.since/<offset>, events.latest/<count>)",
			parts[0],
		)
	}
}

// Write replaces the file at subpath (creating parent directories if needed).
//
// Trace resources are read-only, so this always returns an error.
func (tr *TraceResource) Write(subpath string, _ []byte) error {
	return tr.ReadOnlyResource.Write(subpath, nil)
}

// Append appends bytes to the file at subpath (creating parent directories if needed).
//
// Trace resources are read-only, so this always returns an error.
func (tr *TraceResource) Append(subpath string, _ []byte) error {
	return tr.ReadOnlyResource.Append(subpath, nil)
}

// ReadEventsSince reads events.jsonl from the given byte offset and returns the
// bytes plus the next offset (current file size).
func (tr *TraceResource) ReadEventsSince(offset int64) ([]byte, int64, error) {
	logicalName := "events.since"
	if offset < 0 {
		return nil, 0, fmt.Errorf("trace %s: offset cannot be negative (%d)", logicalName, offset)
	}
	db, err := tr.openSQLite()
	if err != nil {
		return nil, offset, fmt.Errorf("trace %s: open sqlite: %w", logicalName, err)
	}
	defer db.Close()

	// Offset is treated as SQLite event seq (cursor), not an on-disk byte offset.
	// This keeps /log fully virtual and avoids maintaining a mirrored JSONL file.
	rows, err := db.Query(
		`SELECT seq, event_json FROM events WHERE run_id = ? AND seq > ? ORDER BY seq LIMIT ?`,
		tr.RunID,
		offset,
		2000,
	)
	if err != nil {
		return nil, offset, fmt.Errorf("trace %s: query events: %w", logicalName, err)
	}
	defer rows.Close()

	var (
		b         []byte
		nextSeq   = offset
		bytesUsed = 0
	)
	for rows.Next() {
		var seq int64
		var raw string
		if err := rows.Scan(&seq, &raw); err != nil {
			return nil, nextSeq, fmt.Errorf("trace %s: scan: %w", logicalName, err)
		}
		line := []byte(raw + "\n")
		if bytesUsed+len(line) > maxSinceBytes {
			break
		}
		b = append(b, line...)
		bytesUsed += len(line)
		nextSeq = seq
	}
	if err := rows.Err(); err != nil {
		return nil, nextSeq, fmt.Errorf("trace %s: rows error: %w", logicalName, err)
	}
	return b, nextSeq, nil
}

// ReadLastEvents reads the last N events (JSONL lines) from events.jsonl.
// If count is 0, it returns empty.
func (tr *TraceResource) ReadLastEvents(count int) ([]byte, error) {
	logicalName := "events.latest"
	if count == 0 {
		return []byte{}, nil
	}
	if count > maxLatestCount {
		return nil, fmt.Errorf("trace %s: count exceeds max %d", logicalName, maxLatestCount)
	}
	db, err := tr.openSQLite()
	if err != nil {
		return nil, fmt.Errorf("trace %s: open sqlite: %w", logicalName, err)
	}
	defer db.Close()

	rows, err := db.Query(
		`SELECT event_json FROM events WHERE run_id = ? ORDER BY seq DESC LIMIT ?`,
		tr.RunID,
		count,
	)
	if err != nil {
		return nil, fmt.Errorf("trace %s: query events: %w", logicalName, err)
	}
	defer rows.Close()

	lines := make([]string, 0, count)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("trace %s: scan event: %w", logicalName, err)
		}
		lines = append(lines, raw)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("trace %s: rows error: %w", logicalName, err)
	}
	// Reverse to chronological order.
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}
	if len(lines) == 0 {
		return []byte{}, nil
	}
	return []byte(strings.Join(lines, "\n") + "\n"), nil
}

func (tr *TraceResource) openSQLite() (*sql.DB, error) {
	dbPath := fsutil.GetSQLitePath(tr.Cfg.DataDir)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxIdleTime(30 * time.Second)
	return db, nil
}

func (tr *TraceResource) readAllEventsJSONL() ([]byte, error) {
	db, err := tr.openSQLite()
	if err != nil {
		return nil, fmt.Errorf("trace events: open sqlite: %w", err)
	}
	defer db.Close()

	rows, err := db.Query(`SELECT event_json FROM events WHERE run_id = ? ORDER BY seq`, tr.RunID)
	if err != nil {
		return nil, fmt.Errorf("trace events: query: %w", err)
	}
	defer rows.Close()

	var b []byte
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("trace events: scan: %w", err)
		}
		b = append(b, raw...)
		b = append(b, '\n')
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("trace events: rows: %w", err)
	}
	return b, nil
}
