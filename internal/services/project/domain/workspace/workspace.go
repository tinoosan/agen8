package workspace

import (
	"fmt"
	"strings"
	"time"
)

// ID is the workspace identity. Like member.ID, it is defined in the service's
// own domain layer rather than a shared types package.
type ID string

func (id ID) String() string { return string(id) }

const (
	LifecycleActive  = "active"
	LifecycleRemoved = "removed"
)

// Record is one place a project is linked: a (location, root, machine) triple.
// A project owns many workspaces. ProjectID, UserID, and LocationID are plain
// strings — opaque references to identities owned by other services — so the
// workspace domain has no cross-service type coupling.
type Record struct {
	ID             ID         `json:"id"`
	ProjectID      string     `json:"projectId"`
	UserID         string     `json:"userId,omitempty"`
	LocationID     string     `json:"locationId"`
	Root           string     `json:"root"`
	Machine        string     `json:"machine,omitempty"`
	LifecycleState string     `json:"lifecycleState"`
	LinkedAt       time.Time  `json:"linkedAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
	LastSeenAt     *time.Time `json:"lastSeenAt,omitempty"`
}

// Workspace is the aggregate wrapper enforcing lifecycle rules.
type Workspace struct {
	inner Record
}

func WrapWorkspace(w Record) Workspace {
	if strings.TrimSpace(w.LifecycleState) == "" {
		w.LifecycleState = LifecycleActive
	}
	return Workspace{inner: w}
}

func (a Workspace) Inner() Record     { return a.inner }
func (a Workspace) ID() string        { return string(a.inner.ID) }
func (a Workspace) ProjectID() string { return a.inner.ProjectID }
func (a Workspace) IsActive() bool    { return a.inner.LifecycleState == LifecycleActive }

func (a Workspace) Remove(now time.Time) (Workspace, error) {
	if a.inner.LifecycleState == LifecycleRemoved {
		return Workspace{}, fmt.Errorf("workspace is already removed")
	}
	next := a.inner
	next.LifecycleState = LifecycleRemoved
	next.UpdatedAt = now.UTC()
	return Workspace{inner: next}, nil
}

// Touch records that the workspace was seen connecting, without changing its
// identity or lifecycle.
func (a Workspace) Touch(now time.Time) Workspace {
	next := a.inner
	seen := now.UTC()
	next.LastSeenAt = &seen
	next.UpdatedAt = seen
	return Workspace{inner: next}
}

func ValidateLifecycleState(v string) error {
	switch v {
	case LifecycleActive, LifecycleRemoved:
		return nil
	default:
		return fmt.Errorf("invalid lifecycleState %q", v)
	}
}
