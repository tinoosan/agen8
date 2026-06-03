package codex

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/tinoosan/agen8-mcp-server/internal/services/harness/domain"
)

func TestStart_BuildsIdentityAndMCPArgs(t *testing.T) {
	spec, err := buildAppServerSpec(domain.StartParams{
		SystemPrompt:    "You are backend-engineer",
		Model:           "openai/gpt-5.3-codex",
		ReasoningEffort: "high",
		MCPServers:      []string{"mcp_servers.docs.command=\"docs-server\""},
		ExtraArgs:       []string{"--enable", "experimental"},
	}, "ws://127.0.0.1:9999")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if spec.Command != "codex" {
		t.Fatalf("command=%q want codex", spec.Command)
	}
	args := strings.Join(spec.Args, " ")
	if !strings.Contains(args, "app-server --listen ws://127.0.0.1:9999") {
		t.Fatalf("missing app-server args: %v", spec.Args)
	}
	if !strings.Contains(args, `--config model="gpt-5.3-codex"`) {
		t.Fatalf("missing model override: %v", spec.Args)
	}
	if !strings.Contains(args, `--config model_reasoning_effort="high"`) {
		t.Fatalf("missing reasoning effort override: %v", spec.Args)
	}
	if !strings.Contains(args, `--config approval_policy="never"`) {
		t.Fatalf("missing approval policy override: %v", spec.Args)
	}
	if !strings.Contains(args, `--config sandbox_mode="danger-full-access"`) {
		t.Fatalf("missing sandbox mode override: %v", spec.Args)
	}
	if !strings.Contains(args, `--config mcp_servers.docs.command="docs-server"`) {
		t.Fatalf("missing mcp config override: %v", spec.Args)
	}
}

func TestStart_DropsCodexHTTPMCPTypeForAgen8URL(t *testing.T) {
	spec, err := buildAppServerSpec(domain.StartParams{
		MCPServers: []string{
			`mcp_servers.agen8.type="http"`,
			`mcp_servers.agen8.url="http://127.0.0.1:7777/mcp?token=abc"`,
		},
	}, "ws://127.0.0.1:9999")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	args := strings.Join(spec.Args, " ")
	if strings.Contains(args, `mcp_servers.agen8.type`) {
		t.Fatalf("unexpected mcp type override: %v", spec.Args)
	}
	if !strings.Contains(args, `--config mcp_servers.agen8.url="http://127.0.0.1:7777/mcp?token=abc"`) {
		t.Fatalf("missing mcp url override: %v", spec.Args)
	}
}

func TestStart_FailsLoudBecauseExecTransportRemoved(t *testing.T) {
	rt := New()
	_, err := rt.Start(domain.StartParams{})
	if err == nil || !strings.Contains(err.Error(), "exec transport has been removed") {
		t.Fatalf("expected removed exec transport error, got %v", err)
	}
}

func TestExecuteSessionTurnUsesProvidedAppServerURL(t *testing.T) {
	started := make(chan struct{}, 1)
	headers := make(chan http.Header, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case headers <- r.Header.Clone():
		default:
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept websocket: %v", err)
			return
		}
		defer conn.CloseNow()
		for {
			var req jsonrpcMessage
			if err := wsjson.Read(r.Context(), conn, &req); err != nil {
				return
			}
			switch req.Method {
			case "initialize":
				if err := wsjson.Write(r.Context(), conn, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{}}); err != nil {
					t.Errorf("write initialize: %v", err)
					return
				}
			case "initialized":
				continue
			case "thread/start":
				if err := wsjson.Write(r.Context(), conn, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"thread": map[string]any{"id": "thread-remote"}}}); err != nil {
					t.Errorf("write thread/start: %v", err)
					return
				}
			case "turn/start":
				started <- struct{}{}
				if err := wsjson.Write(r.Context(), conn, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"turn": map[string]any{"id": "turn-remote"}}}); err != nil {
					t.Errorf("write turn/start: %v", err)
					return
				}
				if err := wsjson.Write(r.Context(), conn, map[string]any{
					"jsonrpc": "2.0",
					"method":  "turn/completed",
					"params":  map[string]any{"threadId": "thread-remote", "turnId": "turn-remote"},
				}); err != nil {
					t.Errorf("write turn completed: %v", err)
					return
				}
			default:
				t.Errorf("unexpected method %q", req.Method)
				return
			}
		}
	}))
	defer server.Close()

	rt := New()
	_, err := rt.ExecuteSessionTurn(context.Background(), domain.StartParams{
		AppServerURL:    "ws" + strings.TrimPrefix(server.URL, "http"),
		Workdir:         "/srv/playground",
		Model:           "openai/gpt-5.5",
		ReasoningEffort: "medium",
		MCPServers:      []string{`mcp_servers.agen8.url="http://127.0.0.1:38123/mcp?token=abc"`},
	}, domain.SessionTurnInput{TurnID: "turn-1", Text: "hello"}, nil)
	if err != nil {
		t.Fatalf("ExecuteSessionTurn: %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for turn/start")
	}
	select {
	case gotHeader := <-headers:
		got := gotHeader.Values("X-Agen8-Codex-MCP-Config")
		want := []string{`mcp_servers.agen8.url="http://127.0.0.1:38123/mcp?token=abc"`}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("mcp config header=%q", got)
		}
		got = gotHeader.Values("X-Agen8-Codex-Config")
		want = []string{`model="gpt-5.5"`, `model_reasoning_effort="medium"`, `approval_policy="never"`, `sandbox_mode="danger-full-access"`}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("codex config header=%q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for websocket headers")
	}
}

func TestExecuteSessionTurnCapturesRawWebSearchEventMsg(t *testing.T) {
	eventsCh := make(chan domain.Event, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept websocket: %v", err)
			return
		}
		defer conn.CloseNow()
		for {
			var req jsonrpcMessage
			if err := wsjson.Read(r.Context(), conn, &req); err != nil {
				return
			}
			switch req.Method {
			case "initialize":
				if err := wsjson.Write(r.Context(), conn, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{}}); err != nil {
					t.Errorf("write initialize: %v", err)
					return
				}
			case "initialized":
				continue
			case "thread/start":
				if err := wsjson.Write(r.Context(), conn, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"thread": map[string]any{"id": "thread-raw"}}}); err != nil {
					t.Errorf("write thread/start: %v", err)
					return
				}
			case "turn/start":
				if err := wsjson.Write(r.Context(), conn, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"turn": map[string]any{"id": "turn-raw"}}}); err != nil {
					t.Errorf("write turn/start: %v", err)
					return
				}
				if err := wsjson.Write(r.Context(), conn, map[string]any{
					"jsonrpc": "2.0",
					"method":  "event_msg",
					"params": map[string]any{
						"payload": map[string]any{
							"type":    "web_search_end",
							"call_id": "ws-1",
							"query":   "Example Domain",
							"action": map[string]any{
								"type":    "search",
								"query":   "Example Domain",
								"queries": []any{"Example Domain"},
							},
							"results": []any{
								map[string]any{
									"title": "Example Domain",
									"url":   "https://example.com/",
								},
							},
						},
					},
				}); err != nil {
					t.Errorf("write raw event: %v", err)
					return
				}
				if err := wsjson.Write(r.Context(), conn, map[string]any{
					"jsonrpc": "2.0",
					"method":  "turn/completed",
					"params":  map[string]any{"threadId": "thread-raw", "turnId": "turn-raw"},
				}); err != nil {
					t.Errorf("write turn completed: %v", err)
					return
				}
			default:
				t.Errorf("unexpected method %q", req.Method)
				return
			}
		}
	}))
	defer server.Close()

	rt := New()
	_, err := rt.ExecuteSessionTurn(context.Background(), domain.StartParams{
		AppServerURL: "ws" + strings.TrimPrefix(server.URL, "http"),
		Workdir:      "/srv/playground",
	}, domain.SessionTurnInput{TurnID: "turn-1", Text: "search"}, func(ev domain.Event) {
		eventsCh <- ev
	})
	if err != nil {
		t.Fatalf("ExecuteSessionTurn: %v", err)
	}

	var webEvent domain.Event
	for {
		select {
		case ev := <-eventsCh:
			if ev.Type == domain.EventToolResult && ev.ToolName == "web_search" {
				webEvent = ev
			}
		default:
			if webEvent.Type == "" {
				t.Fatalf("missing web search event")
			}
			if webEvent.TurnID != "turn-raw" {
				t.Fatalf("web event turn=%q want turn-raw: %+v", webEvent.TurnID, webEvent)
			}
			if webEvent.ToolCallID != "ws-1" {
				t.Fatalf("tool call id=%q want ws-1", webEvent.ToolCallID)
			}
			if webEvent.Data["op"] != "web_search" || webEvent.Data["query"] != "Example Domain" {
				t.Fatalf("web event data mismatch: %+v", webEvent.Data)
			}
			if !strings.Contains(webEvent.Data["input"], "Example Domain") {
				t.Fatalf("input not preserved: %+v", webEvent.Data)
			}
			if !strings.Contains(webEvent.Data["result"], "https://example.com/") {
				t.Fatalf("web results not preserved: %+v", webEvent.Data)
			}
			return
		}
	}
}

func TestExecuteSessionTurnReusesLiveAppServerForFollowUp(t *testing.T) {
	var mu sync.Mutex
	connections := 0
	turns := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept websocket: %v", err)
			return
		}
		defer conn.CloseNow()
		mu.Lock()
		connections++
		mu.Unlock()
		for {
			var req jsonrpcMessage
			if err := wsjson.Read(r.Context(), conn, &req); err != nil {
				return
			}
			switch req.Method {
			case "initialize":
				if err := wsjson.Write(r.Context(), conn, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{}}); err != nil {
					t.Errorf("write initialize: %v", err)
					return
				}
			case "initialized":
				continue
			case "thread/start":
				if err := wsjson.Write(r.Context(), conn, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"thread": map[string]any{"id": "thread-remote"}}}); err != nil {
					t.Errorf("write thread/start: %v", err)
					return
				}
			case "thread/resume":
				if err := wsjson.Write(r.Context(), conn, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"thread": map[string]any{"id": "thread-remote"}}}); err != nil {
					t.Errorf("write thread/resume: %v", err)
					return
				}
			case "turn/start":
				mu.Lock()
				turns++
				turnID := fmt.Sprintf("turn-%d", turns)
				mu.Unlock()
				if err := wsjson.Write(r.Context(), conn, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"turn": map[string]any{"id": turnID}}}); err != nil {
					t.Errorf("write turn/start: %v", err)
					return
				}
				if err := wsjson.Write(r.Context(), conn, map[string]any{
					"jsonrpc": "2.0",
					"method":  "turn/completed",
					"params":  map[string]any{"threadId": "thread-remote", "turnId": turnID},
				}); err != nil {
					t.Errorf("write turn completed: %v", err)
					return
				}
			default:
				t.Errorf("unexpected method %q", req.Method)
				return
			}
		}
	}))
	defer server.Close()

	rt := New()
	sessionRef := ""
	params := domain.StartParams{
		AppServerURL: "ws" + strings.TrimPrefix(server.URL, "http"),
		Workdir:      "/srv/playground",
		PersistSessionRef: func(next string) error {
			sessionRef = next
			return nil
		},
	}
	_, err := rt.ExecuteSessionTurn(context.Background(), params, domain.SessionTurnInput{TurnID: "turn-1", Text: "first"}, nil)
	if err != nil {
		t.Fatalf("first ExecuteSessionTurn: %v", err)
	}
	if sessionRef != "thread-remote" {
		t.Fatalf("sessionRef=%q want thread-remote", sessionRef)
	}
	params.SessionRef = sessionRef
	params.Continue = true
	_, err = rt.ExecuteSessionTurn(context.Background(), params, domain.SessionTurnInput{TurnID: "turn-2", Text: "follow up"}, nil)
	if err != nil {
		t.Fatalf("follow-up ExecuteSessionTurn: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if connections != 1 {
		t.Fatalf("connections=%d want 1", connections)
	}
	if turns != 2 {
		t.Fatalf("turns=%d want 2", turns)
	}
}

func TestExecuteSessionTurnReconnectsWhenRuntimeConfigChanges(t *testing.T) {
	var mu sync.Mutex
	connections := 0
	resumes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept websocket: %v", err)
			return
		}
		defer conn.CloseNow()
		mu.Lock()
		connections++
		mu.Unlock()
		for {
			var req jsonrpcMessage
			if err := wsjson.Read(r.Context(), conn, &req); err != nil {
				return
			}
			switch req.Method {
			case "initialize":
				if err := wsjson.Write(r.Context(), conn, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{}}); err != nil {
					t.Errorf("write initialize: %v", err)
					return
				}
			case "initialized":
				continue
			case "thread/start":
				if err := wsjson.Write(r.Context(), conn, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"thread": map[string]any{"id": "thread-remote"}}}); err != nil {
					t.Errorf("write thread/start: %v", err)
					return
				}
			case "thread/resume":
				mu.Lock()
				resumes++
				mu.Unlock()
				if err := wsjson.Write(r.Context(), conn, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"thread": map[string]any{"id": "thread-remote"}}}); err != nil {
					t.Errorf("write thread/resume: %v", err)
					return
				}
			case "turn/start":
				if err := wsjson.Write(r.Context(), conn, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"turn": map[string]any{"id": "turn-remote"}}}); err != nil {
					t.Errorf("write turn/start: %v", err)
					return
				}
				if err := wsjson.Write(r.Context(), conn, map[string]any{
					"jsonrpc": "2.0",
					"method":  "turn/completed",
					"params":  map[string]any{"threadId": "thread-remote", "turnId": "turn-remote"},
				}); err != nil {
					t.Errorf("write turn completed: %v", err)
					return
				}
			default:
				t.Errorf("unexpected method %q", req.Method)
				return
			}
		}
	}))
	defer server.Close()

	rt := New()
	sessionRef := ""
	params := domain.StartParams{
		AppServerURL:    "ws" + strings.TrimPrefix(server.URL, "http"),
		Model:           "gpt-5.4",
		ReasoningEffort: "medium",
		PersistSessionRef: func(next string) error {
			sessionRef = next
			return nil
		},
	}
	_, err := rt.ExecuteSessionTurn(context.Background(), params, domain.SessionTurnInput{TurnID: "turn-1", Text: "first"}, nil)
	if err != nil {
		t.Fatalf("first ExecuteSessionTurn: %v", err)
	}
	params.SessionRef = sessionRef
	params.Continue = true
	params.Model = "gpt-5.5"
	params.ReasoningEffort = "low"
	_, err = rt.ExecuteSessionTurn(context.Background(), params, domain.SessionTurnInput{TurnID: "turn-2", Text: "follow up"}, nil)
	if err != nil {
		t.Fatalf("follow-up ExecuteSessionTurn: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if connections != 2 {
		t.Fatalf("connections=%d want 2", connections)
	}
	if resumes != 1 {
		t.Fatalf("resumes=%d want 1", resumes)
	}
}

func TestExecuteSessionTurnReconnectsClosedAppServerForFollowUp(t *testing.T) {
	var mu sync.Mutex
	connections := 0
	resumes := 0
	turns := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept websocket: %v", err)
			return
		}
		defer conn.CloseNow()
		mu.Lock()
		connections++
		connection := connections
		mu.Unlock()
		for {
			var req jsonrpcMessage
			if err := wsjson.Read(r.Context(), conn, &req); err != nil {
				return
			}
			switch req.Method {
			case "initialize":
				if err := wsjson.Write(r.Context(), conn, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{}}); err != nil {
					t.Errorf("write initialize: %v", err)
					return
				}
			case "initialized":
				continue
			case "thread/start":
				if err := wsjson.Write(r.Context(), conn, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"thread": map[string]any{"id": "thread-remote"}}}); err != nil {
					t.Errorf("write thread/start: %v", err)
					return
				}
			case "thread/resume":
				mu.Lock()
				resumes++
				mu.Unlock()
				if err := wsjson.Write(r.Context(), conn, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"thread": map[string]any{"id": "thread-remote"}}}); err != nil {
					t.Errorf("write thread/resume: %v", err)
					return
				}
			case "turn/start":
				mu.Lock()
				turns++
				turnID := fmt.Sprintf("turn-%d", turns)
				mu.Unlock()
				if err := wsjson.Write(r.Context(), conn, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"turn": map[string]any{"id": turnID}}}); err != nil {
					t.Errorf("write turn/start: %v", err)
					return
				}
				if err := wsjson.Write(r.Context(), conn, map[string]any{
					"jsonrpc": "2.0",
					"method":  "turn/completed",
					"params":  map[string]any{"threadId": "thread-remote", "turnId": turnID},
				}); err != nil {
					t.Errorf("write turn completed: %v", err)
					return
				}
				if connection == 1 {
					time.Sleep(50 * time.Millisecond)
					return
				}
			default:
				t.Errorf("unexpected method %q", req.Method)
				return
			}
		}
	}))
	defer server.Close()

	rt := New()
	sessionRef := ""
	params := domain.StartParams{
		AppServerURL: "ws" + strings.TrimPrefix(server.URL, "http"),
		Workdir:      "/srv/playground",
		PersistSessionRef: func(next string) error {
			sessionRef = next
			return nil
		},
	}
	_, err := rt.ExecuteSessionTurn(context.Background(), params, domain.SessionTurnInput{TurnID: "turn-1", Text: "first"}, nil)
	if err != nil {
		t.Fatalf("first ExecuteSessionTurn: %v", err)
	}
	params.SessionRef = sessionRef
	params.Continue = true
	_, err = rt.ExecuteSessionTurn(context.Background(), params, domain.SessionTurnInput{TurnID: "turn-2", Text: "follow up"}, nil)
	if err != nil {
		t.Fatalf("follow-up ExecuteSessionTurn: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if connections != 2 {
		t.Fatalf("connections=%d want 2", connections)
	}
	if resumes != 1 {
		t.Fatalf("resumes=%d want 1", resumes)
	}
	if turns != 2 {
		t.Fatalf("turns=%d want 2", turns)
	}
}

func TestExecuteSessionTurnAcceptsAgentInboxHandoffWhenAppServerClosesAfterTurnStart(t *testing.T) {
	started := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept websocket: %v", err)
			return
		}
		defer conn.CloseNow()
		for {
			var req jsonrpcMessage
			if err := wsjson.Read(r.Context(), conn, &req); err != nil {
				return
			}
			switch req.Method {
			case "initialize":
				if err := wsjson.Write(r.Context(), conn, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{}}); err != nil {
					t.Errorf("write initialize: %v", err)
					return
				}
			case "initialized":
				continue
			case "thread/start":
				if err := wsjson.Write(r.Context(), conn, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"thread": map[string]any{"id": "thread-remote"}}}); err != nil {
					t.Errorf("write thread/start: %v", err)
					return
				}
			case "turn/start":
				started <- struct{}{}
				if err := wsjson.Write(r.Context(), conn, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"turn": map[string]any{"id": "turn-remote"}}}); err != nil {
					t.Errorf("write turn/start: %v", err)
				}
				return
			default:
				t.Errorf("unexpected method %q", req.Method)
				return
			}
		}
	}))
	defer server.Close()

	rt := New()
	_, err := rt.ExecuteSessionTurn(
		context.Background(),
		domain.StartParams{AppServerURL: "ws" + strings.TrimPrefix(server.URL, "http")},
		domain.SessionTurnInput{TurnID: "turn-agent_inbox_msg-1", Text: "deliver inbox message"},
		nil,
	)
	if err != nil {
		t.Fatalf("ExecuteSessionTurn: %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("expected turn/start")
	}
}

func TestExecuteSessionTurnAcceptsUserChatWhenAppServerClosesAfterAssistantProgress(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept websocket: %v", err)
			return
		}
		defer conn.CloseNow()
		for {
			var req jsonrpcMessage
			if err := wsjson.Read(r.Context(), conn, &req); err != nil {
				return
			}
			switch req.Method {
			case "initialize":
				if err := wsjson.Write(r.Context(), conn, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{}}); err != nil {
					t.Errorf("write initialize: %v", err)
					return
				}
			case "initialized":
				continue
			case "thread/start":
				if err := wsjson.Write(r.Context(), conn, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"thread": map[string]any{"id": "thread-remote"}}}); err != nil {
					t.Errorf("write thread/start: %v", err)
					return
				}
			case "turn/start":
				if err := wsjson.Write(r.Context(), conn, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"turn": map[string]any{"id": "turn-remote"}}}); err != nil {
					t.Errorf("write turn/start: %v", err)
					return
				}
				if err := wsjson.Write(r.Context(), conn, map[string]any{
					"jsonrpc": "2.0",
					"method":  "item/agentMessage/delta",
					"params":  map[string]any{"threadId": "thread-remote", "turnId": "turn-remote", "delta": "message delivered"},
				}); err != nil {
					t.Errorf("write assistant delta: %v", err)
				}
				return
			default:
				t.Errorf("unexpected method %q", req.Method)
				return
			}
		}
	}))
	defer server.Close()

	rt := New()
	var events []domain.Event
	_, err := rt.ExecuteSessionTurn(
		context.Background(),
		domain.StartParams{AppServerURL: "ws" + strings.TrimPrefix(server.URL, "http")},
		domain.SessionTurnInput{TurnID: "turn-conversation-1", Text: "normal chat"},
		func(ev domain.Event) { events = append(events, ev) },
	)
	if err != nil {
		t.Fatalf("ExecuteSessionTurn: %v", err)
	}
	textEvents := []domain.Event{}
	for _, ev := range events {
		if ev.Type == domain.EventText {
			textEvents = append(textEvents, ev)
		}
	}
	if len(textEvents) != 1 || textEvents[0].Text != "message delivered" {
		t.Fatalf("events=%+v", events)
	}
}

func TestExecuteSessionTurnFailsUserChatWhenAppServerClosesWithoutProgress(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept websocket: %v", err)
			return
		}
		defer conn.CloseNow()
		for {
			var req jsonrpcMessage
			if err := wsjson.Read(r.Context(), conn, &req); err != nil {
				return
			}
			switch req.Method {
			case "initialize":
				if err := wsjson.Write(r.Context(), conn, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{}}); err != nil {
					t.Errorf("write initialize: %v", err)
					return
				}
			case "initialized":
				continue
			case "thread/start":
				if err := wsjson.Write(r.Context(), conn, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"thread": map[string]any{"id": "thread-remote"}}}); err != nil {
					t.Errorf("write thread/start: %v", err)
					return
				}
			case "turn/start":
				if err := wsjson.Write(r.Context(), conn, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"turn": map[string]any{"id": "turn-remote"}}}); err != nil {
					t.Errorf("write turn/start: %v", err)
				}
				return
			default:
				t.Errorf("unexpected method %q", req.Method)
				return
			}
		}
	}))
	defer server.Close()

	rt := New()
	_, err := rt.ExecuteSessionTurn(
		context.Background(),
		domain.StartParams{AppServerURL: "ws" + strings.TrimPrefix(server.URL, "http")},
		domain.SessionTurnInput{TurnID: "turn-conversation-1", Text: "normal chat"},
		nil,
	)
	errText := strings.ToLower(fmt.Sprint(err))
	if err == nil || (!strings.Contains(errText, "notification stream closed") && !strings.Contains(errText, "failed to read json message")) {
		t.Fatalf("expected websocket EOF error for user chat, got %v", err)
	}
}

func TestExecuteSessionTurnIncludesBridgeErrorBodyOnBadHandshake(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "codex binary was not found for bridge process", http.StatusBadGateway)
	}))
	defer server.Close()

	rt := New()
	_, err := rt.ExecuteSessionTurn(context.Background(), domain.StartParams{
		AppServerURL: "ws" + strings.TrimPrefix(server.URL, "http"),
		Workdir:      "/srv/playground",
	}, domain.SessionTurnInput{TurnID: "turn-1", Text: "hello"}, nil)
	if err == nil {
		t.Fatalf("expected websocket dial error")
	}
	if !strings.Contains(err.Error(), "codex binary was not found for bridge process") {
		t.Fatalf("error did not include bridge response body: %v", err)
	}
}

func TestExecuteSessionTurnIncludesRuntimeHostDiagnosticsOnTurnFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept websocket: %v", err)
			return
		}
		defer conn.CloseNow()
		for {
			var req jsonrpcMessage
			if err := wsjson.Read(r.Context(), conn, &req); err != nil {
				return
			}
			switch req.Method {
			case "initialize":
				if err := wsjson.Write(r.Context(), conn, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{}}); err != nil {
					t.Errorf("write %s: %v", req.Method, err)
					return
				}
			case "initialized":
				continue
			case "thread/start":
				if err := wsjson.Write(r.Context(), conn, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"thread": map[string]any{"id": "thread-remote"}}}); err != nil {
					t.Errorf("write thread/start: %v", err)
					return
				}
			case "turn/start":
				if err := wsjson.Write(r.Context(), conn, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"turn": map[string]any{"id": "turn-remote"}}}); err != nil {
					t.Errorf("write turn/start: %v", err)
					return
				}
				if err := wsjson.Write(r.Context(), conn, map[string]any{
					"jsonrpc": "2.0",
					"method":  "error",
					"params":  map[string]any{"message": "model rejected"},
				}); err != nil {
					t.Errorf("write turn failed: %v", err)
					return
				}
			default:
				t.Errorf("unexpected method %q", req.Method)
				return
			}
		}
	}))
	defer server.Close()

	rt := New()
	_, err := rt.ExecuteSessionTurn(context.Background(), domain.StartParams{
		AppServerURL:           "ws" + strings.TrimPrefix(server.URL, "http"),
		Workdir:                "/srv/playground",
		RuntimeHostDiagnostics: `{"codexPath":"/home/dev/.local/bin/codex","codexVersion":"codex-cli 1.2.3"}`,
	}, domain.SessionTurnInput{TurnID: "turn-1", Text: "hello"}, nil)
	if err == nil {
		t.Fatalf("expected turn failure")
	}
	if !strings.Contains(err.Error(), "model rejected") || !strings.Contains(err.Error(), "/home/dev/.local/bin/codex") || !strings.Contains(err.Error(), "codex-cli 1.2.3") {
		t.Fatalf("error did not include runtime host diagnostics: %v", err)
	}
}

func TestNormalizeCodexCLIModel(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "openai prefixed", in: "openai/gpt-5.3-codex", want: "gpt-5.3-codex"},
		{name: "already normalized", in: "gpt-5.3-codex", want: "gpt-5.3-codex"},
		{name: "non-openai prefix unchanged", in: "custom/foo", want: "custom/foo"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeCodexCLIModel(tc.in); got != tc.want {
				t.Fatalf("normalizeCodexCLIModel(%q)=%q want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestStart_NPXBootstrapsCodexPackage(t *testing.T) {
	spec, err := buildAppServerSpec(domain.StartParams{
		Command: "npx",
	}, "ws://127.0.0.1:9999")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if spec.Command != "npx" {
		t.Fatalf("command=%q want npx", spec.Command)
	}
	if len(spec.Args) < 4 {
		t.Fatalf("args=%v want npx bootstrap args", spec.Args)
	}
	if spec.Args[0] != "-y" || spec.Args[1] != "@openai/codex" {
		t.Fatalf("missing npx bootstrap args: %v", spec.Args)
	}
	if spec.Args[2] != "app-server" || spec.Args[3] != "--listen" {
		t.Fatalf("missing app-server args after npx bootstrap: %v", spec.Args)
	}
}

func TestExecuteSessionTurnRetriesMalformedThreadResumeOnce(t *testing.T) {
	var mu sync.Mutex
	resumes := 0
	turns := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept websocket: %v", err)
			return
		}
		defer conn.CloseNow()
		for {
			var req jsonrpcMessage
			if err := wsjson.Read(r.Context(), conn, &req); err != nil {
				return
			}
			switch req.Method {
			case "initialize":
				if err := wsjson.Write(r.Context(), conn, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{}}); err != nil {
					t.Errorf("write initialize: %v", err)
					return
				}
			case "initialized":
				continue
			case "thread/resume":
				mu.Lock()
				resumes++
				resume := resumes
				mu.Unlock()
				if resume == 1 {
					if err := conn.Write(r.Context(), websocket.MessageText, []byte("{")); err != nil {
						t.Errorf("write malformed resume response: %v", err)
					}
					return
				}
				if err := wsjson.Write(r.Context(), conn, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"thread": map[string]any{"id": "thread-remote"}}}); err != nil {
					t.Errorf("write thread/resume: %v", err)
					return
				}
			case "turn/start":
				mu.Lock()
				turns++
				mu.Unlock()
				if err := wsjson.Write(r.Context(), conn, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"turn": map[string]any{"id": "turn-remote"}}}); err != nil {
					t.Errorf("write turn/start: %v", err)
					return
				}
				if err := wsjson.Write(r.Context(), conn, map[string]any{
					"jsonrpc": "2.0",
					"method":  "turn/completed",
					"params":  map[string]any{"threadId": "thread-remote", "turnId": "turn-remote"},
				}); err != nil {
					t.Errorf("write turn completed: %v", err)
					return
				}
			default:
				t.Errorf("unexpected method %q", req.Method)
				return
			}
		}
	}))
	defer server.Close()

	rt := New()
	_, err := rt.ExecuteSessionTurn(context.Background(), domain.StartParams{
		AppServerURL: "ws" + strings.TrimPrefix(server.URL, "http"),
		SessionRef:   "thread-remote",
		Workdir:      "/srv/playground",
	}, domain.SessionTurnInput{TurnID: "turn-1", Text: "hello"}, nil)
	if err != nil {
		t.Fatalf("ExecuteSessionTurn: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if resumes != 2 {
		t.Fatalf("thread/resume calls=%d want 2", resumes)
	}
	if turns != 1 {
		t.Fatalf("turn/start calls=%d want 1", turns)
	}
}

func TestStart_RejectsInvalidMCPOverride(t *testing.T) {
	_, err := buildAppServerSpec(domain.StartParams{MCPServers: []string{"docs-server"}}, "ws://127.0.0.1:9999")
	if err == nil || !strings.Contains(err.Error(), "must be a --config key=value expression") {
		t.Fatalf("expected invalid override error, got %v", err)
	}
}

func TestThreadResumeParams_DoesNotSendStartOnlyFields(t *testing.T) {
	params := threadResumeParams(domain.StartParams{
		Workdir:         "/repo",
		SystemPrompt:    "developer",
		Model:           "openai/gpt-5.3-codex",
		ReasoningEffort: "high",
	}, "thread-1")
	if params["threadId"] != "thread-1" {
		t.Fatalf("threadId=%v want thread-1", params["threadId"])
	}
	if _, ok := params["experimentalRawEvents"]; ok {
		t.Fatalf("resume params include unsupported experimentalRawEvents field: %+v", params)
	}
	if got := params["persistExtendedHistory"]; got != true {
		t.Fatalf("resume persistExtendedHistory=%v want true", got)
	}
	if _, ok := params["ephemeral"]; ok {
		t.Fatalf("resume params include start-only ephemeral field: %+v", params)
	}
}

func TestThreadResumeParamsIncludesRuntimeConfig(t *testing.T) {
	params := threadResumeParams(domain.StartParams{
		Model:           "openai/gpt-5.5",
		ReasoningEffort: "high",
	}, "thread-1")
	config, ok := params["config"].(map[string]any)
	if !ok {
		t.Fatalf("config=%T want map", params["config"])
	}
	if config["model"] != "gpt-5.5" {
		t.Fatalf("config.model=%v want gpt-5.5", config["model"])
	}
	if config["model_reasoning_effort"] != "high" {
		t.Fatalf("config.model_reasoning_effort=%v want high", config["model_reasoning_effort"])
	}
}

func TestThreadStartParams_PersistsExtendedHistory(t *testing.T) {
	params := threadStartParams(domain.StartParams{})
	if got := params["persistExtendedHistory"]; got != true {
		t.Fatalf("start persistExtendedHistory=%v want true", got)
	}
}

func TestThreadStartParams_CreatesMaterializedThread(t *testing.T) {
	params := threadStartParams(domain.StartParams{})
	if got, ok := params["ephemeral"].(bool); !ok || got {
		t.Fatalf("ephemeral=%v (%T) want false so Codex can resume after app-server restart", params["ephemeral"], params["ephemeral"])
	}
}

func TestThreadStartParamsIncludesRuntimeConfig(t *testing.T) {
	params := threadStartParams(domain.StartParams{
		Model:           "openai/gpt-5.5",
		ReasoningEffort: "low",
	})
	config, ok := params["config"].(map[string]any)
	if !ok {
		t.Fatalf("config=%T want map", params["config"])
	}
	if config["model"] != "gpt-5.5" {
		t.Fatalf("config.model=%v want gpt-5.5", config["model"])
	}
	if config["model_reasoning_effort"] != "low" {
		t.Fatalf("config.model_reasoning_effort=%v want low", config["model_reasoning_effort"])
	}
}

func TestThreadStartParams_DisablesInteractiveApprovals(t *testing.T) {
	params := threadStartParams(domain.StartParams{})
	if params["approvalPolicy"] != "never" {
		t.Fatalf("approvalPolicy=%v want never", params["approvalPolicy"])
	}
	if params["sandbox"] != "danger-full-access" {
		t.Fatalf("sandbox=%v want danger-full-access", params["sandbox"])
	}
	if _, ok := params["permissionProfile"]; ok {
		t.Fatalf("full access should use preset sandbox, not permissionProfile: %+v", params)
	}
}

func TestTurnStartParams_DisablesInteractiveApprovals(t *testing.T) {
	params := turnStartParams(domain.StartParams{Model: "openai/gpt-5.4", ReasoningEffort: "high"}, "thread-1", "hello", nil)
	if params["approvalPolicy"] != "never" {
		t.Fatalf("approvalPolicy=%v want never", params["approvalPolicy"])
	}
	sandbox, ok := params["sandboxPolicy"].(map[string]any)
	if !ok {
		t.Fatalf("sandboxPolicy=%T want map", params["sandboxPolicy"])
	}
	if sandbox["type"] != "dangerFullAccess" {
		t.Fatalf("sandboxPolicy.type=%v want dangerFullAccess", sandbox["type"])
	}
	if _, ok := params["permissionProfile"]; ok {
		t.Fatalf("full access should use preset sandboxPolicy, not permissionProfile: %+v", params)
	}
	if params["model"] != "gpt-5.4" {
		t.Fatalf("model=%v want gpt-5.4", params["model"])
	}
	if params["effort"] != "high" {
		t.Fatalf("effort=%v want high", params["effort"])
	}
}

func TestTurnStartParamsIncludesLocalImageAttachments(t *testing.T) {
	params := turnStartParams(domain.StartParams{}, "thread-1", "inspect this", []domain.PromptAttachment{{
		ID:        "attachment-1",
		Name:      "screen.png",
		MediaType: "image/png",
		URI:       "/tmp/screen.png",
	}})
	input, ok := params["input"].([]map[string]any)
	if !ok {
		t.Fatalf("input=%T want []map[string]any", params["input"])
	}
	if len(input) != 2 {
		t.Fatalf("input len=%d want 2: %+v", len(input), input)
	}
	if input[1]["type"] != "localImage" || input[1]["path"] != "/tmp/screen.png" {
		t.Fatalf("image input=%+v", input[1])
	}
}

func TestValidateResumedThreadIDRejectsChangedThread(t *testing.T) {
	_, err := validateResumedThreadID("thread-original", []byte(`{"thread":{"id":"thread-new"}}`))
	if err == nil || !strings.Contains(err.Error(), "thread-original") || !strings.Contains(err.Error(), "thread-new") {
		t.Fatalf("expected changed thread id error, got %v", err)
	}
}

func TestPersistAppServerThreadIDPersistsSynchronously(t *testing.T) {
	var persisted string
	err := persistAppServerThreadID(domain.StartParams{
		PersistSessionRef: func(sessionRef string) error {
			persisted = strings.TrimSpace(sessionRef)
			return nil
		},
	}, " thread-1 ")
	if err != nil {
		t.Fatalf("persistAppServerThreadID: %v", err)
	}
	if persisted != "thread-1" {
		t.Fatalf("persisted=%q want thread-1", persisted)
	}
}

func TestAppServerClient_DemuxesNotificationsDuringCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept websocket: %v", err)
			return
		}
		defer conn.CloseNow()
		var req jsonrpcMessage
		if err := wsjson.Read(r.Context(), conn, &req); err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		if err := wsjson.Write(r.Context(), conn, map[string]any{
			"jsonrpc": "2.0",
			"method":  "turn/started",
			"params":  map[string]any{"turn": map[string]any{"id": "turn-1"}},
		}); err != nil {
			t.Errorf("write notification: %v", err)
			return
		}
		if err := wsjson.Write(r.Context(), conn, map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result":  map[string]any{"ok": true},
		}); err != nil {
			t.Errorf("write response: %v", err)
			return
		}
	}))
	defer server.Close()

	conn, _, err := websocket.Dial(context.Background(), "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.CloseNow()
	client := newAppServerClient(conn, nil)
	defer client.close()

	result, err := client.call(context.Background(), "test/call", map[string]any{})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !strings.Contains(string(result), `"ok":true`) {
		t.Fatalf("result=%s want ok response", result)
	}

	select {
	case msg := <-client.notifications:
		if msg.Method != "turn/started" {
			t.Fatalf("notification method=%q want turn/started", msg.Method)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for interleaved notification")
	}
}

func TestAppServerClient_RejectsUnsupportedServerRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept websocket: %v", err)
			return
		}
		defer conn.CloseNow()
		if err := wsjson.Write(r.Context(), conn, map[string]any{
			"jsonrpc": "2.0",
			"id":      "server-request-1",
			"method":  "item/tool/call",
			"params":  map[string]any{"tool": "unknown"},
		}); err != nil {
			t.Errorf("write server request: %v", err)
			return
		}
		var resp struct {
			ID    string `json:"id"`
			Error *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := wsjson.Read(r.Context(), conn, &resp); err != nil {
			t.Errorf("read response: %v", err)
			return
		}
		if resp.ID != "server-request-1" {
			t.Errorf("id=%q want server-request-1", resp.ID)
		}
		if resp.Error == nil || resp.Error.Code != -32601 || !strings.Contains(resp.Error.Message, "item/tool/call") {
			t.Errorf("error=%+v want unsupported method response", resp.Error)
		}
	}))
	defer server.Close()

	conn, _, err := websocket.Dial(context.Background(), "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.CloseNow()
	client := newAppServerClient(conn, nil)
	defer client.close()

	select {
	case err := <-client.done:
		if err == nil {
			t.Fatal("connection closed without read error")
		}
	case <-time.After(time.Second):
	}
}

func TestAppServerClient_ApprovesServerApprovalRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept websocket: %v", err)
			return
		}
		defer conn.CloseNow()
		if err := wsjson.Write(r.Context(), conn, map[string]any{
			"jsonrpc": "2.0",
			"id":      "approval-1",
			"method":  "item/commandExecution/requestApproval",
			"params":  map[string]any{},
		}); err != nil {
			t.Errorf("write server request: %v", err)
			return
		}
		var resp struct {
			ID     string         `json:"id"`
			Result map[string]any `json:"result"`
			Error  any            `json:"error"`
		}
		if err := wsjson.Read(r.Context(), conn, &resp); err != nil {
			t.Errorf("read response: %v", err)
			return
		}
		if resp.ID != "approval-1" {
			t.Errorf("id=%q want approval-1", resp.ID)
		}
		if resp.Error != nil {
			t.Errorf("error=%v want nil", resp.Error)
		}
		if resp.Result["decision"] != "acceptForSession" {
			t.Errorf("decision=%v want acceptForSession", resp.Result["decision"])
		}
	}))
	defer server.Close()

	conn, _, err := websocket.Dial(context.Background(), "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.CloseNow()
	client := newAppServerClient(conn, func(_ context.Context, req domain.ApprovalRequest) (domain.ApprovalDecision, error) {
		if req.ApprovalID != "approval-1" {
			t.Errorf("approval id=%q want approval-1", req.ApprovalID)
		}
		if req.ToolName != "bash" {
			t.Errorf("tool name=%q want bash", req.ToolName)
		}
		return domain.ApprovalDecision{Decision: "approve"}, nil
	})
	defer client.close()

	select {
	case err := <-client.done:
		if err == nil {
			t.Fatal("connection closed without read error")
		}
	case <-time.After(time.Second):
	}
}

func TestAppServerNotificationEvents_PreservesDeltaWhitespace(t *testing.T) {
	events, err := appServerNotificationEvents(jsonrpcMessage{
		Method: "item/agentMessage/delta",
		Params: []byte(`{"turnId":"turn-1","delta":" operating here as the "}`),
	})
	if err != nil {
		t.Fatalf("appServerNotificationEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events len=%d want 1", len(events))
	}
	if got := events[0].Text; got != " operating here as the " {
		t.Fatalf("delta text=%q want leading and trailing spaces preserved", got)
	}
}

func TestAppServerNotificationEvents_IgnoresCompletedAgentMessageText(t *testing.T) {
	events, err := appServerNotificationEvents(jsonrpcMessage{
		Method: "item/completed",
		Params: []byte(`{"turnId":"turn-1","item":{"id":"msg-1","type":"agentMessage","text":"final assistant text"}}`),
	})
	if err != nil {
		t.Fatalf("appServerNotificationEvents: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("events len=%d want 0: %+v", len(events), events)
	}

	events, err = appServerNotificationEvents(jsonrpcMessage{
		Method: "item/completed",
		Params: []byte(`{"turnId":"turn-1","item":{"id":"msg-1","type":"agent_message","text":"final assistant text"}}`),
	})
	if err != nil {
		t.Fatalf("appServerNotificationEvents: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("events len=%d want 0: %+v", len(events), events)
	}
}

func TestAppServerNotificationEvents_MapsThreadStartedSessionRef(t *testing.T) {
	events, err := appServerNotificationEvents(jsonrpcMessage{
		Method: "thread/started",
		Params: []byte(`{"thread":{"id":"thread-1"}}`),
	})
	if err != nil {
		t.Fatalf("appServerNotificationEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events len=%d want 1", len(events))
	}
	if events[0].Type != domain.EventTurnStarted || events[0].SessionRef != "thread-1" {
		t.Fatalf("event mismatch: %+v", events[0])
	}
}

func TestAppServerNotificationEvents_MapsTopLevelTurnIDForTurnLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name   string
		method string
		want   domain.EventType
	}{
		{name: "started", method: "turn/started", want: domain.EventTurnStarted},
		{name: "completed", method: "turn/completed", want: domain.EventTurnCompleted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			events, err := appServerNotificationEvents(jsonrpcMessage{
				Method: tc.method,
				Params: []byte(`{"threadId":"thread-1","turnId":"turn-1"}`),
			})
			if err != nil {
				t.Fatalf("appServerNotificationEvents: %v", err)
			}
			if len(events) != 1 {
				t.Fatalf("events len=%d want 1", len(events))
			}
			if events[0].Type != tc.want || events[0].TurnID != "turn-1" {
				t.Fatalf("event mismatch: %+v", events[0])
			}
		})
	}
}

func TestAppServerNotificationEvents_MapsReasoningSummaryDelta(t *testing.T) {
	events, err := appServerNotificationEvents(jsonrpcMessage{
		Method: "item/reasoning/summaryTextDelta",
		Params: []byte(`{"turnId":"turn-1","itemId":"reason-1","summaryIndex":0,"delta":" Thinking"}`),
	})
	if err != nil {
		t.Fatalf("appServerNotificationEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events len=%d want 1", len(events))
	}
	ev := events[0]
	if ev.Type != domain.EventText || ev.TurnID != "turn-1" || ev.Text != " Thinking" {
		t.Fatalf("reasoning summary event mismatch: %+v", ev)
	}
	if ev.Data["kind"] != "reasoning" || ev.Data["itemId"] != "reason-1" || ev.Data["summaryIndex"] != "0" {
		t.Fatalf("reasoning summary data mismatch: %+v", ev.Data)
	}
}

func TestAppServerNotificationEvents_IgnoresRawReasoningTextDelta(t *testing.T) {
	events, err := appServerNotificationEvents(jsonrpcMessage{
		Method: "item/reasoning/textDelta",
		Params: []byte(`{"turnId":"turn-1","itemId":"reason-1","contentIndex":0,"delta":"private reasoning"}`),
	})
	if err != nil {
		t.Fatalf("appServerNotificationEvents: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("events len=%d want 0: %+v", len(events), events)
	}
}

func TestAppServerNotificationEvents_MapsCommandExecutionOutputDelta(t *testing.T) {
	events, err := appServerNotificationEvents(jsonrpcMessage{
		Method: "item/commandExecution/outputDelta",
		Params: []byte(`{"threadId":"thread-1","turnId":"turn-cmd-1","itemId":"cmd-1","delta":"stdout line\n"}`),
	})
	if err != nil {
		t.Fatalf("appServerNotificationEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events len=%d want 1", len(events))
	}
	ev := events[0]
	if ev.Type != domain.EventToolResult || ev.TurnID != "turn-cmd-1" || ev.ToolCallID != "cmd-1" || ev.ToolName != "bash" {
		t.Fatalf("event mismatch: %+v", ev)
	}
	if ev.Text != "stdout line\n" {
		t.Fatalf("text=%q want stdout delta", ev.Text)
	}
	if ev.Data["outputDelta"] != "true" || ev.Data["stdout"] != "stdout line\n" || ev.Data["status"] != "in_progress" {
		t.Fatalf("data mismatch: %+v", ev.Data)
	}
}

func TestAppServerNotificationEvents_CapturesCommandExecutionUpdateOutput(t *testing.T) {
	events, err := appServerNotificationEvents(jsonrpcMessage{
		Method: "item/updated",
		Params: []byte(`{
			"turnId":"turn-cmd-1",
			"item":{
				"id":"cmd-1",
				"type":"command_execution",
				"command":"/usr/bin/zsh -lc \"printf 'shell ok\\n'\"",
				"output":"shell ok\n"
			}
		}`),
	})
	if err != nil {
		t.Fatalf("appServerNotificationEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events len=%d want 1", len(events))
	}
	ev := events[0]
	if ev.Type != domain.EventToolResult || ev.TurnID != "turn-cmd-1" || ev.ToolCallID != "cmd-1" || ev.ToolName != "bash" {
		t.Fatalf("event mismatch: %+v", ev)
	}
	if ev.Data["status"] != "in_progress" || ev.Data["outputFull"] != "shell ok" || ev.Data["result"] != "shell ok" {
		t.Fatalf("data mismatch: %+v", ev.Data)
	}
}

func TestAppServerNotificationEvents_CapturesCommandExecutionAggregatedOutput(t *testing.T) {
	events, err := appServerNotificationEvents(jsonrpcMessage{
		Method: "item/completed",
		Params: []byte(`{
			"turnId":"turn-cmd-1",
			"item":{
				"id":"cmd-1",
				"type":"commandExecution",
				"command":"/usr/bin/zsh -lc \"printf 'shell ok\\n'\"",
				"processId":"4116283",
				"status":"completed",
				"aggregatedOutput":"shell ok\nLICENSE\nMakefile\n",
				"exitCode":0,
				"durationMs":1
			}
		}`),
	})
	if err != nil {
		t.Fatalf("appServerNotificationEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events len=%d want 1", len(events))
	}
	ev := events[0]
	if ev.Type != domain.EventToolResult || ev.TurnID != "turn-cmd-1" || ev.ToolCallID != "cmd-1" || ev.ToolName != "bash" {
		t.Fatalf("event mismatch: %+v", ev)
	}
	if ev.Data["status"] != "completed" || ev.Data["outputFull"] != "shell ok\nLICENSE\nMakefile" || ev.Data["result"] != "shell ok\nLICENSE\nMakefile" {
		t.Fatalf("data mismatch: %+v", ev.Data)
	}
	if ev.Text != "shell ok\nLICENSE\nMakefile" {
		t.Fatalf("text=%q want aggregated output", ev.Text)
	}
}

func TestAppServerNotificationEvents_CapturesGenericCodexToolItems(t *testing.T) {
	events, err := appServerNotificationEvents(jsonrpcMessage{
		Method: "item/completed",
		Params: []byte(`{
			"turnId":"turn-browser-1",
			"item":{
				"id":"call-web-1",
				"type":"function_call",
				"name":"web.run",
				"status":"completed",
				"arguments":{"search_query":[{"q":"example domain"}]},
				"result":{"search_query":[{"title":"Example Domain","url":"https://example.com"}]}
			}
		}`),
	})
	if err != nil {
		t.Fatalf("appServerNotificationEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events len=%d want 1", len(events))
	}
	ev := events[0]
	if ev.Type != domain.EventToolResult {
		t.Fatalf("event type=%q want %q: %+v", ev.Type, domain.EventToolResult, ev)
	}
	if ev.TurnID != "turn-browser-1" || ev.ToolCallID != "call-web-1" || ev.ToolName != "web.run" {
		t.Fatalf("event identity mismatch: %+v", ev)
	}
	if got := ev.Data["codexItemType"]; got != "function_call" {
		t.Fatalf("codex item type=%q", got)
	}
	if got := ev.Data["op"]; got != "web_search" {
		t.Fatalf("op=%q want web_search", got)
	}
	if !strings.Contains(ev.Data["input"], "example domain") {
		t.Fatalf("input not preserved: %+v", ev.Data)
	}
	if !strings.Contains(ev.Data["result"], "Example Domain") {
		t.Fatalf("result not preserved: %+v", ev.Data)
	}
}

func TestAppServerNotificationEvents_NormalizesCodexSpecificToolNames(t *testing.T) {
	for _, tc := range []struct {
		name string
		tool string
		want string
	}{
		{name: "tool search", tool: "ToolSearch", want: "tool_search"},
		{name: "exec command", tool: "functions.exec_command", want: "bash"},
		{name: "apply patch", tool: "functions.apply_patch", want: "edit_file"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params := fmt.Sprintf(`{
				"turnId":"turn-1",
				"item":{
					"id":"call-1",
					"type":"function_call",
					"name":%q,
					"status":"completed",
					"arguments":{"query":"example"},
					"result":{"ok":true}
				}
			}`, tc.tool)
			events, err := appServerNotificationEvents(jsonrpcMessage{
				Method: "item/completed",
				Params: []byte(params),
			})
			if err != nil {
				t.Fatalf("appServerNotificationEvents: %v", err)
			}
			if len(events) != 1 {
				t.Fatalf("events len=%d want 1", len(events))
			}
			if got := events[0].Data["op"]; got != tc.want {
				t.Fatalf("op=%q want %q: %+v", got, tc.want, events[0])
			}
		})
	}
}

func TestAppServerNotificationEvents_CapturesWebSearchThreadItem(t *testing.T) {
	events, err := appServerNotificationEvents(jsonrpcMessage{
		Method: "item/completed",
		Params: []byte(`{
			"threadId":"thread-1",
			"turnId":"turn-1",
			"item":{
				"id":"web-search-1",
				"type":"webSearch",
				"query":"gluetun v3.39.0 release notes",
				"action":{
					"type":"search",
					"query":"gluetun v3.39.0 release notes",
					"queries":["gluetun v3.39.0 release notes"]
				},
				"results":[
					{"title":"Release v3.39.0 · qdm12/gluetun","url":"https://github.com/qdm12/gluetun/releases/tag/v3.39.0"}
				]
			}
		}`),
	})
	if err != nil {
		t.Fatalf("appServerNotificationEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events len=%d want 1", len(events))
	}
	ev := events[0]
	if ev.Type != domain.EventToolResult || ev.TurnID != "turn-1" || ev.ToolCallID != "web-search-1" || ev.ToolName != "web_search" {
		t.Fatalf("event mismatch: %+v", ev)
	}
	if ev.Data["op"] != "web_search" || ev.Data["query"] != "gluetun v3.39.0 release notes" || ev.Data["status"] != "completed" {
		t.Fatalf("data mismatch: %+v", ev.Data)
	}
	if !strings.Contains(ev.Data["input"], "gluetun v3.39.0 release notes") {
		t.Fatalf("input not preserved: %+v", ev.Data)
	}
	if !strings.Contains(ev.Data["result"], "https://github.com/qdm12/gluetun/releases/tag/v3.39.0") {
		t.Fatalf("results not preserved: %+v", ev.Data)
	}
}

func TestAppServerNotificationEvents_CapturesImageGenerationThreadItem(t *testing.T) {
	events, err := appServerNotificationEvents(jsonrpcMessage{
		Method: "item/completed",
		Params: []byte(`{
			"threadId":"thread-1",
			"turnId":"turn-1",
			"item":{
				"id":"image-gen-1",
				"type":"image_generation_call",
				"prompt":"draw a small red square",
				"revised_prompt":"A clean centered red square on a white background.",
				"output_format":"png",
				"size":"1024x1024",
				"result":"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII="
			}
		}`),
	})
	if err != nil {
		t.Fatalf("appServerNotificationEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events len=%d want 1", len(events))
	}
	ev := events[0]
	if ev.Type != domain.EventToolResult || ev.TurnID != "turn-1" || ev.ToolCallID != "image-gen-1" || ev.ToolName != "image_generation" {
		t.Fatalf("event mismatch: %+v", ev)
	}
	if ev.Data["op"] != "image_generation" || ev.Data["prompt"] != "draw a small red square" || ev.Data["status"] != "completed" {
		t.Fatalf("data mismatch: %+v", ev.Data)
	}
	if ev.Data["imageB64"] == "" || ev.Data["mimeType"] != "image/png" || ev.Data["revisedPrompt"] == "" {
		t.Fatalf("image payload not preserved: %+v", ev.Data)
	}
	if !strings.Contains(ev.Data["input"], "draw a small red square") {
		t.Fatalf("input not preserved: %+v", ev.Data)
	}
}

func TestAppServerNotificationEvents_TreatsImagePayloadAsCompleted(t *testing.T) {
	events, err := appServerNotificationEvents(jsonrpcMessage{
		Method: "item/updated",
		Params: []byte(`{
			"threadId":"thread-1",
			"turnId":"turn-1",
			"item":{
				"id":"image-gen-1",
				"type":"imageGeneration",
				"status":"generating",
				"prompt":"draw a small red square",
				"outputFormat":"png",
				"imageB64":"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII="
			}
		}`),
	})
	if err != nil {
		t.Fatalf("appServerNotificationEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events len=%d want 1", len(events))
	}
	ev := events[0]
	if ev.Type != domain.EventToolResult {
		t.Fatalf("event type=%s want tool result: %+v", ev.Type, ev)
	}
	if ev.Data["status"] != "completed" || ev.Data["imageB64"] == "" {
		t.Fatalf("payload-bearing image event should be completed: %+v", ev.Data)
	}
}

func TestAppServerNotificationEvents_MapsCompletedReasoningSummaryItem(t *testing.T) {
	events, err := appServerNotificationEvents(jsonrpcMessage{
		Method: "item/completed",
		Params: []byte(`{"threadId":"thread-1","turnId":"turn-1","item":{"type":"reasoning","id":"reason-1","summary":["Planning","Checking"]}}`),
	})
	if err != nil {
		t.Fatalf("appServerNotificationEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events len=%d want 1", len(events))
	}
	ev := events[0]
	if ev.Type != domain.EventText || ev.TurnID != "turn-1" || ev.Text != "Planning\nChecking" {
		t.Fatalf("completed reasoning event mismatch: %+v", ev)
	}
	if ev.Data["kind"] != "reasoning" || ev.Data["itemId"] != "reason-1" {
		t.Fatalf("completed reasoning data mismatch: %+v", ev.Data)
	}
}

func TestAppServerNotificationEvents_MapsContextUsage(t *testing.T) {
	events, err := appServerNotificationEvents(jsonrpcMessage{
		Method: "thread/tokenUsage/updated",
		Params: []byte(`{
			"threadId":"thread-1",
			"turnId":"turn-1",
			"tokenUsage":{
				"last":{"totalTokens":112,"inputTokens":100,"cachedInputTokens":20,"outputTokens":12,"reasoningOutputTokens":4},
				"total":{"totalTokens":500,"inputTokens":440,"cachedInputTokens":120,"outputTokens":60,"reasoningOutputTokens":10},
				"modelContextWindow":258400
			}
		}`),
	})
	if err != nil {
		t.Fatalf("appServerNotificationEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events len=%d want 1", len(events))
	}
	ev := events[0]
	if ev.Type != domain.EventContextSize {
		t.Fatalf("event type=%q want %q", ev.Type, domain.EventContextSize)
	}
	if ev.TurnID != "turn-1" || ev.CurrentTokens != 100 || ev.BudgetTokens != 258400 {
		t.Fatalf("context event mismatch: %+v", ev)
	}
}

func TestAppServerNotificationEvents_MapsCompactionSignals(t *testing.T) {
	for _, tc := range []struct {
		name   string
		method string
		params string
	}{
		{
			name:   "thread compacted",
			method: "thread/compacted",
			params: `{"threadId":"thread-1","turnId":"turn-1"}`,
		},
		{
			name:   "context compaction item",
			method: "item/completed",
			params: `{"threadId":"thread-1","turnId":"turn-1","item":{"type":"contextCompaction","id":"compact-1"}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			events, err := appServerNotificationEvents(jsonrpcMessage{
				Method: tc.method,
				Params: []byte(tc.params),
			})
			if err != nil {
				t.Fatalf("appServerNotificationEvents: %v", err)
			}
			if len(events) != 1 {
				t.Fatalf("events len=%d want 1", len(events))
			}
			ev := events[0]
			if ev.Type != domain.EventCompaction || ev.TurnID != "turn-1" || !ev.ServerSide {
				t.Fatalf("compaction event mismatch: %+v", ev)
			}
		})
	}
}

func TestParseEvents_MapsCodexExecStream(t *testing.T) {
	rt := New()
	stream := []byte(strings.Join([]string{
		`{"type":"thread.started","thread_id":"thread-1"}`,
		`{"type":"turn.started"}`,
		`{"type":"item.started","turn_id":"turn-1","item":{"id":"call-1","type":"mcp_tool_call","server":"docs","tool":"search","status":"in_progress","arguments":{"action":"list","query":"agents"}}}`,
		`{"type":"item.completed","turn_id":"turn-1","item":{"id":"call-1","type":"mcp_tool_call","server":"docs","tool":"search","status":"completed","result":{"structured_content":{"ok":true}}}}`,
		`{"type":"item.completed","item":{"id":"reason-1","type":"reasoning","text":"thinking"}}`,
		`{"type":"item.completed","item":{"id":"item-1","type":"agent_message","text":"hello"}}`,
		`{"type":"turn.completed","usage":{"input_tokens":1,"cached_input_tokens":0,"output_tokens":1}}`,
	}, "\n"))
	events, err := rt.ParseEvents(stream)
	if err != nil {
		t.Fatalf("ParseEvents: %v", err)
	}
	if len(events) != 7 {
		t.Fatalf("events len=%d want 7", len(events))
	}
	if events[0].Type != domain.EventTurnStarted || events[0].SessionRef != "thread-1" {
		t.Fatalf("thread.started mismatch: %+v", events[0])
	}
	if events[1].Type != domain.EventTurnStarted {
		t.Fatalf("turn.started mismatch: %+v", events[1])
	}
	if events[2].Type != domain.EventToolCall || events[2].ToolName != "docs/search" {
		t.Fatalf("tool call mismatch: %+v", events[2])
	}
	if got := events[2].TurnID; got != "turn-1" {
		t.Fatalf("tool call turnId=%q want turn-1", got)
	}
	if got := events[2].Data["op"]; got != "search" {
		t.Fatalf("tool call op=%q want search", got)
	}
	if got := events[2].Data["turnId"]; got != "turn-1" {
		t.Fatalf("tool call data turnId=%q want turn-1", got)
	}
	if got := events[2].Data["action"]; got != "list" {
		t.Fatalf("tool call action=%q want list", got)
	}
	if !strings.Contains(events[2].Data["input"], `"query":"agents"`) {
		t.Fatalf("tool call input=%q missing query", events[2].Data["input"])
	}
	if events[3].Type != domain.EventToolResult || events[3].ToolCallID != "call-1" {
		t.Fatalf("tool result mismatch: %+v", events[3])
	}
	if got := events[3].TurnID; got != "turn-1" {
		t.Fatalf("tool result turnId=%q want turn-1", got)
	}
	if got := events[3].Data["turnId"]; got != "turn-1" {
		t.Fatalf("tool result data turnId=%q want turn-1", got)
	}
	if got := events[3].Data["result"]; got != `{"ok":true}` {
		t.Fatalf("tool result payload=%q want %q", got, `{"ok":true}`)
	}
	if events[4].Type != domain.EventText || events[4].Text != "thinking" || events[4].Data["kind"] != "reasoning" || events[4].Data["itemId"] != "reason-1" {
		t.Fatalf("reasoning message mismatch: %+v", events[4])
	}
	if events[5].Type != domain.EventText || events[5].Text != "hello" || events[5].Data["kind"] != "assistant" {
		t.Fatalf("agent message mismatch: %+v", events[5])
	}
	if events[6].Type != domain.EventTurnCompleted {
		t.Fatalf("turn.completed mismatch: %+v", events[6])
	}
	if events[6].Usage == nil {
		t.Fatalf("turn.completed usage missing")
	}
	if events[6].Usage.InputTokens != 1 || events[6].Usage.OutputTokens != 1 || events[6].Usage.TotalTokens != 2 {
		t.Fatalf("turn.completed usage=%+v", events[6].Usage)
	}
}

func TestParseEvents_UnderscoreThreadStartedCapturesSessionRef(t *testing.T) {
	rt := New()
	events, err := rt.ParseEvents([]byte(`{"type":"thread_started","session_id":"thread-legacy"}` + "\n"))
	if err != nil {
		t.Fatalf("ParseEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events=%d want 1: %#v", len(events), events)
	}
	if events[0].Type != domain.EventTurnStarted || events[0].SessionRef != "thread-legacy" {
		t.Fatalf("thread_started mismatch: %+v", events[0])
	}
}

func TestParseEvents_CarriesTurnContextToItemEvents(t *testing.T) {
	rt := New()
	stream := []byte(strings.Join([]string{
		`{"type":"turn.started","turn_id":"turn-context-1"}`,
		`{"type":"item.started","item":{"id":"cmd-1","type":"command_execution","command":"pwd"}}`,
		`{"type":"item.completed","item":{"id":"cmd-1","type":"command_execution","command":"pwd","exit_code":0,"aggregated_output":"/tmp/work"}}`,
		`{"type":"item.completed","item":{"id":"item-1","type":"agent_message","text":"done"}}`,
	}, "\n"))

	events, err := rt.ParseEvents(stream)
	if err != nil {
		t.Fatalf("ParseEvents: %v", err)
	}
	if len(events) != 4 {
		t.Fatalf("events len=%d want 4", len(events))
	}
	for i, ev := range events[1:] {
		if got := ev.TurnID; got != "turn-context-1" {
			t.Fatalf("event %d turnId=%q want turn-context-1: %+v", i+1, got, ev)
		}
		if got := ev.Data["turnId"]; got != "turn-context-1" {
			t.Fatalf("event %d data turnId=%q want turn-context-1: %+v", i+1, got, ev)
		}
	}
}

func TestParseEvents_EmitsCompletedAgentMessageOnce(t *testing.T) {
	rt := New()
	stream := []byte(strings.Join([]string{
		`{"type":"item.started","item":{"id":"msg-1","type":"agent_message","text":""}}`,
		`{"type":"item.updated","item":{"id":"msg-1","type":"agent_message","text":"Hi. What do you want to work on"}}`,
		`{"type":"item.completed","item":{"id":"msg-1","type":"agent_message","text":"Hi. What do you want to work on in this repo?"}}`,
	}, "\n"))
	events, err := rt.ParseEvents(stream)
	if err != nil {
		t.Fatalf("ParseEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events len=%d want 1: %+v", len(events), events)
	}
	if events[0].Type != domain.EventText {
		t.Fatalf("event type=%q want text", events[0].Type)
	}
	if got := events[0].Text; got != "Hi. What do you want to work on in this repo?" {
		t.Fatalf("text=%q", got)
	}
}

func TestParseEvents_MapsMCPStructuredContentCamelCase(t *testing.T) {
	rt := New()
	stream := []byte(strings.Join([]string{
		`{"type":"item.started","item":{"id":"call-2","type":"mcp_tool_call","server":"agen8","tool":"plan","status":"in_progress","arguments":{"action":"submit"}}}`,
		`{"type":"item.completed","item":{"id":"call-2","type":"mcp_tool_call","server":"agen8","tool":"plan","status":"completed","result":{"structuredContent":{"op":"plan","ok":true,"text":"submitted"}}}}`,
	}, "\n"))
	events, err := rt.ParseEvents(stream)
	if err != nil {
		t.Fatalf("ParseEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events len=%d want 2", len(events))
	}
	if events[1].Type != domain.EventToolResult {
		t.Fatalf("tool result mismatch: %+v", events[1])
	}
	if got := events[1].Data["result"]; got != `{"ok":true,"op":"plan","text":"submitted"}` {
		t.Fatalf("tool result payload=%q", got)
	}
}

func TestParseEvents_NormalizesMCPShellCommandToolToBashOp(t *testing.T) {
	rt := New()
	stream := []byte(`{"type":"item.started","item":{"id":"call-shell-1","type":"mcp_tool_call","server":"codex","tool":"shell_command","status":"in_progress","arguments":{"command":"pwd"}}}`)
	events, err := rt.ParseEvents(stream)
	if err != nil {
		t.Fatalf("ParseEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events len=%d want 1", len(events))
	}
	if events[0].Type != domain.EventToolCall {
		t.Fatalf("event type=%v want tool call", events[0].Type)
	}
	if got := events[0].Data["op"]; got != "bash" {
		t.Fatalf("tool call op=%q want bash", got)
	}
}

func TestParseEvents_FailsLoudOnMalformedMCPToolCall(t *testing.T) {
	rt := New()
	_, err := rt.ParseEvents([]byte(`{"type":"item.started","item":{"type":"mcp_tool_call","server":"docs","status":"in_progress"}}`))
	if err == nil {
		t.Fatalf("expected malformed mcp tool call error")
	}
	if !strings.Contains(err.Error(), "mcp_tool_call item.tool is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseEvents_MapsCommandExecutionToBashToolEvents(t *testing.T) {
	rt := New()
	stream := []byte(strings.Join([]string{
		`{"type":"item.started","turn_id":"turn-cmd-1","item":{"id":"cmd-1","type":"command_execution","command":"pwd"}}`,
		`{"type":"item.completed","turn_id":"turn-cmd-1","item":{"id":"cmd-1","type":"command_execution","command":"pwd","exit_code":0,"aggregated_output":"/tmp/work"}}`,
	}, "\n"))
	events, err := rt.ParseEvents(stream)
	if err != nil {
		t.Fatalf("ParseEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events len=%d want 2", len(events))
	}
	if events[0].Type != domain.EventToolCall || events[0].ToolName != "bash" {
		t.Fatalf("tool call mismatch: %+v", events[0])
	}
	if got := events[0].TurnID; got != "turn-cmd-1" {
		t.Fatalf("tool call turnId=%q want turn-cmd-1", got)
	}
	if got := events[0].Data["turnId"]; got != "turn-cmd-1" {
		t.Fatalf("tool call data turnId=%q want turn-cmd-1", got)
	}
	if got := events[0].Data["status"]; got != "in_progress" {
		t.Fatalf("tool call status=%q want in_progress", got)
	}
	if got := events[0].Data["op"]; got != "bash" {
		t.Fatalf("tool call op=%q want bash", got)
	}
	if !strings.Contains(events[0].Data["input"], `"command":"pwd"`) {
		t.Fatalf("tool call input=%q missing command", events[0].Data["input"])
	}
	if events[1].Type != domain.EventToolResult || events[1].ToolName != "bash" {
		t.Fatalf("tool result mismatch: %+v", events[1])
	}
	if got := events[1].TurnID; got != "turn-cmd-1" {
		t.Fatalf("tool result turnId=%q want turn-cmd-1", got)
	}
	if got := events[1].Data["turnId"]; got != "turn-cmd-1" {
		t.Fatalf("tool result data turnId=%q want turn-cmd-1", got)
	}
	if got := events[1].Data["status"]; got != "completed" {
		t.Fatalf("tool result status=%q want completed", got)
	}
	if got := events[1].Data["exitCode"]; got != "0" {
		t.Fatalf("tool result exitCode=%q want 0", got)
	}
	if got := events[1].Data["result"]; got != "/tmp/work" {
		t.Fatalf("tool result payload=%q want /tmp/work", got)
	}
}

func TestParseEvents_MapsCommandExecutionUpdatesAsPendingToolResults(t *testing.T) {
	rt := New()
	stream := []byte(strings.Join([]string{
		`{"type":"item.started","item":{"id":"cmd-2","type":"command_execution","command":"npm run dev &"}}`,
		`{"type":"item.updated","item":{"id":"cmd-2","type":"command_execution","command":"npm run dev &","output":"ready on http://localhost:3000"}}`,
		`{"type":"item.completed","item":{"id":"cmd-2","type":"command_execution","command":"npm run dev &","exit_code":0}}`,
	}, "\n"))
	events, err := rt.ParseEvents(stream)
	if err != nil {
		t.Fatalf("ParseEvents: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("events len=%d want 3", len(events))
	}
	if events[0].Type != domain.EventToolCall || events[0].ToolName != "bash" {
		t.Fatalf("started event mismatch: %+v", events[0])
	}
	if got := events[0].Data["background"]; got != "true" {
		t.Fatalf("started background=%q want true", got)
	}
	if events[1].Type != domain.EventToolResult || events[1].ToolName != "bash" {
		t.Fatalf("updated event mismatch: %+v", events[1])
	}
	if got := events[1].Data["status"]; got != "in_progress" {
		t.Fatalf("updated status=%q want in_progress", got)
	}
	if got := events[1].Data["result"]; got != "ready on http://localhost:3000" {
		t.Fatalf("updated result=%q want streamed output", got)
	}
	if events[2].Type != domain.EventToolResult || events[2].ToolName != "bash" {
		t.Fatalf("completed event mismatch: %+v", events[2])
	}
	if got := events[2].Data["status"]; got != "completed" {
		t.Fatalf("completed status=%q want completed", got)
	}
}

func TestParseEvents_FailsLoudOnMalformedCommandExecution(t *testing.T) {
	rt := New()
	_, err := rt.ParseEvents([]byte(`{"type":"item.started","item":{"type":"command_execution","id":"cmd-1"}}`))
	if err == nil {
		t.Fatalf("expected malformed command_execution error")
	}
	if !strings.Contains(err.Error(), "command_execution item.command is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseEvents_MapsFileChangeItemsToToolEvents(t *testing.T) {
	rt := New()
	stream := []byte(strings.Join([]string{
		`{"type":"item.started","item":{"id":"patch-1","type":"file_change","status":"in_progress","changes":[{"path":"notes.txt","kind":"add"}]}}`,
		`{"type":"item.completed","item":{"id":"patch-1","type":"file_change","status":"completed","changes":[{"path":"notes.txt","kind":"add","diff":"@@ -0,0 +1 @@\n+hello"}]}}`,
	}, "\n"))
	events, err := rt.ParseEvents(stream)
	if err != nil {
		t.Fatalf("ParseEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events len=%d want 2", len(events))
	}
	if events[0].Type != domain.EventToolCall || events[0].ToolName != "file_change" {
		t.Fatalf("started file_change mismatch: %+v", events[0])
	}
	if got := events[0].Data["op"]; got != "write_file" {
		t.Fatalf("started op=%q want write_file", got)
	}
	if got := events[0].Data["path"]; got != "notes.txt" {
		t.Fatalf("started path=%q want notes.txt", got)
	}
	if events[1].Type != domain.EventToolResult || events[1].ToolName != "file_change" {
		t.Fatalf("completed file_change mismatch: %+v", events[1])
	}
	if got := events[1].Data["patchPreview"]; !strings.Contains(got, "@@ -0,0 +1 @@") {
		t.Fatalf("completed patchPreview=%q missing unified diff", got)
	}
}

func TestParseEvents_MapsDeniedFileChangeAsFailedResult(t *testing.T) {
	rt := New()
	stream := []byte(`{"type":"item.completed","item":{"id":"patch-denied-1","type":"file_change","status":"denied","message":"Write was denied by permission mode","changes":[{"path":"temp-file.txt","kind":"add","diff":"@@\n+temporary"}]}}`)
	events, err := rt.ParseEvents(stream)
	if err != nil {
		t.Fatalf("ParseEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events=%d want 1", len(events))
	}
	ev := events[0]
	if ev.Type != domain.EventToolResult {
		t.Fatalf("type=%s want tool_result", ev.Type)
	}
	if ev.Data["status"] != "failed" {
		t.Fatalf("status=%q want failed", ev.Data["status"])
	}
	if !strings.Contains(ev.Data["error"], "denied") {
		t.Fatalf("error=%q want denial text", ev.Data["error"])
	}
}

func TestParseEvents_FailsLoudOnMalformedFileChange(t *testing.T) {
	rt := New()
	_, err := rt.ParseEvents([]byte(`{"type":"item.completed","item":{"id":"patch-1","type":"file_change","status":"completed","changes":{}}}`))
	if err == nil {
		t.Fatalf("expected malformed file_change error")
	}
	if !strings.Contains(err.Error(), "file_change item.changes must be an array") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseEvents_FailsLoudOnMalformedLine(t *testing.T) {
	rt := New()
	_, err := rt.ParseEvents([]byte("{bad-json"))
	if err == nil || !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("expected line-aware parse error, got %v", err)
	}
}

func TestParseEvents_MapsTurnFailedAndErrorEvents(t *testing.T) {
	rt := New()
	stream := []byte(strings.Join([]string{
		`{"type":"turn.failed","error":{"message":"approval required"}}`,
		`{"type":"error","message":"stream crashed"}`,
	}, "\n"))
	events, err := rt.ParseEvents(stream)
	if err != nil {
		t.Fatalf("ParseEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events len=%d want 2", len(events))
	}
	if events[0].Type != domain.EventTurnFailed || events[0].Error != "approval required" {
		t.Fatalf("turn.failed mapping mismatch: %+v", events[0])
	}
	if events[1].Type != domain.EventTurnFailed || events[1].Error != "stream crashed" {
		t.Fatalf("error mapping mismatch: %+v", events[1])
	}
}

func TestParseEvents_MapsReconnectProgressToRetryEvent(t *testing.T) {
	rt := New()
	stream := []byte(strings.Join([]string{
		`{"type":"turn.failed","error":{"message":"Reconnecting... 1/5 (stream disconnected before completion: Transport error: network error: error decoding response body)"}}`,
		`{"type":"error","message":"Reconnecting... 2/5 (temporary network failure)"}`,
	}, "\n"))
	events, err := rt.ParseEvents(stream)
	if err != nil {
		t.Fatalf("ParseEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events len=%d want 2", len(events))
	}
	for i, ev := range events {
		if ev.Type != domain.EventRetry {
			t.Fatalf("events[%d].Type=%q want retry", i, ev.Type)
		}
	}
	if events[0].Data["attempt"] != "1" || events[0].Data["max"] != "5" {
		t.Fatalf("first retry data=%v", events[0].Data)
	}
	if !strings.Contains(events[0].Data["reason"], "stream disconnected") {
		t.Fatalf("first retry reason=%q", events[0].Data["reason"])
	}
	if events[1].Data["attempt"] != "2" || events[1].Data["max"] != "5" {
		t.Fatalf("second retry data=%v", events[1].Data)
	}
}

func TestParseCodexRolloutLine_MapsNativeTurnEvents(t *testing.T) {
	state := codexRolloutSyncState{toolNames: map[string]string{}}
	lines := []string{
		`{"timestamp":"2026-05-31T16:21:59.543Z","type":"turn_context","payload":{"turn_id":"native-turn-1"}}`,
		`{"timestamp":"2026-05-31T16:21:59.544Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"please run date"}]}}`,
		`{"timestamp":"2026-05-31T16:22:08.652Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{\"cmd\":\"date\"}","call_id":"call-1"}}`,
		`{"timestamp":"2026-05-31T16:22:09.000Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call-1","output":"ok"}}`,
		`{"timestamp":"2026-05-31T16:22:10.000Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"done"}]}}`,
	}
	var events []domain.Event
	for i, line := range lines {
		got, err := parseCodexRolloutLine(line, i+1, "thread-1", &state)
		if err != nil {
			t.Fatalf("parseCodexRolloutLine line %d: %v", i+1, err)
		}
		events = append(events, got...)
	}
	if len(events) != 5 {
		t.Fatalf("events len=%d want 5: %+v", len(events), events)
	}
	if events[0].Type != domain.EventTurnStarted || events[0].TurnID != "native-turn-1" || events[0].SessionRef != "thread-1" {
		t.Fatalf("turn event mismatch: %+v", events[0])
	}
	if events[1].Type != domain.EventText || events[1].TurnID != "native-turn-1" || events[1].Text != "please run date" || events[1].Data["kind"] != "user" {
		t.Fatalf("user message mismatch: %+v", events[1])
	}
	if events[2].Type != domain.EventToolCall || events[2].ToolName != "exec_command" || events[2].ToolCallID != "call-1" {
		t.Fatalf("tool call mismatch: %+v", events[2])
	}
	if events[3].Type != domain.EventToolResult || events[3].ToolName != "exec_command" || events[3].Text != "ok" {
		t.Fatalf("tool result mismatch: %+v", events[3])
	}
	if events[4].Type != domain.EventText || events[4].TurnID != "native-turn-1" || events[4].Text != "done" || events[4].Data["kind"] != "assistant" {
		t.Fatalf("assistant message mismatch: %+v", events[4])
	}
}

func TestTailLocalCodexRollout_StartsAtEOFAndEmitsAppendedNativeEvents(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "rollout-*.jsonl")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer file.Close()
	if _, err := file.WriteString(`{"type":"event_msg","payload":{"type":"agent_message","message":"old"}}` + "\n"); err != nil {
		t.Fatalf("write initial rollout: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan domain.Event, 16)
	errCh := make(chan error, 1)
	go func() {
		errCh <- tailLocalCodexRollout(ctx, file.Name(), "thread-1", func(ev domain.Event) {
			events <- ev
		})
	}()
	select {
	case ev := <-events:
		if ev.Type != domain.EventTurnStarted || ev.SessionRef != "thread-1" {
			t.Fatalf("initial sync event mismatch: %+v", ev)
		}
	case err := <-errCh:
		t.Fatalf("tailLocalCodexRollout exited before append: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for initial sync event")
	}
	if _, err := file.WriteString(strings.Join([]string{
		`{"type":"turn_context","payload":{"turn_id":"native-turn-2"}}`,
		`{"type":"event_msg","payload":{"type":"user_message","message":"ignored duplicate"}}`,
		`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"native question"}]}}`,
		`{"type":"event_msg","payload":{"type":"agent_message","message":"new duplicate","phase":"commentary"}}`,
		`{"type":"response_item","payload":{"id":"reason-1","type":"reasoning","summary":["Planning","Checking"]}}`,
		`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"new"}]}}`,
	}, "\n") + "\n"); err != nil {
		t.Fatalf("append rollout: %v", err)
	}
	if err := file.Sync(); err != nil {
		t.Fatalf("sync rollout: %v", err)
	}

	var got []domain.Event
	deadline := time.After(3 * time.Second)
	for {
		select {
		case ev := <-events:
			got = append(got, ev)
			if ev.Text == "new" {
				cancel()
				if err := <-errCh; err != context.Canceled {
					t.Fatalf("tailLocalCodexRollout err=%v want context.Canceled", err)
				}
				if len(got) != 4 {
					t.Fatalf("events len=%d want 4: %+v", len(got), got)
				}
				if got[0].Type != domain.EventTurnStarted || got[0].TurnID != "native-turn-2" {
					t.Fatalf("turn event mismatch: %+v", got[0])
				}
				if got[1].Type != domain.EventText || got[1].TurnID != "native-turn-2" || got[1].Text != "native question" || got[1].Data["kind"] != "user" {
					t.Fatalf("user event mismatch: %+v", got[1])
				}
				if got[2].Type != domain.EventText || got[2].TurnID != "native-turn-2" || got[2].Text != "Planning\nChecking" || got[2].Data["kind"] != "reasoning" || got[2].Data["itemId"] != "reason-1" {
					t.Fatalf("reasoning event mismatch: %+v", got[2])
				}
				if got[3].Type != domain.EventText || got[3].TurnID != "native-turn-2" || got[3].Text != "new" || got[3].Data["kind"] != "assistant" {
					t.Fatalf("assistant event mismatch: %+v", got[3])
				}
				return
			}
		case err := <-errCh:
			t.Fatalf("tailLocalCodexRollout exited early: %v; events=%+v", err, got)
		case <-deadline:
			t.Fatalf("timed out waiting for rollout events; got %+v", got)
		}
	}
}

func TestWritePromptAndToolResult(t *testing.T) {
	rt := New()
	var out bytes.Buffer
	if err := rt.WritePrompt(&out, domain.PromptInput{TurnID: "t1", Text: "hello"}); err != nil {
		t.Fatalf("WritePrompt: %v", err)
	}
	if out.String() != "hello" {
		t.Fatalf("prompt payload mismatch: %q", out.String())
	}
	out.Reset()
	if err := rt.WriteToolResult(&out, domain.ToolResultInput{TurnID: "t1", ToolCallID: "call-1", Result: "ok"}); err == nil {
		t.Fatalf("expected unsupported WriteToolResult error")
	}
}

func TestWritePrompt_RejectsEmptyText(t *testing.T) {
	rt := New()
	var out bytes.Buffer
	if err := rt.WritePrompt(&out, domain.PromptInput{Text: "   "}); err == nil {
		t.Fatalf("expected empty text error")
	}
}
