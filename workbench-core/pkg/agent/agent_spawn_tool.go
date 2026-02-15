package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	llmtypes "github.com/tinoosan/workbench-core/pkg/llm/types"
	"github.com/tinoosan/workbench-core/pkg/types"
)

const (
	agentSpawnToolName      = "agent_spawn"
	defaultSpawnMaxDepth    = 3
	defaultSpawnRunDeadline = 2 * time.Minute
)

type AgentSpawnTool struct {
	ParentAgent   Agent
	MaxDepth      int
	CurrentDepth  int
	MaxTokens     int
	ModelOverride string
}

type spawnOpMetadata struct {
	Goal               string   `json:"goal"`
	Model              string   `json:"model,omitempty"`
	RequestedMaxTokens int      `json:"requestedMaxTokens,omitempty"`
	MaxTokens          int      `json:"maxTokens,omitempty"`
	BackgroundCount    int      `json:"backgroundCount,omitempty"`
	BackgroundPreview  []string `json:"backgroundPreview,omitempty"`
	CurrentDepth       int      `json:"currentDepth"`
	MaxDepth           int      `json:"maxDepth"`
}

func (t *AgentSpawnTool) Definition() llmtypes.Tool {
	return llmtypes.Tool{
		Type: "function",
		Function: llmtypes.ToolFunction{
			Name:        agentSpawnToolName,
			Description: "Spawn a recursive child agent for a self-contained sub-task and return the child result.",
			Strict:      true,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"goal": map[string]any{
						"type":        "string",
						"description": "Sub-task objective for the child agent.",
					},
					"background_context": map[string]any{
						"type":        []string{"array", "null"},
						"description": "Optional context snippets for the child (the child does not see parent history).",
						"items": map[string]any{
							"type": "string",
						},
					},
					"max_tokens": map[string]any{
						"type":        []string{"integer", "null"},
						"description": "Optional max output tokens for the child.",
					},
				},
				"required":             []any{"goal", "background_context", "max_tokens"},
				"additionalProperties": false,
			},
		},
	}
}

func (t *AgentSpawnTool) Execute(ctx context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	if t == nil {
		return types.HostOpRequest{}, fmt.Errorf("agent_spawn is not configured")
	}
	if t.ParentAgent == nil {
		return types.HostOpRequest{}, fmt.Errorf("agent_spawn parent agent is not configured")
	}

	maxDepth := t.MaxDepth
	if maxDepth <= 0 {
		maxDepth = defaultSpawnMaxDepth
	}
	if t.CurrentDepth >= maxDepth {
		return types.HostOpRequest{}, fmt.Errorf("max spawn depth %d exceeded", maxDepth)
	}

	var payload struct {
		Goal              string   `json:"goal"`
		BackgroundContext []string `json:"background_context"`
		MaxTokens         *int     `json:"max_tokens"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return types.HostOpRequest{}, err
	}

	goal := strings.TrimSpace(payload.Goal)
	if goal == "" {
		return types.HostOpRequest{}, fmt.Errorf("agent_spawn.goal is required")
	}
	cleanBackground := sanitizeSpawnBackground(payload.BackgroundContext)

	cfg := t.ParentAgent.Config()
	cfg.SystemPrompt = DefaultSubAgentSystemPrompt() // Child agents get dedicated sub-agent prompt
	if override := strings.TrimSpace(t.ModelOverride); override != "" {
		cfg.Model = override
	}

	childMaxTokens := t.MaxTokens
	if payload.MaxTokens != nil && *payload.MaxTokens > 0 {
		childMaxTokens = *payload.MaxTokens
	}
	if childMaxTokens > 0 {
		cfg.MaxTokens = childMaxTokens
	}
	meta := spawnOpMetadata{
		Goal:            goal,
		Model:           strings.TrimSpace(cfg.Model),
		MaxTokens:       childMaxTokens,
		BackgroundCount: len(cleanBackground),
		CurrentDepth:    t.CurrentDepth,
		MaxDepth:        maxDepth,
	}
	if payload.MaxTokens != nil && *payload.MaxTokens > 0 {
		meta.RequestedMaxTokens = *payload.MaxTokens
	}
	if len(cleanBackground) > 0 {
		previewCount := 2
		if len(cleanBackground) < previewCount {
			previewCount = len(cleanBackground)
		}
		meta.BackgroundPreview = append([]string{}, cleanBackground[:previewCount]...)
	}
	metaBytes, _ := json.Marshal(meta)

	child, err := t.ParentAgent.CloneWithConfig(cfg)
	if err != nil {
		return types.HostOpRequest{}, err
	}
	t.configureChildSpawnTool(child, maxDepth, childMaxTokens)

	childCtx := ctx
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		childCtx, cancel = context.WithTimeout(ctx, defaultSpawnRunDeadline)
		defer cancel()
	}

	result, _, _, err := child.RunConversation(childCtx, buildSpawnChildMessages(cleanBackground, goal))
	if err != nil {
		return types.HostOpRequest{
			Op:     types.HostOpNoop,
			Action: agentSpawnToolName,
			Input:  metaBytes,
			Text:   "agent_spawn error: " + err.Error(),
		}, nil
	}

	text := strings.TrimSpace(result.Text)
	if text == "" {
		text = strings.TrimSpace(result.Error)
	}
	if text == "" {
		text = "(agent_spawn completed with no text response)"
	}
	return types.HostOpRequest{
		Op:     types.HostOpNoop,
		Action: agentSpawnToolName,
		Input:  metaBytes,
		Text:   text,
	}, nil
}

func (t *AgentSpawnTool) configureChildSpawnTool(child Agent, maxDepth, childMaxTokens int) {
	if child == nil {
		return
	}
	reg, ok := child.GetToolRegistry().(*HostToolRegistry)
	if !ok || reg == nil {
		return
	}

	nextDepth := t.CurrentDepth + 1
	if nextDepth >= maxDepth {
		reg.Remove(agentSpawnToolName)
		return
	}

	nextMaxTokens := childMaxTokens
	if nextMaxTokens > 1 {
		nextMaxTokens = nextMaxTokens / 2
	}
	reg.Replace(agentSpawnToolName, &AgentSpawnTool{
		ParentAgent:   child,
		MaxDepth:      maxDepth,
		CurrentDepth:  nextDepth,
		MaxTokens:     nextMaxTokens,
		ModelOverride: t.ModelOverride,
	})
}

func buildSpawnChildMessages(background []string, goal string) []llmtypes.LLMMessage {
	goal = strings.TrimSpace(goal)
	clean := sanitizeSpawnBackground(background)
	if len(clean) == 0 {
		return []llmtypes.LLMMessage{{Role: "user", Content: goal}}
	}

	var b strings.Builder
	b.WriteString("Background context from parent agent:\n")
	for i, entry := range clean {
		b.WriteString(fmt.Sprintf("%d. %s\n", i+1, entry))
	}
	b.WriteString("\nSub-task goal:\n")
	b.WriteString(goal)
	return []llmtypes.LLMMessage{{Role: "user", Content: b.String()}}
}

func sanitizeSpawnBackground(background []string) []string {
	clean := make([]string, 0, len(background))
	for _, entry := range background {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		clean = append(clean, entry)
	}
	return clean
}
