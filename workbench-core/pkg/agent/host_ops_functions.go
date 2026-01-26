package agent

import "github.com/tinoosan/workbench-core/pkg/llm"

// HostOpFunctions returns the remaining core host primitives.
func HostOpFunctions() []llm.Tool {
	return []llm.Tool{FinalAnswerTool()}
}
