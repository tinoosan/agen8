package resources

import (
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/vfs"
	"github.com/tinoosan/workbench-core/internal/vfsutil"
)

// VirtualResultsResource is a ResultsResource implementation backed by a ResultsStore.
//
// This replaces the on-disk "/results" directory with a virtual mount while preserving
// the agent-visible VFS contract:
//   - fs.list("/results") returns call IDs (directories)
//   - fs.read("/results/<callId>/response.json") returns response JSON bytes
//   - fs.read("/results/<callId>/<artifactPath>") returns artifact bytes (if present)
//
// Write/Append are not supported: results is read-only to the agent.
type VirtualResultsResource struct {
	// BaseDir is the OS directory backing this resource (the sandbox root).
	//
	// VirtualResultsResource is not disk-backed, so BaseDir is unused and typically empty.
	// It exists only to keep resources consistent and debuggable.
	BaseDir string

	// Mount is the virtual mount name used by the VFS.
	// Example: "results" maps to the virtual namespace "/results".
	Mount string

	// Store is the host-side backing store for call results.
	Store store.ResultsView
}

// NewVirtualResultsResource creates a new virtual /results mount backed by a ResultsStore.
func NewVirtualResultsResource(s store.ResultsView) (*VirtualResultsResource, error) {
	if s == nil {
		return nil, fmt.Errorf("results store is required")
	}
	return &VirtualResultsResource{
		BaseDir: "",
		Mount:   vfs.MountResults,
		Store:   s,
	}, nil
}

// List lists entries under subpath.
//
// List("") lists known call IDs as directories.
// List("<callId>") lists at least "response.json" and any stored artifacts for that call.
func (r *VirtualResultsResource) List(subpath string) ([]vfs.Entry, error) {
	if r == nil || r.Store == nil {
		return nil, fmt.Errorf("results store not configured")
	}
	clean, parts, err := vfsutil.NormalizeResourceSubpath(subpath)
	if err != nil {
		return nil, err
	}

	// Root listing: call IDs.
	if clean == "" || clean == "." {
		callIDs, err := r.Store.ListCallIDs()
		if err != nil {
			return nil, err
		}
		out := make([]vfs.Entry, 0, len(callIDs))
		for _, id := range callIDs {
			out = append(out, vfs.Entry{Path: id, IsDir: true})
		}
		return out, nil
	}

	// Only support listing a call directory: "<callId>"
	if len(parts) != 1 {
		return nil, fmt.Errorf("results list: expected callId or root (got %q)", subpath)
	}
	callID := parts[0]

	respBytes, err := r.Store.GetCallResponseJSON(callID)
	if err != nil {
		return nil, err
	}
	artifacts, err := r.Store.ListArtifacts(callID)
	if err != nil && !errors.Is(err, store.ErrResultsNotFound) {
		return nil, err
	}

	out := make([]vfs.Entry, 0, 1+len(artifacts))
	out = append(out, vfs.NewFileEntry(path.Join(callID, "response.json"), int64(len(respBytes)), time.Time{}))
	// ModTime is not meaningful for virtual stores; mark HasModTime=false.
	out[0].HasModTime = false

	for _, a := range artifacts {
		e := vfs.NewFileEntry(path.Join(callID, a.Path), a.Size, a.ModTime)
		if a.ModTime.IsZero() {
			e.HasModTime = false
		}
		out = append(out, e)
	}
	// Stable sort by path.
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// Read reads bytes under /results.
//
// Supported paths:
//   - "<callId>"                  -> response.json bytes (convenience)
//   - "<callId>/response.json"    -> response.json bytes
//   - "<callId>/<artifactPath>"   -> artifact bytes
func (r *VirtualResultsResource) Read(subpath string) ([]byte, error) {
	if r == nil || r.Store == nil {
		return nil, fmt.Errorf("results store not configured")
	}
	clean, parts, err := vfsutil.NormalizeResourceSubpath(subpath)
	if err != nil {
		return nil, err
	}
	if clean == "" || clean == "." {
		return nil, fmt.Errorf("results read: path is required")
	}

	// Convenience: Read("<callId>") returns response.json.
	if len(parts) == 1 {
		return r.Store.GetCallResponseJSON(parts[0])
	}

	callID := parts[0]
	rel := strings.Join(parts[1:], "/")
	rel = path.Clean(rel)
	if rel == "response.json" {
		return r.Store.GetCallResponseJSON(callID)
	}

	b, _, err := r.Store.GetArtifact(callID, rel)
	return b, err
}

// Write is not supported; /results is read-only to the agent.
func (r *VirtualResultsResource) Write(subpath string, data []byte) error {
	return fmt.Errorf("results is read-only")
}

// Append is not supported; /results is read-only to the agent.
func (r *VirtualResultsResource) Append(subpath string, data []byte) error {
	return fmt.Errorf("results is read-only")
}
