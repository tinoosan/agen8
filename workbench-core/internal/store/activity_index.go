package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/pkg/types"
)

func shouldHideInboxOp(op, path string) bool {
	op = strings.TrimSpace(op)
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	if !strings.HasPrefix(path, "/inbox") {
		return false
	}
	switch op {
	case "fs.list", "fs.read":
		return true
	default:
		return false
	}
}

func singleLinePreview(s string, max int) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	if max < 2 {
		return s[:max]
	}
	return s[:max-1] + "…"
}

func activityTitleFromRequest(d map[string]string) string {
	op := strings.TrimSpace(d["op"])
	path := strings.TrimSpace(d["path"])

	switch op {
	case "fs.list":
		return "List " + path
	case "fs.read":
		return "Read " + path
	case "fs.search":
		q := strings.TrimSpace(d["query"])
		if path != "" && q != "" {
			return fmt.Sprintf("Search %s for %q", path, q)
		}
		if path != "" {
			return "Search " + path
		}
		return "Search"
	case "fs.write":
		return "Write " + path
	case "fs.append":
		return "Append " + path
	case "fs.edit":
		return "Edit " + path
	case "fs.patch":
		return "Patch " + path
	case "shell.exec", "shell_exec":
		if cmd := strings.TrimSpace(d["argvPreview"]); cmd != "" {
			return cmd
		}
		if argv0 := strings.TrimSpace(d["argv0"]); argv0 != "" {
			return argv0
		}
		return "shell.exec"
	case "http.fetch", "http_fetch":
		u := strings.TrimSpace(d["url"])
		if u != "" {
			m := strings.ToUpper(strings.TrimSpace(d["method"]))
			if m == "" {
				m = "GET"
			}
			desc := m + " " + u
			if body := strings.TrimSpace(d["body"]); body != "" {
				bodyText := "body: " + singleLinePreview(body, 120)
				if strings.TrimSpace(d["bodyTruncated"]) == "true" {
					bodyText += " truncated"
				}
				desc = desc + " " + bodyText
			}
			return desc
		}
		return "http.fetch"
	case "trace.run", "trace":
		action := strings.TrimSpace(d["traceAction"])
		key := strings.TrimSpace(d["traceKey"])
		if action != "" {
			if key != "" {
				return fmt.Sprintf("trace.%s %s", action, key)
			}
			return "trace." + action
		}
		return "trace.run"
	default:
		if op != "" && path != "" {
			return op + " " + path
		}
		if op != "" {
			return op
		}
		return "op"
	}
}

func activityIDFromEvent(ev types.EventRecord) string {
	if ev.Data != nil {
		if opID := strings.TrimSpace(ev.Data["opId"]); opID != "" {
			return opID
		}
	}
	if strings.TrimSpace(ev.EventID) != "" {
		return "event:" + strings.TrimSpace(ev.EventID)
	}
	return ""
}

func parseBool(s string) bool {
	return strings.TrimSpace(s) == "true"
}

func upsertActivityFromEventTx(tx *sql.Tx, runID string, eventSeq int64, ev types.EventRecord) error {
	if tx == nil {
		return fmt.Errorf("tx is nil")
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return fmt.Errorf("runID cannot be blank")
	}

	switch strings.TrimSpace(ev.Type) {
	case "agent.op.request":
		return upsertActivityRequestTx(tx, runID, eventSeq, ev)
	case "agent.op.response":
		return upsertActivityResponseTx(tx, runID, eventSeq, ev)
	default:
		return nil
	}
}

func upsertActivityRequestTx(tx *sql.Tx, runID string, eventSeq int64, ev types.EventRecord) error {
	op := ""
	if ev.Data != nil {
		op = strings.TrimSpace(ev.Data["op"])
	}
	if op == "" {
		return nil
	}
	if shouldHideInboxOp(op, strings.TrimSpace(ev.Data["path"])) {
		return nil
	}

	actID := activityIDFromEvent(ev)
	if actID == "" {
		return nil
	}
	ts := ev.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}

	act := types.Activity{
		ID:            actID,
		Kind:          op,
		Title:         activityTitleFromRequest(ev.Data),
		Status:        types.ActivityPending,
		StartedAt:     ts,
		Path:          strings.TrimSpace(ev.Data["path"]),
		MaxBytes:      strings.TrimSpace(ev.Data["maxBytes"]),
		TextPreview:   strings.TrimSpace(ev.Data["textPreview"]),
		TextTruncated: parseBool(ev.Data["textTruncated"]),
		TextRedacted:  parseBool(ev.Data["textRedacted"]),
		TextIsJSON:    parseBool(ev.Data["textIsJSON"]),
		TextBytes:     strings.TrimSpace(ev.Data["textBytes"]),
		Data:          ev.Data,
	}

	meta, err := json.Marshal(act)
	if err != nil {
		return fmt.Errorf("marshal activity meta: %w", err)
	}

	if _, err := tx.Exec(
		`INSERT INTO activities (run_id, activity_id, seq, kind, title, status, started_at, finished_at, meta_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, NULL, ?)
		 ON CONFLICT(activity_id) DO UPDATE SET
		   run_id=excluded.run_id,
		   seq=excluded.seq,
		   kind=excluded.kind,
		   title=excluded.title,
		   status=excluded.status,
		   started_at=excluded.started_at,
		   finished_at=NULL,
		   meta_json=excluded.meta_json`,
		runID,
		actID,
		eventSeq,
		act.Kind,
		act.Title,
		string(act.Status),
		ts.UTC().Format(time.RFC3339Nano),
		string(meta),
	); err != nil {
		return fmt.Errorf("insert activity: %w", err)
	}
	return nil
}

func upsertActivityResponseTx(tx *sql.Tx, runID string, _ int64, ev types.EventRecord) error {
	op := ""
	if ev.Data != nil {
		op = strings.TrimSpace(ev.Data["op"])
	}
	if op == "" {
		return nil
	}
	if shouldHideInboxOp(op, strings.TrimSpace(ev.Data["path"])) {
		return nil
	}

	ts := ev.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	fin := ts
	ok := strings.TrimSpace(ev.Data["ok"])

	newStatus := types.ActivityError
	if ok == "true" {
		newStatus = types.ActivityOK
	}

	targetID := ""
	if ev.Data != nil {
		targetID = strings.TrimSpace(ev.Data["opId"])
	}
	if targetID == "" {
		// Back-compat: if there's no opId, update the last pending activity for this run.
		_ = tx.QueryRow(
			`SELECT activity_id FROM activities WHERE run_id = ? AND status = ? ORDER BY seq DESC LIMIT 1`,
			runID,
			string(types.ActivityPending),
		).Scan(&targetID)
	}
	if strings.TrimSpace(targetID) == "" {
		return nil
	}

	var existingMeta sql.NullString
	var startedAtStr sql.NullString
	var kindStr sql.NullString
	var titleStr sql.NullString
	_ = tx.QueryRow(
		`SELECT meta_json, started_at, kind, title FROM activities WHERE activity_id = ? LIMIT 1`,
		targetID,
	).Scan(&existingMeta, &startedAtStr, &kindStr, &titleStr)

	act := types.Activity{
		ID:        targetID,
		Kind:      op,
		Title:     "",
		Status:    newStatus,
		StartedAt: ts,
		Data:      map[string]string{},
	}
	if kindStr.Valid {
		act.Kind = strings.TrimSpace(kindStr.String)
	}
	if titleStr.Valid {
		act.Title = strings.TrimSpace(titleStr.String)
	}
	if startedAtStr.Valid && strings.TrimSpace(startedAtStr.String) != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(startedAtStr.String)); err == nil {
			act.StartedAt = parsed
		}
	}

	if existingMeta.Valid && strings.TrimSpace(existingMeta.String) != "" {
		_ = json.Unmarshal([]byte(existingMeta.String), &act)
	}
	if act.Data == nil {
		act.Data = map[string]string{}
	}
	for k, v := range ev.Data {
		act.Data[k] = v
	}

	act.Ok = strings.TrimSpace(ev.Data["ok"])
	act.Error = strings.TrimSpace(ev.Data["err"])
	act.BytesLen = strings.TrimSpace(ev.Data["bytesLen"])
	act.Truncated = parseBool(ev.Data["truncated"])
	act.FinishedAt = &fin
	if !act.StartedAt.IsZero() {
		act.Duration = fin.Sub(act.StartedAt)
	}
	act.Status = newStatus

	meta, err := json.Marshal(act)
	if err != nil {
		return fmt.Errorf("marshal activity meta: %w", err)
	}

	res, err := tx.Exec(
		`UPDATE activities
		 SET status = ?, finished_at = ?, meta_json = ?
		 WHERE activity_id = ?`,
		string(newStatus),
		fin.UTC().Format(time.RFC3339Nano),
		string(meta),
		targetID,
	)
	if err != nil {
		return fmt.Errorf("update activity: %w", err)
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		return nil
	}
	// No matching request indexed; skip rather than inventing an activity.
	return nil
}
