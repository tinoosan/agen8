package resources

import (
	"fmt"
	"os"
	"path/filepath"
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

func NewTraceResource(runId string) (*TraceResource, error) {
	if runId == "" {
		return nil, fmt.Errorf("runId cannot be empty")
	}
	baseDir := fsutil.GetTraceDir(config.DataDir, runId)
	// create trace directory if it doesn't exist
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("error creating trace directory %s: %w", baseDir, err)
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
func (tr *TraceResource) List(subpath string) ([]vfs.Entry, error) {
	// Treat "" and "." as the root of the trace resource
	if subpath == "" || subpath == "." {
		return []vfs.Entry{
			{Path: "events", IsDir: false},
			{Path: "run", IsDir: false},
		}, nil
	}

	// Anything else is a non-root list request
	return nil, fmt.Errorf("invalid subpath %q: cannot list non-root", subpath)
}

// Read reads a file at subpath relative to BaseDir.
// subpath is resource-relative (no leading "/").
func (tr *TraceResource) Read(subpath string) ([]byte, error) {
	// vfs.Resolve("/trace/events") gives subpath "events"
	// vfs.Resolve("/trace") gives subpath ""

	// Normalise
	subpath = strings.TrimSpace(subpath)
	if subpath == "" || subpath == "." {
		return nil, fmt.Errorf("trace read: path required (try 'run' or 'events')")
	}
	if strings.HasPrefix(subpath, "/") {
		return nil, fmt.Errorf("trace read: absolute paths not allowed: %q", subpath)
	}

	// Prevent nested paths like "events/1"
	if strings.Contains(subpath, "/") || strings.Contains(subpath, string(filepath.Separator)) {
		return nil, fmt.Errorf("trace read: invalid path %q: nested paths not alloed", subpath)
	}

	var targetPath string

	switch subpath {
	case "run:":
		targetPath = filepath.Join(tr.BaseDir, "run.json")
	case "events":
		targetPath = filepath.Join(tr.BaseDir, "events.jsonl")
	default:
		return nil, fmt.Errorf("trace read: unknown item %q (allowed: run, events)", subpath)
	}

	b, err := os.ReadFile(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("trace read: %s does not exist: %w", targetPath, err)
		}
		return nil, fmt.Errorf("trace read %q (%s): %w", subpath, targetPath, err)
	}
	return b, nil
}

// Write replaces the file at subpath (creating parent directories if needed).
// subpath is resource-relative (no leading "/").
func (tr *TraceResource) Write(subpath string, data []byte) error {
	return fmt.Errorf("write not supported for trace resource")
}

// Append appends bytes to the file at subpath (creating parent directories if needed).
// subpath is resource-relative (no leading "/").
func (tr *TraceResource) Append(subpath string, data []byte) error {
	return fmt.Errorf("append not supported for trace resource")
}
