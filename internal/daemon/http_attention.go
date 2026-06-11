package daemon

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/tinoosan/agen8/internal/caller"
	"github.com/tinoosan/agen8/internal/services/attention"
)

const maxAttentionHookBodyBytes = 16 * 1024

// handleAttentionHook ingests one normalized attention event from a harness
// hook script (Claude Code / Codex). The contract with the hook side is: ack
// fast, never block, never make the hook's failure the agent's problem — a bad
// payload gets a 4xx and the hook script still exits 0.
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
	if err := json.NewDecoder(body).Decode(&ev); err != nil {
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

// attentionHookUserID resolves the caller to a user id from the bearer token:
// MCP API key first (the credential hook scripts actually have), then a web
// session token.
func (d *Daemon) attentionHookUserID(r *http.Request) string {
	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		return ""
	}
	ctx := r.Context()
	if account, err := d.app.AuthSvc.ValidateAPIKey(ctx, token); err == nil {
		return strings.TrimSpace(account.ID.String())
	}
	if identity, err := d.httpIdentity(ctx, "Bearer "+token); err == nil {
		return strings.TrimSpace(identity.UserID)
	}
	return ""
}
