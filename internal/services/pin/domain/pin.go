// Package domain defines the pin entity and its persistence contract.
//
// A Pin marks a graph node (mission, key result, task, decision - any node
// ref) as pinned within a project. Pins are scoped per-project and shared
// across everyone working in that project: the key is (project_id, node_ref),
// with no member dimension. This mirrors the prior localStorage behaviour
// (one pinned set per project) while moving the source of truth server-side
// so pins survive across browsers and sessions.
package domain

import (
	"errors"
	"strings"
	"time"
)

// Pin is a single pinned node within a project.
//
// NodeType is advisory metadata (e.g. "mission", "decision") so readers can
// render or filter without a second lookup; it is NOT part of the identity.
// Identity is (ProjectID, NodeRef) - re-pinning the same node is idempotent.
type Pin struct {
	ProjectID string
	NodeRef   string
	NodeType  string
	CreatedAt time.Time
}

// Validate enforces the minimal shape the store relies on: both halves of
// the composite key must be present. NodeType is intentionally optional.
func (p Pin) Validate() error {
	if strings.TrimSpace(p.ProjectID) == "" {
		return errors.New("pin: projectId is required")
	}
	if strings.TrimSpace(p.NodeRef) == "" {
		return errors.New("pin: nodeRef is required")
	}
	return nil
}
