package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/tinoosan/agen8/pkg/agent/state"
	"github.com/tinoosan/agen8/pkg/protocol"
	"github.com/tinoosan/agen8/pkg/timeutil"
	"github.com/tinoosan/agen8/pkg/types"
)

func (s *RPCServer) messageList(ctx context.Context, p protocol.MessageListParams) (protocol.MessageListResult, error) {
	if s.taskService == nil {
		return protocol.MessageListResult{}, &protocol.ProtocolError{Code: protocol.CodeInternalError, Message: "task store not configured"}
	}
	scope, err := s.resolveMessageScope(ctx, p.ThreadID, p.TeamID, p.RunID)
	if err != nil {
		return protocol.MessageListResult{}, err
	}
	view := strings.ToLower(strings.TrimSpace(p.View))
	if view == "" {
		view = "inbox"
	}
	if view != "inbox" && view != "outbox" {
		return protocol.MessageListResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "view must be inbox or outbox"}
	}
	scopeMode := strings.ToLower(strings.TrimSpace(p.Scope))
	if scopeMode != "" && scopeMode != "team" && scopeMode != "run" {
		return protocol.MessageListResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "scope must be team or run"}
	}
	filter := state.MessageFilter{
		ThreadID:          scope.threadID,
		DestinationTeamID: scope.teamID,
		TeamID:            scope.teamID,
		RunID:             scope.runID,
		Kinds:             append([]string(nil), p.Kinds...),
		Statuses:          append([]string(nil), p.Statuses...),
		SortBy:            "created_at",
	}
	if scopeMode == "team" || (scope.teamID != "" && strings.TrimSpace(p.RunID) == "") {
		filter.RunID = ""
	}
	msgs, err := s.taskService.ListMessages(ctx, filter)
	if err != nil {
		return protocol.MessageListResult{}, err
	}
	rows, err := s.projectMessages(ctx, msgs)
	if err != nil {
		return protocol.MessageListResult{}, err
	}
	rows = filterProjectedMessagesForView(rows, view)
	sortProjectedMessages(rows, view)
	total := len(rows)
	start := max(0, p.Offset)
	if start > total {
		start = total
	}
	end := total
	if limit := clampLimit(p.Limit, 200, 2000); limit > 0 && start+limit < end {
		end = start + limit
	}
	return protocol.MessageListResult{
		Messages:   append([]protocol.MailMessage(nil), rows[start:end]...),
		TotalCount: total,
	}, nil
}

func (s *RPCServer) messageGet(ctx context.Context, p protocol.MessageGetParams) (protocol.MessageGetResult, error) {
	if s.taskService == nil {
		return protocol.MessageGetResult{}, &protocol.ProtocolError{Code: protocol.CodeInternalError, Message: "task store not configured"}
	}
	messageID := strings.TrimSpace(p.MessageID)
	if messageID == "" {
		return protocol.MessageGetResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "messageId is required"}
	}
	msg, err := s.taskService.GetMessage(ctx, messageID)
	if err != nil {
		if errors.Is(err, state.ErrMessageNotFound) {
			return protocol.MessageGetResult{}, &protocol.ProtocolError{Code: protocol.CodeTurnNotFound, Message: "message not found"}
		}
		return protocol.MessageGetResult{}, err
	}
	threadID := strings.TrimSpace(string(p.ThreadID))
	if threadID != "" && strings.TrimSpace(msg.ThreadID) != threadID {
		return protocol.MessageGetResult{}, &protocol.ProtocolError{Code: protocol.CodeTurnNotFound, Message: "message not found"}
	}
	teamID := strings.TrimSpace(p.TeamID)
	if teamID != "" && strings.TrimSpace(msg.DestinationTeamID) != teamID && strings.TrimSpace(msg.TeamID) != teamID {
		return protocol.MessageGetResult{}, &protocol.ProtocolError{Code: protocol.CodeTurnNotFound, Message: "message not found"}
	}
	rows, err := s.projectMessages(ctx, []types.AgentMessage{msg})
	if err != nil {
		return protocol.MessageGetResult{}, err
	}
	if len(rows) == 0 {
		return protocol.MessageGetResult{}, &protocol.ProtocolError{Code: protocol.CodeTurnNotFound, Message: "message not found"}
	}
	return protocol.MessageGetResult{Message: rows[0]}, nil
}

type messageScope struct {
	threadID string
	teamID   string
	runID    string
}

func (s *RPCServer) resolveMessageScope(ctx context.Context, threadID protocol.ThreadID, teamIDOverride, runIDOverride string) (messageScope, error) {
	scope := messageScope{
		threadID: strings.TrimSpace(string(threadID)),
		teamID:   strings.TrimSpace(teamIDOverride),
		runID:    strings.TrimSpace(runIDOverride),
	}
	if scope.threadID != "" {
		resolved, err := s.resolveThreadID(threadID)
		if err != nil {
			return messageScope{}, err
		}
		scope.threadID = strings.TrimSpace(resolved)
		if scope.teamID == "" || scope.runID == "" {
			if artifact, err := s.resolveTeamOrRunScope(ctx, threadID, scope.teamID, scope.runID); err == nil {
				if scope.teamID == "" {
					scope.teamID = strings.TrimSpace(artifact.teamID)
				}
				if scope.runID == "" {
					scope.runID = strings.TrimSpace(artifact.runID)
				}
			}
		}
	}
	if scope.teamID == "" && scope.runID == "" && scope.threadID == "" {
		return messageScope{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "teamId, runId, or threadId is required"}
	}
	return scope, nil
}

func (s *RPCServer) projectMessages(ctx context.Context, msgs []types.AgentMessage) ([]protocol.MailMessage, error) {
	if len(msgs) == 0 {
		return nil, nil
	}
	rows := make([]protocol.MailMessage, 0, len(msgs))
	for _, msg := range msgs {
		row, err := s.projectMessage(ctx, msg)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (s *RPCServer) projectMessage(ctx context.Context, msg types.AgentMessage) (protocol.MailMessage, error) {
	var task *types.Task
	if s.taskService != nil && strings.TrimSpace(msg.TaskRef) != "" {
		loaded, err := s.taskService.GetTask(ctx, strings.TrimSpace(msg.TaskRef))
		if err == nil {
			task = &loaded
		}
	}
	if task == nil {
		task = effectiveTaskForMessage(msg)
	}
	subject, summary, bodyPreview := summarizeMessageContent(msg, task)
	row := protocol.MailMessage{
		MessageID:         strings.TrimSpace(msg.MessageID),
		ThreadID:          protocol.ThreadID(strings.TrimSpace(msg.ThreadID)),
		RunID:             protocol.RunID(strings.TrimSpace(msg.RunID)),
		SourceTeamID:      strings.TrimSpace(msg.SourceTeamID),
		DestinationTeamID: strings.TrimSpace(msg.DestinationTeamID),
		TeamID:            strings.TrimSpace(msg.TeamID),
		Channel:           strings.TrimSpace(msg.Channel),
		Kind:              strings.TrimSpace(msg.Kind),
		Status:            strings.TrimSpace(msg.Status),
		Subject:           subject,
		Summary:           summary,
		BodyPreview:       bodyPreview,
		Error:             firstNonEmpty(strings.TrimSpace(msg.Error), bodyString(msg.Body, "error"), bodyString(msg.Metadata, "error")),
		TaskID:            strings.TrimSpace(msg.TaskRef),
		ReadOnly:          task == nil,
		CreatedAt:         timeutil.OrNow(msg.CreatedAt),
		UpdatedAt:         timeutil.OrNow(msg.UpdatedAt),
		ProcessedAt:       msg.ProcessedAt,
	}
	if task != nil {
		pt := protocolTaskFromTypesTask(*task)
		row.Task = &pt
		row.TaskID = strings.TrimSpace(pt.ID)
		row.TaskStatus = strings.TrimSpace(pt.Status)
		row.ReadOnly = false
		row.CanClaim = row.TaskStatus == string(types.TaskStatusPending)
		row.CanComplete = row.TaskStatus == string(types.TaskStatusActive)
	}
	return row, nil
}

func effectiveTaskForMessage(msg types.AgentMessage) *types.Task {
	if msg.Task == nil {
		return nil
	}
	taskCopy := *msg.Task
	return &taskCopy
}

func summarizeMessageContent(msg types.AgentMessage, task *types.Task) (string, string, string) {
	subject := firstNonEmpty(
		bodyString(msg.Body, "subject"),
		bodyString(msg.Body, "title"),
		bodyString(msg.Body, "goal"),
	)
	summary := firstNonEmpty(
		bodyString(msg.Body, "summary"),
		bodyString(msg.Body, "message"),
		bodyString(msg.Body, "text"),
	)
	bodyPreview := firstNonEmpty(
		bodyString(msg.Body, "preview"),
		bodyString(msg.Body, "body"),
		bodyString(msg.Body, "content"),
		bodyString(msg.Body, "message"),
		bodyString(msg.Body, "text"),
	)
	if task != nil {
		taskGoal := strings.TrimSpace(task.Goal)
		taskSummary := strings.TrimSpace(task.Summary)
		subject = firstNonEmpty(subject, taskGoal, strings.TrimSpace(task.TaskKind))
		summary = firstNonEmpty(summary, taskSummary, taskGoal)
		bodyPreview = firstNonEmpty(bodyPreview, taskSummary, taskGoal)
	} else {
		subject = firstNonEmpty(subject, strings.TrimSpace(msg.Kind), strings.TrimSpace(msg.MessageID))
		summary = firstNonEmpty(summary, bodyPreview, subject)
		bodyPreview = firstNonEmpty(bodyPreview, summary, subject)
	}
	return subject, summary, bodyPreview
}

func bodyString(data map[string]any, key string) string {
	if len(data) == 0 {
		return ""
	}
	raw, ok := data[key]
	if !ok || raw == nil {
		return ""
	}
	return stringifyBodyValue(raw)
}

func stringifyBodyValue(raw any) string {
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	case []string:
		return strings.TrimSpace(strings.Join(v, " "))
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if text := stringifyBodyValue(item); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.TrimSpace(strings.Join(parts, " "))
	case map[string]any:
		for _, key := range []string{"text", "message", "content", "summary"} {
			if text := bodyString(v, key); text != "" {
				return text
			}
		}
		b, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(b))
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func filterProjectedMessagesForView(rows []protocol.MailMessage, view string) []protocol.MailMessage {
	filtered := make([]protocol.MailMessage, 0, len(rows))
	for _, row := range rows {
		taskStatus := strings.TrimSpace(row.TaskStatus)
		switch view {
		case "inbox":
			if row.Task != nil {
				if taskStatus == string(types.TaskStatusPending) || taskStatus == string(types.TaskStatusActive) || taskStatus == string(types.TaskStatusReviewPending) {
					filtered = append(filtered, row)
				}
				continue
			}
			if row.Status == types.MessageStatusAcked || row.Status == types.MessageStatusDeadletter {
				continue
			}
			filtered = append(filtered, row)
		case "outbox":
			if row.Task != nil {
				if taskStatus == string(types.TaskStatusReviewPending) || isTerminalMailTaskStatus(taskStatus) {
					filtered = append(filtered, row)
				}
				continue
			}
			if row.Channel == types.MessageChannelOutbox || row.Status == types.MessageStatusAcked || row.Status == types.MessageStatusDeadletter {
				filtered = append(filtered, row)
			}
		}
	}
	return filtered
}

func sortProjectedMessages(rows []protocol.MailMessage, view string) {
	sort.SliceStable(rows, func(i, j int) bool {
		left := projectedMessageSortTime(rows[i], view)
		right := projectedMessageSortTime(rows[j], view)
		if left.Equal(right) {
			return rows[i].MessageID < rows[j].MessageID
		}
		if view == "inbox" {
			return left.Before(right)
		}
		return left.After(right)
	})
}

func projectedMessageSortTime(row protocol.MailMessage, view string) time.Time {
	if view == "outbox" {
		if row.Task != nil && !row.Task.CompletedAt.IsZero() {
			return row.Task.CompletedAt.UTC()
		}
		if row.ProcessedAt != nil && !row.ProcessedAt.IsZero() {
			return row.ProcessedAt.UTC()
		}
	}
	return row.CreatedAt.UTC()
}

func isTerminalMailTaskStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case string(types.TaskStatusSucceeded), string(types.TaskStatusFailed), string(types.TaskStatusCanceled):
		return true
	default:
		return false
	}
}
