package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/tinoosan/agen8/pkg/events"
	"github.com/tinoosan/agen8/pkg/types"
)

// auditObserver emits side-effectful host ops as structured events (stored via StoreSink as JSONL).
// This replaces per-file audit JSON in /outbox.
type auditObserver struct {
	emit       func(ctx context.Context, ev events.Event)
	runID      string
	seq        *uint64
	auditReads bool
}

func newAuditObserver(runID string, emit func(ctx context.Context, ev events.Event), auditReads bool) *auditObserver {
	if emit == nil {
		return nil
	}
	var zero uint64
	return &auditObserver{emit: emit, runID: strings.TrimSpace(runID), seq: &zero, auditReads: auditReads}
}

func (o *auditObserver) ObserveHostOp(req types.HostOpRequest, resp types.HostOpResponse) {
	if o == nil || o.emit == nil {
		return
	}
	if !shouldAuditOp(req.Op, o.auditReads) {
		return
	}
	seq := atomic.AddUint64(o.seq, 1)
	data := map[string]string{
		"runId":  o.runID,
		"seq":    fmt.Sprintf("%d", seq),
		"op":     strings.TrimSpace(req.Op),
		"path":   strings.TrimSpace(req.Path),
		"ok":     boolToStr(resp.Ok),
		"error":  strings.TrimSpace(resp.Error),
		"bytes":  maybeInt(resp.BytesLen),
		"status": maybeInt(resp.Status),
	}
	o.emit(context.Background(), events.Event{
		Type:    "audit.hostop",
		Message: "Host op",
		Data:    data,
	})
}

func boolToStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func maybeInt(v int) string {
	if v == 0 {
		return ""
	}
	return fmt.Sprintf("%d", v)
}

func shouldAuditOp(op string, auditReads bool) bool {
	switch strings.ToLower(strings.TrimSpace(op)) {
	case types.HostOpFSRead, types.HostOpFSList:
		return auditReads
	case types.HostOpFSWrite, types.HostOpFSAppend, types.HostOpFSEdit, types.HostOpFSPatch,
		types.HostOpShellExec, types.HostOpHTTPFetch:
		return true
	default:
		return false
	}
}
