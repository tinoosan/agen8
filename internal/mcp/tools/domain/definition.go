package domain

// ToolDefinition describes one callable tool exposed to an agent runtime.
type ToolDefinition struct {
	Type     string                 `json:"type"`
	Function ToolFunctionDefinition `json:"function"`
}

// ToolFunctionDefinition carries the function-call contract for a tool.
type ToolFunctionDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Strict      bool   `json:"strict,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}
