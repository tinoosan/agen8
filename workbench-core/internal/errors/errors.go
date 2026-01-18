package errors

import "errors"

var (
	// ErrNotFound indicates a requested resource does not exist.
	ErrNotFound = errors.New("not found")

	// ErrInvalid indicates a provided input (or persisted data) is invalid.
	ErrInvalid = errors.New("invalid")

	// ErrConflict indicates an operation is not allowed due to current state.
	ErrConflict = errors.New("conflict")

	// ErrInvalidPath indicates a provided path is not valid for the contract.
	ErrInvalidPath = errors.New("invalid path")

	// ErrEscapesRoot indicates a path attempted to escape its mount/root.
	ErrEscapesRoot = errors.New("path escapes mount root")

	// ErrRequired indicates a required value was missing.
	ErrRequired = errors.New("required value missing")
)

