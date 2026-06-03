package infra_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tinoosan/agen8-mcp-server/internal/services/message/domain/conversation"
	"github.com/tinoosan/agen8-mcp-server/internal/services/message/infra"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

var conversationTestNow = time.Date(2026, 5, 26, 15, 0, 0, 0, time.UTC)

func setupConversationRepo(t *testing.T) conversation.Repository {
	t.Helper()
	repo, _ := setupConversationRepoWithHandle(t)
	return repo
}

func setupConversationRepoWithHandle(t *testing.T) (conversation.Repository, *storagedb.Handle) {
	t.Helper()
	handle, err := storagedb.Open(context.Background(), storagedb.Config{
		Driver:       storagedb.DriverSQLite,
		DataDir:      t.TempDir(),
		MigrationKey: "message-conversation-test",
		Migrate: func(_ context.Context, db *sql.DB, _ storagedb.Driver) error {
			return infra.MigrateConversationSchema(context.Background(), db)
		},
	})
	require.NoError(t, err)
	repo, err := infra.NewSQLiteConversationRepository(handle)
	require.NoError(t, err)
	return repo, handle
}

func TestSQLiteConversationRepository_SaveAndListByChannel(t *testing.T) {
	repo := setupConversationRepo(t)
	ctx := context.Background()

	inbound := newConversationMessage(t, "msg-1", "channel-1", conversation.DirectionInbound, "user", "hello", conversation.DeliveryQueued)
	outbound := newConversationMessage(t, "msg-2", "channel-1", conversation.DirectionOutbound, "harness", "hello back", "")
	otherChannel := newConversationMessage(t, "msg-3", "channel-2", conversation.DirectionInbound, "user", "elsewhere", conversation.DeliveryQueued)

	require.NoError(t, repo.Save(ctx, inbound))
	require.NoError(t, repo.Save(ctx, outbound))
	require.NoError(t, repo.Save(ctx, otherChannel))

	got, err := repo.ListByChannel(ctx, "channel-1", 20)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "msg-1", got[0].ID)
	assert.Equal(t, conversation.DirectionInbound, got[0].Direction)
	assert.Equal(t, conversation.DeliveryQueued, got[0].Delivery)
	assert.Equal(t, "msg-2", got[1].ID)
	assert.Equal(t, conversation.DirectionOutbound, got[1].Direction)
	assert.Empty(t, got[1].Delivery)
}

func TestSQLiteConversationRepository_SaveAttachmentAndMessageAttachments(t *testing.T) {
	repo := setupConversationRepo(t)
	ctx := context.Background()
	attachment := conversation.Attachment{
		ID:        "attachment-1",
		ProjectID: "project-1",
		SpaceID:   "space-1",
		ChannelID: "channel-1",
		Name:      "screen.png",
		MediaType: "image/png",
		SizeBytes: 12,
		URI:       "/tmp/screen.png",
		CreatedAt: conversationTestNow,
	}
	require.NoError(t, repo.SaveAttachment(ctx, attachment))
	gotAttachments, err := repo.GetAttachments(ctx, []string{"attachment-1"})
	require.NoError(t, err)
	require.Equal(t, []conversation.Attachment{attachment}, gotAttachments)

	msg, err := conversation.NewMessage(conversation.MessageParams{
		ID:          "msg-attachment",
		ChannelID:   "channel-1",
		SpaceID:     "space-1",
		MemberID:    "member-1",
		SessionID:   "session-1",
		Direction:   conversation.DirectionInbound,
		SenderType:  "user",
		SenderID:    "user-1",
		Attachments: []conversation.Attachment{attachment},
		Delivery:    conversation.DeliveryQueued,
		Render:      conversation.RenderVisible,
		Now:         conversationTestNow,
	})
	require.NoError(t, err)
	require.NoError(t, repo.Save(ctx, msg))

	messages, err := repo.ListByChannel(ctx, "channel-1", 20)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, []conversation.Attachment{attachment}, messages[0].Attachments)
}

func TestSQLiteConversationRepository_GetAttachmentsRequiresExistingIDs(t *testing.T) {
	repo := setupConversationRepo(t)
	_, err := repo.GetAttachments(context.Background(), []string{"missing"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "attachment missing not found")
}

func TestSQLiteConversationRepository_UpdateDelivery(t *testing.T) {
	repo := setupConversationRepo(t)
	ctx := context.Background()
	msg := newConversationMessage(t, "msg-1", "channel-1", conversation.DirectionInbound, "user", "hello", conversation.DeliveryQueued)
	msg.SessionID = ""
	require.NoError(t, repo.Save(ctx, msg))

	updated, err := repo.UpdateDelivery(ctx, "msg-1", conversation.DeliveryFailed, "runner crashed", conversationTestNow.Add(time.Minute))
	require.NoError(t, err)
	assert.Equal(t, conversation.DeliveryFailed, updated.Delivery)
	assert.Equal(t, "runner crashed", updated.Error)

	got, err := repo.ListByChannel(ctx, "channel-1", 20)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, conversation.DeliveryFailed, got[0].Delivery)
	assert.Equal(t, "runner crashed", got[0].Error)
}

func TestSQLiteConversationRepository_UpdateDeliveryBinding(t *testing.T) {
	repo := setupConversationRepo(t)
	ctx := context.Background()
	msg := newConversationMessage(t, "msg-1", "channel-1", conversation.DirectionInbound, "user", "hello", conversation.DeliveryQueued)
	msg.SessionID = ""
	require.NoError(t, repo.Save(ctx, msg))

	updated, err := repo.UpdateDeliveryBinding(ctx, "msg-1", conversation.DeliveryDelivered, "session-2", "turn-1", "", conversationTestNow.Add(time.Minute))
	require.NoError(t, err)
	assert.Equal(t, conversation.DeliveryDelivered, updated.Delivery)
	assert.Equal(t, "session-2", updated.SessionID)
	assert.Equal(t, "turn-1", updated.TurnID)
	assert.Empty(t, updated.Error)
}

func TestSQLiteConversationRepository_UpdateRender(t *testing.T) {
	repo := setupConversationRepo(t)
	ctx := context.Background()
	msg := newConversationMessage(t, "msg-1", "channel-1", conversation.DirectionOutbound, "harness", "hello", "")
	require.NoError(t, repo.Save(ctx, msg))

	updated, err := repo.UpdateRender(ctx, "msg-1", conversation.RenderError, "missing payload", conversationTestNow.Add(time.Minute))
	require.NoError(t, err)
	assert.Equal(t, conversation.RenderError, updated.Render)
	assert.Equal(t, "missing payload", updated.Error)
}

func TestSQLiteConversationRepository_SaveAndListActivitiesByChannel(t *testing.T) {
	repo, handle := setupConversationRepoWithHandle(t)
	ctx := context.Background()
	completedAt := conversationTestNow.Add(time.Second)

	started := conversation.Activity{
		ID:         "activity-1",
		ChannelID:  "channel-1",
		SpaceID:    "space-1",
		MemberID:   "member-1",
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		Sequence:   1,
		Kind:       "tool_call",
		Title:      "agen8/mission.list",
		Status:     "pending",
		CreatedAt:  conversationTestNow,
		Data: map[string]string{
			"sourceType": "mcp",
			"server":     "agen8",
		},
	}
	completed := started
	completed.Status = "completed"
	completed.Text = "listed missions"
	completed.CompletedAt = &completedAt
	completed.Data = map[string]string{
		"sourceType":    "mcp",
		"server":        "agen8",
		"outputPreview": "listed missions",
	}
	otherChannel := started
	otherChannel.ID = "activity-2"
	otherChannel.ChannelID = "channel-2"
	otherChannel.ToolCallID = "call-2"
	unorderedLegacy := started
	unorderedLegacy.ID = "activity-legacy"
	unorderedLegacy.ToolCallID = "call-legacy"
	unorderedLegacy.Sequence = 0

	require.NoError(t, repo.SaveActivity(ctx, started))
	require.NoError(t, repo.SaveActivity(ctx, completed))
	require.NoError(t, repo.SaveActivity(ctx, otherChannel))
	require.NoError(t, insertLegacyConversationActivity(ctx, handle.DB(), unorderedLegacy))

	got, err := repo.ListActivitiesByChannel(ctx, "channel-1", 20)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "activity-1", got[0].ID)
	assert.Equal(t, 1, got[0].Sequence)
	assert.Equal(t, "completed", got[0].Status)
	assert.Equal(t, "listed missions", got[0].Text)
	assert.Equal(t, conversationTestNow, got[0].CreatedAt)
	require.NotNil(t, got[0].CompletedAt)
	assert.Equal(t, completedAt, *got[0].CompletedAt)
	assert.Equal(t, "listed missions", got[0].Data["outputPreview"])
}

func insertLegacyConversationActivity(ctx context.Context, db *sql.DB, activity conversation.Activity) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO message_conversation_activities (
			activity_id, channel_id, space_id, member_id, session_id, turn_id,
			tool_call_id, sequence, kind, title, status, text, created_at, completed_at, data_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		activity.ID,
		activity.ChannelID,
		activity.SpaceID,
		activity.MemberID,
		activity.SessionID,
		activity.TurnID,
		activity.ToolCallID,
		activity.Sequence,
		activity.Kind,
		activity.Title,
		activity.Status,
		activity.Text,
		activity.CreatedAt.Format(time.RFC3339Nano),
		nil,
		"{}",
	)
	return err
}

func TestSQLiteConversationRepository_NextQueuedForSession(t *testing.T) {
	repo := setupConversationRepo(t)
	ctx := context.Background()

	first := newConversationMessage(t, "msg-1", "channel-1", conversation.DirectionInbound, "user", "first", conversation.DeliveryQueued)
	delivered := newConversationMessage(t, "msg-2", "channel-1", conversation.DirectionInbound, "user", "second", conversation.DeliveryDelivered)
	outbound := newConversationMessage(t, "msg-3", "channel-1", conversation.DirectionOutbound, "harness", "out", "")
	secondQueued := newConversationMessage(t, "msg-4", "channel-1", conversation.DirectionInbound, "user", "third", conversation.DeliveryQueued)
	secondQueued.CreatedAt = secondQueued.CreatedAt.Add(2)
	secondQueued.UpdatedAt = secondQueued.UpdatedAt.Add(2)

	require.NoError(t, repo.Save(ctx, delivered))
	require.NoError(t, repo.Save(ctx, outbound))
	require.NoError(t, repo.Save(ctx, secondQueued))
	require.NoError(t, repo.Save(ctx, first))

	got, err := repo.NextQueuedForSession(ctx, "session-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "msg-1", got.ID)
}

func TestSQLiteConversationRepository_ValidatesBoundaryInputs(t *testing.T) {
	repo := setupConversationRepo(t)

	_, err := repo.ListByChannel(context.Background(), "", 20)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "channelID is required")

	_, err = repo.ListByChannel(context.Background(), "channel-1", 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "limit must be greater than zero")

	_, err = repo.UpdateDelivery(context.Background(), "msg-1", conversation.DeliveryFailed, "", conversationTestNow)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error is required")
}

func newConversationMessage(t *testing.T, id, channelID string, direction conversation.Direction, senderType, text string, delivery conversation.DeliveryState) conversation.Message {
	t.Helper()
	msg, err := conversation.NewMessage(conversation.MessageParams{
		ID:         id,
		ChannelID:  channelID,
		SpaceID:    "space-1",
		MemberID:   "member-1",
		SessionID:  "session-1",
		Direction:  direction,
		SenderType: senderType,
		SenderID:   senderType + "-1",
		Text:       text,
		Delivery:   delivery,
		Render:     conversation.RenderVisible,
		Now:        conversationTestNow,
	})
	require.NoError(t, err)
	return msg
}
