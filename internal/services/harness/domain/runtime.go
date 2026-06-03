package domain

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const RuntimeKindInternal = "internal"

var ErrTurnCanceled = errors.New("harness turn canceled")

type EventType string

const (
	EventText          EventType = "text"
	EventToolCall      EventType = "tool_call"
	EventToolResult    EventType = "tool_result"
	EventRetry         EventType = "retry"
	EventTurnStarted   EventType = "turn_started"
	EventTurnCompleted EventType = "turn_completed"
	EventTurnFailed    EventType = "turn_failed"
	EventContextSize   EventType = "context_size"
	EventCompaction    EventType = "compaction"
)

type Usage struct {
	InputTokens          int
	OutputTokens         int
	TotalTokens          int
	ReasoningTokens      int
	CacheReadInputTokens int
}

type Event struct {
	Type       EventType
	TurnID     string
	SessionRef string
	Text       string
	Error      string
	ToolCallID string
	ToolName   string
	Data       map[string]string
	Usage      *Usage

	CurrentTokens int
	BudgetTokens  int
	BeforeTokens  int
	AfterTokens   int
	ServerSide    bool
}

type PromptInput struct {
	TurnID          string
	Text            string
	Attachments     []PromptAttachment
	ReasoningEffort string
}

type PromptAttachment struct {
	ID        string
	Name      string
	MediaType string
	SizeBytes int64
	URI       string
}

type ToolResultInput struct {
	TurnID     string
	ToolCallID string
	Result     string
	IsError    bool
}

type ApprovalRequest struct {
	ApprovalID string
	ToolCallID string
	ToolName   string
	Command    string
	Path       string
	Summary    string
	Method     string
	Data       map[string]string
}

type ApprovalDecision struct {
	Decision string
	Note     string
}

type ApprovalHandler func(ctx context.Context, req ApprovalRequest) (ApprovalDecision, error)

type StartParams struct {
	Command                string
	Workdir                string
	Env                    []string
	SystemPrompt           string
	Model                  string
	SessionRef             string
	Continue               bool
	MCPServers             []string
	ExtraArgs              []string
	ReasoningEffort        string
	PermissionMode         string
	ConfigRef              string
	PersistSessionRef      func(sessionRef string) error
	CommandRunner          CommandRunner
	ApprovalHandler        ApprovalHandler
	AppServerURL           string
	RuntimeHostDiagnostics string
}

type StartSpec struct {
	Command string
	Args    []string
	Workdir string
	Env     []string
}

type CommandRunner interface {
	StartCommand(ctx context.Context, spec StartSpec) (CommandProcess, error)
}

type CommandProcess interface {
	StdinPipe() (io.WriteCloser, error)
	StdoutPipe() (io.ReadCloser, error)
	StderrText() string
	Start() error
	Wait() error
	Kill() error
}

type SessionTurnInput struct {
	TurnID      string
	Text        string
	Attachments []PromptAttachment
	Steering    <-chan PromptInput
}

type SessionTurnResult struct{}

type Runtime interface {
	Kind() string
	Start(params StartParams) (StartSpec, error)
	ParseEvents(stream []byte) ([]Event, error)
	WritePrompt(w io.Writer, input PromptInput) error
	WriteToolResult(w io.Writer, input ToolResultInput) error
}

type StreamingInputRuntime interface {
	Runtime
	StartStreamingInput(params StartParams) (StartSpec, error)
	WriteStreamingPrompt(w io.Writer, input PromptInput) error
}

type SessionRuntime interface {
	Runtime
	ExecuteSessionTurn(ctx context.Context, params StartParams, input SessionTurnInput, emit func(Event)) (SessionTurnResult, error)
}

type SessionSteeringRuntime interface {
	SupportsSessionSteering() bool
}

type SessionSyncRuntime interface {
	Runtime
	SyncSession(ctx context.Context, params StartParams, emit func(Event)) error
}

type SessionRuntimeInvalidator interface {
	InvalidateSessionRef(sessionRef string) error
}

func NormalizeRuntimeKind(kind string) string {
	return strings.ToLower(strings.TrimSpace(kind))
}

func ParseNDJSON(stream []byte, parseLine func(line []byte, lineNo int) (Event, error)) ([]Event, error) {
	reader := bufio.NewReader(bytes.NewReader(stream))
	lineNo := 0
	var out []Event
	for {
		rawLine, err := reader.ReadBytes('\n')
		if errors.Is(err, io.EOF) && len(rawLine) == 0 {
			break
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("read event stream: %w", err)
		}
		lineNo++
		line := bytes.TrimSpace(rawLine)
		if len(line) > 0 {
			ev, parseErr := parseLine(line, lineNo)
			if parseErr != nil {
				return nil, parseErr
			}
			if ev.Type != "" {
				out = append(out, ev)
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
	}
	return out, nil
}

func WriteJSONLine(w io.Writer, payload any) error {
	if w == nil {
		return fmt.Errorf("writer is nil")
	}
	enc := json.NewEncoder(w)
	if err := enc.Encode(payload); err != nil {
		return fmt.Errorf("encode event payload: %w", err)
	}
	return nil
}
