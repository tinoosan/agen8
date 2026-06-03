package infra

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"

	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/cluster"
	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/project"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
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

func validateCluster(record cluster.Record) (cluster.Record, error) {
	record.ID = cluster.ID(strings.TrimSpace(string(record.ID)))
	record.ProjectID = types.ProjectID(strings.TrimSpace(string(record.ProjectID)))
	record.Name = strings.TrimSpace(record.Name)
	record.Status = cluster.Status(strings.TrimSpace(string(record.Status)))
	if record.ID == "" {
		return cluster.Record{}, fmt.Errorf("cluster id is required")
	}
	if record.ProjectID == "" {
		return cluster.Record{}, fmt.Errorf("project id is required")
	}
	if record.Name == "" {
		return cluster.Record{}, fmt.Errorf("cluster name is required")
	}
	if record.Status == "" {
		record.Status = cluster.StatusOpen
	}
	if record.Status != cluster.StatusOpen && record.Status != cluster.StatusClosed {
		return cluster.Record{}, fmt.Errorf("unsupported cluster status %q", record.Status)
	}
	if record.CreatedAt.IsZero() {
		return cluster.Record{}, fmt.Errorf("cluster created at is required")
	}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = record.CreatedAt
	}
	record.CreatedAt = record.CreatedAt.UTC()
	record.UpdatedAt = record.UpdatedAt.UTC()
	return record, nil
}

func validateSpaceRef(ref cluster.SpaceRefRecord) (cluster.SpaceRefRecord, error) {
	ref.ClusterID = cluster.ID(strings.TrimSpace(string(ref.ClusterID)))
	ref.SpaceID = spacedomain.SpaceID(strings.TrimSpace(string(ref.SpaceID)))
	if ref.ClusterID == "" {
		return cluster.SpaceRefRecord{}, fmt.Errorf("cluster id is required")
	}
	if ref.SpaceID == "" {
		return cluster.SpaceRefRecord{}, fmt.Errorf("space id is required")
	}
	return ref, nil
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

func pinnedInt(value bool) int {
	if value {
		return 1
	}
	return 0
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

func clusterWhere(filter cluster.Filter) (string, []any, error) {
	var clauses []string
	var args []any
	if filter.ProjectID != "" {
		clauses = append(clauses, "project_id = ?")
		args = append(args, strings.TrimSpace(string(filter.ProjectID)))
	}
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
	if err := scanner.Scan(&record.ID, &record.LocationID, &record.Root, &record.Title, &record.Status, &createdAt, &updatedAt); err != nil {
		return project.Record{}, err
	}
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

func scanCluster(scanner interface{ Scan(dest ...any) error }) (cluster.Record, error) {
	var record cluster.Record
	var createdAt, updatedAt string
	if err := scanner.Scan(&record.ID, &record.ProjectID, &record.Name, &record.Status, &createdAt, &updatedAt); err != nil {
		return cluster.Record{}, err
	}
	record.CreatedAt = parseTime(createdAt)
	record.UpdatedAt = parseTime(updatedAt)
	return validateCluster(record)
}

func scanClusters(rows *sql.Rows) ([]cluster.Record, error) {
	var out []cluster.Record
	for rows.Next() {
		record, err := scanCluster(rows)
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

func scanClusterSpace(scanner interface{ Scan(dest ...any) error }) (cluster.SpaceRefRecord, error) {
	var ref cluster.SpaceRefRecord
	var pinned int
	if err := scanner.Scan(&ref.ClusterID, &ref.SpaceID, &ref.SortOrder, &pinned); err != nil {
		return cluster.SpaceRefRecord{}, err
	}
	ref.Pinned = pinned != 0
	return validateSpaceRef(ref)
}

func scanClusterSpaces(rows *sql.Rows) ([]cluster.SpaceRefRecord, error) {
	var out []cluster.SpaceRefRecord
	for rows.Next() {
		ref, err := scanClusterSpace(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
