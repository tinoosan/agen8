package infra

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	credentialdomain "github.com/tinoosan/agen8-mcp-server/internal/services/credential/domain"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

func (s *sqlStore) rebind(query string) string {
	return storagedb.Rebind(query, s.dialect)
}

func validateCredential(record credentialdomain.Record) (credentialdomain.Record, error) {
	record.ID = credentialdomain.ID(strings.TrimSpace(string(record.ID)))
	record.Kind = credentialdomain.Kind(strings.TrimSpace(string(record.Kind)))
	record.Label = strings.TrimSpace(record.Label)
	record.Status = credentialdomain.Status(strings.TrimSpace(string(record.Status)))
	if record.CreatedAt.IsZero() {
		return credentialdomain.Record{}, fmt.Errorf("credential created at is required")
	}
	if record.UpdatedAt.IsZero() {
		return credentialdomain.Record{}, fmt.Errorf("credential updated at is required")
	}
	credential, err := credentialdomain.Wrap(record)
	if err != nil {
		return credentialdomain.Record{}, err
	}
	return credential.Record(), nil
}

func validateMaterial(material credentialdomain.SecretMaterial) (credentialdomain.SecretMaterial, error) {
	material.CredentialID = credentialdomain.ID(strings.TrimSpace(string(material.CredentialID)))
	material.StorageKind = credentialdomain.StorageKind(strings.TrimSpace(string(material.StorageKind)))
	if material.CredentialID == "" {
		return credentialdomain.SecretMaterial{}, fmt.Errorf("credential material id is required")
	}
	switch material.StorageKind {
	case credentialdomain.StorageLocalEncrypted:
		if len(material.Payload) == 0 {
			return credentialdomain.SecretMaterial{}, fmt.Errorf("credential material payload is required")
		}
	case credentialdomain.StorageSSHAgent:
		material.Payload = nil
	default:
		return credentialdomain.SecretMaterial{}, fmt.Errorf("unsupported credential storage kind %q", material.StorageKind)
	}
	if material.UpdatedAt.IsZero() {
		return credentialdomain.SecretMaterial{}, fmt.Errorf("credential material updated at is required")
	}
	material.UpdatedAt = material.UpdatedAt.UTC()
	return material, nil
}

func marshalFields(fields []credentialdomain.FieldRef) (string, error) {
	raw, err := json.Marshal(fields)
	if err != nil {
		return "", fmt.Errorf("marshal credential fields: %w", err)
	}
	return string(raw), nil
}

func unmarshalFields(raw string) ([]credentialdomain.FieldRef, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var fields []credentialdomain.FieldRef
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return nil, fmt.Errorf("unmarshal credential fields: %w", err)
	}
	return fields, nil
}

func credentialWhere(filter credentialdomain.Filter) (string, []any, error) {
	if filter.Limit < 0 || filter.Offset < 0 {
		return "", nil, fmt.Errorf("credential limit and offset must be non-negative")
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
	if len(clauses) == 0 {
		return "", args, nil
	}
	return " WHERE " + strings.Join(clauses, " AND "), args, nil
}

func scanCredential(scanner interface{ Scan(dest ...any) error }) (credentialdomain.Record, error) {
	var record credentialdomain.Record
	var fieldsJSON, createdAt, updatedAt string
	if err := scanner.Scan(&record.ID, &record.Kind, &record.Label, &record.Status, &fieldsJSON, &createdAt, &updatedAt); err != nil {
		return credentialdomain.Record{}, err
	}
	fields, err := unmarshalFields(fieldsJSON)
	if err != nil {
		return credentialdomain.Record{}, err
	}
	record.Fields = fields
	record.CreatedAt = parseTime(createdAt)
	record.UpdatedAt = parseTime(updatedAt)
	return validateCredential(record)
}

func scanCredentials(rows *sql.Rows) ([]credentialdomain.Record, error) {
	var out []credentialdomain.Record
	for rows.Next() {
		record, err := scanCredential(rows)
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
