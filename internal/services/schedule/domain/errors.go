package domain

import "errors"

var (
	ErrNotFound          = errors.New("schedule entry not found")
	ErrRunNotFound       = errors.New("schedule run not found")
	ErrRunAlreadyClaimed = errors.New("schedule run already claimed")
)
