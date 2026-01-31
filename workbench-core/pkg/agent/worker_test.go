package agent

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/workbench-core/pkg/llm"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// stubAgent is a minimal Agent implementation backed by an in-memory map.
type stubAgent struct {
	files   map[string]string
	runText string
	writes  []string
}

func newStubAgent() *stubAgent {
	return &stubAgent{files: map[string]string{}, runText: "done"}
}

func (s *stubAgent) Run(ctx context.Context, goal string) (string, error) { return s.runText, nil }
func (s *stubAgent) RunConversation(context.Context, []llm.LLMMessage) (string, []llm.LLMMessage, int, error) {
	return "", nil, 0, nil
}

func (s *stubAgent) ExecHostOp(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
	switch req.Op {
	case types.HostOpFSList:
		prefix := strings.TrimSuffix(req.Path, "/") + "/"
		seen := map[string]struct{}{}
		entries := []string{}
		for k := range s.files {
			if strings.HasPrefix(k, prefix) {
				base := strings.TrimPrefix(k, prefix)
				parts := strings.SplitN(base, "/", 2)
				name := parts[0]
				if name == "" {
					continue
				}
				if _, ok := seen[name]; ok {
					continue
				}
				seen[name] = struct{}{}
				entries = append(entries, name)
			}
		}
		sort.Strings(entries)
		return types.HostOpResponse{Op: req.Op, Ok: true, Entries: entries}
	case types.HostOpFSRead:
		txt, ok := s.files[req.Path]
		if !ok {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: "not found"}
		}
		return types.HostOpResponse{Op: req.Op, Ok: true, Text: txt}
	case types.HostOpFSWrite:
		if req.Path == "" {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: "path required"}
		}
		s.writes = append(s.writes, req.Path)
		s.files[req.Path] = req.Text
		return types.HostOpResponse{Op: req.Op, Ok: true}
	default:
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: "unsupported op"}
	}
}

// Configurable no-ops below.
func (s *stubAgent) GetModel() string                         { return "" }
func (s *stubAgent) SetModel(string)                          {}
func (s *stubAgent) WebSearchEnabled() bool                   { return false }
func (s *stubAgent) SetEnableWebSearch(bool)                  {}
func (s *stubAgent) GetApprovalsMode() string                 { return "" }
func (s *stubAgent) SetApprovalsMode(string)                  {}
func (s *stubAgent) GetReasoningEffort() string               { return "" }
func (s *stubAgent) SetReasoningEffort(string)                {}
func (s *stubAgent) GetReasoningSummary() string              { return "" }
func (s *stubAgent) SetReasoningSummary(string)               {}
func (s *stubAgent) GetSystemPrompt() string                  { return "" }
func (s *stubAgent) SetSystemPrompt(string)                   {}
func (s *stubAgent) GetHooks() *Hooks                         { return &Hooks{} }
func (s *stubAgent) SetHooks(Hooks)                           {}
func (s *stubAgent) GetToolRegistry() *ToolRegistry           { return nil }
func (s *stubAgent) SetToolRegistry(*ToolRegistry)            {}
func (s *stubAgent) GetExtraTools() []llm.Tool                { return nil }
func (s *stubAgent) SetExtraTools([]llm.Tool)                 {}
func (s *stubAgent) Clone() Agent                             { return s }
func (s *stubAgent) Config() AgentConfig                      { return AgentConfig{} }
func (s *stubAgent) CloneWithConfig(cfg AgentConfig) (Agent, error) { return s, nil }

func TestWorkerWritesResultToOutbox(t *testing.T) {
	a := newStubAgent()

	now := time.Now().UTC()
	task := types.Task{
		TaskID:    "t1",
		Goal:      "do something",
		Status:    "pending",
		CreatedAt: &now,
	}
	b, _ := json.MarshalIndent(task, "", "  ")
	a.files["/inbox/task-t1.json"] = string(b)

	w, err := NewWorker(WorkerRunnerConfig{
		Agent:        a,
		PollInterval: time.Second,
		InboxPath:    "/inbox",
		OutboxPath:   "/outbox",
	})
	if err != nil {
		t.Fatalf("NewWorker: %v", err)
	}

	if err := w.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce: %v", err)
	}

	outPath := "/outbox/result-t1.json"
	if len(a.writes) == 0 {
		t.Fatalf("no writes recorded, files=%v", keys(a.files))
	}
	got, ok := a.files[outPath]
	if !ok {
		t.Fatalf("expected outbox result at %s, writes=%v files=%v", outPath, a.writes, keys(a.files))
	}
	var result types.TaskResult
	if err := json.Unmarshal([]byte(got), &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.Status != "succeeded" {
		t.Fatalf("result status=%q want succeeded", result.Status)
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
