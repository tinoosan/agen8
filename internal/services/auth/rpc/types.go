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

type APIKeyView struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Prefix    string     `json:"prefix"`
	CreatedAt time.Time  `json:"createdAt"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	RevokedAt *time.Time `json:"revokedAt,omitempty"`
	Active    bool       `json:"active"`
}

type ListAPIKeysParams struct{}

type ListAPIKeysResult struct {
	Keys []APIKeyView `json:"keys"`
}

type RevokeAPIKeyParams struct {
	ID string `json:"id"`
}

type RevokeAPIKeyResult struct {
	Revoked bool `json:"revoked"`
}
