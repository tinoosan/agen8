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
	"github.com/tinoosan/agen8-mcp-server/internal/services/decision/domain"
	humaninput "github.com/tinoosan/agen8-mcp-server/internal/services/humaninput/domain"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

var _ domain.Repository = (*SQLiteRepository)(nil)

// selectColumns is the canonical column list for all SELECT queries.
// Must match the scan order in scanFromScanner.
const selectColumns = `
	id, project_id, space_id, source, kind, source_identity,
	run_id, title, rationale, context, questions_json, answers_json, cancelled, alternatives_rejected, invalidation_conditions_json, confidence,
	outcome, task_ref, key_result_ref, mission_ref,
	plan_ref,
	operator_action_ref, escalation_ref, correlation_ref,
	informed_by_ref, tags_json, metadata_json, created_at`

// SQLiteRepository implements domain.Repository backed by SQLite.
type SQLiteRepository struct {
	db      *sql.DB
	dialect storagedb.Dialect
}

// NewSQLiteRepository creates a new SQLiteRepository.
func NewSQLiteRepository(handle *storagedb.Handle) *SQLiteRepository {
	return &SQLiteRepository{db: handle.DB(), dialect: handle.Dialect()}
}

func (r *SQLiteRepository) rebind(query string) string {
	return storagedb.Rebind(query, r.dialect)
}

// CreateDecision persists a decision. If the ID is empty, a new one is generated.
// Uses INSERT OR REPLACE for upsert semantics.
//
// Kind-specific columns are populated from whichever payload pointer is
// set; columns belonging to the other kind are zero-valued. Validation
// (exactly one payload set) is the caller's responsibility; CreateDecision
// returns an error if both or neither are set.
func (r *SQLiteRepository) CreateDecision(ctx context.Context, decision domain.Decision) error {
	if strings.TrimSpace(string(decision.ID)) == "" {
		decision.ID = domain.DecisionID("dec-" + uuid.NewString())
	}

	metadataJSON, err := json.Marshal(decision.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	if decision.Metadata == nil {
		metadataJSON = []byte("{}")
	}

	tagsJSON, err := json.Marshal(decision.Tags)
	if err != nil {
		return fmt.Errorf("marshal tags: %w", err)
	}
	if decision.Tags == nil {
		tagsJSON = []byte("[]")
	}

	// Pull payload fields out into per-column variables. Exactly one of
	// Log or AskUser must be set; the other path's columns stay zero.
	var (
		kind                       domain.DecisionKind
		rationale                  string
		contextValue               string
		questionsJSON              = []byte("[]")
		answersJSON                = []byte("[]")
		cancelled                  bool
		alternativesRejected       string
		invalidationConditionsJSON = []byte("[]")
		outcome                    string
	)
	switch {
	case decision.Log != nil && decision.AskUser != nil:
		return errors.New("save decision: exactly one payload may be set, got both Log and AskUser")
	case decision.Log != nil:
		kind = domain.DecisionKindLog
		p := decision.Log
		rationale = strings.TrimSpace(p.Rationale)
		contextValue = strings.TrimSpace(p.Context)
		alternativesRejected = strings.TrimSpace(p.AlternativesRejected)
		outcome = strings.TrimSpace(p.Outcome)
		if p.InvalidationConditions != nil {
			b, err := json.Marshal(p.InvalidationConditions)
			if err != nil {
				return fmt.Errorf("marshal invalidation conditions: %w", err)
			}
			invalidationConditionsJSON = b
		}
	case decision.AskUser != nil:
		kind = domain.DecisionKindAskUser
		p := decision.AskUser
		contextValue = strings.TrimSpace(p.Context)
		cancelled = p.Cancelled
		if p.Questions != nil {
			b, err := json.Marshal(p.Questions)
			if err != nil {
				return fmt.Errorf("marshal questions: %w", err)
			}
			questionsJSON = b
		}
		if p.Answers != nil {
			b, err := json.Marshal(p.Answers)
			if err != nil {
				return fmt.Errorf("marshal answers: %w", err)
			}
			answersJSON = b
		}
	default:
		return errors.New("save decision: a payload (Log or AskUser) is required")
	}

	createdAt := decision.CreatedAt.UTC().Format(time.RFC3339Nano)

	_, err = r.db.ExecContext(ctx, r.rebind(`
		INSERT OR REPLACE INTO decisions (
			id, project_id, space_id, source, kind, source_identity,
			run_id, title, rationale, context, questions_json, answers_json, cancelled, alternatives_rejected, invalidation_conditions_json, confidence,
			outcome, task_ref, key_result_ref, mission_ref,
			plan_ref,
			operator_action_ref, escalation_ref, correlation_ref,
			informed_by_ref, tags_json, metadata_json, created_at
		) VALUES (
			?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?,
			?, ?, ?,
			?, ?, ?, ?
		)`),
		string(decision.ID),
		strings.TrimSpace(decision.ProjectID),
		strings.TrimSpace(decision.SpaceID),
		string(decision.Source),
		string(kind),
		strings.TrimSpace(decision.SourceIdentity),
		strings.TrimSpace(decision.RunID),
		strings.TrimSpace(decision.Title),
		rationale,
		contextValue,
		string(questionsJSON),
		string(answersJSON),
		cancelled,
		alternativesRejected,
		string(invalidationConditionsJSON),
		decision.Confidence,
		outcome,
		strings.TrimSpace(decision.TaskRef),
		strings.TrimSpace(decision.KeyResultRef),
		strings.TrimSpace(decision.MissionRef),
		strings.TrimSpace(decision.PlanRef),
		strings.TrimSpace(decision.OperatorActionRef),
		strings.TrimSpace(decision.EscalationRef),
		strings.TrimSpace(decision.CorrelationRef),
		strings.TrimSpace(decision.InformedByRef),
		string(tagsJSON),
		string(metadataJSON),
		createdAt,
	)
	if err != nil {
		return fmt.Errorf("save decision: %w", err)
	}
	return nil
}

// Get retrieves a decision by ID.
func (r *SQLiteRepository) GetDecision(ctx context.Context, id domain.DecisionID) (domain.Decision, error) {
	row := r.db.QueryRowContext(ctx,
		r.rebind(`SELECT`+selectColumns+` FROM decisions WHERE id = ?`),
		strings.TrimSpace(string(id)),
	)
	return scanDecision(row)
}

// Delete removes a decision by ID.
func (r *SQLiteRepository) DeleteDecision(ctx context.Context, id domain.DecisionID) error {
	trimmed := strings.TrimSpace(string(id))
	if trimmed == "" {
		return fmt.Errorf("delete decision: id is required")
	}
	res, err := r.db.ExecContext(ctx, r.rebind(`DELETE FROM decisions WHERE id = ?`), trimmed)
	if err != nil {
		return fmt.Errorf("delete decision: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete decision rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("decision not found")
	}
	return nil
}

// ListDecisions returns decisions matching the supplied filter.
// Results are ordered by created_at DESC (most recent first).
// Filters use AND semantics.
func (r *SQLiteRepository) ListDecisions(ctx context.Context, filter domain.DecisionFilter) ([]domain.Decision, error) {
	where, args := buildFilterClauses(filter)
	order := "DESC"
	if !filter.SortDesc {
		order = "ASC"
	}
	query := `SELECT` + selectColumns + ` FROM decisions WHERE ` +
		strings.Join(where, " AND ") + ` ORDER BY created_at ` + order
	query, args = applyPagination(query, args, filter.Limit, filter.Offset)
	return r.queryDecisions(ctx, query, args...)
}

// ListDecisionsByKeyResult returns all decisions linked to the given key result reference.
func (r *SQLiteRepository) ListDecisionsByKeyResult(ctx context.Context, keyResultRef string) ([]domain.Decision, error) {
	query := `SELECT` + selectColumns + ` FROM decisions WHERE key_result_ref = ? ORDER BY created_at DESC`
	return r.queryDecisions(ctx, query, strings.TrimSpace(keyResultRef))
}

// CountDecisions returns the number of decisions matching the given filter.
func (r *SQLiteRepository) CountDecisions(ctx context.Context, filter domain.DecisionFilter) (int, error) {
	where, args := buildFilterClauses(filter)
	query := `SELECT COUNT(*) FROM decisions WHERE ` + strings.Join(where, " AND ")

	var count int
	if err := r.db.QueryRowContext(ctx, r.rebind(query), args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count decisions: %w", err)
	}
	return count, nil
}

// ExportDecisions returns all decisions matching the filter with no pagination limit.
// Intended for CSV/JSON export use cases.
func (r *SQLiteRepository) ExportDecisions(ctx context.Context, filter domain.DecisionFilter) ([]domain.Decision, error) {
	where, args := buildFilterClauses(filter)
	order := "DESC"
	if !filter.SortDesc {
		order = "ASC"
	}
	query := `SELECT` + selectColumns + ` FROM decisions WHERE ` +
		strings.Join(where, " AND ") + ` ORDER BY created_at ` + order
	// Export ignores pagination — returns all matching rows.
	return r.queryDecisions(ctx, query, args...)
}

// DecisionExistsByFingerprint checks whether a decision with the same fingerprint
// (project, source identity, title, task ref) was created after the given time.
func (r *SQLiteRepository) DecisionExistsByFingerprint(ctx context.Context, projectID, sourceIdentity, title, taskRef string, since time.Time) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		r.rebind(`SELECT COUNT(*) FROM decisions
		 WHERE project_id = ? AND source_identity = ? AND title = ? AND task_ref = ? AND created_at >= ?`),
		strings.TrimSpace(projectID),
		strings.TrimSpace(sourceIdentity),
		strings.TrimSpace(title),
		strings.TrimSpace(taskRef),
		since.UTC().Format(time.RFC3339Nano),
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("exists by fingerprint: %w", err)
	}
	return count > 0, nil
}

// -- filter/pagination helpers -----------------------------------------------

// buildFilterClauses constructs WHERE clause fragments and parameter args
// from a project ID and filter. All filters use AND semantics.
func buildFilterClauses(filter domain.DecisionFilter) ([]string, []any) {
	where := []string{"project_id = ?"}
	args := []any{strings.TrimSpace(filter.ProjectID)}

	if len(filter.Sources) > 0 {
		placeholders := make([]string, len(filter.Sources))
		for i, s := range filter.Sources {
			placeholders[i] = "?"
			args = append(args, string(s))
		}
		where = append(where, "source IN ("+strings.Join(placeholders, ", ")+")")
	}

	if strings.TrimSpace(filter.SpaceID) != "" {
		where = append(where, "space_id = ?")
		args = append(args, strings.TrimSpace(filter.SpaceID))
	}

	if q := strings.TrimSpace(filter.Query); q != "" {
		like := "%" + strings.ToLower(q) + "%"
		where = append(where, `(LOWER(title) LIKE ? OR LOWER(rationale) LIKE ? OR LOWER(alternatives_rejected) LIKE ? OR LOWER(invalidation_conditions_json) LIKE ? OR LOWER(outcome) LIKE ?)`)
		args = append(args, like, like, like, like, like)
	}

	if filter.Since != nil {
		where = append(where, "created_at >= ?")
		args = append(args, filter.Since.UTC().Format(time.RFC3339Nano))
	}

	if filter.Until != nil {
		where = append(where, "created_at <= ?")
		args = append(args, filter.Until.UTC().Format(time.RFC3339Nano))
	}

	if len(filter.Tags) > 0 {
		// AND semantics: every tag must be present in the tags_json array.
		// We use json_each to check containment for each required tag.
		for _, tag := range filter.Tags {
			where = append(where, `EXISTS (SELECT 1 FROM json_each(tags_json) WHERE json_each.value = ?)`)
			args = append(args, strings.TrimSpace(tag))
		}
	}

	return where, args
}

// applyPagination adds LIMIT/OFFSET to a query.
func applyPagination(query string, args []any, limit, offset int) (string, []any) {
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	} else if offset > 0 {
		// SQLite requires LIMIT before OFFSET. Use -1 for "no limit".
		query += " LIMIT -1"
	}
	if offset > 0 {
		query += " OFFSET ?"
		args = append(args, offset)
	}
	return query, args
}

// -- query/scan helpers ------------------------------------------------------

// queryDecisions executes a query and scans all resulting rows into Decision slices.
func (r *SQLiteRepository) queryDecisions(ctx context.Context, query string, args ...any) ([]domain.Decision, error) {
	rows, err := r.db.QueryContext(ctx, r.rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("query decisions: %w", err)
	}
	defer rows.Close()

	var decisions []domain.Decision
	for rows.Next() {
		decision, err := scanDecisionRow(rows)
		if err != nil {
			return nil, err
		}
		decisions = append(decisions, decision)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate decisions: %w", err)
	}
	return decisions, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanDecision(row *sql.Row) (domain.Decision, error) {
	d, err := scanFromScanner(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Decision{}, fmt.Errorf("decision not found")
		}
		return domain.Decision{}, err
	}
	return d, nil
}

func scanDecisionRow(rows *sql.Rows) (domain.Decision, error) {
	return scanFromScanner(rows)
}

func scanFromScanner(s rowScanner) (domain.Decision, error) {
	var (
		d                          domain.Decision
		id                         string
		projectID                  string
		spaceID                    string
		source                     string
		kindStr                    string
		sourceIdentity             string
		runID                      string
		title                      string
		rationale                  string
		contextValue               string
		questionsJSON              string
		answersJSON                string
		cancelled                  bool
		alternativesRejected       string
		invalidationConditionsJSON string
		confidence                 float64
		outcome                    string
		taskRef                    string
		keyResultRef               string
		missionRef                 string
		planRef                    string
		operatorActionRef          string
		escalationRef              string
		correlationRef             string
		informedByRef              string
		tagsJSON                   string
		metadataJSON               string
		createdAt                  string
	)

	err := s.Scan(
		&id, &projectID, &spaceID, &source, &kindStr, &sourceIdentity,
		&runID, &title, &rationale, &contextValue, &questionsJSON, &answersJSON, &cancelled, &alternativesRejected, &invalidationConditionsJSON, &confidence,
		&outcome, &taskRef, &keyResultRef, &missionRef, &planRef,
		&operatorActionRef, &escalationRef, &correlationRef,
		&informedByRef, &tagsJSON, &metadataJSON, &createdAt,
	)
	if err != nil {
		return domain.Decision{}, err
	}

	d.ID = domain.DecisionID(strings.TrimSpace(id))
	d.ProjectID = strings.TrimSpace(projectID)
	d.SpaceID = strings.TrimSpace(spaceID)
	d.Source = domain.DecisionSource(strings.TrimSpace(source))
	d.SourceIdentity = strings.TrimSpace(sourceIdentity)
	d.RunID = strings.TrimSpace(runID)
	d.Title = strings.TrimSpace(title)
	d.Confidence = confidence
	d.TaskRef = strings.TrimSpace(taskRef)
	d.KeyResultRef = strings.TrimSpace(keyResultRef)
	d.MissionRef = strings.TrimSpace(missionRef)
	d.PlanRef = strings.TrimSpace(planRef)
	d.OperatorActionRef = strings.TrimSpace(operatorActionRef)
	d.EscalationRef = strings.TrimSpace(escalationRef)
	d.CorrelationRef = strings.TrimSpace(correlationRef)
	d.InformedByRef = strings.TrimSpace(informedByRef)

	parsedTime, parseErr := time.Parse(time.RFC3339Nano, createdAt)
	if parseErr != nil {
		return domain.Decision{}, fmt.Errorf("parse created_at %q: %w", createdAt, parseErr)
	}
	d.CreatedAt = parsedTime

	// Parse tags JSON.
	if strings.TrimSpace(tagsJSON) != "" && strings.TrimSpace(tagsJSON) != "[]" {
		var tags []string
		if jsonErr := json.Unmarshal([]byte(tagsJSON), &tags); jsonErr != nil {
			return domain.Decision{}, fmt.Errorf("parse tags_json: %w", jsonErr)
		}
		d.Tags = tags
	}

	// Parse metadata JSON.
	if strings.TrimSpace(metadataJSON) != "" && strings.TrimSpace(metadataJSON) != "{}" {
		m := make(map[string]string)
		if jsonErr := json.Unmarshal([]byte(metadataJSON), &m); jsonErr != nil {
			return domain.Decision{}, fmt.Errorf("parse metadata_json: %w", jsonErr)
		}
		d.Metadata = m
	}

	// Set the matching payload pointer based on the row's kind column.
	switch kind := domain.DecisionKind(strings.TrimSpace(kindStr)); kind {
	case domain.DecisionKindLog:
		var conditions []string
		if strings.TrimSpace(invalidationConditionsJSON) != "" && strings.TrimSpace(invalidationConditionsJSON) != "[]" {
			if jsonErr := json.Unmarshal([]byte(invalidationConditionsJSON), &conditions); jsonErr != nil {
				return domain.Decision{}, fmt.Errorf("parse invalidation_conditions_json: %w", jsonErr)
			}
		}
		d.Log = &domain.LogPayload{
			Rationale:              strings.TrimSpace(rationale),
			Context:                strings.TrimSpace(contextValue),
			AlternativesRejected:   strings.TrimSpace(alternativesRejected),
			InvalidationConditions: conditions,
			Outcome:                strings.TrimSpace(outcome),
		}
	case domain.DecisionKindAskUser:
		var (
			questions []humaninput.Question
			answers   []humaninput.Answer
		)
		if strings.TrimSpace(questionsJSON) != "" && strings.TrimSpace(questionsJSON) != "[]" {
			if jsonErr := json.Unmarshal([]byte(questionsJSON), &questions); jsonErr != nil {
				return domain.Decision{}, fmt.Errorf("parse questions_json: %w", jsonErr)
			}
		}
		if strings.TrimSpace(answersJSON) != "" && strings.TrimSpace(answersJSON) != "[]" {
			if jsonErr := json.Unmarshal([]byte(answersJSON), &answers); jsonErr != nil {
				return domain.Decision{}, fmt.Errorf("parse answers_json: %w", jsonErr)
			}
		}
		d.AskUser = &domain.AskUserPayload{
			Context:   strings.TrimSpace(contextValue),
			Questions: questions,
			Answers:   answers,
			Cancelled: cancelled,
		}
	default:
		return domain.Decision{}, fmt.Errorf("scan decision: unknown kind %q", kind)
	}

	return d, nil
}
