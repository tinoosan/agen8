package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/tinoosan/agen8-mcp-server/internal/services/harness/domain"
)

type jsonrpcMessage struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

const appServerReadLimit = 8 << 20

type appServerSession struct {
	mu              sync.Mutex
	threadID        string
	configSignature string
	proc            domain.CommandProcess
	client          *appServerClient
}

func (r *Runtime) SupportsSessionSteering() bool {
	return true
}

func SteerAppServerTurn(ctx context.Context, params domain.StartParams, turnID string, text string, attachments []domain.PromptAttachment) error {
	threadID := strings.TrimSpace(params.SessionRef)
	turnID = strings.TrimSpace(turnID)
	text = strings.TrimSpace(text)
	if strings.TrimSpace(params.AppServerURL) == "" {
		return fmt.Errorf("codex app-server url is required")
	}
	if threadID == "" {
		return fmt.Errorf("codex thread id is required")
	}
	if turnID == "" {
		return fmt.Errorf("codex turn id is required")
	}
	if text == "" && len(attachments) == 0 {
		return fmt.Errorf("codex steer text or attachment is required")
	}
	dialURL, dialOptions, err := appServerDialTarget(params, strings.TrimSpace(params.AppServerURL))
	if err != nil {
		return err
	}
	conn, resp, err := websocket.Dial(ctx, dialURL, dialOptions)
	if err != nil {
		return fmt.Errorf("codex app-server dial: %w%s", err, websocketDialResponseText(resp))
	}
	defer conn.CloseNow()
	conn.SetReadLimit(appServerReadLimit)
	client := newAppServerClient(conn, params.ApprovalHandler)
	if _, err := client.call(ctx, "initialize", map[string]any{
		"clientInfo":   map[string]string{"name": "agen8", "version": "dev"},
		"capabilities": map[string]any{"experimentalApi": true},
	}); err != nil {
		return err
	}
	if err := client.notification(ctx, "initialized", map[string]any{}); err != nil {
		return err
	}
	result, err := client.call(ctx, "turn/steer", map[string]any{
		"threadId":       threadID,
		"expectedTurnId": turnID,
		"input":          codexInputItems(text, attachments),
	})
	if err != nil {
		return err
	}
	steeredTurnID := strings.TrimSpace(nestedJSONRPCString(result, "turnId"))
	if steeredTurnID == "" {
		return fmt.Errorf("codex app-server turn/steer returned no turn id")
	}
	if steeredTurnID != turnID {
		return fmt.Errorf("codex app-server turn/steer returned turn id %q, expected %q", steeredTurnID, turnID)
	}
	return nil
}

func InjectAppServerThreadItems(ctx context.Context, params domain.StartParams, text string, attachments []domain.PromptAttachment) error {
	threadID := strings.TrimSpace(params.SessionRef)
	text = strings.TrimSpace(text)
	if strings.TrimSpace(params.AppServerURL) == "" {
		return fmt.Errorf("codex app-server url is required")
	}
	if threadID == "" {
		return fmt.Errorf("codex thread id is required")
	}
	if text == "" && len(attachments) == 0 {
		return fmt.Errorf("codex inject text or attachment is required")
	}
	dialURL, dialOptions, err := appServerDialTarget(params, strings.TrimSpace(params.AppServerURL))
	if err != nil {
		return err
	}
	conn, resp, err := websocket.Dial(ctx, dialURL, dialOptions)
	if err != nil {
		return fmt.Errorf("codex app-server dial: %w%s", err, websocketDialResponseText(resp))
	}
	defer conn.CloseNow()
	conn.SetReadLimit(appServerReadLimit)
	client := newAppServerClient(conn, params.ApprovalHandler)
	if _, err := client.call(ctx, "initialize", map[string]any{
		"clientInfo":   map[string]string{"name": "agen8", "version": "dev"},
		"capabilities": map[string]any{"experimentalApi": true},
	}); err != nil {
		return err
	}
	if err := client.notification(ctx, "initialized", map[string]any{}); err != nil {
		return err
	}
	if err := ensureAppServerThreadLoaded(ctx, client, threadID); err != nil {
		return err
	}
	_, err = client.call(ctx, "thread/inject_items", map[string]any{
		"threadId": threadID,
		"items":    codexInjectedUserItems(text, attachments),
	})
	return err
}

func ensureAppServerThreadLoaded(ctx context.Context, client *appServerClient, threadID string) error {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return fmt.Errorf("codex thread id is required")
	}
	result, err := client.call(ctx, "thread/loaded/list", map[string]any{})
	if err != nil {
		return fmt.Errorf("codex app-server thread/loaded/list: %w", err)
	}
	loaded, err := appServerLoadedThreadRefs(result)
	if err != nil {
		return fmt.Errorf("codex app-server thread/loaded/list returned invalid response: %w", err)
	}
	for _, item := range loaded {
		if item == threadID {
			return nil
		}
	}
	return fmt.Errorf("codex app-server thread %q is not loaded by the reachable remote-control server", threadID)
}

func AppServerHasLoadedThread(ctx context.Context, appServerURL string, threadID string) (bool, error) {
	appServerURL = strings.TrimSpace(appServerURL)
	threadID = strings.TrimSpace(threadID)
	if appServerURL == "" {
		return false, fmt.Errorf("codex app-server url is required")
	}
	if threadID == "" {
		return false, fmt.Errorf("codex thread id is required")
	}
	dialURL, dialOptions, err := appServerDialTarget(domain.StartParams{}, appServerURL)
	if err != nil {
		return false, err
	}
	conn, resp, err := websocket.Dial(ctx, dialURL, dialOptions)
	if err != nil {
		return false, fmt.Errorf("codex app-server dial: %w%s", err, websocketDialResponseText(resp))
	}
	defer conn.CloseNow()
	conn.SetReadLimit(appServerReadLimit)
	client := newAppServerClient(conn, nil)
	if _, err := client.call(ctx, "initialize", map[string]any{
		"clientInfo":   map[string]string{"name": "agen8", "version": "dev"},
		"capabilities": map[string]any{"experimentalApi": true},
	}); err != nil {
		return false, err
	}
	if err := client.notification(ctx, "initialized", map[string]any{}); err != nil {
		return false, err
	}
	result, err := client.call(ctx, "thread/loaded/list", map[string]any{})
	if err != nil {
		return false, fmt.Errorf("codex app-server thread/loaded/list: %w", err)
	}
	loaded, err := appServerLoadedThreadRefs(result)
	if err != nil {
		return false, fmt.Errorf("codex app-server thread/loaded/list returned invalid response: %w", err)
	}
	for _, item := range loaded {
		if item == threadID {
			return true, nil
		}
	}
	return false, nil
}

func appServerLoadedThreadRefs(result json.RawMessage) ([]string, error) {
	var parsed struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(result, &parsed); err != nil {
		return nil, err
	}
	refs := make([]string, 0, len(parsed.Data)*2)
	for _, raw := range parsed.Data {
		var id string
		if err := json.Unmarshal(raw, &id); err == nil {
			if id = strings.TrimSpace(id); id != "" {
				refs = append(refs, id)
			}
			continue
		}
		var item struct {
			ID        string `json:"id"`
			SessionID string `json:"sessionId"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, err
		}
		if id := strings.TrimSpace(item.ID); id != "" {
			refs = append(refs, id)
		}
		if sessionID := strings.TrimSpace(item.SessionID); sessionID != "" {
			refs = append(refs, sessionID)
		}
	}
	return refs, nil
}

func (r *Runtime) ExecuteSessionTurn(ctx context.Context, params domain.StartParams, input domain.SessionTurnInput, emit func(domain.Event)) (domain.SessionTurnResult, error) {
	text := strings.TrimSpace(input.Text)
	if text == "" && len(input.Attachments) == 0 {
		return domain.SessionTurnResult{}, fmt.Errorf("codex app-server turn text or attachment is required")
	}
	session, err := r.appServerSession(ctx, params, emit)
	if err != nil {
		return domain.SessionTurnResult{}, err
	}
	session.mu.Lock()
	lockedSession := session
	defer func() {
		lockedSession.mu.Unlock()
	}()

	threadID := session.threadID
	client := session.client
	if threadID == "" || client == nil {
		return domain.SessionTurnResult{}, fmt.Errorf("codex app-server session is not initialized")
	}
	result, err := client.call(ctx, "turn/start", turnStartParams(params, threadID, text, input.Attachments))
	if err != nil {
		if !isClosedAppServerConnectionError(err) {
			r.dropAppServerSession(threadID, session)
			return domain.SessionTurnResult{}, fmt.Errorf("%w%s", err, runtimeHostDiagnosticSuffix(params))
		}
		restarted, restartErr := r.restartAppServerSession(ctx, params, threadID, session, emit)
		if restartErr != nil {
			return domain.SessionTurnResult{}, fmt.Errorf("codex app-server reconnect after closed connection: %w; original error: %v%s", restartErr, err, runtimeHostDiagnosticSuffix(params))
		}
		session = restarted
		session.mu.Lock()
		defer session.mu.Unlock()
		threadID = session.threadID
		client = session.client
		result, err = client.call(ctx, "turn/start", turnStartParams(params, threadID, text, input.Attachments))
		if err != nil {
			r.dropAppServerSession(threadID, session)
			return domain.SessionTurnResult{}, fmt.Errorf("%w%s", err, runtimeHostDiagnosticSuffix(params))
		}
	}
	activeTurnID := nestedJSONRPCString(result, "turn", "id")
	if activeTurnID == "" {
		r.dropAppServerSession(threadID, session)
		return domain.SessionTurnResult{}, fmt.Errorf("codex app-server turn/start returned no turn id")
	}
	emittedProgress := false
	processEvents := func(events []domain.Event) (bool, error) {
		for _, ev := range events {
			if ev.TurnID == "" && activeTurnID != "" && appServerEventCanUseActiveTurn(ev.Type) {
				ev.TurnID = activeTurnID
				if ev.Data == nil {
					ev.Data = map[string]string{}
				}
				ev.Data["turnId"] = activeTurnID
			}
			if ev.TurnID != "" {
				activeTurnID = ev.TurnID
			}
			if ev.Type == domain.EventText || ev.Type == domain.EventToolCall || ev.Type == domain.EventToolResult {
				emittedProgress = true
			}
			if emit != nil && ev.Type != "" {
				emit(ev)
			}
			if ev.Type == domain.EventTurnCompleted {
				if activeTurnID != "" && ev.TurnID != "" && ev.TurnID != activeTurnID {
					continue
				}
				return true, nil
			}
			if ev.Type == domain.EventTurnFailed {
				if activeTurnID != "" && ev.TurnID != "" && ev.TurnID != activeTurnID {
					continue
				}
				return false, fmt.Errorf("codex app-server turn failed: %s%s", strings.TrimSpace(ev.Error), runtimeHostDiagnosticSuffix(params))
			}
		}
		return false, nil
	}
	for {
		select {
		case <-ctx.Done():
			if activeTurnID != "" {
				_ = client.notify(context.WithoutCancel(context.Background()), "turn/interrupt", map[string]any{
					"threadId": threadID,
					"turnId":   activeTurnID,
				})
			}
			return domain.SessionTurnResult{}, ctx.Err()
		case steer, ok := <-input.Steering:
			if !ok {
				input.Steering = nil
				continue
			}
			steerText := strings.TrimSpace(steer.Text)
			if (steerText == "" && len(steer.Attachments) == 0) || activeTurnID == "" {
				continue
			}
			if err := client.notify(ctx, "turn/steer", map[string]any{
				"threadId":       threadID,
				"expectedTurnId": activeTurnID,
				"input":          codexInputItems(steerText, steer.Attachments),
			}); err != nil {
				r.dropAppServerSession(threadID, session)
				return domain.SessionTurnResult{}, err
			}
		case err, ok := <-client.done:
			for {
				select {
				case msg, ok := <-client.notifications:
					if !ok {
						goto drainedNotifications
					}
					if msg.Error != nil {
						return domain.SessionTurnResult{}, fmt.Errorf("codex app-server: %s", strings.TrimSpace(msg.Error.Message))
					}
					if msg.Method == "" {
						continue
					}
					events, parseErr := appServerNotificationEvents(msg)
					if parseErr != nil {
						return domain.SessionTurnResult{}, parseErr
					}
					completed, eventErr := processEvents(events)
					if eventErr != nil {
						return domain.SessionTurnResult{}, eventErr
					}
					if completed {
						return domain.SessionTurnResult{}, nil
					}
				default:
					goto drainedNotifications
				}
			}
		drainedNotifications:
			r.dropAppServerSession(threadID, session)
			if ctx.Err() != nil {
				return domain.SessionTurnResult{}, ctx.Err()
			}
			if isSuccessfulCodexEOF(input.TurnID, emittedProgress, err) {
				return domain.SessionTurnResult{}, nil
			}
			if !ok {
				return domain.SessionTurnResult{}, fmt.Errorf("codex app-server connection closed")
			}
			return domain.SessionTurnResult{}, err
		case msg, ok := <-client.notifications:
			if !ok {
				r.dropAppServerSession(threadID, session)
				if isSuccessfulCodexEOF(input.TurnID, emittedProgress, nil) {
					return domain.SessionTurnResult{}, nil
				}
				return domain.SessionTurnResult{}, fmt.Errorf("codex app-server notification stream closed")
			}
			if msg.Error != nil {
				return domain.SessionTurnResult{}, fmt.Errorf("codex app-server: %s", strings.TrimSpace(msg.Error.Message))
			}
			if msg.Method == "" {
				continue
			}
			events, err := appServerNotificationEvents(msg)
			if err != nil {
				return domain.SessionTurnResult{}, err
			}
			completed, eventErr := processEvents(events)
			if eventErr != nil {
				return domain.SessionTurnResult{}, eventErr
			}
			if completed {
				return domain.SessionTurnResult{}, nil
			}
		}
	}
}

func openDedicatedAppServerClient(ctx context.Context, params domain.StartParams, threadID string) (*appServerClient, domain.CommandProcess, json.RawMessage, error) {
	port, err := reserveLocalPort()
	if err != nil {
		return nil, nil, nil, err
	}
	localAddr := fmt.Sprintf("127.0.0.1:%d", port)
	remotePort := port
	appServerURL := strings.TrimSpace(params.AppServerURL)
	if appServerURL != "" {
		localAddr = strings.TrimPrefix(appServerURL, "ws://")
		localAddr = strings.TrimPrefix(localAddr, "wss://")
	}
	listenURL := fmt.Sprintf("ws://127.0.0.1:%d", remotePort)
	var proc domain.CommandProcess
	if appServerURL == "" {
		spec, err := buildAppServerSpec(params, listenURL)
		if err != nil {
			return nil, nil, nil, err
		}
		proc, err = startAppServerProcess(context.WithoutCancel(ctx), params, spec)
		if err != nil {
			return nil, nil, nil, err
		}
		if err := proc.Start(); err != nil {
			stopAppServerProcess(proc)
			return nil, nil, nil, fmt.Errorf("codex app-server sync start: %w", err)
		}
		if err := waitForAppServer(ctx, localAddr); err != nil {
			stopAppServerProcess(proc)
			return nil, nil, nil, fmt.Errorf("codex app-server sync ready: %w: %s", err, strings.TrimSpace(proc.StderrText()))
		}
	}
	dialURL := "ws://" + localAddr
	if appServerURL != "" {
		dialURL = appServerURL
	}
	dialURL, dialOptions, err := appServerDialTarget(params, dialURL)
	if err != nil {
		stopAppServerProcess(proc)
		return nil, nil, nil, err
	}
	conn, resp, err := websocket.Dial(ctx, dialURL, dialOptions)
	if err != nil {
		stopAppServerProcess(proc)
		return nil, nil, nil, fmt.Errorf("codex app-server sync dial: %w%s", err, websocketDialResponseText(resp))
	}
	conn.SetReadLimit(appServerReadLimit)
	client := newAppServerClient(conn, params.ApprovalHandler)
	if _, err := client.call(ctx, "initialize", map[string]any{
		"clientInfo":   map[string]string{"name": "agen8-sync", "version": "dev"},
		"capabilities": map[string]any{"experimentalApi": true},
	}); err != nil {
		conn.CloseNow()
		stopAppServerProcess(proc)
		return nil, nil, nil, err
	}
	if err := client.notification(ctx, "initialized", map[string]any{}); err != nil {
		conn.CloseNow()
		stopAppServerProcess(proc)
		return nil, nil, nil, err
	}
	result, err := client.call(ctx, "thread/resume", threadResumeParams(params, threadID))
	if err != nil {
		conn.CloseNow()
		stopAppServerProcess(proc)
		return nil, nil, nil, fmt.Errorf("codex app-server sync thread/resume: %w%s", err, runtimeHostDiagnosticSuffix(params))
	}
	if _, err := validateResumedThreadID(threadID, result); err != nil {
		conn.CloseNow()
		stopAppServerProcess(proc)
		return nil, nil, nil, err
	}
	return client, proc, result, nil
}

func appServerEventCanUseActiveTurn(eventType domain.EventType) bool {
	switch eventType {
	case domain.EventText, domain.EventToolCall, domain.EventToolResult:
		return true
	default:
		return false
	}
}

func isAgentInboxTurn(turnID string) bool {
	turnID = strings.TrimSpace(turnID)
	return strings.HasPrefix(turnID, "turn-agent_inbox_")
}

func isSuccessfulCodexEOF(turnID string, emittedProgress bool, err error) bool {
	if isAgentInboxTurn(turnID) {
		return err == nil || isClosedAppServerConnectionError(err)
	}
	return emittedProgress && (err == nil || isClosedAppServerConnectionError(err))
}

func runtimeHostDiagnosticSuffix(params domain.StartParams) string {
	diagnostics := strings.TrimSpace(params.RuntimeHostDiagnostics)
	if diagnostics == "" {
		return ""
	}
	return "; runtime host diagnostics: " + diagnostics
}

func (r *Runtime) appServerSession(ctx context.Context, params domain.StartParams, emit func(domain.Event)) (*appServerSession, error) {
	threadID := strings.TrimSpace(params.SessionRef)
	configSignature := appServerConfigSignature(params)
	if threadID != "" {
		r.mu.Lock()
		if r.sessions != nil {
			if session := r.sessions[threadID]; session != nil {
				if session.configSignature == configSignature {
					r.mu.Unlock()
					if emit != nil {
						emit(domain.Event{Type: domain.EventTurnStarted, SessionRef: threadID})
					}
					return session, nil
				}
				delete(r.sessions, threadID)
				r.mu.Unlock()
				session.client.close()
				stopAppServerProcess(session.proc)
			} else {
				r.mu.Unlock()
			}
		} else {
			r.mu.Unlock()
		}
	}

	var resumeErr error
	for attempt := 0; ; attempt++ {
		session, err := r.openAppServerSession(ctx, params, threadID, emit)
		if err == nil {
			return session, nil
		}
		if threadID == "" || attempt > 0 || !isRetriableAppServerResumeError(err) {
			if resumeErr != nil {
				return nil, fmt.Errorf("codex app-server thread/resume failed after reconnect: %w", err)
			}
			return nil, err
		}
		resumeErr = err
	}
}

func (r *Runtime) openAppServerSession(ctx context.Context, params domain.StartParams, threadID string, emit func(domain.Event)) (*appServerSession, error) {
	port, err := reserveLocalPort()
	if err != nil {
		return nil, err
	}
	localAddr := fmt.Sprintf("127.0.0.1:%d", port)
	remotePort := port
	appServerURL := strings.TrimSpace(params.AppServerURL)
	if appServerURL != "" {
		localAddr = strings.TrimPrefix(appServerURL, "ws://")
		localAddr = strings.TrimPrefix(localAddr, "wss://")
	}
	listenURL := fmt.Sprintf("ws://127.0.0.1:%d", remotePort)
	var proc domain.CommandProcess
	if appServerURL == "" {
		spec, err := buildAppServerSpec(params, listenURL)
		if err != nil {
			return nil, err
		}
		proc, err = startAppServerProcess(context.WithoutCancel(ctx), params, spec)
		if err != nil {
			return nil, err
		}
		if err := proc.Start(); err != nil {
			return nil, fmt.Errorf("codex app-server start: %w", err)
		}
		if err := waitForAppServer(ctx, localAddr); err != nil {
			stopAppServerProcess(proc)
			return nil, fmt.Errorf("codex app-server ready: %w: %s", err, strings.TrimSpace(proc.StderrText()))
		}
	}
	dialURL := "ws://" + localAddr
	if appServerURL != "" {
		dialURL = appServerURL
	}
	dialURL, dialOptions, err := appServerDialTarget(params, dialURL)
	if err != nil {
		stopAppServerProcess(proc)
		return nil, err
	}
	conn, resp, err := websocket.Dial(ctx, dialURL, dialOptions)
	if err != nil {
		stopAppServerProcess(proc)
		return nil, fmt.Errorf("codex app-server dial: %w%s", err, websocketDialResponseText(resp))
	}
	conn.SetReadLimit(appServerReadLimit)

	client := newAppServerClient(conn, params.ApprovalHandler)
	if _, err := client.call(ctx, "initialize", map[string]any{
		"clientInfo":   map[string]string{"name": "agen8", "version": "dev"},
		"capabilities": map[string]any{"experimentalApi": true},
	}); err != nil {
		conn.CloseNow()
		stopAppServerProcess(proc)
		return nil, err
	}
	if err := client.notification(ctx, "initialized", map[string]any{}); err != nil {
		conn.CloseNow()
		stopAppServerProcess(proc)
		return nil, err
	}
	if threadID == "" {
		result, err := client.call(ctx, "thread/start", threadStartParams(params))
		if err != nil {
			conn.CloseNow()
			stopAppServerProcess(proc)
			return nil, fmt.Errorf("%w%s", err, runtimeHostDiagnosticSuffix(params))
		}
		threadID = nestedJSONRPCString(result, "thread", "id")
		if threadID == "" {
			conn.CloseNow()
			stopAppServerProcess(proc)
			return nil, fmt.Errorf("codex app-server thread/start returned no thread id")
		}
		if err := persistAppServerThreadID(params, threadID); err != nil {
			conn.CloseNow()
			stopAppServerProcess(proc)
			return nil, err
		}
	} else {
		result, err := client.call(ctx, "thread/resume", threadResumeParams(params, threadID))
		if err != nil {
			conn.CloseNow()
			stopAppServerProcess(proc)
			return nil, fmt.Errorf("codex app-server thread/resume: %w%s", err, runtimeHostDiagnosticSuffix(params))
		}
		got, err := validateResumedThreadID(threadID, result)
		if err != nil {
			conn.CloseNow()
			stopAppServerProcess(proc)
			return nil, err
		}
		if got != "" {
			threadID = got
		}
	}
	session := &appServerSession{
		threadID:        threadID,
		configSignature: appServerConfigSignature(params),
		proc:            proc,
		client:          client,
	}
	r.mu.Lock()
	if r.sessions == nil {
		r.sessions = make(map[string]*appServerSession)
	}
	if existing := r.sessions[threadID]; existing != nil {
		r.mu.Unlock()
		conn.CloseNow()
		stopAppServerProcess(proc)
		if emit != nil {
			emit(domain.Event{Type: domain.EventTurnStarted, SessionRef: threadID})
		}
		return existing, nil
	}
	r.sessions[threadID] = session
	r.mu.Unlock()
	if emit != nil {
		emit(domain.Event{Type: domain.EventTurnStarted, SessionRef: threadID})
	}
	return session, nil
}

func appServerConfigSignature(params domain.StartParams) string {
	parts := []string{
		"command=" + strings.TrimSpace(params.Command),
		"workdir=" + strings.TrimSpace(params.Workdir),
		"model=" + normalizeCodexCLIModel(params.Model),
		"effort=" + strings.TrimSpace(params.ReasoningEffort),
		"permission=" + strings.TrimSpace(params.PermissionMode),
		"configRef=" + strings.TrimSpace(params.ConfigRef),
		"appServerURL=" + strings.TrimSpace(params.AppServerURL),
	}
	for _, server := range params.MCPServers {
		if server = strings.TrimSpace(server); server != "" {
			parts = append(parts, "mcp="+server)
		}
	}
	for _, arg := range params.ExtraArgs {
		if arg = strings.TrimSpace(arg); arg != "" {
			parts = append(parts, "arg="+arg)
		}
	}
	return strings.Join(parts, "\n")
}

func isRetriableAppServerResumeError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "thread/resume") {
		return false
	}
	return strings.Contains(msg, "unexpected end of json input") ||
		strings.Contains(msg, "failed to read json message") ||
		strings.Contains(msg, "connection closed") ||
		strings.Contains(msg, "eof")
}

func isClosedAppServerConnectionError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection is closed") ||
		strings.Contains(msg, "connection closed") ||
		strings.Contains(msg, "connection reset by peer") ||
		strings.Contains(msg, "use of closed network connection") ||
		strings.Contains(msg, "websocket: close") ||
		strings.Contains(msg, "eof")
}

func appServerDialOptions(params domain.StartParams) (*websocket.DialOptions, error) {
	header := http.Header{}
	configs, err := codexConfigOverrides(params)
	if err != nil {
		return nil, err
	}
	for _, config := range codexConfigValues(configs) {
		header.Add("X-Agen8-Codex-Config", config)
	}
	for _, server := range codexMCPConfigOverrides(params.MCPServers) {
		server = strings.TrimSpace(server)
		if server != "" {
			header.Add("X-Agen8-Codex-MCP-Config", server)
		}
	}
	if len(header) == 0 {
		return nil, nil
	}
	return &websocket.DialOptions{HTTPHeader: header}, nil
}

func appServerDialTarget(params domain.StartParams, rawURL string) (string, *websocket.DialOptions, error) {
	rawURL = strings.TrimSpace(rawURL)
	options, err := appServerDialOptions(params)
	if err != nil {
		return "", nil, err
	}
	if !strings.HasPrefix(rawURL, "unix://") {
		return rawURL, options, nil
	}
	socketPath := strings.TrimSpace(strings.TrimPrefix(rawURL, "unix://"))
	if socketPath == "" {
		return "", nil, fmt.Errorf("codex app-server unix socket path is required")
	}
	if options == nil {
		options = &websocket.DialOptions{}
	}
	options.HTTPClient = &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", socketPath)
		},
	}}
	if options.Host == "" {
		options.Host = "localhost"
	}
	return "ws://localhost/", options, nil
}

func codexConfigValues(args []string) []string {
	out := []string{}
	for i := 0; i < len(args); i++ {
		if args[i] != "--config" {
			continue
		}
		if i+1 < len(args) {
			out = append(out, args[i+1])
			i++
		}
	}
	return out
}

func codexMCPConfigOverrides(servers []string) []string {
	out := make([]string, 0, len(servers))
	for _, server := range servers {
		server = strings.TrimSpace(server)
		if server == "" {
			continue
		}
		if strings.HasPrefix(server, "mcp_servers.agen8.type=") {
			continue
		}
		out = append(out, server)
	}
	return out
}

func websocketDialResponseText(resp *http.Response) string {
	if resp == nil || resp.Body == nil {
		return ""
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return fmt.Sprintf("; response status %s", resp.Status)
	}
	text := strings.TrimSpace(string(body))
	if text == "" {
		return fmt.Sprintf("; response status %s", resp.Status)
	}
	return fmt.Sprintf("; response status %s: %s", resp.Status, text)
}

func persistAppServerThreadID(params domain.StartParams, threadID string) error {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" || params.PersistSessionRef == nil {
		return nil
	}
	if err := params.PersistSessionRef(threadID); err != nil {
		return fmt.Errorf("persist codex app-server thread id %q: %w", threadID, err)
	}
	return nil
}

func (r *Runtime) dropAppServerSession(threadID string, session *appServerSession) {
	if r == nil || session == nil {
		return
	}
	r.mu.Lock()
	if r.sessions != nil && r.sessions[threadID] == session {
		delete(r.sessions, threadID)
	}
	r.mu.Unlock()
	if session.client != nil {
		session.client.close()
	}
	stopAppServerProcess(session.proc)
}

func (r *Runtime) InvalidateSessionRef(sessionRef string) error {
	threadID := strings.TrimSpace(sessionRef)
	if threadID == "" {
		return fmt.Errorf("codex app-server session ref is required")
	}
	if r == nil {
		return fmt.Errorf("codex runtime is nil")
	}
	r.mu.Lock()
	session := (*appServerSession)(nil)
	if r.sessions != nil {
		session = r.sessions[threadID]
		delete(r.sessions, threadID)
	}
	r.mu.Unlock()
	if session == nil {
		return nil
	}
	if session.client != nil {
		session.client.close()
	}
	stopAppServerProcess(session.proc)
	return nil
}

func (r *Runtime) restartAppServerSession(ctx context.Context, params domain.StartParams, threadID string, session *appServerSession, emit func(domain.Event)) (*appServerSession, error) {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return nil, fmt.Errorf("codex thread id is required to restart app-server session")
	}
	r.dropAppServerSession(threadID, session)
	restartParams := params
	restartParams.SessionRef = threadID
	restartParams.Continue = true
	return r.appServerSession(ctx, restartParams, emit)
}

func startAppServerProcess(ctx context.Context, params domain.StartParams, spec domain.StartSpec) (domain.CommandProcess, error) {
	runner := params.CommandRunner
	if runner == nil {
		runner = localAppServerCommandRunner{}
	}
	return runner.StartCommand(ctx, spec)
}

type localAppServerCommandRunner struct{}

func (localAppServerCommandRunner) StartCommand(ctx context.Context, spec domain.StartSpec) (domain.CommandProcess, error) {
	if strings.TrimSpace(spec.Command) == "" {
		return nil, fmt.Errorf("command is required")
	}
	cmd := exec.CommandContext(ctx, spec.Command, spec.Args...)
	if dir := strings.TrimSpace(spec.Workdir); dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(cmd.Environ(), spec.Env...)
	return &localAppServerProcess{cmd: cmd, logs: &bytes.Buffer{}}, nil
}

type localAppServerProcess struct {
	cmd  *exec.Cmd
	logs *bytes.Buffer
}

func (p *localAppServerProcess) StdinPipe() (io.WriteCloser, error) { return p.cmd.StdinPipe() }
func (p *localAppServerProcess) StdoutPipe() (io.ReadCloser, error) { return p.cmd.StdoutPipe() }
func (p *localAppServerProcess) StderrText() string {
	if p == nil || p.logs == nil {
		return ""
	}
	return p.logs.String()
}
func (p *localAppServerProcess) Start() error {
	p.cmd.Stdout = p.logs
	p.cmd.Stderr = p.logs
	return p.cmd.Start()
}
func (p *localAppServerProcess) Wait() error { return p.cmd.Wait() }
func (p *localAppServerProcess) Kill() error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	return p.cmd.Process.Kill()
}

func stopAppServerProcess(proc domain.CommandProcess) {
	if proc == nil {
		return
	}
	_ = proc.Kill()
	_ = proc.Wait()
}

type appServerClient struct {
	conn          *websocket.Conn
	next          atomic.Int64
	writeMu       sync.Mutex
	mu            sync.Mutex
	pending       map[string]chan appServerResponse
	notifications chan jsonrpcMessage
	done          chan error
	closeOnce     sync.Once
	closed        chan struct{}
	approvals     domain.ApprovalHandler
}

type appServerResponse struct {
	result json.RawMessage
	err    error
}

func newAppServerClient(conn *websocket.Conn, approvals domain.ApprovalHandler) *appServerClient {
	c := &appServerClient{
		conn:          conn,
		pending:       make(map[string]chan appServerResponse),
		notifications: make(chan jsonrpcMessage, 256),
		done:          make(chan error, 1),
		closed:        make(chan struct{}),
		approvals:     approvals,
	}
	go c.readLoop()
	return c
}

func (c *appServerClient) callNoResult(ctx context.Context, method string, params any) error {
	_, err := c.call(ctx, method, params)
	return err
}

func (c *appServerClient) notify(ctx context.Context, method string, params any) error {
	id := c.next.Add(1)
	req := map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}
	if err := c.write(ctx, req); err != nil {
		return fmt.Errorf("codex app-server %s write: %w", method, err)
	}
	return nil
}

func (c *appServerClient) notification(ctx context.Context, method string, params any) error {
	req := map[string]any{"jsonrpc": "2.0", "method": method, "params": params}
	if err := c.write(ctx, req); err != nil {
		return fmt.Errorf("codex app-server %s write: %w", method, err)
	}
	return nil
}

func (c *appServerClient) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := c.next.Add(1)
	idKey := fmt.Sprint(id)
	respCh := make(chan appServerResponse, 1)
	c.mu.Lock()
	select {
	case <-c.closed:
		c.mu.Unlock()
		return nil, fmt.Errorf("codex app-server %s: connection is closed", method)
	default:
	}
	c.pending[idKey] = respCh
	c.mu.Unlock()
	defer c.forgetPending(idKey)

	req := map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}
	if err := c.write(ctx, req); err != nil {
		return nil, fmt.Errorf("codex app-server %s write: %w", method, err)
	}
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("codex app-server %s read: %w", method, ctx.Err())
		case resp := <-respCh:
			if resp.err != nil {
				return nil, fmt.Errorf("codex app-server %s: %w", method, resp.err)
			}
			return resp.result, nil
		case err, ok := <-c.done:
			if !ok {
				return nil, fmt.Errorf("codex app-server %s read: connection closed", method)
			}
			return nil, fmt.Errorf("codex app-server %s read: %w", method, err)
		}
	}
}

func (c *appServerClient) write(ctx context.Context, msg any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	select {
	case <-c.closed:
		return fmt.Errorf("connection is closed")
	default:
	}
	return wsjson.Write(ctx, c.conn, msg)
}

func (c *appServerClient) forgetPending(id string) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func (c *appServerClient) readLoop() {
	var err error
	defer func() {
		c.mu.Lock()
		for id, ch := range c.pending {
			delete(c.pending, id)
			ch <- appServerResponse{err: err}
			close(ch)
		}
		c.mu.Unlock()
		c.done <- err
		close(c.done)
		close(c.notifications)
		close(c.closed)
	}()
	for {
		var msg jsonrpcMessage
		if readErr := wsjson.Read(context.Background(), c.conn, &msg); readErr != nil {
			err = readErr
			return
		}
		if msg.ID == nil {
			c.notifications <- msg
			continue
		}
		id := fmt.Sprint(msg.ID)
		c.mu.Lock()
		ch := c.pending[id]
		if ch != nil {
			delete(c.pending, id)
		}
		c.mu.Unlock()
		if ch == nil {
			if strings.TrimSpace(msg.Method) != "" {
				_ = c.respondToServerRequest(context.Background(), msg)
			}
			continue
		}
		if msg.Error != nil {
			ch <- appServerResponse{err: fmt.Errorf("%s", strings.TrimSpace(msg.Error.Message))}
		} else {
			ch <- appServerResponse{result: msg.Result}
		}
		close(ch)
	}
}

func (c *appServerClient) close() {
	c.closeOnce.Do(func() {
		c.conn.CloseNow()
	})
}

func (c *appServerClient) respondToServerRequest(ctx context.Context, msg jsonrpcMessage) error {
	method := strings.TrimSpace(msg.Method)
	if method == "" {
		return nil
	}
	result, ok, err := c.appServerServerRequestResult(ctx, msg)
	if err != nil {
		return c.write(ctx, map[string]any{
			"jsonrpc": "2.0",
			"id":      msg.ID,
			"error": map[string]any{
				"code":    -32000,
				"message": err.Error(),
			},
		})
	}
	if !ok {
		return c.write(ctx, map[string]any{
			"jsonrpc": "2.0",
			"id":      msg.ID,
			"error": map[string]any{
				"code":    -32601,
				"message": fmt.Sprintf("agen8 codex adapter does not implement app-server request %q", method),
			},
		})
	}
	return c.write(ctx, map[string]any{
		"jsonrpc": "2.0",
		"id":      msg.ID,
		"result":  result,
	})
}

func (c *appServerClient) appServerServerRequestResult(ctx context.Context, msg jsonrpcMessage) (map[string]any, bool, error) {
	method := strings.TrimSpace(msg.Method)
	switch method {
	case "item/commandExecution/requestApproval":
		decision, err := c.awaitApproval(ctx, msg)
		if err != nil {
			return nil, true, err
		}
		return map[string]any{"decision": commandApprovalDecision(decision)}, true, nil
	case "item/fileChange/requestApproval":
		decision, err := c.awaitApproval(ctx, msg)
		if err != nil {
			return nil, true, err
		}
		return map[string]any{"decision": commandApprovalDecision(decision)}, true, nil
	case "item/permissions/requestApproval":
		decision, err := c.awaitApproval(ctx, msg)
		if err != nil {
			return nil, true, err
		}
		if strings.EqualFold(decision.Decision, "reject") {
			return nil, true, fmt.Errorf("permission request denied")
		}
		return map[string]any{
			"permissions": map[string]any{
				"network": map[string]any{"enabled": true},
				"fileSystem": map[string]any{
					"read":  nil,
					"write": nil,
				},
			},
			"scope":            "session",
			"strictAutoReview": false,
		}, true, nil
	default:
		return nil, false, nil
	}
}

func (c *appServerClient) awaitApproval(ctx context.Context, msg jsonrpcMessage) (domain.ApprovalDecision, error) {
	if c.approvals == nil {
		return domain.ApprovalDecision{}, fmt.Errorf("approval handler is required for codex app-server request %q", strings.TrimSpace(msg.Method))
	}
	req, err := appServerApprovalRequest(msg)
	if err != nil {
		return domain.ApprovalDecision{}, err
	}
	decision, err := c.approvals(ctx, req)
	if err != nil {
		return domain.ApprovalDecision{}, err
	}
	decision.Decision = strings.ToLower(strings.TrimSpace(decision.Decision))
	decision.Note = strings.TrimSpace(decision.Note)
	switch decision.Decision {
	case "approve", "reject":
		return decision, nil
	default:
		return domain.ApprovalDecision{}, fmt.Errorf("unsupported approval decision %q", decision.Decision)
	}
}

func commandApprovalDecision(decision domain.ApprovalDecision) string {
	if strings.EqualFold(decision.Decision, "reject") {
		return "deny"
	}
	return "acceptForSession"
}

func appServerApprovalRequest(msg jsonrpcMessage) (domain.ApprovalRequest, error) {
	var params map[string]any
	if len(msg.Params) > 0 {
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return domain.ApprovalRequest{}, fmt.Errorf("decode approval request params: %w", err)
		}
	}
	method := strings.TrimSpace(msg.Method)
	item := mapField(params, "item")
	if len(item) == 0 {
		item = params
	}
	toolCallID := firstString(params, "itemId", "item_id", "toolCallId", "tool_call_id")
	if toolCallID == "" {
		toolCallID = firstString(item, "id", "itemId", "item_id")
	}
	approvalID := strings.TrimSpace(fmt.Sprint(msg.ID))
	if approvalID == "<nil>" {
		approvalID = ""
	}
	command := firstString(params, "command")
	if command == "" {
		command = firstString(item, "command")
	}
	path := firstString(params, "path")
	if path == "" {
		path = firstApprovalPath(item)
	}
	data := map[string]string{}
	if cwd := firstString(params, "cwd", "workdir", "workingDirectory"); cwd != "" {
		data["cwd"] = cwd
	}
	if turnID := firstString(params, "turnId", "turn_id"); turnID != "" {
		data["turnId"] = turnID
	}
	return domain.ApprovalRequest{
		ApprovalID: approvalID,
		ToolCallID: toolCallID,
		ToolName:   approvalToolName(method),
		Command:    command,
		Path:       path,
		Summary:    firstString(params, "summary", "title", "description"),
		Method:     method,
		Data:       data,
	}, nil
}

func approvalToolName(method string) string {
	switch strings.TrimSpace(method) {
	case "item/commandExecution/requestApproval":
		return "bash"
	case "item/fileChange/requestApproval":
		return "file_change"
	case "item/permissions/requestApproval":
		return "permissions"
	default:
		return "runtime_approval"
	}
}

func firstApprovalPath(item map[string]any) string {
	if path := firstString(item, "path"); path != "" {
		return path
	}
	changes, ok := item["changes"].([]any)
	if !ok {
		return ""
	}
	for _, raw := range changes {
		change, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if path := firstString(change, "path"); path != "" {
			return path
		}
	}
	return ""
}

func appServerNotificationEvents(msg jsonrpcMessage) ([]domain.Event, error) {
	var params map[string]any
	if len(msg.Params) > 0 {
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return nil, fmt.Errorf("codex app-server notification %s params: %w", msg.Method, err)
		}
	}
	switch msg.Method {
	case "event_msg", "event/msg":
		payload := mapField(params, "payload")
		if len(payload) == 0 {
			payload = params
		}
		return appServerRawEventMsgEvents(payload)
	case "raw_response_item", "response_item":
		payload := mapField(params, "payload")
		if len(payload) == 0 {
			payload = mapField(params, "item")
		}
		if len(payload) == 0 {
			payload = params
		}
		return appServerRawResponseItemEvents(payload)
	case "thread/started":
		return []domain.Event{{Type: domain.EventTurnStarted, SessionRef: firstAppServerSessionRef(params)}}, nil
	case "turn/started":
		return []domain.Event{{Type: domain.EventTurnStarted, TurnID: appServerTurnID(params)}}, nil
	case "item/agentMessage/delta":
		return []domain.Event{{
			Type:   domain.EventText,
			TurnID: firstString(params, "turnId", "turn_id"),
			Text:   rawStringField(params, "delta"),
			Data:   map[string]string{"kind": "assistant"},
		}}, nil
	case "item/commandExecution/outputDelta":
		return appServerCommandExecutionOutputDeltaEvents(params)
	case "item/reasoning/summaryTextDelta":
		return []domain.Event{{
			Type:   domain.EventText,
			TurnID: firstString(params, "turnId", "turn_id"),
			Text:   rawStringField(params, "delta"),
			Data: reasoningEventData(
				firstString(params, "itemId", "item_id"),
				firstString(params, "summaryIndex", "summary_index"),
			),
		}}, nil
	case "item/reasoning/summaryPartAdded", "item/reasoning/textDelta":
		return nil, nil
	case "item/started", "item/updated", "item/completed":
		item := mapField(params, "item")
		if itemType := firstString(item, "type"); itemType == "agent_message" || itemType == "agentMessage" {
			return nil, nil
		} else if itemType == "contextCompaction" {
			return []domain.Event{{
				Type:       domain.EventCompaction,
				TurnID:     firstString(params, "turnId", "turn_id"),
				ServerSide: true,
			}}, nil
		} else if itemType == "reasoning" {
			summary := reasoningSummaryText(item)
			if summary == "" {
				return nil, nil
			}
			return []domain.Event{{
				Type:   domain.EventText,
				TurnID: firstString(params, "turnId", "turn_id"),
				Text:   summary,
				Data: reasoningEventData(
					firstString(item, "id", "itemId", "item_id"),
					"",
				),
			}}, nil
		}
		raw := map[string]any{
			"type": strings.ReplaceAll(msg.Method, "/", "."),
			"item": item,
		}
		if turnID := firstString(params, "turnId", "turn_id"); turnID != "" {
			raw["turn_id"] = turnID
		}
		ev, err := parseItemEvent(raw, 1)
		if err != nil || ev.Type == "" {
			return nil, err
		}
		return []domain.Event{ev}, nil
	case "turn/completed":
		return []domain.Event{{Type: domain.EventTurnCompleted, TurnID: appServerTurnID(params)}}, nil
	case "thread/tokenUsage/updated":
		tokenUsage := mapField(params, "tokenUsage")
		last := mapField(tokenUsage, "last")
		currentTokens, err := intField(last, "inputTokens", "input_tokens")
		if err != nil {
			return nil, fmt.Errorf("codex app-server token usage current context: %w", err)
		}
		budgetTokens, err := intField(tokenUsage, "modelContextWindow", "model_context_window")
		if err != nil {
			return nil, fmt.Errorf("codex app-server token usage context window: %w", err)
		}
		return []domain.Event{{
			Type:          domain.EventContextSize,
			TurnID:        firstString(params, "turnId", "turn_id"),
			CurrentTokens: currentTokens,
			BudgetTokens:  budgetTokens,
		}}, nil
	case "thread/compacted":
		return []domain.Event{{
			Type:       domain.EventCompaction,
			TurnID:     firstString(params, "turnId", "turn_id"),
			ServerSide: true,
		}}, nil
	case "error":
		return []domain.Event{{Type: domain.EventTurnFailed, Error: firstString(params, "message", "error")}}, nil
	default:
		return nil, nil
	}
}

func appServerRawEventMsgEvents(payload map[string]any) ([]domain.Event, error) {
	switch firstString(payload, "type") {
	case "web_search_end":
		return []domain.Event{appServerWebSearchEvent(payload)}, nil
	case "image_generation_call", "image_generation_end", "response.image_generation_call.completed":
		return []domain.Event{parseImageGenerationItem(map[string]any{"type": "event_msg", "item": payload}, payload, firstString(payload, "turn_id", "turnId"))}, nil
	default:
		return nil, nil
	}
}

func appServerRawResponseItemEvents(payload map[string]any) ([]domain.Event, error) {
	switch firstString(payload, "type") {
	case "web_search_call":
		return []domain.Event{appServerWebSearchEvent(payload)}, nil
	case "image_generation_call", "imageGenerationCall":
		return []domain.Event{parseImageGenerationItem(map[string]any{"type": "raw_response_item", "item": payload}, payload, firstString(payload, "turn_id", "turnId"))}, nil
	case "tool_search_call", "tool_search_output":
		return []domain.Event{appServerToolSearchEvent(payload)}, nil
	default:
		return nil, nil
	}
}

func appServerWebSearchEvent(payload map[string]any) domain.Event {
	action := mapField(payload, "action")
	query := firstNonEmptyString(
		firstString(payload, "query"),
		firstString(action, "query", "url"),
	)
	toolCallID := firstString(payload, "call_id", "callId", "id", "item_id", "itemId")
	if toolCallID == "" {
		toolCallID = syntheticToolCallID("web_search", query+"|"+encodeToolPayload(action))
	}
	input := map[string]any{}
	if len(action) != 0 {
		input["action"] = action
		if actionType := firstString(action, "type"); actionType != "" {
			input["type"] = actionType
		}
	}
	if query != "" {
		input["query"] = query
	}
	if queries, ok := action["queries"]; ok {
		input["queries"] = queries
	}
	if url := firstString(action, "url"); url != "" {
		input["url"] = url
	}
	result := firstNonEmptyString(
		extractWebSearchResultPayload(payload),
		encodeToolPayload(action),
	)
	data := map[string]string{
		"status": "completed",
		"op":     "web_search",
		"input":  compactJSON(input),
	}
	if query != "" {
		data["query"] = query
	}
	if result != "" {
		data["result"] = result
		data["outputPreview"] = result
	}
	if turnID := firstString(payload, "turn_id", "turnId"); turnID != "" {
		data["turnId"] = turnID
	}
	return domain.Event{
		Type:       domain.EventToolResult,
		TurnID:     firstString(payload, "turn_id", "turnId"),
		ToolCallID: toolCallID,
		ToolName:   "web_search",
		Text:       result,
		Data:       data,
	}
}

func appServerToolSearchEvent(payload map[string]any) domain.Event {
	itemType := firstString(payload, "type")
	toolCallID := firstString(payload, "call_id", "callId", "id", "item_id", "itemId")
	input := map[string]any{}
	if args := mapField(payload, "arguments"); len(args) != 0 {
		for k, v := range args {
			input[k] = v
		}
	}
	query := firstString(input, "query")
	if toolCallID == "" {
		toolCallID = syntheticToolCallID("tool_search", itemType+"|"+query+"|"+encodeToolPayload(input))
	}
	result := firstNonEmptyString(
		encodeToolPayload(payload["tools"]),
		encodeToolPayload(payload["result"]),
		encodeToolPayload(payload["output"]),
	)
	status := firstString(payload, "status")
	if status == "" {
		status = "completed"
	}
	data := map[string]string{
		"status":        status,
		"op":            "tool_search",
		"input":         compactJSON(input),
		"codexItemType": itemType,
	}
	if query != "" {
		data["query"] = query
	}
	if result != "" {
		data["result"] = result
		data["outputPreview"] = result
	}
	if turnID := firstString(payload, "turn_id", "turnId"); turnID != "" {
		data["turnId"] = turnID
	}
	return domain.Event{
		Type:       domain.EventToolResult,
		TurnID:     firstString(payload, "turn_id", "turnId"),
		ToolCallID: toolCallID,
		ToolName:   "tool_search",
		Text:       result,
		Data:       data,
	}
}

func appServerCommandExecutionOutputDeltaEvents(params map[string]any) ([]domain.Event, error) {
	toolCallID := firstString(params, "itemId", "item_id")
	if toolCallID == "" {
		return nil, fmt.Errorf("codex app-server command output delta itemId is required")
	}
	delta := rawStringField(params, "delta")
	if delta == "" {
		return nil, nil
	}
	return []domain.Event{{
		Type:       domain.EventToolResult,
		TurnID:     firstString(params, "turnId", "turn_id"),
		ToolCallID: toolCallID,
		ToolName:   "bash",
		Text:       delta,
		Data: map[string]string{
			"status":        "in_progress",
			"op":            "bash",
			"sourceType":    "cli",
			"outputDelta":   "true",
			"stdout":        delta,
			"result":        delta,
			"outputFull":    delta,
			"outputPreview": delta,
		},
	}}, nil
}

func appServerTurnID(params map[string]any) string {
	if turnID := firstString(params, "turnId", "turn_id"); turnID != "" {
		return turnID
	}
	return nestedMapString(params, "turn", "id")
}

func firstAppServerSessionRef(params map[string]any) string {
	for _, value := range []string{
		nestedMapString(params, "thread", "id"),
		firstString(params, "threadId", "thread_id"),
	} {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func reasoningEventData(itemID, summaryIndex string) map[string]string {
	data := map[string]string{"kind": "reasoning"}
	if itemID != "" {
		data["itemId"] = itemID
	}
	if summaryIndex != "" {
		data["summaryIndex"] = summaryIndex
	}
	return data
}

func reasoningSummaryText(item map[string]any) string {
	if text := stringField(item, "text"); text != "" {
		return text
	}
	value, ok := item["summary"]
	if !ok {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []any:
		parts := make([]string, 0, len(typed))
		for _, part := range typed {
			text, ok := part.(string)
			if !ok {
				continue
			}
			text = strings.TrimSpace(text)
			if text == "" {
				continue
			}
			parts = append(parts, text)
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

func threadStartParams(params domain.StartParams) map[string]any {
	out := map[string]any{
		"cwd":                    strings.TrimSpace(params.Workdir),
		"developerInstructions":  strings.TrimSpace(params.SystemPrompt),
		"ephemeral":              false,
		"persistExtendedHistory": true,
	}
	applyCodexThreadPermissionParams(out, params)
	if config := codexThreadConfigOverrides(params); len(config) > 0 {
		out["config"] = config
	}
	if model := normalizeCodexCLIModel(params.Model); model != "" {
		out["model"] = model
	}
	return out
}

func threadResumeParams(params domain.StartParams, threadID string) map[string]any {
	out := map[string]any{
		"threadId":               strings.TrimSpace(threadID),
		"cwd":                    strings.TrimSpace(params.Workdir),
		"developerInstructions":  strings.TrimSpace(params.SystemPrompt),
		"persistExtendedHistory": true,
	}
	applyCodexThreadPermissionParams(out, params)
	if config := codexThreadConfigOverrides(params); len(config) > 0 {
		out["config"] = config
	}
	if model := normalizeCodexCLIModel(params.Model); model != "" {
		out["model"] = model
	}
	return out
}

func turnStartParams(params domain.StartParams, threadID string, text string, attachments []domain.PromptAttachment) map[string]any {
	out := map[string]any{
		"threadId": strings.TrimSpace(threadID),
		"input":    codexInputItems(text, attachments),
		"cwd":      strings.TrimSpace(params.Workdir),
	}
	applyCodexTurnPermissionParams(out, params)
	if model := normalizeCodexCLIModel(params.Model); model != "" {
		out["model"] = model
	}
	if effort := strings.TrimSpace(params.ReasoningEffort); effort != "" {
		out["effort"] = effort
	}
	return out
}

func codexInputItems(text string, attachments []domain.PromptAttachment) []map[string]any {
	items := make([]map[string]any, 0, 1+len(attachments))
	if strings.TrimSpace(text) != "" {
		items = append(items, map[string]any{"type": "text", "text": text, "text_elements": []any{}})
	}
	for _, attachment := range attachments {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(attachment.MediaType)), "image/") {
			items = append(items, map[string]any{
				"type": "localImage",
				"path": strings.TrimSpace(attachment.URI),
			})
		}
	}
	return items
}

func codexInjectedUserItems(text string, attachments []domain.PromptAttachment) []map[string]any {
	text = strings.TrimSpace(text)
	if len(attachments) > 0 {
		lines := []string{text}
		for _, attachment := range attachments {
			uri := strings.TrimSpace(attachment.URI)
			if uri == "" {
				continue
			}
			name := strings.TrimSpace(attachment.Name)
			if name == "" {
				name = uri
			}
			lines = append(lines, fmt.Sprintf("Attachment: %s (%s)", name, uri))
		}
		text = strings.TrimSpace(strings.Join(lines, "\n"))
	}
	return []map[string]any{{
		"type": "message",
		"role": "user",
		"content": []map[string]any{{
			"type": "input_text",
			"text": text,
		}},
	}}
}

func applyCodexThreadPermissionParams(out map[string]any, params domain.StartParams) {
	switch strings.ToLower(strings.TrimSpace(params.PermissionMode)) {
	case "codex/default":
		return
	case "codex/auto-review":
		out["approvalsReviewer"] = "auto_review"
		out["sandbox"] = "workspace-write"
	default:
		out["approvalPolicy"] = "never"
		out["sandbox"] = "danger-full-access"
	}
}

func applyCodexTurnPermissionParams(out map[string]any, params domain.StartParams) {
	switch strings.ToLower(strings.TrimSpace(params.PermissionMode)) {
	case "codex/default":
		return
	case "codex/auto-review":
		out["approvalsReviewer"] = "auto_review"
		out["sandboxPolicy"] = map[string]any{"type": "workspaceWrite"}
	default:
		out["approvalPolicy"] = "never"
		out["sandboxPolicy"] = map[string]any{"type": "dangerFullAccess"}
	}
}

func codexThreadConfigOverrides(params domain.StartParams) map[string]any {
	config := map[string]any{}
	if model := normalizeCodexCLIModel(params.Model); model != "" {
		config["model"] = model
	}
	if effort := strings.TrimSpace(params.ReasoningEffort); effort != "" {
		config["model_reasoning_effort"] = effort
	}
	return config
}

func validateResumedThreadID(requested string, result json.RawMessage) (string, error) {
	requested = strings.TrimSpace(requested)
	got := nestedJSONRPCString(result, "thread", "id")
	if got == "" {
		return "", nil
	}
	if !strings.EqualFold(got, requested) {
		return "", fmt.Errorf("codex app-server thread/resume returned thread id %q for requested thread id %q", got, requested)
	}
	return got, nil
}

func reserveLocalPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("reserve codex app-server port: %w", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func waitForAppServer(ctx context.Context, addr string) error {
	client := http.Client{Timeout: 250 * time.Millisecond}
	url := fmt.Sprintf("http://%s/readyz", addr)
	deadline := time.Now().Add(30 * time.Second)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %s", url)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("build codex app-server readiness request: %w", err)
		}
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func nestedJSONRPCString(raw json.RawMessage, keys ...string) string {
	if len(raw) == 0 {
		return ""
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return ""
	}
	return nestedMapString(obj, keys...)
}

func nestedMapString(obj map[string]any, keys ...string) string {
	cur := obj
	for i, key := range keys {
		if i == len(keys)-1 {
			return firstString(cur, key)
		}
		next, _ := cur[key].(map[string]any)
		if next == nil {
			return ""
		}
		cur = next
	}
	return ""
}
