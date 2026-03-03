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

	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/fsutil"
	pkgstore "github.com/tinoosan/agen8/pkg/store"
	"github.com/tinoosan/agen8/pkg/timeutil"
	"github.com/tinoosan/agen8/pkg/types"
	"github.com/tinoosan/agen8/pkg/validate"
)

// SessionFilter specifies filtering and pagination for session queries.
//
// This is a type alias so internal and pkg store layers cannot diverge.
type SessionFilter = pkgstore.SessionFilter

// allowedSessionSortColumn validates sort column to prevent SQL injection.
func allowedSessionSortColumn(col string) (string, bool) {
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

// CreateSession creates and persists a new session along with its main run.
//
// The returned Session+Run pair ensures every session starts with an active Main Run.
// Sessions are stored in SQLite (data/agen8.db by default).
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
	createdAt := timeutil.FormatRFC3339Nano(s.CreatedAt)
	updatedAt := timeutil.FormatRFC3339Nano(s.UpdatedAt)
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
		left := timeutil.FirstNonZero(out[i].UpdatedAt, out[i].CreatedAt)
		right := timeutil.FirstNonZero(out[j].UpdatedAt, out[j].CreatedAt)
		if left.Equal(right) {
			return strings.Compare(out[i].SessionID, out[j].SessionID) < 0
		}
		return left.After(right)
	})
	return out, nil
}

// ListSessionsPaginated returns sessions with server-side pagination.
// Uses SQL LIMIT/OFFSET for efficient querying of large session sets.
func ListSessionsPaginated(cfg config.Config, filter SessionFilter) ([]types.Session, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	db, err := getSQLiteDB(cfg)
	if err != nil {
		return nil, err
	}

	sortCol, ok := allowedSessionSortColumn(filter.SortBy)
	if !ok {
		return nil, fmt.Errorf("invalid sort column: %s", filter.SortBy)
	}

	sortDir := "DESC"
	if !filter.SortDesc {
		sortDir = "ASC"
	}

	query := `SELECT session_id, session_json FROM sessions WHERE 1=1`
	args := []any{}
	if !filter.IncludeSystem {
		query += ` AND COALESCE(json_extract(session_json, '$.system'), 0) = 0`
	}

	titleContains := strings.TrimSpace(filter.TitleContains)
	if titleContains != "" {
		query += ` AND (title LIKE ? COLLATE NOCASE OR current_goal LIKE ? COLLATE NOCASE)`
		pattern := "%" + titleContains + "%"
		args = append(args, pattern, pattern)
	}

	projectRoot := strings.TrimSpace(filter.ProjectRoot)
	if projectRoot != "" {
		query += ` AND json_extract(session_json, '$.projectRoot') = ?`
		args = append(args, projectRoot)
	}

	// Ensure a stable ordering across pages.
	query += fmt.Sprintf(` ORDER BY %s %s, session_id ASC`, sortCol, sortDir)

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	query += ` LIMIT ?`
	args = append(args, limit)

	if filter.Offset > 0 {
		query += ` OFFSET ?`
		args = append(args, filter.Offset)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	out := make([]types.Session, 0, limit)
	for rows.Next() {
		var sessionID string
		var b []byte
		if err := rows.Scan(&sessionID, &b); err != nil {
			return nil, err
		}
		var s types.Session
		if err := json.Unmarshal(b, &s); err != nil {
			return nil, fmt.Errorf("unmarshal session: %w", err)
		}
		if strings.TrimSpace(s.SessionID) == "" {
			s.SessionID = strings.TrimSpace(sessionID)
		}
		if strings.TrimSpace(s.Title) == "" {
			s.Title = strings.TrimSpace(s.CurrentGoal)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// CountSessions returns the total number of sessions matching the filter.
// Used for pagination UI ("page X of Y").
func CountSessions(cfg config.Config, filter SessionFilter) (int, error) {
	if err := cfg.Validate(); err != nil {
		return 0, err
	}

	db, err := getSQLiteDB(cfg)
	if err != nil {
		return 0, err
	}

	query := `SELECT COUNT(*) FROM sessions WHERE 1=1`
	args := []any{}

	// Filtering logic to match ListSessionsPaginated
	type sessionJSON struct {
		System bool `json:"system"`
	}

	// Simple filter translation
	if !filter.IncludeSystem {
		query += ` AND COALESCE(json_extract(session_json, '$.system'), 0) = 0`
	}

	titleContains := strings.TrimSpace(filter.TitleContains)
	if titleContains != "" {
		query += ` AND (title LIKE ? COLLATE NOCASE OR current_goal LIKE ? COLLATE NOCASE)`
		pattern := "%" + titleContains + "%"
		args = append(args, pattern, pattern)
	}

	projectRoot := strings.TrimSpace(filter.ProjectRoot)
	if projectRoot != "" {
		query += ` AND json_extract(session_json, '$.projectRoot') = ?`
		args = append(args, projectRoot)
	}

	var count int
	if err := db.QueryRow(query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count sessions: %w", err)
	}
	return count, nil
}

type HistoryClearResult struct {
	SourceRuns          []string
	EventsDeleted       int64
	HistoryDeleted      int64
	ActivitiesDeleted   int64
	ConstructorState    int64
	ConstructorManifest int64
}

// ClearHistoryForSession removes persisted run history artifacts for all runs in a session.
func ClearHistoryForSession(cfg config.Config, sessionID string) (HistoryClearResult, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return HistoryClearResult{}, errors.New("session id required")
	}
	if err := cfg.Validate(); err != nil {
		return HistoryClearResult{}, err
	}
	db, err := getSQLiteDB(cfg)
	if err != nil {
		return HistoryClearResult{}, err
	}
	rows, err := db.Query(`SELECT run_id FROM runs WHERE session_id = ? ORDER BY created_at`, sessionID)
	if err != nil {
		return HistoryClearResult{}, fmt.Errorf("query session runs: %w", err)
	}
	defer rows.Close()
	runIDs := make([]string, 0, 8)
	for rows.Next() {
		var runID string
		if err := rows.Scan(&runID); err != nil {
			return HistoryClearResult{}, fmt.Errorf("scan session runs: %w", err)
		}
		runID = strings.TrimSpace(runID)
		if runID != "" {
			runIDs = append(runIDs, runID)
		}
	}
	if err := rows.Err(); err != nil {
		return HistoryClearResult{}, fmt.Errorf("read session runs: %w", err)
	}
	return ClearHistoryForRunIDs(cfg, runIDs)
}

// ClearHistoryForRunIDs removes persisted run history artifacts for the given run IDs.
func ClearHistoryForRunIDs(cfg config.Config, runIDs []string) (HistoryClearResult, error) {
	if err := cfg.Validate(); err != nil {
		return HistoryClearResult{}, err
	}
	ordered := make([]string, 0, len(runIDs))
	seen := make(map[string]struct{}, len(runIDs))
	for _, runID := range runIDs {
		runID = strings.TrimSpace(runID)
		if runID == "" {
			continue
		}
		if _, ok := seen[runID]; ok {
			continue
		}
		seen[runID] = struct{}{}
		ordered = append(ordered, runID)
	}
	result := HistoryClearResult{SourceRuns: append([]string(nil), ordered...)}
	if len(ordered) == 0 {
		return result, nil
	}
	db, err := getSQLiteDB(cfg)
	if err != nil {
		return HistoryClearResult{}, err
	}
	tx, err := db.Begin()
	if err != nil {
		return HistoryClearResult{}, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	runArgs := make([]any, 0, len(ordered))
	placeholders := make([]string, 0, len(ordered))
	for _, runID := range ordered {
		placeholders = append(placeholders, "?")
		runArgs = append(runArgs, runID)
	}
	inClause := "(" + strings.Join(placeholders, ",") + ")"
	if result.EventsDeleted, err = execRowsAffectedTx(tx, "DELETE FROM events WHERE run_id IN "+inClause, runArgs...); err != nil {
		return HistoryClearResult{}, err
	}
	if result.ActivitiesDeleted, err = execRowsAffectedTx(tx, "DELETE FROM activities WHERE run_id IN "+inClause, runArgs...); err != nil {
		return HistoryClearResult{}, err
	}
	if result.ConstructorState, err = execRowsAffectedTx(tx, "DELETE FROM constructor_state WHERE run_id IN "+inClause, runArgs...); err != nil {
		return HistoryClearResult{}, err
	}
	if result.ConstructorManifest, err = execRowsAffectedTx(tx, "DELETE FROM constructor_manifest WHERE run_id IN "+inClause, runArgs...); err != nil {
		return HistoryClearResult{}, err
	}
	if result.HistoryDeleted, err = execRowsAffectedTx(tx, "DELETE FROM history WHERE run_id IN "+inClause, runArgs...); err != nil {
		return HistoryClearResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return HistoryClearResult{}, fmt.Errorf("commit clear history: %w", err)
	}
	return result, nil
}

// DeleteSession removes the session from the database and deletes its directory.
func DeleteSession(cfg config.Config, sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("session id required")
	}

	if err := cfg.Validate(); err != nil {
		return err
	}

	sess, sessErr := LoadSession(cfg, sessionID)

	db, err := getSQLiteDB(cfg)
	if err != nil {
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	rows, err := tx.Query(`SELECT run_id FROM runs WHERE session_id = ?`, sessionID)
	if err != nil {
		return fmt.Errorf("query session runs: %w", err)
	}
	runIDs := make([]string, 0, 8)
	for rows.Next() {
		var runID string
		if err := rows.Scan(&runID); err != nil {
			rows.Close()
			return fmt.Errorf("scan session runs: %w", err)
		}
		runID = strings.TrimSpace(runID)
		if runID != "" {
			runIDs = append(runIDs, runID)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("read session runs: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close session runs: %w", err)
	}

	if len(runIDs) > 0 {
		runArgs := make([]any, 0, len(runIDs))
		placeholders := make([]string, 0, len(runIDs))
		for _, runID := range runIDs {
			placeholders = append(placeholders, "?")
			runArgs = append(runArgs, runID)
		}
		inClause := "(" + strings.Join(placeholders, ",") + ")"
		if _, err := tx.Exec("DELETE FROM events WHERE run_id IN "+inClause, runArgs...); err != nil {
			return fmt.Errorf("delete run events: %w", err)
		}
		if _, err := tx.Exec("DELETE FROM history WHERE run_id IN "+inClause, runArgs...); err != nil {
			return fmt.Errorf("delete run history: %w", err)
		}
		if _, err := tx.Exec("DELETE FROM activities WHERE run_id IN "+inClause, runArgs...); err != nil {
			return fmt.Errorf("delete run activities: %w", err)
		}
		if _, err := tx.Exec("DELETE FROM constructor_state WHERE run_id IN "+inClause, runArgs...); err != nil {
			return fmt.Errorf("delete run constructor state: %w", err)
		}
		if _, err := tx.Exec("DELETE FROM constructor_manifest WHERE run_id IN "+inClause, runArgs...); err != nil {
			return fmt.Errorf("delete run constructor manifest: %w", err)
		}
		if _, err := tx.Exec("DELETE FROM runs WHERE run_id IN "+inClause, runArgs...); err != nil {
			return fmt.Errorf("delete runs: %w", err)
		}
	}

	// Also delete any remaining session-scoped history rows.
	if _, err := tx.Exec("DELETE FROM history WHERE session_id = ?", sessionID); err != nil {
		return fmt.Errorf("delete session history: %w", err)
	}

	if _, err := tx.Exec("DELETE FROM sessions WHERE session_id = ?", sessionID); err != nil {
		return fmt.Errorf("delete session record: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete: %w", err)
	}

	// Delete run directories under /agents/<runID>.
	for _, runID := range runIDs {
		runDir := fsutil.GetAgentDir(cfg.DataDir, runID)
		if err := os.RemoveAll(runDir); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove run dir %s: %w", runDir, err)
		}
	}

	// If this was a team session, remove the team directory as well.
	if sessErr == nil {
		teamID := strings.TrimSpace(sess.TeamID)
		if teamID != "" {
			teamDir := fsutil.GetTeamDir(cfg.DataDir, teamID)
			if err := os.RemoveAll(teamDir); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove team dir: %w", err)
			}
		}
	}

	// Delete directory
	sessDir := fsutil.GetSessionDir(cfg.DataDir, sessionID)
	if err := os.RemoveAll(sessDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove session dir: %w", err)
	}

	return nil
}

func execRowsAffectedTx(tx *sql.Tx, query string, args ...any) (int64, error) {
	if tx == nil {
		return 0, fmt.Errorf("transaction is nil")
	}
	res, err := tx.Exec(query, args...)
	if err != nil {
		return 0, fmt.Errorf("exec: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}
	return n, nil
}
