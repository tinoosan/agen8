package agenttools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tinoosan/workbench-core/pkg/llm"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// ShellExecTool executes a shell command.
type ShellExecTool struct{}

func (t *ShellExecTool) Definition() llm.Tool {
	return llm.Tool{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "shell_exec",
			Description: "[CORE] Execute a shell command via bash. Supports pipes, redirects, and full shell syntax. Returns stdout, stderr, and exit code.",
			Strict:      true,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{"type": "string", "description": "Shell command to execute (e.g., \"ls -la | grep foo\")."},
					"cwd":     map[string]any{"type": stringOrNull, "description": "Working directory relative to the project root (e.g., \"internal/tools\"; default: \".\"). Do not use /project paths here."},
					"stdin":   map[string]any{"type": stringOrNull, "description": "Standard input to pipe to the command."},
				},
				"required":             []any{"command", "cwd", "stdin"},
				"additionalProperties": false,
			},
		},
	}
}

func (t *ShellExecTool) Execute(_ context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	var payload struct {
		Command string  `json:"command"`
		Cwd     *string `json:"cwd"`
		Stdin   *string `json:"stdin"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return types.HostOpRequest{}, err
	}
	cmd := strings.TrimSpace(payload.Command)
	if cmd == "" {
		return types.HostOpRequest{}, fmt.Errorf("command is required")
	}
	cwd := ""
	if payload.Cwd != nil {
		cwd = *payload.Cwd
	}
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		cwd = "."
	}
	stdin := ""
	if payload.Stdin != nil {
		stdin = *payload.Stdin
	}
	return types.HostOpRequest{
		Op:    types.HostOpShellExec,
		Argv:  []string{"bash", "-c", cmd},
		Cwd:   cwd,
		Stdin: stdin,
	}, nil
}
