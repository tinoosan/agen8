package agent

import (
	"fmt"

	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/pkg/llm"
)

// ErrApprovalRequired is returned when dangerous host ops require user consent.
type ErrApprovalRequired struct {
	// AssistantMsg is the assistant message that triggered the tool plans.
	AssistantMsg llm.LLMMessage
	// PendingOps are the unexecuted host operations awaiting approval.
	PendingOps []types.HostOpRequest
	// PendingToolCallIDs map to the original tool call IDs for each PendingOp.
	PendingToolCallIDs []string
}

func (e ErrApprovalRequired) Error() string {
	return fmt.Sprintf("approval required for %d op(s)", len(e.PendingOps))
}
