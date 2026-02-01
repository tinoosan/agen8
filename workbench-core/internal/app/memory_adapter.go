package app

import (
	"context"
	"strings"

	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/pkg/agent"
)

type vectorMemoryAdapter struct {
	store *store.VectorMemoryStore
}

func (v *vectorMemoryAdapter) Search(ctx context.Context, query string, limit int) ([]agent.MemorySnippet, error) {
	if v == nil || v.store == nil {
		return nil, nil
	}
	results, err := v.store.Search(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	out := make([]agent.MemorySnippet, 0, len(results))
	for _, r := range results {
		filename := strings.TrimSpace(r.Path)
		filename = strings.TrimPrefix(filename, "/memory/")
		out = append(out, agent.MemorySnippet{
			Title:    r.Title,
			Filename: filename,
			Content:  r.Snippet,
			Score:    r.Score,
		})
	}
	return out, nil
}

func (v *vectorMemoryAdapter) Save(ctx context.Context, title, content string) error {
	if v == nil || v.store == nil {
		return nil
	}
	_, err := v.store.Save(ctx, title, content)
	return err
}
