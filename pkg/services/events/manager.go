package events

import (
	"context"
	"errors"
	"strings"

	implstore "github.com/tinoosan/agen8/internal/store"
	"github.com/tinoosan/agen8/pkg/config"
	eventpkg "github.com/tinoosan/agen8/pkg/events"
	"github.com/tinoosan/agen8/pkg/types"
)

// Ensure Service implements eventpkg.StoreAppender (for StoreSink).
var _ eventpkg.StoreAppender = (*Service)(nil)

// Service implements EventStore and pkg/events.StoreAppender by delegating to internal/store.
type Service struct {
	cfg config.Config
}

// ErrRunIDRequired is returned when runID is blank where it is required.
var ErrRunIDRequired = errors.New("runID is required")

// NewService creates an events service that uses the given config (e.g. DataDir for SQLite).
func NewService(cfg config.Config) *Service {
	return &Service{cfg: cfg}
}

// Append persists one event. It delegates to the store.
func (s *Service) Append(ctx context.Context, event types.EventRecord) error {
	return implstore.AppendEvent(ctx, s.cfg, event)
}

// AppendEvent implements pkg/events.StoreAppender for use with StoreSink.
func (s *Service) AppendEvent(ctx context.Context, event types.EventRecord) error {
	return s.Append(ctx, event)
}

// ListPaginated returns events matching the filter and the cursor for the next page.
func (s *Service) ListPaginated(ctx context.Context, filter Filter) ([]types.EventRecord, int64, error) {
	storeFilter := toStoreFilter(filter)
	return implstore.ListEventsPaginated(s.cfg, storeFilter)
}

// Count returns the total number of events matching the filter.
func (s *Service) Count(ctx context.Context, filter Filter) (int, error) {
	storeFilter := toStoreFilter(filter)
	return implstore.CountEvents(s.cfg, storeFilter)
}

// LatestSeq returns the maximum seq for the run without loading events.
func (s *Service) LatestSeq(ctx context.Context, runID string) (int64, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return 0, ErrRunIDRequired
	}
	return implstore.GetLatestEventSeq(s.cfg, runID)
}

// Tail streams new events for the run from fromOffset. Caller cancels ctx to stop.
func (s *Service) Tail(ctx context.Context, runID string, fromOffset int64) (<-chan TailedEvent, <-chan error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		ec := make(chan TailedEvent)
		errc := make(chan error, 1)
		errc <- ErrRunIDRequired
		close(ec)
		close(errc)
		return ec, errc
	}
	storeCh, errCh := implstore.TailEvents(s.cfg, ctx, runID, fromOffset)
	outCh := make(chan TailedEvent)
	go func() {
		defer close(outCh)
		for te := range storeCh {
			outCh <- TailedEvent{Event: te.Event, NextOffset: te.NextOffset}
		}
	}()
	return outCh, errCh
}

func toStoreFilter(f Filter) implstore.EventFilter {
	return implstore.EventFilter{
		RunID:     f.RunID,
		Limit:     f.Limit,
		Offset:    f.Offset,
		AfterSeq:  f.AfterSeq,
		BeforeSeq: f.BeforeSeq,
		Types:     f.Types,
		SortDesc:  f.SortDesc,
	}
}
