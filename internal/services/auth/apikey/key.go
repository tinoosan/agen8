package apikey

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
		return ID{}, fmt.Errorf("api key id is required")
	}
	return ID{value: raw}, nil
}

func (id ID) String() string {
	return id.value
}

type Key struct {
	ID        ID
	UserID    user.ID
	Name      string
	Prefix    string
	TokenHash string
	ExpiresAt *time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

func (k Key) IsActive(now time.Time) bool {
	if k.RevokedAt != nil {
		return false
	}
	if k.ExpiresAt == nil {
		return true
	}
	return k.ExpiresAt.After(now.UTC())
}
