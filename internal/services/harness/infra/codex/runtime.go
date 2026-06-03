package codex

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/tinoosan/agen8-mcp-server/internal/services/harness/domain"
)

const RuntimeKind = "codex"

type Runtime struct {
	mu       sync.Mutex
	sessions map[string]*appServerSession
}

var reconnectProgressPattern = regexp.MustCompile(`(?i)^Reconnecting\.\.\.\s+([0-9]+)\s*/\s*([0-9]+)(?:\s*\((.*)\))?$`)

func New() *Runtime {
	return &Runtime{}
}

func (*Runtime) Kind() string {
	return RuntimeKind
}

func (*Runtime) Start(params domain.StartParams) (domain.StartSpec, error) {
	return domain.StartSpec{}, fmt.Errorf("codex exec transport has been removed; use codex app-server session runtime")
}

func buildAppServerSpec(params domain.StartParams, listenURL string) (domain.StartSpec, error) {
	command := strings.TrimSpace(params.Command)
	if command == "" {
		command = "codex"
	}
	permissionOverrides, err := codexConfigOverrides(params)
	if err != nil {
		return domain.StartSpec{}, err
	}
	args := append([]string{"app-server", "--listen", listenURL}, permissionOverrides...)
	if isNPXCommand(command) {
		args = append([]string{"-y", "@openai/codex"}, args...)
	}
	for i, override := range codexMCPConfigOverrides(params.MCPServers) {
		override = strings.TrimSpace(override)
		if override == "" {
			continue
		}
		if !strings.Contains(override, "=") {
			return domain.StartSpec{}, fmt.Errorf(
				"codex start: mcp override %d must be a --config key=value expression (got %q)",
				i,
				override,
			)
		}
		args = append(args, "--config", override)
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

func codexConfigOverrides(params domain.StartParams) ([]string, error) {
	runtimeOverrides, err := codexRuntimeConfigOverrides(params)
	if err != nil {
		return nil, err
	}
	var permissionOverrides []string
	switch strings.ToLower(strings.TrimSpace(params.PermissionMode)) {
	case "", "codex/full-access":
		permissionOverrides = []string{
			"--config", `approval_policy="never"`,
			"--config", `sandbox_mode="danger-full-access"`,
		}
	case "codex/default":
		permissionOverrides = nil
	case "codex/auto-review":
		permissionOverrides = []string{
			"--config", `approvals_reviewer="auto_review"`,
			"--config", `sandbox_mode="workspace-write"`,
		}
	case "codex/custom-config":
		permissionOverrides, err = codexCustomConfigOverrides(params.ConfigRef)
		if err != nil {
			return nil, err
		}
	default:
		permissionOverrides = []string{
			"--config", `approval_policy="never"`,
			"--config", `sandbox_mode="danger-full-access"`,
		}
	}
	return append(runtimeOverrides, permissionOverrides...), nil
}

func codexRuntimeConfigOverrides(params domain.StartParams) ([]string, error) {
	out := []string{}
	if model := normalizeCodexCLIModel(params.Model); model != "" {
		value, err := codexConfigValue("model", model)
		if err != nil {
			return nil, err
		}
		out = append(out, "--config", value)
	}
	if effort := strings.TrimSpace(params.ReasoningEffort); effort != "" {
		value, err := codexConfigValue("model_reasoning_effort", effort)
		if err != nil {
			return nil, err
		}
		out = append(out, "--config", value)
	}
	return out, nil
}

func codexCustomConfigOverrides(configRef string) ([]string, error) {
	path := strings.TrimSpace(configRef)
	if path == "" {
		return nil, fmt.Errorf("codex custom config ref is required")
	}
	var values map[string]any
	if _, err := toml.DecodeFile(path, &values); err != nil {
		return nil, fmt.Errorf("parse codex custom config %q: %w", path, err)
	}
	approved := map[string]bool{
		"approval_policy":    true,
		"approvals_reviewer": true,
		"permission_profile": true,
		"sandbox_mode":       true,
	}
	out := []string{}
	for key, value := range values {
		if !approved[key] {
			return nil, fmt.Errorf("unsupported codex custom config key %q", key)
		}
		text, err := codexConfigValue(key, value)
		if err != nil {
			return nil, err
		}
		out = append(out, "--config", text)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("codex custom config %q contains no supported overrides", path)
	}
	return out, nil
}

func codexConfigValue(key string, value any) (string, error) {
	switch typed := value.(type) {
	case string:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return "", fmt.Errorf("encode codex custom config key %q: %w", key, err)
		}
		return key + "=" + string(encoded), nil
	case bool:
		return key + "=" + strconv.FormatBool(typed), nil
	default:
		return "", fmt.Errorf("unsupported codex custom config value for key %q", key)
	}
}

func codexDefaultConfigOverrides() []string {
	return []string{
		"--config", `approval_policy="never"`,
		"--config", `sandbox_mode="danger-full-access"`,
	}
}

func normalizeCodexCLIModel(raw string) string {
	model := strings.TrimSpace(raw)
	if model == "" {
		return ""
	}
	if provider, remainder, ok := strings.Cut(model, "/"); ok {
		if strings.EqualFold(strings.TrimSpace(provider), "openai") {
			return strings.TrimSpace(remainder)
		}
	}
	return model
}

func isNPXCommand(command string) bool {
	base := strings.ToLower(strings.TrimSpace(filepath.Base(command)))
	return base == "npx" || base == "npx.cmd" || base == "npx.exe"
}

func (*Runtime) ParseEvents(stream []byte) ([]domain.Event, error) {
	currentTurnID := ""
	return domain.ParseNDJSON(stream, func(line []byte, lineNo int) (domain.Event, error) {
		ev, err := parseLine(line, lineNo)
		if err != nil {
			return domain.Event{}, err
		}
		explicitTurnID := explicitLineTurnID(line)
		if explicitTurnID != "" {
			currentTurnID = explicitTurnID
		}
		if ev.Type == domain.EventTurnStarted {
			if ev.TurnID == "" {
				ev.TurnID = explicitTurnID
			}
			if ev.TurnID != "" {
				currentTurnID = ev.TurnID
			}
		}
		if eventCarriesTurnContext(ev.Type) && ev.TurnID == "" && currentTurnID != "" {
			ev.TurnID = currentTurnID
		}
		if ev.TurnID != "" && eventDataNeedsTurnID(ev.Type) {
			if ev.Data == nil {
				ev.Data = map[string]string{}
			}
			ev.Data["turnId"] = ev.TurnID
		}
		return ev, nil
	})
}

func parseLine(line []byte, lineNo int) (domain.Event, error) {
	var raw map[string]any
	if err := json.Unmarshal(line, &raw); err != nil {
		return domain.Event{}, fmt.Errorf("codex parse events: line %d: %w", lineNo, err)
	}
	eventType := stringField(raw, "type")
	switch eventType {
	case "thread.started", "thread_started":
		return domain.Event{
			Type:       domain.EventTurnStarted,
			SessionRef: firstString(raw, "thread_id", "threadId", "session_id", "sessionId"),
		}, nil
	case "turn.started":
		return domain.Event{
			Type:   domain.EventTurnStarted,
			TurnID: firstString(raw, "turn_id", "turnId"),
		}, nil
	case "turn.completed":
		usage, usageErr := parseUsage(mapField(raw, "usage"))
		if usageErr != nil {
			return domain.Event{}, fmt.Errorf("codex parse events: line %d: %w", lineNo, usageErr)
		}
		return domain.Event{
			Type:  domain.EventTurnCompleted,
			Usage: usage,
		}, nil
	case "turn.failed":
		if ev, ok := parseReconnectProgress(nestedString(raw, "error", "message")); ok {
			return ev, nil
		}
		return domain.Event{
			Type:  domain.EventTurnFailed,
			Error: nestedString(raw, "error", "message"),
		}, nil
	case "error":
		if ev, ok := parseReconnectProgress(firstString(raw, "message", "error")); ok {
			return ev, nil
		}
		return domain.Event{
			Type:  domain.EventTurnFailed,
			Error: firstString(raw, "message", "error"),
		}, nil
	case "item.started", "item.updated", "item.completed":
		return parseItemEvent(raw, lineNo)
	case "text", "text_delta", "assistant_text":
		// Legacy compatibility for older codex wrappers.
		return domain.Event{
			Type:   domain.EventText,
			TurnID: firstString(raw, "turn_id", "turnId"),
			Text:   firstString(raw, "text", "delta", "content"),
		}, nil
	case "tool_call":
		return domain.Event{
			Type:       domain.EventToolCall,
			TurnID:     firstString(raw, "turn_id", "turnId"),
			ToolCallID: firstString(raw, "tool_call_id", "call_id", "id"),
			ToolName:   firstString(raw, "tool_name", "name", "tool"),
		}, nil
	case "tool_result":
		return domain.Event{
			Type:       domain.EventToolResult,
			TurnID:     firstString(raw, "turn_id", "turnId"),
			ToolCallID: firstString(raw, "tool_call_id", "call_id", "id"),
			Text:       firstString(raw, "result", "content", "text"),
		}, nil
	case "turn_started":
		return domain.Event{
			Type:   domain.EventTurnStarted,
			TurnID: firstString(raw, "turn_id", "turnId"),
		}, nil
	case "turn_completed":
		return domain.Event{
			Type:   domain.EventTurnCompleted,
			TurnID: firstString(raw, "turn_id", "turnId"),
		}, nil
	case "turn_failed":
		if ev, ok := parseReconnectProgress(firstString(raw, "error", "message")); ok {
			return ev, nil
		}
		return domain.Event{
			Type:   domain.EventTurnFailed,
			TurnID: firstString(raw, "turn_id", "turnId"),
			Error:  firstString(raw, "error", "message"),
		}, nil
	default:
		return domain.Event{}, fmt.Errorf("codex parse events: line %d: unsupported type %q", lineNo, strings.TrimSpace(eventType))
	}
}

func parseReconnectProgress(message string) (domain.Event, bool) {
	message = strings.TrimSpace(message)
	if message == "" {
		return domain.Event{}, false
	}
	match := reconnectProgressPattern.FindStringSubmatch(message)
	if match == nil {
		return domain.Event{}, false
	}
	data := map[string]string{
		"attempt": strings.TrimSpace(match[1]),
		"max":     strings.TrimSpace(match[2]),
	}
	if len(match) > 3 {
		if reason := strings.TrimSpace(match[3]); reason != "" {
			data["reason"] = reason
		}
	}
	return domain.Event{
		Type: domain.EventRetry,
		Text: message,
		Data: data,
	}, true
}

func (*Runtime) WritePrompt(w io.Writer, input domain.PromptInput) error {
	if strings.TrimSpace(input.Text) == "" && len(input.Attachments) == 0 {
		return fmt.Errorf("codex write prompt: text or attachment is required")
	}
	if w == nil {
		return fmt.Errorf("codex write prompt: writer is nil")
	}
	if _, err := io.WriteString(w, promptTextWithAttachmentPaths(input.Text, input.Attachments)); err != nil {
		return fmt.Errorf("codex write prompt: %w", err)
	}
	return nil
}

func promptTextWithAttachmentPaths(text string, attachments []domain.PromptAttachment) string {
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

func (*Runtime) WriteToolResult(_ io.Writer, _ domain.ToolResultInput) error {
	return fmt.Errorf("codex write tool result: unsupported for codex app-server session runtime")
}

func firstString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		if s := stringField(raw, key); s != "" {
			return s
		}
	}
	return ""
}

func explicitLineTurnID(line []byte) string {
	var raw map[string]any
	if err := json.Unmarshal(line, &raw); err != nil {
		return ""
	}
	if turnID := firstString(raw, "turn_id", "turnId"); turnID != "" {
		return turnID
	}
	item := mapField(raw, "item")
	if turnID := firstString(item, "turn_id", "turnId"); turnID != "" {
		return turnID
	}
	return nestedString(raw, "turn", "id")
}

func eventCarriesTurnContext(eventType domain.EventType) bool {
	switch eventType {
	case domain.EventText, domain.EventToolCall, domain.EventToolResult, domain.EventTurnCompleted, domain.EventTurnFailed:
		return true
	default:
		return false
	}
}

func eventDataNeedsTurnID(eventType domain.EventType) bool {
	switch eventType {
	case domain.EventText, domain.EventToolCall, domain.EventToolResult:
		return true
	default:
		return false
	}
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

func rawStringField(raw map[string]any, key string) string {
	value, ok := raw[key]
	if !ok {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return fmt.Sprintf("%v", typed)
	}
}

func parseUsage(raw map[string]any) (*domain.Usage, error) {
	if len(raw) == 0 {
		return nil, nil
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
	cacheRead, err := intField(raw, "cached_input_tokens", "cachedInputTokens", "cache_read_input_tokens", "cacheReadInputTokens")
	if err != nil {
		return nil, err
	}
	usage := domain.Usage{
		InputTokens:          input,
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

func parseItemEvent(raw map[string]any, lineNo int) (domain.Event, error) {
	item := mapField(raw, "item")
	turnID := itemEventTurnID(raw, item)
	itemType := stringField(item, "type")
	switch itemType {
	case "agent_message", "agentMessage":
		if !strings.EqualFold(firstString(raw, "type"), "item.completed") {
			return domain.Event{}, nil
		}
		text := firstString(item, "text")
		if text == "" {
			return domain.Event{}, nil
		}
		return domain.Event{
			Type:       domain.EventText,
			TurnID:     turnID,
			ToolCallID: firstString(item, "id"),
			Text:       text,
			Data: map[string]string{
				"kind": "assistant",
			},
		}, nil
	case "reasoning":
		if !strings.EqualFold(firstString(raw, "type"), "item.completed") {
			return domain.Event{}, nil
		}
		text := reasoningSummaryText(item)
		if text == "" {
			return domain.Event{}, nil
		}
		return domain.Event{
			Type:       domain.EventText,
			TurnID:     turnID,
			ToolCallID: firstString(item, "id"),
			Text:       text,
			Data: reasoningEventData(
				firstString(item, "id", "itemId", "item_id"),
				"",
			),
		}, nil
	case "mcp_tool_call", "mcpToolCall":
		return parseMCPToolCall(raw, item, lineNo, turnID)
	case "command_execution", "commandExecution":
		return parseCommandExecution(raw, item, lineNo, turnID)
	case "file_change", "fileChange":
		return parseFileChange(raw, item, lineNo, turnID)
	case "web_search", "webSearch":
		return parseWebSearchItem(raw, item, turnID), nil
	case "image_generation_call", "imageGenerationCall", "image_generation", "imageGeneration":
		return parseImageGenerationItem(raw, item, turnID), nil
	case "error":
		msg := firstString(item, "message")
		if msg == "" {
			msg = firstString(raw, "message")
		}
		if msg == "" {
			msg = "codex item.error"
		}
		return domain.Event{
			Type:   domain.EventTurnFailed,
			TurnID: turnID,
			Error:  msg,
		}, nil
	case "":
		return domain.Event{}, fmt.Errorf("codex parse events: line %d: missing item.type for %q", lineNo, firstString(raw, "type"))
	default:
		if ev, ok := parseGenericToolItem(raw, item, turnID); ok {
			return ev, nil
		}
		// Ignore unknown non-tool payloads to remain forward-compatible with new codex item kinds.
		return domain.Event{}, nil
	}
}

func parseImageGenerationItem(raw map[string]any, item map[string]any, turnID string) domain.Event {
	status := resolvedItemStatus(raw, item)
	toolCallID := firstString(item, "id", "itemId", "item_id", "call_id", "callId")
	prompt := firstNonEmptyString(
		firstString(item, "prompt"),
		nestedString(item, "input", "prompt"),
		nestedString(item, "arguments", "prompt"),
	)
	if toolCallID == "" {
		toolCallID = syntheticToolCallID("image_generation", prompt+"|"+inputHint(item))
	}
	input := map[string]any{}
	for _, key := range []string{"prompt", "action", "size", "quality", "format", "output_format", "background", "compression"} {
		if value, ok := item[key]; ok {
			input[key] = value
		}
	}
	if args := mapField(item, "arguments"); len(args) != 0 {
		for key, value := range args {
			if _, exists := input[key]; !exists {
				input[key] = value
			}
		}
	}
	if prompt != "" {
		input["prompt"] = prompt
	}

	imageB64 := firstImageGenerationBase64(item)
	imageURL := firstNonEmptyString(
		firstString(item, "image_url", "imageUrl", "url"),
		nestedString(item, "image", "url"),
		nestedString(item, "output", "url"),
	)
	revisedPrompt := firstString(item, "revised_prompt", "revisedPrompt")
	refusalReason := firstString(item, "refusal_reason", "refusalReason")
	if refusalReason != "" {
		status = "failed"
	}
	if status != "failed" && (imageB64 != "" || imageURL != "") {
		status = "completed"
	}
	outputFormat := firstString(item, "output_format", "outputFormat", "format")
	mimeType := firstNonEmptyString(firstString(item, "mime_type", "mimeType", "content_type", "contentType"), imageGenerationMimeType(outputFormat))
	data := map[string]string{
		"status":        status,
		"op":            "image_generation",
		"input":         compactJSON(input),
		"codexItemType": firstString(item, "type"),
	}
	if turnID != "" {
		data["turnId"] = turnID
	}
	if prompt != "" {
		data["prompt"] = prompt
	}
	if revisedPrompt != "" {
		data["revisedPrompt"] = revisedPrompt
	}
	if imageB64 != "" {
		data["imageB64"] = imageB64
	}
	if imageURL != "" {
		data["imageUrl"] = imageURL
	}
	if outputFormat != "" {
		data["outputFormat"] = outputFormat
	}
	if mimeType != "" {
		data["mimeType"] = mimeType
	}
	if size := firstString(item, "size"); size != "" {
		data["size"] = size
	}
	if quality := firstString(item, "quality"); quality != "" {
		data["quality"] = quality
	}
	if background := firstString(item, "background"); background != "" {
		data["background"] = background
	}
	if refusalReason != "" {
		data["error"] = refusalReason
		data["refusalReason"] = refusalReason
	}
	if status == "completed" || status == "failed" {
		return domain.Event{
			Type:       domain.EventToolResult,
			TurnID:     turnID,
			ToolCallID: toolCallID,
			ToolName:   "image_generation",
			Text:       imageGenerationSummary(data),
			Data:       data,
		}
	}
	return domain.Event{
		Type:       domain.EventToolCall,
		TurnID:     turnID,
		ToolCallID: toolCallID,
		ToolName:   "image_generation",
		Data:       data,
	}
}

func firstImageGenerationBase64(item map[string]any) string {
	for _, value := range []string{
		firstString(item, "result", "b64_json", "b64Json", "image_b64", "imageB64"),
		nestedStringAny(item, "image", "b64_json", "b64Json", "result", "image_b64", "imageB64"),
		nestedStringAny(item, "output", "b64_json", "b64Json", "result", "image_b64", "imageB64"),
	} {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	for _, key := range []string{"images", "data", "output"} {
		items, ok := item[key].([]any)
		if !ok {
			continue
		}
		for _, candidate := range items {
			record, ok := candidate.(map[string]any)
			if !ok {
				continue
			}
			if value := firstImageGenerationBase64(record); value != "" {
				return value
			}
		}
	}
	return ""
}

func imageGenerationMimeType(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "jpg", "jpeg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	case "png":
		return "image/png"
	default:
		return ""
	}
}

func imageGenerationSummary(data map[string]string) string {
	if data["refusalReason"] != "" {
		return data["refusalReason"]
	}
	if data["imageUrl"] != "" {
		return data["imageUrl"]
	}
	if data["imageB64"] != "" {
		return "image generated"
	}
	return ""
}

func parseWebSearchItem(raw map[string]any, item map[string]any, turnID string) domain.Event {
	status := resolvedItemStatus(raw, item)
	toolCallID := firstString(item, "id", "itemId", "item_id", "call_id", "callId")
	query := firstString(item, "query")
	action := mapField(item, "action")
	if query == "" {
		query = firstString(action, "query", "url")
	}
	if toolCallID == "" {
		toolCallID = syntheticToolCallID("web_search", query+"|"+encodeToolPayload(action))
	}
	input := map[string]any{}
	if query != "" {
		input["query"] = query
	}
	if len(action) != 0 {
		input["action"] = action
		if actionType := firstString(action, "type"); actionType != "" {
			input["type"] = actionType
		}
		if queries, ok := action["queries"]; ok {
			input["queries"] = queries
		}
		if url := firstString(action, "url"); url != "" {
			input["url"] = url
		}
	}
	resultText := extractWebSearchResultPayload(item)
	data := map[string]string{
		"status":        status,
		"op":            "web_search",
		"input":         compactJSON(input),
		"codexItemType": firstString(item, "type"),
	}
	if turnID != "" {
		data["turnId"] = turnID
	}
	if query != "" {
		data["query"] = query
	}
	if resultText != "" {
		data["result"] = resultText
		data["outputPreview"] = resultText
	}
	if status == "completed" || status == "failed" {
		return domain.Event{
			Type:       domain.EventToolResult,
			TurnID:     turnID,
			ToolCallID: toolCallID,
			ToolName:   "web_search",
			Text:       resultText,
			Data:       data,
		}
	}
	return domain.Event{
		Type:       domain.EventToolCall,
		TurnID:     turnID,
		ToolCallID: toolCallID,
		ToolName:   "web_search",
		Data:       data,
	}
}

func parseMCPToolCall(raw map[string]any, item map[string]any, lineNo int, turnID string) (domain.Event, error) {
	status := resolvedItemStatus(raw, item)
	toolCallID := firstString(item, "id")
	server := firstString(item, "server")
	tool := firstString(item, "tool")
	if tool == "" {
		return domain.Event{}, fmt.Errorf("codex parse events: line %d: mcp_tool_call item.tool is required", lineNo)
	}
	toolName := strings.TrimSpace(tool)
	if server != "" {
		toolName = strings.TrimSpace(server + "/" + tool)
	}
	if toolName == "/" || toolName == "" {
		toolName = ""
	}
	if toolCallID == "" {
		toolCallID = syntheticToolCallID("mcp_tool_call", toolName+"|"+status+"|"+inputHint(item))
	}
	op := normalizeToolOp(tool)
	inputPayload := extractMCPInput(item)
	action := extractMCPAction(inputPayload)
	commonData := map[string]string{
		"status": status,
		"op":     op,
		"input":  inputPayload,
		"action": action,
	}
	if turnID != "" {
		commonData["turnId"] = turnID
	}
	if errText := deniedOrErrorText(item); errText != "" {
		status = "failed"
		commonData["status"] = status
		commonData["error"] = errText
	}
	switch status {
	case "completed", "failed":
		resultText := nestedStringAny(item, "result", "structured_content", "structuredContent")
		if resultText == "" {
			resultText = nestedStringAny(item, "result", "content")
		}
		if resultText == "" {
			resultText = nestedString(item, "error", "message")
		}
		if resultText == "" {
			resultText = commonData["error"]
		}
		return domain.Event{
			Type:       domain.EventToolResult,
			TurnID:     turnID,
			ToolCallID: toolCallID,
			ToolName:   toolName,
			Text:       resultText,
			Data:       withResultData(commonData, resultText),
		}, nil
	default:
		return domain.Event{
			Type:       domain.EventToolCall,
			TurnID:     turnID,
			ToolCallID: toolCallID,
			ToolName:   toolName,
			Data:       commonData,
		}, nil
	}
}

func parseCommandExecution(raw map[string]any, item map[string]any, lineNo int, turnID string) (domain.Event, error) {
	command := firstString(item, "command")
	if command == "" {
		return domain.Event{}, fmt.Errorf("codex parse events: line %d: command_execution item.command is required", lineNo)
	}
	status := resolvedItemStatus(raw, item)
	toolCallID := firstString(item, "id")
	if toolCallID == "" {
		toolCallID = syntheticToolCallID("command_execution", command)
	}
	data := map[string]string{
		"status":      status,
		"op":          "bash",
		"action":      "run",
		"command":     strings.TrimSpace(command),
		"argvPreview": strings.TrimSpace(command),
		"input":       compactJSON(map[string]string{"command": command}),
	}
	if turnID != "" {
		data["turnId"] = turnID
	}
	if exitCode := firstString(item, "exit_code", "exitCode"); exitCode != "" {
		data["exitCode"] = exitCode
	}
	if pid := firstString(item, "pid", "process_id", "processId"); pid != "" {
		data["pid"] = pid
	}
	if isBackgroundExecution(item, command) {
		data["background"] = "true"
	}
	outputText := extractCommandOutput(item)
	if outputText != "" {
		trimmed := strings.TrimSpace(outputText)
		data["result"] = trimmed
		data["outputFull"] = trimmed
		data["outputPreview"] = trimmed
	}
	if stderr := strings.TrimSpace(firstString(item, "stderr", "error_output", "errorOutput")); stderr != "" {
		data["stderr"] = stderr
		if strings.TrimSpace(data["result"]) == "" {
			data["result"] = stderr
			data["outputPreview"] = stderr
		}
	}
	if errText := deniedOrErrorText(item); errText != "" {
		status = "failed"
		data["status"] = status
		data["error"] = errText
		if strings.TrimSpace(data["result"]) == "" {
			data["result"] = errText
			data["outputPreview"] = errText
		}
	}

	switch status {
	case "completed", "failed":
		return domain.Event{
			Type:       domain.EventToolResult,
			TurnID:     turnID,
			ToolCallID: toolCallID,
			ToolName:   "bash",
			Text:       strings.TrimSpace(outputText),
			Data:       data,
		}, nil
	case "in_progress", "running":
		if strings.TrimSpace(outputText) != "" || strings.TrimSpace(data["stderr"]) != "" {
			return domain.Event{
				Type:       domain.EventToolResult,
				TurnID:     turnID,
				ToolCallID: toolCallID,
				ToolName:   "bash",
				Text:       strings.TrimSpace(outputText),
				Data:       data,
			}, nil
		}
		fallthrough
	default:
		return domain.Event{
			Type:       domain.EventToolCall,
			TurnID:     turnID,
			ToolCallID: toolCallID,
			ToolName:   "bash",
			Data:       data,
		}, nil
	}
}

type fileChangeEntry struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
	Diff string `json:"diff,omitempty"`
}

func parseFileChange(raw map[string]any, item map[string]any, lineNo int, turnID string) (domain.Event, error) {
	status := resolvedItemStatus(raw, item)
	toolCallID := firstString(item, "id")
	if toolCallID == "" {
		toolCallID = syntheticToolCallID("file_change", status+"|"+inputHint(item))
	}
	changes, err := parseFileChanges(item["changes"])
	if err != nil {
		return domain.Event{}, fmt.Errorf("codex parse events: line %d: %w", lineNo, err)
	}
	if len(changes) == 0 {
		return domain.Event{}, fmt.Errorf("codex parse events: line %d: file_change item.changes is required", lineNo)
	}
	encoded, err := json.Marshal(changes)
	if err != nil {
		return domain.Event{}, fmt.Errorf("codex parse events: line %d: encode file_change item.changes: %w", lineNo, err)
	}
	data := map[string]string{
		"status":  status,
		"op":      "file_change",
		"action":  "edit",
		"changes": strings.TrimSpace(string(encoded)),
	}
	if turnID != "" {
		data["turnId"] = turnID
	}

	var firstPath string
	for _, change := range changes {
		if strings.TrimSpace(change.Path) != "" {
			firstPath = strings.TrimSpace(change.Path)
			break
		}
	}
	if firstPath != "" {
		data["path"] = firstPath
	}
	if len(changes) == 1 {
		op, mode := fileChangeKindToOp(changes[0].Kind)
		data["op"] = op
		data["writeMode"] = mode
		if strings.TrimSpace(changes[0].Diff) != "" {
			data["patchPreview"] = strings.TrimSpace(changes[0].Diff)
			data["result"] = strings.TrimSpace(changes[0].Diff)
		}
	}
	data["input"] = compactJSON(map[string]any{
		"changes": changes,
	})
	if errText := deniedOrErrorText(item); errText != "" {
		status = "failed"
		data["status"] = status
		data["error"] = errText
		if strings.TrimSpace(data["result"]) == "" {
			data["result"] = errText
		}
	}

	switch status {
	case "completed", "failed":
		return domain.Event{
			Type:       domain.EventToolResult,
			TurnID:     turnID,
			ToolCallID: toolCallID,
			ToolName:   "file_change",
			Text:       strings.TrimSpace(data["result"]),
			Data:       data,
		}, nil
	default:
		return domain.Event{
			Type:       domain.EventToolCall,
			TurnID:     turnID,
			ToolCallID: toolCallID,
			ToolName:   "file_change",
			Data:       data,
		}, nil
	}
}

func parseGenericToolItem(raw map[string]any, item map[string]any, turnID string) (domain.Event, bool) {
	itemType := strings.TrimSpace(firstString(item, "type"))
	toolName := firstString(item, "tool", "tool_name", "toolName", "name")
	if toolName == "" {
		toolName = nestedString(item, "function", "name")
	}
	if toolName == "" {
		toolName = firstString(item, "server")
	}
	if !genericItemLooksLikeTool(itemType, toolName, item) {
		return domain.Event{}, false
	}

	status := resolvedItemStatus(raw, item)
	toolCallID := firstString(item, "id", "itemId", "item_id", "call_id", "callId", "tool_call_id", "toolCallId")
	if toolCallID == "" {
		toolCallID = syntheticToolCallID("generic_tool", itemType+"|"+toolName+"|"+inputHint(item))
	}
	if toolName == "" {
		toolName = strings.TrimSpace(itemType)
	}
	server := firstString(item, "server")
	if server != "" && toolName != "" && !strings.Contains(toolName, "/") {
		toolName = strings.TrimSpace(server + "/" + toolName)
	}

	inputPayload := extractMCPInput(item)
	data := map[string]string{
		"status": status,
		"op":     normalizeToolOp(toolName),
		"input":  inputPayload,
	}
	if turnID != "" {
		data["turnId"] = turnID
	}
	if itemType != "" {
		data["codexItemType"] = itemType
	}
	if action := extractMCPAction(inputPayload); action != "" {
		data["action"] = action
	}
	if server != "" {
		data["server"] = server
	}
	if errText := deniedOrErrorText(item); errText != "" {
		status = "failed"
		data["status"] = status
		data["error"] = errText
	}

	resultText := extractGenericToolResult(item)
	if resultText != "" {
		data["result"] = resultText
		data["outputPreview"] = resultText
	}

	switch status {
	case "completed", "failed":
		return domain.Event{
			Type:       domain.EventToolResult,
			TurnID:     turnID,
			ToolCallID: toolCallID,
			ToolName:   toolName,
			Text:       resultText,
			Data:       data,
		}, true
	default:
		return domain.Event{
			Type:       domain.EventToolCall,
			TurnID:     turnID,
			ToolCallID: toolCallID,
			ToolName:   toolName,
			Data:       data,
		}, true
	}
}

func genericItemLooksLikeTool(itemType string, toolName string, item map[string]any) bool {
	normalizedType := strings.ToLower(strings.TrimSpace(itemType))
	normalizedType = strings.ReplaceAll(normalizedType, "-", "_")
	if strings.Contains(normalizedType, "tool") || strings.Contains(normalizedType, "function") || strings.Contains(normalizedType, "call") {
		return true
	}
	if strings.TrimSpace(toolName) != "" {
		return true
	}
	for _, key := range []string{"arguments", "input", "args", "result", "output", "structured_content", "structuredContent"} {
		if _, ok := item[key]; ok {
			return true
		}
	}
	return false
}

func extractGenericToolResult(item map[string]any) string {
	for _, value := range []any{
		item["result"],
		item["output"],
		item["response"],
		item["content"],
		item["text"],
		item["structured_content"],
		item["structuredContent"],
	} {
		if encoded := encodeToolPayload(value); encoded != "" {
			return encoded
		}
	}
	if nested := nestedStringAny(item, "result", "structured_content", "structuredContent"); nested != "" {
		return nested
	}
	return ""
}

func extractWebSearchResultPayload(item map[string]any) string {
	for _, key := range []string{
		"result",
		"results",
		"output",
		"response",
		"content",
		"text",
		"links",
		"sources",
		"citations",
		"references",
		"items",
		"structured_content",
		"structuredContent",
	} {
		if encoded := encodeToolPayload(item[key]); encoded != "" {
			return encoded
		}
	}
	if nested := nestedStringAny(item, "result", "structured_content", "structuredContent"); nested != "" {
		return nested
	}
	return ""
}

func encodeToolPayload(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return strings.TrimSpace(fmt.Sprintf("%v", typed))
		}
		return strings.TrimSpace(string(encoded))
	}
}

func itemEventTurnID(raw map[string]any, item map[string]any) string {
	if turnID := firstString(raw, "turn_id", "turnId"); turnID != "" {
		return turnID
	}
	if turnID := firstString(item, "turn_id", "turnId"); turnID != "" {
		return turnID
	}
	return nestedString(raw, "turn", "id")
}

func parseFileChanges(raw any) ([]fileChangeEntry, error) {
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("file_change item.changes must be an array")
	}
	out := make([]fileChangeEntry, 0, len(items))
	for _, item := range items {
		change, ok := item.(map[string]any)
		if !ok {
			continue
		}
		path := firstString(change, "path")
		kind := firstString(change, "kind")
		if strings.TrimSpace(path) == "" || strings.TrimSpace(kind) == "" {
			continue
		}
		out = append(out, fileChangeEntry{
			Path: strings.TrimSpace(path),
			Kind: strings.ToLower(strings.TrimSpace(kind)),
			Diff: strings.TrimSpace(firstString(change, "diff", "unified_diff", "patch")),
		})
	}
	return out, nil
}

func fileChangeKindToOp(kind string) (op, writeMode string) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "add", "create", "created":
		return "write_file", "created"
	case "delete", "removed":
		return "edit_file", "deleted"
	case "update", "edit", "modified":
		return "edit_file", "modified"
	default:
		return "edit_file", "modified"
	}
}

func withResultData(base map[string]string, resultText string) map[string]string {
	out := make(map[string]string, len(base)+1)
	for key, value := range base {
		if strings.TrimSpace(value) == "" {
			continue
		}
		out[key] = value
	}
	if strings.TrimSpace(resultText) != "" {
		out["result"] = strings.TrimSpace(resultText)
	}
	return out
}

func extractMCPInput(item map[string]any) string {
	for _, key := range []string{"arguments", "input", "args"} {
		value, ok := item[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case string:
			trimmed := strings.TrimSpace(typed)
			if trimmed != "" {
				return trimmed
			}
		default:
			encoded, err := json.Marshal(typed)
			if err != nil {
				continue
			}
			trimmed := strings.TrimSpace(string(encoded))
			if trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

func inputHint(item map[string]any) string {
	if input := extractMCPInput(item); strings.TrimSpace(input) != "" {
		return strings.TrimSpace(input)
	}
	return ""
}

func extractMCPAction(inputPayload string) string {
	trimmed := strings.TrimSpace(inputPayload)
	if trimmed == "" {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return ""
	}
	action, ok := payload["action"]
	if !ok {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", action)))
}

func normalizeToolOp(op string) string {
	normalized := strings.ToLower(strings.TrimSpace(op))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	normalized = strings.ReplaceAll(normalized, " ", "_")
	switch normalized {
	case "web.run", "web_run", "web":
		return "web_search"
	case "toolsearch", "tool_search", "tool_search_tool", "tool_search.tool":
		return "tool_search"
	case "functions.exec_command", "exec_command":
		return "bash"
	case "functions.apply_patch":
		return "edit_file"
	case "decision_log":
		return "decision"
	case "operator_action":
		return "operator"
	case "shell_exec", "command_execution", "shell", "shell_command", "shellcommand", "bash", "exec":
		return "bash"
	case "write", "writefile", "create_file", "write_file":
		return "write_file"
	case "edit", "editfile", "apply_patch", "str_replace_editor", "multiedit", "multi_edit", "edit_file":
		return "edit_file"
	case "read", "readfile", "read_file":
		return "read_file"
	case "glob", "grep", "search_files":
		return "search_files"
	case "ls", "list_files", "list_dir", "list_directory":
		return "list_files"
	default:
		return normalized
	}
}

func resolvedItemStatus(raw map[string]any, item map[string]any) string {
	status := strings.ToLower(strings.TrimSpace(firstString(item, "status")))
	status = strings.ReplaceAll(status, "-", "_")
	switch status {
	case "inprogress", "in_progress":
		return "in_progress"
	case "denied", "rejected", "cancelled", "canceled", "permission_denied", "approval_denied":
		return "failed"
	}
	if status != "" {
		return status
	}
	switch strings.ToLower(strings.TrimSpace(firstString(raw, "type"))) {
	case "item.started":
		return "in_progress"
	case "item.updated":
		return "in_progress"
	case "item.completed":
		return "completed"
	default:
		return status
	}
}

func deniedOrErrorText(item map[string]any) string {
	for _, candidate := range []string{
		nestedString(item, "error", "message"),
		firstString(item, "error", "message", "failure", "reason"),
		firstString(item, "stderr", "error_output", "errorOutput"),
	} {
		if isDenialText(candidate) {
			return strings.TrimSpace(candidate)
		}
	}
	status := strings.ToLower(strings.TrimSpace(firstString(item, "status")))
	status = strings.ReplaceAll(status, "-", "_")
	switch status {
	case "denied", "rejected", "cancelled", "canceled", "permission_denied", "approval_denied":
		if message := strings.TrimSpace(firstString(item, "message", "reason")); message != "" {
			return message
		}
		return "operation denied"
	default:
		return ""
	}
}

func isDenialText(value string) bool {
	text := strings.ToLower(strings.TrimSpace(value))
	if text == "" {
		return false
	}
	return strings.Contains(text, "permission denied") ||
		strings.Contains(text, "approval denied") ||
		strings.Contains(text, "was denied") ||
		strings.Contains(text, "denied by") ||
		strings.Contains(text, "request denied")
}

func extractCommandOutput(item map[string]any) string {
	if s := strings.TrimSpace(firstString(item, "aggregatedOutput", "aggregated_output", "stdout", "text")); s != "" {
		return s
	}
	output, ok := item["output"]
	if !ok || output == nil {
		return ""
	}
	switch typed := output.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		if s := strings.TrimSpace(firstString(typed, "stdout", "text", "content")); s != "" {
			return s
		}
		encoded, err := json.Marshal(typed)
		if err != nil {
			return strings.TrimSpace(fmt.Sprintf("%v", typed))
		}
		return strings.TrimSpace(string(encoded))
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", typed))
	}
}

func isBackgroundExecution(item map[string]any, command string) bool {
	for _, key := range []string{"background", "is_background", "isBackground", "detached", "async"} {
		if boolField(item, key) {
			return true
		}
	}
	return looksBackgroundCommand(command)
}

func boolField(raw map[string]any, key string) bool {
	value, ok := raw[key]
	if !ok {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		trimmed := strings.ToLower(strings.TrimSpace(typed))
		return trimmed == "true" || trimmed == "1" || trimmed == "yes"
	case float64:
		return typed != 0
	case int:
		return typed != 0
	default:
		return false
	}
}

func looksBackgroundCommand(command string) bool {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return false
	}
	if strings.HasSuffix(trimmed, " &") || (strings.HasSuffix(trimmed, "&") && !strings.HasSuffix(trimmed, "&&")) {
		return true
	}
	lower := strings.ToLower(trimmed)
	return strings.Contains(lower, " nohup ") || strings.Contains(lower, " disown")
}

func syntheticToolCallID(kind, material string) string {
	h := fnv.New64a()
	h.Write([]byte(kind))
	h.Write([]byte{':'})
	h.Write([]byte(material))
	return fmt.Sprintf("%s-%x", kind, h.Sum64())
}

func compactJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(encoded))
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

func nestedString(raw map[string]any, parentKey, childKey string) string {
	parent := mapField(raw, parentKey)
	if len(parent) == 0 {
		return ""
	}
	if value, ok := parent[childKey]; ok {
		switch typed := value.(type) {
		case string:
			return strings.TrimSpace(typed)
		default:
			encoded, err := json.Marshal(typed)
			if err != nil {
				return strings.TrimSpace(fmt.Sprintf("%v", typed))
			}
			return strings.TrimSpace(string(encoded))
		}
	}
	return ""
}

func nestedStringAny(raw map[string]any, parentKey string, childKeys ...string) string {
	parent := mapField(raw, parentKey)
	if len(parent) == 0 || len(childKeys) == 0 {
		return ""
	}
	for _, childKey := range childKeys {
		trimmed := strings.TrimSpace(childKey)
		if trimmed == "" {
			continue
		}
		if value, ok := parent[trimmed]; ok {
			switch typed := value.(type) {
			case string:
				if strings.TrimSpace(typed) != "" {
					return strings.TrimSpace(typed)
				}
			default:
				encoded, err := json.Marshal(typed)
				if err != nil {
					return strings.TrimSpace(fmt.Sprintf("%v", typed))
				}
				if strings.TrimSpace(string(encoded)) != "" {
					return strings.TrimSpace(string(encoded))
				}
			}
		}
	}
	return ""
}
