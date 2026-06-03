package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	schedule "github.com/tinoosan/agen8-mcp-server/internal/services/schedule/domain"
	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
)

type Service struct {
	repo      schedule.Repository
	clock     schedule.Clock
	executors map[schedule.TargetKind]TargetExecutor
	logger    *slog.Logger
}

type TargetExecutor interface {
	Execute(ctx context.Context, entry schedule.Entry, run schedule.Run) (TargetResult, error)
}

type TargetResult struct {
	TargetObjectID string
}

type CreateParams struct {
	ID          schedule.EntryID
	SpaceID     spacedomain.SpaceID
	CreatedBy   schedule.ActorRef
	Title       string
	Description string
	Timing      schedule.TimingExpression
	Target      schedule.Target
	Context     schedule.Context
	ExpiresAt   *time.Time
	DedupeKey   string
}

type UpdateParams struct {
	EntryID     schedule.EntryID
	Title       *string
	Description *string
	Timing      *schedule.TimingExpression
	Target      *schedule.Target
	ExpiresAt   **time.Time
	DedupeKey   *string
}

func NewService(repo schedule.Repository, clock schedule.Clock, logger *slog.Logger) (*Service, error) {
	switch {
	case repo == nil:
		return nil, fmt.Errorf("schedule service: repository is required")
	case clock == nil:
		return nil, fmt.Errorf("schedule service: clock is required")
	}
	if logger == nil {
		logger = slog.Default().With("service", "schedule")
	}
	return &Service{
		repo:      repo,
		clock:     clock,
		executors: make(map[schedule.TargetKind]TargetExecutor),
		logger:    logger,
	}, nil
}

func (s *Service) RegisterExecutor(kind schedule.TargetKind, executor TargetExecutor) error {
	if strings.TrimSpace(string(kind)) == "" {
		return fmt.Errorf("schedule executor kind is required")
	}
	if executor == nil {
		return fmt.Errorf("schedule executor for %q is required", kind)
	}
	s.executors[kind] = executor
	return nil
}

func (s *Service) Create(ctx context.Context, params CreateParams) (schedule.Entry, error) {
	entry, err := schedule.NewEntry(schedule.NewEntryInput{
		ID:          params.ID,
		SpaceID:     params.SpaceID,
		CreatedBy:   params.CreatedBy,
		Title:       params.Title,
		Description: params.Description,
		Timing:      params.Timing,
		Target:      params.Target,
		Context:     params.Context,
		ExpiresAt:   params.ExpiresAt,
		DedupeKey:   params.DedupeKey,
	}, s.clock.Now())
	if err != nil {
		return schedule.Entry{}, err
	}
	if err := s.repo.Create(ctx, entry); err != nil {
		return schedule.Entry{}, fmt.Errorf("create schedule entry: %w", err)
	}
	return entry, nil
}

func (s *Service) Get(ctx context.Context, id schedule.EntryID) (schedule.Entry, []schedule.Run, error) {
	id = schedule.EntryID(strings.TrimSpace(string(id)))
	if id == "" {
		return schedule.Entry{}, nil, fmt.Errorf("schedule entry id is required")
	}
	entry, err := s.repo.Get(ctx, id)
	if err != nil {
		return schedule.Entry{}, nil, fmt.Errorf("get schedule entry: %w", err)
	}
	runs, err := s.repo.ListRuns(ctx, schedule.RunFilter{EntryID: id, Limit: 10})
	if err != nil {
		return schedule.Entry{}, nil, fmt.Errorf("list schedule runs: %w", err)
	}
	return entry, runs, nil
}

func (s *Service) List(ctx context.Context, filter schedule.Filter) ([]schedule.Entry, error) {
	if filter.Limit < 0 {
		return nil, fmt.Errorf("schedule list limit must be non-negative")
	}
	return s.repo.List(ctx, filter)
}

func (s *Service) Update(ctx context.Context, params UpdateParams) (schedule.Entry, error) {
	id := schedule.EntryID(strings.TrimSpace(string(params.EntryID)))
	if id == "" {
		return schedule.Entry{}, fmt.Errorf("schedule entry id is required")
	}
	entry, err := s.repo.Get(ctx, id)
	if err != nil {
		return schedule.Entry{}, fmt.Errorf("get schedule entry: %w", err)
	}
	if entry.Status != schedule.EntryStatusActive && entry.Status != schedule.EntryStatusPaused {
		return schedule.Entry{}, fmt.Errorf("cannot update schedule entry with status %q", entry.Status)
	}
	next := entry
	if params.Title != nil {
		next.Title = strings.TrimSpace(*params.Title)
	}
	if params.Description != nil {
		next.Description = strings.TrimSpace(*params.Description)
	}
	if params.Timing != nil {
		next.Timing = params.Timing.NormalizedForUpdate()
		runAt, err := next.Timing.FirstRunAfter(s.clock.Now())
		if err != nil {
			return schedule.Entry{}, err
		}
		next.NextRunAt = &runAt
	}
	if params.Target != nil {
		target := params.Target.NormalizedForUpdate()
		next.Target = target
	}
	if params.ExpiresAt != nil {
		next.ExpiresAt = normalizeTimePtr(*params.ExpiresAt)
	}
	if params.DedupeKey != nil {
		next.DedupeKey = strings.TrimSpace(*params.DedupeKey)
	}
	next.UpdatedAt = s.clock.Now().UTC()
	if err := next.Validate(); err != nil {
		return schedule.Entry{}, err
	}
	if next.ExpiresAt != nil && next.NextRunAt != nil && !next.NextRunAt.Before(*next.ExpiresAt) {
		next.Status = schedule.EntryStatusExpired
		next.NextRunAt = nil
	}
	if err := s.repo.Update(ctx, next); err != nil {
		return schedule.Entry{}, fmt.Errorf("update schedule entry: %w", err)
	}
	return next, nil
}

func (s *Service) Cancel(ctx context.Context, id schedule.EntryID) (schedule.Entry, error) {
	id = schedule.EntryID(strings.TrimSpace(string(id)))
	if id == "" {
		return schedule.Entry{}, fmt.Errorf("schedule entry id is required")
	}
	entry, err := s.repo.Get(ctx, id)
	if err != nil {
		return schedule.Entry{}, fmt.Errorf("get schedule entry: %w", err)
	}
	cancelled, err := entry.Cancel(s.clock.Now())
	if err != nil {
		return schedule.Entry{}, err
	}
	if err := s.repo.Update(ctx, cancelled); err != nil {
		return schedule.Entry{}, fmt.Errorf("cancel schedule entry: %w", err)
	}
	return cancelled, nil
}

func (s *Service) RunDue(ctx context.Context, limit int) ([]schedule.Run, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be positive")
	}
	due, err := s.repo.ListDue(ctx, s.clock.Now(), limit)
	if err != nil {
		return nil, fmt.Errorf("list due schedule entries: %w", err)
	}
	runs := make([]schedule.Run, 0, len(due))
	var errs []error
	for _, entry := range due {
		run, err := s.runEntry(ctx, entry)
		if err != nil {
			if run.ID != "" {
				runs = append(runs, run)
			}
			errs = append(errs, err)
			continue
		}
		if run.ID != "" {
			runs = append(runs, run)
		}
	}
	if len(errs) > 0 {
		return runs, fmt.Errorf("run due schedule entries: %w", errorsJoin(errs))
	}
	return runs, nil
}

func (s *Service) runEntry(ctx context.Context, entry schedule.Entry) (schedule.Run, error) {
	if entry.NextRunAt == nil {
		return schedule.Run{}, fmt.Errorf("schedule entry %s has no next run", entry.ID)
	}
	run, err := schedule.NewStartedRun(entry, *entry.NextRunAt, s.clock.Now())
	if err != nil {
		return schedule.Run{}, err
	}
	claimed, ok, err := s.repo.ClaimDue(ctx, run)
	if err != nil {
		return schedule.Run{}, fmt.Errorf("claim schedule entry %s: %w", entry.ID, err)
	}
	if !ok {
		return schedule.Run{}, nil
	}
	executor := s.executors[entry.Target.Kind]
	if executor == nil {
		failed, failErr := claimed.Fail(fmt.Sprintf("no executor registered for target kind %q", entry.Target.Kind), s.clock.Now())
		if failErr != nil {
			return schedule.Run{}, failErr
		}
		if err := s.repo.UpdateRun(ctx, failed); err != nil {
			return schedule.Run{}, err
		}
		if err := s.advanceEntry(ctx, entry); err != nil {
			return failed, err
		}
		return failed, fmt.Errorf("no executor registered for target kind %q", entry.Target.Kind)
	}
	result, execErr := executor.Execute(ctx, entry, claimed)
	var finished schedule.Run
	if execErr != nil {
		finished, err = claimed.Fail(execErr.Error(), s.clock.Now())
	} else {
		finished, err = claimed.Succeed(result.TargetObjectID, s.clock.Now())
	}
	if err != nil {
		return schedule.Run{}, err
	}
	if err := s.repo.UpdateRun(ctx, finished); err != nil {
		return schedule.Run{}, fmt.Errorf("update schedule run %s: %w", finished.ID, err)
	}
	if err := s.advanceEntry(ctx, entry); err != nil {
		return finished, err
	}
	if execErr != nil {
		return finished, fmt.Errorf("execute schedule entry %s: %w", entry.ID, execErr)
	}
	return finished, nil
}

func (s *Service) advanceEntry(ctx context.Context, entry schedule.Entry) error {
	dueAt := *entry.NextRunAt
	next, err := entry.AdvanceAfterRun(dueAt, s.clock.Now())
	if err != nil {
		return err
	}
	if err := s.repo.Update(ctx, next); err != nil {
		return fmt.Errorf("advance schedule entry %s: %w", entry.ID, err)
	}
	return nil
}

func normalizeTimePtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	next := t.UTC()
	return &next
}

func errorsJoin(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}
	return errors.Join(errs...)
}
