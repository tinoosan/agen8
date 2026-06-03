package rpc

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"

	messageapp "github.com/tinoosan/agen8-mcp-server/internal/services/message/app"
	messagedomain "github.com/tinoosan/agen8-mcp-server/internal/services/message/domain"
	messagechannel "github.com/tinoosan/agen8-mcp-server/internal/services/message/domain/channel"
	"github.com/tinoosan/agen8-mcp-server/internal/services/message/domain/conversation"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	taskdomain "github.com/tinoosan/agen8-mcp-server/internal/services/task/domain"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

type MemberLoader interface {
	GetMember(ctx context.Context, memberID member.ID) (member.Record, error)
}

type Handler struct {
	svc     *messageapp.Service
	members MemberLoader
}

func NewHandler(svc *messageapp.Service, members MemberLoader) *Handler {
	if svc == nil {
		panic("message RPC handler requires message service")
	}
	if members == nil {
		panic("message RPC handler requires member reader")
	}
	return &Handler{svc: svc, members: members}
}

func (h *Handler) ChannelList(ctx context.Context, p ChannelListParams) (ChannelListResult, error) {
	spaceID := strings.TrimSpace(p.SpaceID)
	if spaceID == "" {
		return ChannelListResult{}, invalidParams("spaceId is required")
	}
	channels, err := h.svc.ListChannelsBySpace(ctx, spacedomain.SpaceID(spaceID))
	if err != nil {
		return ChannelListResult{}, internalError("list message channels", err)
	}
	ids := make([]types.ChannelID, 0, len(channels))
	for _, ch := range channels {
		ids = append(ids, ch.ID)
	}
	counts, err := h.svc.UnreadCountsByChannel(ctx, userIDFromContext(ctx), ids)
	if err != nil {
		return ChannelListResult{}, internalError("count unread message channels", err)
	}
	views := make([]ChannelView, 0, len(channels))
	for _, ch := range channels {
		view, err := h.channelView(ctx, ch, counts[ch.ID])
		if err != nil {
			return ChannelListResult{}, err
		}
		views = append(views, view)
	}
	return ChannelListResult{Channels: views}, nil
}

func (h *Handler) ChannelGet(ctx context.Context, p ChannelGetParams) (ChannelGetResult, error) {
	channelID := strings.TrimSpace(p.ChannelID)
	if channelID == "" {
		return ChannelGetResult{}, invalidParams("channelId is required")
	}
	ch, err := h.svc.LoadChannel(ctx, types.ChannelID(channelID))
	if err != nil {
		return ChannelGetResult{}, internalError("get message channel", err)
	}
	counts, err := h.svc.UnreadCountsByChannel(ctx, userIDFromContext(ctx), []types.ChannelID{ch.ID})
	if err != nil {
		return ChannelGetResult{}, internalError("count unread message channel", err)
	}
	view, err := h.channelView(ctx, ch, counts[ch.ID])
	if err != nil {
		return ChannelGetResult{}, err
	}
	return ChannelGetResult{Channel: view}, nil
}

func (h *Handler) ChannelEnsureMember(ctx context.Context, p ChannelEnsureMemberParams) (ChannelEnsureMemberResult, error) {
	spaceID := strings.TrimSpace(p.SpaceID)
	memberID := strings.TrimSpace(p.MemberID)
	if spaceID == "" {
		return ChannelEnsureMemberResult{}, invalidParams("spaceId is required")
	}
	if memberID == "" {
		return ChannelEnsureMemberResult{}, invalidParams("memberId is required")
	}
	ch, err := h.svc.EnsureMemberChannel(ctx, messageapp.NewMemberChannelParams{
		SpaceID:   spacedomain.SpaceID(spaceID),
		ProjectID: types.ProjectID(strings.TrimSpace(p.ProjectID)),
		MemberID:  member.ID(memberID),
		Status:    types.ChannelStatus(strings.TrimSpace(p.Status)),
	})
	if err != nil {
		return ChannelEnsureMemberResult{}, internalError("ensure member message channel", err)
	}
	view, err := h.channelView(ctx, ch, 0)
	if err != nil {
		return ChannelEnsureMemberResult{}, err
	}
	return ChannelEnsureMemberResult{Channel: view}, nil
}

func (h *Handler) ChannelMarkRead(ctx context.Context, p ChannelMarkReadParams) (struct{}, error) {
	channelID := strings.TrimSpace(p.ChannelID)
	if channelID == "" {
		return struct{}{}, invalidParams("channelId is required")
	}
	if err := h.svc.MarkChannelRead(ctx, userIDFromContext(ctx), types.ChannelID(channelID)); err != nil {
		return struct{}{}, internalError("mark message channel read", err)
	}
	return struct{}{}, nil
}

func (h *Handler) ChannelUnreadCounts(ctx context.Context, p ChannelUnreadCountsParams) (ChannelUnreadCountsResult, error) {
	ids := parseChannelIDs(p.ChannelIDs)
	if len(ids) == 0 {
		return ChannelUnreadCountsResult{}, invalidParams("channelIds is required")
	}
	counts, err := h.svc.UnreadCountsByChannel(ctx, userIDFromContext(ctx), ids)
	if err != nil {
		return ChannelUnreadCountsResult{}, internalError("count unread message channels", err)
	}
	out := make(map[string]int, len(counts))
	for id, count := range counts {
		out[string(id)] = count
	}
	return ChannelUnreadCountsResult{Counts: out}, nil
}

func (h *Handler) MessageList(ctx context.Context, p MessageListParams) (MessageListResult, error) {
	filter, err := messageFilter(p)
	if err != nil {
		return MessageListResult{}, err
	}
	messages, err := h.svc.ListMessages(ctx, filter)
	if err != nil {
		return MessageListResult{}, internalError("list messages", err)
	}
	count, err := h.svc.CountMessages(ctx, filter)
	if err != nil {
		return MessageListResult{}, internalError("count messages", err)
	}
	views := make([]MessageView, 0, len(messages))
	for _, msg := range messages {
		view, err := h.messageView(ctx, msg)
		if err != nil {
			return MessageListResult{}, err
		}
		views = append(views, view)
	}
	return MessageListResult{Messages: views, TotalCount: count}, nil
}

func (h *Handler) MessageGet(ctx context.Context, p MessageGetParams) (MessageGetResult, error) {
	messageID := strings.TrimSpace(p.MessageID)
	if messageID == "" {
		return MessageGetResult{}, invalidParams("messageId is required")
	}
	msg, err := h.svc.GetMessage(ctx, types.AgentMessageID(messageID))
	if err != nil {
		return MessageGetResult{}, internalError("get message", err)
	}
	view, err := h.messageView(ctx, msg)
	if err != nil {
		return MessageGetResult{}, err
	}
	return MessageGetResult{Message: view}, nil
}

func (h *Handler) MessageCount(ctx context.Context, p MessageCountParams) (MessageCountResult, error) {
	filter, err := messageFilter(MessageListParams(p))
	if err != nil {
		return MessageCountResult{}, err
	}
	count, err := h.svc.CountMessages(ctx, filter)
	if err != nil {
		return MessageCountResult{}, internalError("count messages", err)
	}
	return MessageCountResult{TotalCount: count}, nil
}

func (h *Handler) MessageDeliveryReceiveNext(ctx context.Context, p MessageDeliveryReceiveNextParams) (MessageDeliveryReceiveNextResult, error) {
	memberID := strings.TrimSpace(p.MemberID)
	if memberID == "" {
		return MessageDeliveryReceiveNextResult{}, invalidParams("memberId is required")
	}
	msg, err := h.svc.ReceiveNextForDelivery(ctx, member.ID(memberID))
	if err != nil {
		return MessageDeliveryReceiveNextResult{}, internalError("receive message for delivery", err)
	}
	view, err := h.messageView(ctx, msg)
	if err != nil {
		return MessageDeliveryReceiveNextResult{}, err
	}
	return MessageDeliveryReceiveNextResult{Message: view}, nil
}

func (h *Handler) MessageDeliveryRecordDelivered(ctx context.Context, p MessageDeliveryRecordDeliveredParams) (MessageDeliveryRecordDeliveredResult, error) {
	messageID := strings.TrimSpace(p.MessageID)
	consumerID := strings.TrimSpace(p.ConsumerID)
	if messageID == "" {
		return MessageDeliveryRecordDeliveredResult{}, invalidParams("messageId is required")
	}
	if consumerID == "" {
		return MessageDeliveryRecordDeliveredResult{}, invalidParams("consumerId is required")
	}
	msg, err := h.svc.RecordDelivered(ctx, types.AgentMessageID(messageID), member.ID(consumerID))
	if err != nil {
		return MessageDeliveryRecordDeliveredResult{}, internalError("record message delivered", err)
	}
	view, err := h.messageView(ctx, msg)
	if err != nil {
		return MessageDeliveryRecordDeliveredResult{}, err
	}
	return MessageDeliveryRecordDeliveredResult{Message: view}, nil
}

func (h *Handler) MessageSend(ctx context.Context, p MessageSendParams) (MessageSendResult, error) {
	spaceID := strings.TrimSpace(p.SpaceID)
	destination := strings.TrimSpace(p.DestinationMemberID)
	kind := strings.TrimSpace(p.Kind)
	if spaceID == "" {
		return MessageSendResult{}, invalidParams("spaceId is required")
	}
	if destination == "" {
		return MessageSendResult{}, invalidParams("destinationMemberId is required")
	}
	if kind == "" {
		return MessageSendResult{}, invalidParams("kind is required")
	}
	channelID := strings.TrimSpace(p.ChannelID)
	if channelID == "" {
		channelID = string(messagechannel.MemberChannelID(spacedomain.SpaceID(spaceID), member.ID(destination)))
	}
	msg, err := h.svc.PublishAgentMessage(ctx, messagedomain.NewMessageInput{
		Route: messagedomain.MessageRoute{
			SpaceID:             spacedomain.SpaceID(spaceID),
			SourceMemberID:      memberIDFromContext(ctx),
			DestinationMemberID: member.ID(destination),
			ChannelID:           types.ChannelID(channelID),
		},
		Content: messagedomain.MessageContent{
			Kind:    types.AgentMessageKind(kind),
			Subject: strings.TrimSpace(p.Subject),
			Body:    cloneMap(p.Body),
			TaskRef: taskdomain.TaskID(strings.TrimSpace(p.TaskRef)),
		},
		Producer: messagedomain.MessageProducer{
			IntentID:      types.IntentID(strings.TrimSpace(p.IntentID)),
			CorrelationID: types.CorrelationID(strings.TrimSpace(p.CorrelationID)),
			CausationID:   types.CausationID(strings.TrimSpace(p.CausationID)),
			Producer:      strings.TrimSpace(p.Producer),
		},
		Metadata: cloneMap(p.Metadata),
	})
	if err != nil {
		return MessageSendResult{}, internalError("send message", err)
	}
	view, err := h.messageView(ctx, msg)
	if err != nil {
		return MessageSendResult{}, err
	}
	return MessageSendResult{Message: view}, nil
}

func (h *Handler) ConversationSend(ctx context.Context, p ConversationSendParams) (ConversationSendResult, error) {
	channelID := strings.TrimSpace(p.ChannelID)
	if channelID == "" {
		return ConversationSendResult{}, invalidParams("channelId is required")
	}
	text := strings.TrimSpace(p.Text)
	if text == "" && len(p.AttachmentIDs) == 0 {
		return ConversationSendResult{}, invalidParams("text or attachmentIds is required")
	}
	msg, err := h.svc.SendConversationMessage(ctx, messageapp.SendConversationMessageParams{
		ChannelID:     types.ChannelID(channelID),
		SenderType:    "user",
		SenderID:      userIDFromContext(ctx),
		Text:          text,
		AttachmentIDs: p.AttachmentIDs,
	})
	if err != nil {
		return ConversationSendResult{}, internalError("send conversation message", err)
	}
	return ConversationSendResult{Message: NewConversationMessageView(msg)}, nil
}

func (h *Handler) ConversationSteer(ctx context.Context, p ConversationSteerParams) (ConversationSteerResult, error) {
	messageID := strings.TrimSpace(p.MessageID)
	if messageID == "" {
		return ConversationSteerResult{}, invalidParams("messageId is required")
	}
	msg, err := h.svc.SteerConversationMessage(ctx, messageID)
	if err != nil {
		return ConversationSteerResult{}, internalError("steer conversation message", err)
	}
	return ConversationSteerResult{Message: NewConversationMessageView(msg)}, nil
}

func (h *Handler) AttachmentUpload(ctx context.Context, p AttachmentUploadParams) (AttachmentUploadResult, error) {
	channelID := strings.TrimSpace(p.ChannelID)
	if channelID == "" {
		return AttachmentUploadResult{}, invalidParams("channelId is required")
	}
	if strings.TrimSpace(p.Name) == "" {
		return AttachmentUploadResult{}, invalidParams("name is required")
	}
	if strings.TrimSpace(p.MediaType) == "" {
		return AttachmentUploadResult{}, invalidParams("mediaType is required")
	}
	if strings.TrimSpace(p.DataBase64) == "" {
		return AttachmentUploadResult{}, invalidParams("dataBase64 is required")
	}
	bytes, err := p.DecodeBytes()
	if err != nil {
		return AttachmentUploadResult{}, invalidParams("dataBase64 must be valid base64")
	}
	attachment, err := h.svc.UploadConversationAttachment(ctx, messageapp.UploadConversationAttachmentParams{
		ChannelID: types.ChannelID(channelID),
		ProjectID: types.ProjectID(strings.TrimSpace(p.ProjectID)),
		Name:      p.Name,
		MediaType: p.MediaType,
		Bytes:     bytes,
	})
	if err != nil {
		return AttachmentUploadResult{}, internalError("upload conversation attachment", err)
	}
	return AttachmentUploadResult{Attachment: NewAttachmentViews([]conversation.Attachment{attachment})[0]}, nil
}

func (h *Handler) AttachmentGet(ctx context.Context, p AttachmentGetParams) (AttachmentGetResult, error) {
	attachmentID := strings.TrimSpace(p.AttachmentID)
	if attachmentID == "" {
		return AttachmentGetResult{}, invalidParams("attachmentId is required")
	}
	blob, err := h.svc.GetConversationAttachment(ctx, attachmentID)
	if err != nil {
		return AttachmentGetResult{}, internalError("get conversation attachment", err)
	}
	return AttachmentGetResult{
		Attachment: NewAttachmentViews([]conversation.Attachment{blob.Attachment})[0],
		DataBase64: base64.StdEncoding.EncodeToString(blob.Bytes),
	}, nil
}

func (h *Handler) ConversationList(ctx context.Context, p ConversationListParams) (ConversationListResult, error) {
	channelID := strings.TrimSpace(p.ChannelID)
	if channelID == "" {
		return ConversationListResult{}, invalidParams("channelId is required")
	}
	limit := p.Limit
	if limit <= 0 {
		limit = 50
	}
	messages, err := h.svc.ListConversationMessages(ctx, types.ChannelID(channelID), limit)
	if err != nil {
		return ConversationListResult{}, internalError("list conversation messages", err)
	}
	activities, err := h.svc.ListConversationActivities(ctx, types.ChannelID(channelID), limit)
	if err != nil {
		return ConversationListResult{}, internalError("list conversation activities", err)
	}
	views := make([]ConversationMessageView, 0, len(messages))
	for _, msg := range messages {
		views = append(views, NewConversationMessageView(msg))
	}
	activityViews := make([]ConversationActivityView, 0, len(activities))
	for _, activity := range activities {
		activityViews = append(activityViews, NewConversationActivityView(activity))
	}
	return ConversationListResult{Messages: views, Activities: activityViews}, nil
}

func (h *Handler) channelView(ctx context.Context, ch types.Channel, unreadCount int) (ChannelView, error) {
	memberID := member.ID(strings.TrimSpace(ch.MemberID))
	label, err := h.memberLabel(ctx, memberID)
	if err != nil {
		return ChannelView{}, err
	}
	return ChannelView{
		ID:             string(ch.ID),
		SpaceID:        string(ch.SpaceID),
		ProjectID:      string(ch.ProjectID),
		MemberID:       string(memberID),
		MemberLabel:    label,
		Status:         strings.TrimSpace(ch.Status),
		LastActivityAt: ch.LastMessageAt,
		UnreadCount:    unreadCount,
		CreatedAt:      ch.CreatedAt,
		UpdatedAt:      ch.UpdatedAt,
	}, nil
}

func (h *Handler) messageView(ctx context.Context, msg types.AgentMessage) (MessageView, error) {
	sourceLabel := ""
	if msg.SourceMemberID != "" {
		label, err := h.memberLabel(ctx, msg.SourceMemberID)
		if err != nil {
			return MessageView{}, err
		}
		sourceLabel = label
	}
	destinationLabel, err := h.memberLabel(ctx, msg.DestinationMemberID)
	if err != nil {
		return MessageView{}, err
	}
	return MessageView{
		MessageID:              string(msg.ID),
		SpaceID:                string(msg.SpaceID),
		ChannelID:              string(msg.ChannelID),
		SourceMemberID:         string(msg.SourceMemberID),
		SourceMemberLabel:      sourceLabel,
		DestinationMemberID:    string(msg.DestinationMemberID),
		DestinationMemberLabel: destinationLabel,
		Kind:                   string(msg.Kind),
		Subject:                msg.Subject,
		Body:                   cloneMap(msg.Body),
		TaskRef:                string(msg.TaskRef),
		Status:                 string(msg.Status),
		CreatedAt:              msg.CreatedAt,
		ConsumedAt:             msg.ConsumedAt,
	}, nil
}

func (h *Handler) memberLabel(ctx context.Context, memberID member.ID) (string, error) {
	memberID = member.ID(strings.TrimSpace(string(memberID)))
	if memberID == "" {
		return "", internalError("resolve message member label", fmt.Errorf("member id is required"))
	}
	member, err := h.members.GetMember(ctx, memberID)
	if err != nil {
		return "", internalError("resolve message member label", err)
	}
	label := strings.TrimSpace(member.DisplayName)
	if label == "" {
		return "", internalError("resolve message member label", fmt.Errorf("display name is empty for member %s", memberID))
	}
	return label, nil
}

func messageFilter(p MessageListParams) (messagedomain.MessageFilter, error) {
	if p.Limit < 0 {
		return messagedomain.MessageFilter{}, invalidParams("limit must be non-negative")
	}
	if p.Offset < 0 {
		return messagedomain.MessageFilter{}, invalidParams("offset must be non-negative")
	}
	return messagedomain.MessageFilter{
		SpaceID:             spacedomain.SpaceID(strings.TrimSpace(p.SpaceID)),
		SourceMemberID:      member.ID(strings.TrimSpace(p.SourceMemberID)),
		DestinationMemberID: member.ID(strings.TrimSpace(p.DestinationMemberID)),
		ChannelID:           types.ChannelID(strings.TrimSpace(p.ChannelID)),
		CorrelationID:       types.CorrelationID(strings.TrimSpace(p.CorrelationID)),
		TaskRef:             taskdomain.TaskID(strings.TrimSpace(p.TaskRef)),
		Kinds:               parseKinds(p.Kinds),
		Statuses:            parseStatuses(p.Statuses),
		Limit:               p.Limit,
		Offset:              p.Offset,
	}, nil
}

func parseKinds(raw []string) []types.AgentMessageKind {
	out := make([]types.AgentMessageKind, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, types.AgentMessageKind(item))
		}
	}
	return out
}

func parseStatuses(raw []string) []types.MessageStatus {
	out := make([]types.MessageStatus, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, types.MessageStatus(item))
		}
	}
	return out
}

func parseChannelIDs(raw []string) []types.ChannelID {
	out := make([]types.ChannelID, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, types.ChannelID(item))
		}
	}
	return out
}

func cloneMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

func userIDFromContext(ctx context.Context) string {
	identity, _ := IdentityFromContext(ctx)
	return identity.UserID
}

func memberIDFromContext(ctx context.Context) member.ID {
	identity, _ := IdentityFromContext(ctx)
	return member.ID(identity.MemberID)
}

type identityContextKey struct{}

type Identity struct {
	UserID   string
	MemberID string
}

func ContextWithIdentity(ctx context.Context, identity Identity) context.Context {
	identity.UserID = strings.TrimSpace(identity.UserID)
	identity.MemberID = strings.TrimSpace(identity.MemberID)
	return context.WithValue(ctx, identityContextKey{}, identity)
}

func IdentityFromContext(ctx context.Context) (Identity, bool) {
	identity, ok := ctx.Value(identityContextKey{}).(Identity)
	return identity, ok
}

func invalidParams(message string) error {
	return rpcError{code: -32602, message: strings.TrimSpace(message)}
}

func internalError(action string, err error) error {
	return fmt.Errorf("%s: %w", action, err)
}

type rpcError struct {
	code    int
	message string
}

func (e rpcError) Error() string {
	return e.message
}

func (e rpcError) RPCCode() int {
	return e.code
}
