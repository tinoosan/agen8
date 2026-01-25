package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/pkg/llm"
	"github.com/tinoosan/workbench-core/pkg/validate"
	"github.com/tinoosan/workbench-core/pkg/tools"
)

// Agent is the minimalist streaming loop: stream the model response, execute its tool calls, and return the final text.
//
// The model should call host primitives (fs.* and tool.run) or discovered tools via function calling.
// Each tool call is executed, its response is appended as a tool message, and the loop continues until
// text arrives without accompanying tool calls (or an explicit final_answer tool completes the turn).
type Agent struct {
	LLM  llm.LLMClient
	Exec HostExecutor

	// Model is required. Example: "openai/gpt-4o-mini" (via OpenRouter), etc.
	Model string

	// EnableWebSearch controls whether the agent requests web-search-grounded model variants
	// when supported by the provider (e.g. OpenRouter ":online"). Host controls this.
	EnableWebSearch bool

	// PlanMode enforces the structured planning policy for the first step.
	PlanMode bool

	// ApprovalsMode controls whether dangerous host ops pause for approval.
	ApprovalsMode string

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

	// MaxTokens is the maximum output tokens per turn. If 0, the LLM client's default is used.
	MaxTokens int

	// ExtraTools are additional function tools exposed by the host (derived from manifests).
	ExtraTools []llm.Tool
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
func (a *Agent) RunConversation(ctx context.Context, msgs []llm.LLMMessage) (final string, updated []llm.LLMMessage, steps int, err error) {
	return a.runConversation(ctx, msgs, 1)
}

func (a *Agent) runConversation(ctx context.Context, msgs []llm.LLMMessage, startStep int) (final string, updated []llm.LLMMessage, steps int, err error) {
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

	msgs = append([]llm.LLMMessage(nil), msgs...)
	if startStep < 1 {
		startStep = 1
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
		if a.PlanMode {
			system = strings.TrimSpace(system + "\n\n" + planModePolicyText)
		}

		toolChoice := "auto"

		req := llm.LLMRequest{
			Model:            a.Model,
			System:           system,
			Messages:         msgs,
			MaxTokens:        a.MaxTokens,
			Tools:            hostOpTools,
			ToolChoice:       toolChoice,
			JSONOnly:         false,
			EnableWebSearch:  a.EnableWebSearch,
			ReasoningEffort:  strings.TrimSpace(a.ReasoningEffort),
			ReasoningSummary: strings.TrimSpace(a.ReasoningSummary),
		}

		resp, err := a.streamToAccumulator(ctx, step, req)
		if err != nil {
			return "", nil, 0, err
		}

		if a.Hooks.OnLLMUsage != nil && resp.Usage != nil {
			a.Hooks.OnLLMUsage(step, *resp.Usage)
		}
		if a.Hooks.OnWebSearch != nil && len(resp.Citations) != 0 {
			a.Hooks.OnWebSearch(step, resp.Citations)
		}

		if len(resp.ToolCalls) == 0 {
			finalText := strings.TrimSpace(resp.Text)
			msgs = append(msgs, llm.LLMMessage{Role: "assistant", Content: finalText})
			return finalText, msgs, step, nil
		}

		assistantMsg := llm.LLMMessage{
			Role:      "assistant",
			Content:   strings.TrimSpace(resp.Text),
			ToolCalls: resp.ToolCalls,
		}
		msgs = append(msgs, assistantMsg)

		type pendingHostOp struct {
			req    types.HostOpRequest
			callID string
		}
		pending := make([]pendingHostOp, 0, len(resp.ToolCalls))

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
					msgs = append(msgs, llm.LLMMessage{Role: "tool", ToolCallID: strings.TrimSpace(tc.ID), Content: string(hostRespJSON)})
					continue
				}
				finalText := strings.TrimSpace(args.Text)
				if finalText == "" {
					hostResp := types.HostOpResponse{Op: "final_answer", Ok: false, Error: "final_answer.text is required"}
					hostRespJSON, _ := types.MarshalPretty(hostResp)
					msgs = append(msgs, llm.LLMMessage{Role: "tool", ToolCallID: strings.TrimSpace(tc.ID), Content: string(hostRespJSON)})
					continue
				}
				hostResp := types.HostOpResponse{Op: "final_answer", Ok: true}
				hostRespJSON, _ := types.MarshalPretty(hostResp)
				msgs = append(msgs, llm.LLMMessage{Role: "tool", ToolCallID: strings.TrimSpace(tc.ID), Content: string(hostRespJSON)})
				msgs = append(msgs, llm.LLMMessage{Role: "assistant", Content: finalText})
				return finalText, msgs, step, nil
			}

			op, err := functionCallToHostOp(tc, a.ToolFunctionRoutes)
			if err != nil {
				hostResp := types.HostOpResponse{Op: "tool_call", Ok: false, Error: "invalid tool call args: " + err.Error()}
				hostRespJSON, _ := types.MarshalPretty(hostResp)
				msgs = append(msgs, llm.LLMMessage{Role: "tool", ToolCallID: strings.TrimSpace(tc.ID), Content: string(hostRespJSON)})
				continue
			}
			pending = append(pending, pendingHostOp{req: op, callID: strings.TrimSpace(tc.ID)})
		}

		if a.ApprovalsMode == "enabled" && len(pending) > 0 {
			need := false
			for _, item := range pending {
				if isDangerousHostOp(item.req) {
					need = true
					break
				}
			}
			if need {
				reqs := make([]types.HostOpRequest, len(pending))
				ids := make([]string, len(pending))
				for i, item := range pending {
					reqs[i] = item.req
					ids[i] = item.callID
				}
				return "", msgs, step, ErrApprovalRequired{
					AssistantMsg:       assistantMsg,
					PendingOps:         reqs,
					PendingToolCallIDs: ids,
				}
			}
		}

		for _, item := range pending {
			hostResp := a.Exec.Exec(ctx, item.req)
			hostRespJSON, _ := types.MarshalPretty(hostResp)
			msgs = append(msgs, llm.LLMMessage{Role: "tool", ToolCallID: item.callID, Content: string(hostRespJSON)})
		}

	}
}

func (a *Agent) streamToAccumulator(ctx context.Context, step int, req llm.LLMRequest) (llm.LLMResponse, error) {
	s, ok := a.LLM.(llm.LLMClientStreaming)
	if !ok {
		return llm.LLMResponse{}, fmt.Errorf("LLM client does not support streaming")
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
	resp, err := s.GenerateStream(ctx, req, func(chunk llm.LLMStreamChunk) error {
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

func functionCallToHostOp(tc llm.ToolCall, routes map[string]ToolRoute) (types.HostOpRequest, error) {
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
	case "shell_exec":
		var args struct {
			Command string `json:"command"`
			Cwd     string `json:"cwd"`
			Stdin   string `json:"stdin"`
		}
		if err := json.Unmarshal(argsJSON, &args); err != nil {
			return types.HostOpRequest{}, err
		}
		cmd := strings.TrimSpace(args.Command)
		if cmd == "" {
			return types.HostOpRequest{}, fmt.Errorf("command is required")
		}
		// Cwd is passed as-is (relative to project root, or empty for root)
		// The shell invoker handles validation.
		cwd := strings.TrimSpace(args.Cwd)
		if cwd == "" {
			cwd = "."
		}
		// Wrap with bash -c for full shell syntax support (pipes, redirects, etc.)
		return types.HostOpRequest{
			Op:    types.HostOpShellExec,
			Argv:  []string{"bash", "-c", cmd},
			Cwd:   cwd,
			Stdin: args.Stdin,
		}, nil

	case "http_fetch":
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
		url := strings.TrimSpace(args.URL)
		if url == "" {
			return types.HostOpRequest{}, fmt.Errorf("url is required")
		}
		req := types.HostOpRequest{
			Op:     types.HostOpHTTPFetch,
			URL:    url,
			Method: strings.TrimSpace(args.Method),
		}
		if args.Headers != nil {
			req.Headers = args.Headers
		}
		if strings.TrimSpace(args.Body) != "" {
			req.Body = args.Body
		}
		if args.MaxBytes != nil {
			req.MaxBytes = *args.MaxBytes
		}
		if args.FollowRedirects != nil {
			req.FollowRedirects = args.FollowRedirects
		}
		return req, nil

	case "trace_events_since":
		var args struct {
			Cursor   json.RawMessage `json:"cursor"`
			MaxBytes *int            `json:"maxBytes"`
			Limit    *int            `json:"limit"`
		}
		if err := json.Unmarshal(argsJSON, &args); err != nil {
			return types.HostOpRequest{}, err
		}
		return types.HostOpRequest{
			Op:     types.HostOpTrace,
			Action: "events.since",
			Input:  argsJSON,
		}, nil

	case "trace_events_latest":
		var args struct {
			MaxBytes *int `json:"maxBytes"`
			Limit    *int `json:"limit"`
		}
		if err := json.Unmarshal(argsJSON, &args); err != nil {
			return types.HostOpRequest{}, err
		}
		return types.HostOpRequest{
			Op:     types.HostOpTrace,
			Action: "events.latest",
			Input:  argsJSON,
		}, nil

	case "trace_events_summary":
		var args struct {
			Cursor       json.RawMessage `json:"cursor"`
			MaxBytes     *int            `json:"maxBytes"`
			Limit        *int            `json:"limit"`
			IncludeTypes []string        `json:"includeTypes"`
		}
		if err := json.Unmarshal(argsJSON, &args); err != nil {
			return types.HostOpRequest{}, err
		}
		return types.HostOpRequest{
			Op:     types.HostOpTrace,
			Action: "events.summary",
			Input:  argsJSON,
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

	case "update_plan":
		var args struct {
			Plan string `json:"plan"`
		}
		if err := json.Unmarshal(argsJSON, &args); err != nil {
			return types.HostOpRequest{}, err
		}
		return types.HostOpRequest{
			Op:   types.HostOpFSWrite,
			Path: "/plan/HEAD.md",
			Text: args.Plan,
		}, nil

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
			ToolID:    tools.ToolID(strings.TrimSpace(args.ToolID)),
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

func isDangerousHostOp(req types.HostOpRequest) bool {
	op := strings.ToLower(strings.TrimSpace(req.Op))
	if op == types.HostOpFSWrite && strings.TrimSpace(req.Path) == "/plan/HEAD.md" {
		return false
	}
	switch op {
	case types.HostOpFSWrite,
		types.HostOpFSAppend,
		types.HostOpFSEdit,
		types.HostOpFSPatch,
		types.HostOpShellExec,
		types.HostOpHTTPFetch,
		types.HostOpToolRun:
		return true
	default:
		return false
	}
}

// Run executes the agent loop for a single user goal and returns the final response text.
func (a *Agent) Run(ctx context.Context, goal string) (string, error) {
	if err := validate.NonEmpty("goal", goal); err != nil {
		return "", err
	}
	final, _, _, err := a.RunConversation(ctx, []llm.LLMMessage{
		{Role: "user", Content: goal},
	})
	return final, err
}

func agentLoopV0SystemPrompt() string {
	raw := `<system>
  <identity>You are an agent inside Workbench, an environment with a virtual filesystem (VFS) and host-managed tools; tool output is your own action.</identity>
  <critical_rules>
    <planning>
      <rule id="planning">
        COMPLEX TASKS REQUIRE A PLAN.
        1. INITIALIZATION: If the user request implies multiple steps, create a Markdown checklist at "/plan/HEAD.md" before any fs_write/fs_shell/fun calls.
        2. FORMAT: The checklist must use "- [ ]" / "- [x]" tokens for each actionable step (e.g., "- [ ] Analyze requirements", "- [ ] Implement feature", "- [ ] Verify results").
        3. NARRATIVE: Keep narrative planning out of the checklist file. Use your response text for any prose planning; the checklist remains the single source of truth.
        4. GATE: Without a checklist at "/plan/HEAD.md", do not execute side-effect tools (fs_write, shell_exec, etc.).
        5. EXECUTION: After each step completes, overwrite "/plan/HEAD.md" with the updated checklist, marking done items with "- [x]".
        6. CONTINUOUS: Before starting a new step, re-read the checklist, ensure the next item is accurate, and update it if needed.
        7. ADAPTATION: If the plan evolves, immediately rewrite "/plan/HEAD.md" so the checklist remains the single source of truth.
        7. SKIP: Do NOT create a plan for greetings/smalltalk, single factual questions, or single small edits. Respond directly instead.
      </rule>
      <rule id="planning.externalize">Plans must live in "/plan/HEAD.md" (checklist); don’t keep plan reasoning solely in your head.</rule>
      <rule id="planning.visibility">Whenever asked about planning, point to "/plan/HEAD.md" for the checklist—the mount is always available via fs_list.</rule>
    </planning>
    <rule id="tool_results">Tool results are YOUR output, not user input.</rule>
    <rule id="skills_vs_tools">Skills live under /skills (see SKILL.md) and are not tools; plugins belong to /tools.</rule>
    <rule id="skills_first">If the user mentions skill(s), ALWAYS check /skills before /tools.</rule>
  </critical_rules>
  <capabilities>
    <direct_ops>
      <op name="fs_list">List VFS paths.</op>
      <op name="fs_read">Read file contents.</op>
      <op name="fs_write">Write new files.</op>
      <op name="fs_append">Append to files.</op>
      <op name="fs_edit">Make precise edits via JSON diffs.</op>
      <op name="fs_patch">Apply unified-diff patches.</op>
      <op name="final_answer">Emit the final response once the user's goal is complete.</op>
      <op name="shell_exec">Run shell commands (pipes, redirects, etc.).</op>
      <op name="http_fetch">Make HTTP requests.</op>
      <op name="trace_events_latest">Read the latest trace events.</op>
      <op name="trace_events_since">Stream trace events since a cursor.</op>
      <op name="trace_events_summary">Summarize trace events.</op>
    </direct_ops>
    <skills>Refer to the <available_skills> block below and fs_read /skills/<skill>/SKILL.md to follow documented workflows.</skills>
    <planning>For multi-step work, write the checklist to /plan/HEAD.md (update_plan). Keep it current: re-read before each step, mark completed items with "- [x]", and add/adjust items as work changes. Skip planning for greetings/smalltalk, single factual questions, or single small edits.</planning>
    <external_tools>Use tool_run only after inspecting /tools/<toolId> manifests; prefer direct ops, skills, and /plan first.</external_tools>
  </capabilities>
  <vfs>
    <mount path="/project">User's actual project files.</mount>
    <mount path="/scratch">Temporary workspace for this run.</mount>
    <mount path="/log">Event log for this turn.</mount>
    <mount path="/memory">Run-scoped working memory.</mount>
    <mount path="/skills">These are YOUR skills. ALWAYS check /skills before /tools (SKILL.md).</mount>
    <mount path="/plan">Planning workspace for complex tasks. /plan/HEAD.md is the checklist.</mount>
    <mount path="/history">Session-scoped history (read-only).</mount>
    <mount path="/results/&lt;callId&gt;">Tool output artifacts.</mount>
  </vfs>
  <skill_creation>
    You can create reusable skills when you notice repeatable patterns. Write a SKILL.md file using YAML front matter (name & description) followed by markdown instructions and supporting sections.
    1. Start a skill by writing the SKILL.md file:
       fs.write("/skills/my-skill/SKILL.md", "---\nname: My Skill\ndescription: Brief summary\n---\n# Instructions\nDescribe when and how to run this skill.\n")
    2. Update or extend a skill with fs.append when needed, or add optional helpers:
       - scripts/: executable helpers
       - examples/: reference implementations
       - resources/: templates or assets
    Skills appear in <available_skills> after creation and you can inspect /skills/skill-template/SKILL.md for a starter layout.
  </skill_creation>
  <operating_rules>
    <rule id="stop">Call final_answer only once the overarching goal is complete; plain assistant text without tool calls is treated as final output when finished.</rule>
    <rule id="path_resolution">Shell commands should use relative paths (e.g. ./src) with the project root as cwd; fs_* tools still expect absolute VFS paths.</rule>
    <rule id="tool_usage">Use fs_* for file operations, shell_exec for shell commands, http_fetch for HTTP, and trace event helpers for diagnostics; do not invent other tools.</rule>
    <rule id="handling_denials">If a tool execution returns error code "command_rejected", it means the user declined that specific action. Do not apologize excessively. Acknowledge the denial and immediately propose the next logical step (e.g., "Skipping ls", "Shall I try reading specific files instead?", or "Please provide the content of X manually").</rule>
    <rule id="web_search">Web search is disabled by default; the user can enable it via /web. If you consult search results, include citations and a Sources: list (1–5 links) in your final answer.</rule>
    <rule id="fs_edit">fs_edit expects JSON like {"path": "/project/file", "edits": [{"old": "...", "new": "...", "occurrence": 1}]}; if it fails, re-read the file and try a more specific snippet.</rule>
    <rule id="fs_patch">fs_patch needs a unified diff with hunk headers (e.g., @@ -1,3 +1,3 @@) or adjust until the patch applies cleanly.</rule>
    <rule id="memory">Write durable lessons to /memory/update.md as a short bullet list or key/value pair.</rule>
    <rule id="principles">Action-first, recover gracefully, assume defaults, prefer direct ops, and never hallucinate; always deliver a final response when work is complete.</rule>
    <rule id="skills_block">When asked about "<available_skills>", inspect the <available_skills> section injected below, and respond by describing each skill's name, description, and location instead of repeating the built-in host-capabilities list.</rule>
  </operating_rules>`
	return strings.TrimSpace(raw)
}

const planModePolicyText = `<plan_mode>
For multi-step work, write the authoritative checklist to /plan/HEAD.md (update_plan). Re-read the checklist before each step and update it after each step so progress is always accurate. If steps change, rewrite /plan/HEAD.md immediately. Keep the checklist short and actionable. Skip planning for greetings/smalltalk, single factual questions, or single small edits.
</plan_mode>`
