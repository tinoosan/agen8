package soul

import (
	"context"
	"strings"
	"testing"
)

func TestSeedAndUpdate(t *testing.T) {
	svc := NewService(t.TempDir())
	doc, err := svc.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if doc.Version != 1 || !strings.Contains(doc.Content, "## Constitutional Core") {
		t.Fatalf("unexpected seeded doc: %+v", doc)
	}
	updatedContent := strings.Replace(doc.Content, "Serve operator-defined outcomes with durable memory and accountable autonomy.", "Serve operator goals with accountable autonomy and continuity.", 1)
	updated, err := svc.Update(context.Background(), UpdateRequest{Content: updatedContent, Reason: "adapt intent", Actor: ActorAgent, ExpectedVersion: 1})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Version != 2 {
		t.Fatalf("version=%d, want 2", updated.Version)
	}
}

func TestAgentCannotEditImmutableSections(t *testing.T) {
	svc := NewService(t.TempDir())
	doc, err := svc.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	bad := strings.Replace(doc.Content, "The system must operate within explicit policy and safety bounds.", "Changed immutable core.", 1)
	_, err = svc.Update(context.Background(), UpdateRequest{Content: bad, Reason: "mutate core", Actor: ActorAgent, ExpectedVersion: 1})
	if err == nil {
		t.Fatalf("expected policy violation")
	}
}
