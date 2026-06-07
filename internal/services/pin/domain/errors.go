package domain

import "errors"

// ErrNotFound is returned when a pin lookup or delete targets a node that is
// not pinned in the given project.
var ErrNotFound = errors.New("pin: not found")
