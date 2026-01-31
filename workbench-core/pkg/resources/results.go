package resources

import (
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/pkg/store"
	"github.com/tinoosan/workbench-core/pkg/vfs"
	"github.com/tinoosan/workbench-core/pkg/vfsutil"
)

// ResultsResource is a /results VFS resource backed by a store.ResultsView.
//
// Write/Append are not supported: results is read-only to the agent.
type ResultsResource struct {
	vfs.ReadOnlyResource
	// BaseDir is unused by this resource, but kept for consistency/debugging.
	BaseDir string

	// Mount is the virtual mount name used by the VFS.
	Mount string

	// Store is the host-side backing store for call results.
	Store store.ResultsView
}

// NewResultsResource creates a new virtual /results mount backed by a ResultsView.
func NewResultsResource(s store.ResultsView) (*ResultsResource, error) {
	if s == nil {
		return nil, fmt.Errorf("results store is required")
	}
	return &ResultsResource{
		ReadOnlyResource: vfs.ReadOnlyResource{Name: "results"},
		BaseDir:          "",
		Mount:            vfs.MountResults,
		Store:            s,
	}, nil
}

func (r *ResultsResource) SupportsNestedList() bool {
	return true
}

// List lists entries under subpath.
func (r *ResultsResource) List(subpath string) ([]vfs.Entry, error) {
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
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// Read reads bytes under /results.
func (r *ResultsResource) Read(subpath string) ([]byte, error) {
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
func (r *ResultsResource) Write(_ string, _ []byte) error {
	return r.ReadOnlyResource.Write("", nil)
}

// Append is not supported; /results is read-only to the agent.
func (r *ResultsResource) Append(_ string, _ []byte) error {
	return r.ReadOnlyResource.Append("", nil)
}
