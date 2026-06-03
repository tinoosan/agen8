package infra

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/services/message/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/message/domain/channel"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

func TestSQLiteRepository_MessageLifecycle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := newSQLiteRepoForTest(t)
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)

	msg, err := domain.NewMessage(domain.NewMessageInput{
		ID: "msg-1",
		Route: domain.MessageRoute{
			SpaceID:             "space-1",
			SourceMemberID:      "member-source",
			DestinationMemberID: "member-dest",
			ChannelID:           "channel:space-1:member:member-dest",
		},
		Content: domain.MessageContent{
			Kind:    types.AgentMessageKindInform,
			Subject: "Status",
			Body:    map[string]any{"text": "ready"},
		},
		Producer: domain.MessageProducer{
			IntentID: "intent-1",
			Producer: "test",
		},
	}, now)
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}
	saved, err := repo.SaveQueued(ctx, msg.Inner())
	if err != nil {
		t.Fatalf("SaveQueued: %v", err)
	}
	if saved.ID != "msg-1" || saved.Status != types.MessageStatusQueuedTyped {
		t.Fatalf("saved message = %+v", saved)
	}
	duplicate := msg.Inner()
	duplicate.ID = "msg-duplicate"
	duplicate.Subject = "changed"
	idempotent, err := repo.SaveQueued(ctx, duplicate)
	if err != nil {
		t.Fatalf("SaveQueued duplicate: %v", err)
	}
	if idempotent.ID != saved.ID || idempotent.Subject != saved.Subject {
		t.Fatalf("duplicate rewrote existing message: %+v", idempotent)
	}
	next, err := repo.NextQueuedForMember(ctx, "member-dest", now.Add(time.Second))
	if err != nil {
		t.Fatalf("NextQueuedForMember: %v", err)
	}
	consumed, err := domain.WrapMessage(next).Consume("member-dest", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	marked, err := repo.MarkConsumed(ctx, consumed.Inner())
	if err != nil {
		t.Fatalf("MarkConsumed: %v", err)
	}
	if marked.Status != types.MessageStatusConsumedTyped || marked.ConsumedBy != "member-dest" {
		t.Fatalf("marked message = %+v", marked)
	}
	if _, err := repo.MarkConsumed(ctx, consumed.Inner()); err == nil {
		t.Fatalf("expected second MarkConsumed to fail")
	}
}

func TestSQLiteRepository_MessageListAndCount(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := newSQLiteRepoForTest(t)
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	for _, item := range []struct {
		id     types.AgentMessageID
		intent types.IntentID
		dest   member.ID
	}{
		{"msg-1", "intent-1", "member-a"},
		{"msg-2", "intent-2", "member-b"},
	} {
		msg, err := domain.NewMessage(domain.NewMessageInput{
			ID: item.id,
			Route: domain.MessageRoute{
				SpaceID:             "space-1",
				DestinationMemberID: item.dest,
			},
			Content: domain.MessageContent{
				Kind:    types.AgentMessageKindSystem,
				Subject: "Notice",
				Body:    map[string]any{"message": string(item.id)},
			},
			Producer: domain.MessageProducer{
				IntentID: item.intent,
				Producer: "test",
			},
		}, now)
		if err != nil {
			t.Fatalf("NewMessage: %v", err)
		}
		if _, err := repo.SaveQueued(ctx, msg.Inner()); err != nil {
			t.Fatalf("SaveQueued: %v", err)
		}
	}
	listed, err := repo.List(ctx, domain.MessageFilter{
		SpaceID:             "space-1",
		DestinationMemberID: "member-a",
		Statuses:            []types.MessageStatus{types.MessageStatusQueuedTyped},
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != "msg-1" {
		t.Fatalf("listed = %+v", listed)
	}
	count, err := repo.Count(ctx, domain.MessageFilter{SpaceID: "space-1"})
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 2 {
		t.Fatalf("count=%d", count)
	}
}

func TestSQLiteRepository_ChannelLifecycleAndReads(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := newSQLiteRepoForTest(t)
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)

	ch, err := channel.NewMemberChannel(channel.NewMemberInput{
		SpaceID:   "space-1",
		ProjectID: "project-1",
		MemberID:  "member-1",
	}, now)
	if err != nil {
		t.Fatalf("NewMemberChannel: %v", err)
	}
	saved, err := repo.Save(ctx, ch.Inner())
	if err != nil {
		t.Fatalf("Save channel: %v", err)
	}
	if saved.ID != "channel:space-1:member:member-1" || saved.MemberLabel != "" || saved.Title != "" || saved.RunID != "" {
		t.Fatalf("saved channel owns non-address fields: %+v", saved)
	}
	loaded, err := repo.LoadMemberChannel(ctx, "space-1", "member-1")
	if err != nil {
		t.Fatalf("LoadMemberChannel: %v", err)
	}
	if loaded.ID != saved.ID {
		t.Fatalf("loaded=%+v", loaded)
	}
	if err := repo.RecordActivity(ctx, saved.ID, now.Add(time.Minute)); err != nil {
		t.Fatalf("RecordActivity: %v", err)
	}
	msg, err := domain.NewMessage(domain.NewMessageInput{
		ID: "msg-channel-1",
		Route: domain.MessageRoute{
			SpaceID:             "space-1",
			DestinationMemberID: "member-1",
			ChannelID:           saved.ID,
		},
		Content: domain.MessageContent{
			Kind:    types.AgentMessageKindInform,
			Subject: "Notice",
			Body:    map[string]any{"text": "ready"},
		},
		Producer: domain.MessageProducer{IntentID: "intent-channel-1", Producer: "test"},
	}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}
	if _, err := repo.SaveQueued(ctx, msg.Inner()); err != nil {
		t.Fatalf("SaveQueued channel message: %v", err)
	}
	unread, err := repo.UnreadCountsByChannel(ctx, "user-1", []types.ChannelID{saved.ID})
	if err != nil {
		t.Fatalf("UnreadCountsByChannel: %v", err)
	}
	if unread[saved.ID] != 1 {
		t.Fatalf("unread count=%d want 1", unread[saved.ID])
	}
	if err := repo.MarkRead(ctx, "user-1", saved.ID, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	unread, err = repo.UnreadCountsByChannel(ctx, "user-1", []types.ChannelID{saved.ID})
	if err != nil {
		t.Fatalf("UnreadCountsByChannel after read: %v", err)
	}
	if unread[saved.ID] != 0 {
		t.Fatalf("unread count after read=%d want 0", unread[saved.ID])
	}
}

func newSQLiteRepoForTest(t *testing.T) *SQLiteRepository {
	t.Helper()
	handle, err := storagedb.Open(context.Background(), storagedb.Config{
		Driver:  storagedb.DriverSQLite,
		DataDir: filepath.Join(t.TempDir(), "data"),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	repo, err := NewSQLiteRepository(handle)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	return repo
}
