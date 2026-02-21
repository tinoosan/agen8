package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/agen8/pkg/agent"
)

type fakeMemoryProvider struct {
	searchCalled bool
	searchOut    []agent.MemorySnippet
	searchErr    error
}

func (f *fakeMemoryProvider) Search(ctx context.Context, query string, limit int) ([]agent.MemorySnippet, error) {
	f.searchCalled = true
	return f.searchOut, f.searchErr
}

func (f *fakeMemoryProvider) Save(ctx context.Context, title, content string) error {
	return nil
}

type fakeDailyMemoryStore struct {
	readText      string
	readErr       error
	appendErr     error
	appended      []string
	lastReadDate  string
	lastAppendDay string
}

func (f *fakeDailyMemoryStore) BaseDir() string { return "" }

func (f *fakeDailyMemoryStore) ReadMemory(ctx context.Context, date string) (string, error) {
	f.lastReadDate = date
	return f.readText, f.readErr
}

func (f *fakeDailyMemoryStore) WriteMemory(ctx context.Context, date string, text string) error {
	return nil
}

func (f *fakeDailyMemoryStore) AppendMemory(ctx context.Context, date string, text string) error {
	f.lastAppendDay = date
	f.appended = append(f.appended, text)
	return f.appendErr
}

func (f *fakeDailyMemoryStore) ListMemoryFiles(ctx context.Context) ([]string, error) {
	return nil, nil
}

func TestValidatingMemoryProvider_SearchDelegates(t *testing.T) {
	inner := &fakeMemoryProvider{searchOut: []agent.MemorySnippet{{Title: "x"}}}
	p := &validatingMemoryProvider{inner: inner}
	out, err := p.Search(context.Background(), "q", 1)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !inner.searchCalled {
		t.Fatalf("expected inner search to be called")
	}
	if len(out) != 1 {
		t.Fatalf("expected one result")
	}
}

func TestValidatingMemoryProvider_Save_MapsAndAppends(t *testing.T) {
	store := &fakeDailyMemoryStore{}
	p := &validatingMemoryProvider{
		inner: &fakeMemoryProvider{},
		store: store,
		now: func() time.Time {
			return time.Date(2026, 2, 20, 9, 7, 0, 0, time.Local)
		},
	}
	if err := p.Save(context.Background(), "constraint", "Stay within budget"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if len(store.appended) != 1 {
		t.Fatalf("expected one append, got %d", len(store.appended))
	}
	if got := strings.TrimSpace(store.appended[0]); got != "09:07 | constraint | Stay within budget" {
		t.Fatalf("unexpected append payload: %q", got)
	}
}

func TestValidatingMemoryProvider_Save_DuplicateSkipped(t *testing.T) {
	store := &fakeDailyMemoryStore{readText: "07:00 | blocker | waiting on API key\n"}
	p := &validatingMemoryProvider{
		inner: &fakeMemoryProvider{},
		store: store,
		now: func() time.Time {
			return time.Date(2026, 2, 20, 11, 30, 0, 0, time.Local)
		},
	}
	if err := p.Save(context.Background(), "blocker", "Waiting on api key"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if len(store.appended) != 0 {
		t.Fatalf("expected duplicate to be skipped")
	}
}

func TestValidatingMemoryProvider_Save_UnknownFallsBackToContext(t *testing.T) {
	store := &fakeDailyMemoryStore{}
	p := &validatingMemoryProvider{
		inner: &fakeMemoryProvider{},
		store: store,
		now: func() time.Time {
			return time.Date(2026, 2, 20, 8, 0, 0, 0, time.Local)
		},
	}
	if err := p.Save(context.Background(), "random-title", "something useful"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got := strings.TrimSpace(store.appended[0]); !strings.Contains(got, " | context | ") {
		t.Fatalf("expected context fallback, got %q", got)
	}
}

func TestValidatingMemoryProvider_Save_RoutesGoalAndKnowledge(t *testing.T) {
	store := &fakeDailyMemoryStore{}
	p := &validatingMemoryProvider{
		inner: &fakeMemoryProvider{},
		store: store,
		now: func() time.Time {
			return time.Date(2026, 2, 20, 8, 0, 0, 0, time.Local)
		},
	}
	if err := p.Save(context.Background(), "goal", "ship this"); err == nil || !strings.Contains(err.Error(), "SOUL.md") {
		t.Fatalf("expected SOUL.md routing error, got %v", err)
	}
	if err := p.Save(context.Background(), "knowledge", "x"); err == nil || !strings.Contains(err.Error(), "/knowledge") {
		t.Fatalf("expected /knowledge routing error, got %v", err)
	}
}
