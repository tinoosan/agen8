package resources

import (
	"bufio"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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
	eventsPath := filepath.Join(baseDir, "events.jsonl")
	if _, err := os.Stat(eventsPath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("error checking trace events %s: %w", eventsPath, err)
		}
		if err := os.WriteFile(eventsPath, []byte{}, 0644); err != nil {
			return nil, fmt.Errorf("error creating trace events %s: %w", eventsPath, err)
		}
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
		if err := tr.ensureEventsFile(); err != nil {
			return nil, fmt.Errorf("trace read: ensure events: %w", err)
		}
		targetPath := filepath.Join(tr.BaseDir, "events.jsonl")
		return readFile(targetPath, "events")
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
	targetPath := filepath.Join(tr.BaseDir, "events.jsonl")

	if offset < 0 {
		return nil, 0, fmt.Errorf("trace %s: offset cannot be negative (%d)", logicalName, offset)
	}
	if err := tr.ensureEventsFile(); err != nil {
		return nil, 0, fmt.Errorf("trace %s: ensure events: %w", logicalName, err)
	}

	f, err := os.Open(targetPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []byte{}, 0, nil
		}
		return nil, 0, fmt.Errorf("trace %s: open %s: %w", logicalName, targetPath, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, 0, fmt.Errorf("trace %s: stat %s: %w", logicalName, targetPath, err)
	}

	size := info.Size()
	if offset > size {
		return []byte{}, size, nil
	}
	if size-offset > maxSinceBytes {
		return nil, size, fmt.Errorf("trace %s: requested %d bytes exceeds max %d", logicalName, size-offset, maxSinceBytes)
	}

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, 0, fmt.Errorf("trace %s: seek %s to %d: %w", logicalName, targetPath, offset, err)
	}

	b, err := io.ReadAll(f)
	if err != nil {
		return nil, 0, fmt.Errorf("trace %s: read %s from %d: %w", logicalName, targetPath, offset, err)
	}

	return b, size, nil
}

// ReadLastEvents reads the last N events (JSONL lines) from events.jsonl.
// If count is 0, it returns empty.
func (tr *TraceResource) ReadLastEvents(count int) ([]byte, error) {
	logicalName := "events.latest"
	targetPath := filepath.Join(tr.BaseDir, "events.jsonl")

	if count == 0 {
		return []byte{}, nil
	}
	if count > maxLatestCount {
		return nil, fmt.Errorf("trace %s: count exceeds max %d", logicalName, maxLatestCount)
	}
	if err := tr.ensureEventsFile(); err != nil {
		return nil, fmt.Errorf("trace %s: ensure events: %w", logicalName, err)
	}

	f, err := os.Open(targetPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []byte{}, nil
		}
		return nil, fmt.Errorf("trace %s: open %s: %w", logicalName, targetPath, err)
	}
	defer f.Close()

	b, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("trace %s: read %s: %w", logicalName, targetPath, err)
	}

	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return []byte{}, nil
	}

	if count >= len(lines) {
		return []byte(strings.Join(lines, "\n") + "\n"), nil
	}

	out := strings.Join(lines[len(lines)-count:], "\n") + "\n"
	return []byte(out), nil
}

func (tr *TraceResource) ensureEventsFile() error {
	targetPath := filepath.Join(tr.BaseDir, "events.jsonl")
	if _, err := os.Stat(targetPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("trace ensure events: stat %s: %w", targetPath, err)
	}
	return tr.rebuildEventsFile(targetPath)
}

func (tr *TraceResource) rebuildEventsFile(targetPath string) error {
	if err := os.MkdirAll(tr.BaseDir, 0755); err != nil {
		return fmt.Errorf("trace rebuild: mkdir %s: %w", tr.BaseDir, err)
	}

	dbPath := fsutil.GetSQLitePath(tr.Cfg.DataDir)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("trace rebuild: open sqlite: %w", err)
	}
	defer db.Close()

	rows, err := db.Query(`SELECT event_json FROM events WHERE run_id = ? ORDER BY seq`, tr.RunID)
	if err != nil {
		return fmt.Errorf("trace rebuild: query events: %w", err)
	}
	defer rows.Close()

	f, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("trace rebuild: create %s: %w", targetPath, err)
	}
	defer f.Close()

	writer := bufio.NewWriter(f)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return fmt.Errorf("trace rebuild: scan event: %w", err)
		}
		if _, err := writer.WriteString(raw + "\n"); err != nil {
			return fmt.Errorf("trace rebuild: write event: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("trace rebuild: rows error: %w", err)
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("trace rebuild: flush: %w", err)
	}
	return nil
}

func readFile(targetPath, logicalName string) ([]byte, error) {
	b, err := os.ReadFile(targetPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("trace %s: file %s does not exist: %w", logicalName, targetPath, err)
		}
		return nil, fmt.Errorf("trace %s: error reading file %s: %w", logicalName, targetPath, err)
	}
	return b, nil
}
