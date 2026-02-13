package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/timeutil"
	"github.com/tinoosan/workbench-core/pkg/types"
)

func taskKindFromTask(taskID, createdBy string) string {
	taskID = strings.ToLower(strings.TrimSpace(taskID))
	createdBy = strings.ToLower(strings.TrimSpace(createdBy))
	switch {
	case strings.HasPrefix(taskID, "callback-"), strings.HasPrefix(taskID, "callback-task-"):
		return TaskKindCallback
	case strings.HasPrefix(taskID, "heartbeat-"):
		return TaskKindHeartbeat
	case createdBy == "user" || createdBy == "webhook" || createdBy == "monitor":
		return TaskKindCoordinator
	case strings.HasPrefix(taskID, "task-"):
		return TaskKindTask
	default:
		return TaskKindOther
	}
}

func normalizeTaskKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case TaskKindTask:
		return TaskKindTask
	case TaskKindCallback:
		return TaskKindCallback
	case TaskKindHeartbeat:
		return TaskKindHeartbeat
	case TaskKindCoordinator:
		return TaskKindCoordinator
	case TaskKindOther:
		return TaskKindOther
	default:
		return TaskKindOther
	}
}

func normalizeRole(role string) string {
	role = strings.TrimSpace(role)
	if role == "" {
		return "unassigned"
	}
	return role
}

func dataDirFromSQLitePath(dbPath string) string {
	p := strings.TrimSpace(dbPath)
	if p == "" {
		return ""
	}
	return filepath.Dir(p)
}

func resolveIndexedDiskPath(dataDir, teamID, runID, vpath string) string {
	vpath = strings.TrimSpace(vpath)
	if strings.HasPrefix(vpath, "/workspace/") {
		rel := strings.TrimPrefix(vpath, "/workspace/")
		if strings.TrimSpace(teamID) != "" {
			return filepath.Join(fsutil.GetTeamWorkspaceDir(dataDir, teamID), rel)
		}
		return filepath.Join(fsutil.GetWorkspaceDir(dataDir, runID), rel)
	}
	if strings.HasPrefix(vpath, "/") {
		return vpath
	}
	return ""
}

func standaloneIndexedArtifactVPath(vpath string) bool {
	vpath = strings.TrimSpace(vpath)
	if !strings.HasPrefix(vpath, "/workspace/") {
		return false
	}
	rel := strings.TrimSpace(strings.TrimPrefix(vpath, "/workspace/"))
	if rel == "" {
		return false
	}
	if strings.HasPrefix(rel, "tasks/") || strings.HasPrefix(rel, "deliverables/") {
		base := strings.TrimSpace(filepath.Base(rel))
		if strings.EqualFold(base, "SUMMARY.md") {
			return false
		}
	}
	return true
}

func (s *SQLiteTaskStore) upsertArtifactsTx(ctx context.Context, tx *sql.Tx, task types.Task, result types.TaskResult) error {
	taskKind := normalizeTaskKind(task.TaskKind)
	if taskKind == TaskKindOther {
		taskKind = taskKindFromTask(task.TaskID, task.CreatedBy)
	}
	role := normalizeRole(task.RoleSnapshot)
	if role == "unassigned" {
		role = normalizeRole(task.AssignedRole)
	}

	finishedAt := time.Now().UTC()
	if timeutil.IsSet(result.CompletedAt) {
		finishedAt = result.CompletedAt.UTC()
	} else if timeutil.IsSet(task.CompletedAt) {
		finishedAt = task.CompletedAt.UTC()
	}
	producedAt := finishedAt.Format(time.RFC3339Nano)
	dayBucket := finishedAt.Format("2006-01-02")
	teamID := strings.TrimSpace(task.TeamID)
	runID := strings.TrimSpace(task.RunID)
	dataDir := dataDirFromSQLitePath(s.path)

	if _, err := tx.ExecContext(ctx, `
		UPDATE tasks
		SET task_kind = ?, role_snapshot = ?, updated_at = ?
		WHERE task_id = ?
	`, taskKind, role, time.Now().UTC().Format(time.RFC3339Nano), strings.TrimSpace(task.TaskID)); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM artifacts WHERE task_id = ?`, strings.TrimSpace(task.TaskID)); err != nil {
		return err
	}

	seen := map[string]struct{}{}
	for _, raw := range result.Artifacts {
		vpath := strings.TrimSpace(raw)
		if vpath == "" {
			continue
		}
		if teamID == "" && !standaloneIndexedArtifactVPath(vpath) {
			continue
		}
		key := strings.ToLower(vpath)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		displayName := filepath.Base(vpath)
		if displayName == "" || displayName == "." || displayName == "/" {
			displayName = vpath
		}
		isSummary := strings.EqualFold(displayName, "SUMMARY.md")
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO artifacts (
				task_id, team_id, run_id, role, task_kind, is_summary,
				display_name, vpath, disk_path, produced_at, day_bucket
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, strings.TrimSpace(task.TaskID), teamID, runID, role, taskKind, boolToInt(isSummary),
			displayName, vpath, resolveIndexedDiskPath(dataDir, teamID, runID, vpath), producedAt, dayBucket); err != nil {
			return err
		}
	}
	return nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func (s *SQLiteTaskStore) UpsertTaskClassification(ctx context.Context, taskID, taskKind, roleSnapshot string) error {
	db, err := s.dbConn()
	if err != nil {
		return err
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("taskID is required")
	}
	taskKind = normalizeTaskKind(taskKind)
	roleSnapshot = normalizeRole(roleSnapshot)
	_, err = db.ExecContext(ctx, `
		UPDATE tasks
		SET task_kind = ?, role_snapshot = ?, updated_at = ?
		WHERE task_id = ?
	`, taskKind, roleSnapshot, time.Now().UTC().Format(time.RFC3339Nano), taskID)
	return err
}

func (s *SQLiteTaskStore) ReplaceTaskArtifacts(ctx context.Context, taskID string, records []ArtifactRecord) error {
	db, err := s.dbConn()
	if err != nil {
		return err
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("taskID is required")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM artifacts WHERE task_id = ?`, taskID); err != nil {
		return err
	}
	for _, rec := range records {
		if strings.TrimSpace(rec.VPath) == "" {
			continue
		}
		if strings.TrimSpace(rec.TeamID) == "" && !standaloneIndexedArtifactVPath(rec.VPath) {
			continue
		}
		producedAt := time.Now().UTC()
		if !rec.ProducedAt.IsZero() {
			producedAt = rec.ProducedAt.UTC()
		}
		dayBucket := strings.TrimSpace(rec.DayBucket)
		if dayBucket == "" {
			dayBucket = producedAt.Format("2006-01-02")
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO artifacts (
				task_id, team_id, run_id, role, task_kind, is_summary,
				display_name, vpath, disk_path, produced_at, day_bucket
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, taskID, strings.TrimSpace(rec.TeamID), strings.TrimSpace(rec.RunID), normalizeRole(rec.Role),
			normalizeTaskKind(rec.TaskKind), boolToInt(rec.IsSummary), strings.TrimSpace(rec.DisplayName),
			strings.TrimSpace(rec.VPath), strings.TrimSpace(rec.DiskPath), producedAt.Format(time.RFC3339Nano), dayBucket); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteTaskStore) ListArtifactsByTask(ctx context.Context, filter ArtifactFilter) ([]ArtifactRecord, error) {
	db, err := s.dbConn()
	if err != nil {
		return nil, err
	}
	taskID := strings.TrimSpace(filter.TaskID)
	if taskID == "" {
		return nil, fmt.Errorf("taskID is required")
	}
	rows, err := db.QueryContext(ctx, `
		SELECT
			artifact_id, task_id, COALESCE(team_id, ''), COALESCE(run_id, ''),
			COALESCE(role, ''), COALESCE(task_kind, ''),
			is_summary, COALESCE(display_name, ''), COALESCE(vpath, ''), COALESCE(disk_path, ''),
			COALESCE(produced_at, ''), COALESCE(day_bucket, '')
		FROM artifacts
		WHERE task_id = ?
		ORDER BY is_summary DESC, produced_at DESC, display_name ASC
	`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ArtifactRecord, 0, 8)
	for rows.Next() {
		var rec ArtifactRecord
		var isSummary int
		var producedRaw string
		if err := rows.Scan(
			&rec.ArtifactID, &rec.TaskID, &rec.TeamID, &rec.RunID,
			&rec.Role, &rec.TaskKind, &isSummary, &rec.DisplayName, &rec.VPath, &rec.DiskPath,
			&producedRaw, &rec.DayBucket,
		); err != nil {
			return nil, err
		}
		rec.IsSummary = isSummary == 1
		rec.ProducedAt = parseTime(producedRaw)
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *SQLiteTaskStore) ListArtifactGroups(ctx context.Context, filter ArtifactFilter) ([]ArtifactGroup, error) {
	db, err := s.dbConn()
	if err != nil {
		return nil, err
	}
	where := []string{"1=1"}
	args := make([]any, 0, 8)
	if teamID := strings.TrimSpace(filter.TeamID); teamID != "" {
		where = append(where, "a.team_id = ?")
		args = append(args, teamID)
	} else if runID := strings.TrimSpace(filter.RunID); runID != "" {
		where = append(where, "a.run_id = ?")
		args = append(args, runID)
	}
	if day := strings.TrimSpace(filter.DayBucket); day != "" {
		where = append(where, "a.day_bucket = ?")
		args = append(args, day)
	}
	if role := strings.TrimSpace(filter.Role); role != "" {
		where = append(where, "a.role = ?")
		args = append(args, role)
	}
	if kind := strings.TrimSpace(filter.TaskKind); kind != "" {
		where = append(where, "a.task_kind = ?")
		args = append(args, kind)
	}
	if taskID := strings.TrimSpace(filter.TaskID); taskID != "" {
		where = append(where, "a.task_id = ?")
		args = append(args, taskID)
	}

	q := `
		SELECT
			a.task_id, COALESCE(a.day_bucket, ''), COALESCE(a.role, ''), COALESCE(a.task_kind, ''),
			COALESCE(t.goal, ''), COALESCE(t.status, ''), COALESCE(t.finished_at, ''),
			a.artifact_id, a.is_summary, COALESCE(a.display_name, ''), COALESCE(a.vpath, ''), COALESCE(a.disk_path, ''), COALESCE(a.produced_at, ''), COALESCE(a.team_id, ''), COALESCE(a.run_id, '')
		FROM artifacts a
		JOIN tasks t ON t.task_id = a.task_id
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY a.day_bucket DESC, a.role ASC,
			CASE a.task_kind
				WHEN 'callback' THEN 0
				WHEN 'heartbeat' THEN 1
				WHEN 'task' THEN 2
				WHEN 'coordinator' THEN 3
				ELSE 4
			END ASC,
			COALESCE(t.finished_at, a.produced_at) DESC,
			a.task_id ASC,
			a.is_summary DESC,
			a.display_name ASC
	`
	limit := filter.Limit
	if limit > 0 {
		q += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	groups := make([]ArtifactGroup, 0, 64)
	idxByTask := map[string]int{}
	for rows.Next() {
		var (
			taskID, day, role, kind, goal, status, finishedRaw                   string
			artifactID, displayName, vpath, diskPath, producedRaw, teamID, runID string
			isSummary                                                            int
		)
		if err := rows.Scan(
			&taskID, &day, &role, &kind, &goal, &status, &finishedRaw,
			&artifactID, &isSummary, &displayName, &vpath, &diskPath, &producedRaw, &teamID, &runID,
		); err != nil {
			return nil, err
		}
		idx, ok := idxByTask[taskID]
		if !ok {
			groups = append(groups, ArtifactGroup{
				DayBucket:  day,
				Role:       normalizeRole(role),
				TaskKind:   normalizeTaskKind(kind),
				TaskID:     taskID,
				Goal:       goal,
				Status:     status,
				ProducedAt: parseTime(finishedRaw),
			})
			idx = len(groups) - 1
			idxByTask[taskID] = idx
		}
		groups[idx].Files = append(groups[idx].Files, ArtifactRecord{
			ArtifactID:  artifactID,
			TaskID:      taskID,
			TeamID:      teamID,
			RunID:       runID,
			Role:        normalizeRole(role),
			TaskKind:    normalizeTaskKind(kind),
			IsSummary:   isSummary == 1,
			DisplayName: displayName,
			VPath:       vpath,
			DiskPath:    diskPath,
			ProducedAt:  parseTime(producedRaw),
			DayBucket:   day,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range groups {
		sort.SliceStable(groups[i].Files, func(a, b int) bool {
			if groups[i].Files[a].IsSummary != groups[i].Files[b].IsSummary {
				return groups[i].Files[a].IsSummary
			}
			return groups[i].Files[a].DisplayName < groups[i].Files[b].DisplayName
		})
	}
	return groups, nil
}

func (s *SQLiteTaskStore) SearchArtifacts(ctx context.Context, filter ArtifactSearchFilter) ([]ArtifactRecord, error) {
	db, err := s.dbConn()
	if err != nil {
		return nil, err
	}
	query := strings.TrimSpace(filter.Query)
	if query == "" {
		return nil, nil
	}
	where := []string{"1=1"}
	args := make([]any, 0, 10)
	if teamID := strings.TrimSpace(filter.TeamID); teamID != "" {
		where = append(where, "team_id = ?")
		args = append(args, teamID)
	} else if runID := strings.TrimSpace(filter.RunID); runID != "" {
		where = append(where, "run_id = ?")
		args = append(args, runID)
	}
	if day := strings.TrimSpace(filter.DayBucket); day != "" {
		where = append(where, "day_bucket = ?")
		args = append(args, day)
	}
	if role := strings.TrimSpace(filter.Role); role != "" {
		where = append(where, "role = ?")
		args = append(args, role)
	}
	if kind := strings.TrimSpace(filter.TaskKind); kind != "" {
		where = append(where, "task_kind = ?")
		args = append(args, kind)
	}
	if taskID := strings.TrimSpace(filter.TaskID); taskID != "" {
		where = append(where, "task_id = ?")
		args = append(args, taskID)
	}
	where = append(where, "(LOWER(display_name) LIKE LOWER(?) OR LOWER(vpath) LIKE LOWER(?) OR LOWER(task_id) LIKE LOWER(?) OR LOWER(role) LIKE LOWER(?))")
	pat := "%" + query + "%"
	args = append(args, pat, pat, pat, pat)

	q := `
		SELECT
			artifact_id, task_id, COALESCE(team_id, ''), COALESCE(run_id, ''),
			COALESCE(role, ''), COALESCE(task_kind, ''), is_summary,
			COALESCE(display_name, ''), COALESCE(vpath, ''), COALESCE(disk_path, ''), COALESCE(produced_at, ''), COALESCE(day_bucket, '')
		FROM artifacts
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY day_bucket DESC, produced_at DESC, task_id ASC, is_summary DESC
	`
	limit := filter.Limit
	if limit <= 0 {
		limit = 500
	}
	q += " LIMIT ?"
	args = append(args, limit)
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ArtifactRecord, 0, 64)
	for rows.Next() {
		var rec ArtifactRecord
		var isSummary int
		var producedRaw string
		if err := rows.Scan(
			&rec.ArtifactID, &rec.TaskID, &rec.TeamID, &rec.RunID, &rec.Role, &rec.TaskKind,
			&isSummary, &rec.DisplayName, &rec.VPath, &rec.DiskPath, &producedRaw, &rec.DayBucket,
		); err != nil {
			return nil, err
		}
		rec.IsSummary = isSummary == 1
		rec.ProducedAt = parseTime(producedRaw)
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *SQLiteTaskStore) backfillArtifactIndex(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `
		SELECT
			task_id, COALESCE(team_id, ''), COALESCE(run_id, ''), COALESCE(assigned_role, ''), COALESCE(created_by, ''),
			COALESCE(task_kind, ''), COALESCE(role_snapshot, ''), COALESCE(artifacts_json, '[]'), COALESCE(finished_at, ''), status
		FROM tasks
		WHERE COALESCE(artifacts_json, '[]') != '[]'
		  AND status IN ('succeeded', 'failed', 'canceled')
		  AND NOT EXISTS (SELECT 1 FROM artifacts a WHERE a.task_id = tasks.task_id)
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type backfillRow struct {
		taskID       string
		teamID       string
		runID        string
		assignedRole string
		createdBy    string
		taskKind     string
		roleSnapshot string
		artifactsRaw string
		finishedRaw  string
		status       string
	}
	backfill := make([]backfillRow, 0, 64)
	for rows.Next() {
		var r backfillRow
		if err := rows.Scan(
			&r.taskID, &r.teamID, &r.runID, &r.assignedRole, &r.createdBy,
			&r.taskKind, &r.roleSnapshot, &r.artifactsRaw, &r.finishedRaw, &r.status,
		); err != nil {
			return err
		}
		backfill = append(backfill, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(backfill) == 0 {
		return nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	dataDir := dataDirFromSQLitePath(s.path)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, row := range backfill {
		taskKind := normalizeTaskKind(row.taskKind)
		if taskKind == TaskKindOther {
			taskKind = taskKindFromTask(row.taskID, row.createdBy)
		}
		role := normalizeRole(row.roleSnapshot)
		if role == "unassigned" {
			role = normalizeRole(row.assignedRole)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE tasks SET task_kind = ?, role_snapshot = ?, updated_at = ? WHERE task_id = ?
		`, taskKind, role, now, row.taskID); err != nil {
			return err
		}
		var artifacts []string
		_ = json.Unmarshal([]byte(row.artifactsRaw), &artifacts)
		finished := parseTime(row.finishedRaw)
		if finished.IsZero() {
			finished = time.Now().UTC()
		}
		dayBucket := finished.Format("2006-01-02")
		producedAt := finished.Format(time.RFC3339Nano)
		seen := map[string]struct{}{}
		for _, a := range artifacts {
			vpath := strings.TrimSpace(a)
			if vpath == "" {
				continue
			}
			if _, ok := seen[strings.ToLower(vpath)]; ok {
				continue
			}
			seen[strings.ToLower(vpath)] = struct{}{}
			name := filepath.Base(vpath)
			if name == "" || name == "." || name == "/" {
				name = vpath
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO artifacts (
					task_id, team_id, run_id, role, task_kind, is_summary,
					display_name, vpath, disk_path, produced_at, day_bucket
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`, row.taskID, row.teamID, row.runID, role, taskKind, boolToInt(strings.EqualFold(name, "SUMMARY.md")),
				name, vpath, resolveIndexedDiskPath(dataDir, row.teamID, row.runID, vpath), producedAt, dayBucket); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
