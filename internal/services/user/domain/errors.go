package user

import "errors"

var (
	ErrNotFound           = errors.New("user not found")
	ErrSetupClosed        = errors.New("user setup is closed")
	ErrEmailAlreadyExists = errors.New("user email already exists")
)
