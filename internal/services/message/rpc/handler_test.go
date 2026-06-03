package rpc

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	messageapp "github.com/tinoosan/agen8-mcp-server/internal/services/message/app"
	messageinfra "github.com/tinoosan/agen8-mcp-server/internal/services/message/infra"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

func TestHandlerMessageSendPushesIntoMemberChannel(t *testing.T) {
	handler := newHandlerForTest(t)
	ctx := ContextWithIdentity(context.Background(), Identity{UserID: "user-1", MemberID: "member-source"})

	sent, err := handler.MessageSend(ctx, MessageSendParams{
		SpaceID:             "space-1",
		DestinationMemberID: "member-dest",
		Kind:                string(types.AgentMessageKindInform),
		Subject:             "Status",
		Body:                map[string]any{"text": "ready"},
		IntentID:            "intent-1",
		Producer:            "rpc-test",
	})
	if err != nil {
		t.Fatalf("MessageSend: %v", err)
	}
	if sent.Message.SourceMemberLabel != "Source Agent" || sent.Message.DestinationMemberLabel != "Destination Agent" {
		t.Fatalf("message labels=%q/%q", sent.Message.SourceMemberLabel, sent.Message.DestinationMemberLabel)
	}
	if sent.Message.ChannelID != "channel:space-1:member:member-dest" {
		t.Fatalf("channelId=%q", sent.Message.ChannelID)
	}
	got, err := handler.MessageGet(ctx, MessageGetParams{MessageID: sent.Message.MessageID})
	if err != nil {
		t.Fatalf("MessageGet: %v", err)
	}
	if got.Message.MessageID != sent.Message.MessageID || got.Message.Subject != "Status" {
		t.Fatalf("got message=%+v sent=%+v", got.Message, sent.Message)
	}
	if got.Message.SourceMemberLabel != "Source Agent" || got.Message.DestinationMemberLabel != "Destination Agent" {
		t.Fatalf("got labels=%q/%q", got.Message.SourceMemberLabel, got.Message.DestinationMemberLabel)
	}

	channels, err := handler.ChannelList(ctx, ChannelListParams{SpaceID: "space-1"})
	if err != nil {
		t.Fatalf("ChannelList: %v", err)
	}
	if len(channels.Channels) != 1 {
		t.Fatalf("channels=%+v", channels.Channels)
	}
	if channels.Channels[0].MemberLabel != "Destination Agent" || channels.Channels[0].UnreadCount != 1 {
		t.Fatalf("channel view=%+v", channels.Channels[0])
	}
}

func TestHandlerDeliveryReceiveNextIsRuntimeFacing(t *testing.T) {
	handler := newHandlerForTest(t)
	ctx := ContextWithIdentity(context.Background(), Identity{UserID: "user-1", MemberID: "member-source"})

	_, err := handler.MessageSend(ctx, MessageSendParams{
		SpaceID:             "space-1",
		DestinationMemberID: "member-dest",
		Kind:                string(types.AgentMessageKindSystem),
		Subject:             "Wake",
		Body:                map[string]any{"text": "deliver"},
		IntentID:            "intent-delivery",
		Producer:            "rpc-test",
	})
	if err != nil {
		t.Fatalf("MessageSend: %v", err)
	}

	received, err := handler.MessageDeliveryReceiveNext(ctx, MessageDeliveryReceiveNextParams{
		MemberID: "member-dest",
	})
	if err != nil {
		t.Fatalf("MessageDeliveryReceiveNext: %v", err)
	}
	if received.Message.Status != string(types.MessageStatusQueuedTyped) {
		t.Fatalf("status=%q want queued", received.Message.Status)
	}
	delivered, err := handler.MessageDeliveryRecordDelivered(ctx, MessageDeliveryRecordDeliveredParams{
		MessageID:  received.Message.MessageID,
		ConsumerID: "daemon-delivery",
	})
	if err != nil {
		t.Fatalf("MessageDeliveryRecordDelivered: %v", err)
	}
	if delivered.Message.Status != string(types.MessageStatusConsumedTyped) {
		t.Fatalf("status=%q want consumed", delivered.Message.Status)
	}
	if delivered.Message.DestinationMemberLabel != "Destination Agent" {
		t.Fatalf("destination label=%q", delivered.Message.DestinationMemberLabel)
	}
}

func TestHandlerConversationSendAndList(t *testing.T) {
	handler := newHandlerForTestWithHarness(t, &fakeHarnessChatSender{})
	ctx := ContextWithIdentity(context.Background(), Identity{UserID: "user-1", MemberID: "member-source"})
	ensured, err := handler.ChannelEnsureMember(ctx, ChannelEnsureMemberParams{
		SpaceID:  "space-1",
		MemberID: "member-dest",
	})
	if err != nil {
		t.Fatalf("ChannelEnsureMember: %v", err)
	}

	sent, err := handler.ConversationSend(ctx, ConversationSendParams{
		ChannelID: ensured.Channel.ID,
		Text:      "please help",
	})
	if err != nil {
		t.Fatalf("ConversationSend: %v", err)
	}
	if sent.Message.ChannelID != ensured.Channel.ID || sent.Message.Text != "please help" || sent.Message.Delivery != "delivered" {
		t.Fatalf("sent conversation message=%+v", sent.Message)
	}
	listed, err := handler.ConversationList(ctx, ConversationListParams{ChannelID: ensured.Channel.ID})
	if err != nil {
		t.Fatalf("ConversationList: %v", err)
	}
	if len(listed.Messages) != 2 || listed.Messages[0].ID != sent.Message.ID || listed.Messages[1].Direction != "outbound" || listed.Messages[1].Text != "assistant response" {
		t.Fatalf("listed=%+v sent=%+v", listed.Messages, sent.Message)
	}
}

func TestHandlerConversationSendRequiresText(t *testing.T) {
	handler := newHandlerForTest(t)
	ctx := ContextWithIdentity(context.Background(), Identity{UserID: "user-1", MemberID: "member-source"})

	_, err := handler.ConversationSend(ctx, ConversationSendParams{ChannelID: "channel:space-1:member:member-dest"})
	if err == nil {
		t.Fatal("ConversationSend returned nil error for missing text")
	}
}

func TestHandlerConversationSteerSendsQueuedUserMessage(t *testing.T) {
	harness := &activeRunHarnessChatSender{activeRun: true}
	handler := newHandlerForTestWithHarness(t, harness)
	ctx := ContextWithIdentity(context.Background(), Identity{UserID: "user-1", MemberID: "member-source"})
	ensured, err := handler.ChannelEnsureMember(ctx, ChannelEnsureMemberParams{
		SpaceID:  "space-1",
		MemberID: "member-dest",
	})
	if err != nil {
		t.Fatalf("ChannelEnsureMember: %v", err)
	}
	queued, err := handler.ConversationSend(ctx, ConversationSendParams{
		ChannelID: ensured.Channel.ID,
		Text:      "apply this direction",
	})
	if err != nil {
		t.Fatalf("ConversationSend: %v", err)
	}
	if queued.Message.Delivery != "queued" {
		t.Fatalf("queued delivery=%q", queued.Message.Delivery)
	}

	harness.activeRun = false
	steered, err := handler.ConversationSteer(ctx, ConversationSteerParams{MessageID: queued.Message.ID})
	if err != nil {
		t.Fatalf("ConversationSteer: %v", err)
	}
	if steered.Message.Delivery != "steered" || steered.Message.ID != queued.Message.ID {
		t.Fatalf("steered message=%+v queued=%+v", steered.Message, queued.Message)
	}
	if !harness.input.AllowSteering || harness.input.Text != "apply this direction" {
		t.Fatalf("harness input=%+v", harness.input)
	}
}

func TestHandlerConversationSteerRequiresMessageID(t *testing.T) {
	handler := newHandlerForTest(t)
	ctx := ContextWithIdentity(context.Background(), Identity{UserID: "user-1", MemberID: "member-source"})

	_, err := handler.ConversationSteer(ctx, ConversationSteerParams{})
	if err == nil {
		t.Fatal("ConversationSteer returned nil error for missing message id")
	}
}

func TestHandlerAttachmentUploadAndConversationSend(t *testing.T) {
	harness := &capturingHarnessChatSender{}
	handler := newHandlerForTestWithHarness(t, harness)
	ctx := ContextWithIdentity(context.Background(), Identity{UserID: "user-1", MemberID: "member-source"})
	ensured, err := handler.ChannelEnsureMember(ctx, ChannelEnsureMemberParams{
		SpaceID:   "space-1",
		ProjectID: "project-1",
		MemberID:  "member-dest",
	})
	if err != nil {
		t.Fatalf("ChannelEnsureMember: %v", err)
	}

	uploaded, err := handler.AttachmentUpload(ctx, AttachmentUploadParams{
		ChannelID:  ensured.Channel.ID,
		Name:       "screen.png",
		MediaType:  "image/png",
		DataBase64: base64.StdEncoding.EncodeToString([]byte("png")),
	})
	if err != nil {
		t.Fatalf("AttachmentUpload: %v", err)
	}
	if _, err := os.Stat(uploaded.Attachment.URI); err != nil {
		t.Fatalf("uploaded file missing: %v", err)
	}
	got, err := handler.AttachmentGet(ctx, AttachmentGetParams{AttachmentID: uploaded.Attachment.ID})
	if err != nil {
		t.Fatalf("AttachmentGet: %v", err)
	}
	if got.Attachment.ID != uploaded.Attachment.ID || got.DataBase64 != base64.StdEncoding.EncodeToString([]byte("png")) {
		t.Fatalf("got attachment=%+v data=%q", got.Attachment, got.DataBase64)
	}

	sent, err := handler.ConversationSend(ctx, ConversationSendParams{
		ChannelID:     ensured.Channel.ID,
		AttachmentIDs: []string{uploaded.Attachment.ID},
	})
	if err != nil {
		t.Fatalf("ConversationSend: %v", err)
	}
	if len(sent.Message.Attachments) != 1 || sent.Message.Attachments[0].ID != uploaded.Attachment.ID {
		t.Fatalf("sent attachments=%+v", sent.Message.Attachments)
	}
	if len(harness.input.Attachments) != 1 || harness.input.Attachments[0].ID != uploaded.Attachment.ID {
		t.Fatalf("harness attachments=%+v", harness.input.Attachments)
	}
}

func TestHandlerAttachmentGetRequiresID(t *testing.T) {
	handler := newHandlerForTest(t)
	ctx := ContextWithIdentity(context.Background(), Identity{UserID: "user-1", MemberID: "member-source"})

	_, err := handler.AttachmentGet(ctx, AttachmentGetParams{})
	if err == nil {
		t.Fatal("AttachmentGet returned nil error for missing id")
	}
}

func TestHandlerAttachmentUploadRequiresBase64(t *testing.T) {
	handler := newHandlerForTest(t)
	ctx := ContextWithIdentity(context.Background(), Identity{UserID: "user-1", MemberID: "member-source"})
	_, err := handler.AttachmentUpload(ctx, AttachmentUploadParams{
		ChannelID:  "channel:space-1:member:member-dest",
		Name:       "screen.png",
		MediaType:  "image/png",
		DataBase64: "not-base64",
	})
	if err == nil {
		t.Fatal("AttachmentUpload returned nil error for invalid base64")
	}
}

func TestHandlerMessageListRejectsNegativeLimit(t *testing.T) {
	handler := newHandlerForTest(t)
	ctx := ContextWithIdentity(context.Background(), Identity{UserID: "user-1", MemberID: "member-source"})

	_, err := handler.MessageList(ctx, MessageListParams{Limit: -1})
	if err == nil {
		t.Fatal("MessageList returned nil error for negative limit")
	}
}

func TestHandlerMessageGetRejectsMissingID(t *testing.T) {
	handler := newHandlerForTest(t)
	ctx := ContextWithIdentity(context.Background(), Identity{UserID: "user-1", MemberID: "member-source"})

	_, err := handler.MessageGet(ctx, MessageGetParams{})
	if err == nil {
		t.Fatal("MessageGet returned nil error for missing message id")
	}
}

func newHandlerForTest(t *testing.T) *Handler {
	return newHandlerForTestWithHarness(t, nil)
}

func newHandlerForTestWithHarness(t *testing.T, harness messageapp.HarnessChatSender) *Handler {
	t.Helper()
	handle, err := storagedb.Open(context.Background(), storagedb.Config{
		Driver:  storagedb.DriverSQLite,
		DataDir: filepath.Join(t.TempDir(), "data"),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	repo, err := messageinfra.NewSQLiteRepository(handle)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	conversations, err := messageinfra.NewSQLiteConversationRepository(handle)
	if err != nil {
		t.Fatalf("NewSQLiteConversationRepository: %v", err)
	}
	svc, err := messageapp.NewService(messageapp.NewServiceParams{
		Repository:        repo,
		Conversations:     conversations,
		HarnessChatSender: harness,
		Clock:             messageapp.FixedClock{T: time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	svc.SetProjectRootResolver(fakeProjectRootResolver{root: t.TempDir()})
	return NewHandler(svc, fakeMemberReader{members: map[string]member.Record{
		"member-source": {ID: "member-source", DisplayName: "Source Agent"},
		"member-dest":   {ID: "member-dest", DisplayName: "Destination Agent"},
	}})
}

type fakeHarnessChatSender struct{}

func (fakeHarnessChatSender) SendMessage(_ context.Context, input messageapp.HarnessChatMessage) (messageapp.HarnessChatResult, error) {
	if input.SpaceID == "" || input.MemberID == "" || input.ChannelID == "" || input.ConversationMessageID == "" || (input.Text == "" && len(input.Attachments) == 0) {
		return messageapp.HarnessChatResult{}, fmt.Errorf("missing harness chat input: %+v", input)
	}
	return messageapp.HarnessChatResult{SessionID: "session-1", TurnID: "turn-1", Delivery: "delivered", Text: "assistant response"}, nil
}

type capturingHarnessChatSender struct {
	input messageapp.HarnessChatMessage
}

func (f *capturingHarnessChatSender) SendMessage(_ context.Context, input messageapp.HarnessChatMessage) (messageapp.HarnessChatResult, error) {
	f.input = input
	return messageapp.HarnessChatResult{SessionID: "session-1", TurnID: "turn-1", Delivery: "delivered", Text: "assistant response"}, nil
}

type activeRunHarnessChatSender struct {
	input     messageapp.HarnessChatMessage
	activeRun bool
}

func (f *activeRunHarnessChatSender) SendMessage(_ context.Context, input messageapp.HarnessChatMessage) (messageapp.HarnessChatResult, error) {
	f.input = input
	if f.activeRun {
		return messageapp.HarnessChatResult{}, fmt.Errorf(`harness session "session-1" already has active run "run-1"`)
	}
	if !input.AllowSteering {
		return messageapp.HarnessChatResult{}, fmt.Errorf("steering was not allowed")
	}
	return messageapp.HarnessChatResult{SessionID: "session-1", RunID: "run-1", TurnID: "turn-1", Delivery: "steered"}, nil
}

type fakeProjectRootResolver struct {
	root string
}

func (r fakeProjectRootResolver) ResolveProjectRoot(_ context.Context, _ types.ProjectID) (string, error) {
	if r.root == "" {
		return "", fmt.Errorf("project root is required")
	}
	return r.root, nil
}

type fakeMemberReader struct {
	members map[string]member.Record
}

func (r fakeMemberReader) GetMember(_ context.Context, memberID member.ID) (member.Record, error) {
	rosterMember, ok := r.members[string(memberID)]
	if !ok {
		return member.Record{}, fmt.Errorf("member %s not found", memberID)
	}
	return rosterMember, nil
}
