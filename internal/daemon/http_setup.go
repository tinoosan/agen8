package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	authapp "github.com/tinoosan/agen8/internal/services/auth/app"
	userapp "github.com/tinoosan/agen8/internal/services/user/app"
)

const maxSetupRequestBodyBytes = 64 * 1024

type httpSetupHandler struct {
	auth       *authapp.Service
	users      *userapp.Service
	setupToken string
	httpAddr   string
	publicURL  string
}

func (d *Daemon) setupHandler() httpSetupHandler {
	if d == nil || d.app == nil {
		return httpSetupHandler{}
	}
	return httpSetupHandler{
		auth:       d.app.AuthSvc,
		users:      d.app.UserSvc,
		setupToken: d.cfg.SetupToken,
		httpAddr:   d.cfg.HTTPAddr,
		publicURL:  d.cfg.PublicURL,
	}
}

func (d *Daemon) setupAvailable(ctx context.Context) bool {
	return d.setupHandler().available(ctx)
}

func (h httpSetupHandler) handlePage(w http.ResponseWriter, r *http.Request) {
	if !h.available(r.Context()) || !h.validToken(r.URL.Query().Get("token")) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, setupPageHTML(h.setupToken))
}

func (h httpSetupHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	if !h.available(r.Context()) {
		http.Error(w, "setup is closed", http.StatusConflict)
		return
	}
	if _, err := readRequestBody(r, maxSetupRequestBodyBytes); err != nil {
		if errors.Is(err, errRPCRequestBodyTooLarge) {
			http.Error(w, "setup request body is too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "read setup request body", http.StatusBadRequest)
		return
	}
	req, err := decodeSetupRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !h.validToken(req.Token) {
		http.Error(w, "invalid setup token", http.StatusForbidden)
		return
	}
	created, createErr := h.setupCreator().create(r.Context(), req)
	if createErr != nil {
		http.Error(w, createErr.clientMessage, createErr.status)
		return
	}
	// Setup exposes the MCP API key once, while the web session remains confined
	// to a server-managed cookie.
	// #nosec G124 -- Secure is enabled for direct/proxied TLS and HTTPS PublicURL;
	// loopback HTTP development must remain usable.
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    created.sessionToken,
		Path:     "/",
		Expires:  created.sessionExpiresAt.UTC(),
		HttpOnly: true,
		Secure:   requestUsesHTTPS(r) || strings.HasPrefix(strings.ToLower(strings.TrimSpace(h.publicURL)), "https://"),
		SameSite: http.SameSiteStrictMode,
	})
	mcpArtifacts, err := h.setupMCPArtifacts(r, created.apiKeySecret)
	if err != nil {
		http.Error(w, "create setup mcp artifacts", http.StatusInternalServerError)
		return
	}
	if !setupWantsJSON(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, setupCompleteHTML(created.apiKeySecret, mcpArtifacts))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"user":    created.user,
		"session": map[string]any{"expiresAt": created.sessionExpiresAt},
		"apiKey":  map[string]any{"id": created.apiKey.ID.String(), "name": created.apiKey.Name, "prefix": created.apiKey.Prefix, "secret": created.apiKeySecret},
		"mcp":     mcpArtifacts,
	})
}

func (h httpSetupHandler) available(ctx context.Context) bool {
	if h.users == nil {
		return false
	}
	open, err := h.users.SetupOpen(ctx)
	return err == nil && open
}

func (h httpSetupHandler) validToken(token string) bool {
	return strings.TrimSpace(token) != "" && strings.TrimSpace(token) == strings.TrimSpace(h.setupToken)
}
