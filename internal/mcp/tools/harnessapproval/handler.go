package harnessapproval

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	harnessdomain "github.com/tinoosan/agen8-mcp-server/internal/services/harness/domain"
	humaninput "github.com/tinoosan/agen8-mcp-server/internal/services/humaninput/domain"
)

type HumanInputAwaiter interface {
	Await(context.Context, humaninput.PendingRequest) (json.RawMessage, error)
}

type CallContext struct {
	HumanInputAwaiter HumanInputAwaiter
	ProjectID         string
	SpaceID           string
	MemberID          string
	ChannelID         string
}

type Result struct {
	Text       string
	Structured any
}

type parsedApprovalRequest struct {
	Request        harnessdomain.ApprovalRequest
	Input          json.RawMessage
	NativeToolName string
}

func (h Handler) Handle(ctx context.Context, call CallContext, args json.RawMessage) (Result, error) {
	if call.HumanInputAwaiter == nil {
		return Result{}, fmt.Errorf("harness approval: human input awaiter is not configured")
	}
	parsed, err := approvalRequest(args, call)
	if err != nil {
		return Result{}, err
	}
	if shouldAutoAllow(parsed) {
		return permissionResult("allow", parsed.Input, "")
	}
	req := parsed.Request
	payload := humaninput.ApproveRejectPayload{
		Title:       approvalTitle(req),
		Description: approvalDescription(req),
		Context:     approvalContext(req),
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return Result{}, fmt.Errorf("harness approval: encode payload: %w", err)
	}
	resultBytes, err := call.HumanInputAwaiter.Await(ctx, humaninput.PendingRequest{
		ToolCallID:     req.ToolCallID,
		ToolName:       Name,
		IdempotencyKey: approvalIdempotencyKey(req),
		ProjectID:      strings.TrimSpace(call.ProjectID),
		SpaceID:        strings.TrimSpace(call.SpaceID),
		MemberID:       strings.TrimSpace(call.MemberID),
		ChannelID:      strings.TrimSpace(call.ChannelID),
		Declaration: humaninput.Declaration{
			Kind:    humaninput.PrimitiveApproveReject,
			Payload: payloadBytes,
		},
	})
	if err != nil {
		return Result{}, err
	}
	var resolved humaninput.ApproveRejectResult
	if err := json.Unmarshal(resultBytes, &resolved); err != nil {
		return Result{}, fmt.Errorf("harness approval: decode result: %w", err)
	}
	decision := strings.ToLower(strings.TrimSpace(resolved.Decision))
	if resolved.Cancelled {
		decision = "reject"
	}
	switch decision {
	case "approve":
		return permissionResult("allow", parsed.Input, "")
	case "reject":
		note := strings.TrimSpace(resolved.Note)
		if note == "" {
			note = "User rejected this action"
		}
		return permissionResult("deny", nil, note)
	default:
		return Result{}, fmt.Errorf("harness approval: unsupported approval decision %q", resolved.Decision)
	}
}

func approvalRequest(args json.RawMessage, call CallContext) (parsedApprovalRequest, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(args, &raw); err != nil {
		return parsedApprovalRequest{}, fmt.Errorf("harness approval: invalid arguments: %w", err)
	}
	toolName := firstJSONText(raw, "tool_name", "toolName", "name")
	if toolName == "" {
		return parsedApprovalRequest{}, fmt.Errorf("harness approval: tool_name is required")
	}
	input := firstJSONObject(raw, "tool_input", "toolInput", "input")
	toolCallID := firstJSONText(raw, "tool_use_id", "toolUseId", "tool_call_id", "toolCallId", "id")
	if toolCallID == "" {
		toolCallID = stableCallID(toolName, input)
	}
	data := map[string]string{
		"harness": "claude-code",
	}
	if spaceID := strings.TrimSpace(call.SpaceID); spaceID != "" {
		data["spaceId"] = spaceID
	}
	if memberID := strings.TrimSpace(call.MemberID); memberID != "" {
		data["memberId"] = memberID
	}
	if channelID := strings.TrimSpace(call.ChannelID); channelID != "" {
		data["channelId"] = channelID
	}
	if cwd := firstJSONText(raw, "cwd", "workdir", "working_directory", "workingDirectory"); cwd != "" {
		data["cwd"] = cwd
	}
	if reason := firstJSONText(raw, "reason", "description"); reason != "" {
		data["reason"] = reason
	}
	if len(input) > 0 {
		data["input"] = string(bytes.TrimSpace(input))
	}
	req := harnessdomain.ApprovalRequest{
		ApprovalID: toolCallID,
		ToolCallID: toolCallID,
		ToolName:   "claude/" + toolName,
		Command:    commandFromInput(toolName, input),
		Path:       pathFromInput(input),
		Summary:    "Approve Claude Code " + toolName,
		Method:     "claude.permission_prompt",
		Data:       data,
	}
	return parsedApprovalRequest{Request: req, Input: input, NativeToolName: toolName}, nil
}

func shouldAutoAllow(parsed parsedApprovalRequest) bool {
	toolName := strings.ToLower(strings.TrimSpace(parsed.NativeToolName))
	switch toolName {
	case "mcp__agen8__harness_approval", "harness_approval":
		return true
	case "mcp__agen8__decision", "decision":
		return strings.EqualFold(textField(parsed.Input, "action"), "ask_user")
	default:
		return false
	}
}

func permissionResult(behavior string, input json.RawMessage, message string) (Result, error) {
	payload := map[string]any{"behavior": behavior}
	switch behavior {
	case "allow":
		if len(input) == 0 {
			payload["updatedInput"] = map[string]any{}
		} else {
			var decoded any
			if err := json.Unmarshal(input, &decoded); err != nil {
				return Result{}, fmt.Errorf("harness approval: decode approved input: %w", err)
			}
			payload["updatedInput"] = decoded
		}
	case "deny":
		payload["message"] = strings.TrimSpace(message)
	default:
		return Result{}, fmt.Errorf("harness approval: unsupported permission behavior %q", behavior)
	}
	text, err := json.Marshal(payload)
	if err != nil {
		return Result{}, fmt.Errorf("harness approval: encode permission result: %w", err)
	}
	return Result{Text: string(text), Structured: payload}, nil
}

func approvalTitle(req harnessdomain.ApprovalRequest) string {
	if summary := strings.TrimSpace(req.Summary); summary != "" {
		return summary
	}
	return "Approve Claude Code tool use"
}

func approvalDescription(req harnessdomain.ApprovalRequest) string {
	if command := strings.TrimSpace(req.Command); command != "" {
		return command
	}
	if path := strings.TrimSpace(req.Path); path != "" {
		return path
	}
	return strings.TrimSpace(req.ToolName)
}

func approvalContext(req harnessdomain.ApprovalRequest) string {
	pairs := make([]string, 0, len(req.Data)+3)
	if method := strings.TrimSpace(req.Method); method != "" {
		pairs = append(pairs, "method="+method)
	}
	if approvalID := strings.TrimSpace(req.ApprovalID); approvalID != "" {
		pairs = append(pairs, "approvalId="+approvalID)
	}
	for key, value := range req.Data {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		pairs = append(pairs, key+"="+value)
	}
	return strings.Join(pairs, "\n")
}

func approvalIdempotencyKey(req harnessdomain.ApprovalRequest) string {
	return strings.TrimSpace(req.Method + ":" + req.ToolCallID)
}

func firstJSONText(raw map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		var text string
		if err := json.Unmarshal(value, &text); err == nil {
			if trimmed := strings.TrimSpace(text); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

func firstJSONObject(raw map[string]json.RawMessage, keys ...string) json.RawMessage {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		trimmed := bytes.TrimSpace(value)
		if len(trimmed) == 0 || !json.Valid(trimmed) || trimmed[0] != '{' {
			continue
		}
		return append(json.RawMessage(nil), trimmed...)
	}
	return json.RawMessage(`{}`)
}

func commandFromInput(toolName string, input json.RawMessage) string {
	if !strings.EqualFold(strings.TrimSpace(toolName), "Bash") {
		return ""
	}
	return textField(input, "command")
}

func pathFromInput(input json.RawMessage) string {
	return firstNonEmpty(
		textField(input, "file_path"),
		textField(input, "path"),
	)
}

func textField(input json.RawMessage, key string) string {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(input, &raw); err != nil {
		return ""
	}
	return firstJSONText(raw, key)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func stableCallID(toolName string, input json.RawMessage) string {
	sum := sha256Bytes(bytes.TrimSpace(input))
	return "claude-permission:" + strings.TrimSpace(toolName) + ":" + sum[:16]
}

func sha256Bytes(input []byte) string {
	sum := sha256.Sum256(input)
	return hex.EncodeToString(sum[:])
}
