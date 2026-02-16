package app

import (
	"context"
	"errors"
	"strings"

	"github.com/tinoosan/workbench-core/pkg/protocol"
	eventsvc "github.com/tinoosan/workbench-core/pkg/services/events"
)

func registerEventsHandlers(s *RPCServer, reg methodRegistry) error {
	return registerHandlers(
		func() error {
			return addBoundHandler[protocol.EventsListPaginatedParams, protocol.EventsListPaginatedResult](reg, protocol.MethodEventsListPaginated, false, s.eventsListPaginated)
		},
		func() error {
			return addBoundHandler[protocol.EventsLatestSeqParams, protocol.EventsLatestSeqResult](reg, protocol.MethodEventsLatestSeq, false, s.eventsLatestSeq)
		},
		func() error {
			return addBoundHandler[protocol.EventsCountParams, protocol.EventsCountResult](reg, protocol.MethodEventsCount, false, s.eventsCount)
		},
	)
}

var errEventsServiceNotConfigured = errors.New("events service not configured")

func (s *RPCServer) eventsListPaginated(ctx context.Context, p protocol.EventsListPaginatedParams) (protocol.EventsListPaginatedResult, error) {
	if s.eventsService == nil {
		return protocol.EventsListPaginatedResult{}, errEventsServiceNotConfigured
	}
	runID := strings.TrimSpace(p.RunID)
	if runID == "" {
		return protocol.EventsListPaginatedResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "runId is required"}
	}
	filter := eventsvc.Filter{
		RunID:     runID,
		Limit:     p.Limit,
		Offset:    p.Offset,
		AfterSeq:  p.AfterSeq,
		BeforeSeq: p.BeforeSeq,
		Types:     p.Types,
		SortDesc:  p.SortDesc,
	}
	events, next, err := s.eventsService.ListPaginated(ctx, filter)
	if err != nil {
		return protocol.EventsListPaginatedResult{}, err
	}
	return protocol.EventsListPaginatedResult{Events: events, Next: next}, nil
}

func (s *RPCServer) eventsLatestSeq(ctx context.Context, p protocol.EventsLatestSeqParams) (protocol.EventsLatestSeqResult, error) {
	if s.eventsService == nil {
		return protocol.EventsLatestSeqResult{}, errEventsServiceNotConfigured
	}
	runID := strings.TrimSpace(p.RunID)
	if runID == "" {
		return protocol.EventsLatestSeqResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "runId is required"}
	}
	seq, err := s.eventsService.LatestSeq(ctx, runID)
	if err != nil {
		return protocol.EventsLatestSeqResult{}, err
	}
	return protocol.EventsLatestSeqResult{Seq: seq}, nil
}

func (s *RPCServer) eventsCount(ctx context.Context, p protocol.EventsCountParams) (protocol.EventsCountResult, error) {
	if s.eventsService == nil {
		return protocol.EventsCountResult{}, errEventsServiceNotConfigured
	}
	runID := strings.TrimSpace(p.RunID)
	if runID == "" {
		return protocol.EventsCountResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "runId is required"}
	}
	filter := eventsvc.Filter{RunID: runID, Types: p.Types}
	count, err := s.eventsService.Count(ctx, filter)
	if err != nil {
		return protocol.EventsCountResult{}, err
	}
	return protocol.EventsCountResult{Count: count}, nil
}
