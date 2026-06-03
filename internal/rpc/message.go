package rpc

import (
	"context"
	"fmt"

	messageapp "github.com/tinoosan/agen8-mcp-server/internal/services/message/app"
	messagerpc "github.com/tinoosan/agen8-mcp-server/internal/services/message/rpc"
)

const (
	MethodMessageChannelList             = "message.channel.list"
	MethodMessageChannelGet              = "message.channel.get"
	MethodMessageChannelEnsureMember     = "message.channel.ensureMember"
	MethodMessageChannelMarkRead         = "message.channel.markRead"
	MethodMessageChannelUnreadCounts     = "message.channel.unreadCounts"
	MethodMessageGet                     = "message.get"
	MethodMessageList                    = "message.list"
	MethodMessageCount                   = "message.count"
	MethodMessageDeliveryReceiveNext     = "message.delivery.receiveNext"
	MethodMessageDeliveryRecordDelivered = "message.delivery.recordDelivered"
	MethodMessageSend                    = "message.send"
	MethodMessageAttachmentUpload        = "message.attachment.upload"
	MethodMessageAttachmentGet           = "message.attachment.get"
	MethodMessageConversationSend        = "message.conversation.send"
	MethodMessageConversationSteer       = "message.conversation.steer"
	MethodMessageConversationList        = "message.conversation.list"
)

func RegisterMessage(reg *Registry, messageSvc *messageapp.Service, memberSvc messagerpc.MemberLoader) error {
	if messageSvc == nil {
		return fmt.Errorf("message service is required")
	}
	if memberSvc == nil {
		return fmt.Errorf("space member loader is required")
	}
	handler := messagerpc.NewHandler(messageSvc, memberSvc)
	return RegisterHandlers(
		func() error {
			return AddBoundHandler(reg, MethodMessageChannelList, false, withMessageCaller(handler.ChannelList))
		},
		func() error {
			return AddBoundHandler(reg, MethodMessageChannelGet, false, withMessageCaller(handler.ChannelGet))
		},
		func() error {
			return AddBoundHandler(reg, MethodMessageChannelEnsureMember, false, withMessageCaller(handler.ChannelEnsureMember))
		},
		func() error {
			return AddBoundHandler(reg, MethodMessageChannelMarkRead, false, withMessageCaller(handler.ChannelMarkRead))
		},
		func() error {
			return AddBoundHandler(reg, MethodMessageChannelUnreadCounts, false, withMessageCaller(handler.ChannelUnreadCounts))
		},
		func() error {
			return AddBoundHandler(reg, MethodMessageList, false, withMessageCaller(handler.MessageList))
		},
		func() error {
			return AddBoundHandler(reg, MethodMessageGet, false, withMessageCaller(handler.MessageGet))
		},
		func() error {
			return AddBoundHandler(reg, MethodMessageCount, false, withMessageCaller(handler.MessageCount))
		},
		func() error {
			return AddBoundHandler(reg, MethodMessageDeliveryReceiveNext, false, withMessageCaller(handler.MessageDeliveryReceiveNext))
		},
		func() error {
			return AddBoundHandler(reg, MethodMessageDeliveryRecordDelivered, false, withMessageCaller(handler.MessageDeliveryRecordDelivered))
		},
		func() error {
			return AddBoundHandler(reg, MethodMessageSend, false, withMessageCaller(handler.MessageSend))
		},
		func() error {
			return AddBoundHandler(reg, MethodMessageAttachmentUpload, false, withMessageCaller(handler.AttachmentUpload))
		},
		func() error {
			return AddBoundHandler(reg, MethodMessageAttachmentGet, false, withMessageCaller(handler.AttachmentGet))
		},
		func() error {
			return AddBoundHandler(reg, MethodMessageConversationSend, false, withMessageCaller(handler.ConversationSend))
		},
		func() error {
			return AddBoundHandler(reg, MethodMessageConversationSteer, false, withMessageCaller(handler.ConversationSteer))
		},
		func() error {
			return AddBoundHandler(reg, MethodMessageConversationList, false, withMessageCaller(handler.ConversationList))
		},
	)
}

func withMessageCaller[Params any, Result any](fn func(context.Context, Params) (Result, error)) func(context.Context, Params) (Result, error) {
	return func(ctx context.Context, params Params) (Result, error) {
		identity, err := RequireIdentity(ctx)
		if err != nil {
			var zero Result
			return zero, err
		}
		ctx = messagerpc.ContextWithIdentity(ctx, messagerpc.Identity{
			UserID:   identity.UserID,
			MemberID: identity.MemberID,
		})
		return fn(ctx, params)
	}
}
