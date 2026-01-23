package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/tinoosan/workbench-core/internal/types"
)

type fakeExec struct {
	reqs []types.HostOpRequest
}

func (f *fakeExec) Exec(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
	f.reqs = append(f.reqs, req)
	return types.HostOpResponse{Op: req.Op, Ok: true, Text: "result"}
}

type fakeStreamingLLM struct {
	Responses []types.LLMResponse
	idx       int
}

func (f *fakeStreamingLLM) nextResponse() types.LLMResponse {
	if len(f.Responses) == 0 {
		return types.LLMResponse{}
	}
	if f.idx >= len(f.Responses) {
		return f.Responses[len(f.Responses)-1]
	}
	resp := f.Responses[f.idx]
	f.idx++
	return resp
}

func (f *fakeStreamingLLM) Generate(ctx context.Context, req types.LLMRequest) (types.LLMResponse, error) {
	return f.nextResponse(), nil
}

func (f *fakeStreamingLLM) GenerateStream(ctx context.Context, req types.LLMRequest, cb types.LLMStreamCallback) (types.LLMResponse, error) {
	resp := f.nextResponse()
	if cb != nil && resp.Text != "" {
		_ = cb(types.LLMStreamChunk{Text: resp.Text})
	}
	return resp, nil
}

func TestAgentLoopV0_ResolvesRelativePaths(t *testing.T) {
	tc := types.ToolCall{
		Type: "function",
		Function: types.ToolCallFunction{
			Name:      "fs_read",
			Arguments: `{"path":"docs/example.txt"}`,
		},
	}

	req, err := functionCallToHostOp(tc, nil)
	if err != nil {
		t.Fatalf("functionCallToHostOp: %v", err)
	}
	if req.Op != types.HostOpFSRead {
		t.Fatalf("expected fs.read, got %q", req.Op)
	}
	if req.Path != "/project/docs/example.txt" {
		t.Fatalf("expected /project/docs/example.txt, got %q", req.Path)
	}

	tc = types.ToolCall{
		Type: "function",
		Function: types.ToolCallFunction{
			Name:      "tool_run",
			Arguments: `{"toolId":"builtin.shell","actionId":"exec","input":{"cwd":"notes"}}`,
		},
	}
	req, err = functionCallToHostOp(tc, nil)
	if err != nil {
		t.Fatalf("functionCallToHostOp tool_run: %v", err)
	}
	var input map[string]string
	if err := json.Unmarshal(req.Input, &input); err != nil {
		t.Fatalf("unmarshal input: %v", err)
	}
	if input["cwd"] != "/project/notes" {
		t.Fatalf("expected cwd /project/notes, got %q", input["cwd"])
	}
}

func TestAgentLoopV0_ShellAndHTTPHostOps(t *testing.T) {
	tc := types.ToolCall{
		Type: "function",
		Function: types.ToolCallFunction{
			Name:      "shell_exec",
			Arguments: `{"command":"ls","cwd":"src","stdin":"hi"}`,
		},
	}
	req, err := functionCallToHostOp(tc, nil)
	if err != nil {
		t.Fatalf("shell_exec: %v", err)
	}
	if req.Op != types.HostOpShellExec {
		t.Fatalf("expected shell_exec host op, got %+v", req)
	}
	if req.Cwd != "src" {
		t.Fatalf("expected cwd src, got %q", req.Cwd)
	}

	tc = types.ToolCall{
		Type: "function",
		Function: types.ToolCallFunction{
			Name:      "http_fetch",
			Arguments: `{"url":"https://example.com","method":"GET"}`,
		},
	}
	req, err = functionCallToHostOp(tc, nil)
	if err != nil {
		t.Fatalf("http_fetch: %v", err)
	}
	if req.Op != types.HostOpHTTPFetch {
		t.Fatalf("expected http_fetch host op, got %+v", req)
	}
	if req.URL != "https://example.com" || req.Method != "GET" {
		t.Fatalf("expected https GET, got %#v", req)
	}

	tc = types.ToolCall{
		Type: "function",
		Function: types.ToolCallFunction{
			Name:      "trace_events_latest",
			Arguments: `{"limit":3}`,
		},
	}
	req, err = functionCallToHostOp(tc, nil)
	if err != nil {
		t.Fatalf("trace: %v", err)
	}
	if req.Op != types.HostOpTrace {
		t.Fatalf("expected trace host op, got %+v", req)
	}
	if req.Action != "events.latest" {
		t.Fatalf("unexpected trace action: %+v", req)
	}
	var traceInput struct {
		Limit *int `json:"limit"`
	}
	if err := json.Unmarshal(req.Input, &traceInput); err != nil {
		t.Fatalf("unmarshal trace input: %v", err)
	}
	if traceInput.Limit == nil || *traceInput.Limit != 3 {
		t.Fatalf("unexpected trace limit: %+v", traceInput.Limit)
	}
}

func TestAgentLoop_RunConversation_FinalText(t *testing.T) {
	llm := &fakeStreamingLLM{
		Responses: []types.LLMResponse{{Text: "final result"}},
	}
	exec := &fakeExec{}
	agent := &Agent{LLM: llm, Exec: exec, Model: "test"}
	final, msgs, steps, err := agent.RunConversation(context.Background(), []types.LLMMessage{{Role: "user", Content: "do it"}})
	if err != nil {
		t.Fatalf("RunConversation: %v", err)
	}
	if final != "final result" {
		t.Fatalf("expected final text, got %q", final)
	}
	if steps != 1 {
		t.Fatalf("expected 1 step, got %d", steps)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
}

func TestAgentLoop_RunConversation_ExecutesTool(t *testing.T) {
	llm := &fakeStreamingLLM{
		Responses: []types.LLMResponse{
			{
				ToolCalls: []types.ToolCall{{
					ID: "1",
					Function: types.ToolCallFunction{
						Name:      "fs_read",
						Arguments: `{"path":"/project/file.txt"}`,
					},
				}},
			},
			{Text: "done"},
		},
	}
	exec := &fakeExec{}
	agent := &Agent{LLM: llm, Exec: exec, Model: "test"}
	final, _, _, err := agent.RunConversation(context.Background(), []types.LLMMessage{{Role: "user", Content: "read"}})
	if err != nil {
		t.Fatalf("RunConversation: %v", err)
	}
	if final != "done" {
		t.Fatalf("expected final text, got %q", final)
	}
	if len(exec.reqs) != 1 {
		t.Fatalf("expected one host op, got %d", len(exec.reqs))
	}
	if exec.reqs[0].Op != types.HostOpFSRead || exec.reqs[0].Path != "/project/file.txt" {
		t.Fatalf("unexpected host op: %+v", exec.reqs[0])
	}
}

func TestAgentLoop_RunConversation_FinalAnswerTool(t *testing.T) {
	llm := &fakeStreamingLLM{
		Responses: []types.LLMResponse{{
			ToolCalls: []types.ToolCall{{
				ID: "final",
				Function: types.ToolCallFunction{
					Name:      "final_answer",
					Arguments: `{"text":"All done"}`,
				},
			}},
			Text: "finishing",
		}},
	}
	exec := &fakeExec{}
	agent := &Agent{LLM: llm, Exec: exec, Model: "test"}
	final, _, _, err := agent.RunConversation(context.Background(), []types.LLMMessage{{Role: "user", Content: "finish"}})
	if err != nil {
		t.Fatalf("RunConversation: %v", err)
	}
	if final != "All done" {
		t.Fatalf("expected final_answer payload, got %q", final)
	}
	if len(exec.reqs) != 0 {
		t.Fatalf("expected no host ops, got %d", len(exec.reqs))
	}
}
func TestAgentLoop_RunConversation_RequiresApproval(t *testing.T) {
	llm := &fakeStreamingLLM{
		Responses: []types.LLMResponse{{
			ToolCalls: []types.ToolCall{{
				ID: "1",
				Function: types.ToolCallFunction{
					Name:      "fs_write",
					Arguments: `{"path":"/project/secret.txt","text":"oops"}`,
				},
			}},
		}},
	}
	exec := &fakeExec{}
	agent := &Agent{LLM: llm, Exec: exec, Model: "test", ApprovalsMode: "enabled"}
	final, msgs, _, err := agent.RunConversation(context.Background(), []types.LLMMessage{{Role: "user", Content: "write"}})
	if final != "" {
		t.Fatalf("expected no final text, got %q", final)
	}
	if err == nil {
		t.Fatal("expected approval error")
	}
	var approvalErr ErrApprovalRequired
	if !errors.As(err, &approvalErr) {
		t.Fatalf("expected ErrApprovalRequired, got %v", err)
	}
	if len(approvalErr.PendingOps) != 1 {
		t.Fatalf("expected one pending op, got %d", len(approvalErr.PendingOps))
	}
	if approvalErr.PendingOps[0].Op != types.HostOpFSWrite {
		t.Fatalf("unexpected pending op %q", approvalErr.PendingOps[0].Op)
	}
	if len(approvalErr.PendingToolCallIDs) != 1 || approvalErr.PendingToolCallIDs[0] != "1" {
		t.Fatalf("unexpected tool call IDs %v", approvalErr.PendingToolCallIDs)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages (user + assistant), got %d", len(msgs))
	}
	if len(exec.reqs) != 0 {
		t.Fatalf("expected no host ops executed, got %d", len(exec.reqs))
	}
}
