package infra

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/agen8-mcp-server/internal/services/operator/domain"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

var (
	_ domain.ActionRepository     = (*SQLiteStore)(nil)
	_ domain.EscalationRepository = (*SQLiteStore)(nil)
)

const sqliteTimeFormat = "2006-01-02T15:04:05.000000000Z07:00"

type SQLiteStore struct {
	db      *sql.DB
	dialect storagedb.Dialect
}

func NewSQLiteStore(handle *storagedb.Handle) (*SQLiteStore, error) {
	if handle == nil || handle.DB() == nil || handle.Dialect() == nil {
		return nil, fmt.Errorf("operator db handle is required")
	}
	return &SQLiteStore{db: handle.DB(), dialect: handle.Dialect()}, nil
}

func (s *SQLiteStore) rebind(query string) string {
	if s == nil {
		return query
	}
	return storagedb.Rebind(query, s.dialect)
}

func (s *SQLiteStore) SaveAction(ctx context.Context, action domain.OperatorAction) error {
	outcomePairsJSON, err := marshalJSONNullable(action.OutcomePairs)
	if err != nil {
		return fmt.Errorf("opaction save: marshal outcome_pairs: %w", err)
	}
	attachmentsJSON, err := marshalJSONNullable(action.Attachments)
	if err != nil {
		return fmt.Errorf("opaction save: marshal attachments: %w", err)
	}
	progressNotesJSON, err := marshalJSONNullable(action.ProgressNotes)
	if err != nil {
		return fmt.Errorf("opaction save: marshal progress_notes: %w", err)
	}
	commentsJSON, err := marshalJSONNullable(action.Comments)
	if err != nil {
		return fmt.Errorf("opaction save: marshal comments: %w", err)
	}
	metadataJSON, err := marshalJSONNullable(action.Metadata)
	if err != nil {
		return fmt.Errorf("opaction save: marshal metadata: %w", err)
	}

	_, err = s.db.ExecContext(ctx, s.rebind(`
		INSERT INTO op_actions (
			id, project_id, space_id, task_ref, key_result_ref, mission_ref, run_id,
			blocking, source, member_id, escalation_ref,
			category, urgency, title, description, requires_verification,
			status, outcome_status, outcome_summary, outcome_pairs_json,
			attachments_json, progress_notes_json, comments_json,
			deadline, metadata_json,
			created_at, acknowledged_at, started_at, completed_at, verified_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			space_id = excluded.space_id,
			task_ref = excluded.task_ref,
			key_result_ref = excluded.key_result_ref,
			mission_ref = excluded.mission_ref,
			run_id = excluded.run_id,
			blocking = excluded.blocking,
			source = excluded.source,
			member_id = excluded.member_id,
			escalation_ref = excluded.escalation_ref,
			category = excluded.category,
			urgency = excluded.urgency,
			title = excluded.title,
			description = excluded.description,
			requires_verification = excluded.requires_verification,
			status = excluded.status,
			outcome_status = excluded.outcome_status,
			outcome_summary = excluded.outcome_summary,
			outcome_pairs_json = excluded.outcome_pairs_json,
			attachments_json = excluded.attachments_json,
			progress_notes_json = excluded.progress_notes_json,
			comments_json = excluded.comments_json,
			deadline = excluded.deadline,
			metadata_json = excluded.metadata_json,
			acknowledged_at = excluded.acknowledged_at,
			started_at = excluded.started_at,
			completed_at = excluded.completed_at,
			verified_at = excluded.verified_at
	`),
		string(action.ID),
		action.ProjectID,
		action.SpaceID,
		action.TaskRef,
		action.KeyResultRef,
		action.MissionRef,
		action.RunID,
		action.Blocking,
		string(action.Source),
		action.MemberID,
		action.EscalationRef,
		string(action.Category),
		string(action.Urgency),
		action.Title,
		action.Description,
		action.RequiresVerification,
		string(action.Status),
		nullableString(string(action.OutcomeStatus)),
		nullableString(action.OutcomeSummary),
		outcomePairsJSON,
		attachmentsJSON,
		progressNotesJSON,
		commentsJSON,
		formatTimePtr(action.Deadline),
		metadataJSON,
		formatTime(action.CreatedAt),
		formatTimePtr(action.AcknowledgedAt),
		formatTimePtr(action.StartedAt),
		formatTimePtr(action.CompletedAt),
		formatTimePtr(action.VerifiedAt),
	)
	if err != nil {
		return fmt.Errorf("opaction save: %w", err)
	}
	return nil
}

// Get retrieves a single operator action by ID.
func (s *SQLiteStore) GetAction(ctx context.Context, id domain.OperatorActionID) (domain.OperatorAction, error) {
	row := s.db.QueryRowContext(ctx, s.rebind(`
		SELECT id, project_id, space_id, task_ref, key_result_ref, mission_ref, run_id,
			blocking, source, member_id, escalation_ref,
			category, urgency, title, description, requires_verification,
			status, outcome_status, outcome_summary, outcome_pairs_json,
			attachments_json, progress_notes_json, comments_json,
			deadline, metadata_json,
			created_at, acknowledged_at, started_at, completed_at, verified_at
		FROM op_actions
		WHERE id = ?
	`), string(id))

	oa, err := scanOperatorAction(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.OperatorAction{}, fmt.Errorf("opaction not found: %s", id)
		}
		return domain.OperatorAction{}, fmt.Errorf("opaction get: %w", err)
	}
	return oa, nil
}

// FindByProject returns operator actions for a project, filtered by the given criteria.
func (s *SQLiteStore) FindActionsByProject(ctx context.Context, projectID string, filter domain.ActionFilter) ([]domain.OperatorAction, error) {
	query := `
		SELECT id, project_id, space_id, task_ref, key_result_ref, mission_ref, run_id,
			blocking, source, member_id, escalation_ref,
			category, urgency, title, description, requires_verification,
			status, outcome_status, outcome_summary, outcome_pairs_json,
			attachments_json, progress_notes_json, comments_json,
			deadline, metadata_json,
			created_at, acknowledged_at, started_at, completed_at, verified_at
		FROM op_actions
		WHERE project_id = ?`
	args := []any{projectID}

	if len(filter.Status) > 0 {
		placeholders := make([]string, len(filter.Status))
		for i, s := range filter.Status {
			placeholders[i] = "?"
			args = append(args, string(s))
		}
		query += " AND status IN (" + strings.Join(placeholders, ",") + ")"
	}
	if len(filter.Urgency) > 0 {
		placeholders := make([]string, len(filter.Urgency))
		for i, u := range filter.Urgency {
			placeholders[i] = "?"
			args = append(args, string(u))
		}
		query += " AND urgency IN (" + strings.Join(placeholders, ",") + ")"
	}
	if len(filter.Category) > 0 {
		placeholders := make([]string, len(filter.Category))
		for i, c := range filter.Category {
			placeholders[i] = "?"
			args = append(args, string(c))
		}
		query += " AND category IN (" + strings.Join(placeholders, ",") + ")"
	}
	if filter.SpaceID != "" {
		query += " AND space_id = ?"
		args = append(args, filter.SpaceID)
	}

	query += " ORDER BY created_at DESC"

	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	} else if filter.Offset > 0 {
		// SQLite requires LIMIT before OFFSET; use -1 for unlimited.
		query += " LIMIT -1"
	}
	if filter.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, filter.Offset)
	}

	rows, err := s.db.QueryContext(ctx, s.rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("opaction find by project: %w", err)
	}
	defer rows.Close()

	return scanOperatorActions(rows)
}

// FindByTask returns all operator actions linked to a specific task reference.
func (s *SQLiteStore) FindActionsByTask(ctx context.Context, taskRef string) ([]domain.OperatorAction, error) {
	rows, err := s.db.QueryContext(ctx, s.rebind(`
		SELECT id, project_id, space_id, task_ref, key_result_ref, mission_ref, run_id,
			blocking, source, member_id, escalation_ref,
			category, urgency, title, description, requires_verification,
			status, outcome_status, outcome_summary, outcome_pairs_json,
			attachments_json, progress_notes_json, comments_json,
			deadline, metadata_json,
			created_at, acknowledged_at, started_at, completed_at, verified_at
		FROM op_actions
		WHERE task_ref = ?
		ORDER BY created_at DESC
	`), taskRef)
	if err != nil {
		return nil, fmt.Errorf("opaction find by task: %w", err)
	}
	defer rows.Close()

	return scanOperatorActions(rows)
}

// FindPending returns operator actions that need operator attention for a project.
// This includes all non-terminal statuses: pending, acknowledged, in_progress,
// blocked, and pending_verification.
func (s *SQLiteStore) FindPendingActions(ctx context.Context, projectID string) ([]domain.OperatorAction, error) {
	rows, err := s.db.QueryContext(ctx, s.rebind(`
		SELECT id, project_id, space_id, task_ref, key_result_ref, mission_ref, run_id,
			blocking, source, member_id, escalation_ref,
			category, urgency, title, description, requires_verification,
			status, outcome_status, outcome_summary, outcome_pairs_json,
			attachments_json, progress_notes_json, comments_json,
			deadline, metadata_json,
			created_at, acknowledged_at, started_at, completed_at, verified_at
		FROM op_actions
		WHERE project_id = ? AND status IN ('pending', 'acknowledged', 'in_progress', 'blocked', 'pending_verification')
		ORDER BY
			CASE urgency
				WHEN 'critical' THEN 0
				WHEN 'high' THEN 1
				WHEN 'medium' THEN 2
				WHEN 'low' THEN 3
				ELSE 4
			END,
			created_at ASC
	`), projectID)
	if err != nil {
		return nil, fmt.Errorf("opaction find pending: %w", err)
	}
	defer rows.Close()

	return scanOperatorActions(rows)
}

// CountByStatus returns a count of operator actions grouped by status for a project.
func (s *SQLiteStore) CountActionsByStatus(ctx context.Context, projectID string) (map[domain.OAStatus]int, error) {
	rows, err := s.db.QueryContext(ctx, s.rebind(`
		SELECT status, COUNT(*) FROM op_actions
		WHERE project_id = ?
		GROUP BY status
	`), projectID)
	if err != nil {
		return nil, fmt.Errorf("opaction count by status: %w", err)
	}
	defer rows.Close()

	counts := make(map[domain.OAStatus]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("opaction count by status scan: %w", err)
		}
		counts[domain.OAStatus(status)] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("opaction count by status rows: %w", err)
	}
	return counts, nil
}

// FindByAttachmentID locates the operator action that owns the given attachment.
func (s *SQLiteStore) FindActionByAttachmentID(ctx context.Context, attachmentID string) (domain.OperatorAction, error) {
	attachmentID = strings.TrimSpace(attachmentID)
	if attachmentID == "" {
		return domain.OperatorAction{}, fmt.Errorf("attachment ID is required")
	}

	rows, err := s.db.QueryContext(ctx, s.rebind(`
		SELECT id, project_id, space_id, task_ref, key_result_ref, mission_ref, run_id,
			blocking, source, member_id, escalation_ref,
			category, urgency, title, description, requires_verification,
			status, outcome_status, outcome_summary, outcome_pairs_json,
			attachments_json, progress_notes_json, comments_json,
			deadline, metadata_json,
			created_at, acknowledged_at, started_at, completed_at, verified_at
		FROM op_actions
		WHERE attachments_json IS NOT NULL AND attachments_json != ''
	`))
	if err != nil {
		return domain.OperatorAction{}, fmt.Errorf("opaction find by attachment: %w", err)
	}
	defer rows.Close()

	actions, err := scanOperatorActions(rows)
	if err != nil {
		return domain.OperatorAction{}, fmt.Errorf("opaction find by attachment scan: %w", err)
	}
	for _, oa := range actions {
		for _, attachment := range oa.Attachments {
			if strings.TrimSpace(attachment.ID) == attachmentID {
				return oa, nil
			}
		}
	}
	return domain.OperatorAction{}, fmt.Errorf("attachment not found: %s", attachmentID)
}

// scanOperatorAction scans a single row into an OperatorAction.
func scanOperatorAction(row *sql.Row) (domain.OperatorAction, error) {
	var oa domain.OperatorAction
	var (
		id                   string
		projectID            string
		spaceID              sql.NullString
		taskRef              sql.NullString
		keyResultRef         sql.NullString
		missionRef           sql.NullString
		runID                sql.NullString
		blocking             bool
		source               string
		memberID             sql.NullString
		escalationRef        sql.NullString
		category             string
		urgency              string
		title                string
		description          sql.NullString
		requiresVerification bool
		status               string
		outcomeStatus        sql.NullString
		outcomeSummary       sql.NullString
		outcomePairsJSON     sql.NullString
		attachmentsJSON      sql.NullString
		progressNotesJSON    sql.NullString
		commentsJSON         sql.NullString
		deadline             sql.NullString
		metadataJSON         sql.NullString
		createdAt            string
		acknowledgedAt       sql.NullString
		startedAt            sql.NullString
		completedAt          sql.NullString
		verifiedAt           sql.NullString
	)

	err := row.Scan(
		&id, &projectID, &spaceID, &taskRef, &keyResultRef, &missionRef, &runID,
		&blocking, &source, &memberID, &escalationRef,
		&category, &urgency, &title, &description, &requiresVerification,
		&status, &outcomeStatus, &outcomeSummary, &outcomePairsJSON,
		&attachmentsJSON, &progressNotesJSON, &commentsJSON,
		&deadline, &metadataJSON,
		&createdAt, &acknowledgedAt, &startedAt, &completedAt, &verifiedAt,
	)
	if err != nil {
		return oa, err
	}

	oa.ID = domain.OperatorActionID(id)
	oa.ProjectID = projectID
	oa.SpaceID = spaceID.String
	oa.TaskRef = taskRef.String
	oa.KeyResultRef = keyResultRef.String
	oa.MissionRef = missionRef.String
	oa.RunID = runID.String
	oa.Blocking = blocking
	oa.Source = domain.OASource(source)
	oa.MemberID = memberID.String
	oa.EscalationRef = escalationRef.String
	oa.Category = domain.Category(category)
	oa.Urgency = domain.Urgency(urgency)
	oa.Title = title
	oa.Description = description.String
	oa.RequiresVerification = requiresVerification
	oa.Status = domain.OAStatus(status)
	oa.OutcomeStatus = domain.OutcomeStatus(outcomeStatus.String)
	oa.OutcomeSummary = outcomeSummary.String

	if outcomePairsJSON.Valid && outcomePairsJSON.String != "" {
		if err := json.Unmarshal([]byte(outcomePairsJSON.String), &oa.OutcomePairs); err != nil {
			return oa, fmt.Errorf("opaction: unmarshal outcome_pairs: %w", err)
		}
	}
	if attachmentsJSON.Valid && attachmentsJSON.String != "" {
		if err := json.Unmarshal([]byte(attachmentsJSON.String), &oa.Attachments); err != nil {
			return oa, fmt.Errorf("opaction: unmarshal attachments: %w", err)
		}
	}
	if progressNotesJSON.Valid && progressNotesJSON.String != "" {
		if err := json.Unmarshal([]byte(progressNotesJSON.String), &oa.ProgressNotes); err != nil {
			return oa, fmt.Errorf("opaction: unmarshal progress_notes: %w", err)
		}
	}
	if commentsJSON.Valid && commentsJSON.String != "" {
		if err := json.Unmarshal([]byte(commentsJSON.String), &oa.Comments); err != nil {
			return oa, fmt.Errorf("opaction: unmarshal comments: %w", err)
		}
	}
	if metadataJSON.Valid && metadataJSON.String != "" {
		if err := json.Unmarshal([]byte(metadataJSON.String), &oa.Metadata); err != nil {
			return oa, fmt.Errorf("opaction: unmarshal metadata: %w", err)
		}
	}

	if oa.CreatedAt, err = parseTime(createdAt); err != nil {
		return oa, fmt.Errorf("opaction: %w", err)
	}
	if oa.AcknowledgedAt, err = parseTimePtr(acknowledgedAt); err != nil {
		return oa, fmt.Errorf("opaction: %w", err)
	}
	if oa.StartedAt, err = parseTimePtr(startedAt); err != nil {
		return oa, fmt.Errorf("opaction: %w", err)
	}
	if oa.CompletedAt, err = parseTimePtr(completedAt); err != nil {
		return oa, fmt.Errorf("opaction: %w", err)
	}
	if oa.VerifiedAt, err = parseTimePtr(verifiedAt); err != nil {
		return oa, fmt.Errorf("opaction: %w", err)
	}
	if oa.Deadline, err = parseTimePtr(deadline); err != nil {
		return oa, fmt.Errorf("opaction: %w", err)
	}

	return oa, nil
}

// scanOperatorActions scans multiple rows into a slice of OperatorActions.
func scanOperatorActions(rows *sql.Rows) ([]domain.OperatorAction, error) {
	var result []domain.OperatorAction
	for rows.Next() {
		var oa domain.OperatorAction
		var (
			id                   string
			projectID            string
			spaceID              sql.NullString
			taskRef              sql.NullString
			keyResultRef         sql.NullString
			missionRef           sql.NullString
			runID                sql.NullString
			blocking             bool
			source               string
			memberID             sql.NullString
			escalationRef        sql.NullString
			category             string
			urgency              string
			title                string
			description          sql.NullString
			requiresVerification bool
			status               string
			outcomeStatus        sql.NullString
			outcomeSummary       sql.NullString
			outcomePairsJSON     sql.NullString
			attachmentsJSON      sql.NullString
			progressNotesJSON    sql.NullString
			commentsJSON         sql.NullString
			deadline             sql.NullString
			metadataJSON         sql.NullString
			createdAt            string
			acknowledgedAt       sql.NullString
			startedAt            sql.NullString
			completedAt          sql.NullString
			verifiedAt           sql.NullString
		)

		err := rows.Scan(
			&id, &projectID, &spaceID, &taskRef, &keyResultRef, &missionRef, &runID,
			&blocking, &source, &memberID, &escalationRef,
			&category, &urgency, &title, &description, &requiresVerification,
			&status, &outcomeStatus, &outcomeSummary, &outcomePairsJSON,
			&attachmentsJSON, &progressNotesJSON, &commentsJSON,
			&deadline, &metadataJSON,
			&createdAt, &acknowledgedAt, &startedAt, &completedAt, &verifiedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("opaction scan row: %w", err)
		}

		oa.ID = domain.OperatorActionID(id)
		oa.ProjectID = projectID
		oa.SpaceID = spaceID.String
		oa.TaskRef = taskRef.String
		oa.KeyResultRef = keyResultRef.String
		oa.MissionRef = missionRef.String
		oa.RunID = runID.String
		oa.Blocking = blocking
		oa.Source = domain.OASource(source)
		oa.MemberID = memberID.String
		oa.EscalationRef = escalationRef.String
		oa.Category = domain.Category(category)
		oa.Urgency = domain.Urgency(urgency)
		oa.Title = title
		oa.Description = description.String
		oa.RequiresVerification = requiresVerification
		oa.Status = domain.OAStatus(status)
		oa.OutcomeStatus = domain.OutcomeStatus(outcomeStatus.String)
		oa.OutcomeSummary = outcomeSummary.String

		if outcomePairsJSON.Valid && outcomePairsJSON.String != "" {
			if err := json.Unmarshal([]byte(outcomePairsJSON.String), &oa.OutcomePairs); err != nil {
				return nil, fmt.Errorf("opaction: unmarshal outcome_pairs: %w", err)
			}
		}
		if attachmentsJSON.Valid && attachmentsJSON.String != "" {
			if err := json.Unmarshal([]byte(attachmentsJSON.String), &oa.Attachments); err != nil {
				return nil, fmt.Errorf("opaction: unmarshal attachments: %w", err)
			}
		}
		if progressNotesJSON.Valid && progressNotesJSON.String != "" {
			if err := json.Unmarshal([]byte(progressNotesJSON.String), &oa.ProgressNotes); err != nil {
				return nil, fmt.Errorf("opaction: unmarshal progress_notes: %w", err)
			}
		}
		if commentsJSON.Valid && commentsJSON.String != "" {
			if err := json.Unmarshal([]byte(commentsJSON.String), &oa.Comments); err != nil {
				return nil, fmt.Errorf("opaction: unmarshal comments: %w", err)
			}
		}
		if metadataJSON.Valid && metadataJSON.String != "" {
			if err := json.Unmarshal([]byte(metadataJSON.String), &oa.Metadata); err != nil {
				return nil, fmt.Errorf("opaction: unmarshal metadata: %w", err)
			}
		}

		if oa.CreatedAt, err = parseTime(createdAt); err != nil {
			return nil, fmt.Errorf("opaction: %w", err)
		}
		if oa.AcknowledgedAt, err = parseTimePtr(acknowledgedAt); err != nil {
			return nil, fmt.Errorf("opaction: %w", err)
		}
		if oa.StartedAt, err = parseTimePtr(startedAt); err != nil {
			return nil, fmt.Errorf("opaction: %w", err)
		}
		if oa.CompletedAt, err = parseTimePtr(completedAt); err != nil {
			return nil, fmt.Errorf("opaction: %w", err)
		}
		if oa.VerifiedAt, err = parseTimePtr(verifiedAt); err != nil {
			return nil, fmt.Errorf("opaction: %w", err)
		}
		if oa.Deadline, err = parseTimePtr(deadline); err != nil {
			return nil, fmt.Errorf("opaction: %w", err)
		}

		result = append(result, oa)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("opaction scan rows: %w", err)
	}
	return result, nil
}

// --- Helpers ---

func marshalJSONNullable(v any) (sql.NullString, error) {
	if v == nil {
		return sql.NullString{}, nil
	}
	// Check for nil-typed interfaces and empty collections.
	switch val := v.(type) {
	case map[string]string:
		if len(val) == 0 {
			return sql.NullString{}, nil
		}
	case []domain.Attachment:
		if len(val) == 0 {
			return sql.NullString{}, nil
		}
	case []domain.ProgressNote:
		if len(val) == 0 {
			return sql.NullString{}, nil
		}
	case []domain.Comment:
		if len(val) == 0 {
			return sql.NullString{}, nil
		}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return sql.NullString{}, err
	}
	return sql.NullString{Valid: true, String: string(b)}, nil
}

func nullableString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{Valid: true, String: s}
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func formatTimePtr(t *time.Time) sql.NullString {
	if t == nil {
		return sql.NullString{}
	}
	return sql.NullString{Valid: true, String: t.UTC().Format(time.RFC3339Nano)}
}

func parseTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse time %q: %w", s, err)
	}
	return t, nil
}

func parseTimePtr(ns sql.NullString) (*time.Time, error) {
	if !ns.Valid || ns.String == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339Nano, ns.String)
	if err != nil {
		return nil, fmt.Errorf("parse time %q: %w", ns.String, err)
	}
	return &t, nil
}

func (s *SQLiteStore) SaveEscalation(ctx context.Context, esc domain.Escalation) error {
	if strings.TrimSpace(string(esc.ID)) == "" {
		esc.ID = domain.EscalationID("esc-" + uuid.NewString())
	}

	metadataJSON, err := json.Marshal(esc.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	if esc.Metadata == nil {
		metadataJSON = []byte("{}")
	}

	createdAt := esc.CreatedAt.UTC().Format(sqliteTimeFormat)

	var deadline *string
	if esc.Deadline != nil {
		v := esc.Deadline.UTC().Format(sqliteTimeFormat)
		deadline = &v
	}

	var escalatedAt *string
	if esc.EscalatedAt != nil {
		v := esc.EscalatedAt.UTC().Format(sqliteTimeFormat)
		escalatedAt = &v
	}

	var resolvedAt *string
	if esc.ResolvedAt != nil {
		v := esc.ResolvedAt.UTC().Format(sqliteTimeFormat)
		resolvedAt = &v
	}

	_, err = s.db.ExecContext(ctx, s.rebind(`
		INSERT OR REPLACE INTO escalations (
			id, project_id, space_id, task_ref, key_result_ref, mission_ref,
			source, member_id, category, urgency,
			title, description, recommendation, confidence,
			status, resolution, resolution_note,
			deadline, escalated_at, original_urgency, metadata_json,
			created_at, resolved_at, resolved_by
		) VALUES (
			?, ?, ?, ?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?
		)`),
		string(esc.ID),
		strings.TrimSpace(esc.ProjectID),
		strings.TrimSpace(esc.SpaceID),
		strings.TrimSpace(esc.TaskRef),
		strings.TrimSpace(esc.KeyResultRef),
		strings.TrimSpace(esc.MissionRef),
		strings.TrimSpace(string(esc.Source)),
		strings.TrimSpace(esc.MemberID),
		strings.TrimSpace(string(esc.Category)),
		strings.TrimSpace(string(esc.Urgency)),
		strings.TrimSpace(esc.Title),
		strings.TrimSpace(esc.Description),
		strings.TrimSpace(esc.Recommendation),
		esc.Confidence,
		strings.TrimSpace(string(esc.Status)),
		strings.TrimSpace(string(esc.Resolution)),
		strings.TrimSpace(esc.ResolutionNote),
		deadline,
		escalatedAt,
		strings.TrimSpace(string(esc.OriginalUrgency)),
		string(metadataJSON),
		createdAt,
		resolvedAt,
		strings.TrimSpace(esc.ResolvedBy),
	)
	if err != nil {
		return fmt.Errorf("save escalation: %w", err)
	}
	return nil
}

// Get retrieves an escalation by ID.
func (s *SQLiteStore) GetEscalation(ctx context.Context, id domain.EscalationID) (domain.Escalation, error) {
	row := s.db.QueryRowContext(ctx, s.rebind(`
		SELECT
			id, project_id, space_id, task_ref, key_result_ref, mission_ref,
			source, member_id, category, urgency,
			title, description, recommendation, confidence,
			status, resolution, resolution_note,
			deadline, escalated_at, original_urgency, metadata_json,
			created_at, resolved_at, resolved_by
		FROM escalations
		WHERE id = ?`), strings.TrimSpace(string(id)))

	esc, err := scanFromRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Escalation{}, fmt.Errorf("escalation not found: %s", id)
		}
		return domain.Escalation{}, err
	}
	return esc, nil
}

// FindByProject returns escalations for a project matching the filter.
// Results are ordered by urgency rank DESC, created_at DESC (most urgent first).
func (s *SQLiteStore) FindEscalationsByProject(ctx context.Context, projectID string, filter domain.EscalationFilter) ([]domain.Escalation, error) {
	where := []string{"project_id = ?"}
	args := []any{strings.TrimSpace(projectID)}

	if len(filter.Status) > 0 {
		placeholders := make([]string, len(filter.Status))
		for i, st := range filter.Status {
			placeholders[i] = "?"
			args = append(args, strings.TrimSpace(string(st)))
		}
		where = append(where, "status IN ("+strings.Join(placeholders, ",")+")")
	}
	if len(filter.Urgency) > 0 {
		placeholders := make([]string, len(filter.Urgency))
		for i, u := range filter.Urgency {
			placeholders[i] = "?"
			args = append(args, strings.TrimSpace(string(u)))
		}
		where = append(where, "urgency IN ("+strings.Join(placeholders, ",")+")")
	}
	if len(filter.Category) > 0 {
		placeholders := make([]string, len(filter.Category))
		for i, c := range filter.Category {
			placeholders[i] = "?"
			args = append(args, strings.TrimSpace(string(c)))
		}
		where = append(where, "category IN ("+strings.Join(placeholders, ",")+")")
	}
	if strings.TrimSpace(filter.SpaceID) != "" {
		where = append(where, "space_id = ?")
		args = append(args, strings.TrimSpace(filter.SpaceID))
	}

	query := selectColumns() +
		` FROM escalations WHERE ` + strings.Join(where, " AND ") +
		` ORDER BY ` + urgencyOrderExpr() + ` DESC, created_at DESC`

	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	} else if filter.Offset > 0 {
		query += " LIMIT -1"
	}
	if filter.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, filter.Offset)
	}

	return s.queryEscalations(ctx, query, args...)
}

// FindPendingByTask returns all pending escalations linked to the given task reference.
func (s *SQLiteStore) FindPendingEscalationsByTask(ctx context.Context, taskRef string) ([]domain.Escalation, error) {
	query := selectColumns() +
		` FROM escalations
		WHERE task_ref = ? AND status = 'pending'
		ORDER BY created_at DESC`
	return s.queryEscalations(ctx, query, strings.TrimSpace(taskRef))
}

// CountPending returns the number of pending escalations for a project.
func (s *SQLiteStore) CountPendingEscalations(ctx context.Context, projectID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, s.rebind(
		`SELECT COUNT(*) FROM escalations WHERE project_id = ? AND status = 'pending'`,
	),
		strings.TrimSpace(projectID),
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count pending escalations: %w", err)
	}
	return count, nil
}

// UpdateStatus transitions the escalation to a new status with resolution details.
// Returns error if the escalation is not in a valid state for the transition.
//
// The UPDATE includes the prior status in the WHERE clause so that concurrent
// callers cannot both succeed — only the first writer wins. If RowsAffected
// is 0, the escalation was either not found or concurrently modified.
func (s *SQLiteStore) UpdateEscalationStatus(ctx context.Context, id domain.EscalationID, status domain.Status, resolution domain.Resolution, note string, resolvedBy string) error {
	// Load the escalation to validate the transition.
	esc, err := s.GetEscalation(ctx, id)
	if err != nil {
		return err
	}

	if err := esc.CanTransitionTo(status); err != nil {
		return err
	}

	// Bug 3: validate resolution when resolving.
	if status == domain.StatusResolved {
		if err := domain.ValidateResolution(resolution); err != nil {
			return err
		}
	}

	now := time.Now().UTC()
	var resolvedAt *string
	if status == domain.StatusResolved {
		v := now.Format(sqliteTimeFormat)
		resolvedAt = &v
	}

	// Bug 2: include prior status in WHERE for atomic transition.
	result, err := s.db.ExecContext(ctx, s.rebind(`
		UPDATE escalations
		SET status = ?, resolution = ?, resolution_note = ?, resolved_by = ?, resolved_at = ?
		WHERE id = ? AND status = ?`),
		strings.TrimSpace(string(status)),
		strings.TrimSpace(string(resolution)),
		strings.TrimSpace(note),
		strings.TrimSpace(resolvedBy),
		resolvedAt,
		strings.TrimSpace(string(id)),
		string(esc.Status),
	)
	if err != nil {
		return fmt.Errorf("update escalation status: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("escalation %s: status transition failed (concurrent modification or not found)", id)
	}
	return nil
}

// FindPendingEscalationDuplicate returns the earliest pending escalation
// matching (spaceID, taskRef, category, urgency) created at or after since.
// Urgency is part of the dedup tuple so a critical re-escalation no longer
// silently merges with a still-pending lower-urgency duplicate.
// Returns (zero, false, nil) when no match is found.
func (s *SQLiteStore) FindPendingEscalationDuplicate(ctx context.Context, spaceID, taskRef string, category domain.Category, urgency domain.Urgency, since time.Time) (domain.Escalation, bool, error) {
	query := selectColumns() + `
		FROM escalations
		WHERE space_id = ?
		  AND COALESCE(TRIM(task_ref), '') = ?
		  AND category = ?
		  AND urgency = ?
		  AND status = 'pending'
		  AND created_at >= ?
		ORDER BY created_at ASC
		LIMIT 1`
	row := s.db.QueryRowContext(ctx, s.rebind(query),
		strings.TrimSpace(spaceID),
		strings.TrimSpace(taskRef),
		strings.TrimSpace(string(category)),
		strings.TrimSpace(string(urgency)),
		since.UTC().Format(sqliteTimeFormat),
	)
	esc, err := scanFromRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Escalation{}, false, nil
		}
		return domain.Escalation{}, false, fmt.Errorf("find pending duplicate: %w", err)
	}
	return esc, true, nil
}

// FindExpiredPending returns all pending escalations where deadline < now.
func (s *SQLiteStore) FindExpiredPendingEscalations(ctx context.Context, now time.Time) ([]domain.Escalation, error) {
	query := selectColumns() +
		` FROM escalations
		WHERE deadline IS NOT NULL AND deadline < ? AND status = 'pending'
		ORDER BY deadline ASC`
	return s.queryEscalations(ctx, query, now.UTC().Format(sqliteTimeFormat))
}

// EscalateUrgency bumps the urgency tier and records the escalation timestamp.
// Preserves the original urgency in OriginalUrgency if not already set.
// Returns an error if the escalation ID does not exist.
func (s *SQLiteStore) EscalateEscalationUrgency(ctx context.Context, id domain.EscalationID, newUrgency domain.Urgency, originalUrgency domain.Urgency, escalatedAt time.Time) error {
	result, err := s.db.ExecContext(ctx, s.rebind(`
		UPDATE escalations
		SET urgency = ?, original_urgency = ?, escalated_at = ?
		WHERE id = ?`),
		strings.TrimSpace(string(newUrgency)),
		strings.TrimSpace(string(originalUrgency)),
		escalatedAt.UTC().Format(sqliteTimeFormat),
		strings.TrimSpace(string(id)),
	)
	if err != nil {
		return fmt.Errorf("escalate urgency: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("escalation %s not found", id)
	}
	return nil
}

// ── helpers ─────────────────────────────────────────────────────────────

func selectColumns() string {
	return `SELECT
		id, project_id, space_id, task_ref, key_result_ref, mission_ref,
		source, member_id, category, urgency,
		title, description, recommendation, confidence,
		status, resolution, resolution_note,
		deadline, escalated_at, original_urgency, metadata_json,
		created_at, resolved_at, resolved_by`
}

// urgencyOrderExpr returns a SQL CASE expression that maps urgency strings to
// numeric ranks for sorting: critical=3, high=2, medium=1, low=0.
func urgencyOrderExpr() string {
	return `CASE urgency
		WHEN 'critical' THEN 3
		WHEN 'high' THEN 2
		WHEN 'medium' THEN 1
		WHEN 'low' THEN 0
		ELSE 0
	END`
}

func (s *SQLiteStore) queryEscalations(ctx context.Context, query string, args ...any) ([]domain.Escalation, error) {
	rows, err := s.db.QueryContext(ctx, s.rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("query escalations: %w", err)
	}
	defer rows.Close()

	var results []domain.Escalation
	for rows.Next() {
		esc, err := scanFromRows(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, esc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate escalations: %w", err)
	}
	return results, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanFromRow(row *sql.Row) (domain.Escalation, error) {
	return scanFromScanner(row)
}

func scanFromRows(rows *sql.Rows) (domain.Escalation, error) {
	return scanFromScanner(rows)
}

func scanFromScanner(scanner rowScanner) (domain.Escalation, error) {
	var (
		esc             domain.Escalation
		id              string
		projectID       string
		spaceID         string
		taskRef         string
		keyResultRef    string
		missionRef      string
		source          string
		memberID        string
		category        string
		urgency         string
		title           string
		description     string
		recommendation  string
		confidence      float64
		status          string
		resolution      string
		resolutionNote  string
		deadline        *string
		escalatedAt     *string
		originalUrgency string
		metadataJSON    string
		createdAt       string
		resolvedAt      *string
		resolvedBy      string
	)

	err := scanner.Scan(
		&id, &projectID, &spaceID, &taskRef, &keyResultRef, &missionRef,
		&source, &memberID, &category, &urgency,
		&title, &description, &recommendation, &confidence,
		&status, &resolution, &resolutionNote,
		&deadline, &escalatedAt, &originalUrgency, &metadataJSON,
		&createdAt, &resolvedAt, &resolvedBy,
	)
	if err != nil {
		return domain.Escalation{}, err
	}

	esc.ID = domain.EscalationID(strings.TrimSpace(id))
	esc.ProjectID = strings.TrimSpace(projectID)
	esc.SpaceID = strings.TrimSpace(spaceID)
	esc.TaskRef = strings.TrimSpace(taskRef)
	esc.KeyResultRef = strings.TrimSpace(keyResultRef)
	esc.MissionRef = strings.TrimSpace(missionRef)
	esc.Source = domain.Source(strings.TrimSpace(source))
	esc.MemberID = strings.TrimSpace(memberID)
	esc.Category = domain.Category(strings.TrimSpace(category))
	esc.Urgency = domain.Urgency(strings.TrimSpace(urgency))
	esc.Title = strings.TrimSpace(title)
	esc.Description = strings.TrimSpace(description)
	esc.Recommendation = strings.TrimSpace(recommendation)
	esc.Confidence = confidence
	esc.Status = domain.Status(strings.TrimSpace(status))
	esc.Resolution = domain.Resolution(strings.TrimSpace(resolution))
	esc.ResolutionNote = strings.TrimSpace(resolutionNote)
	esc.OriginalUrgency = domain.Urgency(strings.TrimSpace(originalUrgency))
	esc.ResolvedBy = strings.TrimSpace(resolvedBy)

	if deadline != nil {
		t, parseErr := time.Parse(sqliteTimeFormat, *deadline)
		if parseErr != nil {
			return domain.Escalation{}, fmt.Errorf("parse deadline: %w", parseErr)
		}
		esc.Deadline = &t
	}
	if escalatedAt != nil {
		t, parseErr := time.Parse(sqliteTimeFormat, *escalatedAt)
		if parseErr != nil {
			return domain.Escalation{}, fmt.Errorf("parse escalatedAt: %w", parseErr)
		}
		esc.EscalatedAt = &t
	}
	if resolvedAt != nil {
		t, parseErr := time.Parse(sqliteTimeFormat, *resolvedAt)
		if parseErr != nil {
			return domain.Escalation{}, fmt.Errorf("parse resolvedAt: %w", parseErr)
		}
		esc.ResolvedAt = &t
	}

	if createdAt != "" {
		t, parseErr := time.Parse(sqliteTimeFormat, createdAt)
		if parseErr != nil {
			return domain.Escalation{}, fmt.Errorf("parse createdAt: %w", parseErr)
		}
		esc.CreatedAt = t
	}

	if strings.TrimSpace(metadataJSON) != "" && strings.TrimSpace(metadataJSON) != "{}" {
		m := make(map[string]string)
		if jsonErr := json.Unmarshal([]byte(metadataJSON), &m); jsonErr != nil {
			return domain.Escalation{}, fmt.Errorf("unmarshal metadata: %w", jsonErr)
		}
		esc.Metadata = m
	}

	return esc, nil
}
