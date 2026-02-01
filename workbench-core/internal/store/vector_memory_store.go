package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/types"
)

type MemorySearchResult struct {
	MemoryID   string
	Title      string
	Filename   string
	SourceFile string
	ChunkIndex int
	Content    string
	Score      float64
}

type VectorMemoryStore struct {
	cfg       config.Config
	db        *sql.DB
	memoryDir string

	embedModel string
	emb        openai.EmbeddingService
	embedFn    func(ctx context.Context, text string) ([]float32, error)
}

func NewVectorMemoryStore(cfg config.Config) (*VectorMemoryStore, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	db, err := getSQLiteDB(cfg)
	if err != nil {
		return nil, err
	}
	key := strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))
	if key == "" {
		return nil, fmt.Errorf("OPENROUTER_API_KEY is required for embeddings")
	}
	baseURL := strings.TrimSpace(os.Getenv("OPENROUTER_BASE_URL"))
	if baseURL == "" {
		baseURL = "https://openrouter.ai/api/v1"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	model := strings.TrimSpace(os.Getenv("WORKBENCH_EMBEDDING_MODEL"))
	if model == "" {
		model = openai.EmbeddingModelTextEmbedding3Small
	}
	memDir := filepath.Join(cfg.DataDir, "memory")
	if err := os.MkdirAll(memDir, 0755); err != nil {
		return nil, fmt.Errorf("create memory dir: %w", err)
	}
	return &VectorMemoryStore{
		cfg:        cfg,
		db:         db,
		memoryDir:  memDir,
		embedModel: model,
		emb:        openai.NewEmbeddingService(option.WithAPIKey(key), option.WithBaseURL(baseURL)),
	}, nil
}

func (s *VectorMemoryStore) Save(ctx context.Context, title, content string) (MemorySearchResult, error) {
	title = strings.TrimSpace(title)
	content = strings.TrimSpace(content)
	if title == "" || content == "" {
		return MemorySearchResult{}, fmt.Errorf("title and content are required")
	}
	memID := "mem-" + uuid.NewString()
	now := time.Now().UTC()
	created := now.Format(time.RFC3339Nano)

	filename := fmt.Sprintf("%s-%s.md", now.Format("2006-01-02"), slugify(title))
	path := filepath.Join(s.memoryDir, filename)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return MemorySearchResult{}, err
	}

	emb, err := s.embed(ctx, content)
	if err != nil {
		return MemorySearchResult{}, err
	}

	blob, err := float32SliceToBlob(emb)
	if err != nil {
		return MemorySearchResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MemorySearchResult{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO memories (memory_id, title, filename, source_file, chunk_index, content, created_at, indexed_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		memID, title, filename, filename, 0, content, created, created,
	); err != nil {
		return MemorySearchResult{}, err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO memory_embeddings (memory_id, dim, embedding) VALUES (?, ?, ?)`,
		memID, len(emb), blob,
	); err != nil {
		return MemorySearchResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return MemorySearchResult{}, err
	}

	return MemorySearchResult{
		MemoryID: memID,
		Title:    title,
		Filename: filename,
		Content:  content,
		Score:    1.0,
	}, nil
}

func (s *VectorMemoryStore) DeleteEmbeddingsForFile(ctx context.Context, sourceFile string) error {
	sourceFile = strings.TrimSpace(sourceFile)
	if sourceFile == "" {
		return fmt.Errorf("source file is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM memory_embeddings WHERE memory_id IN (SELECT memory_id FROM memories WHERE source_file = ?)`,
		sourceFile,
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM memories WHERE source_file = ?`, sourceFile); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *VectorMemoryStore) ReindexDailyFile(ctx context.Context, filename string, content []byte) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("vector memory store not initialized")
	}
	sourceFile := strings.TrimSpace(filepath.Base(filename))
	if sourceFile == "" {
		return fmt.Errorf("source file is required")
	}
	if err := s.DeleteEmbeddingsForFile(ctx, sourceFile); err != nil {
		return fmt.Errorf("delete old embeddings: %w", err)
	}
	trimmed := strings.TrimSpace(string(content))
	if trimmed == "" {
		return nil
	}

	chunker := &ParagraphChunker{MaxChunkSize: 1000}
	chunks := chunker.Chunk(trimmed)
	if len(chunks) == 0 {
		return nil
	}

	now := time.Now().UTC()
	indexedAt := now.Format(time.RFC3339Nano)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for i, chunk := range chunks {
		if strings.TrimSpace(chunk.Content) == "" {
			continue
		}
		emb, err := s.embed(ctx, chunk.Content)
		if err != nil {
			return fmt.Errorf("embed chunk %d: %w", i, err)
		}
		blob, err := float32SliceToBlob(emb)
		if err != nil {
			return err
		}
		memID := fmt.Sprintf("daily-%s-%s-%d", sourceFile, uuid.NewString(), i)
		title := sourceFile
		filename := sourceFile
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO memories (memory_id, title, filename, source_file, chunk_index, content, created_at, indexed_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			memID, title, filename, sourceFile, i, chunk.Content, indexedAt, indexedAt,
		); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO memory_embeddings (memory_id, dim, embedding) VALUES (?, ?, ?)`,
			memID, len(emb), blob,
		); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *VectorMemoryStore) IndexAllDailyFiles(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("vector memory store not initialized")
	}
	entries, err := os.ReadDir(s.memoryDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !isDailyMemoryFileName(name) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		needs, err := s.needsReindex(ctx, name, info.ModTime())
		if err != nil || !needs {
			continue
		}
		path := filepath.Join(s.memoryDir, name)
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		_ = s.ReindexDailyFile(ctx, name, b)
	}
	return nil
}

func (s *VectorMemoryStore) needsReindex(ctx context.Context, sourceFile string, modTime time.Time) (bool, error) {
	var indexedAt string
	err := s.db.QueryRowContext(ctx,
		`SELECT indexed_at FROM memories WHERE source_file = ? LIMIT 1`,
		sourceFile,
	).Scan(&indexedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return true, nil
		}
		return false, err
	}
	if strings.TrimSpace(indexedAt) == "" {
		return true, nil
	}
	t, err := time.Parse(time.RFC3339Nano, indexedAt)
	if err != nil {
		return true, nil
	}
	return modTime.After(t), nil
}

func isDailyMemoryFileName(name string) bool {
	if !strings.HasSuffix(name, "-memory.md") {
		return false
	}
	datePart := strings.TrimSuffix(name, "-memory.md")
	_, err := time.Parse("2006-01-02", datePart)
	return err == nil
}

func (s *VectorMemoryStore) Search(ctx context.Context, query string, limit int) ([]types.SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 5
	}
	qemb, err := s.embed(ctx, query)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT m.memory_id, m.title, m.filename, m.source_file, m.chunk_index, m.content, e.dim, e.embedding
		FROM memories m
		JOIN memory_embeddings e ON m.memory_id = e.memory_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]types.SearchResult, 0, limit)
	type scored struct {
		types.SearchResult
	}
	all := make([]scored, 0, 64)
	for rows.Next() {
		var id, title, filename, sourceFile, content string
		var chunkIndex int
		var dim int
		var blob []byte
		if err := rows.Scan(&id, &title, &filename, &sourceFile, &chunkIndex, &content, &dim, &blob); err != nil {
			return nil, err
		}
		_ = id
		_ = sourceFile
		_ = chunkIndex
		emb, err := blobToFloat32Slice(blob, dim)
		if err != nil {
			continue
		}
		score := cosineSimilarity(qemb, emb)
		path := strings.TrimSpace(filename)
		if path != "" && !strings.HasPrefix(path, "/") {
			path = "/memory/" + path
		}
		snippet := strings.TrimSpace(content)
		if len(snippet) > 500 {
			snippet = snippet[:500] + "…"
		}
		all = append(all, scored{SearchResult: types.SearchResult{
			Title:   strings.TrimSpace(title),
			Path:    path,
			Snippet: snippet,
			Score:   score,
		}})
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Score > all[j].Score })
	for i := 0; i < len(all) && i < limit; i++ {
		out = append(out, all[i].SearchResult)
	}
	return out, nil
}

func (s *VectorMemoryStore) embed(ctx context.Context, text string) ([]float32, error) {
	if s != nil && s.embedFn != nil {
		return s.embedFn(ctx, text)
	}
	resp, err := s.emb.New(ctx, openai.EmbeddingNewParams{
		Model: openai.EmbeddingModel(s.embedModel),
		Input: openai.EmbeddingNewParamsInputUnion{
			OfString: param.NewOpt(text),
		},
		EncodingFormat: openai.EmbeddingNewParamsEncodingFormatFloat,
	})
	if err != nil {
		return nil, err
	}
	if resp == nil || len(resp.Data) == 0 {
		return nil, fmt.Errorf("embeddings: empty response")
	}
	vec64 := resp.Data[0].Embedding
	out := make([]float32, 0, len(vec64))
	for _, v := range vec64 {
		out = append(out, float32(v))
	}
	return out, nil
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "memory"
	}
	var b strings.Builder
	lastDash := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		ok := (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')
		if ok {
			b.WriteByte(ch)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "memory"
	}
	return out
}

func float32SliceToBlob(in []float32) ([]byte, error) {
	buf := &bytes.Buffer{}
	if err := binary.Write(buf, binary.LittleEndian, in); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func blobToFloat32Slice(blob []byte, dim int) ([]float32, error) {
	if dim <= 0 {
		return nil, fmt.Errorf("invalid dim")
	}
	need := dim * 4
	if len(blob) < need {
		return nil, fmt.Errorf("blob too small")
	}
	out := make([]float32, dim)
	r := bytes.NewReader(blob[:need])
	if err := binary.Read(r, binary.LittleEndian, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var dot, na, nb float64
	for i := 0; i < n; i++ {
		av := float64(a[i])
		bv := float64(b[i])
		dot += av * bv
		na += av * av
		nb += bv * bv
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
