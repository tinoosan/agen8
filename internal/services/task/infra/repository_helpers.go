package infra

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8/internal/services/task/domain"
)

func taskWhere(filter domain.TaskFilter) (string, []any, error) {
	if len(filter.MetadataFilter) > 0 {
		return "", nil, fmt.Errorf("%w: metadata filter is not supported", domain.ErrInvalidFilter)
	}
	var clauses []string
	var args []any
	if strings.TrimSpace(string(filter.ProjectID)) != "" {
		clauses = append(clauses, "project_id = ?")
		args = append(args, strings.TrimSpace(string(filter.ProjectID)))
	}
	if strings.TrimSpace(string(filter.AssignedTo)) != "" {
		clauses = append(clauses, "assigned_to = ?")
		args = append(args, strings.TrimSpace(string(filter.AssignedTo)))
	}
	if strings.TrimSpace(string(filter.ClaimedBy)) != "" {
		clauses = append(clauses, "claimed_by_member_id = ?")
		args = append(args, strings.TrimSpace(string(filter.ClaimedBy)))
	}
	if strings.TrimSpace(filter.TaskKind) != "" {
		clauses = append(clauses, "task_kind = ?")
		args = append(args, strings.TrimSpace(filter.TaskKind))
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
		args = append(args, taskTimeString(*filter.FromDate))
	}
	if filter.ToDate != nil {
		clauses = append(clauses, "created_at <= ?")
		args = append(args, taskTimeString(*filter.ToDate))
	}
	if len(clauses) == 0 {
		return "", args, nil
	}
	return " WHERE " + strings.Join(clauses, " AND "), args, nil
}

func taskOrderColumn(filter domain.TaskFilter) string {
	switch strings.TrimSpace(filter.SortBy) {
	case "", "created_at", "createdAt":
		return "created_at"
	case "updated_at", "updatedAt":
		return "updated_at"
	case "completed_at", "completedAt":
		return "completed_at"
	case "status":
		return "status"
	default:
		return "created_at"
	}
}

func taskOrderBy(filter domain.TaskFilter) string {
	column := taskOrderColumn(filter)
	direction := " ASC"
	if taskOrderDescending(filter) {
		direction = " DESC"
	}
	if column == "status" {
		return " ORDER BY status" + direction + ", task_id ASC"
	}
	return " ORDER BY " + column + direction + ", task_id ASC"
}

func taskPagination(query string, args []any, filter domain.TaskFilter) (string, []any) {
	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
		if filter.Offset > 0 {
			query += " OFFSET ?"
			args = append(args, filter.Offset)
		}
		return query, args
	}
	if filter.Offset > 0 {
		query += " LIMIT -1 OFFSET ?"
		args = append(args, filter.Offset)
	}
	return query, args
}

func taskResultCapacity(filter domain.TaskFilter) int {
	if filter.Limit > 0 {
		return filter.Limit
	}
	return 0
}

func taskOrderDescending(filter domain.TaskFilter) bool {
	return filter.SortDesc || strings.TrimSpace(filter.SortBy) == ""
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
	return taskTimeString(*t)
}

const taskTimestampLayout = "2006-01-02T15:04:05.000000000Z"

func taskTimeString(value time.Time) string {
	return value.UTC().Format(taskTimestampLayout)
}

func isDuplicateColumnError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate column") ||
		strings.Contains(msg, "already exists")
}
