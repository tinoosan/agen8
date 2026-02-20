package store

import pkgstore "github.com/tinoosan/agen8/pkg/store"

var (
	// ErrNotFound indicates a requested resource does not exist.
	// Prefer checking with errors.Is(err, ErrNotFound).
	ErrNotFound = pkgstore.ErrNotFound

	// ErrInvalid indicates a provided input (or persisted data) is invalid.
	// Prefer checking with errors.Is(err, ErrInvalid).
	ErrInvalid = pkgstore.ErrInvalid

	// ErrConflict indicates the operation is not allowed due to current state
	// (e.g. invalid state transition).
	// Prefer checking with errors.Is(err, ErrConflict).
	ErrConflict = pkgstore.ErrConflict
)
