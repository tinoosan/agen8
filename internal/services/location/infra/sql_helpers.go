package infra

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	locationdomain "github.com/tinoosan/agen8-mcp-server/internal/services/location/domain"
)

func validateLocation(record locationdomain.Record) (locationdomain.Record, error) {
	record.ID = locationdomain.ID(strings.TrimSpace(string(record.ID)))
	record.Kind = locationdomain.Kind(strings.TrimSpace(string(record.Kind)))
	record.Label = strings.TrimSpace(record.Label)
	record.Address = locationdomain.Address{
		Host:     strings.TrimSpace(record.Address.Host),
		Port:     record.Address.Port,
		Username: strings.TrimSpace(record.Address.Username),
	}
	record.Status = locationdomain.Status(strings.TrimSpace(string(record.Status)))
	record.CredentialRef = strings.TrimSpace(record.CredentialRef)
	record.LastProbeError = strings.TrimSpace(record.LastProbeError)
	if record.ID == "" {
		return locationdomain.Record{}, fmt.Errorf("location id is required")
	}
	if record.Kind == "" {
		return locationdomain.Record{}, fmt.Errorf("location kind is required")
	}
	if record.Status == "" {
		return locationdomain.Record{}, fmt.Errorf("location status is required")
	}
	if record.CreatedAt.IsZero() {
		return locationdomain.Record{}, fmt.Errorf("location created at is required")
	}
	if record.UpdatedAt.IsZero() {
		return locationdomain.Record{}, fmt.Errorf("location updated at is required")
	}
	credentialRef := record.CredentialRef
	probe := record.Probe
	lastProbeError := record.LastProbeError
	lastProbedAt := record.LastProbedAt
	location, err := locationdomain.Wrap(record)
	if err != nil {
		return locationdomain.Record{}, err
	}
	record = location.Record()
	record.CredentialRef = credentialRef
	record.Probe = locationdomain.Probe{
		Reachable:    probe.Reachable,
		FileBrowsing: probe.FileBrowsing,
		Exec:         probe.Exec,
		Codex:        probe.Codex,
		Claude:       probe.Claude,
	}
	record.LastProbeError = lastProbeError
	record.LastProbedAt = lastProbedAt
	if record.LastProbedAt != nil {
		probedAt := record.LastProbedAt.UTC()
		record.LastProbedAt = &probedAt
	}
	return record, nil
}

func locationWhere(filter locationdomain.Filter) (string, []any, error) {
	if filter.Limit < 0 || filter.Offset < 0 {
		return "", nil, fmt.Errorf("location limit and offset must be non-negative")
	}
	var clauses []string
	var args []any
	if filter.Kind != "" {
		clauses = append(clauses, "kind = ?")
		args = append(args, strings.TrimSpace(string(filter.Kind)))
	}
	if filter.Status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, strings.TrimSpace(string(filter.Status)))
	}
	if filter.Ready != nil {
		clauses = append(clauses, "ready = ?")
		args = append(args, boolInt(*filter.Ready))
	}
	if len(clauses) == 0 {
		return "", args, nil
	}
	return " WHERE " + strings.Join(clauses, " AND "), args, nil
}

func scanLocation(scanner interface{ Scan(dest ...any) error }) (locationdomain.Record, error) {
	var record locationdomain.Record
	var ready, reachable, fileBrowsing, execReady, codexReady, claudeReady int
	var lastProbedAt string
	var createdAt, updatedAt string
	if err := scanner.Scan(
		&record.ID, &record.Kind, &record.Label, &record.Address.Host, &record.Address.Port, &record.Address.Username,
		&record.Status, &ready, &record.CredentialRef, &reachable, &fileBrowsing, &execReady, &codexReady, &claudeReady,
		&record.LastProbeError, &lastProbedAt, &createdAt, &updatedAt,
	); err != nil {
		return locationdomain.Record{}, err
	}
	record.Ready = ready != 0
	record.Probe = locationdomain.Probe{
		Reachable:    reachable != 0,
		FileBrowsing: fileBrowsing != 0,
		Exec:         execReady != 0,
		Codex:        codexReady != 0,
		Claude:       claudeReady != 0,
	}
	if t := parseTime(lastProbedAt); !t.IsZero() {
		record.LastProbedAt = &t
	}
	record.CreatedAt = parseTime(createdAt)
	record.UpdatedAt = parseTime(updatedAt)
	return validateLocation(record)
}

func scanLocations(rows *sql.Rows) ([]locationdomain.Record, error) {
	var out []locationdomain.Record
	for rows.Next() {
		record, err := scanLocation(rows)
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

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func formatOptionalTime(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return formatTime(*t)
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

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
