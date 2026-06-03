package auth

import "errors"

var (
	ErrInvalidCredential = errors.New("invalid credential")
	ErrTokenExpired      = errors.New("auth token expired")
	ErrTokenRevoked      = errors.New("auth token revoked")
	ErrTokenNotFound     = errors.New("auth token not found")
	ErrUserInactive      = errors.New("user account is not active")
	ErrPasswordTooShort  = errors.New("password must be at least 8 characters")
)
