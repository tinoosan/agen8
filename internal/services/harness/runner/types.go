package runner

import (
	"context"
	"encoding/json"

	harness "github.com/tinoosan/agen8-mcp-server/internal/services/harness/domain"
)

type Status string

const StatusSucceeded Status = "succeeded"

type PromptSource interface {
	SystemPrompt(ctx context.Context, basePrompt string, step int) (string, error)
}

type PromptSourceFunc func(ctx context.Context, basePrompt string, step int) (string, error)

func (f PromptSourceFunc) SystemPrompt(ctx context.Context, basePrompt string, step int) (string, error) {
	return f(ctx, basePrompt, step)
}

type PromptPartitioner interface {
	PromptSource
	StableSystemPrompt(ctx context.Context, basePrompt string, step int) (string, error)
	DynamicContext(ctx context.Context, step int) (string, error)
}

type TurnConfig struct {
	Model            string
	ReasoningEffort  string
	ReasoningSummary string
	SystemPrompt     string
	PromptSource     PromptSource
	MaxTokens        int
	SteeringCh       <-chan TurnInput
}

type TurnInput struct {
	Instruction string
	Attachments []harness.PromptAttachment
}

type Result struct {
	Text      string
	Artifacts []string
	Status    Status
	Error     string
}

type EventKind int

const (
	EventStreamChunk EventKind = iota
	EventToolCall
	EventToolResult
	EventTurnStarted
	EventText
	EventRetry
	EventTurnFailed
	EventTurnCompleted
	EventCompaction
	EventContextSize
	EventUsage
	EventDone
	EventError
)

type Event struct {
	Kind EventKind
	Step int

	Chunk *StreamChunk

	RuntimeToolName     string
	RuntimeToolCallID   string
	RuntimeToolTurnID   string
	RuntimeToolArgs     json.RawMessage
	RuntimeToolResult   string
	RuntimeToolStatus   string
	RuntimeToolSource   string
	RuntimeToolData     json.RawMessage
	HarnessText         string
	HarnessTurnID       string
	StreamID            string
	SegmentID           string
	HarnessErr          string
	HarnessRetryMessage string
	HarnessRetryReason  string
	HarnessRetryAttempt string
	HarnessRetryMax     string

	Model            string
	EffectiveModel   string
	ReasoningSummary string

	BeforeTokens int
	AfterTokens  int
	ServerSide   bool

	CurrentTokens int
	BudgetTokens  int

	Usage Usage

	Result Result

	Err error
}

type Executor interface {
	Execute(ctx context.Context, cfg TurnConfig, input TurnInput) <-chan Event
}

type StreamChunk struct {
	Text        string
	IsReasoning bool
}

type Usage struct {
	InputTokens          int
	OutputTokens         int
	TotalTokens          int
	ReasoningTokens      int
	CacheReadInputTokens int
}
