package agent

import internalagent "github.com/tinoosan/workbench-core/internal/agent"

type Agent = internalagent.Agent
type Config = internalagent.Config
type HostExecutor = internalagent.HostExecutor
type HostExecFunc = internalagent.HostExecFunc
type ContextSource = internalagent.ContextSource
type ContextSourceFunc = internalagent.ContextSourceFunc
type Hooks = internalagent.Hooks

func New(cfg Config) (*Agent, error) {
	return internalagent.New(cfg)
}
