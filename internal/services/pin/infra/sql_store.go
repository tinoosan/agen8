package infra

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	pindomain "github.com/tinoosan/agen8-mcp-server/internal/services/pin/domain"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

// sqlStore is the dialect-neutral pin store. The sqlite and postgres
// repositories embed it; the only dialect-specific concern is placeholder
// rebinding, handled by rebind().
type sqlStore struct {
	db      *sql.DB
	dialect storagedb.Dialect
}

func newSQLStore(handle *storagedb.Handle) *sqlStore {
	return &sqlStore{
		db:      handle.DB(),
		dialect: handle.Dialect(),
	}
}

// ensureSchema creates the pins table if it does not exist. This is the
// "self-migrating vertical" pattern (see the location service): the table is
// provisioned at construction with CREATE TABLE IF NOT EXISTS rather than
// through the version-gated baseline snapshot. That keeps adding pins additive
// and safe - it never forces a currentSchemaVersion bump, so existing
// databases keep starting without a migration step.
//
// Identity is the composite (project_id, node_ref) primary key: per-project,
// shared, one row per pinned node.
func (r *sqlStore) ensureSchema(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS pins (
			project_id TEXT NOT NULL,
			node_ref   TEXT NOT NULL,
			node_type  TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			PRIMARY KEY (project_id, node_ref)
		)
	`); err != nil {
		return fmt.Errorf("ensure pins table: %w", err)
	}
	if _, err := r.db.ExecContext(ctx,
		`CREATE INDEX IF NOT EXISTS idx_pins_project ON pins(project_id)`,
	); err != nil {
		return fmt.Errorf("ensure pins index: %w", err)
	}
	return nil
}

// Save upserts a pin. On conflict we refresh node_type but preserve the
// original created_at, so re-pinning a node keeps its first-pinned time.
func (r *sqlStore) Save(ctx context.Context, pin pindomain.Pin) error {
	if err := pin.Validate(); err != nil {
		return err
	}
	if pin.CreatedAt.IsZero() {
		pin.CreatedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, r.rebind(`
		INSERT INTO pins (project_id, node_ref, node_type, created_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(project_id, node_ref) DO UPDATE SET
			node_type = excluded.node_type
	`),
		strings.TrimSpace(pin.ProjectID),
		strings.TrimSpace(pin.NodeRef),
		strings.TrimSpace(pin.NodeType),
		pin.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("save pin %s/%s: %w", pin.ProjectID, pin.NodeRef, err)
	}
	return nil
}

// Delete removes a pin, returning ErrNotFound when the node was not pinned.
func (r *sqlStore) Delete(ctx context.Context, projectID, nodeRef string) error {
	projectID = strings.TrimSpace(projectID)
	nodeRef = strings.TrimSpace(nodeRef)
	if projectID == "" {
		return fmt.Errorf("pin: projectId is required")
	}
	if nodeRef == "" {
		return fmt.Errorf("pin: nodeRef is required")
	}
	res, err := r.db.ExecContext(ctx,
		r.rebind(`DELETE FROM pins WHERE project_id = ? AND node_ref = ?`),
		projectID, nodeRef,
	)
	if err != nil {
		return fmt.Errorf("delete pin %s/%s: %w", projectID, nodeRef, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete pin %s/%s: %w", projectID, nodeRef, err)
	}
	if affected == 0 {
		return pindomain.ErrNotFound
	}
	return nil
}

// List returns every pin in a project, newest first.
func (r *sqlStore) List(ctx context.Context, projectID string) ([]pindomain.Pin, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, fmt.Errorf("pin: projectId is required")
	}
	rows, err := r.db.QueryContext(ctx, r.rebind(`
		SELECT project_id, node_ref, node_type, created_at
		FROM pins
		WHERE project_id = ?
		ORDER BY created_at DESC, node_ref ASC
	`), projectID)
	if err != nil {
		return nil, fmt.Errorf("list pins for %s: %w", projectID, err)
	}
	defer rows.Close()
	return scanPins(rows)
}

// Exists reports whether a node is pinned in a project.
func (r *sqlStore) Exists(ctx context.Context, projectID, nodeRef string) (bool, error) {
	projectID = strings.TrimSpace(projectID)
	nodeRef = strings.TrimSpace(nodeRef)
	if projectID == "" || nodeRef == "" {
		return false, nil
	}
	var one int
	err := r.db.QueryRowContext(ctx,
		r.rebind(`SELECT 1 FROM pins WHERE project_id = ? AND node_ref = ?`),
		projectID, nodeRef,
	).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("pin exists %s/%s: %w", projectID, nodeRef, err)
	}
	return true, nil
}

func scanPins(rows *sql.Rows) ([]pindomain.Pin, error) {
	pins := []pindomain.Pin{}
	for rows.Next() {
		var (
			pin       pindomain.Pin
			createdAt string
		)
		if err := rows.Scan(&pin.ProjectID, &pin.NodeRef, &pin.NodeType, &createdAt); err != nil {
			return nil, fmt.Errorf("scan pin: %w", err)
		}
		if t, err := time.Parse(time.RFC3339Nano, createdAt); err == nil {
			pin.CreatedAt = t
		}
		pins = append(pins, pin)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pins: %w", err)
	}
	return pins, nil
}

func (r *sqlStore) rebind(query string) string {
	return storagedb.Rebind(query, r.dialect)
}
