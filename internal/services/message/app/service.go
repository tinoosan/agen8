package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"

	"github.com/tinoosan/agen8-mcp-server/internal/services/message/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/message/domain/channel"
	"github.com/tinoosan/agen8-mcp-server/internal/services/message/domain/conversation"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	taskdomain "github.com/tinoosan/agen8-mcp-server/internal/services/task/domain"
	"github.com/tinoosan/agen8-mcp-server/pkg/signalhub"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

type Repository interface {
	domain.Repository
	channel.Repository
}

// Service coordinates message inbox and channel use cases.
type Service struct {
	repo               Repository
	conversations      conversation.Repository
	harnessChatSender  HarnessChatSender
	conversationNotify ConversationNotifier
	projectRoots       ProjectRootResolver
	tasks              TaskStateReader
	logger             *slog.Logger
	clock              Clock
	autoStartDelivery  bool
	wakes              *signalhub.PayloadHub[WakeFilter, domain.MessageWake]
	agentWakeQueue     chan types.AgentMessage
	agentDeliveriesMu  sync.Mutex
	agentDeliveries    map[member.ID]*agentDeliveryWorker
	externalStreamsMu  sync.Mutex
	externalStreams    map[string]*conversationStreamSink
}

// WakeFilter selects member or space wake subscriptions.
type WakeFilter struct {
	MemberID member.ID
	SpaceID  spacedomain.SpaceID
}

type HarnessChatSender interface {
	SendMessage(ctx context.Context, input HarnessChatMessage) (HarnessChatResult, error)
}

type HarnessAssistantDelta struct {
	SessionID string
	TurnID    string
	Sequence  int
	Text      string
}

type HarnessThinkingDelta struct {
	SessionID string
	TurnID    string
	Sequence  int
	Text      string
	Data      map[string]string
}

type HarnessChatStream interface {
	AppendAssistantDelta(ctx context.Context, delta HarnessAssistantDelta) error
	AppendThinkingDelta(ctx context.Context, delta HarnessThinkingDelta) error
	AppendActivity(ctx context.Context, activity HarnessActivity) error
}

type HarnessChatMessage struct {
	SpaceID               string
	MemberID              string
	ChannelID             string
	ConversationMessageID string
	SenderType            string
	SenderID              string
	Text                  string
	Attachments           []conversation.Attachment
	AllowSteering         bool
	Stream                HarnessChatStream
}

type HarnessChatResult struct {
	SessionID string
	RunID     string
	TurnID    string
	Delivery  string
	Text      string
}

type HarnessActivity struct {
	SessionID  string
	TurnID     string
	ToolCallID string
	ToolName   string
	Sequence   int
	Status     string
	Text       string
	Data       map[string]string
}

type ConversationNotifier interface {
	NotifyConversationChanged(ctx context.Context, message conversation.Message) error
}

type TaskStateReader interface {
	Get(ctx context.Context, taskID taskdomain.TaskID) (taskdomain.Task, error)
}

type ProjectRootResolver interface {
	ResolveProjectRoot(ctx context.Context, projectID types.ProjectID) (string, error)
}

// NewServiceParams contains required message service dependencies.
type NewServiceParams struct {
	Repository             Repository
	Conversations          conversation.Repository
	HarnessChatSender      HarnessChatSender
	AutoStartAgentDelivery bool
	Logger                 *slog.Logger
	Clock                  Clock
}

// NewService builds a message service with repository-backed persistence and in-process wakes.
func NewService(params NewServiceParams) (*Service, error) {
	if params.Repository == nil {
		return nil, fmt.Errorf("message service: repository is required")
	}
	clock := params.Clock
	if clock == nil {
		clock = SystemClock{}
	}
	service := &Service{
		repo:              params.Repository,
		conversations:     params.Conversations,
		harnessChatSender: params.HarnessChatSender,
		logger:            params.Logger,
		clock:             clock,
		autoStartDelivery: params.AutoStartAgentDelivery,
		wakes:             signalhub.NewPayload[WakeFilter, domain.MessageWake](),
		agentWakeQueue:    make(chan types.AgentMessage, 256),
		agentDeliveries:   map[member.ID]*agentDeliveryWorker{},
		externalStreams:   map[string]*conversationStreamSink{},
	}
	service.startAgentWakeDispatcher()
	return service, nil
}

func (s *Service) SetProjectRootResolver(resolver ProjectRootResolver) {
	s.projectRoots = resolver
}

func (s *Service) SetConversationNotifier(notifier ConversationNotifier) {
	s.conversationNotify = notifier
}

type HarnessExternalEvent struct {
	SpaceID    string
	ChannelID  string
	MemberID   string
	SessionID  string
	SessionRef string
	TurnID     string
	Sequence   int
	UserText   string
	Text       string
	Thinking   string
	Data       map[string]string
	Activity   *HarnessActivity
	Completed  bool
}

func (s *Service) AppendHarnessExternalEvent(ctx context.Context, event HarnessExternalEvent) error {
	if s == nil {
		return fmt.Errorf("message service is required")
	}
	if s.conversations == nil {
		return fmt.Errorf("message conversation repository is required")
	}
	if strings.TrimSpace(event.ChannelID) == "" {
		return fmt.Errorf("channel id is required")
	}
	if strings.TrimSpace(event.SpaceID) == "" {
		return fmt.Errorf("space id is required")
	}
	if strings.TrimSpace(event.MemberID) == "" {
		return fmt.Errorf("member id is required")
	}
	if strings.TrimSpace(event.SessionID) == "" {
		return fmt.Errorf("session id is required")
	}
	if strings.TrimSpace(event.TurnID) == "" {
		return fmt.Errorf("turn id is required")
	}
	var stream *conversationStreamSink
	if strings.TrimSpace(event.UserText) != "" {
		inbound, err := s.saveExternalInboundConversationMessage(ctx, event)
		if err != nil {
			return err
		}
		stream = s.externalStreamWithInbound(event, &inbound)
	}
	if strings.TrimSpace(event.Text) != "" {
		if stream == nil {
			stream = s.externalStream(event)
		}
		if err := stream.AppendAssistantDelta(ctx, HarnessAssistantDelta{
			SessionID: event.SessionID,
			TurnID:    event.TurnID,
			Sequence:  event.Sequence,
			Text:      event.Text,
		}); err != nil {
			return err
		}
	}
	if strings.TrimSpace(event.Thinking) != "" {
		if stream == nil {
			stream = s.externalStream(event)
		}
		if err := stream.AppendThinkingDelta(ctx, HarnessThinkingDelta{
			SessionID: event.SessionID,
			TurnID:    event.TurnID,
			Sequence:  event.Sequence,
			Text:      event.Thinking,
			Data:      event.Data,
		}); err != nil {
			return err
		}
	}
	if event.Activity != nil {
		if stream == nil {
			stream = s.externalStream(event)
		}
		activity := *event.Activity
		activity.SessionID = event.SessionID
		activity.TurnID = event.TurnID
		if activity.Sequence <= 0 {
			activity.Sequence = event.Sequence
		}
		if activity.Data == nil {
			activity.Data = map[string]string{}
		}
		activity.Data["origin"] = "external_codex"
		if strings.TrimSpace(event.SessionRef) != "" {
			activity.Data["nativeSessionRef"] = strings.TrimSpace(event.SessionRef)
		}
		if err := stream.AppendActivity(ctx, activity); err != nil {
			return err
		}
	}
	if event.Completed {
		s.externalStreamsMu.Lock()
		delete(s.externalStreams, externalStreamKey(event.SessionID, event.TurnID))
		s.externalStreamsMu.Unlock()
	}
	return nil
}

func (s *Service) externalStream(event HarnessExternalEvent) *conversationStreamSink {
	return s.externalStreamWithInbound(event, nil)
}

func (s *Service) externalStreamWithInbound(event HarnessExternalEvent, inbound *conversation.Message) *conversationStreamSink {
	key := externalStreamKey(event.SessionID, event.TurnID)
	s.externalStreamsMu.Lock()
	defer s.externalStreamsMu.Unlock()
	if stream := s.externalStreams[key]; stream != nil {
		if inbound != nil {
			stream.inbound = *inbound
		}
		return stream
	}
	var streamInbound conversation.Message
	if inbound != nil {
		streamInbound = *inbound
	} else {
		at := s.clock.Now()
		if event.Sequence > 0 {
			at = at.Add(-time.Duration(event.Sequence) * time.Millisecond)
		}
		streamInbound = conversation.Message{
			ID:         "external_" + key,
			ChannelID:  strings.TrimSpace(event.ChannelID),
			SpaceID:    strings.TrimSpace(event.SpaceID),
			MemberID:   strings.TrimSpace(event.MemberID),
			Direction:  conversation.DirectionSystem,
			SenderType: "external_codex",
			SenderID:   strings.TrimSpace(event.MemberID),
			Text:       "External Codex turn",
			Render:     conversation.RenderVisible,
			CreatedAt:  at,
			UpdatedAt:  at,
		}
	}
	stream := s.conversationStream(streamInbound)
	s.externalStreams[key] = stream
	return stream
}

func (s *Service) saveExternalInboundConversationMessage(ctx context.Context, event HarnessExternalEvent) (conversation.Message, error) {
	at := s.clock.Now()
	if event.Sequence > 0 {
		at = at.Add(-time.Duration(event.Sequence) * time.Millisecond)
	}
	msg, err := conversation.NewMessage(conversation.MessageParams{
		ID:         "conversation_" + uuid.NewString(),
		ChannelID:  strings.TrimSpace(event.ChannelID),
		SpaceID:    strings.TrimSpace(event.SpaceID),
		MemberID:   strings.TrimSpace(event.MemberID),
		SessionID:  strings.TrimSpace(event.SessionID),
		TurnID:     strings.TrimSpace(event.TurnID),
		Direction:  conversation.DirectionInbound,
		SenderType: "external_codex_user",
		SenderID:   strings.TrimSpace(event.MemberID),
		Text:       strings.TrimSpace(event.UserText),
		Delivery:   conversation.DeliveryDelivered,
		Render:     conversation.RenderVisible,
		Now:        at,
	})
	if err != nil {
		return conversation.Message{}, err
	}
	if err := s.conversations.Save(ctx, msg); err != nil {
		return conversation.Message{}, fmt.Errorf("persist external inbound conversation message: %w", err)
	}
	if err := s.notifyConversationChanged(ctx, msg); err != nil {
		return conversation.Message{}, err
	}
	if err := s.repo.RecordActivity(ctx, types.ChannelID(msg.ChannelID), msg.CreatedAt); err != nil {
		return conversation.Message{}, fmt.Errorf("record external inbound conversation channel activity: %w", err)
	}
	return msg, nil
}

func externalStreamKey(sessionID, turnID string) string {
	raw := strings.NewReplacer(" ", "_", "/", "_", ":", "_").Replace(strings.TrimSpace(sessionID) + "_" + strings.TrimSpace(turnID))
	return raw
}

func (s *Service) SetTaskStateReader(tasks TaskStateReader) {
	s.tasks = tasks
}

const maxConversationAttachmentBytes = 10 * 1024 * 1024

var allowedConversationAttachmentMediaTypes = map[string]struct{}{
	"image/png":  {},
	"image/jpeg": {},
	"image/webp": {},
}

var unsafeAttachmentNameChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

type UploadConversationAttachmentParams struct {
	ChannelID types.ChannelID
	ProjectID types.ProjectID
	Name      string
	MediaType string
	Bytes     []byte
}

type ConversationAttachmentBlob struct {
	Attachment conversation.Attachment
	Bytes      []byte
}

func (s *Service) UploadConversationAttachment(ctx context.Context, params UploadConversationAttachmentParams) (conversation.Attachment, error) {
	if s.conversations == nil {
		return conversation.Attachment{}, fmt.Errorf("message conversation repository is required")
	}
	if s.projectRoots == nil {
		return conversation.Attachment{}, fmt.Errorf("message project root resolver is required")
	}
	channelID := types.ChannelID(strings.TrimSpace(string(params.ChannelID)))
	if channelID == "" {
		return conversation.Attachment{}, fmt.Errorf("channel id is required")
	}
	name := safeAttachmentName(params.Name)
	if name == "" {
		return conversation.Attachment{}, fmt.Errorf("attachment name is required")
	}
	mediaType := strings.ToLower(strings.TrimSpace(params.MediaType))
	if _, ok := allowedConversationAttachmentMediaTypes[mediaType]; !ok {
		return conversation.Attachment{}, fmt.Errorf("unsupported attachment media type %q", params.MediaType)
	}
	if len(params.Bytes) == 0 {
		return conversation.Attachment{}, fmt.Errorf("attachment bytes are required")
	}
	if len(params.Bytes) > maxConversationAttachmentBytes {
		return conversation.Attachment{}, fmt.Errorf("attachment exceeds %d byte limit", maxConversationAttachmentBytes)
	}
	ch, err := s.loadOrEnsureConversationChannel(ctx, channelID)
	if err != nil {
		return conversation.Attachment{}, fmt.Errorf("load conversation channel: %w", err)
	}
	if ch.SpaceID == "" {
		return conversation.Attachment{}, fmt.Errorf("channel %q has no space id", channelID)
	}
	projectID := types.ProjectID(strings.TrimSpace(string(ch.ProjectID)))
	requestProjectID := types.ProjectID(strings.TrimSpace(string(params.ProjectID)))
	if projectID == "" {
		projectID = requestProjectID
	}
	if projectID == "" {
		return conversation.Attachment{}, fmt.Errorf("project id is required")
	}
	if requestProjectID != "" && ch.ProjectID != "" && requestProjectID != ch.ProjectID {
		return conversation.Attachment{}, fmt.Errorf("channel %q belongs to project %q, not %q", channelID, ch.ProjectID, requestProjectID)
	}
	root, err := s.projectRoots.ResolveProjectRoot(ctx, projectID)
	if err != nil {
		return conversation.Attachment{}, fmt.Errorf("resolve project root for attachment: %w", err)
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return conversation.Attachment{}, fmt.Errorf("project %s root is required", projectID)
	}
	attachmentID := "attachment_" + uuid.NewString()
	dir := filepath.Join(attachmentStoreBase(root), "conversation-attachments", sanitizePathSegment(string(channelID)))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return conversation.Attachment{}, fmt.Errorf("create attachment directory: %w", err)
	}
	path := filepath.Join(dir, attachmentID+"-"+name)
	if err := os.WriteFile(path, params.Bytes, 0o600); err != nil {
		return conversation.Attachment{}, fmt.Errorf("write attachment file: %w", err)
	}
	attachment := conversation.Attachment{
		ID:        attachmentID,
		ProjectID: string(projectID),
		SpaceID:   string(ch.SpaceID),
		ChannelID: string(channelID),
		Name:      name,
		MediaType: mediaType,
		SizeBytes: int64(len(params.Bytes)),
		URI:       path,
		CreatedAt: s.clock.Now(),
	}
	if err := s.conversations.SaveAttachment(ctx, attachment); err != nil {
		return conversation.Attachment{}, err
	}
	return attachment, nil
}

func (s *Service) GetConversationAttachment(ctx context.Context, attachmentID string) (ConversationAttachmentBlob, error) {
	if s.conversations == nil {
		return ConversationAttachmentBlob{}, fmt.Errorf("message conversation repository is required")
	}
	attachmentID = strings.TrimSpace(attachmentID)
	if attachmentID == "" {
		return ConversationAttachmentBlob{}, fmt.Errorf("attachment id is required")
	}
	attachments, err := s.conversations.GetAttachments(ctx, []string{attachmentID})
	if err != nil {
		return ConversationAttachmentBlob{}, fmt.Errorf("load conversation attachment: %w", err)
	}
	if len(attachments) != 1 {
		return ConversationAttachmentBlob{}, fmt.Errorf("attachment %s not found", attachmentID)
	}
	attachment := attachments[0]
	if attachment.ID != attachmentID {
		return ConversationAttachmentBlob{}, fmt.Errorf("attachment lookup returned %s for %s", attachment.ID, attachmentID)
	}
	if _, ok := allowedConversationAttachmentMediaTypes[attachment.MediaType]; !ok {
		return ConversationAttachmentBlob{}, fmt.Errorf("unsupported attachment media type %q", attachment.MediaType)
	}
	if strings.TrimSpace(attachment.URI) == "" {
		return ConversationAttachmentBlob{}, fmt.Errorf("attachment %s uri is required", attachment.ID)
	}
	bytes, err := os.ReadFile(attachment.URI)
	if err != nil {
		return ConversationAttachmentBlob{}, fmt.Errorf("read conversation attachment: %w", err)
	}
	if len(bytes) == 0 {
		return ConversationAttachmentBlob{}, fmt.Errorf("attachment %s is empty", attachment.ID)
	}
	if len(bytes) > maxConversationAttachmentBytes {
		return ConversationAttachmentBlob{}, fmt.Errorf("attachment %s exceeds %d byte limit", attachment.ID, maxConversationAttachmentBytes)
	}
	if attachment.SizeBytes != int64(len(bytes)) {
		return ConversationAttachmentBlob{}, fmt.Errorf("attachment %s size mismatch: metadata=%d bytes file=%d bytes", attachment.ID, attachment.SizeBytes, len(bytes))
	}
	return ConversationAttachmentBlob{Attachment: attachment, Bytes: bytes}, nil
}

type SendConversationMessageParams struct {
	ChannelID     types.ChannelID
	SenderType    string
	SenderID      string
	Text          string
	AttachmentIDs []string
}

func (s *Service) SendConversationMessage(ctx context.Context, params SendConversationMessageParams) (conversation.Message, error) {
	if s.conversations == nil {
		return conversation.Message{}, fmt.Errorf("message conversation repository is required")
	}
	channelID := types.ChannelID(strings.TrimSpace(string(params.ChannelID)))
	if channelID == "" {
		return conversation.Message{}, fmt.Errorf("channel id is required")
	}
	text := strings.TrimSpace(params.Text)
	if text == "" && len(params.AttachmentIDs) == 0 {
		return conversation.Message{}, fmt.Errorf("text or attachment is required")
	}
	senderType := strings.TrimSpace(params.SenderType)
	if senderType == "" {
		return conversation.Message{}, fmt.Errorf("sender type is required")
	}
	ch, err := s.loadOrEnsureConversationChannel(ctx, channelID)
	if err != nil {
		return conversation.Message{}, fmt.Errorf("load conversation channel: %w", err)
	}
	if ch.SpaceID == "" {
		return conversation.Message{}, fmt.Errorf("channel %q has no space id", channelID)
	}
	if ch.MemberID == "" {
		return conversation.Message{}, fmt.Errorf("channel %q has no member id", channelID)
	}
	attachments, err := s.resolveConversationAttachments(ctx, channelID, string(ch.SpaceID), params.AttachmentIDs)
	if err != nil {
		return conversation.Message{}, err
	}
	msg, err := conversation.NewMessage(conversation.MessageParams{
		ID:          "conversation_" + uuid.NewString(),
		ChannelID:   string(channelID),
		SpaceID:     string(ch.SpaceID),
		MemberID:    strings.TrimSpace(ch.MemberID),
		Direction:   conversation.DirectionInbound,
		SenderType:  senderType,
		SenderID:    strings.TrimSpace(params.SenderID),
		Text:        text,
		Attachments: attachments,
		Delivery:    conversation.DeliveryQueued,
		Render:      conversation.RenderVisible,
		Now:         s.clock.Now(),
	})
	if err != nil {
		return conversation.Message{}, err
	}
	if err := s.conversations.Save(ctx, msg); err != nil {
		return conversation.Message{}, fmt.Errorf("persist conversation message: %w", err)
	}
	if err := s.notifyConversationChanged(ctx, msg); err != nil {
		return conversation.Message{}, err
	}
	if err := s.repo.RecordActivity(ctx, channelID, msg.CreatedAt); err != nil {
		return conversation.Message{}, fmt.Errorf("record conversation channel activity: %w", err)
	}
	s.logInfo("conversation message persisted",
		"message_id", msg.ID,
		"channel_id", msg.ChannelID,
		"space_id", msg.SpaceID,
		"member_id", msg.MemberID,
	)
	if s.harnessChatSender == nil {
		failed, updateErr := s.conversations.UpdateDeliveryBinding(ctx, msg.ID, conversation.DeliveryFailed, "", "", "harness chat sender is required", s.clock.Now())
		if updateErr != nil {
			return conversation.Message{}, fmt.Errorf("mark conversation message failed: %w", updateErr)
		}
		if notifyErr := s.notifyConversationChanged(ctx, failed); notifyErr != nil {
			return conversation.Message{}, notifyErr
		}
		return failed, fmt.Errorf("harness chat sender is required")
	}
	stream := s.conversationStream(msg)
	result, err := s.harnessChatSender.SendMessage(ctx, HarnessChatMessage{
		SpaceID:               msg.SpaceID,
		MemberID:              msg.MemberID,
		ChannelID:             msg.ChannelID,
		ConversationMessageID: msg.ID,
		SenderType:            msg.SenderType,
		SenderID:              msg.SenderID,
		Text:                  msg.Text,
		Attachments:           append([]conversation.Attachment(nil), msg.Attachments...),
		Stream:                stream,
	})
	if err != nil {
		if stream.HasProgress() {
			result := stream.DeliveryResult()
			delivered, updateErr := s.conversations.UpdateDeliveryBinding(ctx, msg.ID, conversation.DeliveryDelivered, result.SessionID, result.TurnID, "", s.clock.Now())
			if updateErr != nil {
				return conversation.Message{}, fmt.Errorf("mark conversation message delivered after streamed response: %w", updateErr)
			}
			if notifyErr := s.notifyConversationChanged(ctx, delivered); notifyErr != nil {
				return conversation.Message{}, notifyErr
			}
			if bindErr := stream.BindResult(ctx, result); bindErr != nil {
				return conversation.Message{}, bindErr
			}
			s.logInfo("conversation harness send completed after streamed response",
				"message_id", msg.ID,
				"channel_id", msg.ChannelID,
				"space_id", msg.SpaceID,
				"member_id", msg.MemberID,
				"session_id", result.SessionID,
				"turn_id", result.TurnID,
				"post_stream_error", err,
			)
			return delivered, nil
		}
		s.logError("conversation harness send failed",
			"message_id", msg.ID,
			"channel_id", msg.ChannelID,
			"space_id", msg.SpaceID,
			"member_id", msg.MemberID,
			"error", err,
		)
		if isActiveRunDeliveryError(err) {
			s.logInfo("conversation message remains queued behind active run",
				"message_id", msg.ID,
				"channel_id", msg.ChannelID,
				"space_id", msg.SpaceID,
				"member_id", msg.MemberID,
			)
			return msg, nil
		}
		failed, updateErr := s.conversations.UpdateDeliveryBinding(ctx, msg.ID, conversation.DeliveryFailed, "", "", err.Error(), s.clock.Now())
		if updateErr != nil {
			return conversation.Message{}, fmt.Errorf("mark conversation message failed: %w", updateErr)
		}
		if notifyErr := s.notifyConversationChanged(ctx, failed); notifyErr != nil {
			return conversation.Message{}, notifyErr
		}
		return failed, fmt.Errorf("send conversation message to harness: %w", err)
	}
	delivery, err := conversationDeliveryFromHarness(result.Delivery)
	if err != nil {
		failed, updateErr := s.conversations.UpdateDeliveryBinding(ctx, msg.ID, conversation.DeliveryFailed, result.SessionID, result.TurnID, err.Error(), s.clock.Now())
		if updateErr != nil {
			return conversation.Message{}, fmt.Errorf("mark conversation message failed: %w", updateErr)
		}
		if notifyErr := s.notifyConversationChanged(ctx, failed); notifyErr != nil {
			return conversation.Message{}, notifyErr
		}
		return failed, err
	}
	delivered, err := s.conversations.UpdateDeliveryBinding(ctx, msg.ID, delivery, result.SessionID, result.TurnID, "", s.clock.Now())
	if err != nil {
		return conversation.Message{}, err
	}
	if err := s.notifyConversationChanged(ctx, delivered); err != nil {
		return conversation.Message{}, err
	}
	if stream.message != nil {
		if err := stream.BindResult(ctx, result); err != nil {
			return conversation.Message{}, err
		}
	}
	s.logInfo("conversation message delivered to harness",
		"message_id", msg.ID,
		"channel_id", msg.ChannelID,
		"space_id", msg.SpaceID,
		"member_id", msg.MemberID,
		"session_id", result.SessionID,
		"turn_id", result.TurnID,
		"delivery", result.Delivery,
	)
	if strings.TrimSpace(result.Text) != "" && stream.message == nil {
		if _, err := s.saveOutboundConversationMessage(ctx, msg, result); err != nil {
			return conversation.Message{}, err
		}
	}
	return delivered, nil
}

func (s *Service) SteerConversationMessage(ctx context.Context, messageID string) (conversation.Message, error) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return conversation.Message{}, fmt.Errorf("message id is required")
	}
	if s.conversations == nil {
		return conversation.Message{}, fmt.Errorf("message conversation repository is required")
	}
	if s.harnessChatSender == nil {
		return conversation.Message{}, fmt.Errorf("harness chat sender is required")
	}
	msg, err := s.conversations.Get(ctx, messageID)
	if err != nil {
		return conversation.Message{}, err
	}
	if msg == nil {
		return conversation.Message{}, fmt.Errorf("conversation message %q not found", messageID)
	}
	if msg.Direction != conversation.DirectionInbound {
		return conversation.Message{}, fmt.Errorf("conversation message %q is not inbound", messageID)
	}
	if strings.TrimSpace(msg.SenderType) != "user" {
		return conversation.Message{}, fmt.Errorf("conversation message %q was not sent by a user", messageID)
	}
	if msg.Delivery != conversation.DeliveryQueued {
		return conversation.Message{}, fmt.Errorf("conversation message %q is %q, not queued", messageID, msg.Delivery)
	}
	stream := s.conversationStream(*msg)
	result, err := s.harnessChatSender.SendMessage(ctx, HarnessChatMessage{
		SpaceID:               msg.SpaceID,
		MemberID:              msg.MemberID,
		ChannelID:             msg.ChannelID,
		ConversationMessageID: msg.ID,
		SenderType:            msg.SenderType,
		SenderID:              msg.SenderID,
		Text:                  msg.Text,
		Attachments:           append([]conversation.Attachment(nil), msg.Attachments...),
		AllowSteering:         true,
		Stream:                stream,
	})
	if err != nil {
		return conversation.Message{}, fmt.Errorf("steer conversation message to harness: %w", err)
	}
	delivery, err := conversationDeliveryFromHarness(result.Delivery)
	if err != nil {
		return conversation.Message{}, err
	}
	if delivery != conversation.DeliverySteered {
		return conversation.Message{}, fmt.Errorf("harness delivery %q is not steering confirmation", result.Delivery)
	}
	steered, err := s.conversations.UpdateDeliveryBinding(ctx, msg.ID, delivery, result.SessionID, result.TurnID, "", s.clock.Now())
	if err != nil {
		return conversation.Message{}, err
	}
	if err := s.notifyConversationChanged(ctx, steered); err != nil {
		return conversation.Message{}, err
	}
	if stream.message != nil {
		if err := stream.BindResult(ctx, result); err != nil {
			return conversation.Message{}, err
		}
	}
	return steered, nil
}

func (s *Service) resolveConversationAttachments(ctx context.Context, channelID types.ChannelID, spaceID string, ids []string) ([]conversation.Attachment, error) {
	cleaned := cleanAttachmentIDs(ids)
	if len(cleaned) == 0 {
		return nil, nil
	}
	attachments, err := s.conversations.GetAttachments(ctx, cleaned)
	if err != nil {
		return nil, fmt.Errorf("load conversation attachments: %w", err)
	}
	for _, attachment := range attachments {
		if attachment.ChannelID != string(channelID) {
			return nil, fmt.Errorf("attachment %s belongs to channel %s, not %s", attachment.ID, attachment.ChannelID, channelID)
		}
		if attachment.SpaceID != strings.TrimSpace(spaceID) {
			return nil, fmt.Errorf("attachment %s belongs to space %s, not %s", attachment.ID, attachment.SpaceID, spaceID)
		}
	}
	return attachments, nil
}

func cleanAttachmentIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	seen := map[string]struct{}{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func safeAttachmentName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = unsafeAttachmentNameChars.ReplaceAllString(name, "-")
	return strings.Trim(name, ".-")
}

func sanitizePathSegment(value string) string {
	value = unsafeAttachmentNameChars.ReplaceAllString(strings.TrimSpace(value), "-")
	value = strings.Trim(value, ".-")
	if value == "" {
		return "attachments"
	}
	return value
}

func attachmentStoreBase(root string) string {
	root = strings.TrimSpace(root)
	if filepath.Base(root) == ".agen8" {
		return root
	}
	return filepath.Join(root, ".agen8")
}

type conversationStreamSink struct {
	service    *Service
	inbound    conversation.Message
	message    *conversation.Message
	activities map[string]conversation.Activity
	session    string
	turn       string
	order      int
	segmented  bool
}

func (s *Service) conversationStream(inbound conversation.Message) *conversationStreamSink {
	return &conversationStreamSink{service: s, inbound: inbound, activities: make(map[string]conversation.Activity)}
}

func (s *conversationStreamSink) AppendAssistantDelta(ctx context.Context, delta HarnessAssistantDelta) error {
	if s == nil || s.service == nil {
		return fmt.Errorf("conversation stream sink is required")
	}
	text := delta.Text
	if text == "" {
		return fmt.Errorf("assistant delta text is required")
	}
	if strings.TrimSpace(delta.SessionID) != "" {
		s.session = strings.TrimSpace(delta.SessionID)
	}
	if strings.TrimSpace(delta.TurnID) != "" {
		s.turn = strings.TrimSpace(delta.TurnID)
	}
	if delta.Sequence > s.order {
		s.order = delta.Sequence
	} else {
		s.order++
	}
	if s.message == nil || s.segmented {
		msg, err := s.service.saveOutboundConversationMessage(ctx, s.inbound, HarnessChatResult{
			SessionID: s.session,
			TurnID:    s.turn,
			Text:      text,
		}, s.streamTime())
		if err != nil {
			return err
		}
		s.message = &msg
		s.segmented = false
		return nil
	}
	updated, err := s.service.conversations.AppendText(ctx, s.message.ID, text, s.streamTime())
	if err != nil {
		return err
	}
	if err := s.service.notifyConversationChanged(ctx, updated); err != nil {
		return err
	}
	s.message = &updated
	return nil
}

func (s *conversationStreamSink) AppendThinkingDelta(ctx context.Context, delta HarnessThinkingDelta) error {
	if s == nil || s.service == nil {
		return fmt.Errorf("conversation stream sink is required")
	}
	text := delta.Text
	if text == "" {
		return fmt.Errorf("thinking delta text is required")
	}
	sessionID := strings.TrimSpace(delta.SessionID)
	turnID := strings.TrimSpace(delta.TurnID)
	if sessionID == "" {
		return fmt.Errorf("thinking delta sessionID is required")
	}
	if turnID == "" {
		return fmt.Errorf("thinking delta turnID is required")
	}
	if delta.Sequence > s.order {
		s.order = delta.Sequence
	} else {
		s.order++
	}
	now := s.streamTime()
	sequence := delta.Sequence
	if sequence <= 0 {
		sequence = s.order
	}
	data := make(map[string]string, len(delta.Data)+7)
	for k, v := range delta.Data {
		if strings.TrimSpace(k) != "" {
			data[k] = v
		}
	}
	data["channelId"] = s.inbound.ChannelID
	data["spaceId"] = s.inbound.SpaceID
	data["memberId"] = s.inbound.MemberID
	data["sessionId"] = sessionID
	data["turnId"] = turnID
	data["seq"] = fmt.Sprintf("%d", sequence)
	thinkingID := strings.TrimSpace(data["itemId"])
	if thinkingID == "" {
		thinkingID = fmt.Sprintf("seq-%d", sequence)
	}
	toolCallID := "thinking-" + thinkingID
	existing, hasExisting := s.activities[toolCallID]
	if hasExisting {
		for k, v := range existing.Data {
			if strings.TrimSpace(k) != "" && strings.TrimSpace(data[k]) == "" {
				data[k] = v
			}
		}
		switch {
		case strings.HasPrefix(text, existing.Text):
			// The harness can emit a final full summary after streaming deltas.
		case strings.HasPrefix(existing.Text, text):
			text = existing.Text
		default:
			text = existing.Text + text
		}
		sequence = existing.Sequence
		data["seq"] = fmt.Sprintf("%d", sequence)
	}
	completedAt := now
	entry := conversation.Activity{
		ID:          conversationActivityID(sessionID, turnID, toolCallID),
		ChannelID:   s.inbound.ChannelID,
		SpaceID:     s.inbound.SpaceID,
		MemberID:    s.inbound.MemberID,
		SessionID:   sessionID,
		TurnID:      turnID,
		ToolCallID:  toolCallID,
		Sequence:    sequence,
		Kind:        "thinking",
		Title:       "Thinking",
		Status:      "completed",
		Text:        text,
		CreatedAt:   now,
		CompletedAt: &completedAt,
		Data:        data,
	}
	if hasExisting {
		entry.CreatedAt = existing.CreatedAt
	}
	if err := s.service.conversations.SaveActivity(ctx, entry); err != nil {
		return err
	}
	s.activities[toolCallID] = entry
	if err := s.service.notifyConversationChanged(ctx, s.inbound); err != nil {
		return err
	}
	return s.service.repo.RecordActivity(ctx, types.ChannelID(s.inbound.ChannelID), now)
}

func (s *conversationStreamSink) AppendActivity(ctx context.Context, activity HarnessActivity) error {
	if s == nil || s.service == nil {
		return fmt.Errorf("conversation stream sink is required")
	}
	sessionID := strings.TrimSpace(activity.SessionID)
	turnID := strings.TrimSpace(activity.TurnID)
	toolCallID := strings.TrimSpace(activity.ToolCallID)
	toolName := strings.TrimSpace(activity.ToolName)
	if sessionID == "" {
		return fmt.Errorf("activity sessionID is required")
	}
	if turnID == "" {
		return fmt.Errorf("activity turnID is required")
	}
	if toolCallID == "" {
		return fmt.Errorf("activity toolCallID is required")
	}
	if activity.Sequence > s.order {
		s.order = activity.Sequence
	} else {
		s.order++
	}
	s.segmented = s.message != nil
	existing, hasExisting := s.activities[toolCallID]
	if toolName == "" && hasExisting {
		toolName = existing.Title
	}
	if toolName == "" {
		return fmt.Errorf("activity toolName is required")
	}
	status := normalizeActivityStatus(activity.Status)
	now := s.streamTime()
	sequence := activity.Sequence
	if sequence <= 0 {
		sequence = s.order
	}
	data := make(map[string]string, len(activity.Data)+8)
	if hasExisting {
		for k, v := range existing.Data {
			if strings.TrimSpace(k) != "" {
				data[k] = v
			}
		}
	}
	for k, v := range activity.Data {
		if strings.TrimSpace(k) != "" {
			data[k] = v
		}
	}
	if hasExisting && activityDataBool(activity.Data, "outputDelta") {
		appendActivityOutputDelta(data, existing.Data, activity)
	}
	data["channelId"] = s.inbound.ChannelID
	data["spaceId"] = s.inbound.SpaceID
	data["memberId"] = s.inbound.MemberID
	data["sessionId"] = sessionID
	data["turnId"] = turnID
	data["toolCallId"] = toolCallID
	data["toolName"] = toolName
	data["seq"] = fmt.Sprintf("%d", sequence)
	if strings.TrimSpace(data["sourceType"]) == "" {
		data["sourceType"] = inferActivitySourceType(toolName, data)
	}
	if strings.TrimSpace(data["server"]) == "" {
		if server := inferActivityServer(toolName); server != "" {
			data["server"] = server
		}
	}
	if strings.TrimSpace(data["domain"]) == "" && data["server"] == "agen8" {
		if domain := inferAgen8ToolDomain(toolName); domain != "" {
			data["domain"] = domain
		}
	}
	if strings.TrimSpace(activity.Text) != "" && strings.TrimSpace(data["outputPreview"]) == "" {
		data["outputPreview"] = strings.TrimSpace(activity.Text)
	}
	if strings.TrimSpace(activity.Text) != "" && strings.TrimSpace(data["result"]) == "" {
		data["result"] = strings.TrimSpace(activity.Text)
	}
	var completedAt *time.Time
	if status == "completed" || status == "failed" || status == "error" {
		completedAt = &now
	}
	entry := conversation.Activity{
		ID:          conversationActivityID(sessionID, turnID, toolCallID),
		ChannelID:   s.inbound.ChannelID,
		SpaceID:     s.inbound.SpaceID,
		MemberID:    s.inbound.MemberID,
		SessionID:   sessionID,
		TurnID:      turnID,
		ToolCallID:  toolCallID,
		Sequence:    sequence,
		Kind:        "tool_call",
		Title:       toolName,
		Status:      status,
		Text:        strings.TrimSpace(activity.Text),
		CreatedAt:   now,
		CompletedAt: completedAt,
		Data:        data,
	}
	if hasExisting {
		entry.CreatedAt = existing.CreatedAt
		entry.Sequence = existing.Sequence
		entry.Data["seq"] = fmt.Sprintf("%d", existing.Sequence)
	}
	if err := s.service.conversations.SaveActivity(ctx, entry); err != nil {
		return err
	}
	s.activities[toolCallID] = entry
	if err := s.service.notifyConversationChanged(ctx, s.inbound); err != nil {
		return err
	}
	return s.service.repo.RecordActivity(ctx, types.ChannelID(s.inbound.ChannelID), now)
}

func activityDataBool(data map[string]string, key string) bool {
	if data == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(data[key])) {
	case "true", "1", "yes":
		return true
	default:
		return false
	}
}

func appendActivityOutputDelta(data, existing map[string]string, activity HarnessActivity) {
	delta := firstNonEmptyActivityString(
		activity.Data["outputFull"],
		activity.Data["stdout"],
		activity.Data["result"],
		activity.Data["outputPreview"],
		activity.Text,
	)
	if delta == "" {
		return
	}
	full := existing["outputFull"] + delta
	data["outputFull"] = full
	data["stdout"] = existing["stdout"] + delta
	data["result"] = full
	data["outputPreview"] = full
}

func firstNonEmptyActivityString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func conversationActivityID(sessionID, turnID, toolCallID string) string {
	raw := strings.NewReplacer(" ", "_", "/", "_", ":", "_").Replace(sessionID + "_" + turnID + "_" + toolCallID)
	return "conversation_activity_" + raw
}

func (s *conversationStreamSink) streamTime() time.Time {
	if s == nil || s.service == nil {
		return time.Time{}
	}
	if s.order <= 0 {
		s.order = 1
	}
	return s.inbound.CreatedAt.Add(time.Duration(s.order) * time.Millisecond)
}

func normalizeActivityStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "success", "succeeded", "ok", "done":
		return "completed"
	case "failed", "error":
		return "failed"
	case "running", "in_progress", "pending", "started", "streaming", "":
		return "pending"
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}

func inferActivitySourceType(toolName string, data map[string]string) string {
	if source := strings.TrimSpace(data["sourceType"]); source != "" {
		return source
	}
	if strings.Contains(toolName, "/") || strings.HasPrefix(toolName, "mcp__") {
		return "mcp"
	}
	if toolName == "bash" || toolName == "shell_exec" || toolName == "shell_command" {
		return "cli"
	}
	return "native"
}

func inferActivityServer(toolName string) string {
	toolName = strings.TrimSpace(toolName)
	if strings.Contains(toolName, "/") {
		return strings.TrimSpace(strings.SplitN(toolName, "/", 2)[0])
	}
	if strings.HasPrefix(toolName, "mcp__") {
		parts := strings.Split(toolName, "__")
		if len(parts) >= 3 {
			return strings.TrimSpace(parts[1])
		}
	}
	return ""
}

func inferAgen8ToolDomain(toolName string) string {
	name := strings.TrimSpace(toolName)
	if strings.Contains(name, "/") {
		parts := strings.SplitN(name, "/", 2)
		name = parts[1]
	}
	if strings.HasPrefix(name, "mcp__") {
		parts := strings.Split(name, "__")
		name = parts[len(parts)-1]
	}
	name = strings.TrimPrefix(name, "agen8_")
	name = strings.TrimPrefix(name, "agen8-")
	if idx := strings.IndexAny(name, "._-"); idx > 0 {
		return name[:idx]
	}
	return name
}

func (s *conversationStreamSink) BindResult(ctx context.Context, result HarnessChatResult) error {
	if s == nil || s.service == nil || s.message == nil {
		return nil
	}
	if strings.TrimSpace(result.SessionID) != "" {
		s.message.SessionID = strings.TrimSpace(result.SessionID)
	}
	if strings.TrimSpace(result.TurnID) != "" {
		s.message.TurnID = strings.TrimSpace(result.TurnID)
	}
	s.message.UpdatedAt = s.service.nextConversationTime(s.inbound.CreatedAt)
	if err := s.service.conversations.Save(ctx, *s.message); err != nil {
		return fmt.Errorf("bind streamed outbound conversation message: %w", err)
	}
	if err := s.service.notifyConversationChanged(ctx, *s.message); err != nil {
		return err
	}
	return nil
}

func (s *conversationStreamSink) HasProgress() bool {
	return s != nil && (s.message != nil || len(s.activities) > 0)
}

func (s *conversationStreamSink) DeliveryResult() HarnessChatResult {
	if s == nil {
		return HarnessChatResult{Delivery: string(conversation.DeliveryDelivered)}
	}
	result := HarnessChatResult{
		SessionID: s.session,
		TurnID:    s.turn,
		Delivery:  string(conversation.DeliveryDelivered),
	}
	if s.message != nil {
		if strings.TrimSpace(result.SessionID) == "" {
			result.SessionID = s.message.SessionID
		}
		if strings.TrimSpace(result.TurnID) == "" {
			result.TurnID = s.message.TurnID
		}
		result.Text = s.message.Text
	}
	return result
}

func (s *Service) saveOutboundConversationMessage(ctx context.Context, inbound conversation.Message, result HarnessChatResult, at ...time.Time) (conversation.Message, error) {
	now := s.nextConversationTime(inbound.CreatedAt)
	if len(at) > 0 && !at[0].IsZero() {
		now = at[0]
	}
	outbound, err := conversation.NewMessage(conversation.MessageParams{
		ID:         "conversation_" + uuid.NewString(),
		ChannelID:  inbound.ChannelID,
		SpaceID:    inbound.SpaceID,
		MemberID:   inbound.MemberID,
		SessionID:  strings.TrimSpace(result.SessionID),
		TurnID:     strings.TrimSpace(result.TurnID),
		Direction:  conversation.DirectionOutbound,
		SenderType: "harness",
		SenderID:   inbound.MemberID,
		Text:       result.Text,
		Render:     conversation.RenderVisible,
		Now:        now,
	})
	if err != nil {
		return conversation.Message{}, err
	}
	if err := s.conversations.Save(ctx, outbound); err != nil {
		return conversation.Message{}, fmt.Errorf("persist outbound conversation message: %w", err)
	}
	if err := s.notifyConversationChanged(ctx, outbound); err != nil {
		return conversation.Message{}, err
	}
	if err := s.repo.RecordActivity(ctx, types.ChannelID(outbound.ChannelID), outbound.CreatedAt); err != nil {
		return conversation.Message{}, fmt.Errorf("record outbound conversation channel activity: %w", err)
	}
	s.logInfo("conversation harness response persisted",
		"message_id", outbound.ID,
		"channel_id", outbound.ChannelID,
		"space_id", outbound.SpaceID,
		"member_id", outbound.MemberID,
		"session_id", outbound.SessionID,
		"turn_id", outbound.TurnID,
	)
	return outbound, nil
}

func (s *Service) notifyConversationChanged(ctx context.Context, msg conversation.Message) error {
	if s.conversationNotify == nil {
		return nil
	}
	if err := s.conversationNotify.NotifyConversationChanged(ctx, msg); err != nil {
		return fmt.Errorf("notify conversation changed: %w", err)
	}
	return nil
}

func (s *Service) nextConversationTime(after time.Time) time.Time {
	now := s.clock.Now()
	if !now.After(after) {
		now = after.Add(1)
	}
	return now
}

func (s *Service) loadOrEnsureConversationChannel(ctx context.Context, channelID types.ChannelID) (types.Channel, error) {
	ch, err := s.LoadChannel(ctx, channelID)
	if err == nil {
		return ch, nil
	}
	if !errors.Is(err, channel.ErrNotFound) {
		return types.Channel{}, err
	}
	spaceID, memberID, ok := parseMemberChannelID(channelID)
	if !ok {
		return types.Channel{}, err
	}
	ensured, ensureErr := s.EnsureMemberChannel(ctx, NewMemberChannelParams{
		SpaceID:  spaceID,
		MemberID: memberID,
	})
	if ensureErr != nil {
		return types.Channel{}, ensureErr
	}
	s.logInfo("conversation member channel ensured",
		"channel_id", ensured.ID,
		"space_id", ensured.SpaceID,
		"member_id", ensured.MemberID,
	)
	return ensured, nil
}

func parseMemberChannelID(channelID types.ChannelID) (spacedomain.SpaceID, member.ID, bool) {
	raw := strings.TrimSpace(string(channelID))
	const prefix = "channel:"
	const marker = ":member:"
	if !strings.HasPrefix(raw, prefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(raw, prefix)
	parts := strings.Split(rest, marker)
	if len(parts) != 2 {
		return "", "", false
	}
	spaceID := spacedomain.SpaceID(strings.TrimSpace(parts[0]))
	memberID := member.ID(strings.TrimSpace(parts[1]))
	return spaceID, memberID, spaceID != "" && memberID != ""
}

func (s *Service) logInfo(msg string, args ...any) {
	if s != nil && s.logger != nil {
		s.logger.Info(msg, args...)
	}
}

func (s *Service) logError(msg string, args ...any) {
	if s != nil && s.logger != nil {
		s.logger.Error(msg, args...)
	}
}

func conversationDeliveryFromHarness(delivery string) (conversation.DeliveryState, error) {
	switch conversation.DeliveryState(strings.TrimSpace(delivery)) {
	case conversation.DeliveryDelivered:
		return conversation.DeliveryDelivered, nil
	case conversation.DeliverySteered:
		return conversation.DeliverySteered, nil
	case "":
		return "", fmt.Errorf("harness delivery is required")
	default:
		return "", fmt.Errorf("unsupported harness delivery %q", delivery)
	}
}

func (s *Service) ListConversationMessages(ctx context.Context, channelID types.ChannelID, limit int) ([]conversation.Message, error) {
	if s.conversations == nil {
		return nil, fmt.Errorf("message conversation repository is required")
	}
	channelID = types.ChannelID(strings.TrimSpace(string(channelID)))
	if channelID == "" {
		return nil, fmt.Errorf("channel id is required")
	}
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be greater than zero")
	}
	return s.conversations.ListByChannel(ctx, string(channelID), limit)
}

func (s *Service) ListConversationActivities(ctx context.Context, channelID types.ChannelID, limit int) ([]conversation.Activity, error) {
	if s.conversations == nil {
		return nil, fmt.Errorf("message conversation repository is required")
	}
	channelID = types.ChannelID(strings.TrimSpace(string(channelID)))
	if channelID == "" {
		return nil, fmt.Errorf("channel id is required")
	}
	if limit <= 0 {
		limit = 200
	}
	return s.conversations.ListActivitiesByChannel(ctx, string(channelID), limit)
}

// PublishAgentMessage validates, persists, and wakes a destination member for an agent inbox message.
func (s *Service) PublishAgentMessage(ctx context.Context, input domain.NewMessageInput) (types.AgentMessage, error) {
	msg, err := domain.NewMessage(input, s.clock.Now())
	if err != nil {
		return types.AgentMessage{}, err
	}
	if err := s.ensurePublishChannel(ctx, msg.Inner()); err != nil {
		return types.AgentMessage{}, err
	}
	saved, err := s.repo.SaveQueued(ctx, msg.Inner())
	if err != nil {
		return types.AgentMessage{}, fmt.Errorf("publish agent message: %w", err)
	}
	if saved.ChannelID != "" {
		if err := s.repo.RecordActivity(ctx, saved.ChannelID, saved.CreatedAt); err != nil {
			return types.AgentMessage{}, fmt.Errorf("record message channel activity: %w", err)
		}
	}
	if err := s.ensureAgentDelivery(ctx, saved.DestinationMemberID); err != nil {
		return types.AgentMessage{}, err
	}
	s.scheduleMemberWake(saved)
	s.logInfo("agent message queued",
		"message_id", saved.ID,
		"space_id", saved.SpaceID,
		"source_member_id", saved.SourceMemberID,
		"destination_member_id", saved.DestinationMemberID,
		"channel_id", saved.ChannelID,
		"kind", saved.Kind,
		"producer", saved.Producer,
		"correlation_id", saved.CorrelationID,
		"visible_at", saved.VisibleAt,
	)
	return saved, nil
}

func (s *Service) ensureAgentDelivery(ctx context.Context, memberID member.ID) error {
	if s == nil || !s.autoStartDelivery || s.harnessChatSender == nil || s.conversations == nil {
		return nil
	}
	memberID = trimMemberID(memberID)
	if memberID == "" {
		return fmt.Errorf("member id is required")
	}
	if err := s.StartAgentDelivery(context.WithoutCancel(ctx), memberID); err != nil {
		return fmt.Errorf("start message delivery for member %s: %w", memberID, err)
	}
	return nil
}

// ReceiveNextForDelivery returns the next visible queued message for runtime delivery.
func (s *Service) ReceiveNextForDelivery(ctx context.Context, memberID member.ID) (types.AgentMessage, error) {
	memberID = trimMemberID(memberID)
	if memberID == "" {
		return types.AgentMessage{}, fmt.Errorf("member id is required")
	}
	return s.repo.NextQueuedForMember(ctx, memberID, s.clock.Now())
}

// RecordDelivered marks a message consumed after the runtime has handed it to the destination agent.
func (s *Service) RecordDelivered(ctx context.Context, messageID types.AgentMessageID, consumerID member.ID) (types.AgentMessage, error) {
	messageID = types.AgentMessageID(strings.TrimSpace(string(messageID)))
	if messageID == "" {
		return types.AgentMessage{}, fmt.Errorf("message id is required")
	}
	queued, err := s.repo.Get(ctx, messageID)
	if err != nil {
		return types.AgentMessage{}, err
	}
	consumed, err := domain.WrapMessage(queued).Consume(consumerID, s.clock.Now())
	if err != nil {
		return types.AgentMessage{}, err
	}
	saved, err := s.repo.MarkConsumed(ctx, consumed.Inner())
	if err != nil {
		return types.AgentMessage{}, err
	}
	s.logInfo("agent message consumed",
		"message_id", saved.ID,
		"space_id", saved.SpaceID,
		"source_member_id", saved.SourceMemberID,
		"destination_member_id", saved.DestinationMemberID,
		"consumed_by", saved.ConsumedBy,
		"channel_id", saved.ChannelID,
		"kind", saved.Kind,
		"producer", saved.Producer,
		"correlation_id", saved.CorrelationID,
	)
	return saved, nil
}

// GetMessage loads a persisted agent inbox message by id.
func (s *Service) GetMessage(ctx context.Context, id types.AgentMessageID) (types.AgentMessage, error) {
	id = types.AgentMessageID(strings.TrimSpace(string(id)))
	if id == "" {
		return types.AgentMessage{}, fmt.Errorf("message id is required")
	}
	return s.repo.Get(ctx, id)
}

// ListMessages returns persisted agent inbox messages matching the filter.
func (s *Service) ListMessages(ctx context.Context, filter domain.MessageFilter) ([]types.AgentMessage, error) {
	return s.repo.List(ctx, filter)
}

// CountMessages counts persisted agent inbox messages matching the filter.
func (s *Service) CountMessages(ctx context.Context, filter domain.MessageFilter) (int, error) {
	return s.repo.Count(ctx, filter)
}

// SubscribeMemberWake returns best-effort push notifications for messages addressed to memberID.
func (s *Service) SubscribeMemberWake(memberID member.ID) (<-chan domain.MessageWake, func()) {
	memberID = trimMemberID(memberID)
	if s == nil || s.wakes == nil {
		ch := make(chan domain.MessageWake)
		close(ch)
		return ch, func() {}
	}
	return s.wakes.Subscribe(WakeFilter{MemberID: memberID})
}

// SubscribeSpaceWake returns best-effort push notifications for messages queued in a space.
func (s *Service) SubscribeSpaceWake(spaceID spacedomain.SpaceID) (<-chan domain.MessageWake, func()) {
	spaceID = trimSpaceID(spaceID)
	if s == nil || s.wakes == nil {
		ch := make(chan domain.MessageWake)
		close(ch)
		return ch, func() {}
	}
	return s.wakes.Subscribe(WakeFilter{SpaceID: spaceID})
}

func (s *Service) notifyMember(msg types.AgentMessage) {
	if s == nil || s.wakes == nil {
		return
	}
	wake := domain.MessageWake{
		MessageID:           msg.ID,
		SpaceID:             msg.SpaceID,
		DestinationMemberID: msg.DestinationMemberID,
		ChannelID:           msg.ChannelID,
		Kind:                msg.Kind,
	}
	s.wakes.Notify(wake, func(filter WakeFilter) bool {
		if filter.MemberID != "" && filter.MemberID != msg.DestinationMemberID {
			return false
		}
		if filter.SpaceID != "" && filter.SpaceID != msg.SpaceID {
			return false
		}
		return true
	})
}

func (s *Service) scheduleMemberWake(msg types.AgentMessage) {
	if s == nil || s.agentWakeQueue == nil {
		return
	}
	select {
	case s.agentWakeQueue <- msg:
	default:
		s.logError("agent message wake queue full",
			"message_id", msg.ID,
			"space_id", msg.SpaceID,
			"destination_member_id", msg.DestinationMemberID,
		)
	}
}

func (s *Service) startAgentWakeDispatcher() {
	if s == nil || s.agentWakeQueue == nil {
		return
	}
	go func() {
		for msg := range s.agentWakeQueue {
			s.notifyMember(msg)
		}
	}()
}

func (s *Service) ensurePublishChannel(ctx context.Context, msg types.AgentMessage) error {
	if msg.ChannelID == "" {
		return nil
	}
	if msg.ChannelID != channel.MemberChannelID(msg.SpaceID, msg.DestinationMemberID) {
		return nil
	}
	ch, err := channel.NewMemberChannel(channel.NewMemberInput{
		SpaceID:  msg.SpaceID,
		MemberID: msg.DestinationMemberID,
	}, s.clock.Now())
	if err != nil {
		return err
	}
	if _, err := s.repo.Save(ctx, ch.Inner()); err != nil {
		return fmt.Errorf("ensure message channel: %w", err)
	}
	return nil
}

func trimMemberID(id member.ID) member.ID {
	return member.ID(strings.TrimSpace(string(id)))
}

func trimSpaceID(id spacedomain.SpaceID) spacedomain.SpaceID {
	return spacedomain.SpaceID(strings.TrimSpace(string(id)))
}
