package runner

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	harness "github.com/tinoosan/agen8-mcp-server/internal/services/harness/domain"
)

type stubPromptPartitioner struct {
	stable  string
	dynamic string
}

func (s *stubPromptPartitioner) SystemPrompt(_ context.Context, base string, _ int) (string, error) {
	return base, nil
}

func (s *stubPromptPartitioner) StableSystemPrompt(_ context.Context, base string, _ int) (string, error) {
	if s.stable != "" {
		return s.stable, nil
	}
	return base, nil
}

func (s *stubPromptPartitioner) DynamicContext(_ context.Context, _ int) (string, error) {
	return s.dynamic, nil
}

type testHarnessRuntime struct {
	kind            string
	startSpec       harness.StartSpec
	writePromptErr  error
	writeToolResErr error
	lastStartParams harness.StartParams
	startCalls      []harness.StartParams
	promptInputs    []harness.PromptInput
}

func (t *testHarnessRuntime) Kind() string {
	if strings.TrimSpace(t.kind) == "" {
		return "test-harness"
	}
	return strings.TrimSpace(t.kind)
}

func (t *testHarnessRuntime) Start(params harness.StartParams) (harness.StartSpec, error) {
	t.lastStartParams = params
	t.startCalls = append(t.startCalls, params)
	return t.startSpec, nil
}

func (t *testHarnessRuntime) ParseEvents(stream []byte) ([]harness.Event, error) {
	var raw map[string]string
	if err := json.Unmarshal(stream, &raw); err != nil {
		return nil, err
	}
	switch strings.TrimSpace(raw["type"]) {
	case "text":
		data := map[string]string{}
		if kind := strings.TrimSpace(raw["kind"]); kind != "" {
			data["kind"] = kind
		}
		return []harness.Event{{Type: harness.EventText, Text: raw["text"], Data: data}}, nil
	case "tool_call":
		data := map[string]string{}
		if status := strings.TrimSpace(raw["status"]); status != "" {
			data["status"] = status
		}
		if command := strings.TrimSpace(raw["command"]); command != "" {
			data["command"] = command
		}
		if background := strings.TrimSpace(raw["background"]); background != "" {
			data["background"] = background
		}
		return []harness.Event{{
			Type:       harness.EventToolCall,
			ToolCallID: strings.TrimSpace(raw["tool_call_id"]),
			ToolName:   strings.TrimSpace(raw["tool_name"]),
			Data:       data,
		}}, nil
	case "tool_result":
		data := map[string]string{}
		for _, key := range []string{"status", "sourceType", "result", "error", "command", "background", "outputFull"} {
			if value := strings.TrimSpace(raw[key]); value != "" {
				data[key] = value
			}
		}
		return []harness.Event{{
			Type:       harness.EventToolResult,
			ToolCallID: strings.TrimSpace(raw["tool_call_id"]),
			ToolName:   strings.TrimSpace(raw["tool_name"]),
			Text:       strings.TrimSpace(raw["text"]),
			Data:       data,
		}}, nil
	case "retry":
		data := map[string]string{}
		for _, key := range []string{"attempt", "max", "reason"} {
			if value := strings.TrimSpace(raw[key]); value != "" {
				data[key] = value
			}
		}
		return []harness.Event{{Type: harness.EventRetry, Text: raw["text"], Data: data}}, nil
	case "turn_failed":
		return []harness.Event{{Type: harness.EventTurnFailed, Error: raw["error"]}}, nil
	case "turn_completed":
		usage := &harness.Usage{}
		if raw["input"] != "" || raw["output"] != "" || raw["total"] != "" || raw["cache_read"] != "" {
			usage.InputTokens = mustAtoi(raw["input"])
			usage.OutputTokens = mustAtoi(raw["output"])
			usage.TotalTokens = mustAtoi(raw["total"])
			usage.CacheReadInputTokens = mustAtoi(raw["cache_read"])
		} else {
			usage = nil
		}
		return []harness.Event{{Type: harness.EventTurnCompleted, Usage: usage}}, nil
	case "turn_started":
		return []harness.Event{{Type: harness.EventTurnStarted, TurnID: strings.TrimSpace(raw["turn_id"])}}, nil
	case "space_started":
		return []harness.Event{{Type: harness.EventTurnStarted, SessionRef: strings.TrimSpace(raw["session_id"])}}, nil
	default:
		return nil, nil
	}
}

func mustAtoi(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	var out int
	for _, ch := range raw {
		if ch < '0' || ch > '9' {
			panic("test integer contains non-digit")
		}
		out = out*10 + int(ch-'0')
	}
	return out
}

func (t *testHarnessRuntime) WritePrompt(_ io.Writer, input harness.PromptInput) error {
	t.promptInputs = append(t.promptInputs, input)
	return t.writePromptErr
}

func (t *testHarnessRuntime) WriteToolResult(_ io.Writer, _ harness.ToolResultInput) error {
	return t.writeToolResErr
}

type testStreamingHarnessRuntime struct {
	testHarnessRuntime
	streamStartCalls []harness.StartParams
}

func (t *testStreamingHarnessRuntime) StartStreamingInput(params harness.StartParams) (harness.StartSpec, error) {
	t.streamStartCalls = append(t.streamStartCalls, params)
	return t.startSpec, nil
}

func (t *testStreamingHarnessRuntime) WriteStreamingPrompt(w io.Writer, input harness.PromptInput) error {
	t.promptInputs = append(t.promptInputs, input)
	if w == nil {
		return nil
	}
	_, err := io.WriteString(w, `{"type":"user"}`+"\n")
	return err
}

type testSessionHarnessCall struct {
	params harness.StartParams
	input  harness.SessionTurnInput
}

type testSessionHarnessRuntime struct {
	testHarnessRuntime
	sessionRef string
	calls      []testSessionHarnessCall
}

func (t *testSessionHarnessRuntime) ExecuteSessionTurn(_ context.Context, params harness.StartParams, input harness.SessionTurnInput, emit func(harness.Event)) (harness.SessionTurnResult, error) {
	t.calls = append(t.calls, testSessionHarnessCall{params: params, input: input})
	sessionRef := strings.TrimSpace(t.sessionRef)
	if sessionRef == "" {
		sessionRef = "space-session"
	}
	emit(harness.Event{Type: harness.EventTurnStarted, SessionRef: sessionRef})
	emit(harness.Event{Type: harness.EventTurnStarted, TurnID: "turn-session"})
	emit(harness.Event{Type: harness.EventText, TurnID: "turn-session", Text: "ok", Data: map[string]string{"kind": "assistant"}})
	emit(harness.Event{Type: harness.EventTurnCompleted, TurnID: "turn-session"})
	return harness.SessionTurnResult{}, nil
}

type missingCompletionTurnSessionRuntime struct {
	testHarnessRuntime
}

func (t *missingCompletionTurnSessionRuntime) ExecuteSessionTurn(_ context.Context, params harness.StartParams, input harness.SessionTurnInput, emit func(harness.Event)) (harness.SessionTurnResult, error) {
	emit(harness.Event{Type: harness.EventTurnStarted, SessionRef: "space-session"})
	emit(harness.Event{Type: harness.EventText, TurnID: "assistant-msg-1", Text: "ok", Data: map[string]string{"kind": "assistant"}})
	emit(harness.Event{Type: harness.EventTurnCompleted})
	return harness.SessionTurnResult{}, nil
}

type cancelingSessionHarnessRuntime struct {
	testHarnessRuntime
}

func (t *cancelingSessionHarnessRuntime) ExecuteSessionTurn(ctx context.Context, params harness.StartParams, input harness.SessionTurnInput, emit func(harness.Event)) (harness.SessionTurnResult, error) {
	emit(harness.Event{Type: harness.EventTurnStarted, SessionRef: "space-session"})
	<-ctx.Done()
	return harness.SessionTurnResult{}, ctx.Err()
}

func TestService_HarnessEnvIncludesProjectAndWorkspaceRoots(t *testing.T) {
	runner, err := New(Config{
		Runtime:   &testHarnessRuntime{},
		Workdir:   "/tmp/project-root",
		Workspace: "/tmp/project-root/workspace",
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	env := strings.Join(runner.harnessEnv(), "\n")
	if !strings.Contains(env, "PROJECT_ROOT=/tmp/project-root") {
		t.Fatalf("missing PROJECT_ROOT in env: %q", env)
	}
	if !strings.Contains(env, "WORKSPACE_ROOT=/tmp/project-root/workspace") {
		t.Fatalf("missing WORKSPACE_ROOT in env: %q", env)
	}
}

func TestService_ExecuteEmitsRetryAndContinues(t *testing.T) {
	rt := &testHarnessRuntime{
		startSpec: harness.StartSpec{
			Command: "sh",
			Args: []string{
				"-c",
				`cat >/dev/null; printf '{"type":"retry","text":"Reconnecting... 1/5 (network)","attempt":"1","max":"5","reason":"network"}\n'; printf '{"type":"text","text":"recovered"}\n'; printf '{"type":"turn_completed"}\n'`,
			},
		},
	}
	runner, err := New(Config{Runtime: rt})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	events := collectEvents(runner.Execute(context.Background(), TurnConfig{Model: "test-model"}, TurnInput{Instruction: "say hello"}))
	if len(events) < 3 {
		t.Fatalf("events=%d want >=3", len(events))
	}
	if events[0].Kind != EventRetry {
		t.Fatalf("first event kind=%v want harness retry", events[0].Kind)
	}
	if events[0].HarnessRetryAttempt != "1" || events[0].HarnessRetryMax != "5" || events[0].HarnessRetryReason != "network" {
		t.Fatalf("retry event=%+v", events[0])
	}
	last := events[len(events)-1]
	if last.Kind != EventDone {
		t.Fatalf("last event kind=%v want done", last.Kind)
	}
	if got := strings.TrimSpace(last.Result.Text); got != "recovered" {
		t.Fatalf("result text=%q want recovered", got)
	}
}

func TestService_ExecuteEmitsTurnStarted(t *testing.T) {
	rt := &testHarnessRuntime{
		startSpec: harness.StartSpec{
			Command: "sh",
			Args: []string{
				"-c",
				`cat >/dev/null; printf '{"type":"turn_started","turn_id":"turn-started-1"}\n'; printf '{"type":"text","text":"ok"}\n'; printf '{"type":"turn_completed"}\n'`,
			},
		},
	}
	runner, err := New(Config{Runtime: rt})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	events := collectEvents(runner.Execute(context.Background(), TurnConfig{Model: "test-model"}, TurnInput{Instruction: "say ok"}))
	if len(events) < 3 {
		t.Fatalf("events=%d want >=3", len(events))
	}
	if events[0].Kind != EventTurnStarted {
		t.Fatalf("first event=%+v want harness turn started", events[0])
	}
	if events[0].StreamID != "turn-started-1" {
		t.Fatalf("stream id=%q want turn-started-1", events[0].StreamID)
	}
}

func TestService_ExecuteStreamsTextAndCompletes(t *testing.T) {
	rt := &testHarnessRuntime{
		startSpec: harness.StartSpec{
			Command: "sh",
			Args: []string{
				"-c",
				`cat >/dev/null; printf '{"type":"text","text":"hello "}\n'; printf '{"type":"text","text":"world"}\n'; printf '{"type":"turn_completed"}\n'`,
			},
		},
	}
	runner, err := New(Config{Runtime: rt})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	events := collectEvents(runner.Execute(context.Background(), TurnConfig{Model: "test-model"}, TurnInput{Instruction: "say hello"}))
	if len(events) < 3 {
		t.Fatalf("events=%d want >=3", len(events))
	}
	if events[0].Kind != EventText || events[0].HarnessText != "hello " {
		t.Fatalf("first event=%+v want agent text hello", events[0])
	}
	if events[1].Kind != EventText || events[1].HarnessText != "world" {
		t.Fatalf("second event=%+v want agent text world", events[1])
	}
	last := events[len(events)-1]
	if last.Kind != EventDone {
		t.Fatalf("last event kind=%v want done", last.Kind)
	}
	if got := strings.TrimSpace(last.Result.Text); got != "hello world" {
		t.Fatalf("result text=%q want hello world", got)
	}
}

func TestService_ExecuteEmitsUsageFromHarnessTurnCompleted(t *testing.T) {
	rt := &testHarnessRuntime{
		startSpec: harness.StartSpec{
			Command: "sh",
			Args: []string{
				"-c",
				`cat >/dev/null; printf '{"type":"turn_completed","input":"100","output":"25","total":"125","cache_read":"60"}\n'`,
			},
		},
	}
	runner, err := New(Config{Runtime: rt})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	events := collectEvents(runner.Execute(context.Background(), TurnConfig{Model: "test-model"}, TurnInput{Instruction: "hello"}))

	foundUsage := false
	for _, ev := range events {
		if ev.Kind != EventUsage {
			continue
		}
		foundUsage = true
		if ev.Usage.InputTokens != 100 || ev.Usage.OutputTokens != 25 || ev.Usage.TotalTokens != 125 || ev.Usage.CacheReadInputTokens != 60 {
			t.Fatalf("usage=%+v", ev.Usage)
		}
	}
	if !foundUsage {
		t.Fatalf("expected EventUsage in events=%#v", events)
	}
}

func TestService_StreamingInputReusesWarmProcess(t *testing.T) {
	rt := &testStreamingHarnessRuntime{
		testHarnessRuntime: testHarnessRuntime{
			kind: "claude-cli",
			startSpec: harness.StartSpec{
				Command: "sh",
				Args: []string{
					"-c",
					`i=0; while IFS= read -r line; do i=$((i+1)); if [ "$i" -eq 1 ]; then printf '{"type":"space_started","session_id":"stream-123"}\n'; fi; printf '{"type":"text","text":"ok%s"}\n' "$i"; printf '{"type":"turn_completed"}\n'; done`,
				},
			},
		},
	}
	runner, err := New(Config{Runtime: rt})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	first := collectEvents(runner.Execute(context.Background(), TurnConfig{Model: "test-model"}, TurnInput{Instruction: "first request"}))
	second := collectEvents(runner.Execute(context.Background(), TurnConfig{Model: "test-model"}, TurnInput{Instruction: "second request"}))

	if len(rt.streamStartCalls) != 1 {
		t.Fatalf("streaming starts=%d want 1", len(rt.streamStartCalls))
	}
	if got := finalLoopText(first); got != "ok1" {
		t.Fatalf("first result=%q want ok1", got)
	}
	if got := finalLoopText(second); got != "ok2" {
		t.Fatalf("second result=%q want ok2", got)
	}
	if len(rt.promptInputs) != 2 {
		t.Fatalf("prompt writes=%d want 2", len(rt.promptInputs))
	}
	if strings.TrimSpace(rt.promptInputs[1].Text) != "second request" {
		t.Fatalf("second prompt=%q want latest user content", rt.promptInputs[1].Text)
	}
}

func TestService_SessionRuntimePersistsAndResumesLatestUserOnly(t *testing.T) {
	rt := &testSessionHarnessRuntime{sessionRef: "space-session"}
	var persisted []string
	runner, err := New(Config{
		Runtime: rt,
		PersistSessionRef: func(sessionRef string) error {
			persisted = append(persisted, strings.TrimSpace(sessionRef))
			return nil
		},
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	collectEvents(runner.Execute(context.Background(), TurnConfig{Model: "test-model"}, TurnInput{Instruction: "first request"}))
	events := collectEvents(runner.Execute(context.Background(), TurnConfig{Model: "test-model"}, TurnInput{Instruction: "second request"}))
	if got := finalLoopText(events); got != "ok" {
		t.Fatalf("final text=%q want ok", got)
	}
	var completed Event
	for _, event := range events {
		if event.Kind == EventTurnCompleted {
			completed = event
			break
		}
	}
	if completed.Kind != EventTurnCompleted {
		t.Fatalf("missing harness turn completion event: %#v", events)
	}
	if completed.HarnessTurnID != "turn-session" {
		t.Fatalf("completed turn id=%q want turn-session", completed.HarnessTurnID)
	}
	if len(rt.calls) != 2 {
		t.Fatalf("session calls=%d want 2", len(rt.calls))
	}
	if rt.calls[0].params.Continue {
		t.Fatalf("first call should start a new external space")
	}
	if !rt.calls[1].params.Continue || rt.calls[1].params.SessionRef != "space-session" {
		t.Fatalf("second call params=%+v want resume space-session", rt.calls[1].params)
	}
	if got := strings.TrimSpace(rt.calls[1].input.Text); got != "second request" {
		t.Fatalf("second input=%q want latest user only", got)
	}
	if len(persisted) != 1 || persisted[0] != "space-session" {
		t.Fatalf("persisted=%v want [space-session]", persisted)
	}
}

func TestService_SessionRuntimeHydratesMissingCompletionTurnID(t *testing.T) {
	rt := &missingCompletionTurnSessionRuntime{}
	runner, err := New(Config{Runtime: rt})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	events := collectEvents(runner.Execute(context.Background(), TurnConfig{Model: "test-model"}, TurnInput{Instruction: "hello"}))
	var textEvent Event
	var completed Event
	for _, event := range events {
		switch event.Kind {
		case EventText:
			textEvent = event
		case EventTurnCompleted:
			completed = event
		}
	}
	if textEvent.Kind != EventText {
		t.Fatalf("missing harness text event: %#v", events)
	}
	if completed.Kind != EventTurnCompleted {
		t.Fatalf("missing harness turn completion event: %#v", events)
	}
	if completed.HarnessTurnID != textEvent.HarnessTurnID {
		t.Fatalf("completed turn id=%q want text turn id %q", completed.HarnessTurnID, textEvent.HarnessTurnID)
	}
	if completed.HarnessTurnID != "assistant-msg-1" {
		t.Fatalf("completed turn id=%q want assistant-msg-1", completed.HarnessTurnID)
	}
}

func TestService_SessionRuntimeEmitsCancelTerminalEvent(t *testing.T) {
	rt := &cancelingSessionHarnessRuntime{}
	runner, err := New(Config{Runtime: rt})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ch := runner.Execute(ctx, TurnConfig{Model: "test-model"}, TurnInput{Instruction: "stop me"})
	cancel()

	events := collectEvents(ch)
	if len(events) == 0 {
		t.Fatalf("expected terminal cancel event")
	}
	last := events[len(events)-1]
	if last.Kind != EventError || !errors.Is(last.Err, ErrTurnCanceled) {
		t.Fatalf("last event=%+v want ErrTurnCanceled", last)
	}
}

func TestService_SessionRuntimeUsesInitialSessionRef(t *testing.T) {
	rt := &testSessionHarnessRuntime{sessionRef: "space-existing"}
	runner, err := New(Config{
		Runtime:           rt,
		InitialSessionRef: "space-existing",
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	events := collectEvents(runner.Execute(context.Background(), TurnConfig{Model: "test-model"}, TurnInput{Instruction: "hello"}))
	if got := finalLoopText(events); got != "ok" {
		t.Fatalf("final text=%q want ok", got)
	}
	if len(rt.calls) != 1 {
		t.Fatalf("session calls=%d want 1", len(rt.calls))
	}
	if !rt.calls[0].params.Continue || rt.calls[0].params.SessionRef != "space-existing" {
		t.Fatalf("call params=%+v want initial session resume", rt.calls[0].params)
	}
}

func TestService_SessionRuntimeUsesPromptSourceStableSystem(t *testing.T) {
	rt := &testSessionHarnessRuntime{sessionRef: "space-prompt"}
	runner, err := New(Config{
		Runtime:    rt,
		MCPServers: []string{`mcp_servers.agen8.url="http://127.0.0.1:7777/mcp?token=test"`},
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	events := collectEvents(runner.Execute(context.Background(), TurnConfig{
		Model:        "test-model",
		SystemPrompt: "base agen8 prompt",
		PromptSource: &stubPromptPartitioner{
			stable:  "stable agen8 prompt with <available_skills>",
			dynamic: "dynamic should not be developer instructions",
		},
	}, TurnInput{Instruction: "hello"}))
	if got := finalLoopText(events); got != "ok" {
		t.Fatalf("final text=%q want ok", got)
	}
	if got := rt.calls[0].params.SystemPrompt; !strings.Contains(got, "stable agen8 prompt with <available_skills>") {
		t.Fatalf("system prompt=%q missing stable prompt", got)
	}
	if strings.Contains(rt.calls[0].params.SystemPrompt, "dynamic should not be developer instructions") {
		t.Fatalf("system prompt includes dynamic context: %q", rt.calls[0].params.SystemPrompt)
	}
	if len(rt.calls[0].params.MCPServers) != 1 || !strings.Contains(rt.calls[0].params.MCPServers[0], "mcp_servers.agen8.url") {
		t.Fatalf("mcp params=%v", rt.calls[0].params.MCPServers)
	}
}

func TestService_ExecuteContinuationUsesLatestUserPromptOnly(t *testing.T) {
	rt := &testHarnessRuntime{
		kind: "codex",
		startSpec: harness.StartSpec{
			Command: "sh",
			Args: []string{
				"-c",
				`cat >/dev/null; printf '{"type":"space_started","session_id":"space-123"}\n'; printf '{"type":"text","text":"ok"}\n'; printf '{"type":"turn_completed"}\n'`,
			},
		},
	}
	runner, err := New(Config{Runtime: rt})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	collectEvents(runner.Execute(context.Background(), TurnConfig{Model: "test-model"}, TurnInput{Instruction: "first request"}))
	collectEvents(runner.Execute(context.Background(), TurnConfig{Model: "test-model"}, TurnInput{Instruction: "second request"}))

	if len(rt.startCalls) != 2 {
		t.Fatalf("start calls=%d want 2", len(rt.startCalls))
	}
	if rt.startCalls[0].Continue {
		t.Fatalf("first start Continue=true, want false")
	}
	if strings.TrimSpace(rt.startCalls[1].SessionRef) != "space-123" {
		t.Fatalf("second start session=%q want space-123", rt.startCalls[1].SessionRef)
	}
	if !rt.startCalls[1].Continue {
		t.Fatalf("second start Continue=false, want true")
	}
	if len(rt.promptInputs) != 2 {
		t.Fatalf("prompt writes=%d want 2", len(rt.promptInputs))
	}
	if strings.Contains(rt.promptInputs[1].Text, "USER:\nfirst request") {
		t.Fatalf("continuation prompt unexpectedly contains full transcript: %q", rt.promptInputs[1].Text)
	}
	if strings.TrimSpace(rt.promptInputs[1].Text) != "second request" {
		t.Fatalf("continuation prompt=%q want latest user content", rt.promptInputs[1].Text)
	}
}

func TestService_ExecuteClaudeContinuationUsesLatestUserPromptOnly(t *testing.T) {
	rt := &testHarnessRuntime{
		kind: "claude-cli",
		startSpec: harness.StartSpec{
			Command: "sh",
			Args: []string{
				"-c",
				`cat >/dev/null; printf '{"type":"text","text":"ok"}\n'; printf '{"type":"turn_completed"}\n'`,
			},
		},
	}
	runner, err := New(Config{Runtime: rt})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	collectEvents(runner.Execute(context.Background(), TurnConfig{Model: "test-model"}, TurnInput{Instruction: "first request"}))
	collectEvents(runner.Execute(context.Background(), TurnConfig{Model: "test-model"}, TurnInput{Instruction: "second request"}))

	if len(rt.startCalls) != 2 {
		t.Fatalf("start calls=%d want 2", len(rt.startCalls))
	}
	if !rt.startCalls[1].Continue {
		t.Fatalf("second start Continue=false, want true")
	}
	if len(rt.promptInputs) != 2 {
		t.Fatalf("prompt writes=%d want 2", len(rt.promptInputs))
	}
	if strings.Contains(rt.promptInputs[1].Text, "USER:\nfirst request") {
		t.Fatalf("claude continuation prompt unexpectedly contains full transcript: %q", rt.promptInputs[1].Text)
	}
	if strings.TrimSpace(rt.promptInputs[1].Text) != "second request" {
		t.Fatalf("claude continuation prompt=%q want latest user content", rt.promptInputs[1].Text)
	}
}

func TestService_ExecutePassesModelToRuntimeStart(t *testing.T) {
	rt := &testHarnessRuntime{
		kind: "claude-cli",
		startSpec: harness.StartSpec{
			Command: "sh",
			Args: []string{
				"-c",
				`cat >/dev/null; printf '{"type":"turn_completed"}\n'`,
			},
		},
	}
	runner, err := New(Config{Runtime: rt})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	collectEvents(runner.Execute(context.Background(), TurnConfig{Model: "anthropic/claude-sonnet-4-6"}, TurnInput{Instruction: "hello"}))

	if got := strings.TrimSpace(rt.lastStartParams.Model); got != "anthropic/claude-sonnet-4-6" {
		t.Fatalf("start model=%q want anthropic/claude-sonnet-4-6", got)
	}
}

func TestService_ExecuteClaudeSeedsSessionRef(t *testing.T) {
	rt := &testHarnessRuntime{
		kind: "claude-cli",
		startSpec: harness.StartSpec{
			Command: "sh",
			Args: []string{
				"-c",
				`cat >/dev/null; printf '{"type":"text","text":"ok"}\n'; printf '{"type":"turn_completed"}\n'`,
			},
		},
	}
	runner, err := New(Config{Runtime: rt})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	collectEvents(runner.Execute(context.Background(), TurnConfig{Model: "test-model"}, TurnInput{Instruction: "seed session"}))
	if len(rt.startCalls) != 1 {
		t.Fatalf("start calls=%d want 1", len(rt.startCalls))
	}
	if strings.TrimSpace(rt.startCalls[0].SessionRef) == "" {
		t.Fatalf("claude start session id should be generated")
	}
	if rt.startCalls[0].Continue {
		t.Fatalf("first claude turn Continue=true, want false")
	}
}

func TestService_ExecuteStreamsReasoningWithoutPollutingFinalText(t *testing.T) {
	rt := &testHarnessRuntime{
		startSpec: harness.StartSpec{
			Command: "sh",
			Args: []string{
				"-c",
				`cat >/dev/null; printf '{"type":"text","kind":"reasoning","text":"thinking"}\n'; printf '{"type":"text","text":"final answer"}\n'; printf '{"type":"turn_completed"}\n'`,
			},
		},
	}
	runner, err := New(Config{Runtime: rt})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	events := collectEvents(runner.Execute(context.Background(), TurnConfig{Model: "test-model"}, TurnInput{Instruction: "answer"}))
	if len(events) < 3 {
		t.Fatalf("events=%d want >=3", len(events))
	}
	if events[0].Kind != EventStreamChunk || events[0].Chunk == nil || !events[0].Chunk.IsReasoning {
		t.Fatalf("first event=%+v want reasoning stream chunk", events[0])
	}
	if events[1].Kind != EventText || events[1].HarnessText != "final answer" {
		t.Fatalf("second event=%+v want agent text final answer", events[1])
	}
	last := events[len(events)-1]
	if last.Kind != EventDone {
		t.Fatalf("last event kind=%v want done", last.Kind)
	}
	if got := strings.TrimSpace(last.Result.Text); got != "final answer" {
		t.Fatalf("result text=%q want final answer", got)
	}
}

func TestEmitExternalHarnessEvent_ReplacesWhitespaceEquivalentFinalText(t *testing.T) {
	var text strings.Builder
	var emitted []Event
	emit := func(ev Event) { emitted = append(emitted, ev) }

	for _, chunk := range []string{"Hi", ".", "What"} {
		if err := emitExternalHarnessEvent(1, harness.Event{Type: harness.EventText, Text: chunk}, &text, nil, nil, emit); err != nil {
			t.Fatalf("emit chunk %q: %v", chunk, err)
		}
	}
	if err := emitExternalHarnessEvent(1, harness.Event{Type: harness.EventText, Text: "Hi. What"}, &text, nil, nil, emit); err != nil {
		t.Fatalf("emit final full text: %v", err)
	}
	if got := text.String(); got != "Hi. What" {
		t.Fatalf("final text=%q want replacement", got)
	}
	if len(emitted) != 3 {
		t.Fatalf("emitted=%d want only incremental chunks streamed", len(emitted))
	}
}

func TestEmitExternalHarnessEvent_PreservesStandaloneWhitespaceDeltas(t *testing.T) {
	var text strings.Builder
	var emitted []Event
	emit := func(ev Event) { emitted = append(emitted, ev) }

	for _, chunk := range []string{"Hello", " ", "world", "\n\n", "Next"} {
		if err := emitExternalHarnessEvent(1, harness.Event{Type: harness.EventText, Text: chunk}, &text, nil, nil, emit); err != nil {
			t.Fatalf("emit chunk %q: %v", chunk, err)
		}
	}
	if got, want := text.String(), "Hello world\n\nNext"; got != want {
		t.Fatalf("final text=%q want %q", got, want)
	}
	if len(emitted) != 5 {
		t.Fatalf("emitted=%d want all chunks including whitespace-only chunks", len(emitted))
	}
	if emitted[1].HarnessText != " " || emitted[3].HarnessText != "\n\n" {
		t.Fatalf("standalone whitespace chunks were not emitted: %#v", emitted)
	}
}

func TestEmitExternalHarnessEvent_SegmentsTextAroundToolCalls(t *testing.T) {
	var text strings.Builder
	var emitted []Event
	segment := 1
	open := false
	emit := func(ev Event) { emitted = append(emitted, ev) }

	if err := emitExternalHarnessEvent(1, harness.Event{Type: harness.EventText, Text: "before", TurnID: "turn-1"}, &text, &segment, &open, emit); err != nil {
		t.Fatalf("emit first text: %v", err)
	}
	if err := emitExternalHarnessEvent(1, harness.Event{Type: harness.EventToolCall, ToolCallID: "call-1", ToolName: "tool"}, &text, &segment, &open, emit); err != nil {
		t.Fatalf("emit tool call: %v", err)
	}
	if err := emitExternalHarnessEvent(1, harness.Event{Type: harness.EventText, Text: "after", TurnID: "turn-1"}, &text, &segment, &open, emit); err != nil {
		t.Fatalf("emit second text: %v", err)
	}

	if len(emitted) != 3 {
		t.Fatalf("emitted=%d want 3", len(emitted))
	}
	if emitted[0].SegmentID != "1" {
		t.Fatalf("first segment=%q want 1", emitted[0].SegmentID)
	}
	if emitted[2].SegmentID != "2" {
		t.Fatalf("second segment=%q want 2", emitted[2].SegmentID)
	}
	if got := text.String(); got != "beforeafter" {
		t.Fatalf("final text=%q want beforeafter", got)
	}
}

func TestEmitExternalHarnessEvent_PropagatesToolTurnID(t *testing.T) {
	var text strings.Builder
	var emitted []Event
	emit := func(ev Event) { emitted = append(emitted, ev) }

	if err := emitExternalHarnessEvent(1, harness.Event{
		Type:       harness.EventToolCall,
		TurnID:     "turn-7",
		ToolCallID: "call-7",
		ToolName:   "shell_exec",
	}, &text, nil, nil, emit); err != nil {
		t.Fatalf("emit tool call: %v", err)
	}
	if err := emitExternalHarnessEvent(1, harness.Event{
		Type:       harness.EventToolResult,
		TurnID:     "turn-7",
		ToolCallID: "call-7",
		ToolName:   "shell_exec",
		Text:       "ok",
		Data: map[string]string{
			"status": "completed",
		},
	}, &text, nil, nil, emit); err != nil {
		t.Fatalf("emit tool result: %v", err)
	}

	if len(emitted) != 2 {
		t.Fatalf("emitted=%d want 2", len(emitted))
	}
	if emitted[0].Kind != EventToolCall {
		t.Fatalf("first event kind=%v want tool call", emitted[0].Kind)
	}
	if got := emitted[0].RuntimeToolTurnID; got != "turn-7" {
		t.Fatalf("tool call turnId=%q want turn-7", got)
	}
	if emitted[1].Kind != EventToolResult {
		t.Fatalf("second event kind=%v want tool result", emitted[1].Kind)
	}
	if got := emitted[1].RuntimeToolTurnID; got != "turn-7" {
		t.Fatalf("tool result turnId=%q want turn-7", got)
	}
}

func TestSynthesizeShellDiffEvents_PreservesTurnID(t *testing.T) {
	events := synthesizeShellDiffEvents(harness.Event{
		Type:       harness.EventToolResult,
		TurnID:     "turn-9",
		ToolCallID: "call-9",
		ToolName:   "bash",
		Data: map[string]string{
			"status": "completed",
			"turnId": "turn-9",
		},
	}, []shellDiffChange{{
		Path:           "README.md",
		Op:             "edit_file",
		WriteMode:      "modified",
		PatchPreview:   "diff --git a/README.md b/README.md",
		PatchTruncated: false,
		LinesAdded:     1,
		LinesRemoved:   0,
	}})

	if len(events) != 1 {
		t.Fatalf("events=%d want 1", len(events))
	}
	if got := events[0].TurnID; got != "turn-9" {
		t.Fatalf("synthetic turnId=%q want turn-9", got)
	}
	if got := events[0].Data["turnId"]; got != "turn-9" {
		t.Fatalf("synthetic data turnId=%q want turn-9", got)
	}
}

func TestService_ExecuteHydratesToolResultNameFromToolCallID(t *testing.T) {
	rt := &testHarnessRuntime{
		startSpec: harness.StartSpec{
			Command: "sh",
			Args: []string{
				"-c",
				`cat >/dev/null; printf '{"type":"tool_call","tool_call_id":"call-1","tool_name":"mcp__agen8__space","status":"in_progress"}\n'; printf '{"type":"tool_result","tool_call_id":"call-1","status":"completed","text":"ok"}\n'; printf '{"type":"text","text":"done"}\n'; printf '{"type":"turn_completed"}\n'`,
			},
		},
	}
	runner, err := New(Config{Runtime: rt})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	events := collectEvents(runner.Execute(context.Background(), TurnConfig{Model: "test-model"}, TurnInput{Instruction: "list spaces"}))
	if len(events) == 0 {
		t.Fatalf("expected loop events")
	}

	var sawCall, sawResult bool
	for _, ev := range events {
		switch ev.Kind {
		case EventToolCall:
			sawCall = true
			if got := strings.TrimSpace(ev.RuntimeToolName); got != "mcp__agen8__space" {
				t.Fatalf("tool call name=%q want mcp__agen8__space", got)
			}
		case EventToolResult:
			sawResult = true
			if got := strings.TrimSpace(ev.RuntimeToolName); got != "mcp__agen8__space" {
				t.Fatalf("tool result name=%q want mcp__agen8__space", got)
			}
		}
	}
	if !sawCall {
		t.Fatalf("expected EventToolCall in events=%#v", events)
	}
	if !sawResult {
		t.Fatalf("expected EventToolResult in events=%#v", events)
	}
}

func TestService_ExecuteHydratesToolResultDataFromToolCallID(t *testing.T) {
	rt := &testHarnessRuntime{
		startSpec: harness.StartSpec{
			Command: "sh",
			Args: []string{
				"-c",
				`cat >/dev/null; printf '{"type":"tool_call","tool_call_id":"call-1","tool_name":"Edit","status":"in_progress","command":"edit /workspace/demo.txt"}\n'; printf '{"type":"tool_result","tool_call_id":"call-1","status":"completed","text":"ok"}\n'; printf '{"type":"text","text":"done"}\n'; printf '{"type":"turn_completed"}\n'`,
			},
		},
	}
	runner, err := New(Config{Runtime: rt})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	events := collectEvents(runner.Execute(context.Background(), TurnConfig{Model: "test-model"}, TurnInput{Instruction: "edit file"}))
	if len(events) == 0 {
		t.Fatalf("expected loop events")
	}

	var foundResult bool
	for _, ev := range events {
		if ev.Kind != EventToolResult {
			continue
		}
		foundResult = true
		var data map[string]string
		if err := json.Unmarshal(ev.RuntimeToolData, &data); err != nil {
			t.Fatalf("unmarshal tool result data: %v", err)
		}
		if got := strings.TrimSpace(data["command"]); got != `edit /workspace/demo.txt` {
			t.Fatalf("hydrated command=%q", got)
		}
	}
	if !foundResult {
		t.Fatalf("expected EventToolResult in events=%#v", events)
	}
}

func TestService_ExecuteEmitsExternalToolResultMetadata(t *testing.T) {
	rt := &testHarnessRuntime{
		startSpec: harness.StartSpec{
			Command: "sh",
			Args: []string{
				"-c",
				`cat >/dev/null; printf '{"type":"tool_call","tool_call_id":"call-1","tool_name":"shell_exec","status":"in_progress","command":"npm run dev &","background":"true"}\n'; printf '{"type":"tool_result","tool_call_id":"call-1","tool_name":"shell_exec","status":"in_progress","sourceType":"cli","result":"ready","text":"ready"}\n'; printf '{"type":"tool_result","tool_call_id":"call-1","tool_name":"shell_exec","status":"completed","sourceType":"cli","result":"ready","text":"ready"}\n'; printf '{"type":"text","text":"done"}\n'; printf '{"type":"turn_completed"}\n'`,
			},
		},
	}
	runner, err := New(Config{Runtime: rt})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	events := collectEvents(runner.Execute(context.Background(), TurnConfig{Model: "test-model"}, TurnInput{Instruction: "run shell"}))
	if len(events) < 4 {
		t.Fatalf("events=%d want >=4", len(events))
	}
	var foundPending bool
	var firstScopedID string
	for _, ev := range events {
		if ev.Kind != EventToolResult {
			continue
		}
		if strings.TrimSpace(ev.RuntimeToolCallID) != "" && firstScopedID == "" {
			firstScopedID = strings.TrimSpace(ev.RuntimeToolCallID)
		}
		var data map[string]string
		if err := json.Unmarshal(ev.RuntimeToolData, &data); err != nil {
			t.Fatalf("unmarshal tool result data: %v", err)
		}
		if strings.TrimSpace(data["status"]) != "in_progress" {
			continue
		}
		foundPending = true
		if strings.TrimSpace(ev.RuntimeToolSource) != "cli" {
			t.Fatalf("runtime tool source=%q want cli", ev.RuntimeToolSource)
		}
		if strings.TrimSpace(data["result"]) != "ready" {
			t.Fatalf("result=%q want ready", data["result"])
		}
	}
	if !foundPending {
		t.Fatalf("expected pending tool result event, got %#v", events)
	}
	if !strings.Contains(firstScopedID, "call-1") {
		t.Fatalf("scoped tool call id=%q missing raw id", firstScopedID)
	}
}

func TestService_ExecuteScopesRepeatedToolCallIDsPerTurn(t *testing.T) {
	rt := &testHarnessRuntime{
		startSpec: harness.StartSpec{
			Command: "sh",
			Args: []string{
				"-c",
				`cat >/dev/null; printf '{"type":"tool_call","tool_call_id":"call-1","tool_name":"shell_exec","status":"in_progress","command":"echo ok"}\n'; printf '{"type":"tool_result","tool_call_id":"call-1","tool_name":"shell_exec","status":"completed","sourceType":"cli","result":"ok","text":"ok"}\n'; printf '{"type":"text","text":"ok"}\n'; printf '{"type":"turn_completed"}\n'`,
			},
		},
	}
	runner, err := New(Config{Runtime: rt})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	runOnce := func() string {
		events := collectEvents(runner.Execute(context.Background(), TurnConfig{Model: "test-model"}, TurnInput{Instruction: "run shell"}))
		for _, ev := range events {
			if ev.Kind == EventToolCall && strings.TrimSpace(ev.RuntimeToolCallID) != "" {
				return strings.TrimSpace(ev.RuntimeToolCallID)
			}
		}
		t.Fatalf("missing tool call event: %#v", events)
		return ""
	}

	first := runOnce()
	second := runOnce()
	if first == second {
		t.Fatalf("scoped tool call ids must differ across turns: first=%q second=%q", first, second)
	}
	if !strings.HasSuffix(first, ":call-1") {
		t.Fatalf("first scoped id=%q want suffix :call-1", first)
	}
	if !strings.HasSuffix(second, ":call-1") {
		t.Fatalf("second scoped id=%q want suffix :call-1", second)
	}
}

func TestService_ExecuteFailsOnMissingCommand(t *testing.T) {
	rt := &testHarnessRuntime{
		startSpec: harness.StartSpec{
			Command: "definitely_missing_agen8_harness_binary",
		},
	}
	runner, err := New(Config{Runtime: rt})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	events := collectEvents(runner.Execute(context.Background(), TurnConfig{Model: "test-model"}, TurnInput{Instruction: "hello"}))
	last := events[len(events)-1]
	if last.Kind != EventError {
		t.Fatalf("last event kind=%v want error", last.Kind)
	}
	if last.Err == nil || !strings.Contains(last.Err.Error(), "start process") {
		t.Fatalf("error=%v want start process failure", last.Err)
	}
}

func TestService_ExecuteCodexUsesNPXWhenCodexMissing(t *testing.T) {
	prevLookup := lookupCommandPath
	lookupCommandPath = func(file string) (string, error) {
		switch strings.TrimSpace(file) {
		case "codex":
			return "", exec.ErrNotFound
		case "npx":
			return "/usr/bin/npx", nil
		default:
			return "", exec.ErrNotFound
		}
	}
	defer func() { lookupCommandPath = prevLookup }()

	rt := &testHarnessRuntime{
		kind: "codex",
		startSpec: harness.StartSpec{
			Command: "definitely_missing_agen8_harness_binary",
		},
	}
	runner, err := New(Config{Runtime: rt})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	events := collectEvents(runner.Execute(context.Background(), TurnConfig{Model: "test-model"}, TurnInput{Instruction: "hello"}))
	last := events[len(events)-1]
	if last.Kind != EventError {
		t.Fatalf("last event kind=%v want error", last.Kind)
	}
	if got := strings.TrimSpace(rt.lastStartParams.Command); got != "npx" {
		t.Fatalf("start command=%q want npx fallback", got)
	}
}

func TestService_ExecuteCodexFailsWhenNoBootstrapCommand(t *testing.T) {
	prevLookup := lookupCommandPath
	lookupCommandPath = func(_ string) (string, error) {
		return "", exec.ErrNotFound
	}
	defer func() { lookupCommandPath = prevLookup }()

	rt := &testHarnessRuntime{kind: "codex"}
	runner, err := New(Config{Runtime: rt})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	events := collectEvents(runner.Execute(context.Background(), TurnConfig{Model: "test-model"}, TurnInput{Instruction: "hello"}))
	last := events[len(events)-1]
	if last.Kind != EventError {
		t.Fatalf("last event kind=%v want error", last.Kind)
	}
	if last.Err == nil || !strings.Contains(last.Err.Error(), "install codex") {
		t.Fatalf("error=%v want install codex guidance", last.Err)
	}
	if strings.TrimSpace(rt.lastStartParams.Command) != "" {
		t.Fatalf("expected runtime start not invoked, got command=%q", rt.lastStartParams.Command)
	}
}

func TestService_ExecuteFailsOnMalformedStream(t *testing.T) {
	rt := &testHarnessRuntime{
		startSpec: harness.StartSpec{
			Command: "sh",
			Args:    []string{"-c", `cat >/dev/null; printf 'NOT_JSON\n'`},
		},
	}
	runner, err := New(Config{Runtime: rt})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	events := collectEvents(runner.Execute(context.Background(), TurnConfig{Model: "test-model"}, TurnInput{Instruction: "hello"}))
	last := events[len(events)-1]
	if last.Kind != EventError {
		t.Fatalf("last event kind=%v want error", last.Kind)
	}
	if last.Err == nil || !strings.Contains(last.Err.Error(), "parse events") {
		t.Fatalf("error=%v want parse events failure", last.Err)
	}
}

func TestService_ExecuteFailsOnTurnFailedEvent(t *testing.T) {
	rt := &testHarnessRuntime{
		startSpec: harness.StartSpec{
			Command: "sh",
			Args:    []string{"-c", `cat >/dev/null; printf '{"type":"turn_failed","error":"approval required"}\n'`},
		},
	}
	runner, err := New(Config{Runtime: rt})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	events := collectEvents(runner.Execute(context.Background(), TurnConfig{Model: "test-model"}, TurnInput{Instruction: "hello"}))
	if len(events) < 2 {
		t.Fatalf("events=%d want >=2", len(events))
	}
	if events[0].Kind != EventTurnFailed {
		t.Fatalf("first event kind=%v want harness turn failed", events[0].Kind)
	}
	if events[0].HarnessErr != "approval required" {
		t.Fatalf("harness error=%q want approval required", events[0].HarnessErr)
	}
	last := events[len(events)-1]
	if last.Kind != EventError {
		t.Fatalf("last event kind=%v want error", last.Kind)
	}
	if !IsTurnFailed(last.Err) {
		t.Fatalf("last error=%T %[1]v want TurnFailedError", last.Err)
	}
}

func TestService_ExecuteIgnoresModelResumeWarningTurnFailedEvent(t *testing.T) {
	rt := &testHarnessRuntime{
		startSpec: harness.StartSpec{
			Command: "sh",
			Args: []string{
				"-c",
				`cat >/dev/null; printf '{"type":"turn_failed","error":"This session was recorded with model ` + "`gpt-5.4-nano`" + ` but is resuming with ` + "`gpt-5.3-codex`" + `. Consider switching back to ` + "`gpt-5.4-nano`" + ` as it may affect Codex performance."}\n'; printf '{"type":"text","text":"continuing after warning"}\n'; printf '{"type":"turn_completed"}\n'`,
			},
		},
	}
	runner, err := New(Config{Runtime: rt})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	events := collectEvents(runner.Execute(context.Background(), TurnConfig{Model: "test-model"}, TurnInput{Instruction: "hello"}))
	if len(events) < 2 {
		t.Fatalf("events=%d want >=2", len(events))
	}
	last := events[len(events)-1]
	if last.Kind != EventDone {
		t.Fatalf("last event kind=%v want done", last.Kind)
	}
	if got := strings.TrimSpace(last.Result.Text); got != "continuing after warning" {
		t.Fatalf("result text=%q want continuation text", got)
	}
}

func TestService_ExecuteFailsOnWritePromptError(t *testing.T) {
	rt := &testHarnessRuntime{
		startSpec:      harness.StartSpec{Command: "sh", Args: []string{"-c", `cat >/dev/null`}},
		writePromptErr: context.Canceled,
	}
	runner, err := New(Config{Runtime: rt})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	events := collectEvents(runner.Execute(context.Background(), TurnConfig{Model: "test-model"}, TurnInput{Instruction: "hello"}))
	last := events[len(events)-1]
	if last.Kind != EventError {
		t.Fatalf("last event kind=%v want error", last.Kind)
	}
	if last.Err == nil || !strings.Contains(last.Err.Error(), "write prompt") {
		t.Fatalf("error=%v want write prompt failure", last.Err)
	}
}

func TestService_ExecuteEmitsSyntheticDiffEventsForShellEdits(t *testing.T) {
	repoDir := t.TempDir()
	writeFile(t, filepath.Join(repoDir, "notes.txt"), "original\n")
	runCmd(t, repoDir, "git", "init")
	runCmd(t, repoDir, "git", "config", "user.email", "test@example.com")
	runCmd(t, repoDir, "git", "config", "user.name", "Test User")
	runCmd(t, repoDir, "git", "config", "commit.gpgsign", "false")
	runCmd(t, repoDir, "git", "add", "notes.txt")
	runCmd(t, repoDir, "git", "commit", "-m", "initial")

	rt := &testHarnessRuntime{
		startSpec: harness.StartSpec{
			Command: "sh",
			Workdir: repoDir,
			Args: []string{
				"-c",
				`cat >/dev/null; printf '{"type":"tool_call","tool_call_id":"call-1","tool_name":"shell_exec","status":"in_progress","command":"echo updated > notes.txt"}\n'; sleep 0.2; printf 'updated\n' > notes.txt; printf '{"type":"tool_result","tool_call_id":"call-1","tool_name":"shell_exec","status":"completed","sourceType":"cli","result":"ok","text":"ok"}\n'; printf '{"type":"text","text":"done"}\n'; printf '{"type":"turn_completed"}\n'`,
			},
		},
	}
	runner, err := New(Config{
		Runtime: rt,
		Workdir: repoDir,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if len(runner.enrichers) == 0 {
		t.Fatal("expected at least one harness event enricher for git workdir")
	}

	events := collectEvents(runner.Execute(context.Background(), TurnConfig{Model: "test-model"}, TurnInput{Instruction: "edit the file"}))
	if len(events) < 4 {
		t.Fatalf("events=%d want >=4", len(events))
	}

	shellResultIdx := -1
	diffResultIdx := -1
	for i, ev := range events {
		if ev.Kind != EventToolResult {
			continue
		}
		var data map[string]string
		if err := json.Unmarshal(ev.RuntimeToolData, &data); err != nil {
			t.Fatalf("unmarshal tool result data: %v", err)
		}
		op := strings.TrimSpace(data["op"])
		if op == "" {
			op = strings.TrimSpace(ev.RuntimeToolName)
		}
		if op == "shell_exec" || op == "bash" {
			shellResultIdx = i
			if captureErr := strings.TrimSpace(data["diffCaptureError"]); captureErr != "" {
				t.Fatalf("unexpected diff capture error on shell event: %s", captureErr)
			}
			continue
		}
		if op != "edit_file" && op != "write_file" {
			continue
		}
		diffResultIdx = i
		if got := strings.TrimSpace(data["path"]); got != "notes.txt" {
			t.Fatalf("diff path=%q want notes.txt", got)
		}
		patch := strings.TrimSpace(data["patchPreview"])
		if !strings.Contains(patch, "notes.txt") {
			t.Fatalf("patch preview missing path, got: %q", patch)
		}
		if !strings.Contains(patch, "+updated") {
			t.Fatalf("patch preview missing added line, got: %q", patch)
		}
		if strings.TrimSpace(data["linesAdded"]) == "" {
			t.Fatalf("expected linesAdded in synthetic diff event, got %#v", data)
		}
	}
	if shellResultIdx < 0 {
		t.Fatalf("missing shell result event: %#v", events)
	}
	if diffResultIdx < 0 {
		t.Fatalf("missing synthetic diff event: %#v", events)
	}
	if diffResultIdx <= shellResultIdx {
		t.Fatalf("synthetic diff event index=%d should be after shell result index=%d", diffResultIdx, shellResultIdx)
	}
}

func TestService_ExecuteEmitsSyntheticDiffForIgnoredWorkspacePaths(t *testing.T) {
	repoDir := t.TempDir()
	writeFile(t, filepath.Join(repoDir, ".gitignore"), "workspace/\n.agen8/workspace/\n")
	runCmd(t, repoDir, "git", "init")
	runCmd(t, repoDir, "git", "config", "user.email", "test@example.com")
	runCmd(t, repoDir, "git", "config", "user.name", "Test User")
	runCmd(t, repoDir, "git", "config", "commit.gpgsign", "false")
	runCmd(t, repoDir, "git", "add", ".gitignore")
	runCmd(t, repoDir, "git", "commit", "-m", "initial")

	rt := &testHarnessRuntime{
		startSpec: harness.StartSpec{
			Command: "sh",
			Workdir: repoDir,
			Args: []string{
				"-c",
				`cat >/dev/null; printf '{"type":"tool_call","tool_call_id":"call-1","tool_name":"bash","status":"in_progress","command":"printf updated > workspace/report.md"}\n'; sleep 0.2; mkdir -p workspace; printf 'updated\n' > workspace/report.md; printf '{"type":"tool_result","tool_call_id":"call-1","tool_name":"bash","status":"completed","sourceType":"cli","result":"ok","text":"ok"}\n'; printf '{"type":"text","text":"done"}\n'; printf '{"type":"turn_completed"}\n'`,
			},
		},
	}
	runner, err := New(Config{
		Runtime: rt,
		Workdir: repoDir,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	events := collectEvents(runner.Execute(context.Background(), TurnConfig{Model: "test-model"}, TurnInput{Instruction: "edit workspace file"}))
	if len(events) < 4 {
		t.Fatalf("events=%d want >=4", len(events))
	}

	foundDiff := false
	for _, ev := range events {
		if ev.Kind != EventToolResult {
			continue
		}
		var data map[string]string
		if err := json.Unmarshal(ev.RuntimeToolData, &data); err != nil {
			t.Fatalf("unmarshal tool result data: %v", err)
		}
		op := strings.TrimSpace(data["op"])
		if op != "edit_file" && op != "write_file" {
			continue
		}
		if got := strings.TrimSpace(data["path"]); got != "workspace/report.md" {
			continue
		}
		if !strings.Contains(strings.TrimSpace(data["patchPreview"]), "workspace/report.md") {
			t.Fatalf("patch preview missing workspace path: %q", data["patchPreview"])
		}
		foundDiff = true
	}
	if !foundDiff {
		t.Fatalf("missing synthetic diff event for ignored workspace path; events=%#v", events)
	}
}

func collectEvents(ch <-chan Event) []Event {
	out := []Event{}
	for ev := range ch {
		out = append(out, ev)
	}
	return out
}

func finalLoopText(events []Event) string {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Kind == EventDone {
			return strings.TrimSpace(events[i].Result.Text)
		}
	}
	return ""
}

func runCmd(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s failed: %v (%s)", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}
