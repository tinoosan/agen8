package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/agen8-mcp-server/internal/services/events/domain"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

// Service orchestrates event operations. It normalizes events before
// persisting them through the repository.
type Service struct {
	repo domain.EventRepository
}

var (
	_ domain.EventService  = (*Service)(nil)
	_ domain.EventAppender = (*Service)(nil)
)

func NewService(repo domain.EventRepository) *Service {
	return &Service{repo: repo}
}

func normalizeEventRecord(event types.EventRecord) (types.EventRecord, error) {
	runID := strings.TrimSpace(string(event.RunID))
	if runID == "" {
		return types.EventRecord{}, domain.ErrRunIDRequired
	}

	eventType := strings.TrimSpace(event.Type)
	if eventType == "" {
		return types.EventRecord{}, fmt.Errorf("append event: eventType is required")
	}

	message := strings.TrimSpace(event.Message)
	if message == "" {
		return types.EventRecord{}, fmt.Errorf("append event: message is required")
	}

	origin := strings.TrimSpace(event.Origin)
	data := event.Data
	if data != nil {
		// Back-compat: accept "origin" stored in the Data map, but do not persist it there.
		if rawOrigin, ok := data["origin"]; ok {
			if origin == "" {
				origin = strings.TrimSpace(rawOrigin)
			}
			// Never mutate the input map.
			if len(data) == 1 {
				data = nil
			} else {
				out := make(map[string]string, len(data)-1)
				for k, v := range data {
					if k == "origin" {
						continue
					}
					out[k] = v
				}
				data = out
			}
		}
	}

	if strings.TrimSpace(string(event.EventID)) == "" {
		event.EventID = types.EventID("event-" + uuid.NewString())
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	event.RunID = types.RunID(runID)
	event.Type = eventType
	event.Message = message
	event.Data = data
	event.Origin = origin

	return event, nil
}

// Append normalizes the event (origin, EventID, timestamp) and persists it.
func (s *Service) Append(ctx context.Context, event types.EventRecord) error {
	normalized, err := normalizeEventRecord(event)
	if err != nil {
		return err
	}
	return s.repo.Append(ctx, normalized)
}

func (s *Service) ListPaginated(ctx context.Context, filter domain.EventFilter) ([]types.EventRecord, int64, error) {
	return s.repo.ListPaginated(ctx, filter)
}

func (s *Service) Count(ctx context.Context, filter domain.EventFilter) (int, error) {
	return s.repo.Count(ctx, filter)
}

func (s *Service) LatestSeq(ctx context.Context, runID string) (int64, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return 0, domain.ErrRunIDRequired
	}
	return s.repo.LatestSeq(ctx, runID)
}

func (s *Service) Tail(ctx context.Context, runID string, fromOffset int64) (<-chan domain.TailedEvent, <-chan error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		ec := make(chan domain.TailedEvent)
		errc := make(chan error, 1)
		errc <- domain.ErrRunIDRequired
		close(ec)
		close(errc)
		return ec, errc
	}
	return s.repo.Tail(ctx, runID, fromOffset)
}
