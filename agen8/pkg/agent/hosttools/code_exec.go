package hosttools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	llmtypes "github.com/tinoosan/agen8/pkg/llm/types"
	"github.com/tinoosan/agen8/pkg/types"
)

// CodeExecTool executes model-authored Python code with an in-process tools SDK bridge.
type CodeExecTool struct{}

func (t *CodeExecTool) Definition() llmtypes.Tool {
	return llmtypes.Tool{
		Type: "function",
		Function: llmtypes.ToolFunction{
			Name:        "code_exec",
			Description: "[CORE] Execute Python with bridged host tools. Batch multiple tool calls in one invocation to minimize round-trips; avoid single-tool code_exec calls when no logic surrounds them. Call tools as `tools.<tool>(key=value)` (preferred), `tools.<tool>({\"key\": value})` (compat), or `import tools` then `tools.<tool>(...)`. Use Python literals `True`/`False`/`None` (not JSON `true`/`false`/`null`). In code_exec-only workflows, use code_exec to orchestrate all tool operations, including standalone delegation via `tools.task_create(goal=\"...\", spawnWorker=True)` (compat alias: `spawn_worker`) and team delegation via `tools.task_create(goal=\"...\", assignedRole=\"cto\")`. Do not import tool namespaces as Python modules (invalid: `import tasks`; only `import tools` is supported). For side effects, do NOT write files directly from Python; use `tools.fs_write/fs_edit/fs_append/fs_patch` (invalid: `open(\"/workspace/x.txt\", \"w\")`). Set `result = ...` for structured return. Tool failures raise ToolError.",
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
						"description": "Working directory (string or null). Use VFS paths like /workspace (default), /project, /knowledge, /skills, /plan, /tasks, /memory. Use null to accept default.",
					},
					"timeoutMs": map[string]any{
						"type":        intOrNull,
						"description": "Execution timeout in milliseconds (integer or null). Use null to accept runtime default.",
					},
					"maxOutputBytes": map[string]any{
						"type":        intOrNull,
						"description": "Maximum combined output bytes for returned logs/result payload (integer or null). Use null for default.",
					},
				},
				"required":             []any{"language", "code", "cwd", "timeoutMs", "maxOutputBytes"},
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
	return req, nil
}
