package claudecli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultChannelName         = "agen8-channel"
	defaultChannelVersion      = "0.0.0"
	defaultChannelListenAddr   = "127.0.0.1:0"
	defaultChannelInstructions = "Agen8 coordination events arrive as <channel> messages. Treat task, decision, escalation, and operator-action messages as durable work-context updates for this Claude Code session. Send Agen8 messages from channel context with the message tool on this channel server, passing action=send plus destination_member_id, correlation_id, subject, kind, and body."
)

type ChannelOptions struct {
	Name         string
	Version      string
	Instructions string
	ListenAddr   string
	In           io.Reader
	Out          io.Writer
	ErrOut       io.Writer
	Ready        chan<- ChannelReady
	ProjectRoot  string
}

type ChannelReady struct {
	NotifyURL string
}

type channelRouteInstance struct {
	ID        string
	StartedAt time.Time
	ProcessID int
}

type channelNotificationInput struct {
	Content string         `json:"content"`
	Meta    map[string]any `json:"meta,omitempty"`
}

type channelRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type channelRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type channelCallToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type channelWriter struct {
	mu  sync.Mutex
	enc *json.Encoder
}

func RunChannel(ctx context.Context, opts ChannelOptions) error {
	in := opts.In
	if in == nil {
		in = bytes.NewReader(nil)
	}
	out := opts.Out
	if out == nil {
		return fmt.Errorf("channel stdout writer is required")
	}
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = defaultChannelName
	}
	version := strings.TrimSpace(opts.Version)
	if version == "" {
		version = defaultChannelVersion
	}
	instructions := strings.TrimSpace(opts.Instructions)
	if instructions == "" {
		instructions = defaultChannelInstructions
	}
	listenAddr := strings.TrimSpace(opts.ListenAddr)
	if listenAddr == "" {
		listenAddr = defaultChannelListenAddr
	}

	writer := &channelWriter{enc: json.NewEncoder(out)}
	routeInstance := newChannelRouteInstance()
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("start claude channel notify listener: %w", err)
	}
	defer listener.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/notify", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var input channelNotificationInput
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil {
			http.Error(w, "invalid notification payload", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(input.Content) == "" {
			http.Error(w, "content is required", http.StatusBadRequest)
			return
		}
		if err := writer.notification("notifications/claude/channel", map[string]any{
			"content": input.Content,
			"meta":    normalizeChannelMeta(input.Meta),
		}); err != nil {
			http.Error(w, "write channel notification", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	httpServer := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	serverDone := make(chan error, 1)
	go func() {
		err := httpServer.Serve(listener)
		if err == http.ErrServerClosed {
			err = nil
		}
		serverDone <- err
	}()
	notifyURL := "http://" + listener.Addr().String() + "/notify"
	if opts.Ready != nil {
		select {
		case opts.Ready <- ChannelReady{NotifyURL: notifyURL}:
		case <-ctx.Done():
			_ = httpServer.Close()
			<-serverDone
			return ctx.Err()
		}
	}
	logChannel(opts.ErrOut, "listening for local notifications at %s", notifyURL)
	go registerClaudeChannelRoute(ctx, opts, notifyURL, routeInstance)

	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var req channelRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			_ = writer.error(nil, -32700, "parse error")
			continue
		}
		if len(req.ID) == 0 {
			continue
		}
		switch strings.TrimSpace(req.Method) {
		case "initialize":
			_ = writer.result(req.ID, channelInitializeResult(name, version, instructions, req.Params))
		case "tools/list":
			_ = writer.result(req.ID, map[string]any{"tools": []any{channelMessageToolDefinition()}})
		case "tools/call":
			result, err := handleChannelToolCall(ctx, opts.ProjectRoot, req.Params)
			if err != nil {
				_ = writer.error(req.ID, -32000, err.Error())
				continue
			}
			_ = writer.result(req.ID, result)
		default:
			_ = writer.error(req.ID, -32601, "method not found")
		}
	}
	if err := scanner.Err(); err != nil {
		_ = httpServer.Close()
		<-serverDone
		return fmt.Errorf("read claude channel stdio: %w", err)
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
	if err := <-serverDone; err != nil {
		return err
	}
	return nil
}

func handleChannelToolCall(ctx context.Context, projectRoot string, rawParams json.RawMessage) (map[string]any, error) {
	var params channelCallToolParams
	if len(rawParams) == 0 {
		return nil, fmt.Errorf("tool call params are required")
	}
	if err := json.Unmarshal(rawParams, &params); err != nil {
		return nil, fmt.Errorf("parse tool call params: %w", err)
	}
	if strings.TrimSpace(params.Name) != "message" {
		return nil, fmt.Errorf("unknown channel tool %q", params.Name)
	}
	binding, err := readClaudeSessionBinding(projectRoot)
	if err != nil {
		return nil, err
	}
	result, err := sendClaudeChannelMessage(ctx, binding, params.Arguments)
	if err != nil {
		return nil, err
	}
	text := "sent"
	if rawText, ok := result["text"].(string); ok && strings.TrimSpace(rawText) != "" {
		text = strings.TrimSpace(rawText)
	}
	return map[string]any{
		"content": []map[string]string{{
			"type": "text",
			"text": text,
		}},
		"structuredContent": result,
	}, nil
}

func registerClaudeChannelRoute(ctx context.Context, opts ChannelOptions, notifyURL string, routeInstance channelRouteInstance) {
	errOut := opts.ErrOut
	attempt := 0
	for {
		binding, err := readClaudeSessionBinding(opts.ProjectRoot)
		if err != nil {
			attempt++
			if attempt <= 30 || attempt%30 == 0 {
				logChannel(errOut, "route registration attempt %d failed: %v", attempt, err)
			}
		} else {
			if err := registerClaudeChannelRouteWithBinding(ctx, binding, notifyURL, routeInstance); err != nil {
				attempt++
				logChannel(errOut, "route registration attempt %d failed: %v", attempt, err)
			} else {
				logChannel(errOut, "registered route with Agen8 daemon for member %s using channel instance %s", strings.TrimSpace(binding.MemberID), routeInstance.ID)
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

func registerClaudeChannelRouteOnce(ctx context.Context, projectRoot string, notifyURL string) error {
	binding, err := readClaudeSessionBinding(projectRoot)
	if err != nil {
		return err
	}
	return registerClaudeChannelRouteWithBinding(ctx, binding, notifyURL, newChannelRouteInstance())
}

func registerClaudeChannelRouteWithBinding(ctx context.Context, binding claudeSessionBindingFile, notifyURL string, routeInstance channelRouteInstance) error {
	if strings.TrimSpace(binding.MCPURL) == "" || strings.TrimSpace(binding.Token) == "" || strings.TrimSpace(binding.SessionID) == "" {
		return fmt.Errorf("claude session binding is incomplete")
	}
	if strings.TrimSpace(routeInstance.ID) == "" || routeInstance.StartedAt.IsZero() {
		return fmt.Errorf("claude channel route instance is incomplete")
	}
	parsed, err := url.Parse(strings.TrimSpace(binding.MCPURL))
	if err != nil {
		return fmt.Errorf("parse mcp url: %w", err)
	}
	registerURL := url.URL{
		Scheme: parsed.Scheme,
		Host:   parsed.Host,
		Path:   "/harness/claude-channel/register",
	}
	payload := map[string]any{
		"token":             binding.Token,
		"sessionId":         binding.SessionID,
		"memberId":          binding.MemberID,
		"notifyUrl":         notifyURL,
		"channelInstanceId": routeInstance.ID,
		"channelStartedAt":  routeInstance.StartedAt.UTC().Format(time.RFC3339Nano),
		"processId":         routeInstance.ProcessID,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal route registration: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, registerURL.String(), bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("build route registration request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("register route: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("register route status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func newChannelRouteInstance() channelRouteInstance {
	startedAt := time.Now().UTC()
	pid := os.Getpid()
	return channelRouteInstance{
		ID:        fmt.Sprintf("pid-%d:%d", pid, startedAt.UnixNano()),
		StartedAt: startedAt,
		ProcessID: pid,
	}
}

func sendClaudeChannelMessage(ctx context.Context, binding claudeSessionBindingFile, arguments json.RawMessage) (map[string]any, error) {
	if strings.TrimSpace(binding.MCPURL) == "" || strings.TrimSpace(binding.Token) == "" || strings.TrimSpace(binding.SessionID) == "" || strings.TrimSpace(binding.MemberID) == "" {
		return nil, fmt.Errorf("claude session binding is incomplete")
	}
	if len(arguments) == 0 {
		return nil, fmt.Errorf("message arguments are required")
	}
	parsed, err := url.Parse(strings.TrimSpace(binding.MCPURL))
	if err != nil {
		return nil, fmt.Errorf("parse mcp url: %w", err)
	}
	messageURL := url.URL{
		Scheme: parsed.Scheme,
		Host:   parsed.Host,
		Path:   "/harness/claude-channel/message",
	}
	payload := map[string]any{
		"token":     binding.Token,
		"sessionId": binding.SessionID,
		"memberId":  binding.MemberID,
		"arguments": json.RawMessage(arguments),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal message request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, messageURL.String(), bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("build message request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("send message: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("message status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse message response: %w", err)
	}
	return result, nil
}

func readClaudeSessionBinding(projectRoot string) (claudeSessionBindingFile, error) {
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return claudeSessionBindingFile{}, err
		}
		root = cwd
	}
	info, err := os.Stat(root)
	if err != nil {
		return claudeSessionBindingFile{}, fmt.Errorf("stat project root: %w", err)
	}
	if !info.IsDir() {
		root = filepath.Dir(root)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".agen8", "claude-session.json"))
	if err != nil {
		return claudeSessionBindingFile{}, fmt.Errorf("read claude session binding: %w", err)
	}
	var binding claudeSessionBindingFile
	if err := json.Unmarshal(raw, &binding); err != nil {
		return claudeSessionBindingFile{}, fmt.Errorf("parse claude session binding: %w", err)
	}
	return binding, nil
}

func channelInitializeResult(name string, version string, instructions string, rawParams json.RawMessage) map[string]any {
	protocolVersion := "2025-06-18"
	var params struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if len(rawParams) > 0 && json.Unmarshal(rawParams, &params) == nil && strings.TrimSpace(params.ProtocolVersion) != "" {
		protocolVersion = strings.TrimSpace(params.ProtocolVersion)
	}
	return map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities": map[string]any{
			"experimental": map[string]any{
				"claude/channel": map[string]any{},
			},
			"tools": map[string]any{},
		},
		"serverInfo": map[string]any{
			"name":    name,
			"version": version,
		},
		"instructions": instructions,
	}
}

func channelMessageToolDefinition() map[string]any {
	return map[string]any{
		"name":        "message",
		"description": "Send a durable Agen8 member message from this Claude Code channel session.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"enum":        []string{"send"},
					"description": "Message action. Must be send.",
				},
				"destination_member_id": map[string]any{
					"type":        "string",
					"description": "Destination Agen8 member id.",
				},
				"kind": map[string]any{
					"type":        "string",
					"enum":        []string{"inform", "query", "ack", "response"},
					"description": "Message kind.",
				},
				"subject": map[string]any{
					"type":        "string",
					"description": "Short message subject.",
				},
				"body": map[string]any{
					"type":        "string",
					"description": "Message body.",
				},
				"correlation_id": map[string]any{
					"type":        "string",
					"description": "Correlation id from the inbound Agen8 channel message when acknowledging or responding. Required for ack and response.",
				},
			},
			"required": []string{"action", "destination_member_id", "kind", "subject", "body"},
		},
	}
}

func normalizeChannelMeta(meta map[string]any) map[string]any {
	if meta == nil {
		return map[string]any{"source": "agen8"}
	}
	if _, ok := meta["source"]; !ok {
		meta["source"] = "agen8"
	}
	return meta
}

func (w *channelWriter) result(id json.RawMessage, result any) error {
	return w.write(map[string]any{
		"jsonrpc": "2.0",
		"id":      json.RawMessage(id),
		"result":  result,
	})
}

func (w *channelWriter) error(id json.RawMessage, code int, message string) error {
	out := map[string]any{
		"jsonrpc": "2.0",
		"error":   channelRPCError{Code: code, Message: message},
	}
	if len(id) > 0 {
		out["id"] = json.RawMessage(id)
	}
	return w.write(out)
}

func (w *channelWriter) notification(method string, params any) error {
	return w.write(map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	})
}

func (w *channelWriter) write(value any) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.enc.Encode(value)
}

func logChannel(w io.Writer, format string, args ...any) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, "agen8 claude channel: "+format+"\n", args...)
}
