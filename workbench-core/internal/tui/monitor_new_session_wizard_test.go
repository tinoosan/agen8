package tui

import (
	"context"
	"testing"

	pkgstore "github.com/tinoosan/workbench-core/pkg/store"
	"github.com/tinoosan/workbench-core/pkg/types"
)

type wizardSessionQueryStub struct {
	sessions []types.Session
}

func (s wizardSessionQueryStub) LoadSession(_ context.Context, sessionID string) (types.Session, error) {
	for _, sess := range s.sessions {
		if sess.SessionID == sessionID {
			return sess, nil
		}
	}
	return types.Session{}, nil
}

func (s wizardSessionQueryStub) ListSessionsPaginated(_ context.Context, _ pkgstore.SessionFilter) ([]types.Session, error) {
	return append([]types.Session(nil), s.sessions...), nil
}

func (s wizardSessionQueryStub) CountSessions(_ context.Context, _ pkgstore.SessionFilter) (int, error) {
	return len(s.sessions), nil
}

func TestOpenNewSessionWizard_AllowsNilSessionTimestamps(t *testing.T) {
	m := &monitorModel{
		ctx: context.Background(),
		session: wizardSessionQueryStub{
			sessions: []types.Session{
				{SessionID: "sess-1", Title: "No dates", CreatedAt: nil, UpdatedAt: nil},
			},
		},
	}

	if cmd := m.openNewSessionWizard(); cmd != nil {
		t.Fatalf("expected nil command from openNewSessionWizard, got %#v", cmd)
	}
	if !m.newSessionWizardOpen {
		t.Fatalf("expected wizard to be open")
	}
	if len(m.newSessionWizardList.Items()) < 3 {
		t.Fatalf("expected resume + two creation items, got %d", len(m.newSessionWizardList.Items()))
	}

	first, ok := m.newSessionWizardList.Items()[0].(newSessionWizardItem)
	if !ok {
		t.Fatalf("expected wizard item type, got %T", m.newSessionWizardList.Items()[0])
	}
	if first.mode != "resume" {
		t.Fatalf("expected first item to be resume, got %q", first.mode)
	}
}
