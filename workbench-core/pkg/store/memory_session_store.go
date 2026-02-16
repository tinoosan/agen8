package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tinoosan/workbench-core/pkg/timeutil"
	"github.com/tinoosan/workbench-core/pkg/types"
)

type memorySessionEntry struct {
	session     types.Session
	createdAt   time.Time
	updatedAt   time.Time
	title       string
	currentGoal string
}

// MemorySessionStore is an in-memory implementation of SessionStore intended for tests.
// It also implements the session service Store interface (runs, activities) so
// tests can use pkgsession.NewManager(cfg, store, supervisor) and pass the Manager as Session.
//
// It maintains separate created/updated timestamps (like the SQLite columns) so
// sorting/filtering matches the SQL-backed behavior even when Session JSON fields
// omit UpdatedAt.
type MemorySessionStore struct {
	mu         sync.RWMutex
	sessions   map[string]memorySessionEntry
	runs       map[string]types.Run
	activities map[string][]types.Activity // runID -> activities
}

func NewMemorySessionStore() *MemorySessionStore {
	return &MemorySessionStore{
		sessions:   make(map[string]memorySessionEntry),
		runs:       make(map[string]types.Run),
		activities: make(map[string][]types.Activity),
	}
}

func (s *MemorySessionStore) LoadSession(_ context.Context, sessionID string) (types.Session, error) {
	if s == nil {
		return types.Session{}, fmt.Errorf("session store not configured")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return types.Session{}, fmt.Errorf("sessionId is required: %w", ErrInvalid)
	}
	s.mu.RLock()
	ent, ok := s.sessions[sessionID]
	s.mu.RUnlock()
	if !ok {
		return types.Session{}, fmt.Errorf("session %s does not exist: %w", sessionID, errors.Join(ErrNotFound, os.ErrNotExist))
	}
	return cloneSession(ent.session), nil
}

func (s *MemorySessionStore) SaveSession(_ context.Context, sess types.Session) error {
	if s == nil {
		return fmt.Errorf("session store not configured")
	}
	sessionID := strings.TrimSpace(sess.SessionID)
	if sessionID == "" {
		return fmt.Errorf("sessionId is required: %w", ErrInvalid)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Merge runs/current run ID in the same shape as internal/store.SaveSession.
	if existing, ok := s.sessions[sessionID]; ok {
		sess = mergeSessionIndex(existing.session, sess)
	}

	now := time.Now().UTC()
	createdAt := now
	if sess.CreatedAt != nil && !sess.CreatedAt.IsZero() {
		createdAt = sess.CreatedAt.UTC()
	}
	updatedAt := now
	if sess.UpdatedAt != nil && !sess.UpdatedAt.IsZero() {
		updatedAt = sess.UpdatedAt.UTC()
	}

	ent := memorySessionEntry{
		session:     cloneSession(sess),
		title:       strings.TrimSpace(sess.Title),
		currentGoal: strings.TrimSpace(sess.CurrentGoal),
		updatedAt:   updatedAt,
		createdAt:   createdAt,
	}
	if existing, ok := s.sessions[sessionID]; ok {
		// Preserve created_at semantics: do not overwrite an existing created_at.
		ent.createdAt = existing.createdAt
	}
	s.sessions[sessionID] = ent
	return nil
}

func (s *MemorySessionStore) ListSessionIDs(_ context.Context) ([]string, error) {
	if s == nil {
		return nil, fmt.Errorf("session store not configured")
	}
	s.mu.RLock()
	ids := make([]string, 0, len(s.sessions))
	for id := range s.sessions {
		ids = append(ids, id)
	}
	s.mu.RUnlock()
	sort.Strings(ids)
	return ids, nil
}

func (s *MemorySessionStore) ListSessions(ctx context.Context) ([]types.Session, error) {
	ids, err := s.ListSessionIDs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]types.Session, 0, len(ids))
	for _, id := range ids {
		sess, err := s.LoadSession(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	sort.Slice(out, func(i, j int) bool {
		left := timeutil.FirstNonZero(out[i].UpdatedAt, out[i].CreatedAt)
		right := timeutil.FirstNonZero(out[j].UpdatedAt, out[j].CreatedAt)
		if left.Equal(right) {
			return strings.Compare(out[i].SessionID, out[j].SessionID) < 0
		}
		return left.After(right)
	})
	return out, nil
}

func (s *MemorySessionStore) ListSessionsPaginated(_ context.Context, filter SessionFilter) ([]types.Session, error) {
	if s == nil {
		return nil, fmt.Errorf("session store not configured")
	}
	sortCol, ok := normalizeSessionSortColumn(filter.SortBy)
	if !ok {
		return nil, fmt.Errorf("invalid sort column: %s", filter.SortBy)
	}

	titleContains := strings.TrimSpace(filter.TitleContains)
	titleContainsLower := strings.ToLower(titleContains)

	s.mu.RLock()
	all := make([]memorySessionEntry, 0, len(s.sessions))
	for _, ent := range s.sessions {
		if !filter.IncludeSystem && ent.session.System {
			continue
		}
		if titleContainsLower != "" {
			if !strings.Contains(strings.ToLower(ent.title), titleContainsLower) &&
				!strings.Contains(strings.ToLower(ent.currentGoal), titleContainsLower) {
				continue
			}
		}
		all = append(all, ent)
	}
	s.mu.RUnlock()

	sortDesc := true
	if !filter.SortDesc {
		sortDesc = false
	}

	sort.Slice(all, func(i, j int) bool {
		a := all[i]
		b := all[j]
		cmp := 0
		switch sortCol {
		case "updated_at":
			cmp = compareTime(a.updatedAt, b.updatedAt)
		case "created_at":
			cmp = compareTime(a.createdAt, b.createdAt)
		case "title":
			cmp = strings.Compare(strings.ToLower(a.title), strings.ToLower(b.title))
		default:
			cmp = compareTime(a.updatedAt, b.updatedAt)
		}
		if cmp == 0 {
			cmp = strings.Compare(a.session.SessionID, b.session.SessionID)
		}
		if sortDesc {
			return cmp > 0
		}
		return cmp < 0
	})

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	if offset >= len(all) {
		return []types.Session{}, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}

	out := make([]types.Session, 0, end-offset)
	for _, ent := range all[offset:end] {
		out = append(out, cloneSession(ent.session))
	}
	return out, nil
}

func (s *MemorySessionStore) CountSessions(_ context.Context, filter SessionFilter) (int, error) {
	if s == nil {
		return 0, fmt.Errorf("session store not configured")
	}
	titleContains := strings.TrimSpace(filter.TitleContains)
	titleContainsLower := strings.ToLower(titleContains)
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, ent := range s.sessions {
		if !filter.IncludeSystem && ent.session.System {
			continue
		}
		if titleContainsLower == "" {
			count++
			continue
		}
		if strings.Contains(strings.ToLower(ent.title), titleContainsLower) ||
			strings.Contains(strings.ToLower(ent.currentGoal), titleContainsLower) {
			count++
		}
	}
	return count, nil
}

func (s *MemorySessionStore) DeleteSession(_ context.Context, sessionID string) error {
	if s == nil {
		return fmt.Errorf("session store not configured")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("sessionId is required: %w", ErrInvalid)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
	return nil
}

func (s *MemorySessionStore) LoadRun(_ context.Context, runID string) (types.Run, error) {
	if s == nil {
		return types.Run{}, fmt.Errorf("session store not configured")
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return types.Run{}, fmt.Errorf("runID is required")
	}
	s.mu.RLock()
	run, ok := s.runs[runID]
	s.mu.RUnlock()
	if !ok {
		return types.Run{}, fmt.Errorf("run %s not found: %w", runID, ErrNotFound)
	}
	return run, nil
}

func (s *MemorySessionStore) SaveRun(_ context.Context, run types.Run) error {
	if s == nil {
		return fmt.Errorf("session store not configured")
	}
	runID := strings.TrimSpace(run.RunID)
	if runID == "" {
		return fmt.Errorf("runID is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runs == nil {
		s.runs = make(map[string]types.Run)
	}
	s.runs[runID] = run
	return nil
}

func (s *MemorySessionStore) ListRunsBySession(_ context.Context, sessionID string) ([]types.Run, error) {
	if s == nil {
		return nil, fmt.Errorf("session store not configured")
	}
	sessionID = strings.TrimSpace(sessionID)
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []types.Run
	for _, run := range s.runs {
		if strings.TrimSpace(run.SessionID) == sessionID {
			out = append(out, run)
		}
	}
	return out, nil
}

func (s *MemorySessionStore) ListChildRuns(_ context.Context, parentRunID string) ([]types.Run, error) {
	if s == nil {
		return nil, fmt.Errorf("session store not configured")
	}
	parentRunID = strings.TrimSpace(parentRunID)
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []types.Run
	for _, run := range s.runs {
		if strings.TrimSpace(run.ParentRunID) == parentRunID {
			out = append(out, run)
		}
	}
	return out, nil
}

func (s *MemorySessionStore) AddRunToSession(_ context.Context, sessionID, runID string) (types.Session, error) {
	if s == nil {
		return types.Session{}, fmt.Errorf("session store not configured")
	}
	sessionID = strings.TrimSpace(sessionID)
	runID = strings.TrimSpace(runID)
	if sessionID == "" || runID == "" {
		return types.Session{}, fmt.Errorf("sessionID and runID are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ent, ok := s.sessions[sessionID]
	if !ok {
		return types.Session{}, fmt.Errorf("session %s not found", sessionID)
	}
	seen := make(map[string]struct{})
	for _, id := range ent.session.Runs {
		seen[strings.TrimSpace(id)] = struct{}{}
	}
	if _, exists := seen[runID]; !exists {
		ent.session.Runs = append(ent.session.Runs, runID)
	}
	if strings.TrimSpace(ent.session.CurrentRunID) == "" {
		ent.session.CurrentRunID = runID
	}
	s.sessions[sessionID] = ent
	return cloneSession(ent.session), nil
}

func (s *MemorySessionStore) ListActivities(_ context.Context, runID string, limit, offset int) ([]types.Activity, error) {
	if s == nil {
		return nil, fmt.Errorf("session store not configured")
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, fmt.Errorf("runID is required")
	}
	if limit <= 0 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	s.mu.RLock()
	acts := s.activities[runID]
	s.mu.RUnlock()
	if len(acts) <= offset {
		return nil, nil
	}
	end := offset + limit
	if end > len(acts) {
		end = len(acts)
	}
	return acts[offset:end], nil
}

func (s *MemorySessionStore) CountActivities(_ context.Context, runID string) (int, error) {
	if s == nil {
		return 0, fmt.Errorf("session store not configured")
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return 0, fmt.Errorf("runID is required")
	}
	s.mu.RLock()
	n := len(s.activities[runID])
	s.mu.RUnlock()
	return n, nil
}

func normalizeSessionSortColumn(col string) (string, bool) {
	switch strings.TrimSpace(strings.ToLower(col)) {
	case "", "updated_at":
		return "updated_at", true
	case "created_at":
		return "created_at", true
	case "title":
		return "title", true
	default:
		return "", false
	}
}

func compareTime(a, b time.Time) int {
	if a.Equal(b) {
		return 0
	}
	if a.After(b) {
		return 1
	}
	return -1
}

func mergeSessionIndex(existing types.Session, incoming types.Session) types.Session {
	merged := make([]string, 0, len(existing.Runs)+len(incoming.Runs))
	seen := map[string]struct{}{}
	for _, id := range existing.Runs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		merged = append(merged, id)
	}
	for _, id := range incoming.Runs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		merged = append(merged, id)
	}
	if len(merged) > 0 {
		incoming.Runs = merged
	}
	if strings.TrimSpace(incoming.CurrentRunID) == "" {
		incoming.CurrentRunID = existing.CurrentRunID
	}
	if len(incoming.Runs) < len(existing.Runs) {
		incoming.CurrentRunID = existing.CurrentRunID
	}
	return incoming
}

func cloneSession(in types.Session) types.Session {
	out := in
	if in.CreatedAt != nil {
		t := *in.CreatedAt
		out.CreatedAt = &t
	}
	if in.UpdatedAt != nil {
		t := *in.UpdatedAt
		out.UpdatedAt = &t
	}
	if in.Runs != nil {
		out.Runs = append([]string(nil), in.Runs...)
	}
	return out
}
