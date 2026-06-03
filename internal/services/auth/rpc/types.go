package rpc

import "time"

type Identity struct {
	UserID string
	Role   string
}

type AuthStatusParams struct{}

type AuthStatusResult struct {
	Authenticated bool   `json:"authenticated"`
	UserID        string `json:"userId,omitempty"`
	Role          string `json:"role,omitempty"`
	User          *User  `json:"user,omitempty"`
}

type User struct {
	ID   string `json:"id"`
	Role string `json:"role"`
}

type LoginParams struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResult struct {
	UserID    string    `json:"userId"`
	Role      string    `json:"role"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type LogoutParams struct {
	Token string `json:"token"`
}

type LogoutResult struct {
	Revoked bool `json:"revoked"`
}

type CreateAPIKeyParams struct {
	Name      string     `json:"name"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

type CreateAPIKeyResult struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Prefix    string     `json:"prefix"`
	Token     string     `json:"token"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

type RevokeAPIKeyParams struct {
	ID string `json:"id"`
}

type RevokeAPIKeyResult struct {
	Revoked bool `json:"revoked"`
}
