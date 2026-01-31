package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/tinoosan/workbench-core/pkg/events"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// auditObserver emits side-effectful host ops as structured events (stored via StoreSink as JSONL).
// This replaces per-file audit JSON in /outbox.
type auditObserver struct {
	emit  func(ctx context.Context, ev events.Event)
	runID string
	seq   *uint64
}

func newAuditObserver(runID string, emit func(ctx context.Context, ev events.Event)) *auditObserver {
	if emit == nil {
		return nil
	}
	var zero uint64
	return &auditObserver{emit: emit, runID: strings.TrimSpace(runID), seq: &zero}
}

func (o *auditObserver) ObserveHostOp(req types.HostOpRequest, resp types.HostOpResponse) {
	if o == nil || o.emit == nil {
		return
	}
	if !isSideEffectOp(req.Op) {
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

func formatSeq(n uint64) string {
	return strings.TrimSpace(strings.TrimLeft(strings.TrimPrefix(strings.TrimSpace(strings.ToLower(strings.TrimSpace(string([]byte{'a' + byte(n%26)})))), "#"), "#"))
}

func maybeInt(v int) string {
	if v == 0 {
		return ""
	}
	return fmt.Sprintf("%d", v)
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
