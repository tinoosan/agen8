package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	messagedomain "github.com/tinoosan/agen8-mcp-server/internal/services/message/domain"
	messagechannel "github.com/tinoosan/agen8-mcp-server/internal/services/message/domain/channel"
	"github.com/tinoosan/agen8-mcp-server/internal/services/message/domain/conversation"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	taskdomain "github.com/tinoosan/agen8-mcp-server/internal/services/task/domain"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

var agentDeliveryRetryDelay = 2 * time.Second

type agentDeliveryWorker struct {
	id     uint64
	cancel context.CancelFunc
}

// DeliverNextAgentMessage hands the next queued durable inbox message to the
// destination member runtime and records the received-message activity card.
func (s *Service) DeliverNextAgentMessage(ctx context.Context, memberID member.ID) (types.AgentMessage, error) {
	if s.harnessChatSender == nil {
		return types.AgentMessage{}, fmt.Errorf("harness chat sender is required")
	}
	if s.conversations == nil {
		return types.AgentMessage{}, fmt.Errorf("message conversation repository is required")
	}
	msg, err := s.ReceiveNextForDelivery(ctx, memberID)
	if err != nil {
		return types.AgentMessage{}, err
	}
	channelID := msg.ChannelID
	if channelID == "" {
		channelID = messagechannel.MemberChannelID(msg.SpaceID, msg.DestinationMemberID)
	}
	stale, reason, err := s.staleTaskNotification(ctx, msg)
	if err != nil {
		return types.AgentMessage{}, err
	}
	if stale {
		if err := s.saveAgentMessageReceivedActivity(ctx, msg, channelID, staleAgentInboxResult(msg, reason)); err != nil {
			return types.AgentMessage{}, err
		}
		delivered, err := s.RecordDelivered(ctx, msg.ID, msg.DestinationMemberID)
		if err != nil {
			return types.AgentMessage{}, err
		}
		s.logInfo("stale task notification suppressed",
			"message_id", delivered.ID,
			"task_ref", delivered.TaskRef,
			"destination_member_id", delivered.DestinationMemberID,
			"reason", reason,
		)
		return delivered, nil
	}
	text := formatAgentInboxRuntimeMessage(msg)
	s.logInfo("agent message delivery starting",
		"message_id", msg.ID,
		"space_id", msg.SpaceID,
		"source_member_id", msg.SourceMemberID,
		"destination_member_id", msg.DestinationMemberID,
		"channel_id", channelID,
		"kind", msg.Kind,
		"producer", msg.Producer,
		"correlation_id", msg.CorrelationID,
	)
	if err := s.saveAgentMessageReceivedActivity(ctx, msg, channelID, pendingAgentInboxResult(msg)); err != nil {
		return types.AgentMessage{}, err
	}
	inbound, err := s.agentInboxConversationMessage(msg, channelID, text)
	if err != nil {
		return types.AgentMessage{}, err
	}
	stream := s.conversationStream(inbound)
	result, err := s.harnessChatSender.SendMessage(ctx, HarnessChatMessage{
		SpaceID:               string(msg.SpaceID),
		MemberID:              string(msg.DestinationMemberID),
		ChannelID:             string(channelID),
		ConversationMessageID: "agent_inbox_" + string(msg.ID),
		SenderType:            "agen8",
		SenderID:              strings.TrimSpace(msg.Producer),
		Text:                  text,
		Stream:                stream,
	})
	if err != nil {
		if isActiveRunDeliveryError(err) {
			s.logInfo("agent message delivery deferred behind active run",
				"message_id", msg.ID,
				"space_id", msg.SpaceID,
				"source_member_id", msg.SourceMemberID,
				"destination_member_id", msg.DestinationMemberID,
				"channel_id", channelID,
				"kind", msg.Kind,
				"producer", msg.Producer,
				"correlation_id", msg.CorrelationID,
				"error", err,
			)
			return types.AgentMessage{}, fmt.Errorf("deliver agent inbox message to harness: %w", err)
		}
		if activityErr := s.saveAgentMessageReceivedActivity(ctx, msg, channelID, failedAgentInboxResult(msg, err)); activityErr != nil {
			s.logError("agent message delivery failed activity update failed",
				"message_id", msg.ID,
				"space_id", msg.SpaceID,
				"destination_member_id", msg.DestinationMemberID,
				"error", activityErr,
			)
		}
		s.logError("agent message delivery failed",
			"message_id", msg.ID,
			"space_id", msg.SpaceID,
			"source_member_id", msg.SourceMemberID,
			"destination_member_id", msg.DestinationMemberID,
			"channel_id", channelID,
			"kind", msg.Kind,
			"producer", msg.Producer,
			"correlation_id", msg.CorrelationID,
			"error", err,
		)
		return types.AgentMessage{}, fmt.Errorf("deliver agent inbox message to harness: %w", err)
	}
	if err := s.saveAgentMessageReceivedActivity(ctx, msg, channelID, result); err != nil {
		return types.AgentMessage{}, err
	}
	if stream.message != nil {
		if err := stream.BindResult(ctx, result); err != nil {
			return types.AgentMessage{}, err
		}
	} else if strings.TrimSpace(result.Text) != "" {
		if _, err := s.saveOutboundConversationMessage(ctx, inbound, result); err != nil {
			return types.AgentMessage{}, err
		}
	}
	delivered, err := s.RecordDelivered(ctx, msg.ID, msg.DestinationMemberID)
	if err != nil {
		return types.AgentMessage{}, err
	}
	s.logInfo("agent message delivered to harness",
		"message_id", delivered.ID,
		"space_id", delivered.SpaceID,
		"source_member_id", delivered.SourceMemberID,
		"destination_member_id", delivered.DestinationMemberID,
		"channel_id", channelID,
		"kind", delivered.Kind,
		"producer", delivered.Producer,
		"correlation_id", delivered.CorrelationID,
		"session_id", result.SessionID,
		"turn_id", result.TurnID,
		"delivery", result.Delivery,
	)
	return delivered, nil
}

// DrainAgentMessages delivers all currently queued messages for memberID.
func (s *Service) DrainAgentMessages(ctx context.Context, memberID member.ID) error {
	for {
		if _, err := s.DeliverNextAgentMessage(ctx, memberID); err != nil {
			if errors.Is(err, messagedomain.ErrMessageNotFound) {
				return nil
			}
			return err
		}
	}
}

// StartAgentDelivery starts a per-member delivery worker that drains queued
// durable inbox messages and wakes again when new messages are published.
func (s *Service) StartAgentDelivery(ctx context.Context, memberID member.ID) error {
	if s == nil {
		return fmt.Errorf("message service is required")
	}
	if ctx == nil {
		return fmt.Errorf("context is required")
	}
	memberID = trimMemberID(memberID)
	if memberID == "" {
		return fmt.Errorf("member id is required")
	}
	s.agentDeliveriesMu.Lock()
	if s.agentDeliveries == nil {
		s.agentDeliveries = map[member.ID]*agentDeliveryWorker{}
	}
	if _, exists := s.agentDeliveries[memberID]; exists {
		s.agentDeliveriesMu.Unlock()
		return nil
	}
	workerCtx, cancel := context.WithCancel(ctx)
	id := uint64(len(s.agentDeliveries)) + uint64(time.Now().UnixNano())
	worker := &agentDeliveryWorker{id: id, cancel: cancel}
	s.agentDeliveries[memberID] = worker
	s.agentDeliveriesMu.Unlock()

	go func() {
		defer s.clearAgentDelivery(memberID, worker)
		s.runAgentDelivery(workerCtx, memberID)
	}()
	return nil
}

// StopAgentDelivery stops the per-member delivery worker when a runtime session
// is removed or replaced.
func (s *Service) StopAgentDelivery(memberID member.ID) {
	if s == nil {
		return
	}
	memberID = trimMemberID(memberID)
	if memberID == "" {
		return
	}
	s.agentDeliveriesMu.Lock()
	worker := s.agentDeliveries[memberID]
	delete(s.agentDeliveries, memberID)
	s.agentDeliveriesMu.Unlock()
	if worker != nil && worker.cancel != nil {
		worker.cancel()
	}
}

// AgentDeliveryRunning reports whether a durable delivery worker is active for
// a member. It is intended for diagnostics and integration tests; callers must
// not treat it as a delivery guarantee because a worker can still exit on
// context cancellation or process shutdown.
func (s *Service) AgentDeliveryRunning(memberID member.ID) bool {
	if s == nil {
		return false
	}
	memberID = trimMemberID(memberID)
	if memberID == "" {
		return false
	}
	s.agentDeliveriesMu.Lock()
	defer s.agentDeliveriesMu.Unlock()
	return s.agentDeliveries[memberID] != nil
}

func (s *Service) clearAgentDelivery(memberID member.ID, worker *agentDeliveryWorker) {
	if s == nil || worker == nil {
		return
	}
	s.agentDeliveriesMu.Lock()
	current := s.agentDeliveries[memberID]
	if current == worker {
		delete(s.agentDeliveries, memberID)
	}
	s.agentDeliveriesMu.Unlock()
}

func (s *Service) runAgentDelivery(ctx context.Context, memberID member.ID) {
	wakes, cancel := s.SubscribeMemberWake(memberID)
	defer cancel()
	var retry <-chan time.Time
	drain := func() {
		if err := s.drainAgentDelivery(ctx, memberID); err != nil {
			if isRetryableAgentDeliveryError(err) {
				retry = time.After(agentDeliveryRetryDelay)
			} else {
				retry = nil
			}
			return
		}
		retry = nil
	}
	drain()
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-wakes:
			if !ok {
				return
			}
			drain()
		case <-retry:
			drain()
		}
	}
}

func (s *Service) drainAgentDelivery(ctx context.Context, memberID member.ID) error {
	if err := s.DrainAgentMessages(ctx, memberID); err != nil && ctx.Err() == nil {
		s.logError("agent inbox drain failed", "member_id", memberID, "error", err)
		return err
	}
	return nil
}

func isRetryableAgentDeliveryError(err error) bool {
	return isActiveRunDeliveryError(err)
}

func isActiveRunDeliveryError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "already has active run")
}

func (s *Service) saveAgentMessageReceivedActivity(ctx context.Context, msg types.AgentMessage, channelID types.ChannelID, result HarnessChatResult) error {
	sessionID := strings.TrimSpace(result.SessionID)
	if sessionID == "" {
		return fmt.Errorf("agent inbox delivery session id is required")
	}
	turnID := strings.TrimSpace(result.TurnID)
	if turnID == "" {
		return fmt.Errorf("agent inbox delivery turn id is required")
	}
	now := s.clock.Now()
	createdAt := msg.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	status := agentInboxActivityStatus(result.Delivery)
	var completedAt *time.Time
	if status != "pending" {
		completedAt = &now
	}
	data := agentMessageActivityData(msg)
	if status == "error" && strings.TrimSpace(result.Text) != "" {
		data["error"] = strings.TrimSpace(result.Text)
	}
	if status == "stale" {
		data["stale"] = "true"
		if strings.TrimSpace(result.Text) != "" {
			data["staleReason"] = strings.TrimSpace(result.Text)
		}
	}
	activity := conversation.Activity{
		ID:          "agent_message_received_" + string(msg.ID),
		ChannelID:   string(channelID),
		SpaceID:     string(msg.SpaceID),
		MemberID:    string(msg.DestinationMemberID),
		SessionID:   sessionID,
		TurnID:      turnID,
		ToolCallID:  string(msg.ID),
		Sequence:    1,
		Kind:        "agent_message_received",
		Title:       agentMessageReceivedTitle(msg),
		Status:      status,
		Text:        agentMessageBodyText(msg.Body),
		CreatedAt:   createdAt,
		CompletedAt: completedAt,
		Data:        data,
	}
	if err := s.conversations.SaveActivity(ctx, activity); err != nil {
		return fmt.Errorf("persist agent message received activity: %w", err)
	}
	if err := s.notifyConversationChanged(ctx, activityNotificationMessage(activity)); err != nil {
		return err
	}
	return s.repo.RecordActivity(ctx, channelID, now)
}

func (s *Service) agentInboxConversationMessage(msg types.AgentMessage, channelID types.ChannelID, text string) (conversation.Message, error) {
	return conversation.NewMessage(conversation.MessageParams{
		ID:         "agent_inbox_" + string(msg.ID),
		ChannelID:  string(channelID),
		SpaceID:    string(msg.SpaceID),
		MemberID:   string(msg.DestinationMemberID),
		Direction:  conversation.DirectionInbound,
		SenderType: "agen8",
		SenderID:   strings.TrimSpace(msg.Producer),
		Text:       text,
		Delivery:   conversation.DeliveryDelivered,
		Render:     conversation.RenderVisible,
		Now:        msg.CreatedAt,
	})
}

func activityNotificationMessage(activity conversation.Activity) conversation.Message {
	return conversation.Message{
		ID:         activity.ID,
		ChannelID:  activity.ChannelID,
		SpaceID:    activity.SpaceID,
		MemberID:   activity.MemberID,
		SessionID:  activity.SessionID,
		TurnID:     activity.TurnID,
		Direction:  conversation.DirectionSystem,
		SenderType: "system",
		Text:       activity.Title,
		Render:     conversation.RenderVisible,
		CreatedAt:  activity.CreatedAt,
		UpdatedAt:  activity.CreatedAt,
	}
}

func pendingAgentInboxResult(msg types.AgentMessage) HarnessChatResult {
	return HarnessChatResult{
		SessionID: "pending:" + string(msg.ID),
		TurnID:    "turn-agent_inbox_" + string(msg.ID),
		Delivery:  "pending",
	}
}

func failedAgentInboxResult(msg types.AgentMessage, err error) HarnessChatResult {
	return HarnessChatResult{
		SessionID: "failed:" + string(msg.ID),
		TurnID:    "turn-agent_inbox_" + string(msg.ID),
		Delivery:  "error",
		Text:      strings.TrimSpace(err.Error()),
	}
}

func staleAgentInboxResult(msg types.AgentMessage, reason string) HarnessChatResult {
	return HarnessChatResult{
		SessionID: "stale:" + string(msg.ID),
		TurnID:    "turn-agent_inbox_" + string(msg.ID),
		Delivery:  "stale",
		Text:      strings.TrimSpace(reason),
	}
}

func agentInboxActivityStatus(delivery string) string {
	switch strings.TrimSpace(delivery) {
	case "pending":
		return "pending"
	case "error", "failed":
		return "error"
	case "stale":
		return "stale"
	default:
		return "completed"
	}
}

func (s *Service) staleTaskNotification(ctx context.Context, msg types.AgentMessage) (bool, string, error) {
	if strings.TrimSpace(msg.Producer) != "task-service" {
		return false, "", nil
	}
	taskID := strings.TrimSpace(msg.TaskRef)
	if taskID == "" {
		return false, "", nil
	}
	expectedStatus := strings.TrimSpace(stringValue(msg.Body, "taskStatus"))
	expectedUpdatedAtRaw := strings.TrimSpace(stringValue(msg.Body, "taskUpdatedAt"))
	if expectedStatus == "" && expectedUpdatedAtRaw == "" {
		return false, "", nil
	}
	if s.tasks == nil {
		return false, "", fmt.Errorf("message service: task state reader is required for task notifications")
	}
	task, err := s.tasks.Get(ctx, taskdomain.TaskID(taskID))
	if err != nil {
		return false, "", fmt.Errorf("load task %s for notification validation: %w", taskID, err)
	}
	currentStatus := strings.TrimSpace(string(task.Status))
	if expectedStatus != "" && currentStatus != expectedStatus {
		return true, fmt.Sprintf("stale task notification: expected task status %s, current status is %s", expectedStatus, currentStatus), nil
	}
	if expectedUpdatedAtRaw != "" && task.UpdatedAt != nil {
		expectedUpdatedAt, err := time.Parse(time.RFC3339Nano, expectedUpdatedAtRaw)
		if err != nil {
			return false, "", fmt.Errorf("parse task notification updatedAt %q: %w", expectedUpdatedAtRaw, err)
		}
		if task.UpdatedAt.UTC().After(expectedUpdatedAt.UTC()) {
			return true, fmt.Sprintf("stale task notification: task updated at %s after notification version %s", task.UpdatedAt.UTC().Format(time.RFC3339Nano), expectedUpdatedAt.UTC().Format(time.RFC3339Nano)), nil
		}
	}
	return false, "", nil
}

func stringValue(values map[string]any, key string) string {
	if len(values) == 0 {
		return ""
	}
	raw, ok := values[key]
	if !ok {
		return ""
	}
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value)
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func formatAgentInboxRuntimeMessage(msg types.AgentMessage) string {
	label := agentMessageSourceLabel(msg)
	header := "Agen8 system message"
	if msg.SourceMemberID != "" {
		header = "Agen8 member message"
	}
	lines := []string{header}
	if msg.SourceMemberID != "" {
		if label != "" {
			lines = append(lines, fmt.Sprintf("from: %s (%s)", label, msg.SourceMemberID))
		} else {
			lines = append(lines, fmt.Sprintf("from: %s", msg.SourceMemberID))
		}
	}
	lines = append(lines,
		"producer: "+strings.TrimSpace(msg.Producer),
		"kind: "+string(msg.Kind),
		"messageId: "+string(msg.ID),
	)
	if msg.CorrelationID != "" {
		lines = append(lines, "correlationId: "+string(msg.CorrelationID))
	}
	if msg.CausationID != "" {
		lines = append(lines, "causationId: "+string(msg.CausationID))
	}
	if strings.TrimSpace(msg.TaskRef) != "" {
		lines = append(lines, "taskRef: "+strings.TrimSpace(msg.TaskRef))
	}
	if strings.TrimSpace(msg.Subject) != "" {
		lines = append(lines, "subject: "+strings.TrimSpace(msg.Subject))
	}
	lines = append(lines, "", "Body:", agentMessageBodyText(msg.Body))
	return strings.Join(lines, "\n")
}

func agentMessageReceivedTitle(msg types.AgentMessage) string {
	if msg.SourceMemberID == "" {
		return "System message"
	}
	if label := agentMessageSourceLabel(msg); label != "" {
		return "Message from " + label
	}
	return "Message from " + string(msg.SourceMemberID)
}

func agentMessageSourceLabel(msg types.AgentMessage) string {
	for _, key := range []string{"sourceMemberLabel", "source_member_label"} {
		if value, ok := msg.Metadata[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func agentMessageBodyText(body map[string]any) string {
	if text, ok := body["text"].(string); ok && strings.TrimSpace(text) != "" {
		return strings.TrimSpace(text)
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return fmt.Sprintf("%v", body)
	}
	return string(encoded)
}

func agentMessageActivityData(msg types.AgentMessage) map[string]string {
	data := map[string]string{
		"messageId":           string(msg.ID),
		"destinationMemberId": string(msg.DestinationMemberID),
		"producer":            strings.TrimSpace(msg.Producer),
		"kind":                string(msg.Kind),
		"subject":             strings.TrimSpace(msg.Subject),
	}
	if msg.SourceMemberID != "" {
		data["sourceMemberId"] = string(msg.SourceMemberID)
	}
	if label := agentMessageSourceLabel(msg); label != "" {
		data["sourceMemberLabel"] = label
	}
	if msg.CorrelationID != "" {
		data["correlationId"] = string(msg.CorrelationID)
	}
	if msg.CausationID != "" {
		data["causationId"] = string(msg.CausationID)
	}
	if strings.TrimSpace(msg.TaskRef) != "" {
		data["taskRef"] = strings.TrimSpace(msg.TaskRef)
	}
	return data
}
