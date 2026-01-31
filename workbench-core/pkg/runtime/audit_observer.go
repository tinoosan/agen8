package runtime

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tinoosan/workbench-core/pkg/types"
	"github.com/tinoosan/workbench-core/pkg/vfs"
)

// auditObserver records side-effectful host ops to /outbox as append-only audit entries.
// This replaces the prior checklist gate by allowing execution while still emitting
// a durable trail of what happened.
type auditObserver struct {
	fs  *vfs.FS
	seq *uint64
}

func newAuditObserver(fs *vfs.FS) *auditObserver {
	if fs == nil {
		return nil
	}
	var zero uint64
	return &auditObserver{fs: fs, seq: &zero}
}

func (o *auditObserver) ObserveHostOp(req types.HostOpRequest, resp types.HostOpResponse) {
	if o == nil || o.fs == nil {
		return
	}
	if !isSideEffectOp(req.Op) {
		return
	}

	entry := map[string]any{
		"op":        strings.TrimSpace(req.Op),
		"path":      strings.TrimSpace(req.Path),
		"ok":        resp.Ok,
		"error":     strings.TrimSpace(resp.Error),
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
	}
	b, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return
	}
	name := fmt.Sprintf("audit-%d.json", atomic.AddUint64(o.seq, 1))
	_ = o.fs.Write(path.Join("/outbox", name), b)
}

func isSideEffectOp(op string) bool {
	switch strings.ToLower(strings.TrimSpace(op)) {
	case types.HostOpFSWrite, types.HostOpFSAppend, types.HostOpFSEdit, types.HostOpFSPatch,
		types.HostOpShellExec, types.HostOpHTTPFetch, types.HostOpToolRun:
		return true
	default:
		return false
	}
}
