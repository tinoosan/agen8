package runtime

import (
	"context"
	"testing"

	"github.com/tinoosan/agen8/pkg/events"
	"github.com/tinoosan/agen8/pkg/types"
)

func TestAuditObserverEmitsEvent(t *testing.T) {
	var got []string
	obs := newAuditObserver("run-1", func(ctx context.Context, ev events.Event) {
		_ = ctx
		got = append(got, ev.Type)
	}, true)
	if obs == nil {
		t.Fatalf("observer is nil")
	}
	obs.ObserveHostOp(types.HostOpRequest{Op: types.HostOpFSWrite, Path: "/x"}, types.HostOpResponse{Ok: true})
	if len(got) != 1 || got[0] != "audit.hostop" {
		t.Fatalf("expected audit.hostop event, got %v", got)
	}
}
