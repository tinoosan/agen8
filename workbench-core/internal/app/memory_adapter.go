package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/pkg/agent"
	"github.com/tinoosan/workbench-core/pkg/store"
)

type textMemoryAdapter struct {
	store store.DailyMemoryStore
}

func (t *textMemoryAdapter) Search(ctx context.Context, query string, limit int) ([]agent.MemorySnippet, error) {
	if t == nil || t.store == nil {
		return nil, nil
	}

	files, err := t.store.ListMemoryFiles(ctx)
	if err != nil {
		return nil, err
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	qLower := strings.ToLower(query)
	if limit <= 0 {
		limit = 5
	}

	out := make([]agent.MemorySnippet, 0, limit)
	for _, name := range files {
		if len(out) >= limit {
			break
		}
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		// DailyMemoryStore only reads by date; ignore non-daily files for recall.
		if !strings.HasSuffix(name, "-memory.md") || len(name) < len("2006-01-02-memory.md") {
			continue
		}
		date := strings.TrimSuffix(name, "-memory.md")
		text, err := t.store.ReadMemory(ctx, date)
		if err != nil {
			continue
		}
		lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
		bestLine := ""
		bestScore := 0.0
		for _, ln := range lines {
			lnTrim := strings.TrimSpace(ln)
			if lnTrim == "" {
				continue
			}
			if strings.Contains(strings.ToLower(lnTrim), qLower) {
				bestLine = lnTrim
				bestScore = 1.0
				break
			}
		}
		if bestLine == "" {
			continue
		}
		out = append(out, agent.MemorySnippet{
			Title:    name,
			Filename: name,
			Content:  bestLine,
			Score:    bestScore,
		})
	}
	return out, nil
}

func (t *textMemoryAdapter) Save(ctx context.Context, title, content string) error {
	if t == nil || t.store == nil {
		return nil
	}
	title = strings.TrimSpace(title)
	content = strings.TrimSpace(content)
	if title == "" && content == "" {
		return nil
	}
	if title == "" {
		title = "note"
	}
	body := fmt.Sprintf("\n\n## %s\n\n%s\n", title, content)
	today := time.Now().UTC().Format("2006-01-02")
	return t.store.AppendMemory(ctx, today, body)
}
