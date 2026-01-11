package resources

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/fsutil"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

type TraceResource struct {
	// BaseDir is the OS directory backing this resource (the sandbox root).
	// All operations are confined under BaseDir. The resource must reject any
	// subpath that would escape BaseDir (e.g. "..", absolute paths).
	//
	// BaseDir is an implementation detail; callers interact via virtual paths
	// like "/trace/events" through the VFS.
	//
	// Example: "/data/runs/run-123/trace".
	BaseDir string

	// Mount is the virtual mount name used by the VFS.
	// Example: "trace" maps to the virtual namespace "/trace".
	Mount string

	// RunId is the run this trace directory belongs to.
	RunId string
}

const (
	maxLatestCount = 200
	maxSinceBytes  = 64 * 1024
)

func NewTraceResource(runId string) (*TraceResource, error) {
	if runId == "" {
		return nil, fmt.Errorf("runId cannot be empty")
	}
	baseDir := fsutil.GetTraceDir(config.DataDir, runId)
	// create trace directory if it doesn't exist
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
		BaseDir: baseDir,
		Mount:   vfs.MountTrace,
		RunId:   runId,
	}, nil
}

// List lists entries under subpath relative to BaseDir.
// subpath is resource-relative (no leading "/").
// List("") lists the resource root.
//
// Returns an error if:
//   - subpath is non-root
//
// Example:
//
//	entries, err := tr.List("")
//	// Returns entries like "events", "events.since", "events.latest"
func (tr *TraceResource) List(subpath string) ([]vfs.Entry, error) {
	// Treat "" and "." as the root of the trace resource
	if subpath == "" || subpath == "." {
		return []vfs.Entry{
			{Path: "events", IsDir: false},
			{Path: "events.latest", IsDir: false},
			{Path: "events.since", IsDir: false},
		}, nil
	}

	// Anything else is a non-root list request
	return nil, fmt.Errorf("invalid subpath %q: cannot list non-root", subpath)
}

// Read reads a file at subpath relative to BaseDir.
// subpath is resource-relative (no leading "/").
//
// Supported subpaths:
//   - "events": full events.jsonl
//   - "events.since/<offset>": events from byte offset to EOF
//   - "events.latest/<count>": last N events (JSONL lines)
//
// Returns an error if:
//   - subpath is empty or absolute
//   - subpath is not one of the supported patterns
//
// Example:
//
//	b, err := tr.Read("events.since/0")
func (tr *TraceResource) Read(subpath string) ([]byte, error) {
	// Normalise
	subpath = strings.TrimSpace(subpath)
	if subpath == "" || subpath == "." {
		return nil, fmt.Errorf("trace read: path required (try 'events')")
	}
	if strings.HasPrefix(subpath, "/") {
		return nil, fmt.Errorf("trace read: absolute paths not allowed: %q", subpath)
	}

	// Split on "/" so we can support patterns like:
	// "events.since/123" and "events.latest/3"
	parts := strings.Split(subpath, "/")

	// Route by first segment
	switch parts[0] {

	case "events":
		// must be exactly "events"
		if len(parts) != 1 {
			return nil, fmt.Errorf("trace read: 'events' does not take a suffix (got %q)", subpath)
		}
		targetPath := filepath.Join(tr.BaseDir, "events.jsonl")
		return readFile(targetPath, "events")

	case "events.since":
		// expects "events.since/<offset>"
		if len(parts) != 2 {
			return nil, fmt.Errorf("trace read: expected 'events.since/<offset>' (got %q)", subpath)
		}
		offsetStr := parts[1]
		offset, err := strconv.ParseInt(offsetStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("trace read: events.since offset must be a number (got %q)", subpath)
		}
		if offset < 0 {
			return nil, fmt.Errorf("trace read: events.since offset must be non-negative (got %q)", subpath)
		}

		b, _, err := tr.ReadEventsSince(offset)
		if err != nil {
			return nil, fmt.Errorf("trace read: error reading events.since: %w", err)
		}
		return b, nil

	case "events.latest":
		// expects "events.latest/<count>"
		if len(parts) != 2 {
			return nil, fmt.Errorf("trace read: expected 'events.latest/<count>' (got %q)", subpath)
		}
		countStr := parts[1]
		count, err := strconv.Atoi(countStr)
		if err != nil {
			return nil, fmt.Errorf("trace read: events.latest count must be a number (got %q)", subpath)
		}
		if count < 0 {
			return nil, fmt.Errorf("trace read: events.latest count must be non-negative (got %q)", subpath)
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
// subpath is resource-relative (no leading "/").
//
// Trace resources are read-only, so this always returns an error.
func (tr *TraceResource) Write(subpath string, data []byte) error {
	return fmt.Errorf("write not supported for trace resource")
}

// Append appends bytes to the file at subpath (creating parent directories if needed).
// subpath is resource-relative (no leading "/").
//
// Trace resources are read-only, so this always returns an error.
func (tr *TraceResource) Append(subpath string, data []byte) error {
	return fmt.Errorf("append not supported for trace resource")
}

// ReadEventsSince reads events.jsonl from the given byte offset and returns the
// bytes plus the next offset (current file size).
func (tr *TraceResource) ReadEventsSince(offset int64) ([]byte, int64, error) {
	logicalName := "events.since"
	targetPath := filepath.Join(tr.BaseDir, "events.jsonl")

	if offset < 0 {
		return nil, 0, fmt.Errorf("trace %s: offset cannot be negative (%d)", logicalName, offset)
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
