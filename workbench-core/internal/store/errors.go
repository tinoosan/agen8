package store

import (
	"errors"

	werrors "github.com/tinoosan/workbench-core/internal/errors"
)

var (
	// ErrNotFound indicates a requested resource does not exist.
	// Prefer checking with errors.Is(err, ErrNotFound).
	ErrNotFound = errors.Join(werrors.ErrNotFound, errors.New("not found"))

	// ErrInvalid indicates a provided input (or persisted data) is invalid.
	// Prefer checking with errors.Is(err, ErrInvalid).
	ErrInvalid = errors.Join(werrors.ErrInvalid, errors.New("invalid"))

	// ErrConflict indicates the operation is not allowed due to current state
	// (e.g. invalid state transition).
	// Prefer checking with errors.Is(err, ErrConflict).
	ErrConflict = errors.Join(werrors.ErrConflict, errors.New("conflict"))
)

