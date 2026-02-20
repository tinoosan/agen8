package agent

import "context"

type MemorySnippet struct {
	Title    string
	Filename string
	Content  string
	Score    float64
}

// MemoryRecallProvider provides best-effort semantic recall and persistence for the agent.
// This is distinct from the daily memory file store (`store.DailyMemoryStore`).
type MemoryRecallProvider interface {
	Search(ctx context.Context, query string, limit int) ([]MemorySnippet, error)
	Save(ctx context.Context, title, content string) error
}
