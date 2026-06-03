package infra

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	credentialdomain "github.com/tinoosan/agen8-mcp-server/internal/services/credential/domain"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

type sqlStore struct {
	db      *sql.DB
	dialect storagedb.Dialect
	dataDir string
}

func newSQLStore(handle *storagedb.Handle, dataDir string) *sqlStore {
	return &sqlStore{
		db:      handle.DB(),
		dialect: handle.Dialect(),
		dataDir: dataDir,
	}
}

func (s *sqlStore) Get(ctx context.Context, id credentialdomain.ID) (credentialdomain.Record, error) {
	id = credentialdomain.ID(strings.TrimSpace(string(id)))
	if id == "" {
		return credentialdomain.Record{}, fmt.Errorf("credential id is required")
	}
	row := s.db.QueryRowContext(ctx, s.rebind(`
		SELECT credential_id, kind, label, status, fields_json, created_at, updated_at
		FROM credentials
		WHERE credential_id = ?
	`), id)
	record, err := scanCredential(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return credentialdomain.Record{}, credentialdomain.ErrNotFound
		}
		return credentialdomain.Record{}, fmt.Errorf("get credential %s: %w", id, err)
	}
	return record, nil
}

func (s *sqlStore) List(ctx context.Context, filter credentialdomain.Filter) ([]credentialdomain.Record, error) {
	where, args, err := credentialWhere(filter)
	if err != nil {
		return nil, err
	}
	query := `
		SELECT credential_id, kind, label, status, fields_json, created_at, updated_at
		FROM credentials` + where + `
		ORDER BY label ASC, credential_id ASC`
	if filter.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, filter.Limit)
	}
	if filter.Offset > 0 {
		query += ` OFFSET ?`
		args = append(args, filter.Offset)
	}
	rows, err := s.db.QueryContext(ctx, s.rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("list credentials: %w", err)
	}
	defer rows.Close()
	return scanCredentials(rows)
}

func (s *sqlStore) Save(ctx context.Context, record credentialdomain.Record) (credentialdomain.Record, error) {
	record, err := validateCredential(record)
	if err != nil {
		return credentialdomain.Record{}, err
	}
	fieldsJSON, err := marshalFields(record.Fields)
	if err != nil {
		return credentialdomain.Record{}, err
	}
	_, err = s.db.ExecContext(ctx, s.rebind(`
		INSERT INTO credentials (credential_id, kind, label, status, fields_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (credential_id) DO UPDATE SET
			kind = excluded.kind,
			label = excluded.label,
			status = excluded.status,
			fields_json = excluded.fields_json,
			created_at = excluded.created_at,
			updated_at = excluded.updated_at
	`), record.ID, record.Kind, record.Label, record.Status, fieldsJSON, formatTime(record.CreatedAt), formatTime(record.UpdatedAt))
	if err != nil {
		return credentialdomain.Record{}, fmt.Errorf("save credential %s: %w", record.ID, err)
	}
	return s.Get(ctx, record.ID)
}

func (s *sqlStore) Delete(ctx context.Context, id credentialdomain.ID) error {
	id = credentialdomain.ID(strings.TrimSpace(string(id)))
	if id == "" {
		return fmt.Errorf("credential id is required")
	}
	result, err := s.db.ExecContext(ctx, s.rebind(`DELETE FROM credentials WHERE credential_id = ?`), id)
	if err != nil {
		return fmt.Errorf("delete credential %s: %w", id, err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete credential %s rows affected: %w", id, err)
	}
	if count == 0 {
		return credentialdomain.ErrNotFound
	}
	return nil
}

func (s *sqlStore) PutMaterial(ctx context.Context, material credentialdomain.SecretMaterial) error {
	material, err := validateMaterial(material)
	if err != nil {
		return err
	}
	payload := material.Payload
	if material.StorageKind == credentialdomain.StorageLocalEncrypted {
		payload, err = encryptLocal(s.dataDir, material.Payload)
		if err != nil {
			return err
		}
	}
	_, err = s.db.ExecContext(ctx, s.rebind(`
		INSERT INTO credential_material (credential_id, storage_kind, payload, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (credential_id) DO UPDATE SET
			storage_kind = excluded.storage_kind,
			payload = excluded.payload,
			updated_at = excluded.updated_at
	`), material.CredentialID, material.StorageKind, payload, formatTime(material.UpdatedAt))
	if err != nil {
		return fmt.Errorf("put credential material %s: %w", material.CredentialID, err)
	}
	return nil
}

func (s *sqlStore) GetMaterial(ctx context.Context, id credentialdomain.ID) (credentialdomain.SecretMaterial, error) {
	id = credentialdomain.ID(strings.TrimSpace(string(id)))
	if id == "" {
		return credentialdomain.SecretMaterial{}, fmt.Errorf("credential id is required")
	}
	var material credentialdomain.SecretMaterial
	var updatedAt string
	if err := s.db.QueryRowContext(ctx, s.rebind(`
		SELECT credential_id, storage_kind, payload, updated_at
		FROM credential_material
		WHERE credential_id = ?
	`), id).Scan(&material.CredentialID, &material.StorageKind, &material.Payload, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return credentialdomain.SecretMaterial{}, credentialdomain.ErrNotFound
		}
		return credentialdomain.SecretMaterial{}, fmt.Errorf("get credential material %s: %w", id, err)
	}
	material.UpdatedAt = parseTime(updatedAt)
	if material.StorageKind == credentialdomain.StorageLocalEncrypted {
		payload, err := decryptLocal(s.dataDir, material.Payload)
		if err != nil {
			return credentialdomain.SecretMaterial{}, err
		}
		material.Payload = payload
	}
	return validateMaterial(material)
}

func (s *sqlStore) DeleteMaterial(ctx context.Context, id credentialdomain.ID) error {
	id = credentialdomain.ID(strings.TrimSpace(string(id)))
	if id == "" {
		return fmt.Errorf("credential id is required")
	}
	result, err := s.db.ExecContext(ctx, s.rebind(`DELETE FROM credential_material WHERE credential_id = ?`), id)
	if err != nil {
		return fmt.Errorf("delete credential material %s: %w", id, err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete credential material %s rows affected: %w", id, err)
	}
	if count == 0 {
		return credentialdomain.ErrNotFound
	}
	return nil
}
