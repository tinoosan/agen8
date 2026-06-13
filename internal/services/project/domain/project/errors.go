package project

import "errors"

var ErrNotFound = errors.New("project not found")

// ErrNotArchived reports that a delete was attempted on a project that has not
// been archived first. Deletion is permanent, so it is a deliberate two-step:
// archive, then delete. This is a client precondition, not a server fault — the
// RPC layer maps it to an invalid-params error rather than a generic internal
// error so callers see what to do next.
var ErrNotArchived = errors.New("project must be archived before deletion")
