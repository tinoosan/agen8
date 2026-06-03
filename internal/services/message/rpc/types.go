package rpc

import (
	"encoding/base64"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/services/message/domain/conversation"
)

type ChannelView struct {
	ID             string     `json:"id"`
	SpaceID        string     `json:"spaceId"`
	ProjectID      string     `json:"projectId,omitempty"`
	MemberID       string     `json:"memberId"`
	MemberLabel    string     `json:"memberLabel"`
	Status         string     `json:"status"`
	LastActivityAt *time.Time `json:"lastActivityAt,omitempty"`
	UnreadCount    int        `json:"unreadCount"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt,omitempty"`
}

type MessageView struct {
	MessageID              string         `json:"messageId"`
	SpaceID                string         `json:"spaceId"`
	ChannelID              string         `json:"channelId,omitempty"`
	SourceMemberID         string         `json:"sourceMemberId,omitempty"`
	SourceMemberLabel      string         `json:"sourceMemberLabel,omitempty"`
	DestinationMemberID    string         `json:"destinationMemberId"`
	DestinationMemberLabel string         `json:"destinationMemberLabel"`
	Kind                   string         `json:"kind"`
	Subject                string         `json:"subject,omitempty"`
	Body                   map[string]any `json:"body,omitempty"`
	TaskRef                string         `json:"taskRef,omitempty"`
	Status                 string         `json:"status"`
	CreatedAt              time.Time      `json:"createdAt"`
	ConsumedAt             *time.Time     `json:"consumedAt,omitempty"`
}

type ChannelListParams struct {
	SpaceID string `json:"spaceId"`
}

type ChannelListResult struct {
	Channels []ChannelView `json:"channels"`
}

type ChannelGetParams struct {
	ChannelID string `json:"channelId"`
}

type ChannelGetResult struct {
	Channel ChannelView `json:"channel"`
}

type ChannelEnsureMemberParams struct {
	SpaceID   string `json:"spaceId"`
	ProjectID string `json:"projectId,omitempty"`
	MemberID  string `json:"memberId"`
	Status    string `json:"status,omitempty"`
}

type ChannelEnsureMemberResult struct {
	Channel ChannelView `json:"channel"`
}

type ChannelMarkReadParams struct {
	ChannelID string `json:"channelId"`
}

type ChannelUnreadCountsParams struct {
	ChannelIDs []string `json:"channelIds"`
}

type ChannelUnreadCountsResult struct {
	Counts map[string]int `json:"counts"`
}

type MessageListParams struct {
	SpaceID             string   `json:"spaceId,omitempty"`
	SourceMemberID      string   `json:"sourceMemberId,omitempty"`
	DestinationMemberID string   `json:"destinationMemberId,omitempty"`
	ChannelID           string   `json:"channelId,omitempty"`
	CorrelationID       string   `json:"correlationId,omitempty"`
	TaskRef             string   `json:"taskRef,omitempty"`
	Kinds               []string `json:"kinds,omitempty"`
	Statuses            []string `json:"statuses,omitempty"`
	Limit               int      `json:"limit,omitempty"`
	Offset              int      `json:"offset,omitempty"`
}

type MessageListResult struct {
	Messages   []MessageView `json:"messages"`
	TotalCount int           `json:"totalCount"`
}

type MessageGetParams struct {
	MessageID string `json:"messageId"`
}

type MessageGetResult struct {
	Message MessageView `json:"message"`
}

type MessageCountParams MessageListParams

type MessageCountResult struct {
	TotalCount int `json:"totalCount"`
}

type MessageDeliveryReceiveNextParams struct {
	MemberID string `json:"memberId"`
}

type MessageDeliveryReceiveNextResult struct {
	Message MessageView `json:"message"`
}

type MessageDeliveryRecordDeliveredParams struct {
	MessageID  string `json:"messageId"`
	ConsumerID string `json:"consumerId"`
}

type MessageDeliveryRecordDeliveredResult struct {
	Message MessageView `json:"message"`
}

type MessageSendParams struct {
	SpaceID             string         `json:"spaceId"`
	DestinationMemberID string         `json:"destinationMemberId"`
	ChannelID           string         `json:"channelId,omitempty"`
	Kind                string         `json:"kind"`
	Subject             string         `json:"subject,omitempty"`
	Body                map[string]any `json:"body"`
	TaskRef             string         `json:"taskRef,omitempty"`
	IntentID            string         `json:"intentId"`
	CorrelationID       string         `json:"correlationId,omitempty"`
	CausationID         string         `json:"causationId,omitempty"`
	Producer            string         `json:"producer"`
	Metadata            map[string]any `json:"metadata,omitempty"`
}

type MessageSendResult struct {
	Message MessageView `json:"message"`
}

type ConversationMessageView struct {
	ID          string           `json:"id"`
	ChannelID   string           `json:"channelId"`
	SpaceID     string           `json:"spaceId"`
	MemberID    string           `json:"memberId"`
	SessionID   string           `json:"sessionId,omitempty"`
	TurnID      string           `json:"turnId,omitempty"`
	Direction   string           `json:"direction"`
	SenderType  string           `json:"senderType"`
	SenderID    string           `json:"senderId,omitempty"`
	Text        string           `json:"text"`
	Attachments []AttachmentView `json:"attachments,omitempty"`
	Delivery    string           `json:"delivery,omitempty"`
	Render      string           `json:"render"`
	CreatedAt   time.Time        `json:"createdAt"`
	UpdatedAt   time.Time        `json:"updatedAt"`
	Error       string           `json:"error,omitempty"`
}

func NewConversationMessageView(message conversation.Message) ConversationMessageView {
	return ConversationMessageView{
		ID:          message.ID,
		ChannelID:   message.ChannelID,
		SpaceID:     message.SpaceID,
		MemberID:    message.MemberID,
		SessionID:   message.SessionID,
		TurnID:      message.TurnID,
		Direction:   string(message.Direction),
		SenderType:  message.SenderType,
		SenderID:    message.SenderID,
		Text:        message.Text,
		Attachments: NewAttachmentViews(message.Attachments),
		Delivery:    string(message.Delivery),
		Render:      string(message.Render),
		CreatedAt:   message.CreatedAt,
		UpdatedAt:   message.UpdatedAt,
		Error:       message.Error,
	}
}

type AttachmentView struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"projectId"`
	SpaceID   string    `json:"spaceId"`
	ChannelID string    `json:"channelId"`
	Name      string    `json:"name"`
	MediaType string    `json:"mediaType"`
	SizeBytes int64     `json:"sizeBytes"`
	URI       string    `json:"uri"`
	CreatedAt time.Time `json:"createdAt"`
}

func NewAttachmentViews(attachments []conversation.Attachment) []AttachmentView {
	if len(attachments) == 0 {
		return nil
	}
	out := make([]AttachmentView, 0, len(attachments))
	for _, attachment := range attachments {
		out = append(out, AttachmentView{
			ID:        attachment.ID,
			ProjectID: attachment.ProjectID,
			SpaceID:   attachment.SpaceID,
			ChannelID: attachment.ChannelID,
			Name:      attachment.Name,
			MediaType: attachment.MediaType,
			SizeBytes: attachment.SizeBytes,
			URI:       attachment.URI,
			CreatedAt: attachment.CreatedAt,
		})
	}
	return out
}

type AttachmentUploadParams struct {
	ChannelID  string `json:"channelId"`
	ProjectID  string `json:"projectId,omitempty"`
	Name       string `json:"name"`
	MediaType  string `json:"mediaType"`
	DataBase64 string `json:"dataBase64"`
}

type AttachmentUploadResult struct {
	Attachment AttachmentView `json:"attachment"`
}

func (p AttachmentUploadParams) DecodeBytes() ([]byte, error) {
	return base64.StdEncoding.DecodeString(p.DataBase64)
}

type AttachmentGetParams struct {
	AttachmentID string `json:"attachmentId"`
}

type AttachmentGetResult struct {
	Attachment AttachmentView `json:"attachment"`
	DataBase64 string         `json:"dataBase64"`
}

type ConversationSendParams struct {
	ChannelID     string   `json:"channelId"`
	Text          string   `json:"text,omitempty"`
	AttachmentIDs []string `json:"attachmentIds,omitempty"`
}

type ConversationSendResult struct {
	Message ConversationMessageView `json:"message"`
}

type ConversationSteerParams struct {
	MessageID string `json:"messageId"`
}

type ConversationSteerResult struct {
	Message ConversationMessageView `json:"message"`
}

type ConversationListParams struct {
	ChannelID string `json:"channelId"`
	Limit     int    `json:"limit,omitempty"`
}

type ConversationListResult struct {
	Messages   []ConversationMessageView  `json:"messages"`
	Activities []ConversationActivityView `json:"activities,omitempty"`
}

type ConversationActivityView struct {
	ID          string            `json:"id"`
	ChannelID   string            `json:"channelId"`
	SpaceID     string            `json:"spaceId"`
	MemberID    string            `json:"memberId"`
	SessionID   string            `json:"sessionId"`
	TurnID      string            `json:"turnId"`
	ToolCallID  string            `json:"toolCallId"`
	Sequence    int               `json:"sequence"`
	Kind        string            `json:"kind"`
	Title       string            `json:"title"`
	Status      string            `json:"status"`
	Text        string            `json:"text,omitempty"`
	CreatedAt   time.Time         `json:"createdAt"`
	CompletedAt *time.Time        `json:"completedAt,omitempty"`
	Data        map[string]string `json:"data,omitempty"`
}

func NewConversationActivityView(activity conversation.Activity) ConversationActivityView {
	return ConversationActivityView{
		ID:          activity.ID,
		ChannelID:   activity.ChannelID,
		SpaceID:     activity.SpaceID,
		MemberID:    activity.MemberID,
		SessionID:   activity.SessionID,
		TurnID:      activity.TurnID,
		ToolCallID:  activity.ToolCallID,
		Sequence:    activity.Sequence,
		Kind:        activity.Kind,
		Title:       activity.Title,
		Status:      activity.Status,
		Text:        activity.Text,
		CreatedAt:   activity.CreatedAt,
		CompletedAt: activity.CompletedAt,
		Data:        activity.Data,
	}
}
