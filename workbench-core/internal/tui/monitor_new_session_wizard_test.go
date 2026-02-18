package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
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

func TestOpenNewSessionWizard_StartsAtStep0(t *testing.T) {
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
	if m.newSessionWizardStep != 0 {
		t.Fatalf("expected wizard step 0, got %d", m.newSessionWizardStep)
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

func TestWizard_ResumeItemsContainRichMetadata(t *testing.T) {
	now := time.Now()
	m := &monitorModel{
		ctx: context.Background(),
		session: wizardSessionQueryStub{
			sessions: []types.Session{
				{
					SessionID:   "sess-rich",
					Title:       "My Research",
					Mode:        "standalone",
					Profile:     "market_researcher",
					ActiveModel: "openrouter/gpt-5",
					UpdatedAt:   &now,
				},
			},
		},
	}

	m.openNewSessionWizard()

	first, ok := m.newSessionWizardList.Items()[0].(newSessionWizardItem)
	if !ok {
		t.Fatalf("expected wizard item type")
	}
	if first.mode != "resume" {
		t.Fatalf("expected resume mode, got %q", first.mode)
	}

	desc := first.desc
	if !strings.Contains(desc, "standalone") {
		t.Errorf("expected description to contain mode, got %q", desc)
	}
	if !strings.Contains(desc, "market_researcher") {
		t.Errorf("expected description to contain profile, got %q", desc)
	}
	if !strings.Contains(desc, "openrouter/gpt-5") {
		t.Errorf("expected description to contain model, got %q", desc)
	}
	if !strings.Contains(desc, "ago") {
		t.Errorf("expected description to contain time ago, got %q", desc)
	}
}

func TestWizard_StandaloneTransitionsToProfileStep(t *testing.T) {
	m := &monitorModel{
		ctx:     context.Background(),
		session: wizardSessionQueryStub{},
	}

	m.openNewSessionWizard()

	// Select "New Standalone Session" — it's the first item when there are no sessions.
	first, ok := m.newSessionWizardList.Items()[0].(newSessionWizardItem)
	if !ok || first.mode != "standalone" {
		t.Fatalf("expected standalone item first when no sessions, got %+v", first)
	}

	m.newSessionWizardList.Select(0)
	m.updateNewSessionWizardModeStep(tea.KeyMsg{Type: tea.KeyEnter})

	if m.newSessionWizardStep != 1 {
		t.Fatalf("expected step 1 after selecting standalone, got %d", m.newSessionWizardStep)
	}
	if m.newSessionWizardMode != "standalone" {
		t.Fatalf("expected mode 'standalone', got %q", m.newSessionWizardMode)
	}
}

func TestWizard_TeamTransitionsToProfileStep(t *testing.T) {
	m := &monitorModel{
		ctx:     context.Background(),
		session: wizardSessionQueryStub{},
	}

	m.openNewSessionWizard()

	// "New Team Session" is the second item when there are no sessions.
	m.newSessionWizardList.Select(1)
	item, ok := m.newSessionWizardList.SelectedItem().(newSessionWizardItem)
	if !ok || item.mode != "team" {
		t.Fatalf("expected team item at index 1 when no sessions, got %+v", item)
	}

	m.updateNewSessionWizardModeStep(tea.KeyMsg{Type: tea.KeyEnter})

	if m.newSessionWizardStep != 1 {
		t.Fatalf("expected step 1 after selecting team, got %d", m.newSessionWizardStep)
	}
	if m.newSessionWizardMode != "team" {
		t.Fatalf("expected mode 'team', got %q", m.newSessionWizardMode)
	}
}

func TestWizard_BackFromProfileReturnsToModeStep(t *testing.T) {
	m := &monitorModel{
		ctx:     context.Background(),
		session: wizardSessionQueryStub{},
	}

	m.openNewSessionWizard()
	m.newSessionWizardList.Select(0)
	m.updateNewSessionWizardModeStep(tea.KeyMsg{Type: tea.KeyEnter})

	if m.newSessionWizardStep != 1 {
		t.Fatalf("expected step 1, got %d", m.newSessionWizardStep)
	}

	// Press Esc to go back.
	m.updateNewSessionWizardProfileStep(tea.KeyMsg{Type: tea.KeyEsc})

	if m.newSessionWizardStep != 0 {
		t.Fatalf("expected step 0 after back, got %d", m.newSessionWizardStep)
	}
	if !m.newSessionWizardOpen {
		t.Fatal("expected wizard to still be open after back")
	}
	if m.newSessionWizardMode != "" {
		t.Fatalf("expected mode reset after back, got %q", m.newSessionWizardMode)
	}
}

func TestWizard_EscOnStep0ClosesWizard(t *testing.T) {
	m := &monitorModel{
		ctx:     context.Background(),
		session: wizardSessionQueryStub{},
	}

	m.openNewSessionWizard()
	if !m.newSessionWizardOpen {
		t.Fatal("expected wizard open")
	}

	m.updateNewSessionWizardModeStep(tea.KeyMsg{Type: tea.KeyEsc})

	if m.newSessionWizardOpen {
		t.Fatal("expected wizard closed after Esc on step 0")
	}
}

func TestWizard_SelectProfileFromWizard(t *testing.T) {
	m := &monitorModel{
		ctx:     context.Background(),
		session: wizardSessionQueryStub{},
	}

	m.openNewSessionWizard()
	m.newSessionWizardMode = "standalone"
	m.newSessionWizardStep = 1

	// Manually inject a profile item into the profile list.
	listItems := []list.Item{
		monitorProfilePickerItem{ref: "general", id: "general", description: "General purpose"},
	}
	m.newSessionWizardProfileList = list.New(listItems, list.NewDefaultDelegate(), 80, 20)
	m.newSessionWizardProfileList.Select(0)

	cmd := m.selectProfileFromWizard()

	if m.newSessionWizardOpen {
		t.Fatal("expected wizard closed after profile selection")
	}
	if m.profile != "general" {
		t.Fatalf("expected profile 'general', got %q", m.profile)
	}
	if cmd == nil {
		t.Fatal("expected non-nil command after profile selection")
	}
}
