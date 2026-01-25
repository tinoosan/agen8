package agent

import (
	"fmt"

	"github.com/tinoosan/workbench-core/pkg/llm"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// ErrApprovalRequired is returned when dangerous host ops require user consent.
type ErrApprovalRequired struct {
	AssistantMsg       llm.LLMMessage
	PendingOps         []types.HostOpRequest
	PendingToolCallIDs []string
}

func (e ErrApprovalRequired) Error() string {
	return fmt.Sprintf("approval required for %d op(s)", len(e.PendingOps))
}
