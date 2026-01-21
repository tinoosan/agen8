package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/validate"
)

// Agent is the minimalist streaming loop: stream the model response, execute its tool calls, and return the final text.
//
// The model should call host primitives (fs.* and tool.run) or discovered tools via function calling.
// Each tool call is executed, its response is appended as a tool message, and the loop continues until
// text arrives without accompanying tool calls (or an explicit final_answer tool completes the turn).
type Agent struct {
	LLM  types.LLMClient
	Exec HostExecutor

	// Model is required. Example: "openai/gpt-4o-mini" (via OpenRouter), etc.
	Model string

	// EnableWebSearch controls whether the agent requests web-search-grounded model variants
	// when supported by the provider (e.g. OpenRouter ":online"). Host controls this.
	EnableWebSearch bool

	// ReasoningEffort is an optional hint for reasoning-capable models.
	ReasoningEffort string

	// ReasoningSummary controls whether and how providers should emit reasoning summaries.
	ReasoningSummary string

	// SystemPrompt is the base system instructions passed to the model.
	SystemPrompt string

	// Context optionally refreshes bounded context (memory/profile/log/etc) per model step.
	Context ContextSource

	// Hooks are optional observability callbacks invoked by the agent loop.
	Hooks Hooks

	// ExtraTools are additional function tools exposed by the host (derived from manifests).
	ExtraTools []types.Tool
	// ToolFunctionRoutes map function names back to tool.run routes.
	ToolFunctionRoutes map[string]ToolRoute
}

// RunConversation executes the agent loop for an existing conversation.
//
// Conversation model:
//   - msgs is the full chat history so far (typically ending with the latest user message).
//   - The agent appends each model-emitted HostOpRequest (as an assistant message) and the
//     corresponding HostOpResponse (as a user message) as the loop proceeds.
//   - When the model returns {"op":"final","text":"..."}, the agent appends that final JSON
//     object as the last assistant message and returns text to the host to display.
func (a *Agent) RunConversation(ctx context.Context, msgs []types.LLMMessage) (final string, updated []types.LLMMessage, steps int, err error) {
	return a.runConversation(ctx, msgs, 1, "")
}

func (a *Agent) runConversation(ctx context.Context, msgs []types.LLMMessage, startStep int, lastResponseID string) (final string, updated []types.LLMMessage, steps int, err error) {
	if a == nil || a.LLM == nil {
		return "", nil, 0, fmt.Errorf("agent LLM is required")
	}
	if a.Exec == nil {
		return "", nil, 0, fmt.Errorf("agent Exec is required")
	}
	if err := validate.NonEmpty("agent Model", a.Model); err != nil {
		return "", nil, 0, err
	}
	if len(msgs) == 0 {
		return "", nil, 0, fmt.Errorf("msgs is required")
	}

	baseSystem := strings.TrimSpace(a.SystemPrompt)
	if baseSystem == "" {
		baseSystem = agentLoopV0SystemPrompt()
	}

	msgs = append([]types.LLMMessage(nil), msgs...)
	if startStep < 1 {
		startStep = 1
	}
	lastResponseID = strings.TrimSpace(lastResponseID)

	turnUserMessage := ""
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if strings.TrimSpace(m.Role) != "user" {
			continue
		}
		c := strings.TrimSpace(m.Content)
		if c == "" {
			continue
		}
		if strings.HasPrefix(c, "HostOpResponse:") {
			continue
		}
		if strings.HasPrefix(c, "Your last message was not valid JSON") || strings.HasPrefix(c, "Your last JSON op was invalid:") {
			continue
		}
		turnUserMessage = c
		break
	}

	hostOpTools := HostOpFunctions()
	if len(a.ExtraTools) != 0 {
		hostOpTools = append(hostOpTools, a.ExtraTools...)
	}

	for step := startStep; ; step++ {

		system := baseSystem
		if a.Context != nil {
			updatedSystem, err := a.Context.SystemPrompt(ctx, baseSystem, step)
			if err != nil {
				return "", nil, 0, err
			}
			system = updatedSystem
		}
		if strings.TrimSpace(turnUserMessage) != "" {
			system = strings.TrimSpace(system) + "\n\n## Current User Request\n\n" + strings.TrimSpace(turnUserMessage) + "\n"
		}

		req := types.LLMRequest{
			Model:              a.Model,
			System:             system,
			Messages:           msgs,
			MaxTokens:          1024,
			Tools:              hostOpTools,
			ToolChoice:         "auto",
			JSONOnly:           false,
			EnableWebSearch:    a.EnableWebSearch,
			PreviousResponseID: lastResponseID,
			ReasoningEffort:    strings.TrimSpace(a.ReasoningEffort),
			ReasoningSummary:   strings.TrimSpace(a.ReasoningSummary),
		}

		resp, err := a.streamToAccumulator(ctx, step, req)
		if err != nil {
			return "", nil, 0, err
		}

		if strings.TrimSpace(resp.ResponseID) != "" {
			lastResponseID = resp.ResponseID
		} else {
			lastResponseID = ""
		}

		if a.Hooks.OnLLMUsage != nil && resp.Usage != nil {
			a.Hooks.OnLLMUsage(step, *resp.Usage)
		}
		if a.Hooks.OnWebSearch != nil && len(resp.Citations) != 0 {
			a.Hooks.OnWebSearch(step, resp.Citations)
		}

		if len(resp.ToolCalls) == 0 {
			finalText := strings.TrimSpace(resp.Text)
			msgs = append(msgs, types.LLMMessage{Role: "assistant", Content: finalText})
			return finalText, msgs, step, nil
		}

		msgs = append(msgs, types.LLMMessage{
			Role:      "assistant",
			Content:   strings.TrimSpace(resp.Text),
			ToolCalls: resp.ToolCalls,
		})

		for _, tc := range resp.ToolCalls {
			which := strings.TrimSpace(tc.Function.Name)

			if which == "final_answer" {
				var args struct {
					Text string `json:"text"`
				}
				dec := json.NewDecoder(strings.NewReader(tc.Function.Arguments))
				if err := dec.Decode(&args); err != nil {
					hostResp := types.HostOpResponse{Op: "final_answer", Ok: false, Error: "final_answer args were not valid JSON: " + err.Error()}
					hostRespJSON, _ := types.MarshalPretty(hostResp)
					msgs = append(msgs, types.LLMMessage{Role: "tool", ToolCallID: strings.TrimSpace(tc.ID), Content: string(hostRespJSON)})
					continue
				}
				finalText := strings.TrimSpace(args.Text)
				if finalText == "" {
					hostResp := types.HostOpResponse{Op: "final_answer", Ok: false, Error: "final_answer.text is required"}
					hostRespJSON, _ := types.MarshalPretty(hostResp)
					msgs = append(msgs, types.LLMMessage{Role: "tool", ToolCallID: strings.TrimSpace(tc.ID), Content: string(hostRespJSON)})
					continue
				}
				hostResp := types.HostOpResponse{Op: "final_answer", Ok: true}
				hostRespJSON, _ := types.MarshalPretty(hostResp)
				msgs = append(msgs, types.LLMMessage{Role: "tool", ToolCallID: strings.TrimSpace(tc.ID), Content: string(hostRespJSON)})
				msgs = append(msgs, types.LLMMessage{Role: "assistant", Content: finalText})
				return finalText, msgs, step, nil
			}

			op, err := functionCallToHostOp(tc, a.ToolFunctionRoutes)
			if err != nil {
				hostResp := types.HostOpResponse{Op: "tool_call", Ok: false, Error: "invalid tool call args: " + err.Error()}
				hostRespJSON, _ := types.MarshalPretty(hostResp)
				msgs = append(msgs, types.LLMMessage{Role: "tool", ToolCallID: strings.TrimSpace(tc.ID), Content: string(hostRespJSON)})
				continue
			}

			hostResp := a.Exec.Exec(ctx, op)
			hostRespJSON, _ := types.MarshalPretty(hostResp)
			msgs = append(msgs, types.LLMMessage{Role: "tool", ToolCallID: strings.TrimSpace(tc.ID), Content: string(hostRespJSON)})
		}

	}
}

func (a *Agent) streamToAccumulator(ctx context.Context, step int, req types.LLMRequest) (types.LLMResponse, error) {
	s, ok := a.LLM.(types.LLMClientStreaming)
	if !ok {
		return types.LLMResponse{}, fmt.Errorf("LLM client does not support streaming")
	}
	dec := &finalTextStreamDecoder{}
	var streamMode string
	var streamPrefix strings.Builder
	const streamPrefixMax = 1024
	emit := func(token string) {
		if token == "" {
			return
		}
		if a.Hooks.OnToken != nil {
			a.Hooks.OnToken(step, token)
		}
	}
	resp, err := s.GenerateStream(ctx, req, func(chunk types.LLMStreamChunk) error {
		if a.Hooks.OnStreamChunk != nil {
			a.Hooks.OnStreamChunk(step, chunk)
		}
		if chunk.Done || chunk.IsReasoning || chunk.Text == "" {
			return nil
		}
		if streamMode == "" {
			if streamPrefix.Len() < streamPrefixMax {
				remain := streamPrefixMax - streamPrefix.Len()
				if len(chunk.Text) > remain {
					streamPrefix.WriteString(chunk.Text[:remain])
				} else {
					streamPrefix.WriteString(chunk.Text)
				}
			}
			buf := streamPrefix.String()
			first := byte(0)
			for i := 0; i < len(buf); i++ {
				switch buf[i] {
				case ' ', '\t', '\r', '\n':
					continue
				default:
					first = buf[i]
				}
				if first != 0 {
					break
				}
			}
			if first == 0 && streamPrefix.Len() < streamPrefixMax {
				return nil
			}
			if first == '{' {
				streamMode = "json"
			} else {
				streamMode = "raw"
			}
			prefix := streamPrefix.String()
			streamPrefix.Reset()
			if streamMode == "raw" {
				emit(prefix)
				return nil
			}
			out := dec.Consume(prefix)
			if out != "" {
				emit(out)
			}
			return nil
		}
		if streamMode == "raw" {
			emit(chunk.Text)
			return nil
		}
		out := dec.Consume(chunk.Text)
		if out != "" {
			emit(out)
		}
		return nil
	})
	return resp, err
}

func functionCallToHostOp(tc types.ToolCall, routes map[string]ToolRoute) (types.HostOpRequest, error) {
	name := strings.TrimSpace(tc.Function.Name)
	argsJSON := []byte(tc.Function.Arguments)

	if route, ok := routes[name]; ok {
		if len(strings.TrimSpace(tc.Function.Arguments)) == 0 {
			argsJSON = []byte(`{}`)
		}
		var input json.RawMessage
		if err := json.Unmarshal(argsJSON, &input); err != nil {
			return types.HostOpRequest{}, err
		}
		if input == nil {
			input = json.RawMessage(`{}`)
		}
		timeout := route.TimeoutMs
		if timeout <= 0 {
			timeout = defaultToolFunctionTimeoutMs
		}
		return types.HostOpRequest{
			Op:        types.HostOpToolRun,
			ToolID:    route.ToolID,
			ActionID:  strings.TrimSpace(route.ActionID),
			Input:     input,
			TimeoutMs: timeout,
		}, nil
	}

	switch name {
	case "builtin_shell_exec":
		var args struct {
			Argv  []string `json:"argv"`
			Cwd   string   `json:"cwd"`
			Stdin string   `json:"stdin"`
		}
		if err := json.Unmarshal(argsJSON, &args); err != nil {
			return types.HostOpRequest{}, err
		}
		if len(args.Argv) == 0 {
			return types.HostOpRequest{}, fmt.Errorf("argv is required")
		}
		inputMap := map[string]any{"argv": args.Argv}
		if strings.TrimSpace(args.Cwd) != "" {
			inputMap["cwd"] = resolveVFSPath(args.Cwd)
		}
		if args.Stdin != "" {
			inputMap["stdin"] = args.Stdin
		}
		inp, err := json.Marshal(inputMap)
		if err != nil {
			return types.HostOpRequest{}, err
		}
		return types.HostOpRequest{
			Op:       types.HostOpToolRun,
			ToolID:   types.ToolID("builtin.shell"),
			ActionID: "exec",
			Input:    inp,
		}, nil

	case "builtin_http_fetch":
		var args struct {
			URL             string            `json:"url"`
			Method          string            `json:"method"`
			Headers         map[string]string `json:"headers"`
			Body            string            `json:"body"`
			MaxBytes        *int              `json:"maxBytes"`
			FollowRedirects *bool             `json:"followRedirects"`
		}
		if err := json.Unmarshal(argsJSON, &args); err != nil {
			return types.HostOpRequest{}, err
		}
		inputMap := map[string]any{"url": args.URL}
		if strings.TrimSpace(args.Method) != "" {
			inputMap["method"] = args.Method
		}
		if args.Headers != nil {
			inputMap["headers"] = args.Headers
		}
		if args.Body != "" {
			inputMap["body"] = args.Body
		}
		if args.MaxBytes != nil {
			inputMap["maxBytes"] = *args.MaxBytes
		}
		if args.FollowRedirects != nil {
			inputMap["followRedirects"] = *args.FollowRedirects
		}
		inp, err := json.Marshal(inputMap)
		if err != nil {
			return types.HostOpRequest{}, err
		}
		return types.HostOpRequest{
			Op:       types.HostOpToolRun,
			ToolID:   types.ToolID("builtin.http"),
			ActionID: "fetch",
			Input:    inp,
		}, nil

	case "fs_list":
		var args struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(argsJSON, &args); err != nil {
			return types.HostOpRequest{}, err
		}
		return types.HostOpRequest{Op: types.HostOpFSList, Path: resolveVFSPath(args.Path)}, nil

	case "fs_read":
		var args struct {
			Path     string `json:"path"`
			MaxBytes *int   `json:"maxBytes"`
		}
		if err := json.Unmarshal(argsJSON, &args); err != nil {
			return types.HostOpRequest{}, err
		}
		// Default to 1MB to avoid truncation in standard workflows
		maxBytes := 1024 * 1024
		if args.MaxBytes != nil {
			maxBytes = *args.MaxBytes
		}
		return types.HostOpRequest{Op: types.HostOpFSRead, Path: resolveVFSPath(args.Path), MaxBytes: maxBytes}, nil

	case "fs_write":
		var args struct {
			Path string `json:"path"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(argsJSON, &args); err != nil {
			return types.HostOpRequest{}, err
		}
		return types.HostOpRequest{Op: types.HostOpFSWrite, Path: resolveVFSPath(args.Path), Text: args.Text}, nil

	case "fs_append":
		var args struct {
			Path string `json:"path"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(argsJSON, &args); err != nil {
			return types.HostOpRequest{}, err
		}
		return types.HostOpRequest{Op: types.HostOpFSAppend, Path: resolveVFSPath(args.Path), Text: args.Text}, nil

	case "fs_edit":
		var args struct {
			Path  string `json:"path"`
			Edits []struct {
				Old        string `json:"old"`
				New        string `json:"new"`
				Occurrence int    `json:"occurrence"`
			} `json:"edits"`
		}
		if err := json.Unmarshal(argsJSON, &args); err != nil {
			return types.HostOpRequest{}, err
		}
		inp, _ := json.Marshal(map[string]any{"edits": args.Edits})
		return types.HostOpRequest{Op: types.HostOpFSEdit, Path: resolveVFSPath(args.Path), Input: inp}, nil

	case "fs_patch":
		var args struct {
			Path string `json:"path"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(argsJSON, &args); err != nil {
			return types.HostOpRequest{}, err
		}
		return types.HostOpRequest{Op: types.HostOpFSPatch, Path: resolveVFSPath(args.Path), Text: args.Text}, nil

	case "tool_run":
		var args struct {
			ToolID    string          `json:"toolId"`
			ActionID  string          `json:"actionId"`
			Input     json.RawMessage `json:"input"`
			TimeoutMs *int            `json:"timeoutMs"`
		}
		if err := json.Unmarshal(argsJSON, &args); err != nil {
			return types.HostOpRequest{}, err
		}
		// H3 fix: some models omit input entirely; treat missing input as empty object.
		if args.Input == nil {
			args.Input = json.RawMessage(`{}`)
		}
		var inputMap map[string]json.RawMessage
		if err := json.Unmarshal(args.Input, &inputMap); err == nil {
			if cwdRaw, ok := inputMap["cwd"]; ok {
				var cwd string
				if err := json.Unmarshal(cwdRaw, &cwd); err == nil {
					inputMap["cwd"] = json.RawMessage(strconv.Quote(resolveVFSPath(cwd)))
					if updated, err := json.Marshal(inputMap); err == nil {
						args.Input = updated
					}
				}
			}
		}
		timeout := 0
		if args.TimeoutMs != nil {
			timeout = *args.TimeoutMs
		}
		return types.HostOpRequest{
			Op:        types.HostOpToolRun,
			ToolID:    types.ToolID(strings.TrimSpace(args.ToolID)),
			ActionID:  strings.TrimSpace(args.ActionID),
			Input:     args.Input,
			TimeoutMs: timeout,
		}, nil

	default:
		return types.HostOpRequest{}, fmt.Errorf("unknown tool function %q", name)
	}
}

func resolveVFSPath(p string) string {
	pathStr := strings.TrimSpace(p)
	if pathStr == "" {
		return "/project"
	}
	if strings.HasPrefix(pathStr, "/") {
		return pathStr
	}
	cleaned := path.Clean(pathStr)
	joined := path.Join("/project", cleaned)
	if !strings.HasPrefix(joined, "/project") {
		return "/project"
	}
	return joined
}

// Run executes the agent loop for a single user goal and returns the final response text.
func (a *Agent) Run(ctx context.Context, goal string) (string, error) {
	if err := validate.NonEmpty("goal", goal); err != nil {
		return "", err
	}
	final, _, _, err := a.RunConversation(ctx, []types.LLMMessage{
		{Role: "user", Content: goal},
	})
	return final, err
}

func agentLoopV0SystemPrompt() string {
	raw := `
# Workbench Agent

You are an agent inside **Workbench**, a coding environment with a virtual filesystem (VFS).

## Critical: Tool Results Are YOUR Output

When you call a tool (like ~fs_read~), the content that comes back is **the result of YOUR action** — not something the user sent you. If you read a file and see its contents, YOU retrieved it. Do not say "thanks for sharing" or treat tool output as user-provided content.

## Your Tools (Two Categories)

### 1. Direct Host Operations (Use Immediately)

Call these without discovery:

- ~fs_list~, ~fs_read~, ~fs_write~, ~fs_append~, ~fs_edit~, ~fs_patch~, ~final_answer~
- ~builtin_shell_exec~ for shell argv execution inside the repo root (cwd, stdin allowed)
- ~builtin_http_fetch~ for HTTP requests

**For simple tasks like "create 5 files", just call ~fs_write~ directly.**

### 2. External Tools (Require Discovery)

Use ~tool_run~ to invoke tools under ~/tools~ that are NOT in the direct list above:

1. ~fs_read("/tools/<toolId>")~ → read the manifest, learn required input fields
2. ~tool_run(toolId, actionId, input, timeoutMs)~ → call with correct input

**Only use ~tool_run~ when you need capabilities beyond the direct list** (custom/disk tools).

---

## Web Search + Citations

Workbench may provide **web-search-grounded model responses** (provider-dependent).

- Web search is **disabled by default**. The user may enable it via the host command ~/web~.
- If you use information from web search, you **must include citations** in your final response.
- Prefer a short ~Sources:~ section with 1–5 links at the end.

---

## VFS Structure

| Path                | What It Is                                             |
| ------------------- | ------------------------------------------------------ |
| ~/project~          | **User's actual project** — start here for their files |
| ~/scratch~          | Your temporary workspace (run-scoped)                  |
| ~/log~              | This run's event log                                   |
| ~/memory~           | Run-scoped notes                                       |
| ~/history~          | Session-scoped event stream (read-only)                |
| ~/results/<callId>~ | Tool output artifacts                                  |

---

## Key Rules

1.  **Stop Rule**: Call ~final_answer~ ONLY when you have fully completed the user's overarching goal or task chain; plain assistant text without further tool calls is treated as the final response once you are done. Do not stop early just because you have some info; ensure the full request is satisfied.
2.  **Path Resolution**: You may use relative paths (e.g., ~.~ or ~./src~). They will be resolved relative to ~/project~.
3.  **Tool Usage**:
    - Use ~fs_*~ tools for file operations.
    - Use ~builtin_shell_exec~ for advanced search (~grep~, ~find~) or running project binaries. Note: Do NOT try to run ~bash~ or ~sh~ interactive sessions; just pass the command argv directly.
4.  **No Hallucinations**: Do not call tools that are not in your definition list.

---

## fs_edit Details

For surgical edits:

~~~json
{
  "path": "/project/file.txt",
  "edits": [{ "old": "foo", "new": "bar", "occurrence": 1 }]
}
~~~

- ~old~: exact text to find
- ~new~: replacement text
- ~occurrence~: 1-based (which match to replace)

If edit fails, ~fs_read~ the file, pick a more specific ~old~ snippet, retry.

---

## fs_patch Details

Apply a unified diff:

~~~diff
--- a/file.txt
+++ b/file.txt
@@ -1,3 +1,3 @@
 context
-old line
+new line
 context
~~~

Hunk headers must include line ranges: ~@@ -1,3 +1,3 @@~ (not just ~@@~).

---

## Memory

Write durable lessons to ~/memory/update.md~:

- Short bullet list: ~- RULE: prefer fs_edit for small changes~
- Or key/value: ~preferred_editor: vim~

---

## Operating Principles

- **Action-first**: do the minimal ops to complete the task
- **Recover gracefully**: if an op fails, read the file and retry with adjusted input
- **Prefer direct ops**: use ~fs_write~/~fs_read~ before reaching for ~tool_run~
`
	return strings.TrimSpace(strings.ReplaceAll(raw, "~", "`"))
}
