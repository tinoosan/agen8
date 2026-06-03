package runner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	harnessapp "github.com/tinoosan/agen8-mcp-server/internal/services/harness/app"
	harness "github.com/tinoosan/agen8-mcp-server/internal/services/harness/domain"
)

type TurnFailedError struct {
	Message string
}

func (e TurnFailedError) Error() string {
	msg := strings.TrimSpace(e.Message)
	if msg == "" {
		return "external harness turn failed"
	}
	return msg
}

func IsTurnFailed(err error) bool {
	var target TurnFailedError
	return errors.As(err, &target)
}

var ErrTurnCanceled = harness.ErrTurnCanceled

// Config configures execution through an external harness
// process (for example codex/claude-cli).
type Config struct {
	Runtime           harness.Runtime
	Workdir           string
	Workspace         string
	Command           string
	MCPServers        []string
	ExtraArgs         []string
	InitialSessionRef string
	PersistSessionRef func(sessionRef string) error
}

// Service executes turns via an external harness process.
type Service struct {
	runtime           harness.Runtime
	workdir           string
	workspace         string
	command           string
	mcpServers        []string
	extraArgs         []string
	enrichers         []harnessEventEnricher
	execSeq           uint64
	sessionMu         sync.RWMutex
	sessionRef        string
	sessionErr        error
	persistSessionRef func(sessionRef string) error
	streamMu          sync.Mutex
	streamProc        any
}

type streamingHarnessProcess struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	events  chan streamingHarnessEvent
	done    chan error
	stderr  *bytes.Buffer
	key     string
	started bool
}

type streamingHarnessEvent struct {
	events []harness.Event
	err    error
}

var lookupCommandPath = exec.LookPath

func New(cfg Config) (*Service, error) {
	if cfg.Runtime == nil {
		return nil, fmt.Errorf("external harness runtime is required")
	}
	return &Service{
		runtime:           cfg.Runtime,
		workdir:           strings.TrimSpace(cfg.Workdir),
		workspace:         strings.TrimSpace(cfg.Workspace),
		command:           strings.TrimSpace(cfg.Command),
		mcpServers:        append([]string(nil), cfg.MCPServers...),
		extraArgs:         append([]string(nil), cfg.ExtraArgs...),
		enrichers:         defaultHarnessEventEnrichers(strings.TrimSpace(cfg.Workdir)),
		sessionRef:        strings.TrimSpace(cfg.InitialSessionRef),
		persistSessionRef: cfg.PersistSessionRef,
	}, nil
}

var _ Executor = (*Service)(nil)

func (r *Service) harnessEnv() []string {
	if r == nil {
		return nil
	}
	out := make([]string, 0, 2)
	if projectRoot := strings.TrimSpace(r.workdir); projectRoot != "" {
		out = append(out, "PROJECT_ROOT="+projectRoot)
	}
	if workspaceRoot := strings.TrimSpace(r.workspace); workspaceRoot != "" {
		out = append(out, "WORKSPACE_ROOT="+workspaceRoot)
	}
	return out
}

// childEnv builds the environment for a spawned harness process. It inherits
// the current process's environment and appends any harness-specific vars.
// On macOS it also strips Malloc* debug variables (MallocStackLogging,
// MallocScribble, etc.) that Xcode/Instruments inject into the parent process;
// those variables cause spurious warnings and can hang child processes under
// memory pressure. On other platforms the filter is a no-op.
func childEnv(extra []string) []string {
	parent := os.Environ()
	out := make([]string, 0, len(parent)+len(extra))
	for _, kv := range parent {
		if runtime.GOOS == "darwin" {
			key, _, _ := strings.Cut(kv, "=")
			if strings.HasPrefix(key, "Malloc") {
				continue
			}
		}
		out = append(out, kv)
	}
	return append(out, extra...)
}

func (r *Service) Execute(ctx context.Context, cfg TurnConfig, input TurnInput) <-chan Event {
	ch := make(chan Event, 1)
	go func() {
		defer close(ch)
		emit := func(evt Event) {
			select {
			case <-ctx.Done():
				return
			case ch <- evt:
			}
		}
		emitTerminal := func(evt Event) {
			ch <- evt
		}
		result, step, err := r.executeExternal(ctx, cfg, input, emit)
		if err != nil {
			if ctx.Err() != nil {
				err = ErrTurnCanceled
			}
			evt := Event{
				Kind: EventError,
				Step: step,
				Err:  err,
			}
			if errors.Is(err, ErrTurnCanceled) {
				evt.Err = ErrTurnCanceled
				emitTerminal(evt)
				return
			}
			emit(evt)
			return
		}
		emitTerminal(Event{
			Kind:   EventDone,
			Step:   step,
			Result: result,
		})
	}()
	return ch
}

func (r *Service) executeExternal(
	ctx context.Context,
	cfg TurnConfig,
	input TurnInput,
	emit func(Event),
) (Result, int, error) {
	if r == nil || r.runtime == nil {
		return Result{}, 1, fmt.Errorf("external harness runtime is required")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return Result{}, 1, fmt.Errorf("agent model is required")
	}
	if strings.TrimSpace(input.Instruction) == "" {
		return Result{}, 1, fmt.Errorf("external harness instruction is required")
	}
	systemPrompt, err := externalHarnessSystemPrompt(ctx, cfg, 1)
	if err != nil {
		return Result{}, 1, err
	}
	cfg.SystemPrompt = systemPrompt
	if sessionRuntime, ok := r.runtime.(harness.SessionRuntime); ok {
		return r.executeSessionExternal(ctx, sessionRuntime, cfg, input, emit)
	}
	if streamingRuntime, ok := r.runtime.(harness.StreamingInputRuntime); ok {
		return r.executeStreamingExternal(ctx, streamingRuntime, cfg, input, emit)
	}

	command, err := r.resolveHarnessCommand()
	if err != nil {
		return Result{}, 1, err
	}
	sessionRef, continuing := r.prepareSession(r.runtime.Kind())

	start, err := r.runtime.Start(harness.StartParams{
		Command:         command,
		Workdir:         r.workdir,
		Env:             r.harnessEnv(),
		SystemPrompt:    strings.TrimSpace(cfg.SystemPrompt),
		Model:           strings.TrimSpace(cfg.Model),
		SessionRef:      sessionRef,
		Continue:        continuing,
		MCPServers:      append([]string(nil), r.mcpServers...),
		ExtraArgs:       append([]string(nil), r.extraArgs...),
		ReasoningEffort: strings.TrimSpace(cfg.ReasoningEffort),
	})
	if err != nil {
		return Result{}, 1, fmt.Errorf("external harness start spec: %w", err)
	}
	if strings.TrimSpace(start.Command) == "" {
		return Result{}, 1, fmt.Errorf("external harness start command is required")
	}

	cmd := exec.CommandContext(context.WithoutCancel(ctx), start.Command, start.Args...)
	if dir := strings.TrimSpace(start.Workdir); dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = childEnv(start.Env)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, 1, fmt.Errorf("external harness stdout pipe: %w", err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return Result{}, 1, fmt.Errorf("external harness stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return Result{}, 1, fmt.Errorf("external harness start process: %w", err)
	}

	execScope := fmt.Sprintf("ext-%d", atomic.AddUint64(&r.execSeq, 1))
	localTurnID := "turn-" + execScope

	promptText, err := harnessTurnInstruction(input)
	if err != nil {
		stdin.Close()
		cmd.Process.Kill()
		cmd.Wait()
		return Result{}, 1, err
	}
	_ = continuing
	if err := r.runtime.WritePrompt(stdin, harness.PromptInput{
		TurnID:      localTurnID,
		Text:        promptText,
		Attachments: append([]harness.PromptAttachment(nil), input.Attachments...),
	}); err != nil {
		stdin.Close()
		cmd.Wait()
		return Result{}, 1, fmt.Errorf("external harness write prompt: %w", err)
	}
	if err := stdin.Close(); err != nil {
		cmd.Wait()
		return Result{}, 1, fmt.Errorf("external harness close stdin: %w", err)
	}

	step := 1
	activeTurnID := ""
	toolNameByCallID := map[string]string{}
	toolDataByCallID := map[string]map[string]string{}
	var finalText strings.Builder
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 128*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		events, err := r.runtime.ParseEvents([]byte(line + "\n"))
		if err != nil {
			cmd.Process.Kill()
			cmd.Wait()
			return Result{}, step, fmt.Errorf("external harness parse events: %w", err)
		}
		for _, hev := range events {
			hev = hydrateHarnessEventTurnID(hev, &activeTurnID, localTurnID)
			r.captureSessionRef(hev)
			hev = scopeHarnessEventToolCallID(hev, execScope)
			hev = hydrateHarnessEventToolName(hev, toolNameByCallID)
			hev = hydrateHarnessEventToolData(hev, toolDataByCallID)
			rememberHarnessEventToolName(toolNameByCallID, hev)
			rememberHarnessEventToolData(toolDataByCallID, hev)
			for _, enriched := range r.enrichHarnessEvent(ctx, hev) {
				enriched = hydrateHarnessEventTurnID(enriched, &activeTurnID, localTurnID)
				enriched = hydrateHarnessEventToolName(enriched, toolNameByCallID)
				enriched = hydrateHarnessEventToolData(enriched, toolDataByCallID)
				rememberHarnessEventToolName(toolNameByCallID, enriched)
				rememberHarnessEventToolData(toolDataByCallID, enriched)
				if err := emitExternalHarnessEvent(step, enriched, &finalText, nil, nil, emit); err != nil {
					cmd.Process.Kill()
					cmd.Wait()
					return Result{}, step, err
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return Result{}, step, fmt.Errorf("external harness read stdout: %w", err)
	}
	if err := cmd.Wait(); err != nil {
		errText := strings.TrimSpace(stderr.String())
		if errText == "" {
			return Result{}, step, fmt.Errorf("external harness wait: %w", err)
		}
		return Result{}, step, fmt.Errorf("external harness wait: %w: %s", err, errText)
	}

	text := strings.TrimSpace(finalText.String())
	if text == "" {
		return Result{}, step, fmt.Errorf("external harness completed without assistant text")
	}
	return Result{
		Text:   text,
		Status: StatusSucceeded,
	}, step, nil
}

func (r *Service) executeSessionExternal(
	ctx context.Context,
	sessionRuntime harness.SessionRuntime,
	cfg TurnConfig,
	input TurnInput,
	emit func(Event),
) (Result, int, error) {
	r.streamMu.Lock()
	defer r.streamMu.Unlock()

	command, err := r.resolveHarnessCommand()
	if err != nil {
		return Result{}, 1, err
	}
	sessionRef, continuing := r.prepareSession(sessionRuntime.Kind())
	params := harness.StartParams{
		Command:           command,
		Workdir:           r.workdir,
		Env:               r.harnessEnv(),
		SystemPrompt:      strings.TrimSpace(cfg.SystemPrompt),
		Model:             strings.TrimSpace(cfg.Model),
		SessionRef:        sessionRef,
		Continue:          continuing,
		MCPServers:        append([]string(nil), r.mcpServers...),
		ExtraArgs:         append([]string(nil), r.extraArgs...),
		ReasoningEffort:   strings.TrimSpace(cfg.ReasoningEffort),
		PersistSessionRef: r.persistSessionRef,
	}
	promptText, err := harnessTurnInstruction(input)
	if err != nil {
		return Result{}, 1, err
	}

	step := 1
	execScope := fmt.Sprintf("ext-%d", atomic.AddUint64(&r.execSeq, 1))
	localTurnID := "turn-" + execScope
	activeTurnID := ""
	toolNameByCallID := map[string]string{}
	toolDataByCallID := map[string]map[string]string{}
	var finalText strings.Builder
	var textSegment int
	textOpen := true
	steering := make(chan harness.PromptInput)
	var steeringDone <-chan struct{}
	if cfg.SteeringCh != nil {
		done := make(chan struct{})
		steeringDone = done
		go func() {
			defer close(done)
			defer close(steering)
			for {
				select {
				case <-ctx.Done():
					return
				case turn, ok := <-cfg.SteeringCh:
					if !ok {
						return
					}
					text := strings.TrimSpace(turn.Instruction)
					if text == "" {
						continue
					}
					select {
					case steering <- harness.PromptInput{Text: text, Attachments: append([]harness.PromptAttachment(nil), turn.Attachments...)}:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	} else {
		close(steering)
	}
	var emitErr error
	handleEvent := func(hev harness.Event) {
		if emitErr != nil {
			return
		}
		hev = hydrateHarnessEventTurnID(hev, &activeTurnID, localTurnID)
		r.captureSessionRef(hev)
		hev = scopeHarnessEventToolCallID(hev, execScope)
		hev = hydrateHarnessEventToolName(hev, toolNameByCallID)
		hev = hydrateHarnessEventToolData(hev, toolDataByCallID)
		rememberHarnessEventToolName(toolNameByCallID, hev)
		rememberHarnessEventToolData(toolDataByCallID, hev)
		for _, enriched := range r.enrichHarnessEvent(ctx, hev) {
			enriched = hydrateHarnessEventTurnID(enriched, &activeTurnID, localTurnID)
			enriched = hydrateHarnessEventToolName(enriched, toolNameByCallID)
			enriched = hydrateHarnessEventToolData(enriched, toolDataByCallID)
			rememberHarnessEventToolName(toolNameByCallID, enriched)
			rememberHarnessEventToolData(toolDataByCallID, enriched)
			if err := emitExternalHarnessEvent(step, enriched, &finalText, &textSegment, &textOpen, emit); err != nil {
				emitErr = err
				return
			}
		}
	}
	_, err = sessionRuntime.ExecuteSessionTurn(ctx, params, harness.SessionTurnInput{
		TurnID:      localTurnID,
		Text:        promptText,
		Attachments: append([]harness.PromptAttachment(nil), input.Attachments...),
		Steering:    steering,
	}, handleEvent)
	if steeringDone != nil {
		select {
		case <-steeringDone:
		default:
		}
	}
	if err != nil {
		return Result{}, step, err
	}
	if emitErr != nil {
		return Result{}, step, emitErr
	}
	if err := r.sessionPersistError(); err != nil {
		return Result{}, step, err
	}
	text := strings.TrimSpace(finalText.String())
	if text == "" {
		return Result{}, step, fmt.Errorf("external harness completed without assistant text")
	}
	return Result{
		Text:   text,
		Status: StatusSucceeded,
	}, step, nil
}

func (r *Service) sessionPersistError() error {
	if r == nil {
		return nil
	}
	r.sessionMu.RLock()
	defer r.sessionMu.RUnlock()
	return r.sessionErr
}

func externalHarnessSystemPrompt(ctx context.Context, cfg TurnConfig, step int) (string, error) {
	baseSystem := strings.TrimSpace(cfg.SystemPrompt)
	if baseSystem == "" {
		baseSystem = harnessapp.DefaultSystemPrompt()
	}
	if cfg.PromptSource == nil {
		return baseSystem, nil
	}
	if pp, ok := cfg.PromptSource.(PromptPartitioner); ok {
		stableSystem, err := pp.StableSystemPrompt(ctx, baseSystem, step)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(stableSystem) != "" {
			return strings.TrimSpace(stableSystem), nil
		}
		return baseSystem, nil
	}
	updatedSystem, err := cfg.PromptSource.SystemPrompt(ctx, baseSystem, step)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(updatedSystem) != "" {
		return strings.TrimSpace(updatedSystem), nil
	}
	return baseSystem, nil
}

func (r *Service) executeStreamingExternal(
	ctx context.Context,
	streamingRuntime harness.StreamingInputRuntime,
	cfg TurnConfig,
	input TurnInput,
	emit func(Event),
) (Result, int, error) {
	r.streamMu.Lock()
	defer r.streamMu.Unlock()

	command, err := r.resolveHarnessCommand()
	if err != nil {
		return Result{}, 1, err
	}
	sessionRef, continuing := r.prepareSession(streamingRuntime.Kind())
	params := harness.StartParams{
		Command:         command,
		Workdir:         r.workdir,
		Env:             r.harnessEnv(),
		SystemPrompt:    strings.TrimSpace(cfg.SystemPrompt),
		Model:           strings.TrimSpace(cfg.Model),
		SessionRef:      sessionRef,
		Continue:        continuing,
		MCPServers:      append([]string(nil), r.mcpServers...),
		ExtraArgs:       append([]string(nil), r.extraArgs...),
		ReasoningEffort: strings.TrimSpace(cfg.ReasoningEffort),
	}
	key := streamingHarnessKey(params)
	proc, err := r.ensureStreamingHarnessProcess(ctx, streamingRuntime, params, key)
	if err != nil {
		return Result{}, 1, err
	}

	promptText, err := harnessTurnInstruction(input)
	if err != nil {
		return Result{}, 1, err
	}
	_ = continuing
	execScope := fmt.Sprintf("ext-%d", atomic.AddUint64(&r.execSeq, 1))
	localTurnID := "turn-" + execScope
	activeTurnID := ""
	if err := streamingRuntime.WriteStreamingPrompt(proc.stdin, harness.PromptInput{
		TurnID:          localTurnID,
		Text:            promptText,
		Attachments:     append([]harness.PromptAttachment(nil), input.Attachments...),
		ReasoningEffort: strings.TrimSpace(cfg.ReasoningEffort),
	}); err != nil {
		r.stopStreamingHarnessProcess()
		return Result{}, 1, fmt.Errorf("external harness write streaming prompt: %w", err)
	}

	step := 1
	toolNameByCallID := map[string]string{}
	toolDataByCallID := map[string]map[string]string{}
	var finalText strings.Builder
	for {
		select {
		case <-ctx.Done():
			r.stopStreamingHarnessProcess()
			return Result{}, step, ctx.Err()
		case err := <-proc.done:
			r.streamProc = nil
			if err == nil {
				err = fmt.Errorf("external harness streaming process exited before turn completed")
			}
			if stderr := strings.TrimSpace(proc.stderr.String()); stderr != "" {
				return Result{}, step, fmt.Errorf("%w: %s", err, stderr)
			}
			return Result{}, step, err
		case item := <-proc.events:
			if item.err != nil {
				r.stopStreamingHarnessProcess()
				return Result{}, step, fmt.Errorf("external harness parse events: %w", item.err)
			}
			for _, hev := range item.events {
				hev = hydrateHarnessEventTurnID(hev, &activeTurnID, localTurnID)
				r.captureSessionRef(hev)
				hev = scopeHarnessEventToolCallID(hev, execScope)
				hev = hydrateHarnessEventToolName(hev, toolNameByCallID)
				hev = hydrateHarnessEventToolData(hev, toolDataByCallID)
				rememberHarnessEventToolName(toolNameByCallID, hev)
				rememberHarnessEventToolData(toolDataByCallID, hev)
				for _, enriched := range r.enrichHarnessEvent(ctx, hev) {
					enriched = hydrateHarnessEventTurnID(enriched, &activeTurnID, localTurnID)
					enriched = hydrateHarnessEventToolName(enriched, toolNameByCallID)
					enriched = hydrateHarnessEventToolData(enriched, toolDataByCallID)
					rememberHarnessEventToolName(toolNameByCallID, enriched)
					rememberHarnessEventToolData(toolDataByCallID, enriched)
					if err := emitExternalHarnessEvent(step, enriched, &finalText, nil, nil, emit); err != nil {
						r.stopStreamingHarnessProcess()
						return Result{}, step, err
					}
					if enriched.Type == harness.EventTurnCompleted {
						text := strings.TrimSpace(finalText.String())
						if text == "" {
							return Result{}, step, fmt.Errorf("external harness completed without assistant text")
						}
						return Result{
							Text:   text,
							Status: StatusSucceeded,
						}, step, nil
					}
				}
			}
		}
	}
}

func (r *Service) ensureStreamingHarnessProcess(
	ctx context.Context,
	streamingRuntime harness.StreamingInputRuntime,
	params harness.StartParams,
	key string,
) (*streamingHarnessProcess, error) {
	if proc, ok := r.streamProc.(*streamingHarnessProcess); ok && proc.started && proc.key == key {
		select {
		case err := <-proc.done:
			r.streamProc = nil
			if err != nil {
				return nil, err
			}
		default:
			return proc, nil
		}
	}
	r.stopStreamingHarnessProcess()
	start, err := streamingRuntime.StartStreamingInput(params)
	if err != nil {
		return nil, fmt.Errorf("external harness streaming start spec: %w", err)
	}
	if strings.TrimSpace(start.Command) == "" {
		return nil, fmt.Errorf("external harness streaming start command is required")
	}
	cmd := exec.CommandContext(context.WithoutCancel(ctx), start.Command, start.Args...)
	if dir := strings.TrimSpace(start.Workdir); dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = childEnv(start.Env)
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("external harness streaming stdout pipe: %w", err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("external harness streaming stdin pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("external harness streaming start process: %w", err)
	}
	proc := &streamingHarnessProcess{
		cmd:     cmd,
		stdin:   stdin,
		events:  make(chan streamingHarnessEvent, 128),
		done:    make(chan error, 1),
		stderr:  stderr,
		key:     key,
		started: true,
	}
	r.streamProc = proc
	go r.readStreamingHarnessEvents(streamingRuntime, stdout, proc)
	go func() {
		proc.done <- cmd.Wait()
		close(proc.done)
	}()
	return proc, nil
}

func (r *Service) readStreamingHarnessEvents(
	streamingRuntime harness.StreamingInputRuntime,
	stdout io.Reader,
	proc *streamingHarnessProcess,
) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 128*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		events, err := streamingRuntime.ParseEvents([]byte(line + "\n"))
		proc.events <- streamingHarnessEvent{events: events, err: err}
	}
	if err := scanner.Err(); err != nil {
		proc.events <- streamingHarnessEvent{err: fmt.Errorf("external harness read stdout: %w", err)}
	}
}

func (r *Service) stopStreamingHarnessProcess() {
	if r == nil || r.streamProc == nil {
		return
	}
	proc, ok := r.streamProc.(*streamingHarnessProcess)
	if !ok {
		r.streamProc = nil
		return
	}
	r.streamProc = nil
	proc.stdin.Close()
	if proc.cmd != nil && proc.cmd.Process != nil {
		proc.cmd.Process.Kill()
	}
	select {
	case <-proc.done:
	default:
	}
}

func streamingHarnessKey(params harness.StartParams) string {
	parts := []string{
		strings.TrimSpace(params.Command),
		strings.TrimSpace(params.Workdir),
		strings.TrimSpace(params.SystemPrompt),
		strings.TrimSpace(params.Model),
	}
	parts = append(parts, params.MCPServers...)
	parts = append(parts, params.ExtraArgs...)
	return strings.Join(parts, "\x00")
}

func rememberHarnessEventToolData(dataByCallID map[string]map[string]string, ev harness.Event) {
	if len(dataByCallID) == 0 && dataByCallID == nil {
		return
	}
	if ev.Type != harness.EventToolCall {
		return
	}
	callID := strings.TrimSpace(ev.ToolCallID)
	if callID == "" || len(ev.Data) == 0 {
		return
	}
	copied := make(map[string]string, len(ev.Data))
	for key, value := range ev.Data {
		copied[key] = strings.TrimSpace(value)
	}
	dataByCallID[callID] = copied
}

func hydrateHarnessEventToolData(ev harness.Event, dataByCallID map[string]map[string]string) harness.Event {
	if ev.Type != harness.EventToolResult {
		return ev
	}
	callID := strings.TrimSpace(ev.ToolCallID)
	if callID == "" {
		return ev
	}
	callData, ok := dataByCallID[callID]
	if !ok || len(callData) == 0 {
		return ev
	}
	if ev.Data == nil {
		ev.Data = map[string]string{}
	}
	for _, key := range []string{
		"input", "action", "op", "command", "argvPreview", "background", "pid",
		"path", "writeMode", "toolNameRaw", "sourceType", "sourceId",
	} {
		if strings.TrimSpace(ev.Data[key]) != "" {
			continue
		}
		if value := strings.TrimSpace(callData[key]); value != "" {
			ev.Data[key] = value
		}
	}
	return ev
}

func (r *Service) currentSessionRef() (string, bool) {
	if r == nil {
		return "", false
	}
	r.sessionMu.RLock()
	defer r.sessionMu.RUnlock()
	sessionRef := strings.TrimSpace(r.sessionRef)
	return sessionRef, sessionRef != ""
}

func (r *Service) prepareSession(kind string) (sessionRef string, continuing bool) {
	if current, ok := r.currentSessionRef(); ok {
		return current, true
	}
	if harness.NormalizeRuntimeKind(kind) != "claude-cli" {
		return "", false
	}
	generated := uuid.NewString()
	r.sessionMu.Lock()
	defer r.sessionMu.Unlock()
	if strings.TrimSpace(r.sessionRef) == "" {
		r.sessionRef = generated
	}
	return strings.TrimSpace(r.sessionRef), false
}

func (r *Service) captureSessionRef(ev harness.Event) {
	if r == nil {
		return
	}
	sessionRef := strings.TrimSpace(ev.SessionRef)
	if sessionRef == "" {
		return
	}
	shouldPersist := false
	r.sessionMu.Lock()
	if strings.TrimSpace(r.sessionRef) == sessionRef {
		r.sessionMu.Unlock()
		return
	}
	r.sessionRef = sessionRef
	shouldPersist = r.persistSessionRef != nil
	r.sessionMu.Unlock()
	if !shouldPersist {
		return
	}
	if err := r.persistSessionRef(sessionRef); err != nil {
		r.sessionMu.Lock()
		r.sessionErr = fmt.Errorf("persist external harness session id: %w", err)
		r.sessionMu.Unlock()
	}
}

func (r *Service) enrichHarnessEvent(ctx context.Context, ev harness.Event) []harness.Event {
	if r == nil || len(r.enrichers) == 0 {
		return []harness.Event{ev}
	}
	current := []harness.Event{ev}
	for _, enricher := range r.enrichers {
		if enricher == nil {
			continue
		}
		next := make([]harness.Event, 0, len(current))
		for _, item := range current {
			next = append(next, enricher.Enrich(ctx, item)...)
		}
		current = next
	}
	if len(current) == 0 {
		return []harness.Event{ev}
	}
	return current
}

func (r *Service) resolveHarnessCommand() (string, error) {
	command := strings.TrimSpace(r.command)
	kind := harness.NormalizeRuntimeKind(r.runtime.Kind())
	if kind != "codex" || command != "" {
		return command, nil
	}

	if _, err := lookupCommandPath("codex"); err == nil {
		return "", nil
	}
	if _, err := lookupCommandPath("npx"); err == nil {
		return "npx", nil
	}
	return "", fmt.Errorf(
		`external harness codex runtime unavailable: neither "codex" nor "npx" is in PATH; install codex with "npm install -g @openai/codex"`,
	)
}

func emitExternalHarnessEvent(step int, ev harness.Event, text *strings.Builder, textSegment *int, textOpen *bool, emit func(Event)) error {
	switch ev.Type {
	case harness.EventTurnStarted:
		if emit != nil {
			streamID := strings.TrimSpace(ev.TurnID)
			emit(Event{
				Kind:          EventTurnStarted,
				Step:          step,
				HarnessTurnID: streamID,
				StreamID:      streamID,
			})
		}
		return nil
	case harness.EventText:
		chunk := ev.Text
		if chunk == "" {
			return nil
		}
		if strings.EqualFold(strings.TrimSpace(ev.Data["kind"]), "reasoning") {
			if emit != nil {
				copied := StreamChunk{
					Text:        chunk,
					IsReasoning: true,
				}
				emit(Event{
					Kind:  EventStreamChunk,
					Step:  step,
					Chunk: &copied,
				})
			}
			return nil
		}
		if text != nil {
			existing := text.String()
			if externalHarnessTextEquivalent(existing, chunk) {
				text.Reset()
				text.WriteString(chunk)
				return nil
			}
			text.WriteString(chunk)
		}
		if emit != nil {
			turnID := strings.TrimSpace(ev.TurnID)
			streamID := strings.TrimSpace(ev.TurnID)
			segmentID := ""
			if textSegment != nil {
				if *textSegment <= 0 {
					*textSegment = 1
				}
				segmentID = strconv.Itoa(*textSegment)
			}
			if textOpen != nil {
				*textOpen = true
			}
			emit(Event{
				Kind:          EventText,
				Step:          step,
				HarnessText:   chunk,
				HarnessTurnID: turnID,
				StreamID:      streamID,
				SegmentID:     segmentID,
			})
		}
		return nil
	case harness.EventToolCall:
		if textSegment != nil && textOpen != nil && *textOpen {
			*textSegment++
			*textOpen = false
		}
		var toolArgs json.RawMessage
		if len(ev.Data) > 0 {
			raw, err := json.Marshal(ev.Data)
			if err != nil {
				return fmt.Errorf("marshal tool call data: %w", err)
			}
			toolArgs = raw
		}
		if emit != nil {
			emit(Event{
				Kind:              EventToolCall,
				Step:              step,
				RuntimeToolName:   strings.TrimSpace(ev.ToolName),
				RuntimeToolCallID: strings.TrimSpace(ev.ToolCallID),
				RuntimeToolTurnID: strings.TrimSpace(ev.TurnID),
				RuntimeToolArgs:   toolArgs,
				RuntimeToolData:   toolArgs,
			})
		}
		return nil
	case harness.EventToolResult:
		status := strings.ToLower(strings.TrimSpace(ev.Data["status"]))
		if status == "" {
			status = "completed"
		}
		resultData := copyHarnessEventData(ev.Data)
		if strings.TrimSpace(ev.Text) != "" && strings.TrimSpace(resultData["result"]) == "" {
			resultData["result"] = strings.TrimSpace(ev.Text)
		}
		resultText := strings.TrimSpace(ev.Text)
		if status == "failed" || status == "error" {
			if resultText == "" {
				resultText = strings.TrimSpace(ev.Data["error"])
			}
		}
		if emit != nil {
			emit(Event{
				Kind:              EventToolResult,
				Step:              step,
				RuntimeToolName:   strings.TrimSpace(ev.ToolName),
				RuntimeToolCallID: strings.TrimSpace(ev.ToolCallID),
				RuntimeToolTurnID: strings.TrimSpace(ev.TurnID),
				RuntimeToolResult: resultText,
				RuntimeToolStatus: status,
				RuntimeToolSource: strings.TrimSpace(ev.Data["sourceType"]),
				RuntimeToolData:   marshalRuntimeToolData(resultData),
			})
		}
		return nil
	case harness.EventRetry:
		msg := strings.TrimSpace(ev.Text)
		if msg == "" {
			msg = strings.TrimSpace(ev.Error)
		}
		if msg == "" {
			msg = "External runtime retrying"
		}
		if emit != nil {
			emit(Event{
				Kind:                EventRetry,
				Step:                step,
				HarnessRetryMessage: msg,
				HarnessRetryReason:  strings.TrimSpace(ev.Data["reason"]),
				HarnessRetryAttempt: strings.TrimSpace(ev.Data["attempt"]),
				HarnessRetryMax:     strings.TrimSpace(ev.Data["max"]),
			})
		}
		return nil
	case harness.EventTurnFailed:
		msg := strings.TrimSpace(ev.Error)
		if msg == "" {
			msg = "external harness turn failed"
		}
		if isNonBlockingHarnessResumeWarning(msg) {
			return nil
		}
		if emit != nil {
			emit(Event{
				Kind:       EventTurnFailed,
				Step:       step,
				HarnessErr: msg,
			})
		}
		return TurnFailedError{Message: msg}
	case harness.EventTurnCompleted:
		if emit != nil {
			emit(Event{
				Kind:          EventTurnCompleted,
				Step:          step,
				HarnessTurnID: strings.TrimSpace(ev.TurnID),
				StreamID:      strings.TrimSpace(ev.TurnID),
			})
		}
		if emit != nil && ev.Usage != nil {
			emit(Event{
				Kind:  EventUsage,
				Step:  step,
				Usage: harnessUsageToUsage(*ev.Usage),
			})
		}
		return nil
	case harness.EventContextSize:
		if emit != nil {
			emit(Event{
				Kind:          EventContextSize,
				Step:          step,
				CurrentTokens: ev.CurrentTokens,
				BudgetTokens:  ev.BudgetTokens,
			})
		}
		return nil
	case harness.EventCompaction:
		if emit != nil {
			emit(Event{
				Kind:         EventCompaction,
				Step:         step,
				BeforeTokens: ev.BeforeTokens,
				AfterTokens:  ev.AfterTokens,
				ServerSide:   ev.ServerSide,
			})
		}
		return nil
	default:
		return nil
	}
}

func harnessUsageToUsage(usage harness.Usage) Usage {
	return Usage{
		InputTokens:          usage.InputTokens,
		OutputTokens:         usage.OutputTokens,
		TotalTokens:          usage.TotalTokens,
		ReasoningTokens:      usage.ReasoningTokens,
		CacheReadInputTokens: usage.CacheReadInputTokens,
	}
}

func externalHarnessTextEquivalent(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	return stripExternalHarnessWhitespace(a) == stripExternalHarnessWhitespace(b)
}

func stripExternalHarnessWhitespace(s string) string {
	return strings.Join(strings.Fields(s), "")
}

func isNonBlockingHarnessResumeWarning(msg string) bool {
	normalized := strings.ToLower(strings.TrimSpace(msg))
	return strings.Contains(normalized, "session was recorded with model") &&
		strings.Contains(normalized, "but is resuming with")
}

func scopeHarnessEventToolCallID(ev harness.Event, scope string) harness.Event {
	if strings.TrimSpace(scope) == "" {
		return ev
	}
	callID := strings.TrimSpace(ev.ToolCallID)
	if callID == "" {
		return ev
	}
	ev.ToolCallID = scope + ":" + callID
	return ev
}

func hydrateHarnessEventTurnID(ev harness.Event, activeTurnID *string, localTurnID string) harness.Event {
	if activeTurnID == nil {
		return ev
	}
	turnID := strings.TrimSpace(ev.TurnID)
	if turnID == "" && harnessEventNeedsTurnID(ev.Type) {
		turnID = strings.TrimSpace(*activeTurnID)
		if turnID == "" {
			turnID = strings.TrimSpace(localTurnID)
		}
		ev.TurnID = turnID
	}
	if turnID != "" && harnessEventAdvancesTurn(ev.Type) {
		*activeTurnID = turnID
	}
	return ev
}

func harnessEventNeedsTurnID(eventType harness.EventType) bool {
	switch eventType {
	case harness.EventTurnStarted, harness.EventText, harness.EventToolCall, harness.EventToolResult, harness.EventTurnCompleted, harness.EventTurnFailed:
		return true
	default:
		return false
	}
}

func harnessEventAdvancesTurn(eventType harness.EventType) bool {
	switch eventType {
	case harness.EventTurnStarted, harness.EventText, harness.EventToolCall, harness.EventToolResult:
		return true
	default:
		return false
	}
}

func copyHarnessEventData(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func hydrateHarnessEventToolName(ev harness.Event, namesByCallID map[string]string) harness.Event {
	if strings.TrimSpace(ev.ToolName) != "" {
		return ev
	}
	callID := strings.TrimSpace(ev.ToolCallID)
	if callID == "" {
		return ev
	}
	if namesByCallID == nil {
		return ev
	}
	if known := strings.TrimSpace(namesByCallID[callID]); known != "" {
		ev.ToolName = known
	}
	return ev
}

func rememberHarnessEventToolName(namesByCallID map[string]string, ev harness.Event) {
	if namesByCallID == nil {
		return
	}
	callID := strings.TrimSpace(ev.ToolCallID)
	if callID == "" {
		return
	}
	name := strings.TrimSpace(ev.ToolName)
	if name == "" {
		return
	}
	namesByCallID[callID] = name
}

func marshalRuntimeToolData(data map[string]string) json.RawMessage {
	if len(data) == 0 {
		return nil
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return nil
	}
	return encoded
}

func harnessTurnInstruction(input TurnInput) (string, error) {
	text := strings.TrimSpace(input.Instruction)
	if text == "" && len(input.Attachments) == 0 {
		return "", fmt.Errorf("external harness instruction or attachment is required")
	}
	return text, nil
}
