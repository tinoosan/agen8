package hosttools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	llmtypes "github.com/tinoosan/workbench-core/pkg/llm/types"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// CodeExecTool executes model-authored Python code with an in-process tools SDK bridge.
type CodeExecTool struct{}

func (t *CodeExecTool) Definition() llmtypes.Tool {
	return llmtypes.Tool{
		Type: "function",
		Function: llmtypes.ToolFunction{
			Name:        "code_exec",
			Description: "[CORE] Execute Python code with a direct tools SDK bridge. In Python, call tools as `tools.<tool>(key=value)` (preferred), `tools.<tool>({\"key\": value})` (compatibility), or `import tools` then call `tools.<tool>(...)`. Example: `import tools; result = tools.fs_list(path=\"/project\")`. Set `result = ...` for structured return. Tool failures raise ToolError in Python.",
			Strict:      true,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"language": map[string]any{
						"type":        "string",
						"description": "Execution language. Must be \"python\".",
					},
					"code": map[string]any{
						"type":        "string",
						"description": "Python source code to run. Example: `data = tools.fs_read(path=\"/project/README.md\"); result = {\"len\": len(data.get(\"text\", \"\"))}`.",
					},
					"cwd": map[string]any{
						"type":        stringOrNull,
						"description": "Working directory (string or null). Use VFS paths like /workspace (default), /project, /skills, /plan, /tasks, /memory. Use null to accept default.",
					},
					"timeoutMs": map[string]any{
						"type":        intOrNull,
						"description": "Execution timeout in milliseconds (integer or null). Use null to accept runtime default.",
					},
					"maxOutputBytes": map[string]any{
						"type":        intOrNull,
						"description": "Maximum combined output bytes for returned logs/result payload (integer or null). Use null for default.",
					},
					"maxToolCalls": map[string]any{
						"type":        intOrNull,
						"description": "Maximum number of in-code tools.* calls allowed in this execution (integer or null). Use null for default.",
					},
				},
				"required":             []any{"language", "code", "cwd", "timeoutMs", "maxOutputBytes", "maxToolCalls"},
				"additionalProperties": false,
			},
		},
	}
}

func (t *CodeExecTool) Execute(_ context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	var payload struct {
		Language       string  `json:"language"`
		Code           string  `json:"code"`
		Cwd            *string `json:"cwd"`
		TimeoutMs      *int    `json:"timeoutMs"`
		MaxOutputBytes *int    `json:"maxOutputBytes"`
		MaxToolCalls   *int    `json:"maxToolCalls"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return types.HostOpRequest{}, err
	}
	lang := strings.ToLower(strings.TrimSpace(payload.Language))
	if lang == "" {
		return types.HostOpRequest{}, fmt.Errorf("language is required")
	}
	if lang != "python" {
		return types.HostOpRequest{}, fmt.Errorf("language must be \"python\"")
	}
	code := strings.TrimSpace(payload.Code)
	if code == "" {
		return types.HostOpRequest{}, fmt.Errorf("code is required")
	}
	req := types.HostOpRequest{
		Op:       types.HostOpCodeExec,
		Language: lang,
		Code:     code,
	}
	if payload.Cwd != nil {
		req.Cwd = strings.TrimSpace(*payload.Cwd)
	}
	if payload.TimeoutMs != nil {
		req.TimeoutMs = *payload.TimeoutMs
	}
	if payload.MaxOutputBytes != nil {
		req.MaxBytes = *payload.MaxOutputBytes
	}
	if payload.MaxToolCalls != nil {
		inp, err := json.Marshal(map[string]any{"maxToolCalls": *payload.MaxToolCalls})
		if err != nil {
			return types.HostOpRequest{}, err
		}
		req.Input = inp
	}
	return req, nil
}
