package agent

import (
	"strings"
	"testing"
	"time"
)

func TestNewMemoryEntry_ValidatesCategorySet(t *testing.T) {
	now := time.Now()
	for _, cat := range ValidMemoryCategories() {
		if _, err := NewMemoryEntry(now, cat, "value"); err != nil {
			t.Fatalf("NewMemoryEntry(%s) error: %v", cat, err)
		}
	}

	if _, err := NewMemoryEntry(now, MemoryCategory("goal"), "x"); err == nil {
		t.Fatalf("expected goal category to be rejected")
	}
	if _, err := NewMemoryEntry(now, MemoryCategory("fact"), "x"); err == nil {
		t.Fatalf("expected fact category to be rejected")
	}
}

func TestMemoryEntry_FormatParse_RoundTrip(t *testing.T) {
	in, err := NewMemoryEntry(time.Date(2026, 2, 20, 14, 5, 0, 0, time.Local), MemoryCategoryDecision, "Adopt wrapper")
	if err != nil {
		t.Fatalf("NewMemoryEntry: %v", err)
	}
	line := in.FormatLine()
	parsed, err := ParseMemoryLine(line)
	if err != nil {
		t.Fatalf("ParseMemoryLine: %v", err)
	}
	if parsed.Time.Format("15:04") != "14:05" {
		t.Fatalf("time mismatch: got %s", parsed.Time.Format("15:04"))
	}
	if parsed.Category != MemoryCategoryDecision {
		t.Fatalf("category mismatch: got %s", parsed.Category)
	}
	if parsed.Content != "Adopt wrapper" {
		t.Fatalf("content mismatch: got %q", parsed.Content)
	}
}

func TestIsDuplicate_IgnoresTimestamp_AndCase(t *testing.T) {
	entry, err := NewMemoryEntry(time.Now(), MemoryCategoryPattern, "Use wrapper decorators")
	if err != nil {
		t.Fatalf("NewMemoryEntry: %v", err)
	}
	existing := strings.Join([]string{
		"08:10 | pattern | use wrapper decorators",
		"legacy markdown",
	}, "\n")
	if !IsDuplicate(entry, existing) {
		t.Fatalf("expected duplicate")
	}

	notDup, err := NewMemoryEntry(time.Now(), MemoryCategoryPattern, "different value")
	if err != nil {
		t.Fatalf("NewMemoryEntry: %v", err)
	}
	if IsDuplicate(notDup, existing) {
		t.Fatalf("did not expect duplicate")
	}
}

func TestParseMemoryLine_InvalidShape(t *testing.T) {
	if _, err := ParseMemoryLine("bad line"); err == nil {
		t.Fatalf("expected invalid line error")
	}
}
