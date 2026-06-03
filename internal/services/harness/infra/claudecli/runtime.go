package claudecli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/coder/websocket"
	"github.com/tinoosan/agen8-mcp-server/internal/services/harness/domain"
)

const RuntimeKind = "claude-cli"
const claudePermissionPromptTool = "mcp__agen8__harness_approval"

type Runtime struct{}

var claudeVersionSegmentPattern = regexp.MustCompile(`(\d)\.(\d)`)

type localCommandRunner struct{}

func (localCommandRunner) StartCommand(ctx context.Context, spec domain.StartSpec) (domain.CommandProcess, error) {
	if strings.TrimSpace(spec.Command) == "" {
		return nil, fmt.Errorf("command is required")
	}
	cmd := exec.CommandContext(ctx, spec.Command, spec.Args...)
	if dir := strings.TrimSpace(spec.Workdir); dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(cmd.Environ(), spec.Env...)
	return &localCommandProcess{cmd: cmd, stderr: &bytes.Buffer{}}, nil
}

type localCommandProcess struct {
	cmd    *exec.Cmd
	stderr *bytes.Buffer
}

func (p *localCommandProcess) StdinPipe() (io.WriteCloser, error) { return p.cmd.StdinPipe() }
func (p *localCommandProcess) StdoutPipe() (io.ReadCloser, error) { return p.cmd.StdoutPipe() }
func (p *localCommandProcess) StderrText() string                 { return p.stderr.String() }
func (p *localCommandProcess) Start() error {
	p.cmd.Stderr = p.stderr
	return p.cmd.Start()
}
func (p *localCommandProcess) Wait() error { return p.cmd.Wait() }
func (p *localCommandProcess) Kill() error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	return p.cmd.Process.Kill()
}

type remoteBridgeCommandRunner struct {
	url string
}

const remoteBridgeStdinEOF = `{"agen8Bridge":"stdin_eof"}`
const remoteBridgeStartMessageType = "start"

func (r remoteBridgeCommandRunner) StartCommand(ctx context.Context, spec domain.StartSpec) (domain.CommandProcess, error) {
	if strings.TrimSpace(r.url) == "" {
		return nil, fmt.Errorf("claude bridge url is required")
	}
	if strings.TrimSpace(spec.Command) == "" {
		return nil, fmt.Errorf("command is required")
	}
	if strings.TrimSpace(spec.Workdir) == "" {
		return nil, fmt.Errorf("workdir is required")
	}
	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()
	return &remoteBridgeCommandProcess{
		ctx:          ctx,
		url:          r.url,
		spec:         spec,
		stdinReader:  stdinReader,
		stdinWriter:  stdinWriter,
		stdoutReader: stdoutReader,
		stdoutWriter: stdoutWriter,
		done:         make(chan error, 1),
	}, nil
}

type remoteBridgeCommandProcess struct {
	ctx          context.Context
	url          string
	spec         domain.StartSpec
	stdinReader  *io.PipeReader
	stdinWriter  *io.PipeWriter
	stdoutReader *io.PipeReader
	stdoutWriter *io.PipeWriter
	conn         *websocket.Conn
	stderr       string
	done         chan error
	once         sync.Once
}

func (p *remoteBridgeCommandProcess) StdinPipe() (io.WriteCloser, error) {
	return p.stdinWriter, nil
}

func (p *remoteBridgeCommandProcess) StdoutPipe() (io.ReadCloser, error) {
	return p.stdoutReader, nil
}

func (p *remoteBridgeCommandProcess) StderrText() string {
	if p == nil {
		return ""
	}
	return p.stderr
}

func (p *remoteBridgeCommandProcess) Start() error {
	payload, err := json.Marshal(remoteBridgeCommandSpec{
		Type:    remoteBridgeStartMessageType,
		Command: strings.TrimSpace(p.spec.Command),
		Args:    append([]string(nil), p.spec.Args...),
		Workdir: strings.TrimSpace(p.spec.Workdir),
		Env:     append([]string(nil), p.spec.Env...),
	})
	if err != nil {
		return fmt.Errorf("marshal claude bridge command spec: %w", err)
	}
	conn, resp, err := websocket.Dial(p.ctx, p.url, nil)
	if err != nil {
		p.stderr = remoteBridgeDialResponse(resp)
		if p.stderr != "" {
			return fmt.Errorf("%w: %s", err, p.stderr)
		}
		return err
	}
	p.conn = conn
	if err := p.conn.Write(p.ctx, websocket.MessageText, payload); err != nil {
		p.conn.CloseNow()
		p.conn = nil
		return fmt.Errorf("write claude bridge command spec: %w", err)
	}
	go p.writeLoop()
	go p.readLoop()
	return nil
}

type remoteBridgeCommandSpec struct {
	Type    string   `json:"type"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Workdir string   `json:"workdir"`
	Env     []string `json:"env"`
}

func (p *remoteBridgeCommandProcess) Wait() error {
	err := <-p.done
	_ = p.stdinReader.Close()
	_ = p.stdoutWriter.Close()
	if p.conn != nil {
		p.conn.CloseNow()
	}
	return err
}

func (p *remoteBridgeCommandProcess) Kill() error {
	p.once.Do(func() {
		if p.conn != nil {
			_ = p.conn.Close(websocket.StatusGoingAway, "killed")
		}
		_ = p.stdinReader.Close()
		_ = p.stdinWriter.Close()
		_ = p.stdoutReader.Close()
		_ = p.stdoutWriter.Close()
	})
	return nil
}

func (p *remoteBridgeCommandProcess) writeLoop() {
	scanner := bufio.NewScanner(p.stdinReader)
	scanner.Buffer(make([]byte, 0, 128*1024), 4*1024*1024)
	for scanner.Scan() {
		if err := p.conn.Write(p.ctx, websocket.MessageText, scanner.Bytes()); err != nil {
			p.finish(err)
			return
		}
	}
	if err := scanner.Err(); err != nil {
		p.finish(err)
		return
	}
	if err := p.conn.Write(p.ctx, websocket.MessageText, []byte(remoteBridgeStdinEOF)); err != nil {
		p.finish(err)
	}
}

func (p *remoteBridgeCommandProcess) readLoop() {
	defer p.stdoutWriter.Close()
	for {
		msgType, reader, err := p.conn.Reader(p.ctx)
		if err != nil {
			var closeErr websocket.CloseError
			if errors.As(err, &closeErr) && strings.TrimSpace(closeErr.Reason) != "" {
				p.stderr = strings.TrimSpace(closeErr.Reason)
				p.finish(fmt.Errorf("claude bridge closed: %s", p.stderr))
				return
			}
			p.finish(nil)
			return
		}
		if msgType != websocket.MessageText {
			p.finish(fmt.Errorf("claude bridge returned non-text message"))
			return
		}
		if _, err := io.Copy(p.stdoutWriter, reader); err != nil {
			p.finish(err)
			return
		}
		if _, err := p.stdoutWriter.Write([]byte("\n")); err != nil {
			p.finish(err)
			return
		}
	}
}

func (p *remoteBridgeCommandProcess) finish(err error) {
	p.once.Do(func() {
		p.done <- err
	})
}

func remoteBridgeDialResponse(resp *http.Response) string {
	if resp == nil || resp.Body == nil {
		return ""
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return strings.TrimSpace(resp.Status)
	}
	text := strings.TrimSpace(string(body))
	if text == "" {
		return strings.TrimSpace(resp.Status)
	}
	return strings.TrimSpace(resp.Status + ": " + text)
}

func New() Runtime {
	return Runtime{}
}

func (Runtime) Kind() string {
	return RuntimeKind
}

func (r Runtime) ExecuteSessionTurn(ctx context.Context, params domain.StartParams, input domain.SessionTurnInput, emit func(domain.Event)) (domain.SessionTurnResult, error) {
	text := strings.TrimSpace(input.Text)
	if text == "" && len(input.Attachments) == 0 {
		return domain.SessionTurnResult{}, fmt.Errorf("claude-cli session turn text or attachment is required")
	}
	start, err := r.StartStreamingInput(params)
	if err != nil {
		return domain.SessionTurnResult{}, fmt.Errorf("claude-cli session turn start spec: %w", err)
	}
	if strings.TrimSpace(start.Command) == "" {
		return domain.SessionTurnResult{}, fmt.Errorf("claude-cli session turn command is required")
	}
	runner := params.CommandRunner
	if runner == nil && strings.TrimSpace(params.AppServerURL) != "" {
		runner = remoteBridgeCommandRunner{url: strings.TrimSpace(params.AppServerURL)}
	}
	if runner == nil {
		runner = localCommandRunner{}
	}
	proc, err := runner.StartCommand(ctx, start)
	if err != nil {
		return domain.SessionTurnResult{}, fmt.Errorf("claude-cli session turn command process: %w", err)
	}
	stdout, err := proc.StdoutPipe()
	if err != nil {
		return domain.SessionTurnResult{}, fmt.Errorf("claude-cli session turn stdout pipe: %w", err)
	}
	stdin, err := proc.StdinPipe()
	if err != nil {
		return domain.SessionTurnResult{}, fmt.Errorf("claude-cli session turn stdin pipe: %w", err)
	}
	if err := proc.Start(); err != nil {
		return domain.SessionTurnResult{}, fmt.Errorf("claude-cli session turn start process: %w", err)
	}
	if err := r.WriteStreamingPrompt(stdin, domain.PromptInput{
		TurnID:          strings.TrimSpace(input.TurnID),
		Text:            text,
		Attachments:     append([]domain.PromptAttachment(nil), input.Attachments...),
		ReasoningEffort: strings.TrimSpace(params.ReasoningEffort),
	}); err != nil {
		_ = stdin.Close()
		_ = proc.Kill()
		_ = proc.Wait()
		return domain.SessionTurnResult{}, fmt.Errorf("claude-cli session turn write prompt: %w", err)
	}
	if err := stdin.Close(); err != nil {
		_ = proc.Kill()
		_ = proc.Wait()
		return domain.SessionTurnResult{}, fmt.Errorf("claude-cli session turn close stdin: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 128*1024), 4*1024*1024)
	completed := false
	emittedProgress := false
	sessionRef := strings.TrimSpace(params.SessionRef)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		events, err := r.ParseEvents([]byte(line + "\n"))
		if err != nil {
			_ = proc.Kill()
			_ = proc.Wait()
			return domain.SessionTurnResult{}, err
		}
		for _, ev := range events {
			if eventSessionRef := strings.TrimSpace(ev.SessionRef); eventSessionRef != "" {
				sessionRef = eventSessionRef
				if params.PersistSessionRef != nil {
					if err := params.PersistSessionRef(eventSessionRef); err != nil {
						_ = proc.Kill()
						_ = proc.Wait()
						return domain.SessionTurnResult{}, fmt.Errorf("claude-cli session turn persist session ref: %w", err)
					}
				}
			}
			if emit != nil && ev.Type != "" {
				emit(ev)
			}
			switch ev.Type {
			case domain.EventText, domain.EventToolCall, domain.EventToolResult:
				emittedProgress = true
			case domain.EventTurnCompleted:
				completed = true
			case domain.EventTurnFailed:
				_ = proc.Kill()
				_ = proc.Wait()
				return domain.SessionTurnResult{}, fmt.Errorf("claude-cli session turn failed: %s", strings.TrimSpace(ev.Error))
			}
		}
	}
	if err := scanner.Err(); err != nil {
		_ = proc.Kill()
		_ = proc.Wait()
		return domain.SessionTurnResult{}, fmt.Errorf("claude-cli session turn read stdout: %w", err)
	}
	if err := proc.Wait(); err != nil {
		errText := strings.TrimSpace(proc.StderrText())
		if errText == "" {
			return domain.SessionTurnResult{}, fmt.Errorf("claude-cli session turn wait: %w", err)
		}
		return domain.SessionTurnResult{}, fmt.Errorf("claude-cli session turn wait: %w: %s", err, errText)
	}
	if !completed {
		if emittedProgress {
			if emit != nil {
				emit(domain.Event{
					Type:       domain.EventTurnCompleted,
					TurnID:     strings.TrimSpace(input.TurnID),
					SessionRef: sessionRef,
				})
			}
			return domain.SessionTurnResult{}, nil
		}
		errText := strings.TrimSpace(proc.StderrText())
		if errText == "" {
			return domain.SessionTurnResult{}, fmt.Errorf("claude-cli session turn completed without terminal event")
		}
		return domain.SessionTurnResult{}, fmt.Errorf("claude-cli session turn completed without terminal event: %s", errText)
	}
	return domain.SessionTurnResult{}, nil
}

func (Runtime) Start(params domain.StartParams) (domain.StartSpec, error) {
	command := strings.TrimSpace(params.Command)
	if command == "" {
		command = "claude"
	}
	args := []string{"--output-format", "stream-json", "--verbose"}
	sessionRef := strings.TrimSpace(params.SessionRef)
	if params.Continue {
		if sessionRef != "" {
			args = append(args, "--resume", sessionRef)
		} else {
			args = append(args, "--continue")
		}
	} else if sessionRef != "" {
		args = append(args, "--session-id", sessionRef)
	}
	if model := normalizeClaudeCLIModel(params.Model); model != "" {
		args = append(args, "--model", model)
	}
	if effort := claudeCLIReasoningEffort(params.ReasoningEffort); effort != "" {
		args = append(args, "--effort", effort)
	}
	if mode, err := claudePermissionModeArg(params.PermissionMode); mode != "" || err != nil {
		if err != nil {
			return domain.StartSpec{}, err
		}
		if !hasClaudePermissionMode(params.ExtraArgs) {
			args = append(args, "--permission-mode", mode)
		}
	}
	if identity := strings.TrimSpace(params.SystemPrompt); identity != "" && !params.Continue {
		args = append(args, "--system-prompt", identity)
	}
	for _, server := range params.MCPServers {
		server = strings.TrimSpace(server)
		if server == "" {
			continue
		}
		args = append(args, "--mcp-config", server)
	}
	for _, arg := range params.ExtraArgs {
		arg = strings.TrimSpace(arg)
		if arg == "" {
			continue
		}
		args = append(args, arg)
	}
	return domain.StartSpec{
		Command: command,
		Args:    args,
		Workdir: strings.TrimSpace(params.Workdir),
		Env:     append([]string(nil), params.Env...),
	}, nil
}

func (Runtime) StartStreamingInput(params domain.StartParams) (domain.StartSpec, error) {
	command := strings.TrimSpace(params.Command)
	if command == "" {
		command = "claude"
	}
	args := []string{
		"--print",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--verbose",
	}
	if mode, err := claudePermissionModeArg(params.PermissionMode); mode != "" || err != nil {
		if err != nil {
			return domain.StartSpec{}, err
		}
		if !hasClaudePermissionMode(params.ExtraArgs) {
			args = append(args, "--permission-mode", mode)
		}
	} else if strings.TrimSpace(params.AppServerURL) != "" && !hasClaudePermissionMode(params.ExtraArgs) {
		args = append(args, "--permission-mode", "bypassPermissions")
	}
	if shouldUseClaudePermissionPromptTool(params) {
		args = append(args, "--permission-prompt-tool", claudePermissionPromptTool)
	}
	sessionRef := strings.TrimSpace(params.SessionRef)
	if params.Continue && sessionRef != "" {
		args = append(args, "--resume", sessionRef)
	} else if params.Continue {
		args = append(args, "--continue")
	} else if sessionRef != "" {
		args = append(args, "--session-id", sessionRef)
	}
	if model := normalizeClaudeCLIModel(params.Model); model != "" {
		args = append(args, "--model", model)
	}
	if effort := claudeCLIReasoningEffort(params.ReasoningEffort); effort != "" {
		args = append(args, "--effort", effort)
	}
	if identity := strings.TrimSpace(params.SystemPrompt); identity != "" && !params.Continue {
		args = append(args, "--system-prompt", identity)
	}
	for _, server := range params.MCPServers {
		server = strings.TrimSpace(server)
		if server == "" {
			continue
		}
		args = append(args, "--mcp-config", server)
	}
	for _, arg := range params.ExtraArgs {
		arg = strings.TrimSpace(arg)
		if arg == "" {
			continue
		}
		args = append(args, arg)
	}
	return domain.StartSpec{
		Command: command,
		Args:    args,
		Workdir: strings.TrimSpace(params.Workdir),
		Env:     append([]string(nil), params.Env...),
	}, nil
}

func claudePermissionModeArg(permissionMode string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(permissionMode)) {
	case "":
		return "", nil
	case "claude-code/ask-permissions":
		return "default", nil
	case "claude-code/accept-edits":
		return "acceptEdits", nil
	case "claude-code/plan":
		return "plan", nil
	case "claude-code/auto":
		return "auto", nil
	case "claude-code/bypass-permissions":
		return "bypassPermissions", nil
	default:
		return "", fmt.Errorf("unsupported claude permission mode %q", permissionMode)
	}
}

func hasClaudePermissionMode(args []string) bool {
	for _, arg := range args {
		switch strings.TrimSpace(arg) {
		case "--permission-mode", "--dangerously-skip-permissions", "--allow-dangerously-skip-permissions":
			return true
		}
	}
	return false
}

func shouldUseClaudePermissionPromptTool(params domain.StartParams) bool {
	if params.ApprovalHandler == nil || hasClaudePermissionPromptTool(params.ExtraArgs) {
		return false
	}
	mode, err := claudePermissionModeArg(params.PermissionMode)
	if err != nil {
		return false
	}
	return mode != "" && !strings.EqualFold(strings.TrimSpace(mode), "bypassPermissions")
}

func hasClaudePermissionPromptTool(args []string) bool {
	for _, arg := range args {
		if strings.TrimSpace(arg) == "--permission-prompt-tool" {
			return true
		}
	}
	return false
}

func normalizeClaudeCLIModel(raw string) string {
	model := strings.TrimSpace(raw)
	if model == "" {
		return ""
	}
	if _, remainder, ok := strings.Cut(model, "/"); ok {
		model = strings.TrimSpace(remainder)
	}
	return claudeVersionSegmentPattern.ReplaceAllString(model, "$1-$2")
}

// claudeCLIReasoningEffort maps the canonical reasoning effort to the Claude
// Code --effort flag value. Claude Code supports low, medium, high, xhigh, and
// max on models that expose effort; Agen8's canonical values map directly.
func claudeCLIReasoningEffort(canonical string) string {
	switch strings.ToLower(strings.TrimSpace(canonical)) {
	case "low":
		return "low"
	case "medium":
		return "medium"
	case "high":
		return "high"
	case "xhigh":
		return "xhigh"
	case "max":
		return "max"
	default:
		return ""
	}
}

func (Runtime) ParseEvents(stream []byte) ([]domain.Event, error) {
	reader := bufio.NewReader(bytes.NewReader(stream))
	lineNo := 0
	var out []domain.Event
	for {
		rawLine, err := reader.ReadBytes('\n')
		if errors.Is(err, io.EOF) && len(rawLine) == 0 {
			break
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("claude-cli parse events: read stream: %w", err)
		}
		lineNo++
		line := bytes.TrimSpace(rawLine)
		if len(line) > 0 {
			events, parseErr := parseLineEvents(line, lineNo)
			if parseErr != nil {
				return nil, parseErr
			}
			for _, ev := range events {
				if ev.Type != "" {
					out = append(out, ev)
				}
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
	}
	return out, nil
}

func parseLineEvents(line []byte, lineNo int) ([]domain.Event, error) {
	var raw map[string]any
	if err := json.Unmarshal(line, &raw); err != nil {
		return nil, fmt.Errorf("claude-cli parse events: line %d: %w", lineNo, err)
	}
	sessionRef := firstString(raw, "session_id", "sessionId", "conversation_id", "conversationId")
	turnID := firstString(raw, "turn_id", "turnId")
	eventType := stringField(raw, "type")
	switch eventType {
	case "content_block_delta", "text", "text_delta":
		return []domain.Event{{
			Type:       domain.EventText,
			TurnID:     turnID,
			SessionRef: sessionRef,
			Text:       firstString(raw, "delta", "text"),
		}}, nil
	case "tool_use", "tool_call":
		toolName, inputPayload := normalizedToolUse(raw)
		return []domain.Event{{
			Type:       domain.EventToolCall,
			TurnID:     turnID,
			SessionRef: sessionRef,
			ToolCallID: firstString(raw, "id", "tool_call_id"),
			ToolName:   toolName,
			Data: map[string]string{
				"status": "in_progress",
				"input":  inputPayload,
			},
		}}, nil
	case "tool_result":
		status := "completed"
		if boolField(raw, "is_error") {
			status = "failed"
		}
		text := firstString(raw, "content", "result", "text", "message")
		return []domain.Event{{
			Type:       domain.EventToolResult,
			TurnID:     turnID,
			SessionRef: sessionRef,
			ToolCallID: firstString(raw, "tool_call_id", "id"),
			Text:       text,
			Data: map[string]string{
				"status": status,
				"result": text,
			},
		}}, nil
	case "turn_started", "message_start":
		return []domain.Event{{
			Type:       domain.EventTurnStarted,
			TurnID:     turnID,
			SessionRef: sessionRef,
		}}, nil
	case "turn_completed", "message_stop":
		return []domain.Event{{
			Type:       domain.EventTurnCompleted,
			TurnID:     turnID,
			SessionRef: sessionRef,
		}}, nil
	case "turn_failed", "error":
		return []domain.Event{{
			Type:       domain.EventTurnFailed,
			TurnID:     turnID,
			SessionRef: sessionRef,
			Error:      firstString(raw, "error", "message", "result"),
		}}, nil
	case "assistant":
		return parseAssistantLine(raw, lineNo)
	case "user":
		return parseUserLine(raw, lineNo)
	case "result":
		subtype := strings.ToLower(strings.TrimSpace(firstString(raw, "subtype", "status")))
		message := firstString(raw, "message", "result")
		if message == "" {
			message = nestedString(raw, "error", "message")
		}
		isFailureSubtype := strings.Contains(subtype, "error") ||
			strings.Contains(subtype, "fail") ||
			strings.Contains(subtype, "cancel")
		if isFailureSubtype || boolField(raw, "is_error") {
			return []domain.Event{{
				Type:       domain.EventTurnFailed,
				SessionRef: sessionRef,
				Error:      message,
			}}, nil
		}
		usage, usageErr := parseUsage(mapField(raw, "usage"))
		if usageErr != nil {
			return nil, fmt.Errorf("claude-cli parse events: line %d: %w", lineNo, usageErr)
		}
		return []domain.Event{{
			Type:       domain.EventTurnCompleted,
			SessionRef: sessionRef,
			Usage:      usage,
		}}, nil
	case "system":
		// Map init to turn_started so the session id is captured early; ignore the
		// rest of system lifecycle rows.
		if strings.EqualFold(firstString(raw, "subtype"), "init") {
			return []domain.Event{{
				Type:       domain.EventTurnStarted,
				SessionRef: sessionRef,
			}}, nil
		}
		return nil, nil
	case "stream_event":
		// Complete assistant/user/result records still arrive in stream-json mode.
		// Ignore raw stream deltas to avoid duplicate text emission.
		return nil, nil
	case "rate_limit_event":
		// Out-of-band telemetry row emitted by Claude CLI in stream-json mode.
		// It does not represent a turn transition or tool event.
		return nil, nil
	default:
		return nil, fmt.Errorf("claude-cli parse events: line %d: unsupported type %q", lineNo, strings.TrimSpace(eventType))
	}
}

func parseAssistantLine(raw map[string]any, lineNo int) ([]domain.Event, error) {
	message := mapField(raw, "message")
	sessionRef := firstString(raw, "session_id", "sessionId", "conversation_id", "conversationId")
	turnID := firstString(raw, "turn_id", "turnId")
	if turnID == "" {
		turnID = firstString(message, "id")
	}
	blocks := contentBlocks(message)
	events := make([]domain.Event, 0, len(blocks))
	for idx, block := range blocks {
		switch stringField(block, "type") {
		case "text":
			text := firstString(block, "text")
			if text == "" {
				continue
			}
			if claudeAPIErrorText(text) != "" {
				events = append(events, domain.Event{
					Type:       domain.EventTurnFailed,
					TurnID:     turnID,
					SessionRef: sessionRef,
					Error:      claudeAPIErrorText(text),
				})
				continue
			}
			events = append(events, domain.Event{
				Type:       domain.EventText,
				TurnID:     turnID,
				SessionRef: sessionRef,
				Text:       text,
				Data: map[string]string{
					"kind": "assistant",
				},
			})
		case "tool_use":
			toolCallID := firstString(block, "id", "tool_call_id", "tool_use_id")
			if toolCallID == "" {
				return nil, fmt.Errorf("claude-cli parse events: line %d: assistant content[%d] tool_use id is required", lineNo, idx)
			}
			toolName, inputPayload := normalizedToolUse(block)
			if toolName == "" {
				return nil, fmt.Errorf("claude-cli parse events: line %d: assistant content[%d] tool_use name is required", lineNo, idx)
			}
			events = append(events, domain.Event{
				Type:       domain.EventToolCall,
				TurnID:     turnID,
				SessionRef: sessionRef,
				ToolCallID: toolCallID,
				ToolName:   toolName,
				Data: map[string]string{
					"status": "in_progress",
					"input":  inputPayload,
				},
			})
		}
	}
	return events, nil
}

func normalizedToolUse(block map[string]any) (toolName string, inputPayload string) {
	toolName = firstString(block, "name", "tool_name")
	inputPayload = compactJSON(block["input"])
	if toolName == "" {
		return "", inputPayload
	}
	lowerName := strings.ToLower(strings.TrimSpace(toolName))
	// Claude's MCP bridge often emits a generic tool name with the actual
	// MCP target in input.{server,tool,arguments}. Normalize that into a
	// namespaced tool name so downstream op mapping can render native cards.
	if lowerName != "tool" && lowerName != "mcp_tool" {
		return toolName, inputPayload
	}
	input := mapField(block, "input")
	if len(input) == 0 {
		return toolName, inputPayload
	}
	server := firstString(input, "server", "server_name", "source", "source_id", "mcp_server")
	nestedTool := firstString(input, "tool", "tool_name", "name")
	if nestedTool != "" {
		if server != "" && !strings.ContainsAny(nestedTool, "/:") {
			toolName = strings.TrimSpace(server + "/" + nestedTool)
		} else {
			toolName = nestedTool
		}
	}
	if args := input["arguments"]; args != nil {
		inputPayload = compactJSON(args)
		return toolName, inputPayload
	}
	if args := input["args"]; args != nil {
		inputPayload = compactJSON(args)
		return toolName, inputPayload
	}
	return toolName, inputPayload
}

func parseUserLine(raw map[string]any, lineNo int) ([]domain.Event, error) {
	message := mapField(raw, "message")
	sessionRef := firstString(raw, "session_id", "sessionId", "conversation_id", "conversationId")
	turnID := firstString(raw, "turn_id", "turnId")
	if turnID == "" {
		turnID = firstString(message, "id")
	}
	blocks := contentBlocks(message)
	events := make([]domain.Event, 0, len(blocks))
	for idx, block := range blocks {
		if !strings.EqualFold(firstString(block, "type"), "tool_result") {
			continue
		}
		toolCallID := firstString(block, "tool_use_id", "tool_call_id", "id")
		if toolCallID == "" {
			return nil, fmt.Errorf("claude-cli parse events: line %d: user content[%d] tool_result tool_use_id is required", lineNo, idx)
		}
		text := toolResultText(block["content"])
		if text == "" {
			text = firstString(block, "result", "text")
		}
		status := "completed"
		if boolField(block, "is_error") {
			status = "failed"
		}
		events = append(events, domain.Event{
			Type:       domain.EventToolResult,
			TurnID:     turnID,
			SessionRef: sessionRef,
			ToolCallID: toolCallID,
			Text:       text,
			Data: map[string]string{
				"status": status,
				"result": text,
			},
		})
	}
	return events, nil
}

func compactJSON(raw any) string {
	if raw == nil {
		return ""
	}
	switch typed := raw.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(encoded))
	}
}

func toolResultText(raw any) string {
	switch typed := raw.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		return strings.TrimSpace(firstString(typed, "text", "content", "result"))
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			switch block := item.(type) {
			case string:
				if text := strings.TrimSpace(block); text != "" {
					parts = append(parts, text)
				}
			case map[string]any:
				if strings.TrimSpace(stringField(block, "type")) != "" && !strings.EqualFold(stringField(block, "type"), "text") {
					continue
				}
				if text := strings.TrimSpace(firstString(block, "text", "content")); text != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", typed))
	}
}

func mapField(raw map[string]any, key string) map[string]any {
	value, ok := raw[key]
	if !ok {
		return map[string]any{}
	}
	typed, ok := value.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return typed
}

func contentBlocks(message map[string]any) []map[string]any {
	value, ok := message["content"]
	if !ok {
		return nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	blocks := make([]map[string]any, 0, len(items))
	for _, item := range items {
		block, ok := item.(map[string]any)
		if !ok {
			continue
		}
		blocks = append(blocks, block)
	}
	return blocks
}

func nestedString(raw map[string]any, keys ...string) string {
	current := any(raw)
	for _, key := range keys {
		typed, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current, ok = typed[key]
		if !ok {
			return ""
		}
	}
	switch typed := current.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", typed))
	}
}

func boolField(raw map[string]any, key string) bool {
	value, ok := raw[key]
	if !ok {
		return false
	}
	typed, ok := value.(bool)
	return ok && typed
}

func (Runtime) WritePrompt(w io.Writer, input domain.PromptInput) error {
	if strings.TrimSpace(input.Text) == "" && len(input.Attachments) == 0 {
		return fmt.Errorf("claude-cli write prompt: text or attachment is required")
	}
	payload := map[string]any{
		"type":    "prompt",
		"turn_id": strings.TrimSpace(input.TurnID),
		"text":    claudePromptTextWithAttachmentPaths(input.Text, input.Attachments),
	}
	return domain.WriteJSONLine(w, payload)
}

func (Runtime) WriteStreamingPrompt(w io.Writer, input domain.PromptInput) error {
	if strings.TrimSpace(input.Text) == "" && len(input.Attachments) == 0 {
		return fmt.Errorf("claude-cli write streaming prompt: text or attachment is required")
	}
	content := []map[string]string{}
	if strings.TrimSpace(input.Text) != "" {
		content = append(content, map[string]string{
			"type": "text",
			"text": input.Text,
		})
	}
	for _, attachment := range input.Attachments {
		content = append(content, map[string]string{
			"type": "text",
			"text": fmt.Sprintf("Attached image %s (%s): %s", strings.TrimSpace(attachment.Name), strings.TrimSpace(attachment.MediaType), strings.TrimSpace(attachment.URI)),
		})
	}
	payload := map[string]any{
		"type": "user",
		"message": map[string]any{
			"role":    "user",
			"content": content,
		},
	}
	return domain.WriteJSONLine(w, payload)
}

func claudeAPIErrorText(text string) string {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "API Error:") {
		return ""
	}
	return text
}

func claudePromptTextWithAttachmentPaths(text string, attachments []domain.PromptAttachment) string {
	text = strings.TrimSpace(text)
	if len(attachments) == 0 {
		return text
	}
	var b strings.Builder
	if text != "" {
		b.WriteString(text)
		b.WriteString("\n\n")
	}
	b.WriteString("Attached files:\n")
	for _, attachment := range attachments {
		b.WriteString("- ")
		b.WriteString(strings.TrimSpace(attachment.Name))
		b.WriteString(" (")
		b.WriteString(strings.TrimSpace(attachment.MediaType))
		b.WriteString("): ")
		b.WriteString(strings.TrimSpace(attachment.URI))
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func (Runtime) WriteToolResult(w io.Writer, input domain.ToolResultInput) error {
	if strings.TrimSpace(input.ToolCallID) == "" {
		return fmt.Errorf("claude-cli write tool result: tool_call_id is required")
	}
	payload := map[string]any{
		"type":         "tool_result",
		"turn_id":      strings.TrimSpace(input.TurnID),
		"tool_call_id": strings.TrimSpace(input.ToolCallID),
		"result":       input.Result,
		"is_error":     input.IsError,
	}
	return domain.WriteJSONLine(w, payload)
}

func firstString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		if s := stringField(raw, key); s != "" {
			return s
		}
	}
	return ""
}

func stringField(raw map[string]any, key string) string {
	value, ok := raw[key]
	if !ok {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", typed))
	}
}

func parseUsage(raw map[string]any) (*domain.Usage, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	cacheCreation, err := intField(raw, "cache_creation_input_tokens", "cacheCreationInputTokens")
	if err != nil {
		return nil, err
	}
	input, err := intField(raw, "input_tokens", "inputTokens", "prompt_tokens", "promptTokens")
	if err != nil {
		return nil, err
	}
	output, err := intField(raw, "output_tokens", "outputTokens", "completion_tokens", "completionTokens")
	if err != nil {
		return nil, err
	}
	total, err := intField(raw, "total_tokens", "totalTokens")
	if err != nil {
		return nil, err
	}
	reasoning, err := intField(raw, "reasoning_tokens", "reasoningTokens")
	if err != nil {
		return nil, err
	}
	cacheRead, err := intField(raw, "cache_read_input_tokens", "cacheReadInputTokens", "cache_read_tokens", "cacheReadTokens")
	if err != nil {
		return nil, err
	}
	usage := domain.Usage{
		InputTokens:          input + cacheCreation + cacheRead,
		OutputTokens:         output,
		TotalTokens:          total,
		ReasoningTokens:      reasoning,
		CacheReadInputTokens: cacheRead,
	}
	if usage.TotalTokens == 0 && (usage.InputTokens != 0 || usage.OutputTokens != 0) {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	}
	if usage.InputTokens == 0 && usage.OutputTokens == 0 && usage.TotalTokens == 0 && usage.ReasoningTokens == 0 && usage.CacheReadInputTokens == 0 {
		return nil, nil
	}
	return &usage, nil
}

func intField(raw map[string]any, keys ...string) (int, error) {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case float64:
			return int(typed), nil
		case int:
			return typed, nil
		case json.Number:
			n, err := typed.Int64()
			if err == nil {
				return int(n), nil
			}
			return 0, fmt.Errorf("usage field %q must be an integer", key)
		case string:
			n, err := strconv.Atoi(strings.TrimSpace(typed))
			if err == nil {
				return n, nil
			}
			return 0, fmt.Errorf("usage field %q must be an integer", key)
		default:
			return 0, fmt.Errorf("usage field %q must be an integer", key)
		}
	}
	return 0, nil
}
