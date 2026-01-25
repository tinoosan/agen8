package agent

import (
	internalagent "github.com/tinoosan/workbench-core/internal/agent"
	"github.com/tinoosan/workbench-core/internal/types"
)

type Agent = internalagent.Agent
type Config = internalagent.Config
type HostExecutor = internalagent.HostExecutor
type HostExecFunc = internalagent.HostExecFunc
type ContextSource = internalagent.ContextSource
type ContextSourceFunc = internalagent.ContextSourceFunc
type Hooks = internalagent.Hooks
type HostOpExecutor = internalagent.HostOpExecutor
type TraceMiddleware = internalagent.TraceMiddleware
type ContextConstructor = internalagent.ContextConstructor
type ContextUpdater = internalagent.ContextUpdater
type MemoryEvaluator = internalagent.MemoryEvaluator
type ProfileEvaluator = internalagent.ProfileEvaluator
type FileAttachment = internalagent.FileAttachment
type ErrApprovalRequired = internalagent.ErrApprovalRequired

func New(cfg Config) (*Agent, error) {
	return internalagent.New(cfg)
}

func DefaultMemoryEvaluator() *MemoryEvaluator {
	return internalagent.DefaultMemoryEvaluator()
}

func DefaultProfileEvaluator() *ProfileEvaluator {
	return internalagent.DefaultProfileEvaluator()
}

func SHA256Hex(s string) string {
	return internalagent.SHA256Hex(s)
}

func SessionContextBlock(s types.Session) string {
	return internalagent.SessionContextBlock(s)
}

func ApplyStructuredEdits(before string, input []byte) (string, error) {
	return internalagent.ApplyStructuredEdits(before, input)
}

func ApplyUnifiedDiffStrict(before string, diff string) (string, error) {
	return internalagent.ApplyUnifiedDiffStrict(before, diff)
}
