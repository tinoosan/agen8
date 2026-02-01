package state

import (
	"context"
	"sync"
	"time"

	"github.com/tinoosan/workbench-core/pkg/types"
)

type MemoryStore struct {
	mu sync.Mutex
	m  map[string]Record
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{m: map[string]Record{}}
}

func (s *MemoryStore) RecoverExpired(ctx context.Context, now time.Time) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, rec := range s.m {
		if rec.Status == StatusActive && !rec.LeaseUntil.IsZero() && rec.LeaseUntil.Before(now) {
			rec.Status = StatusFailed
			rec.UpdatedAt = now
			if rec.Error == "" {
				rec.Error = "lease expired"
			}
			s.m[id] = rec
		}
	}
	return nil
}

func (s *MemoryStore) Claim(ctx context.Context, taskID string, ttl time.Duration) (ClaimResult, error) {
	_ = ctx
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	now := time.Now().UTC()
	leaseUntil := now.Add(ttl)

	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.m[taskID]
	if !ok {
		s.m[taskID] = Record{
			TaskID:     taskID,
			Status:     StatusActive,
			Attempts:   1,
			LeaseUntil: leaseUntil,
			UpdatedAt:  now,
		}
		return ClaimResult{Claimed: true, Attempts: 1, LeaseUntil: leaseUntil}, nil
	}
	switch rec.Status {
	case StatusSucceeded, StatusCanceled, StatusQuarantined:
		return ClaimResult{Claimed: false, Attempts: rec.Attempts}, nil
	}
	if rec.Status == StatusActive && rec.LeaseUntil.After(now) {
		return ClaimResult{Claimed: false, Attempts: rec.Attempts, LeaseUntil: rec.LeaseUntil}, nil
	}
	rec.Status = StatusActive
	rec.Attempts++
	rec.LeaseUntil = leaseUntil
	rec.UpdatedAt = now
	s.m[taskID] = rec
	return ClaimResult{Claimed: true, Attempts: rec.Attempts, LeaseUntil: leaseUntil}, nil
}

func (s *MemoryStore) Extend(ctx context.Context, taskID string, ttl time.Duration) error {
	_ = ctx
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	now := time.Now().UTC()
	leaseUntil := now.Add(ttl)
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.m[taskID]
	if !ok {
		return nil
	}
	if rec.Status != StatusActive {
		return nil
	}
	rec.LeaseUntil = leaseUntil
	rec.UpdatedAt = now
	s.m[taskID] = rec
	return nil
}

func (s *MemoryStore) Complete(ctx context.Context, taskID string, result types.TaskResult) error {
	_ = ctx
	now := time.Now().UTC()
	status := Status(result.Status)
	switch status {
	case StatusSucceeded, StatusFailed, StatusCanceled:
	default:
		status = StatusFailed
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.m[taskID]
	if !ok {
		rec = Record{TaskID: taskID}
	}
	rec.Status = status
	rec.LeaseUntil = time.Time{}
	rec.UpdatedAt = now
	rec.Error = result.Error
	rec.Result = &result
	s.m[taskID] = rec
	return nil
}

func (s *MemoryStore) Quarantine(ctx context.Context, taskID string, errMsg string) error {
	_ = ctx
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.m[taskID]
	if !ok {
		rec = Record{TaskID: taskID}
	}
	rec.Status = StatusQuarantined
	rec.LeaseUntil = time.Time{}
	rec.UpdatedAt = now
	rec.Error = errMsg
	s.m[taskID] = rec
	return nil
}

func (s *MemoryStore) Get(ctx context.Context, taskID string) (Record, bool, error) {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.m[taskID]
	return rec, ok, nil
}

