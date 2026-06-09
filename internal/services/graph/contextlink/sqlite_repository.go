package contextlink

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	storagedb "github.com/tinoosan/agen8/internal/storage/db"
)

// compile-time check
var _ Repository = (*SQLiteRepository)(nil)

type SQLiteRepository struct {
	db      *sql.DB
	dialect storagedb.Dialect
}

func NewSQLiteRepository(handle *storagedb.Handle) (*SQLiteRepository, error) {
	if handle == nil || handle.DB() == nil || handle.Dialect() == nil {
		return nil, fmt.Errorf("context link db handle is required")
	}
	return &SQLiteRepository{db: handle.DB(), dialect: handle.Dialect()}, nil
}

func (r *SQLiteRepository) rebind(query string) string {
	if r == nil {
		return query
	}
	return storagedb.Rebind(query, r.dialect)
}

func (r *SQLiteRepository) Save(ctx context.Context, link Link) error {
	if err := link.Validate(); err != nil {
		return fmt.Errorf("save context link: %w", err)
	}
	if link.ID == "" {
		link.ID = ID("cl-" + uuid.NewString())
	}
	if link.CreatedAt.IsZero() {
		link.CreatedAt = time.Now().UTC()
	}

	metadataJSON, err := json.Marshal(link.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	_, err = r.db.ExecContext(ctx, r.rebind(`
		INSERT INTO context_links (
			id, source_type, source_id, target_type, target_id,
			edge_type, confidence, metadata_json, created_at, created_by
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		string(link.ID),
		string(link.Source.Type),
		link.Source.ID,
		string(link.Target.Type),
		link.Target.ID,
		string(link.EdgeType),
		link.Confidence,
		string(metadataJSON),
		link.CreatedAt.UTC().Format(time.RFC3339Nano),
		link.CreatedBy,
	)
	if err != nil {
		return fmt.Errorf("save context link: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) Replace(ctx context.Context, link Link) error {
	if err := link.Validate(); err != nil {
		return fmt.Errorf("replace context link: %w", err)
	}
	existing, err := r.FindBetween(ctx, link.Source, link.Target)
	if err != nil {
		return fmt.Errorf("replace context link find existing: %w", err)
	}
	for _, candidate := range existing {
		if candidate.EdgeType != link.EdgeType {
			continue
		}
		link.ID = candidate.ID
		if link.CreatedAt.IsZero() {
			link.CreatedAt = candidate.CreatedAt
		}
		if link.CreatedBy == "" {
			link.CreatedBy = candidate.CreatedBy
		}
		if err := r.Delete(ctx, candidate.ID); err != nil {
			return fmt.Errorf("replace context link delete existing: %w", err)
		}
		if err := r.Save(ctx, link); err != nil {
			return fmt.Errorf("replace context link save: %w", err)
		}
		return nil
	}
	return r.Save(ctx, link)
}

func (r *SQLiteRepository) FindByID(ctx context.Context, id ID) (Link, error) {
	var link Link
	row := r.db.QueryRowContext(ctx, r.rebind(`
		SELECT id, source_type, source_id, target_type, target_id,
		       edge_type, confidence, metadata_json, created_at, created_by
		FROM context_links
		WHERE id = ?`),
		string(id),
	)
	link, err := scanLink(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return Link{}, fmt.Errorf("context link not found: %s", id)
		}
		return Link{}, fmt.Errorf("find by id: %w", err)
	}
	return link, nil
}

func (r *SQLiteRepository) FindBySource(ctx context.Context, source NodeRef) ([]Link, error) {
	rows, err := r.db.QueryContext(ctx, r.rebind(`
		SELECT id, source_type, source_id, target_type, target_id,
		       edge_type, confidence, metadata_json, created_at, created_by
		FROM context_links
		WHERE source_type = ? AND source_id = ?`),
		string(source.Type), source.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("find by source: %w", err)
	}
	defer rows.Close()
	return scanLinks(rows)
}

func (r *SQLiteRepository) FindByTarget(ctx context.Context, target NodeRef) ([]Link, error) {
	rows, err := r.db.QueryContext(ctx, r.rebind(`
		SELECT id, source_type, source_id, target_type, target_id,
		       edge_type, confidence, metadata_json, created_at, created_by
		FROM context_links
		WHERE target_type = ? AND target_id = ?`),
		string(target.Type), target.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("find by target: %w", err)
	}
	defer rows.Close()
	return scanLinks(rows)
}

func (r *SQLiteRepository) FindBetween(ctx context.Context, source NodeRef, target NodeRef) ([]Link, error) {
	rows, err := r.db.QueryContext(ctx, r.rebind(`
		SELECT id, source_type, source_id, target_type, target_id,
		       edge_type, confidence, metadata_json, created_at, created_by
		FROM context_links
		WHERE source_type = ? AND source_id = ? AND target_type = ? AND target_id = ?`),
		string(source.Type), source.ID, string(target.Type), target.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("find between: %w", err)
	}
	defer rows.Close()
	return scanLinks(rows)
}

func (r *SQLiteRepository) FindByEdgeType(ctx context.Context, edgeType EdgeType, limit int) ([]Link, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("find by edge type: limit must be > 0, got %d", limit)
	}
	rows, err := r.db.QueryContext(ctx, r.rebind(`
		SELECT id, source_type, source_id, target_type, target_id,
		       edge_type, confidence, metadata_json, created_at, created_by
		FROM context_links
		WHERE edge_type = ?
		LIMIT ?`),
		string(edgeType), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("find by edge type: %w", err)
	}
	defer rows.Close()
	return scanLinks(rows)
}

func (r *SQLiteRepository) Delete(ctx context.Context, id ID) error {
	res, err := r.db.ExecContext(ctx, r.rebind(`DELETE FROM context_links WHERE id = ?`), string(id))
	if err != nil {
		return fmt.Errorf("delete context link: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("context link not found: %s", id)
	}
	return nil
}

func (r *SQLiteRepository) DeleteLinksForEntity(ctx context.Context, ref NodeRef) error {
	_, err := r.db.ExecContext(ctx, r.rebind(`DELETE FROM context_links WHERE (source_type = ? AND source_id = ?) OR (target_type = ? AND target_id = ?)`),
		string(ref.Type), ref.ID, string(ref.Type), ref.ID,
	)
	if err != nil {
		return fmt.Errorf("delete links for entity: %w", err)
	}
	return nil
}

func scanLinks(rows *sql.Rows) ([]Link, error) {
	var links []Link
	for rows.Next() {
		link, err := scanLink(rows)
		if err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scan rows: %w", err)
	}
	return links, nil
}

func scanLink(s interface{ Scan(dest ...any) error }) (Link, error) {
	var (
		link         Link
		id           string
		sourceType   string
		sourceID     string
		targetType   string
		targetID     string
		edgeType     string
		metadataJSON string
		createdAt    string
	)
	err := s.Scan(
		&id,
		&sourceType,
		&sourceID,
		&targetType,
		&targetID,
		&edgeType,
		&link.Confidence,
		&metadataJSON,
		&createdAt,
		&link.CreatedBy,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return Link{}, sql.ErrNoRows
		}
		return Link{}, fmt.Errorf("scan context link: %w", err)
	}
	link.ID = ID(id)
	link.Source = NodeRef{Type: NodeType(sourceType), ID: sourceID}
	link.Target = NodeRef{Type: NodeType(targetType), ID: targetID}
	link.EdgeType = EdgeType(edgeType)

	if metadataJSON != "" && metadataJSON != "{}" {
		if err := json.Unmarshal([]byte(metadataJSON), &link.Metadata); err != nil {
			return Link{}, fmt.Errorf("unmarshal metadata: %w", err)
		}
	}
	if createdAt != "" {
		t, err := time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return Link{}, fmt.Errorf("parse createdAt: %w", err)
		}
		link.CreatedAt = t
	}
	return link, nil
}
