package infra

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8/internal/core/types"
	"github.com/tinoosan/agen8/internal/services/project/domain/member"
	"github.com/tinoosan/agen8/internal/services/task/domain"
)

func validateTaskFilter(filter domain.TaskFilter) error {
	_, _, err := taskWhere(filter)
	return err
}

func taskWhere(filter domain.TaskFilter) (string, []any, error) {
	if len(filter.MetadataFilter) > 0 {
		return "", nil, fmt.Errorf("%w: metadata filter is not supported", domain.ErrInvalidFilter)
	}
	var clauses []string
	var args []any
	if filter.ProjectID != "" {
		clauses = append(clauses, "project_id = ?")
		args = append(args, strings.TrimSpace(string(filter.ProjectID)))
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

func taskMatchesFilter(task domain.Task, filter domain.TaskFilter) bool {
	if strings.TrimSpace(string(filter.ProjectID)) != "" && task.ProjectID != types.ProjectID(strings.TrimSpace(string(filter.ProjectID))) {
		return false
	}
	if strings.TrimSpace(string(filter.AssignedTo)) != "" && task.AssignedTo != member.ID(strings.TrimSpace(string(filter.AssignedTo))) {
		return false
	}
	if strings.TrimSpace(string(filter.ClaimedBy)) != "" && task.ClaimedByMemberID != member.ID(strings.TrimSpace(string(filter.ClaimedBy))) {
		return false
	}
	if strings.TrimSpace(filter.TaskKind) != "" && task.TaskKind != strings.TrimSpace(filter.TaskKind) {
		return false
	}
	if len(filter.Status) > 0 {
		matchedStatus := false
		for _, status := range filter.Status {
			if status == "" {
				continue
			}
			if task.Status == domain.TaskStatus(strings.TrimSpace(string(status))) {
				matchedStatus = true
				break
			}
		}
		if !matchedStatus {
			return false
		}
	}
	if filter.FromDate != nil {
		filteredAfter := filter.FromDate.UTC()
		if task.CreatedAt == nil || task.CreatedAt.Before(filteredAfter) {
			return false
		}
	}
	if filter.ToDate != nil {
		filteredBefore := filter.ToDate.UTC()
		if task.CreatedAt != nil && task.CreatedAt.After(filteredBefore) {
			return false
		}
		if task.CreatedAt == nil {
			return false
		}
	}
	return true
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
	return t.UTC().Format(time.RFC3339Nano)
}

func isDuplicateColumnError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate column") ||
		strings.Contains(msg, "already exists")
}
