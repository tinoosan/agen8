package conversation_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tinoosan/agen8-mcp-server/internal/services/message/domain/conversation"
)

var conversationTestNow = time.Date(2026, 5, 26, 15, 0, 0, 0, time.UTC)

func TestNewInboundMessageRequiresDeliveryState(t *testing.T) {
	msg, err := conversation.NewMessage(conversation.MessageParams{
		ID:         "msg-1",
		ChannelID:  "channel-1",
		SpaceID:    "space-1",
		MemberID:   "member-1",
		SessionID:  "session-1",
		Direction:  conversation.DirectionInbound,
		SenderType: "user",
		SenderID:   "user-1",
		Text:       "hello",
		Delivery:   conversation.DeliveryQueued,
		Render:     conversation.RenderVisible,
		Now:        conversationTestNow,
	})
	require.NoError(t, err)
	assert.Equal(t, conversation.DirectionInbound, msg.Direction)
	assert.Equal(t, conversation.DeliveryQueued, msg.Delivery)
	assert.Equal(t, conversation.RenderVisible, msg.Render)
	assert.Equal(t, conversationTestNow, msg.CreatedAt)
	assert.Equal(t, conversationTestNow, msg.UpdatedAt)
}

func TestNewOutboundMessageRejectsDeliveryState(t *testing.T) {
	_, err := conversation.NewMessage(conversation.MessageParams{
		ID:         "msg-1",
		ChannelID:  "channel-1",
		SpaceID:    "space-1",
		MemberID:   "member-1",
		SessionID:  "session-1",
		Direction:  conversation.DirectionOutbound,
		SenderType: "harness",
		Text:       "hello back",
		Delivery:   conversation.DeliveryDelivered,
		Render:     conversation.RenderVisible,
		Now:        conversationTestNow,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delivery state is only valid for inbound messages")
}

func TestNewMessageValidatesRequiredFields(t *testing.T) {
	_, err := conversation.NewMessage(conversation.MessageParams{
		ID:         "msg-1",
		ChannelID:  "",
		SpaceID:    "space-1",
		MemberID:   "member-1",
		SessionID:  "session-1",
		Direction:  conversation.DirectionInbound,
		SenderType: "user",
		Text:       "hello",
		Delivery:   conversation.DeliveryQueued,
		Render:     conversation.RenderVisible,
		Now:        conversationTestNow,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "channelID is required")
}

func TestNewInboundMessageCanWaitForHarnessSessionBinding(t *testing.T) {
	msg, err := conversation.NewMessage(conversation.MessageParams{
		ID:         "msg-1",
		ChannelID:  "channel-1",
		SpaceID:    "space-1",
		MemberID:   "member-1",
		Direction:  conversation.DirectionInbound,
		SenderType: "user",
		Text:       "hello",
		Delivery:   conversation.DeliveryQueued,
		Render:     conversation.RenderVisible,
		Now:        conversationTestNow,
	})
	require.NoError(t, err)
	assert.Empty(t, msg.SessionID)
}
