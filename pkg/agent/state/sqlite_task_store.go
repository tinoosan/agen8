package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"github.com/tinoosan/agen8/pkg/timeutil"
	"github.com/tinoosan/agen8/pkg/types"
	"github.com/tinoosan/agen8/pkg/validate"
)

type SQLiteTaskStore struct {
	path string

	mu   sync.Mutex
	db   *sql.DB
	once sync.Once
}

const (
	defaultSQLiteBusyTimeoutMS  = 10000
	sqliteBusyRetryMaxAttempts  = 6
	sqliteBusyRetryBaseDelay    = 15 * time.Millisecond
	sqliteBusyRetryMaxBackoff   = 250 * time.Millisecond
	sqliteBusyRetryJitterWindow = 20 * time.Millisecond
)

func NewSQLiteTaskStore(path string) (*SQLiteTaskStore, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("sqlite store path is required")
	}
	return &SQLiteTaskStore{path: path}, nil
}

func (s *SQLiteTaskStore) init() error {
	var initErr error
	s.once.Do(func() {
		if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
			initErr = fmt.Errorf("sqlite: create dir: %w", err)
			return
		}
		db, err := sql.Open("sqlite", s.path)
		if err != nil {
			initErr = fmt.Errorf("sqlite: open: %w", err)
			return
		}
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
		if _, err := db.Exec(`PRAGMA journal_mode=WAL;`); err != nil {
			_ = db.Close()
			initErr = fmt.Errorf("sqlite: set journal_mode: %w", err)
			return
		}
		if _, err := db.Exec(fmt.Sprintf(`PRAGMA busy_timeout=%d;`, sqliteBusyTimeoutMS())); err != nil {
			_ = db.Close()
			initErr = fmt.Errorf("sqlite: set busy_timeout: %w", err)
			return
		}
		if _, err := db.Exec(`PRAGMA foreign_keys=ON;`); err != nil {
			// Best-effort: older databases/driver configs may ignore this.
			_ = err
		}

		if _, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS tasks (
				task_id TEXT PRIMARY KEY,
				session_id TEXT NOT NULL,
				run_id TEXT NOT NULL,
				source_team_id TEXT DEFAULT '',
				destination_team_id TEXT DEFAULT '',
				team_id TEXT DEFAULT '',
				assigned_role TEXT DEFAULT '',
				assigned_to_type TEXT DEFAULT 'agent',
				assigned_to TEXT DEFAULT '',
				claimed_by TEXT DEFAULT '',
				created_by TEXT DEFAULT '',
				goal TEXT NOT NULL,
				inputs_json TEXT,
				priority INTEGER DEFAULT 0,
				status TEXT NOT NULL,
				created_at TEXT NOT NULL,
				started_at TEXT,
				finished_at TEXT,
				completed_at TEXT,
				summary TEXT,
				artifacts_json TEXT,
				error TEXT,
				attempts INTEGER DEFAULT 0,
				lease_until TEXT,
				updated_at TEXT NOT NULL,
				input_tokens INTEGER DEFAULT 0,
				output_tokens INTEGER DEFAULT 0,
				total_tokens INTEGER DEFAULT 0,
				cost_usd REAL DEFAULT 0.0,
				duration_seconds INTEGER DEFAULT 0,
				metadata_json TEXT
			);
		`); err != nil {
			_ = db.Close()
			initErr = fmt.Errorf("sqlite: create tasks: %w", err)
			return
		}
		if _, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS messages (
				message_id TEXT PRIMARY KEY,
				intent_id TEXT NOT NULL,
				correlation_id TEXT NOT NULL,
				causation_id TEXT DEFAULT '',
				producer TEXT DEFAULT '',
				thread_id TEXT NOT NULL,
				run_id TEXT DEFAULT '',
				source_team_id TEXT DEFAULT '',
				destination_team_id TEXT DEFAULT '',
				team_id TEXT DEFAULT '',
				channel TEXT NOT NULL,
				kind TEXT NOT NULL,
				body_json TEXT NOT NULL DEFAULT '{}',
				task_ref TEXT DEFAULT '',
				task_json TEXT DEFAULT '',
				status TEXT NOT NULL,
				lease_owner TEXT DEFAULT '',
				lease_until TEXT,
				attempts INTEGER NOT NULL DEFAULT 0,
				visible_at TEXT NOT NULL,
				priority INTEGER NOT NULL DEFAULT 0,
				error TEXT DEFAULT '',
				metadata_json TEXT NOT NULL DEFAULT '{}',
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				processed_at TEXT
			);
		`); err != nil {
			_ = db.Close()
			initErr = fmt.Errorf("sqlite: create messages: %w", err)
			return
		}
		migrations := []string{
			`ALTER TABLE tasks ADD COLUMN source_team_id TEXT DEFAULT '';`,
			`ALTER TABLE tasks ADD COLUMN destination_team_id TEXT DEFAULT '';`,
			`ALTER TABLE tasks ADD COLUMN team_id TEXT DEFAULT '';`,
			`ALTER TABLE tasks ADD COLUMN assigned_role TEXT DEFAULT '';`,
			`ALTER TABLE tasks ADD COLUMN assigned_to_type TEXT DEFAULT 'agent';`,
			`ALTER TABLE tasks ADD COLUMN assigned_to TEXT DEFAULT '';`,
			`ALTER TABLE tasks ADD COLUMN claimed_by TEXT DEFAULT '';`,
			`ALTER TABLE tasks ADD COLUMN created_by TEXT DEFAULT '';`,
			`ALTER TABLE tasks ADD COLUMN task_kind TEXT DEFAULT 'task';`,
			`ALTER TABLE tasks ADD COLUMN role_snapshot TEXT DEFAULT '';`,
			`ALTER TABLE tasks ADD COLUMN completed_at TEXT;`,
			`ALTER TABLE messages ADD COLUMN source_team_id TEXT DEFAULT '';`,
			`ALTER TABLE messages ADD COLUMN destination_team_id TEXT DEFAULT '';`,
			`ALTER TABLE messages ADD COLUMN team_id TEXT DEFAULT '';`,
		}
		for _, migration := range migrations {
			if _, err := db.Exec(migration); err != nil {
				if !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
					_ = db.Close()
					initErr = fmt.Errorf("sqlite: migrate tasks: %w", err)
					return
				}
			}
		}
		indexes := []string{
			`CREATE INDEX IF NOT EXISTS idx_tasks_session ON tasks(session_id);`,
			`CREATE INDEX IF NOT EXISTS idx_tasks_run ON tasks(run_id);`,
			`CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);`,
			`CREATE INDEX IF NOT EXISTS idx_tasks_created ON tasks(created_at DESC);`,
			`CREATE INDEX IF NOT EXISTS idx_tasks_finished ON tasks(finished_at DESC);`,
			`CREATE INDEX IF NOT EXISTS idx_tasks_cost ON tasks(cost_usd DESC);`,
			`CREATE INDEX IF NOT EXISTS idx_tasks_run_status ON tasks(run_id, status);`,
			`CREATE INDEX IF NOT EXISTS idx_tasks_destination_role ON tasks(destination_team_id, assigned_role);`,
			`CREATE INDEX IF NOT EXISTS idx_tasks_destination_kind ON tasks(destination_team_id, task_kind);`,
			`CREATE INDEX IF NOT EXISTS idx_tasks_source_destination ON tasks(source_team_id, destination_team_id);`,
			`CREATE INDEX IF NOT EXISTS idx_tasks_assigned_to ON tasks(assigned_to_type, assigned_to, status);`,
			`CREATE INDEX IF NOT EXISTS idx_tasks_claimed_by ON tasks(claimed_by, status);`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_messages_thread_intent ON messages(thread_id, intent_id);`,
			`CREATE INDEX IF NOT EXISTS idx_messages_thread_queue ON messages(thread_id, channel, status, visible_at, priority, created_at);`,
			`CREATE INDEX IF NOT EXISTS idx_messages_run_status_visible ON messages(run_id, status, visible_at);`,
			`CREATE INDEX IF NOT EXISTS idx_messages_destination ON messages(destination_team_id, status, visible_at);`,
			`CREATE INDEX IF NOT EXISTS idx_messages_source_destination ON messages(source_team_id, destination_team_id);`,
			`CREATE INDEX IF NOT EXISTS idx_messages_task_ref ON messages(task_ref);`,
			`CREATE INDEX IF NOT EXISTS idx_messages_correlation_id ON messages(correlation_id);`,
			`CREATE INDEX IF NOT EXISTS idx_messages_causation_id ON messages(causation_id);`,
			`CREATE INDEX IF NOT EXISTS idx_messages_status_lease ON messages(status, lease_until);`,
		}
		for _, idx := range indexes {
			if _, err := db.Exec(idx); err != nil {
				_ = db.Close()
				initErr = fmt.Errorf("sqlite: create index: %w", err)
				return
			}
		}
		if _, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS artifacts (
				artifact_id INTEGER PRIMARY KEY AUTOINCREMENT,
				task_id TEXT NOT NULL,
				team_id TEXT DEFAULT '',
				run_id TEXT DEFAULT '',
				role TEXT DEFAULT '',
				task_kind TEXT DEFAULT 'task',
				is_summary INTEGER DEFAULT 0,
				display_name TEXT DEFAULT '',
				vpath TEXT NOT NULL,
				disk_path TEXT DEFAULT '',
				produced_at TEXT NOT NULL,
				day_bucket TEXT NOT NULL
			);
		`); err != nil {
			_ = db.Close()
			initErr = fmt.Errorf("sqlite: create artifacts: %w", err)
			return
		}
		artifactIndexes := []string{
			`CREATE INDEX IF NOT EXISTS idx_artifacts_team_day_role_kind_time ON artifacts(team_id, day_bucket DESC, role, task_kind, produced_at DESC);`,
			`CREATE INDEX IF NOT EXISTS idx_artifacts_task_id ON artifacts(task_id);`,
			`CREATE INDEX IF NOT EXISTS idx_artifacts_team_display_name ON artifacts(team_id, display_name);`,
		}
		for _, idx := range artifactIndexes {
			if _, err := db.Exec(idx); err != nil {
				_ = db.Close()
				initErr = fmt.Errorf("sqlite: create artifact index: %w", err)
				return
			}
		}
		if err := s.backfillArtifactIndex(context.Background(), db); err != nil {
			_ = db.Close()
			initErr = fmt.Errorf("sqlite: backfill artifacts: %w", err)
			return
		}
		if err := migrateDeliverablesToTasks(context.Background(), db, filepath.Dir(s.path)); err != nil {
			_ = db.Close()
			initErr = fmt.Errorf("sqlite: migrate deliverables to tasks: %w", err)
			return
		}
		s.db = db
	})
	return initErr
}

func migrateDeliverablesToTasks(ctx context.Context, db *sql.DB, dataDir string) error {
	dataDir = strings.TrimSpace(dataDir)
	if db == nil || dataDir == "" {
		return nil
	}
	if err := migrateWorkspaceRootsToTasks(dataDir); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `
		UPDATE tasks
		SET artifacts_json = REPLACE(artifacts_json, '/workspace/deliverables/', '/workspace/tasks/')
		WHERE COALESCE(artifacts_json, '') LIKE '%/workspace/deliverables/%'
	`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `
		UPDATE artifacts
		SET vpath = REPLACE(vpath, '/workspace/deliverables/', '/workspace/tasks/'),
		    disk_path = REPLACE(disk_path, '/workspace/deliverables/', '/workspace/tasks/')
		WHERE COALESCE(vpath, '') LIKE '/workspace/deliverables/%'
		   OR COALESCE(disk_path, '') LIKE '%/workspace/deliverables/%'
	`); err != nil {
		return err
	}
	return nil
}

func migrateWorkspaceRootsToTasks(dataDir string) error {
	agentsDir := filepath.Join(dataDir, "agents")
	teamsDir := filepath.Join(dataDir, "teams")
	if err := migrateWorkspaceRoots(filepath.Join(agentsDir), "workspace"); err != nil {
		return err
	}
	if err := migrateWorkspaceRoots(filepath.Join(teamsDir), "workspace"); err != nil {
		return err
	}
	return nil
}

func migrateWorkspaceRoots(root, workspaceLeaf string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		workspaceDir := filepath.Join(root, ent.Name(), workspaceLeaf)
		if err := migrateWorkspaceDeliverablesDir(workspaceDir); err != nil {
			return err
		}
	}
	return nil
}

func migrateWorkspaceDeliverablesDir(workspaceDir string) error {
	legacyDir := filepath.Join(workspaceDir, "deliverables")
	targetDir := filepath.Join(workspaceDir, "tasks")
	if _, err := os.Stat(legacyDir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if _, err := os.Stat(targetDir); err != nil {
		if os.IsNotExist(err) {
			return os.Rename(legacyDir, targetDir)
		}
		return err
	}
	if err := filepath.WalkDir(legacyDir, func(src string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(legacyDir, src)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		dst := filepath.Join(targetDir, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		if _, err := os.Stat(dst); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return copyFileNoOverwrite(src, dst)
	}); err != nil {
		return err
	}
	return os.RemoveAll(legacyDir)
}

func copyFileNoOverwrite(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return nil
		}
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func sqliteBusyTimeoutMS() int {
	raw := strings.TrimSpace(os.Getenv("AGEN8_SQLITE_BUSY_TIMEOUT_MS"))
	if raw == "" {
		return defaultSQLiteBusyTimeoutMS
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return defaultSQLiteBusyTimeoutMS
	}
	return v
}

func isSQLiteBusyErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "sqlite_busy") || strings.Contains(msg, "database is locked")
}

func sqliteBusyRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	backoff := sqliteBusyRetryBaseDelay * time.Duration(1<<(attempt-1))
	if backoff > sqliteBusyRetryMaxBackoff {
		backoff = sqliteBusyRetryMaxBackoff
	}
	if sqliteBusyRetryJitterWindow <= 0 {
		return backoff
	}
	jitter := time.Duration(time.Now().UTC().UnixNano() % int64(sqliteBusyRetryJitterWindow))
	return backoff + jitter
}

func withSQLiteBusyRetry[T any](ctx context.Context, fn func() (T, error)) (T, error) {
	var zero T
	for attempt := 1; attempt <= sqliteBusyRetryMaxAttempts; attempt++ {
		out, err := fn()
		if err == nil {
			return out, nil
		}
		if !isSQLiteBusyErr(err) || attempt == sqliteBusyRetryMaxAttempts {
			return zero, err
		}
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(sqliteBusyRetryDelay(attempt)):
		}
	}
	return zero, fmt.Errorf("sqlite busy retry exhausted")
}

func withSQLiteBusyRetryErr(ctx context.Context, fn func() error) error {
	_, err := withSQLiteBusyRetry(ctx, func() (struct{}, error) {
		return struct{}{}, fn()
	})
	return err
}

func (s *SQLiteTaskStore) dbConn() (*sql.DB, error) {
	if err := s.init(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	db := s.db
	s.mu.Unlock()
	if db == nil {
		return nil, fmt.Errorf("sqlite: db not initialized")
	}
	return db, nil
}

func metadataBool(raw any) bool {
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	case float64:
		return v != 0
	case int:
		return v != 0
	default:
		return strings.EqualFold(strings.TrimSpace(fmt.Sprint(v)), "true")
	}
}

func metadataString(m map[string]any, key string) string {
	if len(m) == 0 {
		return ""
	}
	raw, ok := m[key]
	if !ok || raw == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(raw))
}

func metadataMap(m map[string]any, key string) map[string]any {
	if len(m) == 0 {
		return nil
	}
	v, _ := m[key].(map[string]any)
	return v
}

func metadataArray(m map[string]any, key string) []any {
	if len(m) == 0 {
		return nil
	}
	v, _ := m[key].([]any)
	return v
}

func batchItemStatusFromDecision(decision string) string {
	switch strings.ToLower(strings.TrimSpace(decision)) {
	case "approve":
		return "approved"
	case "retry":
		return "retry"
	case "escalate":
		return "escalated"
	default:
		return "pending_review"
	}
}

// CloseBatchAndHandoffAtomic closes a synthetic batch callback and creates one deterministic
// coordinator handoff task in a single SQL transaction.
func (s *SQLiteTaskStore) CloseBatchAndHandoffAtomic(ctx context.Context, batchTaskID, reviewerIdentity, reviewSummary string) (handoffTaskID string, approved, retried, escalated int, err error) {
	db, err := s.dbConn()
	if err != nil {
		return "", 0, 0, 0, err
	}
	batchTaskID = strings.TrimSpace(batchTaskID)
	if batchTaskID == "" {
		return "", 0, 0, 0, fmt.Errorf("batchTaskID is required")
	}
	reviewerIdentity = strings.TrimSpace(reviewerIdentity)
	if reviewerIdentity == "" {
		reviewerIdentity = "reviewer"
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", 0, 0, 0, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var sessionID, runID, sourceTeamID, destinationTeamID, teamID, assignedRole, assignedToType, assignedTo, taskKind, goal string
	var status, artifactsJSON, inputsJSON, metadataJSON string
	rowErr := tx.QueryRowContext(ctx, `
		SELECT session_id, run_id, COALESCE(source_team_id,''), COALESCE(destination_team_id,''), COALESCE(team_id,''), COALESCE(assigned_role,''), COALESCE(assigned_to_type,''), COALESCE(assigned_to,''), COALESCE(task_kind,''), goal, status, COALESCE(artifacts_json,'[]'), COALESCE(inputs_json,'{}'), COALESCE(metadata_json,'{}')
		FROM tasks WHERE task_id = ?
	`, batchTaskID).Scan(&sessionID, &runID, &sourceTeamID, &destinationTeamID, &teamID, &assignedRole, &assignedToType, &assignedTo, &taskKind, &goal, &status, &artifactsJSON, &inputsJSON, &metadataJSON)
	if rowErr != nil {
		if errors.Is(rowErr, sql.ErrNoRows) {
			return "", 0, 0, 0, ErrTaskNotFound
		}
		return "", 0, 0, 0, rowErr
	}

	metadata := map[string]any{}
	_ = json.Unmarshal([]byte(metadataJSON), &metadata)
	source := metadataString(metadata, "source")
	if source != "team.batch.callback" && source != "subagent.batch.callback" {
		return "", 0, 0, 0, fmt.Errorf("task %s is not a synthetic batch callback (source=%q)", batchTaskID, source)
	}

	decisions := metadataMap(metadata, "batchItemDecisions")
	if decisions == nil {
		decisions = map[string]any{}
	}
	for _, raw := range decisions {
		switch strings.ToLower(strings.TrimSpace(fmt.Sprint(raw))) {
		case "approve":
			approved++
		case "retry":
			retried++
		case "escalate":
			escalated++
		}
	}

	handoffTaskID = strings.TrimSpace(metadataString(metadata, "batchHandoffTaskId"))
	if handoffTaskID == "" {
		handoffTaskID = "review-handoff-" + batchTaskID
	}
	if metadataBool(metadata["batchClosed"]) {
		if err := tx.Commit(); err != nil {
			return "", 0, 0, 0, err
		}
		return handoffTaskID, approved, retried, escalated, nil
	}

	inputs := map[string]any{}
	_ = json.Unmarshal([]byte(inputsJSON), &inputs)
	items := metadataArray(inputs, "items")
	now := time.Now().UTC()
	nowRaw := now.Format(time.RFC3339Nano)
	decisionNote := strings.TrimSpace(reviewSummary)
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if item == nil {
			continue
		}
		callbackTaskID := strings.TrimSpace(fmt.Sprint(item["callbackTaskId"]))
		if callbackTaskID == "" {
			continue
		}
		decision := strings.ToLower(strings.TrimSpace(fmt.Sprint(item["decision"])))
		if decision == "" {
			decision = strings.ToLower(strings.TrimSpace(fmt.Sprint(decisions[callbackTaskID])))
		}

		var childMetaJSON, childSummary, childStatus string
		if err := tx.QueryRowContext(ctx, `
			SELECT COALESCE(metadata_json,'{}'), COALESCE(summary,''), status
			FROM tasks WHERE task_id = ?
		`, callbackTaskID).Scan(&childMetaJSON, &childSummary, &childStatus); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return "", 0, 0, 0, err
		}
		childMeta := map[string]any{}
		_ = json.Unmarshal([]byte(childMetaJSON), &childMeta)
		childMeta["batchItemStatus"] = batchItemStatusFromDecision(decision)
		childMeta["batchReviewedAt"] = nowRaw
		childMeta["batchReviewedBy"] = reviewerIdentity
		if decisionNote != "" {
			childMeta["batchDecisionNote"] = decisionNote
		}
		newChildMetaJSON, _ := json.Marshal(childMeta)
		reviewSummaryLine := strings.TrimSpace(fmt.Sprintf("Reviewed in batch %s: %s", batchTaskID, decision))
		if reviewSummaryLine == "" {
			reviewSummaryLine = "Reviewed in batch " + batchTaskID
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE tasks
			SET metadata_json = ?, status = ?, summary = CASE WHEN TRIM(COALESCE(summary,'')) = '' THEN ? ELSE summary END,
			    finished_at = COALESCE(finished_at, ?), completed_at = COALESCE(completed_at, ?), updated_at = ?
			WHERE task_id = ?
		`, string(newChildMetaJSON), string(types.TaskStatusSucceeded), reviewSummaryLine, nowRaw, nowRaw, nowRaw, callbackTaskID); err != nil {
			return "", 0, 0, 0, err
		}
	}

	coordinatorRole := strings.TrimSpace(metadataString(metadata, "coordinatorRole"))
	if coordinatorRole == "" {
		coordinatorRole = strings.TrimSpace(assignedRole)
	}
	if strings.TrimSpace(reviewSummary) == "" {
		reviewSummary = fmt.Sprintf("Batch review complete: approved=%d retry=%d escalate=%d.", approved, retried, escalated)
	}
	var artifacts []string
	_ = json.Unmarshal([]byte(artifactsJSON), &artifacts)
	reviewSummaryPath := ""
	if len(artifacts) > 0 {
		reviewSummaryPath = strings.TrimSpace(artifacts[0])
	}
	reviewArtifactPaths := extractReviewArtifactPaths(items)
	if reviewSummaryPath == "" && len(reviewArtifactPaths) > 0 {
		reviewSummaryPath = strings.TrimSpace(reviewArtifactPaths[0])
	}
	handoffMetadata := map[string]any{
		"source":              "review.handoff",
		"batchTaskId":         batchTaskID,
		"batchParentTaskId":   metadataString(metadata, "batchParentTaskId"),
		"batchWaveId":         metadataString(metadata, "batchWaveId"),
		"reviewerRole":        reviewerIdentity,
		"reviewSummaryPath":   reviewSummaryPath,
		"reviewArtifactPaths": reviewArtifactPaths,
		"approvedCount":       approved,
		"retryCount":          retried,
		"escalateCount":       escalated,
	}
	handoffMetadataJSON, _ := json.Marshal(handoffMetadata)
	handoffInputs := map[string]any{
		"batchTaskId":         batchTaskID,
		"batchParentTaskId":   metadataString(metadata, "batchParentTaskId"),
		"batchWaveId":         metadataString(metadata, "batchWaveId"),
		"reviewSummary":       strings.TrimSpace(reviewSummary),
		"reviewSummaryPath":   reviewSummaryPath,
		"reviewArtifactPaths": reviewArtifactPaths,
		"approvedCount":       approved,
		"retryCount":          retried,
		"escalateCount":       escalated,
	}
	handoffInputsJSON, _ := json.Marshal(handoffInputs)
	handoffGoal := "REVIEW HANDOFF: Batch review completed. Review report is ready; coordinator can resume orchestration and next-step delegation."
	if reviewSummaryPath != "" {
		handoffGoal += " Review summary: " + reviewSummaryPath + "."
	}

	handoffAssignedType := "agent"
	handoffAssignedTo := strings.TrimSpace(runID)
	handoffAssignedRole := ""
	if strings.TrimSpace(destinationTeamID) != "" {
		handoffAssignedType = "role"
		handoffAssignedRole = coordinatorRole
		handoffAssignedTo = coordinatorRole
		if coordinatorRunID, coordinatorSessionID := resolveCoordinatorRunForTeamTx(ctx, tx, strings.TrimSpace(destinationTeamID), coordinatorRole); coordinatorRunID != "" {
			runID = coordinatorRunID
			if strings.TrimSpace(coordinatorSessionID) != "" {
				sessionID = coordinatorSessionID
			}
		}
	}
	_, err = tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO tasks (
			task_id, session_id, run_id, source_team_id, destination_team_id, team_id, assigned_role, assigned_to_type, assigned_to, claimed_by, role_snapshot, task_kind, created_by,
			goal, inputs_json, priority, status, created_at, updated_at, metadata_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, handoffTaskID, strings.TrimSpace(sessionID), strings.TrimSpace(runID), strings.TrimSpace(sourceTeamID), strings.TrimSpace(destinationTeamID), strings.TrimSpace(destinationTeamID), strings.TrimSpace(handoffAssignedRole), handoffAssignedType, strings.TrimSpace(handoffAssignedTo), "", "", normalizeTaskKind("callback"), reviewerIdentity,
		strings.TrimSpace(handoffGoal), string(handoffInputsJSON), 1, string(types.TaskStatusPending), nowRaw, nowRaw, string(handoffMetadataJSON))
	if err != nil {
		return "", 0, 0, 0, err
	}

	metadata["batchClosed"] = true
	metadata["batchClosedAt"] = nowRaw
	metadata["batchClosedBy"] = reviewerIdentity
	metadata["batchCloseTxnId"] = uuid.NewString()
	metadata["batchHandoffTaskId"] = handoffTaskID
	metadata["batchReviewedCount"] = float64(approved + retried + escalated)
	metadata["batchReviewComplete"] = true
	newBatchMetaJSON, _ := json.Marshal(metadata)
	if _, err := tx.ExecContext(ctx, `
		UPDATE tasks
		SET metadata_json = ?, status = ?, summary = ?, finished_at = COALESCE(finished_at, ?), completed_at = COALESCE(completed_at, ?), updated_at = ?
		WHERE task_id = ?
	`, string(newBatchMetaJSON), string(types.TaskStatusSucceeded), strings.TrimSpace(reviewSummary), nowRaw, nowRaw, nowRaw, batchTaskID); err != nil {
		return "", 0, 0, 0, err
	}

	if err := tx.Commit(); err != nil {
		return "", 0, 0, 0, err
	}
	return handoffTaskID, approved, retried, escalated, nil
}

func extractReviewArtifactPaths(items []any) []string {
	if len(items) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, 8)
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if item == nil {
			continue
		}
		arts, _ := item["artifacts"].([]any)
		for _, av := range arts {
			p := strings.TrimSpace(fmt.Sprint(av))
			if p == "" {
				continue
			}
			if _, ok := seen[p]; ok {
				continue
			}
			seen[p] = struct{}{}
			out = append(out, p)
		}
	}
	return out
}

func resolveCoordinatorRunForTeamTx(ctx context.Context, tx *sql.Tx, teamID, coordinatorRole string) (runID, sessionID string) {
	teamID = strings.TrimSpace(teamID)
	coordinatorRole = strings.TrimSpace(coordinatorRole)
	if teamID == "" || coordinatorRole == "" {
		return "", ""
	}
	var resolvedRunID, resolvedSessionID string
	err := tx.QueryRowContext(ctx, `
		SELECT run_id, COALESCE(session_id, '')
		FROM runs
		WHERE COALESCE(json_extract(run_json, '$.runtime.teamId'), '') = ?
		  AND LOWER(COALESCE(json_extract(run_json, '$.runtime.role'), '')) = LOWER(?)
		  AND COALESCE(parent_run_id, '') = ''
		ORDER BY created_at ASC
		LIMIT 1
	`, teamID, coordinatorRole).Scan(&resolvedRunID, &resolvedSessionID)
	if err != nil {
		return "", ""
	}
	return strings.TrimSpace(resolvedRunID), strings.TrimSpace(resolvedSessionID)
}

func parseTime(raw string) time.Time {
	return timeutil.ParseRFC3339Nano(raw)
}

func (s *SQLiteTaskStore) CreateTask(ctx context.Context, task types.Task) error {
	if err := validate.NonEmpty("taskId", strings.TrimSpace(task.TaskID)); err != nil {
		return err
	}
	if err := validate.NonEmpty("sessionId", strings.TrimSpace(task.SessionID)); err != nil {
		return err
	}
	if err := validate.NonEmpty("runId", strings.TrimSpace(task.RunID)); err != nil {
		return err
	}
	if err := validate.NonEmpty("goal", strings.TrimSpace(task.Goal)); err != nil {
		return err
	}
	db, err := s.dbConn()
	if err != nil {
		return err
	}

	if task.Inputs == nil {
		task.Inputs = map[string]any{}
	}
	if task.Metadata == nil {
		task.Metadata = map[string]any{}
	}
	task.NormalizeTeamFields()
	task.AssignedToType = strings.TrimSpace(task.AssignedToType)
	task.AssignedTo = strings.TrimSpace(task.AssignedTo)
	if task.AssignedToType == "" {
		if strings.TrimSpace(task.DestinationTeamID) != "" {
			if strings.TrimSpace(task.AssignedRole) != "" {
				task.AssignedToType = "role"
				task.AssignedTo = strings.TrimSpace(task.AssignedRole)
			} else {
				task.AssignedToType = "team"
				task.AssignedTo = strings.TrimSpace(task.DestinationTeamID)
			}
		} else {
			task.AssignedToType = "agent"
			task.AssignedTo = strings.TrimSpace(task.RunID)
		}
	}
	if task.AssignedTo == "" {
		switch task.AssignedToType {
		case "team":
			task.AssignedTo = strings.TrimSpace(task.DestinationTeamID)
		case "role":
			task.AssignedTo = strings.TrimSpace(task.AssignedRole)
		case "agent":
			task.AssignedTo = strings.TrimSpace(task.RunID)
		}
	}

	inputsJSON, _ := json.Marshal(task.Inputs)
	metadataJSON, _ := json.Marshal(task.Metadata)

	now := time.Now().UTC()
	createdAt := now
	if task.CreatedAt != nil && !task.CreatedAt.IsZero() {
		createdAt = task.CreatedAt.UTC()
	}
	status := strings.TrimSpace(string(task.Status))
	if status == "" {
		status = string(types.TaskStatusPending)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO tasks (
			task_id, session_id, run_id, source_team_id, destination_team_id, team_id, assigned_role, assigned_to_type, assigned_to, claimed_by, role_snapshot, task_kind, created_by,
			goal, inputs_json, priority,
			status, created_at, updated_at, metadata_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, strings.TrimSpace(task.TaskID), strings.TrimSpace(task.SessionID), strings.TrimSpace(task.RunID),
		strings.TrimSpace(task.SourceTeamID), strings.TrimSpace(task.DestinationTeamID), strings.TrimSpace(task.TeamID), strings.TrimSpace(task.AssignedRole), strings.TrimSpace(task.AssignedToType), strings.TrimSpace(task.AssignedTo), strings.TrimSpace(task.ClaimedByAgentID), strings.TrimSpace(task.RoleSnapshot), normalizeTaskKind(task.TaskKind), strings.TrimSpace(task.CreatedBy),
		strings.TrimSpace(task.Goal), string(inputsJSON), task.Priority,
		status, createdAt.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), string(metadataJSON))
	if err != nil {
		return err
	}
	return nil
}

func (s *SQLiteTaskStore) DeleteTask(ctx context.Context, taskID string) error {
	db, err := s.dbConn()
	if err != nil {
		return err
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("taskID is required")
	}
	_, err = db.ExecContext(ctx, `DELETE FROM tasks WHERE task_id = ?`, taskID)
	return err
}

func (s *SQLiteTaskStore) GetTask(ctx context.Context, taskID string) (types.Task, error) {
	db, err := s.dbConn()
	if err != nil {
		return types.Task{}, err
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return types.Task{}, fmt.Errorf("taskID is required")
	}

	var t types.Task
	var status string
	var inputsJSON string
	var artifactsJSON string
	var metadataJSON string
	var createdRaw string
	var startedRaw string
	var finishedRaw string
	var updatedRaw string
	var leaseRaw string
	err = db.QueryRowContext(ctx, `
		SELECT
			task_id, session_id, run_id, COALESCE(source_team_id, ''), COALESCE(destination_team_id, ''), COALESCE(team_id, ''), COALESCE(assigned_role, ''), COALESCE(assigned_to_type, ''), COALESCE(assigned_to, ''), COALESCE(claimed_by, ''), COALESCE(role_snapshot, ''), COALESCE(task_kind, ''), COALESCE(created_by, ''), goal,
			COALESCE(inputs_json, '{}'), priority, status,
			created_at, COALESCE(started_at, ''), COALESCE(finished_at, ''),
			COALESCE(summary, ''), COALESCE(artifacts_json, '[]'),
			COALESCE(error, ''), attempts, COALESCE(lease_until, ''),
			updated_at,
			input_tokens, output_tokens, total_tokens, cost_usd,
			duration_seconds,
			COALESCE(metadata_json, '{}')
		FROM tasks
		WHERE task_id = ?
	`, taskID).Scan(
		&t.TaskID, &t.SessionID, &t.RunID, &t.SourceTeamID, &t.DestinationTeamID, &t.TeamID, &t.AssignedRole, &t.AssignedToType, &t.AssignedTo, &t.ClaimedByAgentID, &t.RoleSnapshot, &t.TaskKind, &t.CreatedBy, &t.Goal,
		&inputsJSON, &t.Priority, &status,
		&createdRaw, &startedRaw, &finishedRaw,
		&t.Summary, &artifactsJSON,
		&t.Error, &t.Attempts, &leaseRaw,
		&updatedRaw,
		&t.InputTokens, &t.OutputTokens, &t.TotalTokens, &t.CostUSD,
		&t.DurationSeconds,
		&metadataJSON,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return types.Task{}, ErrTaskNotFound
		}
		return types.Task{}, err
	}

	t.Status = types.TaskStatus(strings.TrimSpace(status))
	_ = json.Unmarshal([]byte(inputsJSON), &t.Inputs)
	_ = json.Unmarshal([]byte(artifactsJSON), &t.Artifacts)
	_ = json.Unmarshal([]byte(metadataJSON), &t.Metadata)
	t.NormalizeTeamFields()

	if tt := parseTime(createdRaw); !tt.IsZero() {
		t.CreatedAt = &tt
	}
	if tt := parseTime(startedRaw); !tt.IsZero() {
		t.StartedAt = &tt
	}
	if tt := parseTime(finishedRaw); !tt.IsZero() {
		t.CompletedAt = &tt
	}
	if tt := parseTime(updatedRaw); !tt.IsZero() {
		t.UpdatedAt = &tt
	}
	if tt := parseTime(leaseRaw); !tt.IsZero() {
		t.LeaseUntil = &tt
	}

	return t, nil
}

func allowedSortColumn(sortBy string) (string, bool) {
	switch strings.TrimSpace(sortBy) {
	case "", "created_at":
		return "created_at", true
	case "completed_at", "finished_at":
		return "finished_at", true
	case "cost_usd":
		return "cost_usd", true
	case "priority":
		return "priority", true
	case "updated_at":
		return "updated_at", true
	case "started_at":
		return "started_at", true
	case "status":
		return "status", true
	default:
		return "", false
	}
}

func (s *SQLiteTaskStore) GetRunStats(ctx context.Context, runID string) (RunStats, error) {
	db, err := s.dbConn()
	if err != nil {
		return RunStats{}, err
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return RunStats{}, fmt.Errorf("runID is required")
	}

	var total int
	var succeeded int
	var failed int
	var cost float64
	var tokens int
	var durationSeconds int64

	if err := db.QueryRowContext(ctx, `
		SELECT
			COUNT(*) as total,
			SUM(CASE WHEN status = 'succeeded' THEN 1 ELSE 0 END) as succeeded,
			SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) as failed,
			COALESCE(SUM(cost_usd), 0.0) as cost,
			COALESCE(SUM(total_tokens), 0) as tokens,
			COALESCE(SUM(duration_seconds), 0) as duration
		FROM tasks
		WHERE run_id = ?
	`, runID).Scan(&total, &succeeded, &failed, &cost, &tokens, &durationSeconds); err != nil {
		return RunStats{}, err
	}

	return RunStats{
		TotalTasks:    total,
		Succeeded:     succeeded,
		Failed:        failed,
		TotalCost:     cost,
		TotalTokens:   tokens,
		TotalDuration: time.Duration(durationSeconds) * time.Second,
	}, nil
}

func (s *SQLiteTaskStore) ListTasks(ctx context.Context, filter TaskFilter) ([]types.Task, error) {
	db, err := s.dbConn()
	if err != nil {
		return nil, err
	}
	if filter.Limit < 0 || filter.Offset < 0 {
		return nil, ErrInvalidFilter
	}
	sortBy, ok := allowedSortColumn(filter.SortBy)
	if !ok {
		return nil, ErrInvalidFilter
	}
	sortDir := "ASC"
	if filter.SortDesc {
		sortDir = "DESC"
	}

	q := `
		SELECT
			task_id, session_id, run_id, COALESCE(source_team_id, ''), COALESCE(destination_team_id, ''), COALESCE(team_id, ''), COALESCE(assigned_role, ''), COALESCE(assigned_to_type, ''), COALESCE(assigned_to, ''), COALESCE(claimed_by, ''), COALESCE(role_snapshot, ''), COALESCE(task_kind, ''), COALESCE(created_by, ''), goal,
			priority, status, created_at,
			COALESCE(started_at, ''), COALESCE(finished_at, ''),
			COALESCE(summary, ''), COALESCE(error, ''),
			attempts, COALESCE(lease_until, ''),
			input_tokens, output_tokens, total_tokens, cost_usd,
			duration_seconds, updated_at, COALESCE(metadata_json, '{}')
		FROM tasks
		WHERE 1=1
	`
	args := []any{}

	if strings.TrimSpace(filter.SessionID) != "" {
		q += " AND session_id = ?"
		args = append(args, strings.TrimSpace(filter.SessionID))
	}
	if strings.TrimSpace(filter.RunID) != "" {
		q += " AND run_id = ?"
		args = append(args, strings.TrimSpace(filter.RunID))
	}
	if strings.TrimSpace(filter.SourceTeamID) != "" {
		q += " AND source_team_id = ?"
		args = append(args, strings.TrimSpace(filter.SourceTeamID))
	}
	destinationTeamID := strings.TrimSpace(filter.DestinationTeamID)
	if destinationTeamID == "" {
		destinationTeamID = strings.TrimSpace(filter.TeamID)
	}
	if destinationTeamID != "" {
		q += " AND destination_team_id = ?"
		args = append(args, destinationTeamID)
	}
	if strings.TrimSpace(filter.AssignedRole) != "" {
		q += " AND assigned_role = ?"
		args = append(args, strings.TrimSpace(filter.AssignedRole))
	}
	if strings.TrimSpace(filter.AssignedToType) != "" {
		q += " AND assigned_to_type = ?"
		args = append(args, strings.TrimSpace(filter.AssignedToType))
	}
	if strings.TrimSpace(filter.AssignedTo) != "" {
		q += " AND assigned_to = ?"
		args = append(args, strings.TrimSpace(filter.AssignedTo))
	}
	if strings.TrimSpace(filter.ClaimedBy) != "" {
		q += " AND claimed_by = ?"
		args = append(args, strings.TrimSpace(filter.ClaimedBy))
	}
	if strings.TrimSpace(filter.TaskKind) != "" {
		q += " AND task_kind = ?"
		args = append(args, normalizeTaskKind(filter.TaskKind))
	}
	if filter.UnassignedOnly {
		q += " AND COALESCE(assigned_role, '') = ''"
	}
	switch strings.ToLower(strings.TrimSpace(filter.View)) {
	case "inbox":
		if len(filter.Status) == 0 {
			q += " AND status IN (?, ?, ?)"
			args = append(args, string(types.TaskStatusPending), string(types.TaskStatusActive), string(types.TaskStatusReviewPending))
		}
		q += " AND COALESCE(json_extract(metadata_json, '$.source'), '') NOT IN ('team.callback', 'subagent.callback')"
	case "outbox":
		if len(filter.Status) == 0 {
			q += " AND status IN (?, ?, ?)"
			args = append(args, string(types.TaskStatusSucceeded), string(types.TaskStatusFailed), string(types.TaskStatusCanceled))
		}
	}
	if len(filter.Status) != 0 {
		ph := make([]string, 0, len(filter.Status))
		for _, st := range filter.Status {
			ph = append(ph, "?")
			args = append(args, strings.TrimSpace(string(st)))
		}
		q += " AND status IN (" + strings.Join(ph, ",") + ")"
	}
	if filter.FromDate != nil && !filter.FromDate.IsZero() {
		q += " AND created_at >= ?"
		args = append(args, filter.FromDate.UTC().Format(time.RFC3339Nano))
	}
	if filter.ToDate != nil && !filter.ToDate.IsZero() {
		q += " AND created_at <= ?"
		args = append(args, filter.ToDate.UTC().Format(time.RFC3339Nano))
	}

	orderBy := fmt.Sprintf("%s %s", sortBy, sortDir)
	if sortBy == "priority" {
		orderBy = fmt.Sprintf("priority %s, created_at ASC", sortDir)
	}
	q += " ORDER BY " + orderBy

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	q += " LIMIT ?"
	args = append(args, limit)
	if filter.Offset > 0 {
		q += " OFFSET ?"
		args = append(args, filter.Offset)
	}

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []types.Task{}
	for rows.Next() {
		var t types.Task
		var status string
		var createdRaw string
		var startedRaw string
		var finishedRaw string
		var leaseRaw string
		var updatedRaw string
		var metadataJSON string
		if err := rows.Scan(
			&t.TaskID, &t.SessionID, &t.RunID, &t.SourceTeamID, &t.DestinationTeamID, &t.TeamID, &t.AssignedRole, &t.AssignedToType, &t.AssignedTo, &t.ClaimedByAgentID, &t.RoleSnapshot, &t.TaskKind, &t.CreatedBy, &t.Goal,
			&t.Priority, &status, &createdRaw,
			&startedRaw, &finishedRaw,
			&t.Summary, &t.Error,
			&t.Attempts, &leaseRaw,
			&t.InputTokens, &t.OutputTokens, &t.TotalTokens, &t.CostUSD,
			&t.DurationSeconds, &updatedRaw, &metadataJSON,
		); err != nil {
			return nil, err
		}
		t.Status = types.TaskStatus(strings.TrimSpace(status))
		if tt := parseTime(createdRaw); !tt.IsZero() {
			t.CreatedAt = &tt
		}
		if tt := parseTime(startedRaw); !tt.IsZero() {
			t.StartedAt = &tt
		}
		if tt := parseTime(finishedRaw); !tt.IsZero() {
			t.CompletedAt = &tt
		}
		if tt := parseTime(updatedRaw); !tt.IsZero() {
			t.UpdatedAt = &tt
		}
		_ = json.Unmarshal([]byte(metadataJSON), &t.Metadata)
		t.NormalizeTeamFields()
		if tt := parseTime(leaseRaw); !tt.IsZero() {
			t.LeaseUntil = &tt
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *SQLiteTaskStore) CountTasks(ctx context.Context, filter TaskFilter) (int, error) {
	db, err := s.dbConn()
	if err != nil {
		return 0, err
	}
	if filter.Offset < 0 || filter.Limit < 0 {
		return 0, ErrInvalidFilter
	}
	q := `SELECT COUNT(*) FROM tasks WHERE 1=1`
	args := []any{}
	if strings.TrimSpace(filter.SessionID) != "" {
		q += " AND session_id = ?"
		args = append(args, strings.TrimSpace(filter.SessionID))
	}
	if strings.TrimSpace(filter.RunID) != "" {
		q += " AND run_id = ?"
		args = append(args, strings.TrimSpace(filter.RunID))
	}
	if strings.TrimSpace(filter.SourceTeamID) != "" {
		q += " AND source_team_id = ?"
		args = append(args, strings.TrimSpace(filter.SourceTeamID))
	}
	destinationTeamID := strings.TrimSpace(filter.DestinationTeamID)
	if destinationTeamID == "" {
		destinationTeamID = strings.TrimSpace(filter.TeamID)
	}
	if destinationTeamID != "" {
		q += " AND destination_team_id = ?"
		args = append(args, destinationTeamID)
	}
	if strings.TrimSpace(filter.AssignedRole) != "" {
		q += " AND assigned_role = ?"
		args = append(args, strings.TrimSpace(filter.AssignedRole))
	}
	if strings.TrimSpace(filter.AssignedToType) != "" {
		q += " AND assigned_to_type = ?"
		args = append(args, strings.TrimSpace(filter.AssignedToType))
	}
	if strings.TrimSpace(filter.AssignedTo) != "" {
		q += " AND assigned_to = ?"
		args = append(args, strings.TrimSpace(filter.AssignedTo))
	}
	if strings.TrimSpace(filter.ClaimedBy) != "" {
		q += " AND claimed_by = ?"
		args = append(args, strings.TrimSpace(filter.ClaimedBy))
	}
	if strings.TrimSpace(filter.TaskKind) != "" {
		q += " AND task_kind = ?"
		args = append(args, normalizeTaskKind(filter.TaskKind))
	}
	if filter.UnassignedOnly {
		q += " AND COALESCE(assigned_role, '') = ''"
	}
	switch strings.ToLower(strings.TrimSpace(filter.View)) {
	case "inbox":
		if len(filter.Status) == 0 {
			q += " AND status IN (?, ?, ?)"
			args = append(args, string(types.TaskStatusPending), string(types.TaskStatusActive), string(types.TaskStatusReviewPending))
		}
		q += " AND COALESCE(json_extract(metadata_json, '$.source'), '') NOT IN ('team.callback', 'subagent.callback')"
	case "outbox":
		if len(filter.Status) == 0 {
			q += " AND status IN (?, ?, ?)"
			args = append(args, string(types.TaskStatusSucceeded), string(types.TaskStatusFailed), string(types.TaskStatusCanceled))
		}
	}
	if len(filter.Status) != 0 {
		ph := make([]string, 0, len(filter.Status))
		for _, st := range filter.Status {
			ph = append(ph, "?")
			args = append(args, strings.TrimSpace(string(st)))
		}
		q += " AND status IN (" + strings.Join(ph, ",") + ")"
	}
	if filter.FromDate != nil && !filter.FromDate.IsZero() {
		q += " AND created_at >= ?"
		args = append(args, filter.FromDate.UTC().Format(time.RFC3339Nano))
	}
	if filter.ToDate != nil && !filter.ToDate.IsZero() {
		q += " AND created_at <= ?"
		args = append(args, filter.ToDate.UTC().Format(time.RFC3339Nano))
	}
	var n int
	if err := db.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func (s *SQLiteTaskStore) UpdateTask(ctx context.Context, task types.Task) error {
	taskID := strings.TrimSpace(task.TaskID)
	if taskID == "" {
		return fmt.Errorf("taskID is required")
	}
	if strings.TrimSpace(task.SessionID) == "" || strings.TrimSpace(task.RunID) == "" {
		return fmt.Errorf("sessionId and runId are required")
	}
	db, err := s.dbConn()
	if err != nil {
		return err
	}
	if task.Inputs == nil {
		task.Inputs = map[string]any{}
	}
	if task.Metadata == nil {
		task.Metadata = map[string]any{}
	}
	if task.Artifacts == nil {
		task.Artifacts = []string{}
	}
	task.NormalizeTeamFields()
	inputsJSON, _ := json.Marshal(task.Inputs)
	metadataJSON, _ := json.Marshal(task.Metadata)
	artifactsJSON, _ := json.Marshal(task.Artifacts)

	startedAt := ""
	if timeutil.IsSet(task.StartedAt) {
		startedAt = timeutil.FormatRFC3339Nano(task.StartedAt)
	}
	finishedAt := ""
	if timeutil.IsSet(task.CompletedAt) {
		finishedAt = timeutil.FormatRFC3339Nano(task.CompletedAt)
	}
	leaseUntil := ""
	if task.LeaseUntil != nil && !task.LeaseUntil.IsZero() {
		leaseUntil = task.LeaseUntil.UTC().Format(time.RFC3339Nano)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = db.ExecContext(ctx, `
		UPDATE tasks
		SET session_id = ?, run_id = ?, source_team_id = ?, destination_team_id = ?, team_id = ?, assigned_role = ?, assigned_to_type = ?, assigned_to = ?, claimed_by = ?, role_snapshot = ?, task_kind = ?, created_by = ?,
		    goal = ?, inputs_json = ?, priority = ?,
		    status = ?, started_at = ?, finished_at = ?, completed_at = ?, summary = ?, artifacts_json = ?,
		    error = ?, attempts = ?, lease_until = ?, updated_at = ?,
		    input_tokens = ?, output_tokens = ?, total_tokens = ?, cost_usd = ?, duration_seconds = ?,
		    metadata_json = ?
		WHERE task_id = ?
	`, strings.TrimSpace(task.SessionID), strings.TrimSpace(task.RunID), strings.TrimSpace(task.SourceTeamID), strings.TrimSpace(task.DestinationTeamID), strings.TrimSpace(task.TeamID), strings.TrimSpace(task.AssignedRole), strings.TrimSpace(task.AssignedToType), strings.TrimSpace(task.AssignedTo), strings.TrimSpace(task.ClaimedByAgentID), strings.TrimSpace(task.RoleSnapshot), normalizeTaskKind(task.TaskKind), strings.TrimSpace(task.CreatedBy),
		strings.TrimSpace(task.Goal), string(inputsJSON), task.Priority,
		strings.TrimSpace(string(task.Status)),
		nullIfEmpty(startedAt),
		nullIfEmpty(finishedAt),
		nullIfEmpty(finishedAt),
		strings.TrimSpace(task.Summary),
		string(artifactsJSON),
		strings.TrimSpace(task.Error),
		task.Attempts,
		nullIfEmpty(leaseUntil),
		now,
		task.InputTokens, task.OutputTokens, task.TotalTokens, task.CostUSD, task.DurationSeconds,
		string(metadataJSON),
		taskID,
	)
	return err
}

func nullIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func isTerminalStatus(status types.TaskStatus) bool {
	switch status {
	case types.TaskStatusSucceeded, types.TaskStatusFailed, types.TaskStatusCanceled:
		return true
	default:
		return false
	}
}

func (s *SQLiteTaskStore) ClaimTask(ctx context.Context, taskID string, ttl time.Duration) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("taskID is required")
	}
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	db, err := s.dbConn()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	leaseUntil := now.Add(ttl)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var statusRaw string
	var leaseRaw string
	var attempts int
	row := tx.QueryRowContext(ctx, `
		SELECT status, COALESCE(lease_until, ''), attempts
		FROM tasks
		WHERE task_id = ?
	`, taskID)
	if err := row.Scan(&statusRaw, &leaseRaw, &attempts); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrTaskNotFound
		}
		return err
	}

	status := types.TaskStatus(strings.TrimSpace(statusRaw))
	if isTerminalStatus(status) {
		return ErrTaskTerminal
	}
	lease := parseTime(leaseRaw)
	if status == types.TaskStatusActive && !lease.IsZero() && lease.After(now) {
		return ErrTaskClaimed
	}

	attempts++
	_, err = tx.ExecContext(ctx, `
		UPDATE tasks
		SET status = ?, attempts = ?, lease_until = ?, updated_at = ?,
		    started_at = COALESCE(started_at, ?)
		WHERE task_id = ?
	`, string(types.TaskStatusActive), attempts, leaseUntil.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano),
		now.Format(time.RFC3339Nano), taskID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteTaskStore) ExtendLease(ctx context.Context, taskID string, ttl time.Duration) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("taskID is required")
	}
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	db, err := s.dbConn()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	leaseUntil := now.Add(ttl)
	_, err = db.ExecContext(ctx, `
		UPDATE tasks
		SET lease_until = ?, updated_at = ?
		WHERE task_id = ? AND status = ?
	`, leaseUntil.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), taskID, string(types.TaskStatusActive))
	return err
}

func (s *SQLiteTaskStore) ReleaseLease(ctx context.Context, taskID string) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("taskID is required")
	}
	db, err := s.dbConn()
	if err != nil {
		return err
	}
	// Extra safety: do not release lease on terminal tasks (invariant: terminal never → pending).
	var statusRaw string
	if err := db.QueryRowContext(ctx, `SELECT status FROM tasks WHERE task_id = ?`, taskID).Scan(&statusRaw); err == nil {
		if isTerminalStatus(types.TaskStatus(strings.TrimSpace(statusRaw))) {
			return nil // Idempotent: already terminal.
		}
	}
	// Fall through: update only active (WHERE status = ? already enforces non-terminal).
	now := time.Now().UTC()
	res, err := db.ExecContext(ctx, `
		UPDATE tasks
		SET status = ?, lease_until = NULL, claimed_by = '', updated_at = ?
		WHERE task_id = ? AND status = ?
	`, string(types.TaskStatusPending), now.Format(time.RFC3339Nano), taskID, string(types.TaskStatusActive))
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil // Task not active or not found; idempotent
	}
	return nil
}

func (s *SQLiteTaskStore) DelegateTask(ctx context.Context, taskID string) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("taskID is required")
	}
	db, err := s.dbConn()
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := db.ExecContext(ctx, `
		UPDATE tasks
		SET status = ?, lease_until = NULL, updated_at = ?
		WHERE task_id = ? AND status = ?
	`, string(types.TaskStatusDelegated), now, taskID, string(types.TaskStatusActive))
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("delegate: task %s is not in active state", taskID)
	}
	return nil
}

func (s *SQLiteTaskStore) ResumeTask(ctx context.Context, taskID string) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("taskID is required")
	}
	db, err := s.dbConn()
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := db.ExecContext(ctx, `
		UPDATE tasks
		SET status = ?, lease_until = NULL, claimed_by = '', updated_at = ?
		WHERE task_id = ? AND status = ?
	`, string(types.TaskStatusPending), now, taskID, string(types.TaskStatusDelegated))
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		// Idempotent: task already resumed or not in delegated state; treat as success.
		return nil
	}
	return nil
}

func (s *SQLiteTaskStore) CompleteTask(ctx context.Context, taskID string, result types.TaskResult) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("taskID is required")
	}
	db, err := s.dbConn()
	if err != nil {
		return err
	}
	status := types.TaskStatus(strings.TrimSpace(string(result.Status)))
	if !isTerminalStatus(status) {
		// Fall back to failed if provided status isn't terminal.
		status = types.TaskStatusFailed
	}
	now := time.Now().UTC()
	finishedAt := now
	if timeutil.IsSet(result.CompletedAt) {
		finishedAt = result.CompletedAt.UTC()
	}
	artifactsJSON, _ := json.Marshal(result.Artifacts)

	total := result.TotalTokens
	if total == 0 {
		total = result.InputTokens + result.OutputTokens
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Only transition from non-terminal states; never overwrite succeeded/failed/canceled.
	res, err := tx.ExecContext(ctx, `
		UPDATE tasks
		SET status = ?, finished_at = ?, completed_at = ?, summary = ?, artifacts_json = ?, error = ?,
		    input_tokens = ?, output_tokens = ?, total_tokens = ?, cost_usd = ?,
		    lease_until = NULL, updated_at = ?
		WHERE task_id = ? AND status IN (?, ?, ?)
`, string(status), finishedAt.Format(time.RFC3339Nano), finishedAt.Format(time.RFC3339Nano),
		strings.TrimSpace(result.Summary), string(artifactsJSON), strings.TrimSpace(result.Error),
		result.InputTokens, result.OutputTokens, total, result.CostUSD,
		now.Format(time.RFC3339Nano), taskID,
		string(types.TaskStatusActive), string(types.TaskStatusPending), string(types.TaskStatusDelegated))
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		// Task already terminal or not found; idempotent success.
		return nil
	}

	var task types.Task
	var finishedRaw string
	var metadataRaw string
	if err := tx.QueryRowContext(ctx, `
		SELECT task_id, COALESCE(source_team_id, ''), COALESCE(destination_team_id, ''), COALESCE(team_id, ''), COALESCE(run_id, ''), COALESCE(assigned_role, ''),
		       COALESCE(role_snapshot, ''), COALESCE(task_kind, ''), COALESCE(created_by, ''), COALESCE(goal, ''), COALESCE(status, ''), COALESCE(finished_at, ''),
		       COALESCE(metadata_json, '{}')
		FROM tasks
		WHERE task_id = ?
	`, taskID).Scan(
		&task.TaskID, &task.SourceTeamID, &task.DestinationTeamID, &task.TeamID, &task.RunID, &task.AssignedRole, &task.RoleSnapshot, &task.TaskKind, &task.CreatedBy, &task.Goal, &task.Status, &finishedRaw, &metadataRaw,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrTaskNotFound
		}
		return err
	}
	if strings.TrimSpace(metadataRaw) != "" {
		_ = json.Unmarshal([]byte(metadataRaw), &task.Metadata)
	}
	if tt := parseTime(finishedRaw); !tt.IsZero() {
		task.CompletedAt = &tt
	}
	task.NormalizeTeamFields()
	if err := s.upsertArtifactsTx(ctx, tx, task, result); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteTaskStore) RecoverExpiredLeases(ctx context.Context) error {
	db, err := s.dbConn()
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = db.ExecContext(ctx, `
		UPDATE tasks
		SET status = ?,
		    error = CASE
		      WHEN COALESCE(TRIM(error), '') = '' THEN 'lease expired'
		      ELSE TRIM(error) || '; lease expired'
		    END,
		    lease_until = NULL,
		    updated_at = ?
		WHERE status = ?
		  AND COALESCE(TRIM(lease_until), '') != ''
		  AND lease_until < ?
	`, string(types.TaskStatusFailed), now, string(types.TaskStatusActive), now)
	return err
}

// CancelActiveTasksByRun marks all currently-active tasks for a run as canceled.
// This is used when a run is paused/stopped so in-flight tasks do not remain active.
func (s *SQLiteTaskStore) CancelActiveTasksByRun(ctx context.Context, runID string, reason string) (int, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return 0, fmt.Errorf("runID is required")
	}
	db, err := s.dbConn()
	if err != nil {
		return 0, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "run paused"
	}
	res, err := db.ExecContext(ctx, `
		UPDATE tasks
		SET status = ?,
		    finished_at = ?,
		    completed_at = ?,
		    error = CASE
		      WHEN COALESCE(TRIM(error), '') = '' THEN ?
		      ELSE TRIM(error) || '; ' || ?
		    END,
		    lease_until = NULL,
		    updated_at = ?
		WHERE run_id = ?
		  AND status = ?
	`, string(types.TaskStatusCanceled), now, now, reason, reason, now, runID, string(types.TaskStatusActive))
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, nil
	}
	return int(n), nil
}

func allowedMessageSortColumn(sortBy string) (string, bool) {
	switch strings.TrimSpace(sortBy) {
	case "", "created_at":
		return "created_at", true
	case "visible_at":
		return "visible_at", true
	case "processed_at":
		return "processed_at", true
	case "priority":
		return "priority", true
	case "updated_at":
		return "updated_at", true
	default:
		return "", false
	}
}

func normalizeMessage(msg types.AgentMessage) types.AgentMessage {
	now := time.Now().UTC()
	out := msg
	out.MessageID = strings.TrimSpace(out.MessageID)
	if out.MessageID == "" {
		out.MessageID = "msg-" + uuid.NewString()
	}
	out.IntentID = strings.TrimSpace(out.IntentID)
	out.CorrelationID = strings.TrimSpace(out.CorrelationID)
	out.CausationID = strings.TrimSpace(out.CausationID)
	out.Producer = strings.TrimSpace(out.Producer)
	out.ThreadID = strings.TrimSpace(out.ThreadID)
	out.RunID = strings.TrimSpace(out.RunID)
	out.NormalizeTeamFields()
	out.Channel = strings.TrimSpace(out.Channel)
	out.Kind = strings.TrimSpace(out.Kind)
	out.TaskRef = strings.TrimSpace(out.TaskRef)
	out.Status = strings.TrimSpace(out.Status)
	if out.Status == "" {
		out.Status = types.MessageStatusPending
	}
	if out.VisibleAt.IsZero() {
		out.VisibleAt = now
	} else {
		out.VisibleAt = out.VisibleAt.UTC()
	}
	if out.Body == nil {
		out.Body = map[string]any{}
	}
	if out.Metadata == nil {
		out.Metadata = map[string]any{}
	}
	return out
}

func mapSQLiteMessage(rowScanner interface {
	Scan(dest ...any) error
}) (types.AgentMessage, error) {
	var (
		msg         types.AgentMessage
		bodyJSON    string
		taskJSON    string
		metaJSON    string
		leaseUntil  string
		visibleAt   string
		createdAt   string
		updatedAt   string
		processedAt string
	)
	if err := rowScanner.Scan(
		&msg.MessageID,
		&msg.IntentID,
		&msg.CorrelationID,
		&msg.CausationID,
		&msg.Producer,
		&msg.ThreadID,
		&msg.RunID,
		&msg.SourceTeamID,
		&msg.DestinationTeamID,
		&msg.TeamID,
		&msg.Channel,
		&msg.Kind,
		&bodyJSON,
		&msg.TaskRef,
		&taskJSON,
		&msg.Status,
		&msg.LeaseOwner,
		&leaseUntil,
		&msg.Attempts,
		&visibleAt,
		&msg.Priority,
		&msg.Error,
		&metaJSON,
		&createdAt,
		&updatedAt,
		&processedAt,
	); err != nil {
		return types.AgentMessage{}, err
	}
	if strings.TrimSpace(bodyJSON) != "" {
		_ = json.Unmarshal([]byte(bodyJSON), &msg.Body)
	}
	if strings.TrimSpace(taskJSON) != "" {
		var task types.Task
		if err := json.Unmarshal([]byte(taskJSON), &task); err == nil {
			task.NormalizeTeamFields()
			msg.Task = &task
		}
	}
	if strings.TrimSpace(metaJSON) != "" {
		_ = json.Unmarshal([]byte(metaJSON), &msg.Metadata)
	}
	if tt := parseTime(leaseUntil); !tt.IsZero() {
		msg.LeaseUntil = &tt
	}
	msg.VisibleAt = parseTime(visibleAt)
	if tt := parseTime(createdAt); !tt.IsZero() {
		msg.CreatedAt = &tt
	}
	if tt := parseTime(updatedAt); !tt.IsZero() {
		msg.UpdatedAt = &tt
	}
	if tt := parseTime(processedAt); !tt.IsZero() {
		msg.ProcessedAt = &tt
	}
	msg.NormalizeTeamFields()
	return msg, nil
}

func (s *SQLiteTaskStore) PublishMessage(ctx context.Context, msg types.AgentMessage) (types.AgentMessage, error) {
	msg = normalizeMessage(msg)
	if msg.IntentID == "" {
		return types.AgentMessage{}, fmt.Errorf("intentID is required")
	}
	if msg.CorrelationID == "" {
		return types.AgentMessage{}, fmt.Errorf("correlationID is required")
	}
	if msg.ThreadID == "" {
		return types.AgentMessage{}, fmt.Errorf("threadID is required")
	}
	if msg.Channel == "" {
		return types.AgentMessage{}, fmt.Errorf("channel is required")
	}
	if msg.Kind == "" {
		return types.AgentMessage{}, fmt.Errorf("kind is required")
	}
	if msg.Kind == types.MessageKindTask && msg.TaskRef == "" {
		return types.AgentMessage{}, fmt.Errorf("taskRef is required for task messages")
	}
	return withSQLiteBusyRetry(ctx, func() (types.AgentMessage, error) {
		db, err := s.dbConn()
		if err != nil {
			return types.AgentMessage{}, err
		}
		now := time.Now().UTC()
		bodyJSON, _ := json.Marshal(msg.Body)
		metaJSON, _ := json.Marshal(msg.Metadata)
		taskJSON := ""
		if msg.Task != nil {
			b, _ := json.Marshal(msg.Task)
			taskJSON = string(b)
		}

		_, err = db.ExecContext(ctx, `
			INSERT INTO messages (
				message_id, intent_id, correlation_id, causation_id, producer,
				thread_id, run_id, source_team_id, destination_team_id, team_id, channel, kind, body_json, task_ref, task_json,
				status, lease_owner, lease_until, attempts, visible_at, priority, error,
				metadata_json, created_at, updated_at, processed_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, msg.MessageID, msg.IntentID, msg.CorrelationID, msg.CausationID, msg.Producer,
			msg.ThreadID, msg.RunID, msg.SourceTeamID, msg.DestinationTeamID, msg.TeamID, msg.Channel, msg.Kind, string(bodyJSON), msg.TaskRef, taskJSON,
			msg.Status, strings.TrimSpace(msg.LeaseOwner), nullIfEmpty(timeutil.FormatRFC3339Nano(msg.LeaseUntil)), msg.Attempts,
			msg.VisibleAt.Format(time.RFC3339Nano), msg.Priority, msg.Error, string(metaJSON),
			now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), nullIfEmpty(timeutil.FormatRFC3339Nano(msg.ProcessedAt)))
		if err != nil {
			if !strings.Contains(strings.ToLower(err.Error()), "unique") {
				return types.AgentMessage{}, err
			}
			existing, gerr := s.getMessageByThreadIntent(ctx, msg.ThreadID, msg.IntentID)
			if gerr != nil {
				return types.AgentMessage{}, err
			}
			return existing, nil
		}
		return s.GetMessage(ctx, msg.MessageID)
	})
}

func (s *SQLiteTaskStore) getMessageByThreadIntent(ctx context.Context, threadID, intentID string) (types.AgentMessage, error) {
	db, err := s.dbConn()
	if err != nil {
		return types.AgentMessage{}, err
	}
	row := db.QueryRowContext(ctx, `
		SELECT message_id, intent_id, correlation_id, COALESCE(causation_id, ''), COALESCE(producer, ''),
		       thread_id, COALESCE(run_id, ''), COALESCE(source_team_id, ''), COALESCE(destination_team_id, ''), COALESCE(team_id, ''), channel, kind,
		       COALESCE(body_json, '{}'), COALESCE(task_ref, ''), COALESCE(task_json, ''),
		       status, COALESCE(lease_owner, ''), COALESCE(lease_until, ''), attempts,
		       visible_at, priority, COALESCE(error, ''), COALESCE(metadata_json, '{}'),
		       created_at, updated_at, COALESCE(processed_at, '')
		FROM messages
		WHERE thread_id = ? AND intent_id = ?
	`, strings.TrimSpace(threadID), strings.TrimSpace(intentID))
	msg, err := mapSQLiteMessage(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return types.AgentMessage{}, ErrMessageNotFound
		}
		return types.AgentMessage{}, err
	}
	return msg, nil
}

func (s *SQLiteTaskStore) GetMessage(ctx context.Context, messageID string) (types.AgentMessage, error) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return types.AgentMessage{}, fmt.Errorf("messageID is required")
	}
	db, err := s.dbConn()
	if err != nil {
		return types.AgentMessage{}, err
	}
	row := db.QueryRowContext(ctx, `
		SELECT message_id, intent_id, correlation_id, COALESCE(causation_id, ''), COALESCE(producer, ''),
		       thread_id, COALESCE(run_id, ''), COALESCE(source_team_id, ''), COALESCE(destination_team_id, ''), COALESCE(team_id, ''), channel, kind,
		       COALESCE(body_json, '{}'), COALESCE(task_ref, ''), COALESCE(task_json, ''),
		       status, COALESCE(lease_owner, ''), COALESCE(lease_until, ''), attempts,
		       visible_at, priority, COALESCE(error, ''), COALESCE(metadata_json, '{}'),
		       created_at, updated_at, COALESCE(processed_at, '')
		FROM messages
		WHERE message_id = ?
	`, messageID)
	msg, err := mapSQLiteMessage(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return types.AgentMessage{}, ErrMessageNotFound
		}
		return types.AgentMessage{}, err
	}
	return msg, nil
}

func (s *SQLiteTaskStore) ListMessages(ctx context.Context, filter MessageFilter) ([]types.AgentMessage, error) {
	db, err := s.dbConn()
	if err != nil {
		return nil, err
	}
	if filter.Limit < 0 || filter.Offset < 0 {
		return nil, ErrInvalidMsgFilter
	}
	sortBy, ok := allowedMessageSortColumn(filter.SortBy)
	if !ok {
		return nil, ErrInvalidMsgFilter
	}
	sortDir := "ASC"
	if filter.SortDesc {
		sortDir = "DESC"
	}
	q := `
		SELECT message_id, intent_id, correlation_id, COALESCE(causation_id, ''), COALESCE(producer, ''),
		       thread_id, COALESCE(run_id, ''), COALESCE(source_team_id, ''), COALESCE(destination_team_id, ''), COALESCE(team_id, ''), channel, kind,
		       COALESCE(body_json, '{}'), COALESCE(task_ref, ''), COALESCE(task_json, ''),
		       status, COALESCE(lease_owner, ''), COALESCE(lease_until, ''), attempts,
		       visible_at, priority, COALESCE(error, ''), COALESCE(metadata_json, '{}'),
		       created_at, updated_at, COALESCE(processed_at, '')
		FROM messages
		WHERE 1=1
	`
	args := []any{}
	if id := strings.TrimSpace(filter.ThreadID); id != "" {
		q += " AND thread_id = ?"
		args = append(args, id)
	}
	if id := strings.TrimSpace(filter.RunID); id != "" {
		q += " AND run_id = ?"
		args = append(args, id)
	}
	if id := strings.TrimSpace(filter.SourceTeamID); id != "" {
		q += " AND source_team_id = ?"
		args = append(args, id)
	}
	destinationTeamID := strings.TrimSpace(filter.DestinationTeamID)
	if destinationTeamID == "" {
		destinationTeamID = strings.TrimSpace(filter.TeamID)
	}
	if destinationTeamID != "" {
		q += " AND destination_team_id = ?"
		args = append(args, destinationTeamID)
	}
	if id := strings.TrimSpace(filter.TaskRef); id != "" {
		q += " AND task_ref = ?"
		args = append(args, id)
	}
	if ch := strings.TrimSpace(filter.Channel); ch != "" {
		q += " AND channel = ?"
		args = append(args, ch)
	}
	if len(filter.Kinds) > 0 {
		ph := make([]string, 0, len(filter.Kinds))
		for _, kind := range filter.Kinds {
			kind = strings.TrimSpace(kind)
			if kind == "" {
				continue
			}
			ph = append(ph, "?")
			args = append(args, kind)
		}
		if len(ph) > 0 {
			q += " AND kind IN (" + strings.Join(ph, ",") + ")"
		}
	}
	if len(filter.Statuses) > 0 {
		ph := make([]string, 0, len(filter.Statuses))
		for _, st := range filter.Statuses {
			st = strings.TrimSpace(st)
			if st == "" {
				continue
			}
			ph = append(ph, "?")
			args = append(args, st)
		}
		if len(ph) > 0 {
			q += " AND status IN (" + strings.Join(ph, ",") + ")"
		}
	}
	q += " ORDER BY " + sortBy + " " + sortDir
	if filter.Limit > 0 {
		q += " LIMIT ?"
		args = append(args, filter.Limit)
	}
	if filter.Offset > 0 {
		if filter.Limit <= 0 {
			q += " LIMIT -1"
		}
		q += " OFFSET ?"
		args = append(args, filter.Offset)
	}
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]types.AgentMessage, 0)
	for rows.Next() {
		msg, err := mapSQLiteMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *SQLiteTaskStore) CountMessages(ctx context.Context, filter MessageFilter) (int, error) {
	db, err := s.dbConn()
	if err != nil {
		return 0, err
	}
	if filter.Limit < 0 || filter.Offset < 0 {
		return 0, ErrInvalidMsgFilter
	}
	q := `SELECT COUNT(*) FROM messages WHERE 1=1`
	args := []any{}
	if id := strings.TrimSpace(filter.ThreadID); id != "" {
		q += " AND thread_id = ?"
		args = append(args, id)
	}
	if id := strings.TrimSpace(filter.RunID); id != "" {
		q += " AND run_id = ?"
		args = append(args, id)
	}
	if id := strings.TrimSpace(filter.SourceTeamID); id != "" {
		q += " AND source_team_id = ?"
		args = append(args, id)
	}
	destinationTeamID := strings.TrimSpace(filter.DestinationTeamID)
	if destinationTeamID == "" {
		destinationTeamID = strings.TrimSpace(filter.TeamID)
	}
	if destinationTeamID != "" {
		q += " AND destination_team_id = ?"
		args = append(args, destinationTeamID)
	}
	if id := strings.TrimSpace(filter.TaskRef); id != "" {
		q += " AND task_ref = ?"
		args = append(args, id)
	}
	if ch := strings.TrimSpace(filter.Channel); ch != "" {
		q += " AND channel = ?"
		args = append(args, ch)
	}
	if len(filter.Kinds) > 0 {
		ph := make([]string, 0, len(filter.Kinds))
		for _, kind := range filter.Kinds {
			kind = strings.TrimSpace(kind)
			if kind == "" {
				continue
			}
			ph = append(ph, "?")
			args = append(args, kind)
		}
		if len(ph) > 0 {
			q += " AND kind IN (" + strings.Join(ph, ",") + ")"
		}
	}
	if len(filter.Statuses) > 0 {
		ph := make([]string, 0, len(filter.Statuses))
		for _, st := range filter.Statuses {
			st = strings.TrimSpace(st)
			if st == "" {
				continue
			}
			ph = append(ph, "?")
			args = append(args, st)
		}
		if len(ph) > 0 {
			q += " AND status IN (" + strings.Join(ph, ",") + ")"
		}
	}
	var count int
	if err := db.QueryRowContext(ctx, q, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func isTerminalMessageStatus(status string) bool {
	status = strings.TrimSpace(strings.ToLower(status))
	switch status {
	case types.MessageStatusAcked, types.MessageStatusDeadletter:
		return true
	default:
		return false
	}
}

func (s *SQLiteTaskStore) ClaimNextMessage(ctx context.Context, filter MessageClaimFilter, ttl time.Duration, consumerID string) (types.AgentMessage, error) {
	consumerID = strings.TrimSpace(consumerID)
	if consumerID == "" {
		return types.AgentMessage{}, fmt.Errorf("consumerID is required")
	}
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	return withSQLiteBusyRetry(ctx, func() (types.AgentMessage, error) {
		db, err := s.dbConn()
		if err != nil {
			return types.AgentMessage{}, err
		}
		now := time.Now().UTC()
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return types.AgentMessage{}, err
		}
		defer tx.Rollback()

		where := " WHERE status = ? AND visible_at <= ?"
		args := []any{types.MessageStatusPending, now.Format(time.RFC3339Nano)}
		if id := strings.TrimSpace(filter.ThreadID); id != "" {
			where += " AND thread_id = ?"
			args = append(args, id)
		}
		if id := strings.TrimSpace(filter.RunID); id != "" {
			where += " AND run_id = ?"
			args = append(args, id)
		}
		if id := strings.TrimSpace(filter.SourceTeamID); id != "" {
			where += " AND source_team_id = ?"
			args = append(args, id)
		}
		destinationTeamID := strings.TrimSpace(filter.DestinationTeamID)
		if destinationTeamID == "" {
			destinationTeamID = strings.TrimSpace(filter.TeamID)
		}
		if destinationTeamID != "" {
			where += " AND destination_team_id = ?"
			args = append(args, destinationTeamID)
		}
		if id := strings.TrimSpace(filter.TaskRef); id != "" {
			where += " AND task_ref = ?"
			args = append(args, id)
		}
		if assignedToType := strings.TrimSpace(filter.AssignedToType); assignedToType != "" {
			where += " AND EXISTS (SELECT 1 FROM tasks t WHERE t.task_id = messages.task_ref AND t.assigned_to_type = ?"
			args = append(args, assignedToType)
			if assignedTo := strings.TrimSpace(filter.AssignedTo); assignedTo != "" {
				where += " AND t.assigned_to = ?"
				args = append(args, assignedTo)
			}
			where += ")"
		}
		if ch := strings.TrimSpace(filter.Channel); ch != "" {
			where += " AND channel = ?"
			args = append(args, ch)
		}
		if len(filter.Kinds) > 0 {
			ph := make([]string, 0, len(filter.Kinds))
			for _, kind := range filter.Kinds {
				kind = strings.TrimSpace(kind)
				if kind == "" {
					continue
				}
				ph = append(ph, "?")
				args = append(args, kind)
			}
			if len(ph) > 0 {
				where += " AND kind IN (" + strings.Join(ph, ",") + ")"
			}
		}

		var messageID string
		if err := tx.QueryRowContext(ctx, `
			SELECT message_id
			FROM messages
		`+where+`
			ORDER BY priority ASC, created_at ASC
			LIMIT 1
		`, args...).Scan(&messageID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return types.AgentMessage{}, ErrMessageNotFound
			}
			return types.AgentMessage{}, err
		}

		leaseUntil := now.Add(ttl)
		res, err := tx.ExecContext(ctx, `
			UPDATE messages
			SET status = ?, lease_owner = ?, lease_until = ?, attempts = attempts + 1, updated_at = ?
			WHERE message_id = ? AND status = ?
		`, types.MessageStatusClaimed, consumerID, leaseUntil.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), messageID, types.MessageStatusPending)
		if err != nil {
			return types.AgentMessage{}, err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return types.AgentMessage{}, ErrMessageClaimed
		}

		row := tx.QueryRowContext(ctx, `
			SELECT message_id, intent_id, correlation_id, COALESCE(causation_id, ''), COALESCE(producer, ''),
			       thread_id, COALESCE(run_id, ''), COALESCE(source_team_id, ''), COALESCE(destination_team_id, ''), COALESCE(team_id, ''), channel, kind,
			       COALESCE(body_json, '{}'), COALESCE(task_ref, ''), COALESCE(task_json, ''),
			       status, COALESCE(lease_owner, ''), COALESCE(lease_until, ''), attempts,
			       visible_at, priority, COALESCE(error, ''), COALESCE(metadata_json, '{}'),
			       created_at, updated_at, COALESCE(processed_at, '')
			FROM messages
			WHERE message_id = ?
		`, messageID)
		msg, err := mapSQLiteMessage(row)
		if err != nil {
			return types.AgentMessage{}, err
		}
		if err := tx.Commit(); err != nil {
			return types.AgentMessage{}, err
		}
		return msg, nil
	})
}

func (s *SQLiteTaskStore) AckMessage(ctx context.Context, messageID string, result MessageAckResult) error {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return fmt.Errorf("messageID is required")
	}
	return withSQLiteBusyRetryErr(ctx, func() error {
		db, err := s.dbConn()
		if err != nil {
			return err
		}
		msg, err := s.GetMessage(ctx, messageID)
		if err != nil {
			return err
		}
		if isTerminalMessageStatus(msg.Status) {
			return nil
		}
		status := strings.TrimSpace(result.Status)
		if status == "" {
			status = types.MessageStatusAcked
		}
		processed := time.Now().UTC()
		if result.Processed != nil && !result.Processed.IsZero() {
			processed = result.Processed.UTC()
		}
		metadata := msg.Metadata
		if result.Metadata != nil {
			metadata = result.Metadata
		}
		metadataJSON, _ := json.Marshal(metadata)
		_, err = db.ExecContext(ctx, `
			UPDATE messages
			SET status = ?, error = ?, metadata_json = ?, lease_owner = '', lease_until = NULL, processed_at = ?, updated_at = ?
			WHERE message_id = ?
		`, status, strings.TrimSpace(result.Error), string(metadataJSON), processed.Format(time.RFC3339Nano), processed.Format(time.RFC3339Nano), messageID)
		return err
	})
}

func (s *SQLiteTaskStore) NackMessage(ctx context.Context, messageID string, reason string, retryAt *time.Time) error {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return fmt.Errorf("messageID is required")
	}
	return withSQLiteBusyRetryErr(ctx, func() error {
		db, err := s.dbConn()
		if err != nil {
			return err
		}
		msg, err := s.GetMessage(ctx, messageID)
		if err != nil {
			return err
		}
		if isTerminalMessageStatus(msg.Status) {
			return nil
		}
		const maxAttempts = 5
		now := time.Now().UTC()
		reason = strings.TrimSpace(reason)
		if reason == "" {
			reason = "message processing failed"
		}
		nextStatus := types.MessageStatusDeadletter
		nextVisible := now
		processedAt := now
		if retryAt != nil && !retryAt.IsZero() && msg.Attempts < maxAttempts {
			nextStatus = types.MessageStatusPending
			nextVisible = retryAt.UTC()
			processedAt = time.Time{}
		}
		_, err = db.ExecContext(ctx, `
			UPDATE messages
			SET status = ?, error = ?, lease_owner = '', lease_until = NULL, visible_at = ?, processed_at = ?, updated_at = ?
			WHERE message_id = ?
		`, nextStatus, reason, nextVisible.Format(time.RFC3339Nano), nullIfEmpty(processedAt.Format(time.RFC3339Nano)), now.Format(time.RFC3339Nano), messageID)
		return err
	})
}

func (s *SQLiteTaskStore) RequeueExpiredClaims(ctx context.Context) error {
	return withSQLiteBusyRetryErr(ctx, func() error {
		db, err := s.dbConn()
		if err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		_, err = db.ExecContext(ctx, `
			UPDATE messages
			SET status = ?, lease_owner = '', lease_until = NULL, updated_at = ?
			WHERE status = ?
			  AND COALESCE(TRIM(lease_until), '') != ''
			  AND lease_until < ?
		`, types.MessageStatusPending, now, types.MessageStatusClaimed, now)
		return err
	})
}
