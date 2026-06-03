package infra

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/services/auth/apikey"
	"github.com/tinoosan/agen8-mcp-server/internal/services/auth/session"
	user "github.com/tinoosan/agen8-mcp-server/internal/services/user/domain"
)

func timeString(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func nullableTimeString(value *time.Time) any {
	if value == nil || value.IsZero() {
		return nil
	}
	return timeString(*value)
}

func parseTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, fmt.Errorf("timestamp is required")
	}
	value, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, err
	}
	return value.UTC(), nil
}

func parseNullTime(raw sql.NullString) (*time.Time, error) {
	if !raw.Valid || strings.TrimSpace(raw.String) == "" {
		return nil, nil
	}
	value, err := parseTime(raw.String)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func scanSession(rawID, rawUserID, tokenHash, expiresAt string, revokedAt sql.NullString, createdAt string) (session.Session, error) {
	id, err := session.NewID(rawID)
	if err != nil {
		return session.Session{}, fmt.Errorf("scan session id: %w", err)
	}
	userID, err := user.NewID(rawUserID)
	if err != nil {
		return session.Session{}, fmt.Errorf("scan session user id: %w", err)
	}
	expires, err := parseTime(expiresAt)
	if err != nil {
		return session.Session{}, fmt.Errorf("scan session expires at: %w", err)
	}
	revoked, err := parseNullTime(revokedAt)
	if err != nil {
		return session.Session{}, fmt.Errorf("scan session revoked at: %w", err)
	}
	created, err := parseTime(createdAt)
	if err != nil {
		return session.Session{}, fmt.Errorf("scan session created at: %w", err)
	}
	record := session.Session{
		ID:        id,
		UserID:    userID,
		TokenHash: strings.TrimSpace(tokenHash),
		ExpiresAt: expires,
		RevokedAt: revoked,
		CreatedAt: created,
	}
	if err := validateSession(record); err != nil {
		return session.Session{}, fmt.Errorf("scan session record: %w", err)
	}
	return record, nil
}

func scanAPIKey(rawID, rawUserID, name, prefix, tokenHash string, expiresAt sql.NullString, revokedAt sql.NullString, createdAt string) (apikey.Key, error) {
	id, err := apikey.NewID(rawID)
	if err != nil {
		return apikey.Key{}, fmt.Errorf("scan api key id: %w", err)
	}
	userID, err := user.NewID(rawUserID)
	if err != nil {
		return apikey.Key{}, fmt.Errorf("scan api key user id: %w", err)
	}
	expires, err := parseNullTime(expiresAt)
	if err != nil {
		return apikey.Key{}, fmt.Errorf("scan api key expires at: %w", err)
	}
	revoked, err := parseNullTime(revokedAt)
	if err != nil {
		return apikey.Key{}, fmt.Errorf("scan api key revoked at: %w", err)
	}
	created, err := parseTime(createdAt)
	if err != nil {
		return apikey.Key{}, fmt.Errorf("scan api key created at: %w", err)
	}
	record := apikey.Key{
		ID:        id,
		UserID:    userID,
		Name:      strings.TrimSpace(name),
		Prefix:    strings.TrimSpace(prefix),
		TokenHash: strings.TrimSpace(tokenHash),
		ExpiresAt: expires,
		RevokedAt: revoked,
		CreatedAt: created,
	}
	if err := validateAPIKey(record); err != nil {
		return apikey.Key{}, fmt.Errorf("scan api key record: %w", err)
	}
	return record, nil
}

func validateSession(record session.Session) error {
	if strings.TrimSpace(record.ID.String()) == "" {
		return fmt.Errorf("session id is required")
	}
	if record.UserID.IsZero() {
		return fmt.Errorf("session user id is required")
	}
	if strings.TrimSpace(record.TokenHash) == "" {
		return fmt.Errorf("session token hash is required")
	}
	if record.ExpiresAt.IsZero() {
		return fmt.Errorf("session expires at is required")
	}
	if record.CreatedAt.IsZero() {
		return fmt.Errorf("session created at is required")
	}
	return nil
}

func validateAPIKey(record apikey.Key) error {
	if strings.TrimSpace(record.ID.String()) == "" {
		return fmt.Errorf("api key id is required")
	}
	if record.UserID.IsZero() {
		return fmt.Errorf("api key user id is required")
	}
	if strings.TrimSpace(record.Name) == "" {
		return fmt.Errorf("api key name is required")
	}
	if strings.TrimSpace(record.Prefix) == "" {
		return fmt.Errorf("api key prefix is required")
	}
	if strings.TrimSpace(record.TokenHash) == "" {
		return fmt.Errorf("api key token hash is required")
	}
	if record.CreatedAt.IsZero() {
		return fmt.Errorf("api key created at is required")
	}
	return nil
}

func requireAffected(result sql.Result, entity string) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s rows affected: %w", entity, err)
	}
	if affected == 0 {
		return fmt.Errorf("%s not found", entity)
	}
	return nil
}
