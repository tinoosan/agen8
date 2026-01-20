package vfsutil

import "errors"

var (
	// ErrInvalidPath indicates a provided path is not valid for the contract.
	ErrInvalidPath = errors.New("invalid path")

	// ErrEscapesRoot indicates a path attempted to escape its mount/root.
	ErrEscapesRoot = errors.New("path escapes mount root")
)

