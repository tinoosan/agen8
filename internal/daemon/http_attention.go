package daemon

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/tinoosan/agen8/internal/caller"
	"github.com/tinoosan/agen8/internal/services/attention"
	projectapp "github.com/tinoosan/agen8/internal/services/project/app"
	projectdomain "github.com/tinoosan/agen8/internal/services/project/domain/project"
)

const maxAttentionHookBodyBytes = 16 * 1024

// handleAttentionHook ingests one attention event from a harness hook script
// (Claude Code / Codex). Two body shapes are accepted:
//
//   - Raw mode (?harness=...&kind=... query params): the body is the harness's
//     own hook payload, piped through untouched — the installed hook is a bare
//     `curl --data-binary @-` one-liner with zero local dependencies, which
//     also works against a hosted agen8. The session id is extracted
//     server-side (both harnesses name it "session_id").
//   - Normalized mode (no kind param): the body is an attention.Event.
//
// The contract with the hook side is: ack fast, never block, never make the
// hook's failure the agent's problem — a bad payload gets a 4xx and the hook
// script still exits 0.
//
// Auth accepts the member's MCP API key (ak_..., already present in every
// harness MCP config) or a web session token, both as a bearer header.
func (d *Daemon) handleAttentionHook(w http.ResponseWriter, r *http.Request) {
	userID := d.attentionHookUserID(r)
	if userID == "" {
		http.Error(w, "valid bearer token is required", http.StatusUnauthorized)
		return
	}

	var ev attention.Event
	body := http.MaxBytesReader(w, r.Body, maxAttentionHookBodyBytes)
	if kind := strings.TrimSpace(r.URL.Query().Get("kind")); kind != "" {
		sessionRef, cwd := rawHookPayloadFields(body)
		ev = attention.Event{
			Harness:    strings.TrimSpace(r.URL.Query().Get("harness")),
			Kind:       attention.Kind(kind),
			SessionRef: sessionRef,
			Cwd:        cwd,
		}
	} else if err := json.NewDecoder(body).Decode(&ev); err != nil {
		http.Error(w, "invalid attention event payload", http.StatusBadRequest)
		return
	}
	// Member attribution goes through the project service, which scopes reads
	// to the caller in context — bind the authenticated user as that caller.
	ctx := caller.ContextWithCaller(r.Context(), caller.Caller{UserID: userID})
	entry, err := d.attention.Report(ctx, userID, ev)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":         true,
		"kind":       entry.Kind,
		"attributed": entry.MemberID != "",
	})
}

// rawHookPayloadFields pulls the session identifier and working directory out
// of a harness's own hook payload. Claude Code and Codex hooks both deliver
// stdin JSON with top-level "session_id" and "cwd". Returns empties on any
// parse failure — Report then rejects with a 400 and the hook script still
// exits 0.
func rawHookPayloadFields(body io.Reader) (sessionRef, cwd string) {
	var payload struct {
		SessionID string `json:"session_id"`
		Cwd       string `json:"cwd"`
	}
	if err := json.NewDecoder(body).Decode(&payload); err != nil {
		return "", ""
	}
	return strings.TrimSpace(payload.SessionID), strings.TrimSpace(payload.Cwd)
}

// projectDirLookup adapts the project service to attention.ProjectLookup:
// resolve a session's working directory to the project whose root contains it,
// preferring the longest (most specific) matching root.
type projectDirLookup struct {
	projects *projectapp.Service
}

func (l projectDirLookup) ProjectIDForDir(ctx context.Context, dir string) (string, bool) {
	dir = strings.TrimSpace(dir)
	if dir == "" || l.projects == nil {
		return "", false
	}
	all, err := l.projects.ListProjects(ctx, projectdomain.Filter{})
	if err != nil {
		return "", false
	}
	bestID, bestLen := "", 0
	for _, p := range all {
		root := strings.TrimRight(strings.TrimSpace(l.projects.ResolveRoot(ctx, p)), string(filepath.Separator))
		if root == "" || len(root) <= bestLen {
			continue
		}
		if dir == root || strings.HasPrefix(dir, root+string(filepath.Separator)) {
			bestID, bestLen = string(p.ID()), len(root)
		}
	}
	return bestID, bestID != ""
}
