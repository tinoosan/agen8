package app

import (
	"context"

	"github.com/tinoosan/agen8-mcp-server/internal/services/events/domain"
	"github.com/tinoosan/agen8-mcp-server/pkg/protocol"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

// BroadcastingService wraps an EventService and broadcasts every Append
// as a protocol notification to the given channel.
type BroadcastingService struct {
	inner       domain.EventService
	broadcastCh chan<- protocol.Message
}

var _ domain.EventService = (*BroadcastingService)(nil)

func NewBroadcastingService(inner domain.EventService, broadcastCh chan<- protocol.Message) *BroadcastingService {
	return &BroadcastingService{inner: inner, broadcastCh: broadcastCh}
}

func (b *BroadcastingService) Append(ctx context.Context, event types.EventRecord) error {
	// Normalize before the inner call so that the event broadcast via
	// sendNotification has the same populated Timestamp and EventID that
	// will be persisted. Without this, event is passed by value to
	// inner.Append which normalizes its own copy, leaving the original
	// Timestamp as the zero value and causing clients to mis-sort the entry.
	normalized, err := normalizeEventRecord(event)
	if err != nil {
		return err
	}
	err = b.inner.Append(ctx, normalized)
	if err == nil {
		b.sendNotification(normalized)
	}
	return err
}

func (b *BroadcastingService) ListPaginated(ctx context.Context, filter domain.EventFilter) ([]types.EventRecord, int64, error) {
	return b.inner.ListPaginated(ctx, filter)
}

func (b *BroadcastingService) LatestSeq(ctx context.Context, runID string) (int64, error) {
	return b.inner.LatestSeq(ctx, runID)
}

func (b *BroadcastingService) Count(ctx context.Context, filter domain.EventFilter) (int, error) {
	return b.inner.Count(ctx, filter)
}

func (b *BroadcastingService) sendNotification(event types.EventRecord) {
	if b.broadcastCh == nil {
		return
	}
	msg, err := protocol.NewNotification(protocol.MethodNotifyEventAppend, event)
	if err != nil {
		return
	}
	select {
	case b.broadcastCh <- msg:
	default:
	}
}
