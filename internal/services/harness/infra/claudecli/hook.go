package claudecli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

const (
	EnvAgen8MCPURL  = "AGEN8_MCP_URL"
	EnvAgen8Token   = "AGEN8_MCP_TOKEN"
	EnvAgen8SpaceID = "AGEN8_SPACE_ID"
)

// Claude Code hooks receive the native session_id on stdin. Agen8 uses that
// lifecycle surface to bind Claude's session to the shared MCP token/member
// before member-scoped MCP tools need to disambiguate active sessions.
type HookInput struct {
	SessionID      string `json:"session_id"`
	HookEventName  string `json:"hook_event_name"`
	CWD            string `json:"cwd"`
	TranscriptPath string `json:"transcript_path"`
	SpaceID        string `json:"space_id"`
}

type HookBinder interface {
	Bind(ctx context.Context, input HookInput) (BindResult, error)
}

type BindResult struct {
	MemberID         string
	SpaceID          string
	SessionID        string
	LogicalSessionID string
	NativeSessionRef string
}

type hookOutput struct {
	HookSpecificOutput *hookSpecificOutput `json:"hookSpecificOutput,omitempty"`
	SystemMessage      string              `json:"systemMessage,omitempty"`
}

type hookSpecificOutput struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext"`
}

func RunHook(ctx context.Context, binder HookBinder, r io.Reader, w io.Writer, errOut io.Writer) {
	out := hookOutput{}
	defer func() {
		_ = json.NewEncoder(w).Encode(out)
	}()

	raw, err := io.ReadAll(r)
	if err != nil {
		logHook(errOut, "read hook stdin: %v", err)
		return
	}
	var in HookInput
	if err := json.Unmarshal(bytes.TrimSpace(raw), &in); err != nil {
		logHook(errOut, "parse hook stdin: %v", err)
		return
	}
	if strings.TrimSpace(in.SessionID) == "" || binder == nil {
		return
	}
	result, err := binder.Bind(ctx, in)
	if err != nil {
		logHook(errOut, "bind session %s: %v", strings.TrimSpace(in.SessionID), err)
		return
	}
	if strings.TrimSpace(result.MemberID) == "" {
		return
	}
	if mcpBinder, ok := binder.(MCPHookBinder); ok {
		if rawURL, err := mcpBinder.resolveMCPURL(in); err == nil {
			if err := writeClaudeSessionBinding(in, result, rawURL); err != nil {
				logHook(errOut, "write session binding: %v", err)
			}
		}
	}

	contextText := fmt.Sprintf("Agen8 bound this Claude Code session to member %s in space %s.", strings.TrimSpace(result.MemberID), strings.TrimSpace(result.SpaceID))
	event := strings.TrimSpace(in.HookEventName)
	if event == "" {
		out.SystemMessage = contextText
		return
	}
	out.HookSpecificOutput = &hookSpecificOutput{
		HookEventName:     event,
		AdditionalContext: contextText,
	}
}

type claudeSessionBindingFile struct {
	MCPURL           string `json:"mcpUrl"`
	Token            string `json:"token"`
	MemberID         string `json:"memberId"`
	SpaceID          string `json:"spaceId"`
	SessionID        string `json:"sessionId"`
	LogicalSessionID string `json:"logicalSessionId,omitempty"`
	NativeSessionRef string `json:"nativeSessionRef"`
	UpdatedAt        string `json:"updatedAt"`
}

func writeClaudeSessionBinding(input HookInput, result BindResult, rawURL string) error {
	root := strings.TrimSpace(input.CWD)
	if root == "" {
		return nil
	}
	info, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("stat cwd: %w", err)
	}
	if !info.IsDir() {
		root = filepath.Dir(root)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse mcp url: %w", err)
	}
	nativeSessionRef := strings.TrimSpace(result.NativeSessionRef)
	if nativeSessionRef == "" {
		nativeSessionRef = strings.TrimSpace(input.SessionID)
	}
	logicalSessionID := strings.TrimSpace(result.LogicalSessionID)
	if logicalSessionID == "" {
		logicalSessionID = strings.TrimSpace(result.SessionID)
	}
	binding := claudeSessionBindingFile{
		MCPURL:           rawURL,
		Token:            strings.TrimSpace(parsed.Query().Get("token")),
		MemberID:         strings.TrimSpace(result.MemberID),
		SpaceID:          strings.TrimSpace(result.SpaceID),
		SessionID:        nativeSessionRef,
		LogicalSessionID: logicalSessionID,
		NativeSessionRef: nativeSessionRef,
		UpdatedAt:        time.Now().UTC().Format(time.RFC3339Nano),
	}
	data, err := json.MarshalIndent(binding, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal binding: %w", err)
	}
	data = append(data, '\n')
	path := filepath.Join(root, ".agen8", "claude-session.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create binding dir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write binding file: %w", err)
	}
	return nil
}

type MCPHookBinder struct {
	MCPURL     string
	HTTPClient *http.Client
}

func (b MCPHookBinder) Bind(ctx context.Context, input HookInput) (BindResult, error) {
	sessionID := strings.TrimSpace(input.SessionID)
	if sessionID == "" {
		return BindResult{}, nil
	}
	rawURL, err := b.resolveMCPURL(input)
	if err != nil {
		return BindResult{}, err
	}
	client := b.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	c := mcpHTTPClient{url: rawURL, client: client, nativeSessionRef: sessionID}
	protocolSession, err := c.initialize(ctx)
	if err != nil {
		return BindResult{}, err
	}
	if err := c.initialized(ctx, protocolSession); err != nil {
		return BindResult{}, err
	}
	return c.register(ctx, protocolSession, input)
}

func (b MCPHookBinder) resolveMCPURL(input HookInput) (string, error) {
	if raw := strings.TrimSpace(b.MCPURL); raw != "" {
		return normalizeMCPURL(raw)
	}
	if raw := strings.TrimSpace(os.Getenv(EnvAgen8MCPURL)); raw != "" {
		return normalizeMCPURL(raw)
	}
	if raw := strings.TrimSpace(os.Getenv(EnvAgen8Token)); raw != "" {
		return normalizeMCPURL("http://127.0.0.1:7777/mcp?token=" + url.QueryEscape(raw))
	}
	return discoverAgen8MCPURL(input)
}

func discoverAgen8MCPURL(input HookInput) (string, error) {
	start := strings.TrimSpace(input.CWD)
	if start == "" && strings.TrimSpace(input.TranscriptPath) != "" {
		start = filepath.Dir(strings.TrimSpace(input.TranscriptPath))
	}
	if start == "" {
		return "", fmt.Errorf("cwd is required to discover .mcp.json")
	}
	info, err := os.Stat(start)
	if err != nil {
		return "", fmt.Errorf("stat cwd: %w", err)
	}
	if !info.IsDir() {
		start = filepath.Dir(start)
	}
	for dir := start; ; dir = filepath.Dir(dir) {
		raw, err := os.ReadFile(filepath.Join(dir, ".mcp.json"))
		if err == nil {
			if found, ok := agen8URLFromMCPConfig(raw); ok {
				return normalizeMCPURL(found)
			}
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("read .mcp.json: %w", err)
		}
		next := filepath.Dir(dir)
		if next == dir {
			break
		}
	}
	return "", fmt.Errorf("agen8 MCP URL was not found in .mcp.json or environment")
}

func agen8URLFromMCPConfig(raw []byte) (string, bool) {
	var cfg struct {
		MCPServers map[string]struct {
			Type string `json:"type"`
			URL  string `json:"url"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return "", false
	}
	if server, ok := cfg.MCPServers["agen8"]; ok {
		if rawURL := strings.TrimSpace(server.URL); rawURL != "" {
			return rawURL, true
		}
	}
	for _, server := range cfg.MCPServers {
		if rawURL := strings.TrimSpace(server.URL); strings.Contains(rawURL, "/mcp") && strings.Contains(rawURL, "token=") {
			return rawURL, true
		}
	}
	return "", false
}

func normalizeMCPURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("parse mcp url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("mcp url must be absolute")
	}
	if strings.TrimSpace(parsed.Query().Get("token")) == "" {
		return "", fmt.Errorf("mcp url token is required")
	}
	return parsed.String(), nil
}

type mcpHTTPClient struct {
	url              string
	client           *http.Client
	nativeSessionRef string
	nextID           atomic.Int64
}

func (c *mcpHTTPClient) initialize(ctx context.Context) (string, error) {
	resp, protocolSession, err := c.request(ctx, "", map[string]any{
		"jsonrpc": "2.0",
		"id":      c.id(),
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "agen8-claude-hook",
				"version": "0.0.0",
			},
		},
	})
	if err != nil {
		return "", err
	}
	if err := responseError(resp); err != nil {
		return "", err
	}
	if strings.TrimSpace(protocolSession) == "" {
		return "", fmt.Errorf("initialize response missing Mcp-Session-Id")
	}
	return protocolSession, nil
}

func (c *mcpHTTPClient) initialized(ctx context.Context, protocolSession string) error {
	_, _, err := c.request(ctx, protocolSession, map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
		"params":  map[string]any{},
	})
	return err
}

func (c *mcpHTTPClient) register(ctx context.Context, protocolSession string, input HookInput) (BindResult, error) {
	logicalSessionID := stableClaudeLogicalSessionID(input, c.url)
	nativeSessionRef := strings.TrimSpace(input.SessionID)
	args := map[string]any{
		"action":             "register",
		"project_root":       strings.TrimSpace(input.CWD),
		"harness_kind":       "claude-cli",
		"session_id":         logicalSessionID,
		"native_session_ref": nativeSessionRef,
	}
	if spaceID := resolveHookSpaceID(input); spaceID != "" {
		args["space_id"] = spaceID
	}
	resp, _, err := c.request(ctx, protocolSession, map[string]any{
		"jsonrpc": "2.0",
		"id":      c.id(),
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "space",
			"arguments": args,
		},
	})
	if err != nil {
		return BindResult{}, err
	}
	if err := responseError(resp); err != nil {
		return BindResult{}, err
	}
	var out struct {
		Result struct {
			StructuredContent struct {
				MemberID         string `json:"memberId"`
				SpaceID          string `json:"spaceId"`
				SessionID        string `json:"sessionId"`
				NativeSessionRef string `json:"nativeSessionRef"`
			} `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.Unmarshal(resp, &out); err != nil {
		return BindResult{}, fmt.Errorf("parse register response: %w", err)
	}
	resultNativeSessionRef := strings.TrimSpace(out.Result.StructuredContent.NativeSessionRef)
	if resultNativeSessionRef == "" {
		resultNativeSessionRef = nativeSessionRef
	}
	return BindResult{
		MemberID:         strings.TrimSpace(out.Result.StructuredContent.MemberID),
		SpaceID:          strings.TrimSpace(out.Result.StructuredContent.SpaceID),
		SessionID:        strings.TrimSpace(out.Result.StructuredContent.SessionID),
		LogicalSessionID: logicalSessionID,
		NativeSessionRef: resultNativeSessionRef,
	}, nil
}

func resolveHookSpaceID(input HookInput) string {
	if spaceID := strings.TrimSpace(input.SpaceID); spaceID != "" {
		return spaceID
	}
	if spaceID := strings.TrimSpace(os.Getenv(EnvAgen8SpaceID)); spaceID != "" {
		return spaceID
	}
	if context, err := readClaudeLaunchContext(strings.TrimSpace(input.CWD)); err == nil {
		return strings.TrimSpace(context.SpaceID)
	}
	return ""
}

func stableClaudeLogicalSessionID(input HookInput, rawURL string) string {
	if existing, err := readClaudeSessionBinding(strings.TrimSpace(input.CWD)); err == nil {
		nativeSessionRef := strings.TrimSpace(input.SessionID)
		existingNativeRef := firstNonEmptyString(existing.NativeSessionRef, existing.SessionID)
		if logical := strings.TrimSpace(existing.LogicalSessionID); logical != "" && nativeSessionRef != "" && existingNativeRef == nativeSessionRef {
			return logical
		}
	}
	parts := []string{"claude-cli"}
	if nativeSessionRef := strings.TrimSpace(input.SessionID); nativeSessionRef != "" {
		parts = append(parts, "native-session", nativeSessionRef)
	}
	if transcript := strings.TrimSpace(input.TranscriptPath); transcript != "" {
		if abs, err := filepath.Abs(transcript); err == nil {
			transcript = abs
		}
		parts = append(parts, "transcript", transcript)
	}
	root := strings.TrimSpace(input.CWD)
	if root != "" {
		if abs, err := filepath.Abs(root); err == nil {
			root = abs
		}
	}
	token := ""
	if parsed, err := url.Parse(strings.TrimSpace(rawURL)); err == nil {
		token = strings.TrimSpace(parsed.Query().Get("token"))
	}
	parts = append(parts, "project", root, "token", token)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "claude-logical-" + hex.EncodeToString(sum[:])[:24]
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func (c *mcpHTTPClient) request(ctx context.Context, protocolSession string, payload map[string]any) ([]byte, string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(data))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if protocolSession = strings.TrimSpace(protocolSession); protocolSession != "" {
		req.Header.Set("Mcp-Session-Id", protocolSession)
	}
	if nativeSessionRef := strings.TrimSpace(c.nativeSessionRef); nativeSessionRef != "" {
		req.Header.Set("Agen8-Native-Session-Id", nativeSessionRef)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, "", readErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("mcp http status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, strings.TrimSpace(resp.Header.Get("Mcp-Session-Id")), nil
}

func (c *mcpHTTPClient) id() string {
	return fmt.Sprintf("agen8-claude-hook-%d", c.nextID.Add(1))
}

func responseError(raw []byte) error {
	var envelope struct {
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("parse mcp response: %w", err)
	}
	if envelope.Error == nil {
		return nil
	}
	return fmt.Errorf("mcp error %d: %s", envelope.Error.Code, envelope.Error.Message)
}

func logHook(w io.Writer, format string, args ...any) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, "agen8 claude hook: "+format+"\n", args...)
}
