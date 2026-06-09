package password

import (
	"fmt"
	"strings"
	"time"

	user "github.com/tinoosan/agen8/internal/services/user/domain"
)

type Credential struct {
	UserID       user.ID
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type NewInput struct {
	UserID       user.ID
	PasswordHash string
	Now          time.Time
}

func New(input NewInput) (Credential, error) {
	input.PasswordHash = strings.TrimSpace(input.PasswordHash)
	if input.UserID.IsZero() {
		return Credential{}, fmt.Errorf("user id is required")
	}
	if input.PasswordHash == "" {
		return Credential{}, fmt.Errorf("password hash is required")
	}
	if input.Now.IsZero() {
		return Credential{}, fmt.Errorf("timestamp is required")
	}
	now := input.Now.UTC()
	return Credential{
		UserID:       input.UserID,
		PasswordHash: input.PasswordHash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}
