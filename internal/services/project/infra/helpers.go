package infra

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8/internal/core/types"
	"github.com/tinoosan/agen8/internal/services/project/domain/project"
)

func validateProject(record project.Record) (project.Record, error) {
	record.ID = types.ProjectID(strings.TrimSpace(string(record.ID)))
	record.LocationID = types.LocationID(strings.TrimSpace(string(record.LocationID)))
	record.Root = strings.TrimSpace(record.Root)
	record.Title = strings.TrimSpace(record.Title)
	record.Status = project.Status(strings.TrimSpace(string(record.Status)))
	if record.ID == "" {
		return project.Record{}, fmt.Errorf("project id is required")
	}
	if record.Root == "" {
		return project.Record{}, fmt.Errorf("project root is required")
	}
	if record.LocationID == "" {
		record.LocationID = "local"
	}
	if record.Status == "" {
		record.Status = project.StatusOpen
	}
	if record.Status != project.StatusOpen && record.Status != project.StatusArchived {
		return project.Record{}, fmt.Errorf("unsupported project status %q", record.Status)
	}
	if record.CreatedAt.IsZero() {
		return project.Record{}, fmt.Errorf("project created at is required")
	}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = record.CreatedAt
	}
	record.CreatedAt = record.CreatedAt.UTC()
	record.UpdatedAt = record.UpdatedAt.UTC()
	return record, nil
}

func timeString(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) time.Time {
	if strings.TrimSpace(value) == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

func projectWhere(filter project.Filter) (string, []any, error) {
	if filter.Limit < 0 || filter.Offset < 0 {
		return "", nil, fmt.Errorf("project limit and offset must be non-negative")
	}
	var clauses []string
	var args []any
	if filter.Status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, strings.TrimSpace(string(filter.Status)))
	}
	if len(clauses) == 0 {
		return "", args, nil
	}
	return " WHERE " + strings.Join(clauses, " AND "), args, nil
}

func scanProject(scanner interface{ Scan(dest ...any) error }) (project.Record, error) {
	var record project.Record
	var createdAt, updatedAt string
	if err := scanner.Scan(&record.ID, &record.LocationID, &record.Root, &record.Title, &record.Status, &createdAt, &updatedAt, &record.UserID); err != nil {
		return project.Record{}, err
	}
	record.UserID = strings.TrimSpace(record.UserID)
	record.CreatedAt = parseTime(createdAt)
	record.UpdatedAt = parseTime(updatedAt)
	return validateProject(record)
}

func scanProjects(rows *sql.Rows) ([]project.Record, error) {
	var out []project.Record
	for rows.Next() {
		record, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
