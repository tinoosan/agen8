package session

import (
	"fmt"
	"strings"
	"time"

	user "github.com/tinoosan/agen8-mcp-server/internal/services/user/domain"
)

type ID struct {
	value string
}

func NewID(raw string) (ID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ID{}, fmt.Errorf("session id is required")
	}
	return ID{value: raw}, nil
}

func (id ID) String() string {
	return id.value
}

type Session struct {
	ID        ID
	UserID    user.ID
	TokenHash string
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

func (s Session) IsActive(now time.Time) bool {
	if s.RevokedAt != nil {
		return false
	}
	return s.ExpiresAt.After(now.UTC())
}
