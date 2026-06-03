package infra

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/agen8-mcp-server/internal/services/task/domain"
)

func taskWhere(filter domain.TaskFilter) (string, []any, error) {
	if len(filter.MetadataFilter) > 0 {
		return "", nil, fmt.Errorf("%w: metadata filter is not supported", domain.ErrInvalidFilter)
	}
	var clauses []string
	var args []any
	if filter.SpaceID != "" {
		clauses = append(clauses, "space_id = ?")
		args = append(args, strings.TrimSpace(string(filter.SpaceID)))
	}
	if filter.AssignedTo != "" {
		clauses = append(clauses, "assigned_to = ?")
		args = append(args, strings.TrimSpace(string(filter.AssignedTo)))
	}
	if filter.ClaimedBy != "" {
		clauses = append(clauses, "claimed_by_member_id = ?")
		args = append(args, strings.TrimSpace(string(filter.ClaimedBy)))
	}
	if filter.TaskKind != "" {
		clauses = append(clauses, "task_kind = ?")
		args = append(args, strings.TrimSpace(filter.TaskKind))
	}
	if filter.PlanPhaseID != nil {
		clauses = append(clauses, "plan_phase_id = ?")
		args = append(args, filter.PlanPhaseID.String())
	}
	if filter.PlanTodoID != nil {
		clauses = append(clauses, "plan_todo_id = ?")
		args = append(args, filter.PlanTodoID.String())
	}
	if len(filter.Status) > 0 {
		placeholders := make([]string, 0, len(filter.Status))
		for _, status := range filter.Status {
			status = domain.TaskStatus(strings.TrimSpace(string(status)))
			if status == "" {
				continue
			}
			placeholders = append(placeholders, "?")
			args = append(args, string(status))
		}
		if len(placeholders) > 0 {
			clauses = append(clauses, "status IN ("+strings.Join(placeholders, ", ")+")")
		}
	}
	if filter.FromDate != nil {
		clauses = append(clauses, "created_at >= ?")
		args = append(args, filter.FromDate.UTC().Format(time.RFC3339Nano))
	}
	if filter.ToDate != nil {
		clauses = append(clauses, "created_at <= ?")
		args = append(args, filter.ToDate.UTC().Format(time.RFC3339Nano))
	}
	if len(clauses) == 0 {
		return "", args, nil
	}
	return " WHERE " + strings.Join(clauses, " AND "), args, nil
}

func taskOrderBy(filter domain.TaskFilter) string {
	column := "created_at"
	switch strings.TrimSpace(filter.SortBy) {
	case "", "created_at", "createdAt":
		column = "created_at"
	case "updated_at", "updatedAt":
		column = "updated_at"
	case "completed_at", "completedAt":
		column = "completed_at"
	case "status":
		column = "status"
	default:
		column = "created_at"
	}
	direction := "ASC"
	if filter.SortDesc || strings.TrimSpace(filter.SortBy) == "" {
		direction = "DESC"
	}
	return " ORDER BY " + column + " " + direction + ", task_id ASC"
}

func unmarshalTask(raw []byte) (domain.Task, error) {
	if len(raw) == 0 {
		return domain.Task{}, fmt.Errorf("task_json is empty")
	}
	var task domain.Task
	if err := json.Unmarshal(raw, &task); err != nil {
		return domain.Task{}, err
	}
	return task, nil
}

func timeString(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func uuidString(id *uuid.UUID) any {
	if id == nil {
		return nil
	}
	return id.String()
}

func isDuplicateColumnError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate column") ||
		strings.Contains(msg, "already exists")
}
