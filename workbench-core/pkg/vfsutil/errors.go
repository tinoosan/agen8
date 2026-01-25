package vfsutil

import "errors"

var (
	ErrInvalidPath = errors.New("invalid path")
	ErrEscapesRoot = errors.New("path escapes mount root")
)
