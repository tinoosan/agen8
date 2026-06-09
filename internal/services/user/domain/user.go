package user

import (
	"fmt"
	"strings"
	"time"
)

type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
)

type Lifecycle string

const (
	LifecycleActive    Lifecycle = "active"
	LifecycleSuspended Lifecycle = "suspended"
	LifecycleClosed    Lifecycle = "closed"
)

type User struct {
	ID          ID
	Email       string
	Name        string
	Role        Role
	Lifecycle   Lifecycle
	Preferences Preferences
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Preferences struct {
	Theme              string `json:"theme,omitempty"`
	LastDarkTheme      string `json:"lastDarkTheme,omitempty"`
	LastLightTheme     string `json:"lastLightTheme,omitempty"`
	DefaultProjectView string `json:"defaultProjectView,omitempty"`
	FontFamily         string `json:"fontFamily,omitempty"`
	FontScale          int    `json:"fontScale,omitempty"`
}

type NewInput struct {
	ID    ID
	Email string
	Name  string
	Role  Role
	Now   time.Time
}

func New(input NewInput) (User, error) {
	if input.ID.IsZero() {
		return User{}, fmt.Errorf("user id is required")
	}
	email := strings.ToLower(strings.TrimSpace(input.Email))
	if email == "" {
		return User{}, fmt.Errorf("user email is required")
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return User{}, fmt.Errorf("user name is required")
	}
	role := input.Role
	if role == "" {
		return User{}, fmt.Errorf("user role is required")
	}
	if role != RoleAdmin && role != RoleUser {
		return User{}, fmt.Errorf("unsupported user role %q", role)
	}
	now := input.Now
	if now.IsZero() {
		return User{}, fmt.Errorf("user timestamp is required")
	}
	now = now.UTC()
	return User{
		ID:        input.ID,
		Email:     email,
		Name:      name,
		Role:      role,
		Lifecycle: LifecycleActive,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (u User) IsActive() bool {
	return u.Lifecycle == LifecycleActive
}
