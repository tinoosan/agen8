package domain

import (
	"errors"
)

var (
	ErrMessageNotFound = errors.New("message not found")
	ErrConsumed        = errors.New("message is already consumed")
	ErrInvalidFilter   = errors.New("invalid message filter")
)
