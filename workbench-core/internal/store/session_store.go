package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/types"
	"github.com/tinoosan/workbench-core/pkg/validate"
)

// CreateSession creates and persists a new session along with its main run.
//
// The returned Session+Run pair ensures every session starts with an active Main Run.
// Sessions are stored in SQLite (data/workbench.db by default).
func CreateSession(cfg config.Config, goal string, maxBytesForContext int) (types.Session, types.Run, error) {
	if err := cfg.Validate(); err != nil {
		return types.Session{}, types.Run{}, err
	}
	s := types.NewSession(goal)
	if err := SaveSession(cfg, s); err != nil {
		return types.Session{}, types.Run{}, err
	}

	run := types.NewRun(goal, maxBytesForContext, s.SessionID)
	if err := SaveRun(cfg, run); err != nil {
		return types.Session{}, types.Run{}, err
	}
	updated, err := AddRunToSession(cfg, s.SessionID, run.RunID)
	if err != nil {
		return types.Session{}, types.Run{}, err
	}
	return updated, run, nil
}

// SaveSession persists a session's session.json file.
func SaveSession(cfg config.Config, s types.Session) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := validate.NonEmpty("sessionId", s.SessionID); err != nil {
		return err
	}
	existing, err := LoadSession(cfg, s.SessionID)
	if err == nil {
		merged := make([]string, 0, len(existing.Runs)+len(s.Runs))
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
		for _, id := range s.Runs {
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
			s.Runs = merged
		}
		if strings.TrimSpace(s.CurrentRunID) == "" {
			s.CurrentRunID = existing.CurrentRunID
		}
		if len(s.Runs) < len(existing.Runs) {
			s.CurrentRunID = existing.CurrentRunID
		}
	}
	db, err := getSQLiteDB(cfg)
	if err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	b, err := types.MarshalPretty(s)
	if err != nil {
		return err
	}
	runsJSON, err := json.Marshal(s.Runs)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	createdAt := timePtrToString(s.CreatedAt)
	updatedAt := timePtrToString(s.UpdatedAt)
	if updatedAt == "" {
		updatedAt = now
	}
	if createdAt == "" {
		createdAt = now
	}
	_, err = tx.Exec(
		`INSERT INTO sessions (session_id, title, current_run_id, current_goal, runs_json, session_json, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(session_id) DO UPDATE SET
		   title=excluded.title,
		   current_run_id=excluded.current_run_id,
		   current_goal=excluded.current_goal,
		   runs_json=excluded.runs_json,
		   session_json=excluded.session_json,
		   created_at=COALESCE(sessions.created_at, excluded.created_at),
		   updated_at=excluded.updated_at`,
		s.SessionID,
		nullIfEmpty(s.Title),
		nullIfEmpty(s.CurrentRunID),
		nullIfEmpty(s.CurrentGoal),
		string(runsJSON),
		string(b),
		createdAt,
		updatedAt,
	)
	if err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	return tx.Commit()
}

// LoadSession reads session.json for a session ID.
func LoadSession(cfg config.Config, sessionID string) (types.Session, error) {
	if err := cfg.Validate(); err != nil {
		return types.Session{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if err := validate.NonEmpty("sessionId", sessionID); err != nil {
		return types.Session{}, err
	}
	db, err := getSQLiteDB(cfg)
	if err != nil {
		return types.Session{}, err
	}
	var b []byte
	if err := db.QueryRow(`SELECT session_json FROM sessions WHERE session_id = ?`, sessionID).Scan(&b); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return types.Session{}, fmt.Errorf("session %s does not exist: %w", sessionID, errors.Join(ErrNotFound, os.ErrNotExist))
		}
		return types.Session{}, fmt.Errorf("error reading session %s: %w", sessionID, err)
	}
	var s types.Session
	if err := json.Unmarshal(b, &s); err != nil {
		return types.Session{}, fmt.Errorf("error unmarshalling session json: %w", err)
	}
	if err := validate.NonEmpty("sessionId", s.SessionID); err != nil {
		return types.Session{}, fmt.Errorf("invalid session.json: missing sessionId: %w", ErrInvalid)
	}
	return s, nil
}

// AddRunToSession appends runId to the session index (if not already present)
// and updates CurrentRunID.
func AddRunToSession(cfg config.Config, sessionID, runID string) (types.Session, error) {
	if err := cfg.Validate(); err != nil {
		return types.Session{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	runID = strings.TrimSpace(runID)
	if err := validate.NonEmpty("sessionId", sessionID); err != nil {
		return types.Session{}, err
	}
	if err := validate.NonEmpty("runId", runID); err != nil {
		return types.Session{}, err
	}
	s, err := LoadSession(cfg, sessionID)
	if err != nil {
		return types.Session{}, err
	}
	seen := false
	for _, existing := range s.Runs {
		if existing == runID {
			seen = true
			break
		}
	}
	if !seen {
		s.Runs = append(s.Runs, runID)
	}
	s.CurrentRunID = runID
	return s, SaveSession(cfg, s)
}

// ListSessionIDs returns all session IDs currently on disk, sorted ascending.
func ListSessionIDs(cfg config.Config) ([]string, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	db, err := getSQLiteDB(cfg)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT session_id FROM sessions ORDER BY session_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// ListSessions returns all sessions sorted by most recently updated.
func ListSessions(cfg config.Config) ([]types.Session, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	ids, err := ListSessionIDs(cfg)
	if err != nil {
		return nil, err
	}
	out := make([]types.Session, 0, len(ids))
	for _, id := range ids {
		s, err := LoadSession(cfg, id)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		return sessionSortTime(out[i]).After(sessionSortTime(out[j]))
	})
	return out, nil
}

func sessionSortTime(s types.Session) time.Time {
	if s.UpdatedAt != nil {
		return *s.UpdatedAt
	}
	if s.CreatedAt != nil {
		return *s.CreatedAt
	}
	return time.Time{}
}
