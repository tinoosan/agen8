package infra

import (
	"fmt"
	"strings"
	"time"

	user "github.com/tinoosan/agen8-mcp-server/internal/services/user/domain"
)

func validateUserRecord(record user.User) error {
	if record.ID.IsZero() {
		return fmt.Errorf("user id is required")
	}
	if normalizeEmail(record.Email) == "" {
		return fmt.Errorf("user email is required")
	}
	if strings.TrimSpace(record.Name) == "" {
		return fmt.Errorf("user name is required")
	}
	if record.Role != user.RoleAdmin && record.Role != user.RoleUser {
		return fmt.Errorf("unsupported user role %q", record.Role)
	}
	switch record.Lifecycle {
	case user.LifecycleActive, user.LifecycleSuspended, user.LifecycleClosed:
	default:
		return fmt.Errorf("unsupported user lifecycle %q", record.Lifecycle)
	}
	if record.CreatedAt.IsZero() {
		return fmt.Errorf("user created at is required")
	}
	if record.UpdatedAt.IsZero() {
		return fmt.Errorf("user updated at is required")
	}
	return nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func timeString(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("timestamp is required")
	}
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

func mapEmailUniqueError(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "users.email") ||
		strings.Contains(msg, "idx_users_email") ||
		(strings.Contains(msg, "unique") && strings.Contains(msg, "email")) ||
		(strings.Contains(msg, "duplicate") && strings.Contains(msg, "email")) {
		return user.ErrEmailAlreadyExists
	}
	return err
}
