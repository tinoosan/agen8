package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/resources"
)

func TestParagraphChunker_SplitsParagraphs(t *testing.T) {
	chunker := &ParagraphChunker{MaxChunkSize: 1000}
	content := "First paragraph.\n\nSecond paragraph."
	chunks := chunker.Chunk(content)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	if chunks[0].Content != "First paragraph." {
		t.Fatalf("unexpected first chunk: %q", chunks[0].Content)
	}
	if chunks[1].Content != "Second paragraph." {
		t.Fatalf("unexpected second chunk: %q", chunks[1].Content)
	}
}

func TestTimeBasedChunker_SplitsByTimestamp(t *testing.T) {
	chunker := &TimeBasedChunker{MaxChunkSize: 1000}
	content := "09:00 Start\nNote.\n10:00 Next\nMore."
	chunks := chunker.Chunk(content)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	if chunks[0].Content[:5] != "09:00" {
		t.Fatalf("unexpected first chunk: %q", chunks[0].Content)
	}
	if chunks[1].Content[:5] != "10:00" {
		t.Fatalf("unexpected second chunk: %q", chunks[1].Content)
	}
}

func TestDeleteEmbeddingsForFile_RemovesRows(t *testing.T) {
	store := newTestVectorStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339Nano)

	if _, err := store.db.Exec(
		`INSERT INTO memories (memory_id, title, filename, source_file, chunk_index, content, created_at, indexed_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"mem-1", "title", "file.md", "file.md", 0, "content", now, now,
	); err != nil {
		t.Fatalf("insert memory: %v", err)
	}
	if _, err := store.db.Exec(
		`INSERT INTO memory_embeddings (memory_id, dim, embedding) VALUES (?, ?, ?)`,
		"mem-1", 1, []byte{0, 0, 0, 0},
	); err != nil {
		t.Fatalf("insert embedding: %v", err)
	}

	if err := store.DeleteEmbeddingsForFile(ctx, "file.md"); err != nil {
		t.Fatalf("delete embeddings: %v", err)
	}
	if got := countMemoriesBySource(t, store.db, "file.md"); got != 0 {
		t.Fatalf("expected 0 memories, got %d", got)
	}
}

func TestReindexDailyFile_InsertsChunks(t *testing.T) {
	store := newTestVectorStore(t)
	ctx := context.Background()

	content := []byte("First paragraph.\n\nSecond paragraph.")
	if err := store.ReindexDailyFile(ctx, "2026-01-31-memory.md", content); err != nil {
		t.Fatalf("reindex: %v", err)
	}
	if got := countMemoriesBySource(t, store.db, "2026-01-31-memory.md"); got != 2 {
		t.Fatalf("expected 2 memories, got %d", got)
	}
	if got := countEmbeddings(t, store.db); got != 2 {
		t.Fatalf("expected 2 embeddings, got %d", got)
	}
}

func TestDailyMemoryResource_ReindexAndSearch(t *testing.T) {
	store := newTestVectorStore(t)
	memRes, err := resources.NewDailyMemoryResource(store.memoryDir, store, nil)
	if err != nil {
		t.Fatalf("create memory resource: %v", err)
	}
	name := time.Now().Format("2006-01-02") + "-memory.md"
	if err := memRes.Write(name, []byte("Alpha memory entry")); err != nil {
		t.Fatalf("write memory: %v", err)
	}
	results, err := store.Search(context.Background(), "Alpha", 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected search results, got 0")
	}
	if !strings.Contains(results[0].Snippet, "Alpha") {
		t.Fatalf("unexpected search content: %q", results[0].Snippet)
	}
}

func newTestVectorStore(t *testing.T) *VectorMemoryStore {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Config{DataDir: dir}
	db, err := getSQLiteDB(cfg)
	if err != nil {
		t.Fatalf("get db: %v", err)
	}
	memDir := filepath.Join(dir, "memory")
	if err := os.MkdirAll(memDir, 0755); err != nil {
		t.Fatalf("create mem dir: %v", err)
	}
	return &VectorMemoryStore{
		cfg:       cfg,
		db:        db,
		memoryDir: memDir,
		embedFn: func(_ context.Context, _ string) ([]float32, error) {
			return []float32{1.0}, nil
		},
	}
}

func countMemoriesBySource(t *testing.T, db *sql.DB, source string) int {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM memories WHERE source_file = ?`, source).Scan(&count); err != nil {
		t.Fatalf("count memories: %v", err)
	}
	return count
}

func countEmbeddings(t *testing.T, db *sql.DB) int {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM memory_embeddings`).Scan(&count); err != nil {
		t.Fatalf("count embeddings: %v", err)
	}
	return count
}
