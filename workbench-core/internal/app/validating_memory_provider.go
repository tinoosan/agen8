package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/pkg/agent"
	pkgstore "github.com/tinoosan/workbench-core/pkg/store"
)

type validatingMemoryProvider struct {
	inner agent.MemoryRecallProvider
	store pkgstore.DailyMemoryStore
	now   func() time.Time
}

func (v *validatingMemoryProvider) Search(ctx context.Context, query string, limit int) ([]agent.MemorySnippet, error) {
	if v == nil || v.inner == nil {
		return nil, nil
	}
	return v.inner.Search(ctx, query, limit)
}

func (v *validatingMemoryProvider) Save(ctx context.Context, title, content string) error {
	if v == nil || v.store == nil {
		return nil
	}
	title = strings.TrimSpace(title)
	content = strings.TrimSpace(content)
	if title == "" && content == "" {
		return nil
	}
	category, err := categoryFromTitle(title)
	if err != nil {
		return err
	}
	now := time.Now
	if v.now != nil {
		now = v.now
	}
	entry, err := agent.NewMemoryEntry(now(), category, content)
	if err != nil {
		return err
	}
	today := now().Format("2006-01-02")
	existing, err := v.store.ReadMemory(ctx, today)
	if err != nil {
		return err
	}
	if agent.IsDuplicate(entry, existing) {
		return nil
	}
	return v.store.AppendMemory(ctx, today, entry.FormatLine()+"\n")
}

func categoryFromTitle(title string) (agent.MemoryCategory, error) {
	n := normalizeMemoryTitle(title)
	if n == "" {
		return agent.MemoryCategoryContext, nil
	}
	if isGoalLike(n) {
		return "", fmt.Errorf("memory category %q belongs in SOUL.md (goals/intent), not /memory", title)
	}
	if isKnowledgeLike(n) {
		return "", fmt.Errorf("memory category %q belongs in /knowledge (future mount), not /memory", title)
	}
	switch n {
	case string(agent.MemoryCategoryPreference):
		return agent.MemoryCategoryPreference, nil
	case string(agent.MemoryCategoryCorrection):
		return agent.MemoryCategoryCorrection, nil
	case string(agent.MemoryCategoryDecision):
		return agent.MemoryCategoryDecision, nil
	case string(agent.MemoryCategoryPattern):
		return agent.MemoryCategoryPattern, nil
	case string(agent.MemoryCategoryConstraint):
		return agent.MemoryCategoryConstraint, nil
	case string(agent.MemoryCategoryBlocker):
		return agent.MemoryCategoryBlocker, nil
	case string(agent.MemoryCategoryHandoff):
		return agent.MemoryCategoryHandoff, nil
	case string(agent.MemoryCategoryContext):
		return agent.MemoryCategoryContext, nil
	default:
		return agent.MemoryCategoryContext, nil
	}
}

func normalizeMemoryTitle(in string) string {
	n := strings.ToLower(strings.TrimSpace(in))
	n = strings.ReplaceAll(n, "_", "-")
	n = strings.ReplaceAll(n, " ", "-")
	for strings.Contains(n, "--") {
		n = strings.ReplaceAll(n, "--", "-")
	}
	return strings.Trim(n, "-")
}

func isGoalLike(n string) bool {
	switch n {
	case "goal", "goals", "objective", "objectives", "mission", "north-star", "northstar":
		return true
	default:
		return false
	}
}

func isKnowledgeLike(n string) bool {
	switch n {
	case "fact", "facts", "knowledge", "learned", "lesson", "lessons":
		return true
	default:
		return false
	}
}
