package linktoken

import (
	"fmt"
	"strings"
	"time"

	user "github.com/tinoosan/agen8/internal/services/user/domain"
)

type ID struct {
	value string
}

func NewID(raw string) (ID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ID{}, fmt.Errorf("link token id is required")
	}
	return ID{value: raw}, nil
}

func (id ID) String() string {
	return id.value
}

// LinkToken is the wlt_ workspace link token. It mirrors apikey.Key but binds a
// project (and optionally a workspace), not just a user. ProjectID and
// WorkspaceID are opaque strings — references to identities owned by the project
// service — so the auth domain never imports the project domain.
type LinkToken struct {
	ID          ID
	UserID      user.ID
	ProjectID   string
	WorkspaceID string
	Label       string
	Prefix      string
	TokenHash   string
	ExpiresAt   *time.Time
	RevokedAt   *time.Time
	CreatedAt   time.Time
}

func (t LinkToken) IsActive(now time.Time) bool {
	if t.RevokedAt != nil {
		return false
	}
	if t.ExpiresAt == nil {
		return true
	}
	return t.ExpiresAt.After(now.UTC())
}
