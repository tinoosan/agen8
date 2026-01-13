package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tinoosan/workbench-core/internal/types"
)

// Agent is the minimal "agent loop v0": ask a model what to do next and execute it.
//
// This loop is deliberately tiny so you can validate the core architecture:
//   - host primitives (fs.* and tool.run) are always available (not discovered)
//   - tools are discovered via /tools manifests
//   - tool execution writes results under /results/<callId>/...
//
// v0 limitations:
//   - no streaming
//   - no function calling / tool calling features
//   - no events (telemetry can be added later as a decorator)
//
// Contract:
// The model must output exactly one JSON object per turn:
//   - either a HostOpRequest (op=fs.list/fs.read/fs.write/fs.append/tool.run)
//   - or {"op":"final","text":"..."} to stop the loop
type Agent struct {
	LLM  types.LLMClient
	Exec func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse

	// Model is required. Example: "openai/gpt-4o-mini" (via OpenRouter), etc.
	Model string

	// SystemPrompt is the system instructions passed to the model.
	//
	// If empty, the agent uses an internal default prompt.
	// In production, prefer setting this explicitly (e.g. from INITIAL_PROMPT.md)
	// so the contract is centralized and pkgsite-visible.
	SystemPrompt string

	// MaxSteps caps the number of model -> host op iterations.
	// If zero, a default is used.
	MaxSteps int

	// Logf is an optional logger used to print what the agent is doing.
	// Example: log.Printf.
	Logf func(format string, args ...any)
}

// Run executes the agent loop for a single user goal and returns the final response text.
func (a *Agent) Run(ctx context.Context, goal string) (string, error) {
	if a == nil || a.LLM == nil {
		return "", fmt.Errorf("agent LLM is required")
	}
	if a.Exec == nil {
		return "", fmt.Errorf("agent Exec is required")
	}
	if strings.TrimSpace(a.Model) == "" {
		return "", fmt.Errorf("agent Model is required")
	}
	if strings.TrimSpace(goal) == "" {
		return "", fmt.Errorf("goal is required")
	}

	maxSteps := a.MaxSteps
	if maxSteps == 0 {
		maxSteps = 20
	}
	if maxSteps < 1 {
		return "", fmt.Errorf("MaxSteps must be >= 1")
	}

	system := strings.TrimSpace(a.SystemPrompt)
	if system == "" {
		system = agentLoopV0SystemPrompt()
	}

	msgs := []types.LLMMessage{
		{Role: "user", Content: goal},
	}

	for step := 1; step <= maxSteps; step++ {
		resp, err := a.LLM.Generate(ctx, types.LLMRequest{
			Model:     a.Model,
			System:    system,
			Messages:  msgs,
			MaxTokens: 1024,
			JSONOnly:  true,
		})
		if err != nil {
			return "", err
		}

		opJSON := extractJSONObject(resp.Text)
		if a.Logf != nil {
			a.Logf("model -> host (step %d): %s", step, strings.TrimSpace(opJSON))
		}
		var op types.HostOpRequest
		if err := json.Unmarshal([]byte(opJSON), &op); err != nil {
			// Feed the parse error back to the model as a user message and keep going.
			msgs = append(msgs,
				types.LLMMessage{Role: "assistant", Content: resp.Text},
				types.LLMMessage{Role: "user", Content: "Your last message was not valid JSON for the required schema. Error: " + err.Error() + ". Return ONLY one JSON object."},
			)
			continue
		}

		if err := validateModelOp(op); err != nil {
			msgs = append(msgs,
				types.LLMMessage{Role: "assistant", Content: resp.Text},
				types.LLMMessage{Role: "user", Content: "Your last JSON op was invalid: " + err.Error() + ". Return ONLY one corrected JSON object."},
			)
			continue
		}

		if op.Op == "final" {
			return strings.TrimSpace(op.Text), nil
		}

		hostResp := a.Exec(ctx, op)
		hostRespJSON, _ := json.MarshalIndent(hostResp, "", "  ")

		// Feed the host response back to the model as the next user turn.
		msgs = append(msgs,
			types.LLMMessage{Role: "assistant", Content: opJSON},
			types.LLMMessage{
				Role: "user",
				Content: "HostOpResponse:\n" + string(hostRespJSON) +
					"\n\nReturn the next HostOpRequest as ONE JSON object (or {\"op\":\"final\",\"text\":\"...\"}).",
			},
		)
	}

	return "", fmt.Errorf("agent exceeded max steps (%d) without final", maxSteps)
}

func agentLoopV0SystemPrompt() string {
	return strings.TrimSpace(`
You are an agent running inside a host-controlled environment.

You can ONLY interact with the environment by returning exactly ONE JSON object per turn.
Do not include any other text, markdown, or code fences.

Allowed operations (host primitives; always available):
  - fs.list:   {"op":"fs.list","path":"/tools"}
  - fs.read:   {"op":"fs.read","path":"/tools/<toolId>","maxBytes":2048}
  - fs.write:  {"op":"fs.write","path":"/workspace/file.txt","text":"..."}
  - fs.append: {"op":"fs.append","path":"/workspace/log.txt","text":"..."}
  - tool.run:  {"op":"tool.run","toolId":"<toolId>","actionId":"<actionId>","input":{...},"timeoutMs":5000}
  - final:     {"op":"final","text":"..."}   (stop)

Tool discovery rules:
  - Tools are discovered via VFS only:
      fs.list("/tools") returns tool IDs as directory entries.
      fs.read("/tools/<toolId>") returns the tool manifest JSON bytes.
  - Do NOT assume tool actions; read the manifest first.

Tool execution + results rules:
  - To run a tool, use tool.run(...) with the selected toolId/actionId and an input object.
  - After tool.run, the host persists:
      /results/<callId>/response.json
      /results/<callId>/<artifactPath>
  - You can read those files with fs.read to inspect outputs/artifacts.

Always:
  - Prefer discovering tools first.
  - Use small reads (maxBytes) unless you need more.
  - If a host op fails (ok=false), recover and try a different op or return final with an explanation.
`)
}

func extractJSONObject(s string) string {
	trim := strings.TrimSpace(s)
	trim = strings.TrimPrefix(trim, "```json")
	trim = strings.TrimPrefix(trim, "```")
	trim = strings.TrimSuffix(trim, "```")
	trim = strings.TrimSpace(trim)

	start := strings.Index(trim, "{")
	end := strings.LastIndex(trim, "}")
	if start >= 0 && end >= 0 && end > start {
		return trim[start : end+1]
	}
	return trim
}

func validateModelOp(op types.HostOpRequest) error {
	switch op.Op {
	case "fs.list", "fs.read", "fs.write", "fs.append", "tool.run", "final":
	default:
		return fmt.Errorf("unknown op %q", op.Op)
	}

	switch op.Op {
	case "final":
		if strings.TrimSpace(op.Text) == "" {
			return fmt.Errorf("final.text is required")
		}
		return nil
	case "fs.list", "fs.read":
		if strings.TrimSpace(op.Path) == "" {
			return fmt.Errorf("path is required")
		}
	case "fs.write", "fs.append":
		if strings.TrimSpace(op.Path) == "" {
			return fmt.Errorf("path is required")
		}
		// text can be empty (writing empty file is valid), but the field must exist in JSON;
		// we can't reliably distinguish "missing" vs "" after unmarshal, so keep it lenient.
	case "tool.run":
		if op.ToolID.String() == "" {
			return fmt.Errorf("toolId is required")
		}
		if strings.TrimSpace(op.ActionID) == "" {
			return fmt.Errorf("actionId is required")
		}
		if op.Input == nil {
			return fmt.Errorf("input is required")
		}
		if op.TimeoutMs < 0 {
			return fmt.Errorf("timeoutMs must be >= 0")
		}
	}
	return nil
}
