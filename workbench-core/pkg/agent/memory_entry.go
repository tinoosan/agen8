package agent

import (
	"fmt"
	"strings"
	"time"
)

type MemoryCategory string

const (
	MemoryCategoryPreference MemoryCategory = "preference"
	MemoryCategoryCorrection MemoryCategory = "correction"
	MemoryCategoryDecision   MemoryCategory = "decision"
	MemoryCategoryPattern    MemoryCategory = "pattern"
	MemoryCategoryConstraint MemoryCategory = "constraint"
	MemoryCategoryBlocker    MemoryCategory = "blocker"
	MemoryCategoryHandoff    MemoryCategory = "handoff"
	MemoryCategoryContext    MemoryCategory = "context"
)

func ValidMemoryCategories() []MemoryCategory {
	return []MemoryCategory{
		MemoryCategoryPreference,
		MemoryCategoryCorrection,
		MemoryCategoryDecision,
		MemoryCategoryPattern,
		MemoryCategoryConstraint,
		MemoryCategoryBlocker,
		MemoryCategoryHandoff,
		MemoryCategoryContext,
	}
}

type MemoryEntry struct {
	Time     time.Time
	Category MemoryCategory
	Content  string
}

func NewMemoryEntry(t time.Time, category MemoryCategory, content string) (MemoryEntry, error) {
	if t.IsZero() {
		return MemoryEntry{}, fmt.Errorf("time is required")
	}
	cat, err := normalizeMemoryCategory(category)
	if err != nil {
		return MemoryEntry{}, err
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return MemoryEntry{}, fmt.Errorf("content is required")
	}
	return MemoryEntry{Time: t, Category: cat, Content: content}, nil
}

func (e MemoryEntry) FormatLine() string {
	return fmt.Sprintf("%s | %s | %s", e.Time.Format("15:04"), string(e.Category), strings.TrimSpace(e.Content))
}

func ParseMemoryLine(line string) (MemoryEntry, error) {
	parts := strings.SplitN(strings.TrimSpace(line), "|", 3)
	if len(parts) != 3 {
		return MemoryEntry{}, fmt.Errorf("memory line must match HH:MM | category | content")
	}
	timePart := strings.TrimSpace(parts[0])
	catPart := strings.TrimSpace(parts[1])
	content := strings.TrimSpace(parts[2])
	if content == "" {
		return MemoryEntry{}, fmt.Errorf("memory content is required")
	}
	parsedClock, err := time.Parse("15:04", timePart)
	if err != nil {
		return MemoryEntry{}, fmt.Errorf("invalid memory timestamp %q: %w", timePart, err)
	}
	cat, err := normalizeMemoryCategory(MemoryCategory(catPart))
	if err != nil {
		return MemoryEntry{}, err
	}
	now := time.Now()
	entryTime := time.Date(now.Year(), now.Month(), now.Day(), parsedClock.Hour(), parsedClock.Minute(), 0, 0, now.Location())
	return MemoryEntry{Time: entryTime, Category: cat, Content: content}, nil
}

func IsDuplicate(entry MemoryEntry, existingContent string) bool {
	target := dedupKey(entry.Category, entry.Content)
	if target == "" {
		return false
	}
	lines := strings.Split(strings.ReplaceAll(existingContent, "\r\n", "\n"), "\n")
	for _, ln := range lines {
		parsed, err := ParseMemoryLine(ln)
		if err != nil {
			continue
		}
		if dedupKey(parsed.Category, parsed.Content) == target {
			return true
		}
	}
	return false
}

func dedupKey(category MemoryCategory, content string) string {
	cat := strings.ToLower(strings.TrimSpace(string(category)))
	ct := strings.ToLower(strings.TrimSpace(content))
	if cat == "" || ct == "" {
		return ""
	}
	return cat + "|" + ct
}

func normalizeMemoryCategory(category MemoryCategory) (MemoryCategory, error) {
	candidate := strings.ToLower(strings.TrimSpace(string(category)))
	for _, valid := range ValidMemoryCategories() {
		if candidate == string(valid) {
			return valid, nil
		}
	}
	vals := make([]string, 0, len(ValidMemoryCategories()))
	for _, valid := range ValidMemoryCategories() {
		vals = append(vals, string(valid))
	}
	return "", fmt.Errorf("invalid memory category %q; valid categories: %s", category, strings.Join(vals, ", "))
}
