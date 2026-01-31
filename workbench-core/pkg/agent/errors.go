package agent

import "github.com/tinoosan/workbench-core/pkg/types"

// ErrApprovalRequired is retained for backward compatibility but is no longer emitted.
// Autonomous mode executes without approval gates.
type ErrApprovalRequired struct {
	PendingOps         []types.HostOpRequest
	PendingToolCallIDs []string
}

func (ErrApprovalRequired) Error() string { return "approvals are disabled" }
