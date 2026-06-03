package app

import (
	"context"
	"fmt"
	"testing"

	"github.com/tinoosan/agen8-mcp-server/internal/services/events/domain"
	"github.com/tinoosan/agen8-mcp-server/pkg/protocol"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

type stubEventService struct {
	appendErr error
	appended  []types.EventRecord
}

func (s *stubEventService) Append(_ context.Context, event types.EventRecord) error {
	if s.appendErr != nil {
		return s.appendErr
	}
	s.appended = append(s.appended, event)
	return nil
}

func (s *stubEventService) ListPaginated(context.Context, domain.EventFilter) ([]types.EventRecord, int64, error) {
	return nil, 0, nil
}

func (s *stubEventService) LatestSeq(context.Context, string) (int64, error) {
	return 0, nil
}

func (s *stubEventService) Count(context.Context, domain.EventFilter) (int, error) {
	return 0, nil
}

func TestBroadcastingService_AppendBroadcastsAfterSuccess(t *testing.T) {
	inner := &stubEventService{}
	broadcastCh := make(chan protocol.Message, 1)
	svc := NewBroadcastingService(inner, broadcastCh)

	event := types.EventRecord{RunID: "run-1", Type: "test.event", Message: "hello"}
	if err := svc.Append(context.Background(), event); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	if len(inner.appended) != 1 {
		t.Fatalf("len(inner.appended)=%d want 1", len(inner.appended))
	}

	select {
	case msg := <-broadcastCh:
		if msg.Method != protocol.MethodNotifyEventAppend {
			t.Fatalf("method=%q want %q", msg.Method, protocol.MethodNotifyEventAppend)
		}
	default:
		t.Fatalf("expected broadcast notification")
	}
}

func TestBroadcastingService_AppendSkipsBroadcastOnFailure(t *testing.T) {
	inner := &stubEventService{appendErr: errStub}
	broadcastCh := make(chan protocol.Message, 1)
	svc := NewBroadcastingService(inner, broadcastCh)

	err := svc.Append(context.Background(), types.EventRecord{RunID: "run-1", Type: "test.event", Message: "hello"})
	if err == nil {
		t.Fatal("expected error")
	}

	select {
	case msg := <-broadcastCh:
		t.Fatalf("unexpected broadcast: %+v", msg)
	default:
	}
}

var errStub = fmt.Errorf("stub error")
