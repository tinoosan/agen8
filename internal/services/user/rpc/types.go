package rpc

import (
	"time"

	user "github.com/tinoosan/agen8-mcp-server/internal/services/user/domain"
)

type UserView struct {
	ID        string     `json:"id"`
	Email     string     `json:"email"`
	Name      string     `json:"name"`
	Role      string     `json:"role"`
	Lifecycle string     `json:"lifecycle"`
	CreatedAt *time.Time `json:"createdAt,omitempty"`
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`
}

func NewUserView(record user.User) UserView {
	return UserView{
		ID:        record.ID.String(),
		Email:     record.Email,
		Name:      record.Name,
		Role:      string(record.Role),
		Lifecycle: string(record.Lifecycle),
		CreatedAt: cloneTime(record.CreatedAt),
		UpdatedAt: cloneTime(record.UpdatedAt),
	}
}

type StatusParams struct{}

type StatusResult struct {
	SetupOpen bool      `json:"setupOpen"`
	User      *UserView `json:"user,omitempty"`
}

type GetParams struct {
	UserID string `json:"userId,omitempty"`
}

type GetResult struct {
	User UserView `json:"user"`
}

type UpdateProfileParams struct {
	Email *string `json:"email,omitempty"`
	Name  *string `json:"name,omitempty"`
}

type UpdateProfileResult struct {
	User UserView `json:"user"`
}

type SuspendParams struct {
	UserID string `json:"userId"`
}

type SuspendResult struct {
	User UserView `json:"user"`
}

type CloseParams struct {
	UserID string `json:"userId,omitempty"`
}

type CloseResult struct {
	User UserView `json:"user"`
}

func cloneTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}
