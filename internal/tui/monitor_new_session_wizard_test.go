package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/fsutil"
	pkgsession "github.com/tinoosan/agen8/pkg/services/session"
	pkgstore "github.com/tinoosan/agen8/pkg/store"
	"github.com/tinoosan/agen8/pkg/types"
)

// wizardSessionQueryStub implements pkgsession.Service for wizard tests.
// Only ListSessionsPaginated, CountSessions, and LoadSession are used; others return stub values.
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

func (s wizardSessionQueryStub) SaveSession(_ context.Context, _ types.Session) error { return nil }

func (s wizardSessionQueryStub) Start(_ context.Context, _ pkgsession.StartOptions) (types.Session, types.Run, error) {
	return types.Session{}, types.Run{}, errors.New("stub: Start not implemented")
}

func (s wizardSessionQueryStub) Delete(_ context.Context, _ string) error { return nil }

func (s wizardSessionQueryStub) ListSessionsPaginated(_ context.Context, _ pkgstore.SessionFilter) ([]types.Session, error) {
	return append([]types.Session(nil), s.sessions...), nil
}

func (s wizardSessionQueryStub) CountSessions(_ context.Context, _ pkgstore.SessionFilter) (int, error) {
	return len(s.sessions), nil
}

func (s wizardSessionQueryStub) LoadRun(_ context.Context, _ string) (types.Run, error) {
	return types.Run{}, errors.New("stub: LoadRun not implemented")
}

func (s wizardSessionQueryStub) SaveRun(_ context.Context, _ types.Run) error { return nil }

func (s wizardSessionQueryStub) StopRun(_ context.Context, _, _, _ string) (types.Run, error) {
	return types.Run{}, errors.New("stub: StopRun not implemented")
}

func (s wizardSessionQueryStub) ListRunsBySession(_ context.Context, _ string) ([]types.Run, error) {
	return nil, nil
}

func (s wizardSessionQueryStub) ListRunsByStatus(_ context.Context, _ []string) ([]types.Run, error) {
	return nil, nil
}

func (s wizardSessionQueryStub) ListChildRuns(_ context.Context, _ string) ([]types.Run, error) {
	return nil, nil
}

func (s wizardSessionQueryStub) AddRunToSession(_ context.Context, sessionID, runID string) (types.Session, error) {
	return types.Session{}, nil
}

func (s wizardSessionQueryStub) ListActivities(_ context.Context, _ string, _, _ int) ([]types.Activity, error) {
	return nil, nil
}

func (s wizardSessionQueryStub) CountActivities(_ context.Context, _ string) (int, error) {
	return 0, nil
}

func (s wizardSessionQueryStub) LatestRun(_ context.Context) (types.Run, error) {
	return types.Run{}, errors.New("stub: LatestRun not implemented")
}

func (s wizardSessionQueryStub) LatestRunningRun(_ context.Context) (types.Run, error) {
	return types.Run{}, errors.New("stub: LatestRunningRun not implemented")
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
	if len(m.newSessionWizardList.Items()) < 2 {
		t.Fatalf("expected resume + New Session item, got %d", len(m.newSessionWizardList.Items()))
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

func TestWizard_NewSessionTransitionsToProfileStep(t *testing.T) {
	m := &monitorModel{
		ctx:     context.Background(),
		session: wizardSessionQueryStub{},
	}

	m.openNewSessionWizard()

	// "New Session" is the first item when there are no sessions.
	first, ok := m.newSessionWizardList.Items()[0].(newSessionWizardItem)
	if !ok || first.mode != "new" {
		t.Fatalf("expected New Session item first when no sessions, got %+v", first)
	}

	m.newSessionWizardList.Select(0)
	m.updateNewSessionWizardModeStep(tea.KeyMsg{Type: tea.KeyEnter})

	if m.newSessionWizardStep != 1 {
		t.Fatalf("expected step 1 after selecting New Session, got %d", m.newSessionWizardStep)
	}
	if m.newSessionWizardMode != "new" {
		t.Fatalf("expected mode 'new', got %q", m.newSessionWizardMode)
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
	dir := t.TempDir()
	profilesDir := filepath.Join(fsutil.GetProfilesDir(dir), "general")
	if err := os.MkdirAll(profilesDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profilesDir, "profile.yaml"), []byte(
		"id: general\nname: General Agent\ndescription: General purpose\nmodel: gpt-5\nprompts:\n  systemPrompt: hi\n",
	), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	m := &monitorModel{
		ctx:     context.Background(),
		cfg:     config.Config{DataDir: dir},
		session: wizardSessionQueryStub{},
	}

	m.openNewSessionWizard()
	m.newSessionWizardMode = "new"
	m.newSessionWizardStep = 1

	// Manually inject a profile item into the profile list.
	listItems := []list.Item{
		monitorProfilePickerItem{ref: "general", id: "general", name: "General Agent", description: "General purpose"},
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
