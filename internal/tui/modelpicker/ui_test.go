package modelpicker

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestController_EscBehavior(t *testing.T) {
	var c Controller
	_ = c.Open("tcp://127.0.0.1:1", "sess-1")

	_, _ = c.Update(listLoadedMsg{reqID: c.pendingReqID, providerView: true, items: []pickerListItem{{provider: "openai", isProvider: true, count: 1}}})
	if !c.providerView {
		t.Fatalf("expected provider view")
	}

	cmd, ev := c.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if ev.Type != EventNone {
		t.Fatalf("unexpected event: %s", ev.Type)
	}
	if cmd == nil {
		t.Fatalf("expected fetch command when selecting provider")
	}

	_, _ = c.Update(listLoadedMsg{reqID: c.pendingReqID, providerView: false, provider: "openai", items: []pickerListItem{{id: "openai/gpt-5", provider: "openai"}}})
	if c.providerView {
		t.Fatalf("expected model view")
	}

	cmd, ev = c.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if ev.Type != EventNone {
		t.Fatalf("unexpected event on esc back: %s", ev.Type)
	}
	if cmd == nil {
		t.Fatalf("expected fetch command when escaping to provider view")
	}

	_, _ = c.Update(listLoadedMsg{reqID: c.pendingReqID, providerView: true, items: []pickerListItem{{provider: "openai", isProvider: true, count: 1}}})
	cmd, ev = c.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		t.Fatalf("expected no fetch command when closing picker")
	}
	if ev.Type != EventClosed {
		t.Fatalf("expected closed event, got %s", ev.Type)
	}
}

func TestController_ModelSelectionEvent(t *testing.T) {
	var c Controller
	_ = c.Open("tcp://127.0.0.1:1", "sess-1")
	_, _ = c.Update(listLoadedMsg{reqID: c.pendingReqID, providerView: false, provider: "openai", items: []pickerListItem{{id: "openai/gpt-5", provider: "openai"}}})

	cmd, ev := c.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatalf("expected no follow-up command after selecting model")
	}
	if ev.Type != EventModelSelected {
		t.Fatalf("expected model selected event, got %s", ev.Type)
	}
	if ev.ModelID != "openai/gpt-5" {
		t.Fatalf("unexpected model id: %q", ev.ModelID)
	}
}

func TestController_SetSizeClampsListHeight(t *testing.T) {
	var c Controller
	_ = c.Open("tcp://127.0.0.1:1", "sess-1")
	_ = c.SetSize(40, 10)
	if c.list.Height() < 4 {
		t.Fatalf("expected list height clamp >= 4, got %d", c.list.Height())
	}
}

func TestController_SetSizeWhenClosedDoesNotPanic(t *testing.T) {
	var c Controller
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("SetSize panicked while picker closed: %v", r)
		}
	}()
	_ = c.SetSize(120, 40)
}

func TestController_IgnoresStaleListLoadedMsg(t *testing.T) {
	var c Controller
	_ = c.Open("tcp://127.0.0.1:1", "sess-1")

	latestReqID := c.pendingReqID
	_, _ = c.Update(tea.KeyMsg{Runes: []rune{'g'}})
	latestReqID = c.pendingReqID

	staleReqID := latestReqID - 1
	_, _ = c.Update(listLoadedMsg{
		reqID:        staleReqID,
		providerView: true,
		query:        "",
		items:        []pickerListItem{{provider: "anthropic", isProvider: true, count: 1}},
	})

	if !c.loading {
		t.Fatalf("expected stale response to be ignored while latest request is pending")
	}

	_, _ = c.Update(listLoadedMsg{
		reqID:        latestReqID,
		providerView: true,
		query:        "g",
		items:        []pickerListItem{{provider: "openai", isProvider: true, count: 1}},
	})

	if c.loading {
		t.Fatalf("expected latest response to clear loading state")
	}
	if c.query != "g" {
		t.Fatalf("expected query to remain latest value, got %q", c.query)
	}
	if len(c.lastItems) != 1 || c.lastItems[0].Provider != "openai" {
		t.Fatalf("expected latest items to be applied, got %+v", c.lastItems)
	}
}
